package feed

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// pl builds a payload map; Go ints become json.Number as the feed expects.
func pl(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		switch v := kv[i+1].(type) {
		case int:
			m[kv[i].(string)] = json.Number(strconv.Itoa(v))
		default:
			m[kv[i].(string)] = v
		}
	}
	return m
}

func fmtInt(i int) string { return strconv.Itoa(i) }

// mustAppend is setup-only sugar for tests where the append itself isn't
// what's under test: it fails the test immediately, at the real point of
// failure, instead of letting a bad append surface later as a confusing
// mismatch several assertions downstream.
func mustAppend(t *testing.T, f *Feed, typ, id, effective string, payload map[string]any) Record {
	t.Helper()
	r, err := f.Append(typ, id, effective, payload)
	if err != nil {
		t.Fatalf("setup Append(%s) failed: %v", id, err)
	}
	return r
}

func mustOpen(t *testing.T, path string) *Feed {
	t.Helper()
	f, err := Open(path)
	if err != nil {
		t.Fatalf("setup Open(%s) failed: %v", path, err)
	}
	return f
}

func TestAppendThenReopenVerifiesChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := f.Append("fill", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1000, "qty", 100, "side", "buy", "trade_id", "T-1", "venue", "X"))
	if err != nil {
		t.Fatal(err)
	}
	if r1.Seq != 1 || r1.Prev != Genesis {
		t.Fatalf("bad first record: %+v", r1)
	}
	r2, err := f.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 1100))
	if err != nil {
		t.Fatal(err)
	}
	if r2.Prev != r1.LineHash {
		t.Fatalf("chain not linked: prev=%s want %s", r2.Prev, r1.LineHash)
	}
	f.Close()

	raw, _ := os.ReadFile(path)
	line1 := strings.SplitN(string(raw), "\n", 2)[0]
	want := `{"effective":"2026-01-05","id":"ev-1","payload":{"instrument":"AAA","price":1000,"qty":100,"side":"buy","trade_id":"T-1","venue":"X"},"prev":"` + Genesis + `","seq":1,"type":"fill"}`
	if line1 != want {
		t.Fatalf("line 1 not canonical:\n%s\n%s", line1, want)
	}

	g, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if g.Len() != 2 || g.Records()[1].Payload["price"].(json.Number).String() != "1100" {
		t.Fatalf("reopen lost records: %+v", g.Records())
	}
	h, _ := g.PrefixHash(2)
	if h != "sha256:"+r2.LineHash {
		t.Fatalf("prefix hash %s want sha256:%s", h, r2.LineHash)
	}
	if h0, _ := g.PrefixHash(0); h0 != "sha256:"+Genesis {
		t.Fatal(h0)
	}
	r3, err := g.Append("price", "ev-3", "2026-01-07", pl("instrument", "AAA", "price", 1200))
	if err != nil {
		t.Fatal(err)
	}
	if r3.Seq != 3 || r3.Prev != r2.LineHash {
		t.Fatalf("append after reopen broke chain: %+v", r3)
	}
}

func TestCRLFCheckoutHashesIdentically(t *testing.T) {
	dir := t.TempDir()
	lf := filepath.Join(dir, "lf.jsonl")
	f := mustOpen(t, lf)
	mustAppend(t, f, "price", "ev-1", "2026-01-06", pl("instrument", "AAA", "price", 1100))
	mustAppend(t, f, "price", "ev-2", "2026-01-07", pl("instrument", "AAA", "price", 1200))
	f.Close()
	raw, _ := os.ReadFile(lf)
	crlf := filepath.Join(dir, "crlf.jsonl")
	os.WriteFile(crlf, []byte(strings.ReplaceAll(string(raw), "\n", "\r\n")), 0o644)
	g, err := Open(crlf)
	if err != nil {
		t.Fatalf("CRLF feed must verify: %v", err)
	}
	defer g.Close()
	a, _ := g.PrefixHash(2)
	h := mustOpen(t, lf)
	defer h.Close()
	b, _ := h.PrefixHash(2)
	if a != b {
		t.Fatalf("CRLF changed the prefix hash: %s vs %s", a, b)
	}
}

func TestTamperedRecordIsRefusedAtNextSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	for i := 1; i <= 3; i++ {
		mustAppend(t, f, "price", "ev-"+fmtInt(i), "2026-01-0"+fmtInt(i+4), pl("instrument", "AAA", "price", 1000+i))
	}
	f.Close()
	raw, _ := os.ReadFile(path)
	tampered := strings.Replace(string(raw), `"price":1002`, `"price":1003`, 1)
	os.WriteFile(path, []byte(tampered), 0o644)
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) || ce.Seq != 3 {
		t.Fatalf("want ChainError at seq 3, got %v", err)
	}
}

func TestSeqGapIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	mustAppend(t, f, "price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	f.Close()
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	os.WriteFile(path, []byte(lines[1]+"\n"), 0o644) // drop seq 1
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) || ce.Seq != 2 {
		t.Fatalf("want ChainError at seq 2, got %v", err)
	}
	if !strings.Contains(ce.Reason, "gap") {
		t.Fatalf("want a gap-specific message, got %q", ce.Reason)
	}
}

// TestSeqGapMidFeedIsRefused: TestSeqGapIsRefused only drops the head
// record (seq 1). A gap in the MIDDLE of an otherwise-intact feed must be
// refused the same way.
func TestSeqGapMidFeedIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	for i := 1; i <= 4; i++ {
		mustAppend(t, f, "price", "ev-"+fmtInt(i), "2026-01-0"+fmtInt(i+4), pl("instrument", "AAA", "price", 1000+i))
	}
	f.Close()
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	kept := []string{lines[0], lines[1], lines[3]} // drop seq 3, keep 1, 2, 4
	os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChainError, got %v", err)
	}
	if ce.Seq != 4 {
		t.Fatalf("want ChainError at seq 4 (the gap), got %d", ce.Seq)
	}
}

// TestDuplicateSeqGetsDistinctMessageFromGap: a seq at or below the last
// accepted one is a duplicate/regression, not a gap, and must be reported
// with its own message — a gap means something is missing; a duplicate
// means two records were independently accepted at the same position
// (this is exactly what the pre-poisoning fsync-retry defect would have
// produced from the same handle).
func TestDuplicateSeqGetsDistinctMessageFromGap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	r2 := mustAppend(t, f, "price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	f.Close()
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	withDup := append(append([]string{}, lines...), lines[1]) // duplicate record 2's own line
	os.WriteFile(path, []byte(strings.Join(withDup, "\n")+"\n"), 0o644)
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChainError, got %v", err)
	}
	if ce.Seq != r2.Seq {
		t.Fatalf("want ChainError naming the duplicate seq %d, got %d", r2.Seq, ce.Seq)
	}
	if !strings.Contains(ce.Reason, "duplicate") {
		t.Fatalf("want the duplicate-specific message, got %q", ce.Reason)
	}
	// The gap message's distinguishing phrase is "expected (gap)" (see
	// TestSeqGapIsRefused/TestSeqGapMidFeedIsRefused) — the duplicate
	// message correctly says "not a gap", which would trip a naive
	// substring check for "gap" alone, so check for the gap message's
	// actual shape instead.
	if strings.Contains(ce.Reason, "expected (gap)") {
		t.Fatalf("a duplicate must not be described with the gap message: %q", ce.Reason)
	}
}

// TestNonCanonicalRecordIsRefused: an extra key ("extra", inserted before
// "effective" — the wrong position for it: "effective" < "extra"
// alphabetically) survives decode with every required field still present
// and correctly typed. Refused via the explicit key-set check (7 keys,
// not 6) — NOT, as an earlier version of this test claimed, via the
// canonicality re-marshal: the key-set check runs first and catches any
// extra key regardless of where it sorts, so canonicality never gets a
// chance to fire here. See TestExtraKeyAtSortedPositionIsRefused for the
// specific bypass this closed (an extra key at a position that WOULD
// survive the canonicality re-marshal), and
// TestReorderedKeysOnAnUnchangedKeySetIsRefused for a case the key-set
// check does NOT catch and canonicality alone must.
func TestNonCanonicalRecordIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	for i := 1; i <= 3; i++ {
		mustAppend(t, f, "price", "ev-"+fmtInt(i), "2026-01-0"+fmtInt(i+4), pl("instrument", "AAA", "price", 1000+i))
	}
	f.Close()
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	lines[1] = strings.Replace(lines[1], `{"effective"`, `{"extra":"malicious","effective"`, 1)
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChainError for a non-canonical record, got %v", err)
	}
	if ce.Seq != 2 {
		t.Fatalf("want ChainError at seq 2, got %d", ce.Seq)
	}
	if !strings.Contains(ce.Reason, "extra key") {
		t.Fatalf("want the extra-key-specific message, got %q", ce.Reason)
	}
}

// TestReorderedKeysOnAnUnchangedKeySetIsRefused: exactly the 6 required
// keys, just not in canon.Marshal's sorted order — the key-set check
// (which only counts keys) does not fire, so this is the case that proves
// the canonicality re-marshal check still does independent work after the
// key-set check was added.
func TestReorderedKeysOnAnUnchangedKeySetIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Close()
	raw, _ := os.ReadFile(path)
	line := strings.TrimRight(string(raw), "\n")
	const suffix = `,"type":"price"}`
	if !strings.HasSuffix(line, suffix) {
		t.Fatalf("fixture assumption broke, line = %s", line)
	}
	body := strings.TrimSuffix(line, suffix)
	// Move "type" from its canonical (last) position to first.
	reordered := strings.Replace(body, `{`, `{"type":"price",`, 1) + `}`
	os.WriteFile(path, []byte(reordered+"\n"), 0o644)

	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChainError for reordered keys, got %v", err)
	}
	if !strings.Contains(ce.Reason, "canonical") {
		t.Fatalf("want the canonicality-specific message, got %q", ce.Reason)
	}
}

