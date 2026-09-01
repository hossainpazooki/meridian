// Package gates holds the property gates. Each p<N>_test.go runs its live
// cell (must be GREEN) and its twin cell (must be RED with exactly the
// planted counts) and emits BASELINE-schema verdict rows.
package gates

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/hossainpazooki/meridian/internal/asof"
	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

const FixturesDir = "../fixtures"

type Manifest map[string]any

type Planted struct {
	Mutation           string
	MutatedRows        int64
	ExpectedViolations map[string]int64
}

func LoadManifest(t *testing.T) Manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(FixturesDir, "base", "manifest.json"))
	if err != nil {
		t.Fatalf("manifest: %v (run python fixtures/generate.py)", err)
	}
	v, err := canon.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return Manifest(v.(map[string]any))
}

func (m Manifest) get(path ...string) any {
	var cur any = map[string]any(m)
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			panic(fmt.Sprintf("manifest path %v: not an object at %q", path, p))
		}
		cur, ok = mm[p]
		if !ok {
			panic(fmt.Sprintf("manifest path %v: missing %q", path, p))
		}
	}
	return cur
}

func (m Manifest) Int(path ...string) int64 {
	n, err := m.get(path...).(json.Number).Int64()
	if err != nil {
		panic(err)
	}
	return n
}
func (m Manifest) Str(path ...string) string { return m.get(path...).(string) }
func (m Manifest) Strs(path ...string) []string {
	var out []string
	for _, v := range m.get(path...).([]any) {
		out = append(out, v.(string))
	}
	return out
}
func (m Manifest) Ints(path ...string) []int64 {
	var out []int64
	for _, v := range m.get(path...).([]any) {
		n, _ := v.(json.Number).Int64()
		out = append(out, n)
	}
	return out
}

// Planted reads <prop>.<twinKey> (default "twin").
func (m Manifest) Planted(prop string, twinKey ...string) Planted {
	key := "twin"
	if len(twinKey) > 0 {
		key = twinKey[0]
	}
	tw := m.get(prop, key).(map[string]any)
	p := Planted{ExpectedViolations: map[string]int64{}}
	if s, ok := tw["mutation"].(string); ok {
		p.Mutation = s
	}
	if n, ok := tw["mutated_rows"].(json.Number); ok {
		p.MutatedRows, _ = n.Int64()
	}
	for k, v := range tw["expected_violations"].(map[string]any) {
		p.ExpectedViolations[k], _ = v.(json.Number).Int64()
	}
	return p
}

