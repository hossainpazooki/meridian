package gates

import (
	"testing"

	"github.com/hossainpazooki/meridian/internal/snapshot"
)

// SetEquality, PositionKeys and UnevaluableInstruments have no gate in this
// build where they currently fire non-zero (P1's live and twin are both 0/0
// on both checks), so nothing in the suite proves the comparator itself
// works. This table test drives SetEquality directly over invented, dropped,
// swapped, and unsorted inputs so the comparator is proven independently of
// any gate wiring it. It does not substitute for a red cell in a real gate —
// only a planted twin that actually invents or drops a position does that —
// but it does prove the arithmetic.
//
// wantEvaluated is len(set(got) ∪ set(want)), NOT len(want) — an earlier
// version of both SetEquality and this table used len(want), which is wrong
// whenever the correct published want is legitimately empty. That shape is
// real: see "empty want, invented got" below, which reproduces P6's phantom
// twin (positions_at/unevaluable_at have no entry at P6's own viewpoint, so
// want=[] is the honest published set, and a twin that invents an untraded
// position must still be credited for catching it, not refused as
// unevaluated).
func TestSetEqualityTable(t *testing.T) {
	cases := []struct {
		name           string
		got, want      []string
		wantMismatches int64
		wantEvaluated  int64
	}{
		{
			name: "exact match, both already sorted",
			got:  []string{"AAA", "BBB", "CCC"}, want: []string{"AAA", "BBB", "CCC"},
			wantMismatches: 0, wantEvaluated: 3,
		},
		{
			name: "exact match, both unsorted — order must not matter",
			got:  []string{"CCC", "AAA", "BBB"}, want: []string{"BBB", "CCC", "AAA"},
			wantMismatches: 0, wantEvaluated: 3,
		},
		{
			name: "invented: got has one extra instrument want never published",
			got:  []string{"AAA", "BBB", "CCC", "ZZZ"}, want: []string{"AAA", "BBB", "CCC"},
			wantMismatches: 1, wantEvaluated: 4, // union includes the invented ZZZ
		},
		{
			name: "dropped: got is missing one instrument want published",
			got:  []string{"AAA", "BBB"}, want: []string{"AAA", "BBB", "CCC"},
			wantMismatches: 1, wantEvaluated: 3, // union == want here, since got ⊆ want
		},
		{
			name: "swapped: one dropped and a different one invented in its place",
			got:  []string{"AAA", "BBB", "ZZZ"}, want: []string{"AAA", "BBB", "CCC"},
			wantMismatches: 2, wantEvaluated: 4, // union: AAA,BBB,CCC,ZZZ
		},
		{
			name: "both empty",
			got:  nil, want: nil,
			wantMismatches: 0, wantEvaluated: 0, // genuinely nothing to compare — correctly vacuous
		},
		{
			name: "got empty, want non-empty — total drop",
			got:  nil, want: []string{"AAA", "BBB"},
			wantMismatches: 2, wantEvaluated: 2,
		},
		{
			name: "empty want, invented got — the P6 phantom-twin shape",
			got:  []string{"ZZZ"}, want: nil,
			wantMismatches: 1, wantEvaluated: 1, // union={ZZZ}: must be credited, NOT refused as evaluated=0
		},
		{
			name: "duplicates within got collapse under set semantics",
			got:  []string{"AAA", "AAA", "BBB"}, want: []string{"AAA", "BBB"},
			wantMismatches: 0, wantEvaluated: 2,
		},
		{
			name: "duplicates within want also collapse — evaluated counts the SET, not the list",
			got:  []string{"AAA", "BBB"}, want: []string{"AAA", "AAA", "BBB", "BBB"},
			wantMismatches: 0, wantEvaluated: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCounts("x")
			SetEquality(c, "x", tc.got, tc.want)
			if c.Checks["x"] != tc.wantMismatches {
				t.Fatalf("mismatches = %d, want %d (got=%v want=%v)", c.Checks["x"], tc.wantMismatches, tc.got, tc.want)
			}
			if c.Evaluated["x"] != tc.wantEvaluated {
				t.Fatalf("evaluated = %d, want %d", c.Evaluated["x"], tc.wantEvaluated)
			}
		})
	}
}

