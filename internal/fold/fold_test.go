package fold

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"

	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/feed"
)

func num(i int) json.Number { return json.Number(strconv.Itoa(i)) }

type ev struct {
	typ, eff string
	p        map[string]any
}

func fill(eff, inst, side string, qty, price int, tid string) ev {
	return ev{"fill", eff, map[string]any{"instrument": inst, "price": num(price), "qty": num(qty), "side": side, "trade_id": tid, "venue": "X"}}
}
func price(eff, inst string, px int) ev {
	return ev{"price", eff, map[string]any{"instrument": inst, "price": num(px)}}
}
func split(eff, id, inst string, ratio int) ev {
	return ev{"action", eff, map[string]any{"action_id": id, "announced": eff, "instrument": inst, "kind": "split", "processed": eff, "ratio": num(ratio)}}
}
func dividend(eff, id, inst string, rate int) ev {
	return ev{"action", eff, map[string]any{"action_id": id, "announced": eff, "instrument": inst, "kind": "dividend", "processed": eff, "rate": num(rate)}}
}
func amendRatio(eff, id string, ratio int) ev {
	return ev{"action_amendment", eff, map[string]any{"action_id": id, "ratio": num(ratio)}}
}

func records(evs ...ev) []feed.Record {
	out := make([]feed.Record, len(evs))
	for i, e := range evs {
		out[i] = feed.Record{Seq: int64(i + 1), Type: e.typ, ID: "ev-" + strconv.Itoa(i+1), Effective: e.eff, Payload: e.p}
	}
	return out
}

// Hand-computed golden (see Task 9 p6): AAA buy 100@1000, buy 50@1301,
// split 2, sell 120@700, dividend 25, price 800.
func TestFoldGoldenAAA(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 100, 1000, "T-1"),
		fill("2026-01-06", "AAA", "buy", 50, 1301, "T-2"),
		split("2026-01-10", "CA-1", "AAA", 2),
		fill("2026-01-12", "AAA", "sell", 120, 700, "T-3"),
		dividend("2026-01-15", "CA-2", "AAA", 25),
		price("2026-01-16", "AAA", 800),
	)
	s, err := Fold(rs, 6)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Positions["AAA"]
	if p.Qty != 180 || p.TotalCost != 99030 {
		t.Fatalf("position %+v", p)
	}
	if s.Cash != -76550 || s.RealizedPnL != 17980 || s.DividendIncome != 4500 {
		t.Fatalf("cash %d realized %d div %d", s.Cash, s.RealizedPnL, s.DividendIncome)
	}
	v := s.Valuations["AAA"]
	if v.Price != 800 || v.PriceSeq != 6 || v.Unrealized != 44970 || s.UnrealizedPnL() != 44970 {
		t.Fatalf("valuation %+v total %d", v, s.UnrealizedPnL())
	}
	if len(s.Refusals) != 0 || len(s.Absorbed) != 0 || len(s.Unevaluable) != 0 {
		t.Fatalf("unexpected records: %+v %+v %+v", s.Refusals, s.Absorbed, s.Unevaluable)
	}
}

func TestFoldHalfEvenBothDirections(t *testing.T) {
	rs := records(
		fill("2026-01-05", "BBB", "buy", 1, 500, "T-1"),
		fill("2026-01-05", "BBB", "buy", 1, 503, "T-2"),  // cost 1003
		fill("2026-01-06", "BBB", "sell", 1, 600, "T-3"), // 501.5 -> 502
		fill("2026-01-05", "CCC", "buy", 1, 500, "T-4"),
		fill("2026-01-05", "CCC", "buy", 1, 501, "T-5"),  // cost 1001
		fill("2026-01-06", "CCC", "sell", 1, 600, "T-6"), // 500.5 -> 500
	)
	s, _ := Fold(rs, 6)
	if s.Positions["BBB"].TotalCost != 501 || s.Positions["CCC"].TotalCost != 501 {
		t.Fatalf("%+v", s.Positions)
	}
	if s.RealizedPnL != 98+100 {
		t.Fatalf("realized %d", s.RealizedPnL)
	}
}