// TestPrevWithNonHexCharactersIsRefused: prev is length-checked (inside
// recordFromValue, residue-tolerable if it fails) AND, separately,
// character-class-checked by Open itself as an unconditional check (a
// right-length-wrong-class value cannot come from a torn write — only
// editing produces a complete, correctly-typed record with an internally
// wrong field). Confirms this fires through the explicit check, not by
// accident via the generic residue path: a single record with a right-
// length, wrong-class prev refuses even though it is the LAST line (residue
// tolerance would otherwise apply if this fell through to markResidue).
func TestPrevWithNonHexCharactersIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Close()
	raw, _ := os.ReadFile(path)
	badHex := strings.Repeat("z", 64) // right length, wrong character class
	tampered := strings.Replace(string(raw), `"prev":"`+Genesis+`"`, `"prev":"`+badHex+`"`, 1)
	os.WriteFile(path, []byte(tampered), 0o644)
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChainError, got %v", err)
	}
	if !strings.Contains(ce.Reason, "hex") {
		t.Fatalf("want the hex-specific message naming the field, got %q", ce.Reason)
	}
}

// TestExtraKeyAtSortedPositionIsRefused pins the fix for the hole a
// reviewer found in the canonicality check: canon.Marshal sorts keys, so
// an extra key inserted at exactly the position its name would sort to
// (here "backdoor" sorts before "effective") round-trips byte-for-byte —
// the canonicality re-marshal cannot see a key it faithfully preserves.
// Closed by a separate, explicit key-set check (exactly
// {effective,id,payload,prev,seq,type}, no more) that Open runs
// unconditionally once a record's required fields all check out. Tested
// on both a middle line (no chain knock-on to confuse the result — the
// line itself is what's refused) and the last line (the specific case
// measured as silently ACCEPTED before this fix).
func TestExtraKeyAtSortedPositionIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		lineIdx int // which physical line (0-indexed) to tamper
	}{
		{"middle line", 1},
		{"last line", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "feed.jsonl")
			f := mustOpen(t, path)
			for i := 1; i <= 3; i++ {
				mustAppend(t, f, "price", "ev-"+fmtInt(i), "2026-01-0"+fmtInt(i+4), pl("instrument", "AAA", "price", 1000+i))
			}
			f.Close()
			raw, _ := os.ReadFile(path)
			lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
			lines[tc.lineIdx] = strings.Replace(lines[tc.lineIdx], `{"effective"`, `{"backdoor":"yes","effective"`, 1)
			os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
			_, err := Open(path)
			var ce *ChainError
			if !errors.As(err, &ce) {
				t.Fatalf("want ChainError for an extra key at its sorted position, got %v", err)
			}
			if !strings.Contains(ce.Reason, "extra key") {
				t.Fatalf("want the extra-key-specific message, got %q", ce.Reason)
			}
			wantSeq := int64(tc.lineIdx + 1)
			if ce.Seq != wantSeq {
				t.Fatalf("want ChainError at seq %d, got %d", wantSeq, ce.Seq)
			}
		})
	}
}

// TestResidueBypassIsAnAcceptedHonestLimit pins the known, deliberately
// unclosed gap documented in the package doc's "Honest limits" section: a
// corrupted last record, followed by a single unterminated junk byte, is
// byte-identical to the legitimate crash-loop-recovery-in-progress shape
// (see TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly), so it is
// silently tolerated rather than refused, and the corrupted record is
// gone. This is NOT a desired outcome — it is pinned here, explicitly
// labeled as an accepted limit, so a future change doesn't "fix" this one
// case in a way that silently reintroduces the bound-of-one bricking bug
// this package already paid to remove.
func TestResidueBypassIsAnAcceptedHonestLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	mustAppend(t, f, "price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	f.Close()

	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	lines[1] = "corrupted, not json" // the last record, now garbage
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fh.WriteString(strings.Join(lines, "\n") + "\n")
	fh.Close()
	// One more unterminated byte, as if a second write started and died.
	fh2, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fh2.WriteString("x")
	fh2.Close()

	g, err := Open(path)
	if err != nil {
		t.Fatalf("this shape is accepted by design (see package doc): %v", err)
	}
	defer g.Close()
	if g.Len() != 1 {
		t.Fatalf("the corrupted record is silently dropped by design (see package doc): len=%d", g.Len())
	}
	if g.UnparseableLines() != 2 {
		t.Fatalf("want 2 tolerated residue lines, got %d", g.UnparseableLines())
	}
}

