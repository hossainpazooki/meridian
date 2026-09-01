package gates

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Counts holds, per named check, Checks (the violation count Emit judges —
// 0 means the check passed) and Evaluated (that check's denominator).
//
// Evaluated MEANS the size of the universe the check actually examined —
// NOT the size of the thing it was looking for. Those two are the same
// number for a check like "duplicate_absorbed" (one planted duplicate, one
// thing examined), which is why the distinction was easy to miss, but they
// diverge for a set-equality check: a check comparing two sets is not
// examining "the size of the published set" (len(want)), it is examining
// every element either side ever mentions (the union — see SetEquality), or
// — when the two sets being compared aren't themselves the full scope of
// the assertion — a caller-declared universe (see SetEqualityOverUniverse).
// Both the len(want) denominator bug and the once-vacuous
// TestEmitRejectsWrongCells cases (gates/verdict_test.go) came from this
// distinction being implicit rather than written down: a reader (or a test
// author copying NewCounts's zero default) could not tell "the denominator
// IS the target size" from "the denominator is unrelated to the target
// size" without already knowing the check's internals.
//
// Emit refuses any row where a check present in Checks has an Evaluated
// entry that is missing or <= 0 (see emitWith) — Evaluated is load-bearing,
// not documentation: a check with no evaluated denominator is refused
// rather than trusted, on both the live and twin cells.
type Counts struct{ Checks, Evaluated map[string]int64 }

func NewCounts(names ...string) Counts {
	c := Counts{Checks: map[string]int64{}, Evaluated: map[string]int64{}}
	for _, n := range names {
		c.Checks[n], c.Evaluated[n] = 0, 0
	}
	return c
}

type Row struct {
	Prop        int
	Cell        string
	Scope       string
	ContentHash string
	Basis       string
	Rows        int64
	Params      map[string]any
	Counts      Counts
	Planted     *Planted
}

// failer lets tests exercise Emit's enforcement without aborting themselves.
type failer interface {
	Helper()
	Fatalf(string, ...any)
}

type fakeT struct {
	*testing.T
	failed  bool
	lastMsg string // the formatted Fatalf message, so a test can assert WHICH rule refused the row, not just that something did
}

func (f *fakeT) Fatalf(format string, a ...any) {
	f.failed = true
	f.lastMsg = fmt.Sprintf(format, a...)
}

func gitStamp() (sha, worktree string, err error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", "", fmt.Errorf("git rev-parse: %w", err)
	}
	sha = strings.TrimSpace(string(out))
	st, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return "", "", fmt.Errorf("git status: %w", err)
	}
	worktree = "clean"
	if strings.TrimSpace(string(st)) != "" {
		worktree = "dirty"
	}
	return sha, worktree, nil
}

// Emit writes the verdict row and enforces the cell rule.
func Emit(t *testing.T, r Row) { t.Helper(); emitWith(t, r) }