func TestDuplicateAbsorbedCollisionRefused(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 10, 100, "T-1"),
		fill("2026-01-05", "AAA", "buy", 10, 100, "T-1"), // exact duplicate
		fill("2026-01-05", "AAA", "buy", 11, 100, "T-1"), // same key, different payload
	)
	s, _ := Fold(rs, 3)
	if s.Positions["AAA"].Qty != 10 {
		t.Fatalf("qty %d", s.Positions["AAA"].Qty)
	}
	if len(s.Absorbed) != 1 || s.Absorbed[0].Seq != 2 || s.Absorbed[0].EventID != "ev-2" {
		t.Fatalf("absorbed %+v", s.Absorbed)
	}
	if len(s.Refusals) != 1 || s.Refusals[0].Kind != "collision" || s.Refusals[0].Seq != 3 {
		t.Fatalf("refusals %+v", s.Refusals)
	}
	k, _ := FillKey(rs[0].Payload)
	if s.Absorbed[0].Key != k || s.Refusals[0].Key != k {
		t.Fatal("key mismatch")
	}
}

func TestOversellRefusedFailClosed(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 10, 100, "T-1"),
		fill("2026-01-06", "AAA", "sell", 11, 100, "T-2"),
	)
	s, _ := Fold(rs, 2)
	if s.Positions["AAA"].Qty != 10 || len(s.Refusals) != 1 || s.Refusals[0].Kind != "oversell" {
		t.Fatalf("%+v %+v", s.Positions, s.Refusals)
	}
}

func TestSellAllRemovesPosition(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 3, 333, "T-1"),
		fill("2026-01-06", "AAA", "sell", 3, 400, "T-2"),
	)
	s, _ := Fold(rs, 2)
	if _, ok := s.Positions["AAA"]; ok {
		t.Fatal("position should be removed at qty 0")
	}
	if s.RealizedPnL != 3*400-999 {
		t.Fatal(s.RealizedPnL)
	}
}

func TestAmendmentThreeViewpoints(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 100, 1000, "T-1"),
		split("2026-01-10", "CA-1", "AAA", 2), // seq 2
		fill("2026-01-11", "AAA", "sell", 50, 600, "T-2"),
		amendRatio("2026-01-10", "CA-1", 3), // seq 4: split was really 3-for-1
	)
	v1, _ := Fold(rs, 1)
	v2, _ := Fold(rs, 3)
	v3, _ := Fold(rs, 4)
	if v1.Positions["AAA"].Qty != 100 {
		t.Fatal(v1.Positions)
	}
	// V2: 200 shares cost 100000, sell 50 -> relieved 25000, qty 150, cost 75000
	if v2.Positions["AAA"].Qty != 150 || v2.Positions["AAA"].TotalCost != 75000 || v2.RealizedPnL != 30000-25000 {
		t.Fatalf("V2 %+v realized %d", v2.Positions, v2.RealizedPnL)
	}
	// V3: 300 shares cost 100000, sell 50 -> relieved rhe(5000000/300)=16667 (16666.67), qty 250, cost 83333
	if v3.Positions["AAA"].Qty != 250 || v3.Positions["AAA"].TotalCost != 83333 || v3.RealizedPnL != 30000-16667 {
		t.Fatalf("V3 %+v realized %d", v3.Positions, v3.RealizedPnL)
	}
}

func TestPriceLatestByEffectiveThenSeq(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 1, 100, "T-1"),
		price("2026-01-09", "AAA", 900), // later effective, earlier seq
		price("2026-01-08", "AAA", 800), // late-arriving older price must not win
	)
	s, _ := Fold(rs, 3)
	if s.Valuations["AAA"].Price != 900 || s.Valuations["AAA"].PriceSeq != 2 {
		t.Fatalf("%+v", s.Valuations)
	}
}

func TestMissingPriceIsUnevaluable(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 1, 100, "T-1"),
		fill("2026-01-05", "CCC", "buy", 1, 100, "T-2"),
		price("2026-01-06", "AAA", 120),
	)
	s, _ := Fold(rs, 3)
	if len(s.Unevaluable) != 1 || s.Unevaluable[0].Instrument != "CCC" || s.Unevaluable[0].Reason != "no_price_in_prefix" {
		t.Fatalf("%+v", s.Unevaluable)
	}
	if _, ok := s.Valuations["CCC"]; ok {
		t.Fatal("CCC must not be valued")
	}
	if s.UnrealizedPnL() != 20 {
		t.Fatal(s.UnrealizedPnL())
	}
}

