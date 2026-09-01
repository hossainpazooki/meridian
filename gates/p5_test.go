package gates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/reconcile"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

// p5Check reconciles doc against the custodian statement at statementPath,
// field by field, zero tolerance. Reconcile fails closed on a doc it cannot
// read (a missing/non-integer cash, a missing/non-object positions map, or a
// malformed position) — that is not a reconciliation this gate can credit,
// so it is a fatal test error, not a silently-zeroed comparison.
func p5Check(t *testing.T, doc snapshot.Doc, statementPath string) (Counts, []reconcile.Mismatch) {
	t.Helper()
	st, err := reconcile.LoadStatement(statementPath)
	if err != nil {
		t.Fatal(err)
	}
	ms, compared, err := reconcile.Reconcile(doc, st)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	c := NewCounts("field_mismatch")
	c.Evaluated["field_mismatch"] = int64(compared)
	c.Checks["field_mismatch"] = int64(len(ms))
	return c, ms
}

// TestP5ReconciliationCanFail proves reconciliation is not vacuous: the live
// snapshot must reconcile exactly against the honest custodian statement
// (fixtures/base/statement.json), and a twin statement carrying one planted
// cost_basis drift must be caught AND NAMED -- the exact instrument, field,
// and signed delta, not merely a nonzero mismatch count.
//
// The custodian statement is a same-contract reimplementation (a separately
// written Python fold), not an independent oracle: both folds were written
// from the same written contract, so a shared misreading of that contract
// would reproduce identically on both sides and this gate would stay green.
// This demonstrates cross-implementation agreement on the contract as
// written, not independent verification of the contract's correctness.
func TestP5ReconciliationCanFail(t *testing.T) {
	m := LoadManifest(t)
	live := ReadFixture(t, "base/feed.jsonl", -1)
	params := map[string]any{"viewpoint": m.Int("end_seq")}

	c, ms := p5Check(t, live.Doc, filepath.Join(FixturesDir, "base", "statement.json"))
	if len(ms) != 0 {
		t.Fatalf("live snapshot does not reconcile against the honest custodian statement: %+v", ms)
	}
	Emit(t, Row{Prop: 5, Cell: "live", Scope: "fixtures/base snapshot vs naive-fold custodian statement, field by field", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: params, Counts: c})

	twinPath := filepath.Join(FixturesDir, "p5", "twin", "statement.json")
	ct, mst := p5Check(t, live.Doc, twinPath)
	// The gate must NAME the drift, not just count it: the right instrument,
	// the right field, and the right signed delta. Delta = Ledger -
	// Custodian, and the drift is planted by ADDING to the custodian's
	// cost_basis, so the ledger-minus-custodian delta the reconciler must
	// report is the NEGATIVE of the manifest's planted (positive) amount.
	if len(mst) != 1 {
		t.Fatalf("expected exactly one named mismatch, got %d: %+v", len(mst), mst)
	}
	wantInst, wantField, wantDelta := m.Str("p5", "drift", "instrument"), m.Str("p5", "drift", "field"), m.Int("p5", "drift", "delta")
	if mst[0].Instrument != wantInst || mst[0].Field != wantField || mst[0].Delta != -wantDelta {
		t.Fatalf("drift not named correctly: got %+v, want instrument=%s field=%s delta=%d", mst[0], wantInst, wantField, -wantDelta)
	}

	raw, err := os.ReadFile(twinPath)
	if err != nil {
		t.Fatal(err)
	}
	tp := map[string]any{"viewpoint": m.Int("end_seq"), "drift_instrument": wantInst, "drift_field": wantField, "drift_delta": wantDelta}
	Emit(t, Row{Prop: 5, Cell: "twin", Scope: "same snapshot vs a statement with one planted cost_basis drift", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the twin statement file bytes", Rows: int64(len(live.State.Positions)), Params: tp, Counts: ct, Planted: ptr(m.Planted("p5"))})
}