func LoadDoc(t *testing.T, path string) snapshot.Doc {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := snapshot.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func ReadFixture(t *testing.T, rel string, seq int64) asof.Result {
	t.Helper()
	r, err := asof.Read(filepath.Join(FixturesDir, rel), seq)
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return r
}

func ptr(p Planted) *Planted { return &p }

func sha256Hex(b []byte) string { return canon.SHA256Hex(b) }

// PositionKeys returns the sorted instrument keys of doc's "positions" map.
// snapshot.Diff is one-sided (it walks the golden's keys only, which is
// required so a twin may legitimately carry keys a reduced golden never
// has) and so cannot by itself catch a ledger that invents a position.
// Callers pair this with SetEquality against the manifest's published
// positions_at list to close that hole as its own named check.
func PositionKeys(doc snapshot.Doc) []string {
	pm, _ := doc["positions"].(map[string]any)
	out := make([]string, 0, len(pm))
	for k := range pm {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// UnevaluableInstruments returns the sorted instruments named in doc's
// "unevaluable" list. Pairs with SetEquality against the manifest's
// published unevaluable_at list, for the same reason as PositionKeys: Diff
// alone cannot catch a ledger that silently drops an instrument it should
// have marked unevaluable.
func UnevaluableInstruments(doc snapshot.Doc) []string {
	list, _ := doc["unevaluable"].([]any)
	out := make([]string, 0, len(list))
	for _, u := range list {
		if um, ok := u.(map[string]any); ok {
			if inst, ok := um["instrument"].(string); ok {
				out = append(out, inst)
			}
		}
	}
	sort.Strings(out)
	return out
}

// SetEquality records, under name, len(set(got) ∪ set(want)) as the number
// of items evaluated, and len(set(got) ^ set(want)) — the symmetric-
// difference count between got and want (0 means an exact set match) —
// matching the generator's own sym_diff_count() definition exactly (true set
// semantics: duplicates within either slice collapse before comparing, so
// this is not a sorted-multiset merge). got and want need not be pre-sorted
// or pre-deduped. Counts' maps are shared (Counts wraps two map[string]int64
// fields), so mutations through c are visible to the caller's copy.
//
// The denominator is the UNION, not len(want). It was len(want) originally,
// which is wrong whenever the correct published want is legitimately empty
// (e.g. P6's phantom twin: no instrument should be in "unevaluable" at that
// viewpoint, so want=[] is honest) — that made Evaluated 0 even when got
// caught a real invented entry (want=[], got=["ZZZ"]), and the harness's
// evaluated>0 rule then refused a check that had correctly gone RED. The
// union is 0 only when both sets are truly empty (nothing published, nothing
// observed — genuinely vacuous, correctly refused) and is >0 whenever either
// side has anything to compare, including the invented-against-empty-want
// case this must credit.
func SetEquality(c Counts, name string, got, want []string) {
	gs, ws := map[string]bool{}, map[string]bool{}
	for _, s := range got {
		gs[s] = true
	}
	for _, s := range want {
		ws[s] = true
	}
	union := map[string]bool{}
	for s := range gs {
		union[s] = true
	}
	for s := range ws {
		union[s] = true
	}
	c.Evaluated[name] = int64(len(union))
	var mismatches int64
	for s := range gs {
		if !ws[s] {
			mismatches++
		}
	}
	for s := range ws {
		if !gs[s] {
			mismatches++
		}
	}
	c.Checks[name] = mismatches
}

// SetEqualityOverUniverse records the same mismatch arithmetic as SetEquality
// (symmetric difference of set(got) and set(want)) but sets the evaluated
// denominator to len(set(universe)) instead of len(set(got) ∪ set(want)).
//
// One rule governs which of the two functions a check should call, and it is
// the same rule for every gate — a check name's denominator is always the
// size of the universe it asserts over, and the two set-equality checks this
// harness defines assert over two DIFFERENT universes:
//
//   - "positions_match_manifest" asserts over the position sets themselves:
//     the check's whole claim is "the document's position keys are exactly
//     the published ones," so got and want ARE the universe, and plain
//     SetEquality (denominator = their union) is correct.
//
//   - "unevaluable_match_manifest" does NOT assert over "the set of
//     currently-unevaluable instruments" in isolation — it asserts, for
//     EVERY position in the ledger, whether that position's unevaluable-or-
//     not status matches the golden. The universe it examines is the
//     position set (typically the same list passed as positions_match_
//     manifest's want), not the union of two unevaluable lists. A portfolio
//     where every position is priced and the golden's "unevaluable" list is
//     also empty is a real, non-vacuous assertion — "we checked N positions
//     for unevaluable-ness and correctly found none" — not nothing examined;
//     SetEquality's own union denominator would wrongly read that as
//     Evaluated=0 and get the whole row refused by Emit's evaluated>0 rule.
//     Callers must use SetEqualityOverUniverse(..., positionUniverse) for
//     this check, in every gate, so "unevaluable_match_manifest" means the
//     same thing everywhere rather than being decided per gate.
//
// universe need not be pre-sorted or pre-deduped (it is deduped here, same
// set semantics as got/want).
func SetEqualityOverUniverse(c Counts, name string, got, want, universe []string) {
	SetEquality(c, name, got, want)
	u := map[string]bool{}
	for _, s := range universe {
		u[s] = true
	}
	c.Evaluated[name] = int64(len(u))
}
