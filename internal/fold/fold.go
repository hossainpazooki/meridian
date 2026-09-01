// Package fold turns a feed prefix into ledger state. It is pure: no I/O, no
// clock, no floats; the same records in the same order always produce the
// same State. Malformed input becomes a refusal record, never an error.
package fold

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/feed"
)

type Position struct{ Qty, TotalCost int64 }
type Valuation struct{ Price, PriceSeq, Unrealized int64 }
type Absorbed struct {
	Seq          int64
	EventID, Key string
}
type Refusal struct {
	Seq                        int64
	EventID, Key, Kind, Detail string
}
type Unevaluable struct{ Instrument, Reason string }

// State is the complete fold output at Seq.
type State struct {
	Seq            int64
	Cash           int64
	RealizedPnL    int64
	DividendIncome int64
	Positions      map[string]Position
	Valuations     map[string]Valuation
	Unevaluable    []Unevaluable
	Absorbed       []Absorbed
	Refusals       []Refusal
}

// UnrealizedPnL sums valued positions only.
func (s State) UnrealizedPnL() int64 {
	var t int64
	for _, v := range s.Valuations {
		t += v.Unrealized
	}
	return t
}

// FillKey is the ledger-derived idempotency key: sha256 of canonical
// {"trade_id","venue"}. Never producer-supplied.
func FillKey(payload map[string]any) (string, error) {
	tid, ok1 := payload["trade_id"].(string)
	ven, ok2 := payload["venue"].(string)
	if !ok1 || !ok2 || tid == "" || ven == "" {
		return "", fmt.Errorf("fill identity fields missing")
	}
	b, err := canon.Marshal(map[string]any{"trade_id": tid, "venue": ven})
	if err != nil {
		return "", err
	}
	return canon.SHA256Hex(b), nil
}

// PayloadHash is sha256 of the canonical payload; "" if it cannot marshal.
// "" is not a valid hash value and must never be compared for equality
// against another PayloadHash result to decide duplicate-vs-collision: two
// payloads that both fail to canonicalize would then compare equal by
// accident. Fold does not call this for that reason -- it detects the
// canonicalization failure itself and refuses the record as malformed
// instead. This function is exposed for callers that only need the hash
// of a payload already known to canonicalize.
func PayloadHash(payload map[string]any) string {
	b, err := canon.Marshal(payload)
	if err != nil {
		return ""
	}
	return canon.SHA256Hex(b)
}

func getInt(m map[string]any, k string) (int64, bool) {
	n, ok := m[k].(json.Number)
	if !ok {
		return 0, false
	}
	v, err := n.Int64()
	return v, err == nil
}

func getStr(m map[string]any, k string) (string, bool) {
	s, ok := m[k].(string)
	return s, ok && s != ""
}

// mulOverflows reports whether a*b is not representable as an int64. Every
// call site in this package supplies non-negative operands -- qty, price,
// ratio, rate, and position Qty/TotalCost are never negative by
// construction -- so this pre-check (rather than computing the product and
// inspecting it after the fact) is sufficient and sidesteps the
// MinInt64/-1 edge case a general-purpose signed version would need to
// special-case.
func mulOverflows(a, b int64) bool {
	if a < 0 || b < 0 {
		panic("fold: mulOverflows requires non-negative operands")
	}
	if a == 0 || b == 0 {
		return false
	}
	return a > math.MaxInt64/b
}

// addOverflows reports whether a+b is not representable as an int64. Cash
// and RealizedPnL are signed (they go negative), so this is the general
// wraparound-based check; Go defines signed-integer overflow as
// two's-complement wraparound, so relying on the computed (wrapped) sum to
// detect its own overflow is well-defined, not undefined behavior.
func addOverflows(a, b int64) bool {
	c := a + b
	return (b > 0 && c < a) || (b < 0 && c > a)
}

// applicable is one resolved event ready to apply in (effective, seq) order.
type applicable struct {
	seq        int64
	effective  string
	kind       string // buy | sell | split | dividend | price
	instrument string
	qty, price int64 // fills
	ratio      int64 // split
	rate       int64 // dividend
}

