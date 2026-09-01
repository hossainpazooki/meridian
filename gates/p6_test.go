package gates

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/snapshot"
)

// p6Check evaluates portfolio math (average-cost basis, realized/unrealized
// P&L) against the hand-computed golden. golden_match walks every leaf of
// golden and requires byte-identical leaves in doc; the golden.json fixture
// is the one artifact in the tree whose authority comes from hand
// arithmetic rather than the Python naive fold, so this is the gate's only
// cross-implementation-independent check.
func p6Check(doc, golden snapshot.Doc) (Counts, []snapshot.Mismatch) {
	ms := snapshot.Diff(golden, doc)
	c := NewCounts("golden_match")
	c.Evaluated["golden_match"] = int64(snapshot.Leaves(golden))
	c.Checks["golden_match"] = int64(len(ms))
	return c, ms
}

func TestP6PortfolioMath(t *testing.T) {
	m := LoadManifest(t)
	golden := LoadDoc(t, filepath.Join(FixturesDir, "p6", "golden.json"))

	// P6 has its own feed and its own end_seq (14) — key-set equality here
	// must compare against golden.json's own position/unevaluable sets, not
	// the manifest's positions_at/unevaluable_at, which are scoped to
	// fixtures/base (a different feed entirely) and would silently compare
	// against the wrong viewpoint.
	positionsWant := PositionKeys(golden)
	// golden.json now carries an explicit "unevaluable": [] — a hand-verified
	// fact (every instrument that ever trades in fixtures/p6/feed.jsonl also
	// gets a price event by end_seq, so nothing is unevaluable), not silence.
	// That closes the case where a doc entirely omits a real unevaluable
	// entry: want is a stated [] rather than an absent key defaulting to [].
	// It is still a fact about THIS golden and THESE planted mutations
	// (fill_qty_plus_one, price_plus_one), not a structural guarantee: a
	// future P6 twin whose mutation could plausibly push an instrument into
	// unevaluable status would need to account for this leaf explicitly —
	// both in the mutation's effect on unevWant/got here and in the
	// manifest's golden_match expected_violations, since the new leaf adds
	// one to Leaves(golden) (16 -> 17) and would carry its own mismatch if
	// such a mutation ever caused doc and golden to disagree on it.
	unevWant := UnevaluableInstruments(golden)

	viewpoint := m.Int("p6", "end_seq")
	basisParams := map[string]any{"viewpoint": viewpoint, "golden": "fixtures/p6/golden.json (hand-computed)"}

	live := ReadFixture(t, "p6/feed.jsonl", -1)
	c, _ := p6Check(live.Doc, golden)
	SetEquality(c, "positions_match_golden", PositionKeys(live.Doc), positionsWant)
	SetEqualityOverUniverse(c, "unevaluable_match_golden", UnevaluableInstruments(live.Doc), unevWant, positionsWant)
	Emit(t, Row{Prop: 6, Cell: "live", Scope: "fixtures/p6 hand-scripted portfolio vs hand-computed golden, to the minor unit", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: basisParams, Counts: c})

	// twin_fill: one fill quantity mutated.
	fill := ReadFixture(t, "p6/twin-fill/feed.jsonl", -1)
	cf, _ := p6Check(fill.Doc, golden)
	SetEquality(cf, "positions_match_golden", PositionKeys(fill.Doc), positionsWant)
	SetEqualityOverUniverse(cf, "unevaluable_match_golden", UnevaluableInstruments(fill.Doc), unevWant, positionsWant)
	Emit(t, Row{Prop: 6, Cell: "twin", Scope: "fixtures/p6 with one fill quantity mutated", ContentHash: fill.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(fill.State.Positions)),
		Params: map[string]any{"mutated_seq": m.Int("p6", "twin_fill", "seq"), "viewpoint": viewpoint},
		Counts: cf, Planted: ptr(m.Planted("p6", "twin_fill"))})

	// twin_price: one price perturbed by one minor unit. Exactly the
	// dependent unrealized fields may move; everything else must be
	// byte-stable — a perturbation that moved an unrelated field would be a
	// real defect in the fold, not a false alarm in the gate.
	price := ReadFixture(t, "p6/twin-price/feed.jsonl", -1)
	cp, ms := p6Check(price.Doc, golden)
	for _, x := range ms {
		switch x.Path {
		case "positions.AAA.valuation.price", "positions.AAA.valuation.unrealized", "unrealized_pnl":
		default:
			t.Fatalf("price perturbation moved an unrelated field: %+v", x)
		}
	}
	SetEquality(cp, "positions_match_golden", PositionKeys(price.Doc), positionsWant)
	SetEqualityOverUniverse(cp, "unevaluable_match_golden", UnevaluableInstruments(price.Doc), unevWant, positionsWant)
	Emit(t, Row{Prop: 6, Cell: "twin", Scope: "fixtures/p6 with one price perturbed by one minor unit", ContentHash: price.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(price.State.Positions)),
		Params: map[string]any{"mutated_seq": m.Int("p6", "twin_price", "seq"), "viewpoint": viewpoint},
		Counts: cp, Planted: ptr(m.Planted("p6", "twin_price"))})

	// twin_phantom: an invented, never-traded instrument (ZZZ). snapshot.Diff
	// is one-sided — it only walks golden's keys, so it is structurally
	// blind to a doc that invents a position golden never had. golden_match
	// therefore scores 0 mismatches against this twin and must NOT be
	// emitted as a check here (the manifest plants no expectation for it,
	// and Emit refuses a twin whose Checks and planted expectations differ
	// over the union of their keys). positions_match_golden is the only
	// check in the whole build that catches this defect.
	phantom := LoadDoc(t, filepath.Join(FixturesDir, "p6", "twin-phantom", "snapshot.json"))
	cph := NewCounts("positions_match_golden", "unevaluable_match_golden")
	SetEquality(cph, "positions_match_golden", PositionKeys(phantom), positionsWant)
	SetEqualityOverUniverse(cph, "unevaluable_match_golden", UnevaluableInstruments(phantom), unevWant, positionsWant)
	raw, err := json.Marshal(phantom)
	if err != nil {
		t.Fatal(err)
	}
	positions, _ := phantom["positions"].(map[string]any)
	Emit(t, Row{Prop: 6, Cell: "twin", Scope: "fixtures/p6 with one invented untraded position (ZZZ)", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the twin document as decoded and re-marshaled", Rows: int64(len(positions)),
		Params: map[string]any{"instrument": m.Str("p6", "twin_phantom", "instrument"), "viewpoint": viewpoint},
		Counts: cph, Planted: ptr(m.Planted("p6", "twin_phantom"))})
}