func TestTornTailToleratedAndNextAppendStartsFreshLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Close()
	fh, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fh.WriteString(`{"effective":"2026-01-06","id":"ev-2","pay`) // torn, no newline
	fh.Close()
	g, err := Open(path)
	if err != nil {
		t.Fatalf("torn tail must be ignored: %v", err)
	}
	if g.Len() != 1 {
		t.Fatalf("torn tail must be ignored: len=%d", g.Len())
	}
	r, err := g.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	if err != nil {
		t.Fatal(err)
	}
	if r.Seq != 2 {
		t.Fatal(r)
	}
	g.Close()
	h, err := Open(path)
	if err != nil {
		t.Fatalf("append after torn tail must recover cleanly: %v", err)
	}
	if h.Len() != 2 {
		t.Fatalf("append after torn tail must recover cleanly: len=%d", h.Len())
	}
	h.Close()
}

// --- Fix round 3: canonicality check, hex-validated prev, duplicate-vs-gap
// diagnostics, blank lines counted as residue. See feed.go's package doc.
// ---

// TestBlankLineCountsAsResidueAndIsToleratedWhenFollowedByValidRecord: a
// blank line is not a record, but it must not be invisible either — it
// counts toward UnparseableLines(). This is the "something valid follows
// it" shape; see TestBlankLinesAtEOFAreToleratedAndCounted for proof that
// tolerance does NOT depend on that (a blank line is tolerated regardless
// of what, if anything, follows it — see the package doc for why).
func TestBlankLineCountsAsResidueAndIsToleratedWhenFollowedByValidRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	mustAppend(t, f, "price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	f.Close()
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	// Splice a blank line between the two valid records — the blank line
	// is terminated and IS followed by a valid record, the shape that
	// must be tolerated.
	out := []string{lines[0], "", lines[1]}
	os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)

	g := mustOpen(t, path)
	defer g.Close()
	if g.Len() != 2 {
		t.Fatalf("want 2 valid records (the blank line is not one): len=%d", g.Len())
	}
	if g.UnparseableLines() != 1 {
		t.Fatalf("blank line must be counted as residue: got %d", g.UnparseableLines())
	}
}

// TestBlankLinesAtEOFAreToleratedAndCounted: fix round 5's regression test.
// An earlier version of this test asserted the opposite — that trailing
// blank lines at EOF with nothing after them must be REFUSED, by the same
// EOF guard that governs non-blank residue. That was itself a bug: a
// blank line carries no content, so unlike non-blank garbage it can never
// itself be the mangled remains of a corrupted record — a terminated
// blank line at EOF is not evidence of a dropped record, on any
// reasoning that also has to explain why the crash-loop shape (see
// TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly) is tolerated.
// Under the old rule, any editor, `>>`, or "ensure a final newline" tool
// permanently bricked an otherwise-pristine feed — the same class of
// defect as the bound-of-one bug, reintroduced by folding blank lines
// into the wrong rule. Blank lines must still open cleanly, and must
// still be counted (the file is reported as not pristine).
func TestBlankLinesAtEOFAreToleratedAndCounted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Close()
	fh, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fh.WriteString("\n\n") // two blank lines, both terminated, nothing after
	fh.Close()
	g, err := Open(path)
	if err != nil {
		t.Fatalf("trailing blank lines must not brick the feed: %v", err)
	}
	defer g.Close()
	if g.Len() != 1 {
		t.Fatalf("want the one real record, got len=%d", g.Len())
	}
	if g.UnparseableLines() != 2 {
		t.Fatalf("want both blank lines counted as residue, got %d", g.UnparseableLines())
	}
}

// flakyReader returns a fixed sequence of bytes, then fails every
// subsequent Read with a non-EOF error — simulating a genuine I/O error
// partway through a file, as opposed to genuinely reaching EOF. A real
// *os.File can't be made to fail a Read deterministically in a unit test,
// which is why this drives the internal `scan` function directly instead
// of going through the public Open/*os.File path. bufio.Reader still
// bundles whatever partial line bytes it had already buffered together
// with this error on the call that returns it, exactly matching what a
// real I/O error produces mid-file (see TestNonEOFReadErrorHardFails).
type flakyReader struct {
	data []byte
	pos  int
	err  error
}