func emitWith(t failer, r Row) {
	t.Helper()
	// A row that measured nothing is not evidence of anything: an empty (or
	// nil) Checks map must never be credited GREEN by the zero-value
	// default of "result" below. This applies to both cells — a twin with
	// no checks would eventually fail the "never went RED" test further
	// down, but failing it here, before that, gives an unambiguous reason
	// and also covers the live cell, which has no other guard against it.
	if len(r.Counts.Checks) == 0 {
		t.Fatalf("P%d %s row has no checks: a cell that examines nothing cannot be credited", r.Prop, r.Cell)
		return
	}
	result := "GREEN"
	names := make([]string, 0, len(r.Counts.Checks))
	for k, v := range r.Counts.Checks {
		names = append(names, k)
		if v != 0 {
			result = "RED"
		}
		// A check's Evaluated entry is the denominator a reader uses to
		// judge its Checks value — in particular to tell a genuine zero
		// (many rows examined, none violated) from a vacuous one (nothing
		// examined, so of course nothing violated). A missing or <=0
		// Evaluated count for a check that ran is refused rather than
		// silently trusted, for both cells: fail-closed on an unevaluated
		// denominator, the same discipline applied everywhere else in this
		// repo to "unevaluable" rather than fabricating a pass.
		if r.Counts.Evaluated[k] <= 0 {
			t.Fatalf("P%d %s check %q has no evaluated denominator (evaluated=%d)", r.Prop, r.Cell, k, r.Counts.Evaluated[k])
			return
		}
	}
	sort.Strings(names)
	switch r.Cell {
	case "live":
		if result != "GREEN" {
			t.Fatalf("P%d live cell is RED: %v", r.Prop, r.Counts.Checks)
			return
		}
	case "twin":
		if r.Planted == nil {
			t.Fatalf("P%d twin row needs Planted", r.Prop)
			return
		}
		if result != "RED" {
			t.Fatalf("P%d twin never went RED: %v", r.Prop, r.Counts.Checks)
			return
		}
		// Compared over the union of Checks' and ExpectedViolations' keys,
		// matching claimability.py's plain dict equality exactly: a
		// planted expectation whose check was never computed must fail
		// regardless of its value, not just when that value is non-zero.
		// Map indexing with the two-value form is required here — a bare
		// r.Counts.Checks[k] silently returns 0 for an absent key, which
		// is exactly the hole that let an unwired "expected 0" pass.
		for k, want := range r.Planted.ExpectedViolations {
			got, present := r.Counts.Checks[k]
			if !present {
				t.Fatalf("P%d twin check %q has a planted expectation (%d) but was never computed", r.Prop, k, want)
				return
			}
			if got != want {
				t.Fatalf("P%d twin check %q caught %d, planted %d", r.Prop, k, got, want)
				return
			}
		}
		for k := range r.Counts.Checks {
			if _, ok := r.Planted.ExpectedViolations[k]; !ok {
				t.Fatalf("P%d twin check %q has no planted expectation", r.Prop, k)
				return
			}
		}
	default:
		t.Fatalf("cell must be live|twin")
		return
	}
	sha, wt, err := gitStamp()
	if err != nil {
		t.Fatalf("verdict cannot be stamped: %v", err)
		return
	}
	now := time.Now().UTC()
	runner := os.Getenv("MERIDIAN_RUNNER")
	if runner == "" {
		runner = "local"
	}
	row := map[string]any{
		"kind": "GATE_VERDICT", "surface": fmt.Sprintf("meridian-lane1-p%d", r.Prop), "lane": 1, "cell": r.Cell,
		"result": result, "checks": r.Counts.Checks, "evaluated": r.Counts.Evaluated, "rows": r.Rows, "scope": r.Scope,
		"params": r.Params, "parallax_sha": sha, "parallax_worktree": wt, "content_hash": r.ContentHash,
		"content_hash_basis": r.Basis, "ran_at": now.Format("2006-01-02T15:04:05.000000Z"), "runner": runner,
	}
	if r.Cell == "twin" {
		row["planted"] = map[string]any{"mutation": r.Planted.Mutation, "mutated_rows": r.Planted.MutatedRows, "expected_violations": r.Planted.ExpectedViolations}
	}
	dir := os.Getenv("MERIDIAN_VERDICT_DIR")
	if dir == "" {
		dir = "out"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("%v", err)
		return
	}
	// Params is caller-supplied (map[string]any); an unmarshalable value
	// (e.g. a float, which canon.Marshal would refuse but json.MarshalIndent
	// silently emits or a genuinely unsupported type errors on) must not
	// yield a truncated/empty row indistinguishable from a missing file.
	// Discarding this error was exactly that: b == nil on failure, then
	// os.WriteFile happily writes a one-byte "\n" verdict file and a test
	// exercising a bad Params value would still pass.
	b, err := json.MarshalIndent(row, "", "  ")
	if err != nil {
		t.Fatalf("P%d %s row does not marshal: %v", r.Prop, r.Cell, err)
		return
	}
	name := fmt.Sprintf("meridian-lane1-p%d-%s-%s.json", r.Prop, r.Cell, now.Format("20060102T150405.000000Z"))
	if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
}
