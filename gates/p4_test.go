package gates

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

// lastPrice is the (price, seq) a correct fold would carry for one
// instrument at upTo: the price event with the greatest (effective, seq)
// among that instrument's price events in the visible prefix.
type lastPrice struct{ Price, Seq int64 }

// trueLastPrices independently recomputes, straight from the feed, the
// last-known price per instrument in [1..upTo] -- the same tie-break rule
// fold.Fold's own pass 3 documents (later effective date wins; same
// effective date, higher seq wins). This is deliberately NOT read off any
// already-built snapshot: it is the source-of-truth P4's checks measure
// candidate valuations against, so it has to come from the feed itself,
// not from the artifact under test.
func trueLastPrices(records []feed.Record, upTo int64) map[string]lastPrice {
	out := map[string]lastPrice{}
	lastEff := map[string]string{}
	for _, r := range records {
		if r.Seq > upTo || r.Type != "price" {
			continue
		}
		inst, ok := r.Payload["instrument"].(string)
		if !ok {
			continue
		}
		pxN, ok := r.Payload["price"].(json.Number)
		if !ok {
			continue
		}
		px, err := pxN.Int64()
		if err != nil {
			continue
		}
		prevEff, seen := lastEff[inst]
		if !seen || r.Effective > prevEff || (r.Effective == prevEff && r.Seq > out[inst].Seq) {
			out[inst] = lastPrice{Price: px, Seq: r.Seq}
			lastEff[inst] = r.Effective
		}
	}
	return out
}

// mustInt64 reads a decoded JSON number as int64. It panics rather than
// defaulting to 0 on anything unreadable -- a valuation field that fails to
// parse as a number is itself a defect in the artifact under test, and this
// gate exists to fail closed on exactly that class of problem, not to let
// an unparseable value quietly masquerade as a legitimate zero. Used for
// "qty"/"total_cost": a position that exists at all always carries these
// (snapshot.Build never emits a position without them), so a missing or
// malformed one here means the document itself is broken in a way outside
// P4's scope, and a loud crash is the right report.
func mustInt64(v any) int64 {
	n, ok := v.(json.Number)
	if !ok {
		panic(fmt.Sprintf("p4: expected a JSON number, got %T (%v)", v, v))
	}
	i, err := n.Int64()
	if err != nil {
		panic(fmt.Sprintf("p4: %v does not parse as int64: %v", n, err))
	}
	return i
}

// int64OrZero reads a decoded JSON number as int64, reporting false instead
// of panicking when the value is missing or unparseable. Used ONLY for the
// three valuation sub-fields (price, price_seq, unrealized): unlike
// qty/total_cost, snapshot.Build never emits a partially-populated
// valuation object, but the twin fixtures in this package are hand-built
// JSON from an independent generator, not required to go through Build --
// a `"valuation": {}` (an object present but empty, so it does not hit the
// val == nil guard below) is a legal JSON shape nothing here rules out. A
// missing/malformed price or price_seq is, if anything, a WORSE case of
// exactly what silent_zero exists to catch -- "nothing was known, dressed
// up as though it were known" -- so callers fold a false ok into the same
// zero-valued branch that price == 0 already takes, rather than crash the
// gate on a shape that is itself evidence of the defect being measured.
func int64OrZero(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	if err != nil {
		return 0, false
	}
	return i, true
}

// nullValuationInstruments returns the sorted instrument keys of doc's
// "positions" whose "valuation" is null (or otherwise not an object) --
// the same null-ness test p4Check's own loop uses to decide a position has
// no valuation at all. This is the "unpriced" side of the biconditional
// undeclared_unpriced checks: valuation is null if and only if the
// instrument is listed in "unevaluable". A ledger that stops pricing a
// position without declaring it unevaluable breaks that biconditional
// while leaving every other check quiet -- silent_zero and
// stale_carry_forward only ever look at positions that DO carry a
// valuation, so a dropped valuation is invisible to both of them.
func nullValuationInstruments(doc snapshot.Doc) []string {
	pos, _ := doc["positions"].(map[string]any)
	out := make([]string, 0, len(pos))
	for inst, raw := range pos {
		p, _ := raw.(map[string]any)
		if _, ok := p["valuation"].(map[string]any); !ok {
			out = append(out, inst)
		}
	}
	sort.Strings(out)
	return out
}

