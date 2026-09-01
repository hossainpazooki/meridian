package gates

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/snapshot"
)

// p1Check evaluates at-most-once ingestion over a snapshot document.
func p1Check(doc snapshot.Doc, expected snapshot.Doc, dupID, colID string) Counts {
	c := NewCounts("duplicate_absorbed", "collision_refused", "position_after_dedupe")
	absorbed, _ := doc["absorbed"].([]any)
	c.Evaluated["duplicate_absorbed"] = 1
	found := false
	for _, a := range absorbed {
		if am, _ := a.(map[string]any); am["event_id"] == dupID {
			found = true
		}
	}
	if !found {
		c.Checks["duplicate_absorbed"] = 1
	}
	refusals, _ := doc["refusals"].([]any)
	c.Evaluated["collision_refused"] = 1
	found = false
	for _, r := range refusals {
		if rm, _ := r.(map[string]any); rm["event_id"] == colID && rm["kind"] == "collision" {
			found = true
		}
	}
	if !found {
		c.Checks["collision_refused"] = 1
	}
	ms := snapshot.Diff(expected, doc)
	c.Evaluated["position_after_dedupe"] = int64(snapshot.Leaves(expected))
	c.Checks["position_after_dedupe"] = int64(len(ms))
	return c
}

func TestP1AtMostOnce(t *testing.T) {
	m := LoadManifest(t)
	dupID, colID := m.Str("p1", "duplicate", "event_id"), m.Str("p1", "collision", "event_id")
	expected := LoadDoc(t, filepath.Join(FixturesDir, "base", "expected", "V3.json"))
	params := map[string]any{"duplicate_seq": m.Int("p1", "duplicate", "seq"), "collision_seq": m.Int("p1", "collision", "seq"), "viewpoint": m.Int("end_seq")}

	live := ReadFixture(t, "base/feed.jsonl", -1)
	c := p1Check(live.Doc, expected, dupID, colID)
	// Belt and braces: the absorbed record must carry the ledger-derived
	// key, AND there must be exactly one of it. Exactly one, not "at least
	// one" or "any" — P1 is the at-most-once property, so a fold that
	// absorbs the planted duplicate PLUS something spurious is itself a
	// violation, not a pass with an unchecked extra. The old `len(a) == 1`
	// guard treated any other count as "nothing to check here," which let a
	// spurious extra absorption through silently: the event_id loop in
	// p1Check already found the planted duplicate and reported 0, "absorbed"
	// is absent from the reduced golden (fixtures/base/expected/V3.json has
	// no "absorbed" key) so position_after_dedupe can't see it either, and
	// this guard was the only place left that could have caught it.
	absorbedList, _ := live.Doc["absorbed"].([]any)
	switch len(absorbedList) {
	case 1:
		if absorbedList[0].(map[string]any)["key"] != m.Str("p1", "duplicate", "key") {
			c.Checks["duplicate_absorbed"]++
		}
	default:
		c.Checks["duplicate_absorbed"]++
	}
	// snapshot.Diff only walks the golden's keys, so a doc that invents a
	// position or drops an instrument from "unevaluable" scores zero
	// mismatches and would otherwise pass. Close that hole with its own
	// named set-equality check against the manifest's published lists —
	// for BOTH cells. positions_at/unevaluable_at are keyed by viewpoint
	// (V1/V2/V3); P1 replays the whole base feed, i.e. viewpoint V3
	// (manifest.end_seq), for both the live doc and its twin, so both
	// compare against the same "V3" published list.
	//
	// positions_match_manifest stays on plain SetEquality: the compared
	// position sets ARE that check's full scope (see SetEquality's doc).
	// unevaluable_match_manifest uses SetEqualityOverUniverse: it asserts,
	// for every position P1 examines, whether that position's unevaluable-
	// or-not status matches golden — the universe is the published position
	// set, not the union of two unevaluable lists (which, for a document
	// where nothing is legitimately unevaluable, would be vacuous even
	// though a real universe was checked). This mirrors P6's identical
	// convention (positionsWant as the universe), landed the same way
	// across P3/P4/P6 for cross-gate consistency.
	positionsWant := m.Strs("positions_at", "V3")
	SetEquality(c, "positions_match_manifest", PositionKeys(live.Doc), positionsWant)
	SetEqualityOverUniverse(c, "unevaluable_match_manifest", UnevaluableInstruments(live.Doc), m.Strs("unevaluable_at", "V3"), positionsWant)

	Emit(t, Row{Prop: 1, Cell: "live", Scope: "fixtures/base end-of-feed snapshot", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: params, Counts: c})

	twin := LoadDoc(t, filepath.Join(FixturesDir, "p1", "twin", "snapshot.json"))
	ct := p1Check(twin, expected, dupID, colID)
	SetEquality(ct, "positions_match_manifest", PositionKeys(twin), positionsWant)
	SetEqualityOverUniverse(ct, "unevaluable_match_manifest", UnevaluableInstruments(twin), m.Strs("unevaluable_at", "V3"), positionsWant)
	raw, _ := json.Marshal(twin)
	Emit(t, Row{Prop: 1, Cell: "twin", Scope: "naive no-dedupe snapshot over fixtures/base", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the twin document as decoded and re-marshaled", Rows: int64(len(twin["positions"].(map[string]any))), Params: params, Counts: ct, Planted: ptr(m.Planted("p1"))})
}
