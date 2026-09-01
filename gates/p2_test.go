package gates

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hossainpazooki/meridian/internal/feed"
)

func binary(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("MERIDIAN_BIN"); b != "" {
		return b
	}
	bin := filepath.Join(t.TempDir(), "meridian")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/meridian").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// freshProcessSnapshot invokes the CLI as two separate subprocess calls (a
// fresh process for each) and returns the pinned-hash-format string
// ("sha256:<hex>", first whitespace field of `snapshot`'s stdout) and the raw
// canonical snapshot bytes `asof` writes to stdout. Both commands replay the
// same feed prefix independently, so any statefulness leaking between calls
// within one process can never be mistaken for determinism here.
func freshProcessSnapshot(t *testing.T, bin, feedPath string) (hash string, bytes []byte) {
	t.Helper()
	out, err := exec.Command(bin, "asof", "--feed", feedPath).Output()
	if err != nil {
		t.Fatalf("asof: %v", err)
	}
	h, err := exec.Command(bin, "snapshot", "--feed", feedPath, "--out", t.TempDir()).Output()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return strings.Fields(string(h))[0], out
}

// replaysIdentical is the comparator behind fresh_process_identical, factored
// out of the check inline so TestReplaysIdenticalDiscriminates can drive it
// directly with known-different inputs. That test proves this function CAN
// report two artifacts as different — i.e. the comparator is not a tautology
// that would report identical no matter what. It does NOT, and cannot, prove
// the ledger itself is capable of producing two different fresh-process
// replays: planting genuine non-determinism into the fold would mean
// deliberately breaking the property P2 exists to guarantee, which this
// build does not do. Both facts hold at once — see that test's doc comment.
func replaysIdentical(h1 string, b1 []byte, h2 string, b2 []byte) bool {
	return h1 == h2 && bytes.Equal(b1, b2)
}

// TestP2DeterministicReplay: the live cell folds fixtures/base/feed.jsonl
// twice, each in its own subprocess, and requires the two runs to be
// byte-identical and to match the pinned hash. The twin cell exercises three
// raw re-chained feeds under fixtures/p2 — mutated, reordered, tampered —
// each of which must either diverge from the pin or (tampered) break the
// hash chain at the exact planted seq.
func TestP2DeterministicReplay(t *testing.T) {
	m := LoadManifest(t)
	bin := binary(t)
	base := filepath.Join(FixturesDir, "base", "feed.jsonl")
	pinRaw, err := os.ReadFile(filepath.Join(FixturesDir, "base", "snapshot.sha256"))
	if err != nil {
		t.Fatalf("pin missing: run `go run ./cmd/meridian snapshot --feed fixtures/base/feed.jsonl --out /tmp/x` and write the hash to fixtures/base/snapshot.sha256")
	}
	pin := strings.TrimSpace(string(pinRaw))

	h1, b1 := freshProcessSnapshot(t, bin, base)
	h2, b2 := freshProcessSnapshot(t, bin, base)
	c := NewCounts("fresh_process_identical", "pinned_hash_match", "chain_verifies")
	// fresh_process_identical examines two things about the pair of fresh
	// runs: hash equality and byte equality.
	c.Evaluated["fresh_process_identical"] = 2
	// pinned_hash_match examines one thing: the first run's hash against the
	// pin.
	c.Evaluated["pinned_hash_match"] = 1
	// chain_verifies' denominator is the number of records the chain check
	// actually walks: the whole base feed, i.e. end_seq.
	c.Evaluated["chain_verifies"] = m.Int("end_seq")
	if !replaysIdentical(h1, b1, h2, b2) {
		c.Checks["fresh_process_identical"] = 1
	}
	if h1 != pin {
		c.Checks["pinned_hash_match"] = 1
	}
	if _, err := feed.Open(base); err != nil {
		c.Checks["chain_verifies"] = 1
	}
	params := map[string]any{"pinned": pin, "viewpoint": m.Int("end_seq")}
	Emit(t, Row{Prop: 2, Cell: "live", Scope: "fixtures/base folded twice in fresh processes", ContentHash: h1,
		Basis: "sha256 of canonical snapshot bytes", Rows: m.Int("end_seq"), Params: params, Counts: c})

	// Polarity note: in every OTHER gate's twin, a non-zero Checks value means
	// the artifact FAILED the check. Here it means the opposite: the guard
	// CORRECTLY fired on a defective feed. snapshot_hash_diverges_mutated is
	// the identical predicate (hm != pin) as the live cell's
	// pinned_hash_match (h1 != pin), just read the other way — which is what
	// makes this a genuine proof that the live check's sensitivity is real,
	// not assumed: the same comparator that scores 0 against a good feed in
	// the live row is shown here to score 1 against a known-bad one. The cost
	// is that the emitted row cannot be read in isolation — see the Scope
	// string on the Emit call below for the reader-facing version of this
	// note, since Emit's row schema has no separate field for it.
	ct := NewCounts("snapshot_hash_diverges_mutated", "snapshot_hash_diverges_reordered", "chain_break_detected")
	ct.Evaluated["snapshot_hash_diverges_mutated"] = 1
	ct.Evaluated["snapshot_hash_diverges_reordered"] = 1
	ct.Evaluated["chain_break_detected"] = 1
	if hm, _ := freshProcessSnapshot(t, bin, filepath.Join(FixturesDir, "p2", "mutated", "feed.jsonl")); hm != pin {
		ct.Checks["snapshot_hash_diverges_mutated"] = 1
	}
	if hr, _ := freshProcessSnapshot(t, bin, filepath.Join(FixturesDir, "p2", "reordered", "feed.jsonl")); hr != pin {
		ct.Checks["snapshot_hash_diverges_reordered"] = 1
	}
	_, err = feed.Open(filepath.Join(FixturesDir, "p2", "tampered", "feed.jsonl"))
	var ce *feed.ChainError
	if errors.As(err, &ce) && ce.Seq == m.Int("p2", "tampered", "break_at_seq") {
		ct.Checks["chain_break_detected"] = 1
	}
	tp := map[string]any{"mutated_seq": m.Int("p2", "mutated", "seq"), "reordered_seqs": m.Ints("p2", "reordered", "seqs"), "tampered_seq": m.Int("p2", "tampered", "seq"), "break_at_seq": m.Int("p2", "tampered", "break_at_seq")}
	Emit(t, Row{Prop: 2, Cell: "twin",
		Scope:       "fixtures/p2 mutated, reordered, tampered feeds vs the pin — NOTE polarity: unlike every other gate's twin, a non-zero check here means the guard correctly DETECTED the planted defect (a pass), not that the artifact failed it",
		ContentHash: pin, Basis: "pinned sha256 of the base snapshot the twins must diverge from", Rows: 3, Params: tp, Counts: ct, Planted: ptr(m.Planted("p2"))})
}

// TestReplaysIdenticalDiscriminates drives replaysIdentical directly with
// known-different inputs, the same shape as TestSetEqualityTable in
// manifest_test.go: P2's headline check (fresh_process_identical, the
// determinism claim itself) never goes non-zero anywhere in this build — its
// twin exercises pinned_hash_match and chain_verifies instead, so nothing
// demonstrates this specific comparator CAN report two artifacts as
// different. A 0 that has never been shown capable of being non-zero is not
// evidence, by this repo's own rule. This table closes that gap by proving
// the arithmetic in isolation, without touching the ledger — planting actual
// non-determinism into the fold would mean deliberately breaking the thing
// under test, which is not what this does or should do.
//
// What this DOES prove: the comparator discriminates — given two artifacts
// that really differ (by hash, by bytes, or both), it reports them as not
// identical, rather than being a tautology that always reports "identical"
// regardless of input.
//
// What this does NOT prove: that the ledger itself is capable of producing
// two different fresh-process replays of the same feed. That is exactly the
// property P2 asserts always holds, so no honest test in this build can
// demonstrate its negation without first breaking the ledger. The live
// cell's fresh_process_identical: 0 remains credible (it compares two real
// subprocess runs, so it is not vacuous) but is, and stays, unfalsified in
// that specific sense.
func TestReplaysIdenticalDiscriminates(t *testing.T) {
	cases := []struct {
		name          string
		h1            string
		b1            []byte
		h2            string
		b2            []byte
		wantIdentical bool
	}{
		{name: "same hash, same bytes — identical", h1: "sha256:aaa", b1: []byte("x"), h2: "sha256:aaa", b2: []byte("x"), wantIdentical: true},
		{name: "same hash, both empty bytes — identical", h1: "sha256:empty", b1: nil, h2: "sha256:empty", b2: nil, wantIdentical: true},
		{name: "different hash, same bytes — not identical", h1: "sha256:aaa", b1: []byte("x"), h2: "sha256:bbb", b2: []byte("x"), wantIdentical: false},
		{name: "same hash, different bytes — not identical", h1: "sha256:aaa", b1: []byte("x"), h2: "sha256:aaa", b2: []byte("y"), wantIdentical: false},
		{name: "different hash, different bytes — not identical", h1: "sha256:aaa", b1: []byte("x"), h2: "sha256:bbb", b2: []byte("y"), wantIdentical: false},
		{name: "different length bytes — not identical", h1: "sha256:aaa", b1: []byte("x"), h2: "sha256:aaa", b2: []byte("xx"), wantIdentical: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := replaysIdentical(tc.h1, tc.b1, tc.h2, tc.b2); got != tc.wantIdentical {
				t.Fatalf("replaysIdentical(%q, %v, %q, %v) = %v, want %v", tc.h1, tc.b1, tc.h2, tc.b2, got, tc.wantIdentical)
			}
		})
	}
}