// p4Check evaluates fail-closed valuation over a snapshot document:
//
//   - silent_zero: a valued position's price and price_seq are each
//     checked, independently, for being 0 -- a valuation of price 0 or
//     price_seq 0 is a position valued at zero because nothing was known
//     about it, dressed up as though it were known.
//   - stale_carry_forward: a valued position's price, price_seq, and the
//     unrealized figure derived from them are each checked, independently
//     in principle, against the instrument's OWN true last price
//     recomputed straight from the feed (trueLastPrices) -- not merely
//     "does some price exist at this seq," which a carried-forward price
//     from a DIFFERENT instrument's price event would satisfy. "Independent
//     in principle" is deliberately qualified: a document could carry a
//     correct price with an independently-wrong unrealized, and this leg
//     exists to catch exactly that case, which is why it stays even though
//     it does not add independent evidence on the current twin. On the
//     DDD twin specifically, the unrealized leg is an arithmetic
//     CONSEQUENCE of the price leg, not a third independent failure: the
//     twin's own stored unrealized already equals qty*price-total_cost for
//     DDD's fabricated price (qty=77 != 0), so a wrong price there forces a
//     wrong unrealized by construction. The count of 3 for DDD therefore
//     reflects two independent failures (price, price_seq) and one
//     consequence of the first, not three independent ones.
//
// A position whose price or price_seq is 0 is not also run through
// stale_carry_forward: it already failed closed by a stale (or absent)
// pointer, so there is nothing more meaningful to say about which price
// event that zero "points at."
//
// This function deliberately does NOT also check "doc's unevaluable set
// equals the generator's withheld list" (p4.withheld): that measurement is
// identical to the unevaluable_match_manifest check wired at each call
// site (both compare UnevaluableInstruments(doc) to a set that is, today,
// exactly {"CCC"}), so keeping a second check under a second name would be
// one planted fact counted twice. unevaluable_match_manifest is kept
// because unevaluable_at is the cross-gate oracle P1/P3/P4 all read and
// the generator asserts independently by dying on a mismatch; withheld is
// P4-only plant metadata with no independent authority of its own.
func p4Check(doc snapshot.Doc, truth map[string]lastPrice) Counts {
	c := NewCounts("silent_zero", "stale_carry_forward")

	pos, _ := doc["positions"].(map[string]any)
	for inst, raw := range pos {
		p, _ := raw.(map[string]any)
		val, _ := p["valuation"].(map[string]any)
		if val == nil {
			continue
		}
		price, priceOK := int64OrZero(val["price"])
		seq, seqOK := int64OrZero(val["price_seq"])
		c.Evaluated["silent_zero"] += 2
		zero := false
		if !priceOK || price == 0 {
			c.Checks["silent_zero"]++
			zero = true
		}
		if !seqOK || seq == 0 {
			c.Checks["silent_zero"]++
			zero = true
		}
		if zero {
			continue
		}
		c.Evaluated["stale_carry_forward"] += 3
		want, ok := truth[inst]
		if !ok {
			// A valued position for an instrument with no true price event
			// at all in the prefix cannot be pointing at a legitimate one:
			// every leg this check would otherwise compare is wrong.
			c.Checks["stale_carry_forward"] += 3
			continue
		}
		if price != want.Price {
			c.Checks["stale_carry_forward"]++
		}
		if seq != want.Seq {
			c.Checks["stale_carry_forward"]++
		}
		qty, totalCost := mustInt64(p["qty"]), mustInt64(p["total_cost"])
		wantUnrealized := qty*want.Price - totalCost
		unrealized, urOK := int64OrZero(val["unrealized"])
		if !urOK || unrealized != wantUnrealized {
			c.Checks["stale_carry_forward"]++
		}
	}
	return c
}

