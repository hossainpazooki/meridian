package gates

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestEmitWritesBaselineSchemaAndEnforcesCells(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MERIDIAN_VERDICT_DIR", dir)
	c := NewCounts("a", "b")
	c.Evaluated["a"], c.Evaluated["b"] = 10, 5
	Emit(t, Row{Prop: 9, Cell: "live", Scope: "unit", ContentHash: "sha256:x", Basis: "test", Rows: 10, Params: map[string]any{"k": "v"}, Counts: c})
	c2 := NewCounts("a", "b")
	c2.Checks["a"], c2.Evaluated["a"], c2.Evaluated["b"] = 1, 10, 5
	Emit(t, Row{Prop: 9, Cell: "twin", Scope: "unit twin", ContentHash: "sha256:y", Basis: "test", Rows: 10, Params: map[string]any{},
		Counts: c2, Planted: &Planted{Mutation: "m", MutatedRows: 1, ExpectedViolations: map[string]int64{"a": 1, "b": 0}}})
	files, _ := filepath.Glob(filepath.Join(dir, "meridian-lane1-p9-*.json"))
	if len(files) != 2 {
		t.Fatal(files)
	}
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		var m map[string]any
		json.Unmarshal(raw, &m)
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		want := "cell checks content_hash content_hash_basis evaluated kind lane parallax_sha parallax_worktree params ran_at result rows runner scope surface"
		if strings.Contains(f, "-twin-") {
			want = "cell checks content_hash content_hash_basis evaluated kind lane parallax_sha parallax_worktree params planted ran_at result rows runner scope surface"
		}
		if strings.Join(keys, " ") != want {
			t.Fatalf("%s keys: %s", f, strings.Join(keys, " "))
		}
		if m["kind"] != "GATE_VERDICT" || m["surface"] != "meridian-lane1-p9" || m["lane"].(float64) != 1 {
			t.Fatal(m)
		}
		if strings.Contains(f, "-live-") && m["result"] != "GREEN" || strings.Contains(f, "-twin-") && m["result"] != "RED" {
			t.Fatal(m["result"])
		}
		if len(m["parallax_sha"].(string)) != 40 || (m["parallax_worktree"] != "clean" && m["parallax_worktree"] != "dirty") {
			t.Fatal(m)
		}
	}
}

func TestEmitRejectsWrongCells(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MERIDIAN_VERDICT_DIR", dir)
	// A twin whose violations do not equal the planted counts must fail the
	// test that emits it — and must fail specifically on that COUNT
	// mismatch, not incidentally on some other guard. Evaluated["a"] is set
	// to a real positive value (5) in every case below so the
	// evaluated-denominator rule cannot fire first and mask which rule this
	// test is actually meant to exercise; fakeT.lastMsg is asserted against
	// a substring unique to the intended rule, not just ft.failed, because
	// "it failed" alone cannot tell you it failed for the right reason —
	// the same defect class this project keeps finding in its own guards.
	ft := &fakeT{T: t}
	c := NewCounts("a")
	c.Checks["a"], c.Evaluated["a"] = 2, 5
	emitWith(ft, Row{Prop: 9, Cell: "twin", Counts: c, Params: map[string]any{}, Planted: &Planted{ExpectedViolations: map[string]int64{"a": 1}}})
	if !ft.failed || !strings.Contains(ft.lastMsg, "caught 2, planted 1") {
		t.Fatalf("twin with wrong counts must fail on the COUNT mismatch: failed=%v msg=%q", ft.failed, ft.lastMsg)
	}
	ft = &fakeT{T: t}
	c = NewCounts("a")
	c.Checks["a"], c.Evaluated["a"] = 1, 5
	emitWith(ft, Row{Prop: 9, Cell: "live", Counts: c, Params: map[string]any{}})
	if !ft.failed || !strings.Contains(ft.lastMsg, "live cell is RED") {
		t.Fatalf("live with violations must fail as LIVE-RED: failed=%v msg=%q", ft.failed, ft.lastMsg)
	}
	ft = &fakeT{T: t}
	c = NewCounts("a")
	c.Evaluated["a"] = 5
	emitWith(ft, Row{Prop: 9, Cell: "twin", Counts: c, Params: map[string]any{}, Planted: &Planted{ExpectedViolations: map[string]int64{"a": 0}}})
	if !ft.failed || !strings.Contains(ft.lastMsg, "never went RED") {
		t.Fatalf("twin that never goes red must fail as TWIN-NEVER-RED: failed=%v msg=%q", ft.failed, ft.lastMsg)
	}
}