func (r *flakyReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// TestNonEOFReadErrorHardFails pins fix round 5's I2: a genuine (non-EOF)
// read error partway through the file must hard-fail, never be silently
// treated as a torn tail — even though the partial line it leaves behind
// (no trailing '\n', fails to parse) looks structurally identical to one.
// Before this fix, all three `!complete { break }` sites in scan exited
// the loop before ever reaching the rerr-is-a-real-error check below the
// loop, so this exact shape opened successfully with the second record
// silently missing.
func TestNonEOFReadErrorHardFails(t *testing.T) {
	line1 := `{"effective":"2026-01-05","id":"ev-1","payload":{"instrument":"AAA","price":1},"prev":"` + Genesis + `","seq":1,"type":"price"}` + "\n"
	partial := `{"effective":"2026-01-06","id":"ev-2","partial` // no trailing newline: looks exactly like a torn tail
	simulatedErr := errors.New("simulated EIO")
	r := &flakyReader{data: []byte(line1 + partial), err: simulatedErr}

	fd := &Feed{}
	err := scan(fd, bufio.NewReader(r))
	if err == nil {
		t.Fatal("want a hard failure, got nil — a real read error must never be silently treated as a torn tail")
	}
	if errors.Is(err, io.EOF) {
		t.Fatalf("want the real error, not io.EOF: %v", err)
	}
	if !errors.Is(err, simulatedErr) {
		t.Fatalf("want the actual read error to propagate, got %v", err)
	}
	if fd.Len() != 1 {
		t.Fatalf("the one fully-valid record before the error should still be visible either way, got len=%d", fd.Len())
	}
}

// flakyFile wraps a real *os.File so a test can deterministically fail the
// Nth Write (as a short write — some bytes really do land, like a real
// ENOSPC mid-write) or the Nth Sync call, without depending on OS-level
// disk-full conditions.
//
// This is an in-package seam, not a real syscall failure: it proves that
// Feed.Append's error-handling code (needNewline + poisoning) correctly
// reacts when Write/Sync return an error through the fileHandle interface.
// It does NOT prove behavior under a genuine OS-level failure (real
// ENOSPC/EIO) — that path is exercised through the exact same Go code
// (Append doesn't know or care whether *os.File or flakyFile is behind the
// interface), but was not itself triggered here.
type flakyFile struct {
	*os.File
	failWriteAt int // 1-indexed call number to fail; 0 = never
	failSyncAt  int
	writeCalls  int
	syncCalls   int
}

func (w *flakyFile) Write(p []byte) (int, error) {
	w.writeCalls++
	if w.failWriteAt != 0 && w.writeCalls == w.failWriteAt {
		n := len(p) / 2 // partial bytes really land, then the call fails
		w.File.Write(p[:n])
		return n, io.ErrShortWrite
	}
	return w.File.Write(p)
}

func (w *flakyFile) Sync() error {
	w.syncCalls++
	if w.failSyncAt != 0 && w.syncCalls == w.failSyncAt {
		return errors.New("simulated fsync failure")
	}
	return w.File.Sync()
}

// TestWriteFailurePoisonsHandleAndRecoversCleanly: a short Write must not
// let a same-handle retry glue its bytes onto the unknown leftover ones.
// Poisoning prevents this outright — the retry is refused, not silently
// accepted with different content at the same seq. Reopening then tolerates
// the partial bytes as a torn tail and recovers cleanly (asserts len==2
// after the retry-via-reopen).
func TestWriteFailurePoisonsHandleAndRecoversCleanly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))

	real := f.f.(*os.File)
	flaky := &flakyFile{File: real, failWriteAt: 1}
	f.f = flaky

	_, err := f.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	if err == nil {
		t.Fatal("expected the simulated short write to fail")
	}
	if !f.needNewline {
		t.Fatal("needNewline must be set after a write failure")
	}
	if f.poisoned == nil {
		t.Fatal("handle must be poisoned after a write failure")
	}
	if !errors.Is(f.poisoned, io.ErrShortWrite) {
		t.Fatalf("poisoning cause should be traceable to the write failure, got %v", f.poisoned)
	}

	// Retry on the SAME handle: must be refused outright (poisoned), never
	// silently glued onto the partial bytes from the failed write.
	if _, err2 := f.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2)); !errors.Is(err2, ErrPoisoned) {
		t.Fatalf("want ErrPoisoned on a poisoned handle, got %v", err2)
	}
	f.Close()

	// Reopen: the partial write is a torn tail (unterminated, at EOF) —
	// tolerated, Len()==1, no error.
	g, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after a write failure must tolerate the partial bytes: %v", err)
	}
	if g.Len() != 1 {
		t.Fatalf("reopen after a write failure must tolerate the partial bytes: len=%d", g.Len())
	}
	r, err := g.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	if err != nil || r.Seq != 2 {
		t.Fatalf("append on the reopened handle must recover cleanly: err=%v r=%+v", err, r)
	}
	g.Close()

	// Final reopen: the recovery append's leading '\n' terminates the old
	// torn bytes, which are now followed by a valid record — tolerated.
	// This is the "partial-write-plus-retry ends at len==2" assertion.
	h, err := Open(path)
	if err != nil {
		t.Fatalf("final reopen must be clean: %v", err)
	}
	if h.Len() != 2 {
		t.Fatalf("final reopen must be clean: len=%d", h.Len())
	}
	h.Close()
}

