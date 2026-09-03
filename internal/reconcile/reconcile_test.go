package reconcile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hossainpazooki/meridian/internal/snapshot"
)

const doc = `{"absorbed":[],"cash":-403,"dividend_income":0,"feed_prefix_hash":"sha256:x","feed_seq":3,"positions":{"AAA":{"qty":10,"total_cost":1000,"valuation":null},"BBB":{"qty":1,"total_cost":501,"valuation":null}},"realized_pnl":0,"refusals":[],"unevaluable":[],"unrealized_pnl":0}`

func TestReconcileExactAndDriftNamed(t *testing.T) {
	d, _ := snapshot.Decode([]byte(doc))
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	os.WriteFile(good, []byte(`{"as_of_seq":3,"cash":-403,"holdings":[{"cost_basis":1000,"instrument":"AAA","quantity":10},{"cost_basis":501,"instrument":"BBB","quantity":1}]}`), 0o644)
	st, err := LoadStatement(good)
	if err != nil {
		t.Fatal(err)
	}
	ms, compared, err := Reconcile(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 0 || compared != 5 {
		t.Fatalf("%+v %d", ms, compared)
	}

	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"as_of_seq":3,"cash":-403,"holdings":[{"cost_basis":1001,"instrument":"AAA","quantity":10},{"cost_basis":501,"instrument":"CCC","quantity":1}]}`), 0o644)
	st2, _ := LoadStatement(bad)
	ms, _, err = Reconcile(d, st2)
	if err != nil {
		t.Fatal(err)
	}
	// AAA cost_basis drift 1; BBB missing from statement (2 fields); CCC missing from ledger (2 fields)
	if len(ms) != 5 {
		t.Fatalf("%+v", ms)
	}
	if ms[0].Instrument != "AAA" || ms[0].Field != "cost_basis" || ms[0].Ledger != 1000 || ms[0].Custodian != 1001 || ms[0].Delta != -1 {
		t.Fatalf("%+v", ms[0])
	}
}

// TestReconcileFailsClosedOnMalformedDoc reproduces the exact fabrication
// found in fix round 1: a doc missing "cash" and missing "total_cost" on a
// position must never surface as confident-looking custodian mismatches.
// Reconcile must refuse to judge it instead.
func TestReconcileFailsClosedOnMalformedDoc(t *testing.T) {
	badDoc := `{"positions":{"AAA":{"qty":10}},"refusals":[]}`
	d, err := snapshot.Decode([]byte(badDoc))
	if err != nil {
		t.Fatal(err)
	}
	st := Statement{Cash: -403, Holdings: []Holding{{Instrument: "AAA", Quantity: 10, CostBasis: 1000}}}
	ms, compared, err := Reconcile(d, st)
	if err == nil {
		t.Fatalf("expected error, got ms=%+v compared=%d", ms, compared)
	}
	if len(ms) != 0 {
		t.Fatalf("expected zero mismatches on error, got %+v", ms)
	}
}

// TestReconcileFailsClosedOnNonObjectPosition covers a positions map whose
// value is not itself an object.
func TestReconcileFailsClosedOnNonObjectPosition(t *testing.T) {
	badDoc := `{"cash":-403,"positions":{"AAA":10}}`
	d, err := snapshot.Decode([]byte(badDoc))
	if err != nil {
		t.Fatal(err)
	}
	st := Statement{Cash: -403}
	if ms, compared, err := Reconcile(d, st); err == nil {
		t.Fatalf("expected error, got ms=%+v compared=%d", ms, compared)
	}
}

// TestReconcileFailsClosedOnNonIntegerQty covers a qty that decodes as a
// json.Number but is not an integer (Int64() fails on a fractional value).
func TestReconcileFailsClosedOnNonIntegerQty(t *testing.T) {
	badDoc := `{"cash":-403,"positions":{"AAA":{"qty":10.5,"total_cost":1000}}}`
	d, err := snapshot.Decode([]byte(badDoc))
	if err != nil {
		t.Fatal(err)
	}
	st := Statement{Cash: -403}
	if ms, compared, err := Reconcile(d, st); err == nil {
		t.Fatalf("expected error, got ms=%+v compared=%d", ms, compared)
	}
}

// TestReconcileFailsClosedOnMissingPositionsKey covers the "positions" key
// being entirely absent from the doc, as distinct from present-but-wrong-type
// (TestReconcileFailsClosedOnNonObjectPosition): doc["positions"] on a nil
// key is a nil interface, which must fail the same type assertion.
func TestReconcileFailsClosedOnMissingPositionsKey(t *testing.T) {
	badDoc := `{"cash":-403}`
	d, err := snapshot.Decode([]byte(badDoc))
	if err != nil {
		t.Fatal(err)
	}
	st := Statement{Cash: -403}
	ms, compared, err := Reconcile(d, st)
	if err == nil {
		t.Fatalf("expected error, got ms=%+v compared=%d", ms, compared)
	}
	if len(ms) != 0 {
		t.Fatalf("expected zero mismatches on error, got %+v", ms)
	}
}

func TestLoadStatementBytesMatchesLoadStatement(t *testing.T) {
	path := filepath.Join("..", "..", "fixtures", "base", "statement.json")
	fromPath, err := LoadStatement(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fromBytes, err := LoadStatementBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromPath, fromBytes) {
		t.Fatalf("path %+v != bytes %+v", fromPath, fromBytes)
	}
}

func TestLoadStatementBytesRefusesNonObjectAndBadJSON(t *testing.T) {
	for _, raw := range []string{"[]", "{", "", `{"as_of_seq":1,"cash":0}`} {
		if _, err := LoadStatementBytes([]byte(raw)); err == nil {
			t.Fatalf("%q: expected error", raw)
		}
	}
}