func TestManifestLoads(t *testing.T) {
	m := LoadManifest(t)
	if m.Int("end_seq") < 10 || m.Int("viewpoints", "V1") >= m.Int("viewpoints", "V2") || m.Str("action", "instrument") != "AAA" {
		t.Fatalf("%v", m)
	}
	p := m.Planted("p1")
	if p.Mutation != "naive_fold_no_dedupe" || p.ExpectedViolations["duplicate_absorbed"] != 1 {
		t.Fatalf("%+v", p)
	}
}

// --- Regression tests for the two crediting holes a reviewer found in the
// original emitWith, plus the evaluated-denominator rule added alongside
// the fix. Each of these must FAIL (ft.failed == true); none of them are
// happy-path checks.

// Hole 1: a live row that examined nothing (empty/nil Checks map) must not
// be credited GREEN by "result" defaulting to GREEN and never being
// flipped.
func TestEmitRejectsEmptyChecksLive(t *testing.T) {
	ft := &fakeT{T: t}
	emitWith(ft, Row{Prop: 9, Cell: "live", Counts: NewCounts(), Params: map[string]any{}})
	if !ft.failed {
		t.Fatal("live row with zero checks must fail, not be credited GREEN")
	}

	ft = &fakeT{T: t}
	emitWith(ft, Row{Prop: 9, Cell: "live", Counts: Counts{}, Params: map[string]any{}})
	if !ft.failed {
		t.Fatal("live row with nil Checks/Evaluated maps must fail, not be credited GREEN")
	}
}

// Same hole, twin side: a twin that examined nothing must fail too (belt
// and braces — it would also fail the "never went RED" check below the
// empty-checks guard, but this locks in the earlier, unambiguous refusal).
func TestEmitRejectsEmptyChecksTwin(t *testing.T) {
	ft := &fakeT{T: t}
	emitWith(ft, Row{Prop: 9, Cell: "twin", Counts: NewCounts(), Params: map[string]any{}, Planted: &Planted{ExpectedViolations: map[string]int64{}}})
	if !ft.failed {
		t.Fatal("twin row with zero checks must fail, not be credited")
	}
}

// Hole 2: a planted expectation of value 0 whose check was never computed
// must fail. Before the fix, r.Counts.Checks[k] returned Go's zero value
// for the absent key "b", which matched a want of 0 by accident — exactly
// the shape the generator now publishes eight times over (p1, p3, p6
// twin_fill, p6 twin_price each carry two zero-valued
// positions_match_manifest/unevaluable_match_manifest expectations).
func TestEmitRejectsUnwiredZeroExpectation(t *testing.T) {
	ft := &fakeT{T: t}
	c := NewCounts("a")
	c.Checks["a"], c.Evaluated["a"] = 1, 1 // "a" is wired and matches its own (non-zero) expectation
	// "b" is never added to c.Checks or c.Evaluated at all — it was never computed.
	emitWith(ft, Row{Prop: 9, Cell: "twin", Counts: c, Params: map[string]any{},
		Planted: &Planted{ExpectedViolations: map[string]int64{"a": 1, "b": 0}}})
	if !ft.failed {
		t.Fatal("twin with an unwired check expected to be 0 must fail, not be silently accepted")
	}
}

// Evaluated-denominator rule: a check present in Checks (even with a
// non-zero violation count) but absent from — or zero in — Evaluated must
// fail on both cells. Evaluated is the denominator a reader uses to judge
// the Checks value; a check that examined zero rows cannot certify
// anything, zero violations included. This is deliberately stricter than
// only refusing an empty *map*: it catches one check silently left
// unwired inside an otherwise-real Counts, which TestEmitRejectsEmptyChecksLive
// does not.
func TestEmitRejectsZeroEvaluatedDenominator(t *testing.T) {
	ft := &fakeT{T: t}
	c := NewCounts("a") // NewCounts zeroes Evaluated["a"]; nothing overwrites it.
	c.Checks["a"] = 0
	emitWith(ft, Row{Prop: 9, Cell: "live", Counts: c, Params: map[string]any{}})
	if !ft.failed {
		t.Fatal("live check with evaluated=0 must fail, not be credited as a real zero")
	}

	ft = &fakeT{T: t}
	c2 := Counts{Checks: map[string]int64{"a": 1}, Evaluated: map[string]int64{}} // "a" has no Evaluated entry at all
	emitWith(ft, Row{Prop: 9, Cell: "twin", Counts: c2, Params: map[string]any{}, Planted: &Planted{ExpectedViolations: map[string]int64{"a": 1}}})
	if !ft.failed {
		t.Fatal("twin check missing its evaluated entry must fail even though checks/planted otherwise agree")
	}
}