// Fold folds records with Seq <= upTo per the shared contract.
func Fold(records []feed.Record, upTo int64) (State, error) {
	if upTo < 0 || upTo > int64(len(records)) {
		return State{}, fmt.Errorf("fold: upTo %d out of range [0,%d]", upTo, len(records))
	}
	st := State{Seq: upTo, Positions: map[string]Position{}, Valuations: map[string]Valuation{},
		Unevaluable: []Unevaluable{}, Absorbed: []Absorbed{}, Refusals: []Refusal{}}
	refuse := func(r feed.Record, key, kind, detail string) {
		st.Refusals = append(st.Refusals, Refusal{Seq: r.Seq, EventID: r.ID, Key: key, Kind: kind, Detail: detail})
	}

	// The visible prefix is every record whose Seq is <= upTo, not the
	// first upTo elements of the slice. For a feed-produced slice the two
	// coincide (Seq == index+1, contiguous from 1), but Fold's contract is
	// stated in terms of Seq and must hold for any slice a caller hands
	// it -- including a hand-built, non-contiguous, or out-of-order one.
	// Order is preserved from the input slice; later passes re-sort by
	// (effective, seq) where order matters for application.
	prefix := make([]feed.Record, 0, len(records))
	for _, r := range records {
		if r.Seq <= upTo {
			prefix = append(prefix, r)
		}
	}

	// Pass 1: action terms, then amendments overwrite in seq order.
	type action struct {
		rec        feed.Record
		instrument string
		kind       string
		ratio      int64
		rate       int64
	}
	actions := map[string]*action{}
	for _, r := range prefix {
		if r.Type != "action" {
			continue
		}
		id, ok1 := getStr(r.Payload, "action_id")
		inst, ok2 := getStr(r.Payload, "instrument")
		kind, ok3 := getStr(r.Payload, "kind")
		if !ok1 || !ok2 || !ok3 {
			refuse(r, "", "malformed", "action missing action_id/instrument/kind")
			continue
		}
		if _, dup := actions[id]; dup {
			refuse(r, "", "malformed", "duplicate action_id "+id)
			continue
		}
		a := &action{rec: r, instrument: inst, kind: kind}
		switch kind {
		case "split":
			if v, ok := getInt(r.Payload, "ratio"); ok && v >= 1 {
				a.ratio = v
			} else {
				refuse(r, "", "malformed", "split ratio must be a whole number >= 1")
				continue
			}
		case "dividend":
			if v, ok := getInt(r.Payload, "rate"); ok && v >= 0 {
				a.rate = v
			} else {
				refuse(r, "", "malformed", "dividend rate must be an integer >= 0")
				continue
			}
		default:
			refuse(r, "", "malformed", "unknown action kind "+kind)
			continue
		}
		actions[id] = a
	}
	for _, r := range prefix {
		if r.Type != "action_amendment" {
			continue
		}
		id, ok := getStr(r.Payload, "action_id")
		if !ok {
			refuse(r, "", "malformed", "amendment missing action_id")
			continue
		}
		a, ok := actions[id]
		if !ok {
			refuse(r, "", "unknown_action", "amendment references unknown action "+id)
			continue
		}
		switch a.kind {
		case "split":
			if v, ok := getInt(r.Payload, "ratio"); ok && v >= 1 {
				a.ratio = v
			} else {
				refuse(r, "", "malformed", "amendment ratio must be a whole number >= 1")
			}
		case "dividend":
			if v, ok := getInt(r.Payload, "rate"); ok && v >= 0 {
				a.rate = v
			} else {
				refuse(r, "", "malformed", "amendment rate must be an integer >= 0")
			}
		}
	}

	// Pass 2: dedupe fills, decode prices, collect applicables.
	var apps []applicable
	seen := map[string]string{} // key -> payload hash
	for _, r := range prefix {
		switch r.Type {
		case "fill":
			key, err := FillKey(r.Payload)
			if err != nil {
				refuse(r, "", "malformed", err.Error())
				continue
			}
			// The payload hash must be computed here, not via PayloadHash,
			// so a canonicalization failure is visible as an error rather
			// than collapsing to "". Two different payloads that both fail
			// to canonicalize would otherwise hash equal (both "") and the
			// second would be silently absorbed as a duplicate of the
			// first -- a real hole in the at-most-once guarantee. If the
			// fold cannot compute the hash, it cannot tell a duplicate
			// from a collision, so it must refuse rather than guess -- the
			// same fail-closed discipline as an unpriced position going to
			// Unevaluable instead of a fabricated valuation.
			phBytes, hashErr := canon.Marshal(r.Payload)
			if hashErr != nil {
				refuse(r, key, "malformed", "payload cannot canonicalize: "+hashErr.Error())
				continue
			}
			ph := canon.SHA256Hex(phBytes)
			if prev, dup := seen[key]; dup {
				if prev == ph {
					st.Absorbed = append(st.Absorbed, Absorbed{Seq: r.Seq, EventID: r.ID, Key: key})
				} else {
					refuse(r, key, "collision", "payload hash mismatch")
				}
				continue
			}
			inst, ok1 := getStr(r.Payload, "instrument")
			side, ok2 := getStr(r.Payload, "side")
			qty, ok3 := getInt(r.Payload, "qty")
			px, ok4 := getInt(r.Payload, "price")
			if !ok1 || !ok2 || !ok3 || !ok4 || qty <= 0 || px <= 0 || (side != "buy" && side != "sell") {
				refuse(r, key, "malformed", "fill fields invalid")
				continue
			}
			seen[key] = ph
			apps = append(apps, applicable{seq: r.Seq, effective: r.Effective, kind: side, instrument: inst, qty: qty, price: px})
		case "price":
			inst, ok1 := getStr(r.Payload, "instrument")
			px, ok2 := getInt(r.Payload, "price")
			if !ok1 || !ok2 || px <= 0 {
				refuse(r, "", "malformed", "price fields invalid")
				continue
			}
			apps = append(apps, applicable{seq: r.Seq, effective: r.Effective, kind: "price", instrument: inst, price: px})
		case "action", "action_amendment":
			// handled in pass 1
		default:
			refuse(r, "", "malformed", "unknown event type "+r.Type)
		}
	}
	for _, a := range actions {
		apps = append(apps, applicable{seq: a.rec.Seq, effective: a.rec.Effective, kind: a.kind, instrument: a.instrument, ratio: a.ratio, rate: a.rate})
	}
	sort.SliceStable(apps, func(i, j int) bool {
		if apps[i].effective != apps[j].effective {
			return apps[i].effective < apps[j].effective
		}
		return apps[i].seq < apps[j].seq
	})

	// Pass 3: apply.
	lastPrice := map[string]Valuation{}
	lastEff := map[string]string{}
	bySeq := map[int64]feed.Record{}
	for _, r := range prefix {
		bySeq[r.Seq] = r
	}
	// refuseApp refuses the record behind app seq s. FillKey returns "" on
	// a non-fill payload (an action's payload has no trade_id/venue), which
	// matches the "" key already used for non-fill refusals elsewhere.
	refuseApp := func(seq int64, kind, detail string) {
		r := bySeq[seq]
		key, _ := FillKey(r.Payload)
		refuse(r, key, kind, detail)
	}
	for _, a := range apps {
		p := st.Positions[a.instrument]
		switch a.kind {
		case "buy":
			// An event whose arithmetic is not representable in int64 is an
			// input defect of the same class as a non-positive qty/price:
			// refuse it and leave all running state untouched, rather than
			// let a wrapped-around value corrupt the position, cash, or a
			// later RelieveCost call (whose own invariant guard exists to
			// catch a caller bug, not to be reachable from feed input). The
			// running-total checks (TotalCost, Cash) matter as much as the
			// per-event product check: two individually-representable buys
			// can still overflow when accumulated.
			if mulOverflows(a.qty, a.price) {
				refuseApp(a.seq, "malformed", "buy cost (qty*price) overflows int64")
				continue
			}
			cost := a.qty * a.price
			if addOverflows(p.Qty, a.qty) {
				refuseApp(a.seq, "malformed", "buy would overflow position quantity")
				continue
			}
			if addOverflows(p.TotalCost, cost) {
				refuseApp(a.seq, "malformed", "buy would overflow position total cost")
				continue
			}
			if addOverflows(st.Cash, -cost) {
				refuseApp(a.seq, "malformed", "buy would overflow cash balance")
				continue
			}
			p.Qty += a.qty
			p.TotalCost += cost
			st.Cash -= cost
			st.Positions[a.instrument] = p
		case "sell":
			if a.qty > p.Qty {
				refuseApp(a.seq, "oversell", fmt.Sprintf("sell %d exceeds held %d", a.qty, p.Qty))
				continue
			}
			if mulOverflows(a.qty, a.price) {
				refuseApp(a.seq, "malformed", "sell proceeds (qty*price) overflows int64")
				continue
			}
			proceeds := a.qty * a.price
			if addOverflows(st.Cash, proceeds) {
				refuseApp(a.seq, "malformed", "sell would overflow cash balance")
				continue
			}
			relieved := RelieveCost(p.TotalCost, a.qty, p.Qty)
			// diff is always representable: proceeds and relieved are both
			// non-negative and <= MaxInt64 (relieved <= p.TotalCost by
			// RelieveCost's own invariant), and the difference of two such
			// values is always in [-MaxInt64, MaxInt64], which never hits
			// the unrepresentable MinInt64 edge.
			diff := proceeds - relieved
			if addOverflows(st.RealizedPnL, diff) {
				refuseApp(a.seq, "malformed", "sell would overflow realized P&L")
				continue
			}
			p.TotalCost -= relieved
			p.Qty -= a.qty
			st.Cash += proceeds
			st.RealizedPnL += diff
			if p.Qty == 0 {
				if p.TotalCost != 0 {
					panic("fold: cost remains after full relief")
				}
				delete(st.Positions, a.instrument)
			} else {
				st.Positions[a.instrument] = p
			}
		case "split":
			if _, ok := st.Positions[a.instrument]; ok {
				if mulOverflows(p.Qty, a.ratio) {
					refuseApp(a.seq, "malformed", "split qty*ratio overflows int64")
					continue
				}
				p.Qty *= a.ratio
				st.Positions[a.instrument] = p
			}
		case "dividend":
			if _, ok := st.Positions[a.instrument]; ok && p.Qty > 0 {
				if mulOverflows(p.Qty, a.rate) {
					refuseApp(a.seq, "malformed", "dividend qty*rate overflows int64")
					continue
				}
				d := p.Qty * a.rate
				if addOverflows(st.Cash, d) {
					refuseApp(a.seq, "malformed", "dividend would overflow cash balance")
					continue
				}
				if addOverflows(st.DividendIncome, d) {
					refuseApp(a.seq, "malformed", "dividend would overflow dividend income")
					continue
				}
				st.Cash += d
				st.DividendIncome += d
			}
		case "price":
			if prevEff, ok := lastEff[a.instrument]; !ok || a.effective > prevEff || (a.effective == prevEff && a.seq > lastPrice[a.instrument].PriceSeq) {
				lastPrice[a.instrument] = Valuation{Price: a.price, PriceSeq: a.seq}
				lastEff[a.instrument] = a.effective
			}
		}
	}

	// Pass 4: valuation. A fill that already applied cannot be retroactively
	// refused, so an unrepresentable qty*Price here is reported as
	// Unevaluable (distinct reason) rather than a wrapped-around,
	// plausible-looking Unrealized number -- the same fail-closed choice
	// P4 already makes for a position with no price in the prefix.
	candidates := map[string]Valuation{}
	for inst, p := range st.Positions {
		if v, ok := lastPrice[inst]; ok {
			if mulOverflows(p.Qty, v.Price) {
				st.Unevaluable = append(st.Unevaluable, Unevaluable{Instrument: inst, Reason: "valuation_overflow"})
				continue
			}
			// p.Qty*v.Price - p.TotalCost is always representable: the
			// product didn't overflow (checked above) so it's in
			// [0,MaxInt64], TotalCost is in [0,MaxInt64] by construction
			// (Pass 3 never lets it overflow), and the difference of two
			// such values is always in [-MaxInt64, MaxInt64].
			v.Unrealized = p.Qty*v.Price - p.TotalCost
			candidates[inst] = v
		} else {
			st.Unevaluable = append(st.Unevaluable, Unevaluable{Instrument: inst, Reason: "no_price_in_prefix"})
		}
	}

	// The AGGREGATE sum across positions (UnrealizedPnL) can overflow even
	// when every individual Unrealized value is representable -- the same
	// running-total-vs-per-item distinction the buy/sell accumulation
	// guards above already had to make (two individually-representable
	// terms can still overflow when added together). To decide membership
	// deterministically -- Go map iteration order is randomized, and this
	// fold must produce byte-for-byte identical State for the same input,
	// see the package doc -- candidates are walked in a fixed order
	// (sorted by instrument, the same key already used to order the final
	// Unevaluable list) and folded into a running total with the same
	// addOverflows check used everywhere else; any position whose
	// inclusion would overflow that running total is evicted to
	// Unevaluable (reusing "valuation_overflow": an overflow is still what
	// prevents this position from getting a trustworthy published number,
	// whether the overflow is in its own product or in the running total
	// it would join) instead of being published.
	//
	// This is sufficient, not merely per-term-safe: because every accepted
	// step's addition is checked before being applied, the running total
	// after processing all accepted candidates equals their true,
	// infinite-precision sum -- not a wrapped approximation of it -- and
	// that sum is a valid int64 (the loop only ever advances when it is).
	// Go's int64 addition is exact modular arithmetic (two's complement,
	// mod 2^64): summing the SAME accepted values in any other order --
	// including UnrealizedPnL's unordered range over st.Valuations --
	// reduces to that identical, already-in-range true sum, bit for bit.
	// So State.UnrealizedPnL() needs no change and no error channel: the
	// fold guarantees by construction that whatever Valuations it
	// publishes has a representable aggregate.
	instruments := make([]string, 0, len(candidates))
	for inst := range candidates {
		instruments = append(instruments, inst)
	}
	sort.Strings(instruments)
	var aggregate int64
	for _, inst := range instruments {
		v := candidates[inst]
		if addOverflows(aggregate, v.Unrealized) {
			st.Unevaluable = append(st.Unevaluable, Unevaluable{Instrument: inst, Reason: "valuation_overflow"})
			continue
		}
		aggregate += v.Unrealized
		st.Valuations[inst] = v
	}
	sort.Slice(st.Unevaluable, func(i, j int) bool { return st.Unevaluable[i].Instrument < st.Unevaluable[j].Instrument })
	sort.SliceStable(st.Refusals, func(i, j int) bool { return st.Refusals[i].Seq < st.Refusals[j].Seq })
	sort.SliceStable(st.Absorbed, func(i, j int) bool { return st.Absorbed[i].Seq < st.Absorbed[j].Seq })
	return st, nil
}