// TestSyncFailurePoisonsHandleButBytesAreDurable pins the asymmetry with
// the Write-failure case: when Write fully succeeds and only Sync fails,
// the complete valid record IS on disk (fsync failing means durability
// wasn't confirmed, not that the write didn't happen), so a reopen finds
// and accepts it even though the Feed instance that attempted it was told
// it failed. The handle is still poisoned (asserted directly below), and
// needNewline is still forced, purely because this handle's own
// bookkeeping can no longer be trusted — without poisoning, a same-handle
// retry would be handed seq 2 again and silently duplicate it (that is the
// defect this test guards against; see also
// TestDuplicateSeqGetsDistinctMessageFromGap for the diagnostic a caller
// would have seen had poisoning not existed).
func TestSyncFailurePoisonsHandleButBytesAreDurable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))

	real := f.f.(*os.File)
	flaky := &flakyFile{File: real, failSyncAt: 1}
	f.f = flaky

	_, err := f.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	if err == nil {
		t.Fatal("expected the simulated fsync failure")
	}
	if f.poisoned == nil {
		t.Fatal("handle must be poisoned after a sync failure")
	}
	if !f.needNewline {
		t.Fatal("needNewline must be set after a sync failure")
	}
	if _, err2 := f.Append("price", "ev-2-retry", "2026-01-06", pl("instrument", "AAA", "price", 2)); !errors.Is(err2, ErrPoisoned) {
		t.Fatalf("want ErrPoisoned, got %v", err2)
	}
	f.Close()

	// The Write fully landed before Sync failed, so the record is really
	// there — a reopen must find it, even though this Feed never recorded
	// it as a success.
	g, err := Open(path)
	if err != nil {
		t.Fatalf("bytes from the Write that preceded the failed Sync must survive: %v", err)
	}
	if g.Len() != 2 {
		t.Fatalf("bytes from the Write that preceded the failed Sync must survive: len=%d", g.Len())
	}
	g.Close()
}

// TestLastLineGarbledTerminatedIsRefused: a newline-terminated, unparseable
// last line with nothing after it must be refused, not tolerated as a torn
// tail. The distinguishing feature of a genuine torn tail is that it's
// either unterminated or followed by a valid record — this line is
// neither.
func TestLastLineGarbledTerminatedIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Close()
	fh, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fh.WriteString(`{"effective":"2026-01-06","id":"ev-2","payload":{BROKEN` + "\n")
	fh.Close()
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChainError, got %v", err)
	}
}

// TestLastRecordSeqAsStringIsRefused: the brief's own parse() requires
// "seq" to be a JSON number; quoting it produces syntactically valid JSON
// that still fails our record-shape parse. As the terminated last line
// with nothing after it, it must hard-fail.
func TestLastRecordSeqAsStringIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	for i := 1; i <= 3; i++ {
		mustAppend(t, f, "price", "ev-"+fmtInt(i), "2026-01-0"+fmtInt(i+4), pl("instrument", "AAA", "price", 1000+i))
	}
	f.Close()
	raw, _ := os.ReadFile(path)
	tampered := strings.Replace(string(raw), `"seq":3`, `"seq":"3"`, 1)
	os.WriteFile(path, []byte(tampered), 0o644)
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChainError, got %v", err)
	}
	if ce.Seq != 3 {
		t.Fatalf("want ChainError at seq 3, got %d", ce.Seq)
	}
}

// TestJunkLinesMidFeedToleratedWhenFollowedByValidRecords supersedes what
// was TestThreeJunkLinesMidFeedRefused: the bound of one tolerated
// unparseable line has been removed (see feed.go's package doc — it
// permanently bricked a feed after two ordinary crashes). Three junk lines
// interspersed among five valid records, ending on a valid record, must
// now open cleanly with all five records intact and the junk count
// reported.
func TestJunkLinesMidFeedToleratedWhenFollowedByValidRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	for i := 1; i <= 5; i++ {
		mustAppend(t, f, "price", "ev-"+fmtInt(i), "2026-01-0"+fmtInt(i+4), pl("instrument", "AAA", "price", 1000+i))
	}
	f.Close()
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	out := []string{lines[0], "not json at all 1", lines[1], "not json at all 2", lines[2], "not json at all 3", lines[3], lines[4]}
	os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
	g, err := Open(path)
	if err != nil {
		t.Fatalf("junk lines each followed by a valid record must be tolerated: %v", err)
	}
	defer g.Close()
	if g.Len() != 5 {
		t.Fatalf("want all 5 valid records, got %d", g.Len())
	}
	if g.UnparseableLines() != 3 {
		t.Fatalf("want 3 tolerated junk lines, got %d", g.UnparseableLines())
	}
}