func TestMalformedAndUnknownActionAreRefusals(t *testing.T) {
	rs := records(
		ev{"fill", "2026-01-05", map[string]any{"instrument": "AAA", "price": num(100), "qty": num(0), "side": "buy", "trade_id": "T-1", "venue": "X"}},
		ev{"fill", "2026-01-05", map[string]any{"instrument": "AAA", "price": num(100), "qty": num(1), "side": "short", "trade_id": "T-2", "venue": "X"}},
		amendRatio("2026-01-05", "CA-404", 2),
		ev{"bogus", "2026-01-05", map[string]any{}},
	)
	s, _ := Fold(rs, 4)
	kinds := map[string]int{}
	for _, r := range s.Refusals {
		kinds[r.Kind]++
	}
	if kinds["malformed"] != 3 || kinds["unknown_action"] != 1 || len(s.Positions) != 0 {
		t.Fatalf("%+v", s.Refusals)
	}
}

func TestFoldUpToBeyondFeedIsError(t *testing.T) {
	if _, err := Fold(records(price("2026-01-05", "AAA", 1)), 2); err == nil {
		t.Fatal("expected error")
	}
}

// Fix round 1, item 1: a collision must not be silently absorbed just
// because both payloads fail to canonicalize (canon.Marshal refuses
// non-ASCII strings). Before the fix, PayloadHash returned "" on failure,
// so two DIFFERENT payloads that both failed to canonicalize hashed equal
// and the second was recorded as an absorbed duplicate -- a hole in the
// at-most-once (P1) guarantee. The fold cannot tell a duplicate from a
// collision when it cannot compute the hash, so both must be refused as
// malformed rather than either being absorbed or misclassified.
func TestUnhashablePayloadCollisionIsRefusedNotAbsorbed(t *testing.T) {
	rs := []feed.Record{
		{Seq: 1, Type: "fill", ID: "ev-1", Effective: "2026-01-05", Payload: map[string]any{
			"instrument": "AAÉ", "price": num(1000), "qty": num(10), "side": "buy", "trade_id": "T-1", "venue": "X",
		}},
		{Seq: 2, Type: "fill", ID: "ev-2", Effective: "2026-01-05", Payload: map[string]any{
			"instrument": "BBÉ", "price": num(2000), "qty": num(20), "side": "sell", "trade_id": "T-1", "venue": "X",
		}},
	}
	// Sanity: both payloads really do fail to canonicalize (non-ASCII
	// instrument), and both share the same FillKey (same trade_id/venue) --
	// the exact shape that used to collapse into a silent absorb.
	if _, err := canon.Marshal(rs[0].Payload); err == nil {
		t.Fatal("test setup: payload 0 must fail to canonicalize")
	}
	if _, err := canon.Marshal(rs[1].Payload); err == nil {
		t.Fatal("test setup: payload 1 must fail to canonicalize")
	}
	k0, err0 := FillKey(rs[0].Payload)
	k1, err1 := FillKey(rs[1].Payload)
	if err0 != nil || err1 != nil || k0 != k1 {
		t.Fatalf("test setup: both fills must share one FillKey, got %q(%v) %q(%v)", k0, err0, k1, err1)
	}

	s, err := Fold(rs, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Absorbed) != 0 {
		t.Fatalf("an unhashable payload must never be absorbed as a duplicate: %+v", s.Absorbed)
	}
	if len(s.Refusals) != 2 || s.Refusals[0].Kind != "malformed" || s.Refusals[1].Kind != "malformed" {
		t.Fatalf("both unhashable fills must be refused as malformed: %+v", s.Refusals)
	}
	if len(s.Positions) != 0 {
		t.Fatalf("no position should form from unhashable payloads: %+v", s.Positions)
	}
}