// SetEqualityOverUniverse must produce the SAME mismatch count as
// SetEquality (the got-vs-want arithmetic is unchanged), but a denominator
// driven by the caller-supplied universe instead of len(got ∪ want). This
// is the harness-level generalization of what P6's local unevaluableCheck
// wrapper did — the P6 numbers are reproduced exactly below (universe of 3
// positions, both got/want empty) to confirm this function is a drop-in
// replacement.
func TestSetEqualityOverUniverseTable(t *testing.T) {
	cases := []struct {
		name                string
		got, want, universe []string
		wantMismatches      int64
		wantEvaluated       int64
	}{
		{
			name: "P6 phantom-twin unevaluable_match_manifest shape: both empty, but a real 3-position universe was checked",
			got:  nil, want: nil, universe: []string{"AAA", "BBB", "CCC"},
			wantMismatches: 0, wantEvaluated: 3, // NOT 0 — this is the case Emit's evaluated>0 rule would otherwise wrongly refuse
		},
		{
			name: "mismatch arithmetic is untouched by the universe substitution",
			got:  []string{"AAA", "ZZZ"}, want: []string{"AAA"}, universe: []string{"AAA", "BBB", "CCC"},
			wantMismatches: 1, wantEvaluated: 3,
		},
		{
			name: "universe smaller than got/want union is still just the universe (denominator is a caller assertion, not derived)",
			got:  []string{"AAA", "BBB"}, want: []string{"AAA", "BBB"}, universe: []string{"AAA"},
			wantMismatches: 0, wantEvaluated: 1,
		},
		{
			name: "universe with duplicates collapses under set semantics like got/want do",
			got:  nil, want: nil, universe: []string{"AAA", "AAA", "BBB"},
			wantMismatches: 0, wantEvaluated: 2,
		},
		{
			name: "everything empty, including the universe — genuinely vacuous, correctly 0",
			got:  nil, want: nil, universe: nil,
			wantMismatches: 0, wantEvaluated: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCounts("x")
			SetEqualityOverUniverse(c, "x", tc.got, tc.want, tc.universe)
			if c.Checks["x"] != tc.wantMismatches {
				t.Fatalf("mismatches = %d, want %d (got=%v want=%v universe=%v)", c.Checks["x"], tc.wantMismatches, tc.got, tc.want, tc.universe)
			}
			if c.Evaluated["x"] != tc.wantEvaluated {
				t.Fatalf("evaluated = %d, want %d", c.Evaluated["x"], tc.wantEvaluated)
			}
		})
	}
}

// End-to-end proof this actually unblocks Emit: a row carrying the exact P6
// phantom-twin shape (unevaluable check both empty, real 3-position
// universe) must be credited, not refused as unevaluated.
func TestSetEqualityOverUniverseUnblocksEmit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MERIDIAN_VERDICT_DIR", dir)
	c := NewCounts()
	SetEquality(c, "positions_match_manifest", []string{"AAA", "BBB", "CCC", "ZZZ"}, []string{"AAA", "BBB", "CCC"})
	SetEqualityOverUniverse(c, "unevaluable_match_manifest", nil, nil, []string{"AAA", "BBB", "CCC"})

	ft := &fakeT{T: t}
	emitWith(ft, Row{Prop: 6, Cell: "twin", Counts: c, Params: map[string]any{},
		Planted: &Planted{ExpectedViolations: map[string]int64{"positions_match_manifest": 1, "unevaluable_match_manifest": 0}}})
	if ft.failed {
		t.Fatalf("P6 phantom-twin shape must be credited, got refused: %s", ft.lastMsg)
	}
}

// PositionKeys and UnevaluableInstruments are the extraction half of the
// same check; a bug in either would make TestSetEqualityTable's proof moot
// against a real snapshot.Doc, so both are driven directly here too.
func TestPositionKeysAndUnevaluableInstruments(t *testing.T) {
	doc := snapshot.Doc{
		"positions": map[string]any{
			"CCC": map[string]any{"qty": "1"},
			"AAA": map[string]any{"qty": "2"},
			"BBB": map[string]any{"qty": "3"},
		},
		"unevaluable": []any{
			map[string]any{"instrument": "CCC", "reason": "no_price_in_prefix"},
		},
	}
	pk := PositionKeys(doc)
	if len(pk) != 3 || pk[0] != "AAA" || pk[1] != "BBB" || pk[2] != "CCC" {
		t.Fatalf("PositionKeys = %v, want sorted [AAA BBB CCC]", pk)
	}
	ui := UnevaluableInstruments(doc)
	if len(ui) != 1 || ui[0] != "CCC" {
		t.Fatalf("UnevaluableInstruments = %v, want [CCC]", ui)
	}

	// Absent keys / wrong shapes must not panic — both return an empty
	// slice rather than crashing a gate that reads a malformed document.
	empty := snapshot.Doc{}
	if got := PositionKeys(empty); len(got) != 0 {
		t.Fatalf("PositionKeys(empty doc) = %v, want empty", got)
	}
	if got := UnevaluableInstruments(empty); len(got) != 0 {
		t.Fatalf("UnevaluableInstruments(empty doc) = %v, want empty", got)
	}
}