// TestTornTailAcrossMultipleReopensStillRecovers: a torn tail must stay
// tolerated across any number of Opens that don't append — Open is a pure
// re-derivation from disk, so re-scanning the same unterminated bytes
// twice must be as harmless as scanning them once. (Torn-tail ordering 2
// of 3 — see also TestTornTailToleratedAndNextAppendStartsFreshLine and
// TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly.)
func TestTornTailAcrossMultipleReopensStillRecovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Close()
	fh, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fh.WriteString(`{"effective":"2026-01-06","id":"ev-2","tor`) // torn, no newline
	fh.Close()

	for i := 0; i < 3; i++ {
		g, err := Open(path)
		if err != nil {
			t.Fatalf("reopen %d: torn tail must still be tolerated: %v", i, err)
		}
		if g.Len() != 1 {
			t.Fatalf("reopen %d: torn tail must still be tolerated: len=%d", i, g.Len())
		}
		g.Close()
	}

	g := mustOpen(t, path)
	r, err := g.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	if err != nil || r.Seq != 2 {
		t.Fatalf("append after repeated reopens must recover cleanly: err=%v r=%+v", err, r)
	}
	g.Close()
	h, err := Open(path)
	if err != nil {
		t.Fatalf("final reopen must be clean: %v", err)
	}
	if h.Len() != 2 {
		t.Fatalf("final reopen must be clean: len=%d", h.Len())
	}
	h.Close()
}

// TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly is the
// regression test for the bound-of-one defect: two successive crashes,
// each one's recovery attempt gluing its own leading '\n' onto the
// previous crash's residue (terminating it without making it valid),
// followed by one genuinely successful append. Under the old
// bound-of-one rule this file was permanently unopenable. It must now
// open cleanly, with both garbage lines counted and the valid record
// intact. (Torn-tail ordering 3 of 3.)
func TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))

	// Crash 1: a short write leaves a genuine torn tail.
	real1 := f.f.(*os.File)
	f.f = &flakyFile{File: real1, failWriteAt: 1}
	if _, err := f.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2)); err == nil {
		t.Fatal("expected the first simulated crash to fail")
	}
	f.Close()

	// Recovery attempt after crash 1 also crashes (crash 2). Its write
	// buffer is "\n" + record + "\n" — any non-empty partial write of it
	// necessarily includes that leading '\n', which terminates crash 1's
	// residue without validating it, and itself leaves a second, still
	// unterminated, residue.
	g, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after crash 1 must tolerate the torn tail: %v", err)
	}
	if g.Len() != 1 {
		t.Fatalf("reopen after crash 1: want len 1, got %d", g.Len())
	}
	real2 := g.f.(*os.File)
	g.f = &flakyFile{File: real2, failWriteAt: 1}
	if _, err := g.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2)); err == nil {
		t.Fatal("expected the second simulated crash to fail")
	}
	g.Close()

	// Before any successful append: the file now holds one valid record
	// and two unparseable lines (crash 1's residue, now terminated by
	// crash 2's leading '\n'; crash 2's own residue, still unterminated).
	// This must still open — under the old bound-of-one rule it would not.
	h, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after two successive crashes must not brick the feed: %v", err)
	}
	if h.Len() != 1 {
		t.Fatalf("want len 1 after two crashes with no successful append yet, got %d", h.Len())
	}
	if h.UnparseableLines() != 2 {
		t.Fatalf("want 2 tolerated garbage lines, got %d", h.UnparseableLines())
	}

	// The actual recovery: this append's leading '\n' terminates crash 2's
	// residue, and finally succeeds.
	r, err := h.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	if err != nil || r.Seq != 2 {
		t.Fatalf("append after two crashes must recover cleanly: err=%v r=%+v", err, r)
	}
	h.Close()

	// This is the regression this test pins: a file with two unparseable
	// lines from successive crashes, followed by a valid record, must
	// open cleanly.
	final, err := Open(path)
	if err != nil {
		t.Fatalf("final reopen (2 garbage lines + 1 valid record) must be clean: %v", err)
	}
	if final.Len() != 2 {
		t.Fatalf("want final len 2, got %d", final.Len())
	}
	if final.UnparseableLines() != 2 {
		t.Fatalf("want 2 tolerated garbage lines on final reopen, got %d", final.UnparseableLines())
	}
	final.Close()
}

// TestRecordsPayloadIsDeepCopied: mutating a Payload map returned by
// Records() must not change what the Feed subsequently reports.
func TestRecordsPayloadIsDeepCopied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	mustAppend(t, f, "price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	defer f.Close()

	recs := f.Records()
	recs[0].Payload["price"] = json.Number("999999")

	recs2 := f.Records()
	if recs2[0].Payload["price"].(json.Number).String() != "1" {
		t.Fatalf("mutating a returned Record's Payload must not affect the Feed: got %v", recs2[0].Payload["price"])
	}
}