// Fix round 1, item 2: Fold's documented contract folds records with
// Seq <= upTo -- a selection by VALUE, not by slice position. This pins
// that selection against a hand-built, non-contiguous slice where index
// and Seq disagree, alongside the ordinary feed-produced (contiguous,
// Seq == index+1) case to confirm the fix leaves that behavior unchanged.
func TestFoldSelectsPrefixBySeqNotIndex(t *testing.T) {
	// Normal, feed-produced-style contiguous slice: selection-by-seq and
	// selection-by-index agree here, so this pins unchanged behavior.
	rs := records(
		fill("2026-01-05", "AAA", "buy", 10, 100, "T-1"),
		fill("2026-01-06", "AAA", "buy", 5, 100, "T-2"),
	)
	s1, err := Fold(rs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Positions["AAA"].Qty != 10 {
		t.Fatalf("contiguous upTo=1 must select only seq 1: %+v", s1.Positions)
	}

	// Hand-built, non-contiguous slice: index 0 carries Seq 10, index 1
	// carries Seq 20. This is the reviewer's exact probe: Fold(nc, 1) asks
	// for the prefix through Seq 1, which contains NEITHER record -- not
	// records[:1] (the seq-10 record at index 0, which the old index-based
	// slicing incorrectly folded).
	nc := []feed.Record{
		{Seq: 10, Type: "fill", ID: "ev-10", Effective: "2026-01-05", Payload: map[string]any{
			"instrument": "AAA", "price": num(100), "qty": num(1), "side": "buy", "trade_id": "T-1", "venue": "X",
		}},
		{Seq: 20, Type: "fill", ID: "ev-20", Effective: "2026-01-06", Payload: map[string]any{
			"instrument": "AAA", "price": num(100), "qty": num(1), "side": "buy", "trade_id": "T-2", "venue": "X",
		}},
	}
	s2, err := Fold(nc, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Positions) != 0 || len(s2.Refusals) != 0 || len(s2.Absorbed) != 0 {
		t.Fatalf("upTo=1 with seqs {10,20} must select nothing: %+v", s2)
	}

	// upTo=2 stays within the len(nc)=2 bound-check but Seq<=2 still
	// selects neither record: selection is by Seq value, not by how many
	// records happen to fit under the len bound. (Old code would have
	// folded records[:2] -- both records -- producing qty 2.)
	s3, err := Fold(nc, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(s3.Positions) != 0 {
		t.Fatalf("upTo=2 with seqs {10,20} must still select nothing: %+v", s3.Positions)
	}
}

// -- Critical: int64 overflow must never panic or report nonsense --
//
// These four tests go through the REAL path -- feed.Append, then a fresh
// feed.Open (which fully re-verifies the chain), then Fold -- rather than a
// hand-built []feed.Record slice, because the reported defect is reachable
// through the actual API, not just by hand-crafted bytes.

func numI64(i int64) json.Number { return json.Number(strconv.FormatInt(i, 10)) }

func fillI64(eff, inst, side string, qty, price int64, tid string) ev {
	return ev{"fill", eff, map[string]any{"instrument": inst, "price": numI64(price), "qty": numI64(qty), "side": side, "trade_id": tid, "venue": "X"}}
}
func priceI64(eff, inst string, px int64) ev {
	return ev{"price", eff, map[string]any{"instrument": inst, "price": numI64(px)}}
}
func splitI64(eff, id, inst string, ratio int64) ev {
	return ev{"action", eff, map[string]any{"action_id": id, "announced": eff, "instrument": inst, "kind": "split", "processed": eff, "ratio": numI64(ratio)}}
}

// openFeedWith appends evs to a fresh on-disk feed, closes it, reopens it
// (forcing a full chain re-verification exactly as a real reader would),
// and returns the verified records -- the same shape the team lead used to
// reproduce each overflow shape.
func openFeedWith(t *testing.T, evs ...ev) []feed.Record {
	t.Helper()
	path := t.TempDir() + "/feed.jsonl"
	fd, err := feed.Open(path)
	if err != nil {
		t.Fatalf("feed.Open: %v", err)
	}
	for i, e := range evs {
		if _, err := fd.Append(e.typ, "ev-"+strconv.Itoa(i+1), e.eff, e.p); err != nil {
			t.Fatalf("feed.Append(%d): %v", i, err)
		}
	}
	if err := fd.Close(); err != nil {
		t.Fatalf("feed.Close: %v", err)
	}
	fd2, err := feed.Open(path)
	if err != nil {
		t.Fatalf("feed.Open (reopen, re-verifies chain): %v", err)
	}
	t.Cleanup(func() { fd2.Close() })
	return fd2.Records()
}

// Shape 1: buy 4e9@4e9 (cost 1.6e19, overflows int64) then sell 1@1. Before
// the fix this panicked inside RelieveCost ("invariant violated") because
// the buy's wrapped-around TotalCost had already gone negative. The buy's
// own product must be refused before it ever touches Position/Cash, so the
// sell that follows sees an (still-empty) position and refuses as an
// ordinary oversell -- no panic anywhere.
func TestFoldOverflowBuyRefusedNotPanicked(t *testing.T) {
	const big = 4000000000 // 4e9; big*big = 1.6e19 > MaxInt64 (~9.22e18)
	rs := openFeedWith(t,
		fillI64("2026-01-05", "AAA", "buy", big, big, "T-1"),
		fillI64("2026-01-06", "AAA", "sell", 1, 1, "T-2"),
	)
	s, err := Fold(rs, int64(len(rs)))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Positions) != 0 {
		t.Fatalf("an overflowing buy must never create a position: %+v", s.Positions)
	}
	if len(s.Refusals) != 2 {
		t.Fatalf("expected 2 refusals (overflow buy + oversell sell), got %+v", s.Refusals)
	}
	if s.Refusals[0].Kind != "malformed" || s.Refusals[0].Seq != rs[0].Seq {
		t.Fatalf("first refusal must be the overflowing buy, kind malformed: %+v", s.Refusals[0])
	}
	if s.Refusals[1].Kind != "oversell" || s.Refusals[1].Seq != rs[1].Seq {
		t.Fatalf("second refusal must be an ordinary oversell (no position exists to sell from): %+v", s.Refusals[1])
	}
}

// Shape 2: two buys, each individually representable (1 * MaxInt64 =
// MaxInt64, no overflow), but the RUNNING TOTAL TotalCost overflows on the
// second buy's accumulation (MaxInt64 + MaxInt64 wraps to -2). The check
// must be on the accumulator, not the per-event product, or this silently
// produces a negative cost basis.
func TestFoldOverflowRunningTotalAccumulationRefused(t *testing.T) {
	rs := openFeedWith(t,
		fillI64("2026-01-05", "AAA", "buy", 1, math.MaxInt64, "T-1"),
		fillI64("2026-01-06", "AAA", "buy", 1, math.MaxInt64, "T-2"),
	)
	s, err := Fold(rs, int64(len(rs)))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := s.Positions["AAA"]
	if !ok || p.Qty != 1 || p.TotalCost != math.MaxInt64 {
		t.Fatalf("only the first buy should have applied: %+v", p)
	}
	if p.TotalCost < 0 {
		t.Fatal("total cost must never go negative")
	}
	if len(s.Refusals) != 1 || s.Refusals[0].Kind != "malformed" || s.Refusals[0].Seq != rs[1].Seq {
		t.Fatalf("second buy must be refused as malformed (running TotalCost overflow): %+v", s.Refusals)
	}
}

// Shape 3: buy 4e9@1 (cost 4e9, fine) then a price of 4e9. qty*Price at
// valuation time overflows, but the fill already applied and cannot be
// retroactively refused -- the position must be marked Unevaluable with a
// distinct reason (valuation_overflow) instead of publishing a wrapped,
// nonsense Unrealized number.
func TestFoldOverflowValuationIsUnevaluableNotWrong(t *testing.T) {
	const big = 4000000000
	rs := openFeedWith(t,
		fillI64("2026-01-05", "AAA", "buy", big, 1, "T-1"),
		priceI64("2026-01-06", "AAA", big),
	)
	s, err := Fold(rs, int64(len(rs)))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Refusals) != 0 {
		t.Fatalf("the fill applied cleanly; only the valuation is unrepresentable: %+v", s.Refusals)
	}
	if _, ok := s.Valuations["AAA"]; ok {
		t.Fatal("an unrepresentable qty*price must never publish a Valuation")
	}
	if len(s.Unevaluable) != 1 || s.Unevaluable[0].Instrument != "AAA" || s.Unevaluable[0].Reason != "valuation_overflow" {
		t.Fatalf("expected exactly one valuation_overflow Unevaluable entry: %+v", s.Unevaluable)
	}
	if p := s.Positions["AAA"]; p.Qty != big || p.TotalCost != big {
		t.Fatalf("the position itself must be unaffected by the valuation-time overflow: %+v", p)
	}
}