func TestP4FailClosedValuation(t *testing.T) {
	m := LoadManifest(t)
	withheld := m.Strs("p4", "withheld")
	f, err := feed.Open(filepath.Join(FixturesDir, "base", "feed.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	records := f.Records()
	f.Close()
	end := m.Int("end_seq")
	truth := trueLastPrices(records, end)
	params := map[string]any{"withheld": withheld, "viewpoint": end}
	positionsWant := m.Strs("positions_at", "V3")
	unevWant := m.Strs("unevaluable_at", "V3")

	live := ReadFixture(t, "base/feed.jsonl", -1)
	c := p4Check(live.Doc, truth)
	SetEquality(c, "positions_match_manifest", PositionKeys(live.Doc), positionsWant)
	SetEqualityOverUniverse(c, "unevaluable_match_manifest", UnevaluableInstruments(live.Doc), unevWant, positionsWant)
	SetEqualityOverUniverse(c, "undeclared_unpriced", nullValuationInstruments(live.Doc), UnevaluableInstruments(live.Doc), positionsWant)
	Emit(t, Row{Prop: 4, Cell: "live", Scope: "fixtures/base end-of-feed snapshot; CCC price withheld by the generator", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: params, Counts: c})

	// twin: a fabricated zero (CCC) and a stale carry-forward (DDD from
	// BBB). Both instruments still carry a (fabricated) valuation object,
	// so this twin's own "unevaluable" list still equals {CCC} and
	// undeclared_unpriced reads 0 here -- it is the OTHER twin below that
	// isolates this check.
	twin := LoadDoc(t, filepath.Join(FixturesDir, "p4", "twin", "snapshot.json"))
	ct := p4Check(twin, truth)
	SetEquality(ct, "positions_match_manifest", PositionKeys(twin), positionsWant)
	SetEqualityOverUniverse(ct, "unevaluable_match_manifest", UnevaluableInstruments(twin), unevWant, positionsWant)
	SetEqualityOverUniverse(ct, "undeclared_unpriced", nullValuationInstruments(twin), UnevaluableInstruments(twin), positionsWant)
	raw, _ := json.Marshal(twin)
	Emit(t, Row{Prop: 4, Cell: "twin", Scope: "snapshot with a silent zero (CCC) and a stale carry-forward (DDD from BBB)", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the twin document as decoded and re-marshaled", Rows: int64(len(twin["positions"].(map[string]any))), Params: params, Counts: ct, Planted: ptr(m.Planted("p4"))})

	// twin_silent_omission: DDD's valuation is dropped (null) WITHOUT
	// adding DDD to "unevaluable" -- the pure silent-omission defect,
	// isolated from the other two so that when this twin goes red it is
	// for exactly one reason. Its manifest expectations name only
	// positions_match_manifest, unevaluable_match_manifest, and
	// undeclared_unpriced; silent_zero, stale_carry_forward, and
	// unevaluable_matches_planted are deliberately NOT computed for this
	// cell -- p4Check is not called here, because Emit refuses a twin row
	// whose Checks and planted expectations differ over the union of
	// their keys, and this twin plants no expectation for those three.
	omitted := LoadDoc(t, filepath.Join(FixturesDir, "p4", "twin-silent-omission", "snapshot.json"))
	co := NewCounts("positions_match_manifest", "unevaluable_match_manifest", "undeclared_unpriced")
	SetEquality(co, "positions_match_manifest", PositionKeys(omitted), positionsWant)
	SetEqualityOverUniverse(co, "unevaluable_match_manifest", UnevaluableInstruments(omitted), unevWant, positionsWant)
	SetEqualityOverUniverse(co, "undeclared_unpriced", nullValuationInstruments(omitted), UnevaluableInstruments(omitted), positionsWant)
	rawOmitted, _ := json.Marshal(omitted)
	Emit(t, Row{Prop: 4, Cell: "twin", Scope: "snapshot with DDD's valuation silently dropped, never declared unevaluable", ContentHash: "sha256:" + sha256Hex(rawOmitted),
		Basis: "sha256 of the twin document as decoded and re-marshaled", Rows: int64(len(omitted["positions"].(map[string]any))), Params: params, Counts: co, Planted: ptr(m.Planted("p4", "twin_silent_omission"))})
}