// TestAppendCopiesCallerPayload: mutating the payload map after passing it
// to Append must not change what was recorded in memory or on disk — AND,
// per fix round 5, mutating the RETURNED Record's Payload must not change
// what the Feed itself subsequently reports either. The two are separate
// bugs: an earlier version of Append deep-copied the caller's map on
// ingress (closing the first) but then handed out and stored the SAME
// copy (leaving the second open) — a caller mutating r.Payload was
// silently mutating the Feed's own in-memory state, with disk and memory
// diverging and no error to show for it. This test previously only
// exercised the first direction, which is exactly why it passed despite
// the second bug being live.
func TestAppendCopiesCallerPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	defer f.Close()

	payload := pl("instrument", "AAA", "price", 1)
	r, err := f.Append("price", "ev-1", "2026-01-05", payload)
	if err != nil {
		t.Fatal(err)
	}
	payload["price"] = json.Number("999999") // mutate the caller's own map after the call

	if r.Payload["price"].(json.Number).String() != "1" {
		t.Fatalf("Append must not retain the caller's map: got %v", r.Payload["price"])
	}
	if f.Records()[0].Payload["price"].(json.Number).String() != "1" {
		t.Fatalf("Feed's stored record must not reflect a later mutation of the caller's payload map: got %v", f.Records()[0].Payload["price"])
	}

	r.Payload["price"] = json.Number("1002") // mutate the RETURNED record, not the caller's map
	if f.Records()[0].Payload["price"].(json.Number).String() != "1" {
		t.Fatalf("Feed's stored record must not reflect a mutation of the returned Record's Payload: got %v", f.Records()[0].Payload["price"])
	}
}

// --- Fix round 6: honest blank-line-tolerance reasoning (comment-only —
// see feed.go's package doc and the blank-line branch in scan), and
// Append refusing an unidentifiable record. ---

// TestAppendRefusesEmptyType: a durable log that accepts a record it can
// never route to a fold handler isn't doing its one job. This is the
// feed-level half of defense in depth — cmd/meridian also refuses an
// empty --type before ever calling Append.
func TestAppendRefusesEmptyType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	defer f.Close()
	_, err := f.Append("", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	if !errors.Is(err, ErrMissingIdentity) {
		t.Fatalf("want ErrMissingIdentity, got %v", err)
	}
	if f.Len() != 0 {
		t.Fatalf("a refused append must not be recorded, got len=%d", f.Len())
	}
}

// TestAppendRefusesEmptyID: an empty id can never be told apart from any
// other empty id — a durable log that accepts one has recorded an event
// it can't distinguish from every other unidentifiable event.
func TestAppendRefusesEmptyID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	defer f.Close()
	_, err := f.Append("price", "", "2026-01-05", pl("instrument", "AAA", "price", 1))
	if !errors.Is(err, ErrMissingIdentity) {
		t.Fatalf("want ErrMissingIdentity, got %v", err)
	}
	if f.Len() != 0 {
		t.Fatalf("a refused append must not be recorded, got len=%d", f.Len())
	}
}

// TestAppendAllowsEmptyEffective pins the deliberate decision NOT to
// extend the identity check to effective: it's a domain-meaningful field
// (what "effective" means, and what counts as valid beyond "non-empty",
// differs per event type) that the fold/domain layer already validates —
// the same reasoning already applied to not constraining payload's keys.
// This durable-log layer only refuses what it has no other guard for and
// cannot function without (type/id); it stays agnostic about domain-field
// content it doesn't own.
func TestAppendAllowsEmptyEffective(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	defer f.Close()
	r, err := f.Append("price", "ev-1", "", pl("instrument", "AAA", "price", 1))
	if err != nil {
		t.Fatalf("empty effective must not be refused by this package: %v", err)
	}
	if r.Effective != "" {
		t.Fatalf("want empty effective preserved as given, got %q", r.Effective)
	}
}

// TestAppendRefusalForEmptyIdentityDoesNotPoisonHandle: rejecting a bad
// call is caller-input validation, not a write/fsync failure — it must
// not poison the handle (a later, valid Append must still work) and must
// not have consumed a seq number (the refused attempt never got that far).
func TestAppendRefusalForEmptyIdentityDoesNotPoisonHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f := mustOpen(t, path)
	defer f.Close()
	if _, err := f.Append("", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1)); !errors.Is(err, ErrMissingIdentity) {
		t.Fatalf("want ErrMissingIdentity, got %v", err)
	}
	r, err := f.Append("price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	if err != nil {
		t.Fatalf("a rejected empty-identity call must not poison the handle: %v", err)
	}
	if r.Seq != 1 {
		t.Fatalf("want seq 1 — the refused attempt must not have consumed a seq, got %d", r.Seq)
	}
}