// Shape 4: buy 100@10 (position fine) then a split with ratio=MaxInt64.
// 100 * MaxInt64 overflows and, before the fix, wrapped to a NEGATIVE
// quantity -- which the spec forbids outright. The split must be refused
// and the position left exactly as it was.
func TestFoldOverflowSplitRefusedNoNegativeQty(t *testing.T) {
	rs := openFeedWith(t,
		fillI64("2026-01-05", "AAA", "buy", 100, 10, "T-1"),
		splitI64("2026-01-06", "CA-1", "AAA", math.MaxInt64),
	)
	s, err := Fold(rs, int64(len(rs)))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := s.Positions["AAA"]
	if !ok || p.Qty != 100 || p.TotalCost != 1000 {
		t.Fatalf("split must not apply when it would overflow qty; position must be unchanged: %+v", p)
	}
	if p.Qty < 0 {
		t.Fatal("position quantity must never go negative")
	}
	if len(s.Refusals) != 1 || s.Refusals[0].Kind != "malformed" || s.Refusals[0].Seq != rs[1].Seq {
		t.Fatalf("split overflow must be refused as malformed: %+v", s.Refusals)
	}
}

// Fifth overflow shape, caught by review after the first four: the
// AGGREGATE across positions in State.UnrealizedPnL() (the `t +=
// v.Unrealized` accumulation) is not guarded just because each position's
// own Unrealized is individually representable -- two positions each
// carrying Unrealized == MaxInt64-1 sum to a value that overflows int64,
// mirroring the reported repro (`sum of two MaxInt64 unrealized values =
// -2`). AAA and BBB each buy 1@1 (TotalCost 1) then mark to MaxInt64,
// giving Unrealized = 1*MaxInt64 - 1 = MaxInt64-1 for each -- individually
// fine, but AAA+BBB overflows. Fold must publish a Valuation only for the
// survivor(s) whose inclusion keeps the running aggregate representable
// (here, sorted-instrument order keeps AAA and evicts BBB to Unevaluable),
// so UnrealizedPnL() returns the true, un-wrapped value rather than -2 (or
// any other wrapped number), with no signature change.
func TestFoldOverflowAggregateUnrealizedPnLRefused(t *testing.T) {
	rs := openFeedWith(t,
		fillI64("2026-01-05", "AAA", "buy", 1, 1, "T-1"),
		fillI64("2026-01-05", "BBB", "buy", 1, 1, "T-2"),
		priceI64("2026-01-06", "AAA", math.MaxInt64),
		priceI64("2026-01-06", "BBB", math.MaxInt64),
	)
	s, err := Fold(rs, int64(len(rs)))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Refusals) != 0 {
		t.Fatalf("every fill and price is individually valid; only the aggregate is unrepresentable: %+v", s.Refusals)
	}
	want := int64(math.MaxInt64 - 1)
	va, ok := s.Valuations["AAA"]
	if !ok || va.Unrealized != want {
		t.Fatalf("AAA (first in sorted order) must survive with its true value: %+v", va)
	}
	if _, ok := s.Valuations["BBB"]; ok {
		t.Fatal("BBB must be evicted, not published with a wrapped Unrealized value")
	}
	if len(s.Unevaluable) != 1 || s.Unevaluable[0].Instrument != "BBB" || s.Unevaluable[0].Reason != "valuation_overflow" {
		t.Fatalf("expected BBB evicted with reason valuation_overflow: %+v", s.Unevaluable)
	}
	if got := s.UnrealizedPnL(); got != want {
		t.Fatalf("UnrealizedPnL() = %d, want %d (the true survivor sum, never a wrapped negative)", got, want)
	}
}
