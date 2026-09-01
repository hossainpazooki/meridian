// Package feed is the only durable input: an append-only JSONL file, one
// canonical record per line, each carrying the sha256 of the previous line
// (hash chain), fsync'd on every append.
//
// Open verifies the whole chain: contiguous seq starting at 1, and each
// record's prev equal to the sha256 of the previous line's stripped bytes.
// A line already on disk is never rewritten or truncated — Open only ever
// reads, Append only ever appends.
//
// Any number of NON-BLANK lines may fail to parse as a record; there is no
// cap. Only the LAST such line in the file governs whether the feed opens:
// if it is unterminated (no trailing '\n'), it can only be the file's final
// bytes — a write interrupted mid-record, the classic torn tail — and is
// tolerated. If it is newline-terminated, it is tolerated only when at
// least one valid, chain-consistent record appears after it before EOF
// (the shape left behind when Append recovers from an earlier failed
// write: its leading '\n' retroactively terminates whatever garbage
// preceded it, without making that garbage valid). A terminated
// unparseable line with nothing valid ever following it is refused with a
// ChainError: that shape is corruption of what was, or was meant to be,
// the last committed record, and cannot be produced by an interrupted
// write, only by a write that never resumed. An earlier discipline capped
// the number of tolerated unparseable lines at one; that was wrong — two
// ordinary crashes in a row (each one's recovery write terminating the
// previous one's residue with its own leading '\n', per above) produces
// two such lines before a successful append ever lands, and capping the
// count made that permanently unopenable. Record integrity does not
// depend on a cap: the seq and prev checks below already refuse any
// deleted, altered, or reordered record on their own, so interior junk
// cannot hide a record-level change.
//
// A blank line is counted toward UnparseableLines too (so a file cannot be
// padded with blank lines with nothing to show for it), but does NOT
// participate in the accept/refuse decision above, on EITHER end. This is
// NOT because a blank line can't be evidence of a dropped record — it can:
// replacing a record's line with an empty one is a two-character edit, and
// a blank line at the tail is then indistinguishable from a legitimately-
// terminated crash artifact, so the record it replaced is silently gone —
// the FOURTH honest-limits case below. That case concedes nothing the
// SECOND honest limit (truncation) doesn't already concede, and is the
// LOUDER of the two: truncation leaves UnparseableLines at 0, this leaves
// it at 1 — visibly not pristine even though the specific record is gone
// either way.
//
// A blank line is tolerated anyway, for two reasons that hold on their
// own: it carries no content, so — unlike non-blank garbage — it can
// never CONCEAL anything (there's nothing in it to hide behind); and,
// concretely, before this rule existed, folding blank lines into the very
// same accept/refuse path as non-blank residue meant a single stray
// trailing newline — the kind any editor, `>>`, or "ensure final newline"
// tool adds without asking — permanently bricked an otherwise-pristine
// feed, with no repair path (lines are never rewritten); that was a worse
// trade than the tail-integrity gap accepted above. An earlier version of
// this comment gave a THIRD reason — that refusing a blank line "would
// require telling it apart from a blank line a legitimate crash-loop
// recovery can leave behind" — which was checked against
// TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly and found
// false: that test's file contains no blank lines at all, only non-blank
// terminated and unterminated residue. The mechanism explains why no
// construction under normal (single-writer) operation produces one
// either: a recovery Append prepends '\n' only when needNewline is set,
// which happens only when the file ends mid-line, so that prepended
// newline always terminates non-blank residue, never creates a blank
// line. Removed rather than left standing on nothing; the two reasons
// above carry the rule on their own. UnparseableLines reports the
// combined total (blank + unparseable), so a caller that wants to notice
// accumulating residue — garbled or blank — still can; it just never
// causes a false refusal on its own.
//
// A genuine I/O read error while scanning (not io.EOF) is always a hard
// failure, never treated as a torn tail, even though it can leave the
// current line looking exactly like one (bufio.Reader.ReadBytes returns
// whatever partial data it read alongside a non-nil, non-EOF error, not
// only at true EOF) — silently accepting that shape would open a feed
// that's missing whatever came after the error without any record of it.
//
// Open also checks canonicality, not just parseability: a successfully
// decoded record is re-marshaled through canon.Marshal and must reproduce
// the line's exact stored bytes, or it's refused. This catches reordered or
// duplicated keys and any other non-canonical encoding of a value that IS
// present. It does NOT, by itself, catch every extra key: canon.Marshal
// sorts keys, so an extra key inserted at the exact position its name
// would sort to (e.g. "backdoor" sorts before "effective") round-trips
// byte-for-byte and the re-marshal comparison cannot see it — the
// round-trip cannot detect a key it faithfully preserves. That gap is
// closed by a separate, explicit check: a record's top-level key set must
// be exactly {effective, id, payload, prev, seq, type}, no more; any extra
// key, wherever it sorts, is refused (see errExtraKeys). This key-set
// check does NOT extend into payload — payload's shape is
// event-type-specific and this package is deliberately agnostic about it
// (validating it is the fold/domain layer's job, not the durable-log
// layer's) — so an extra key inserted at its sorting position inside
// payload specifically is not caught by anything in this package; the
// canonicality re-marshal still catches one inserted anywhere else inside
// payload. Neither check catches a pure value substitution (e.g. a price
// field edited in place, keeping the object otherwise canonical): that
// re-marshals identically regardless of key set, and is the prev-hash
// leg's job.
//
// prev is checked in two steps for the same reason canonicality and the
// key-set are split from ordinary residue: length (must be 64 characters)
// stays inside the general record-shape check and is residue-tolerable,
// because a torn write conceivably could leave prev short. Character
// class (must be lowercase hex, matching what SHA256Hex/Genesis ever
// produce) is a separate, unconditional check once the record otherwise
// looks complete — a torn write cannot leave a complete, correctly-typed
// record with an internally wrong-but-still-64-character value; only
// editing can.
//
// Two distinct diagnoses on the seq leg: a seq at or below the last
// accepted one is reported as a duplicate/regression, not a gap — the two
// have different causes (a duplicate can only mean two records were
// independently accepted at the same position, e.g. by two writers or two
// handles; a gap means something in between was lost) and an operator
// reading the message should not have to guess which.
//
// Append: on a Write or Sync error, no record is added to memory, but bytes
// may already be on disk — a short write can land a prefix before failing,
// and a Sync failure can follow a fully-landed Write. Either way the handle
// is poisoned: every further Append on it fails until the caller reopens,
// so a caller that ignores one error and blindly retries can never glue new
// bytes onto an unknown quantity of leftover ones, and can never receive a
// silently-different record at the seq it was told had failed. The next
// successful write — on a reopened handle, since the poisoned one refuses —
// always starts on a fresh line for the same reason.
//
// Honest limits: this is a bare hash chain and it does not catch
// everything. Editing the payload of the LAST record in the file is
// undetectable here — nothing downstream commits to that record's hash.
// Truncating the file to a shorter valid prefix is likewise undetectable —
// a prefix of a valid chain is itself a valid chain. A THIRD limit follows
// directly from the residue rule above and is not fixable within it: take
// a valid feed, corrupt the last record in place (still terminated, still
// on its own line), then append a single byte with no trailing newline.
// The file now reads as {valid record(s), terminated garbage, unterminated
// garbage} — Open accepts it, the corrupted record is gone with no error,
// and UnparseableLines reports 2. This is BYTE-IDENTICAL to the legitimate
// shape two ordinary crashes in a row produce during recovery (see above,
// and TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly), so no rule
// operating on this file alone can refuse one shape without also bricking
// the other — and refusing the crash-loop shape is the exact defect this
// package's residue rule exists to not reintroduce. This is a genuine,
// unclosed gap, not an oversight: it was found, reasoned about, and left
// open deliberately rather than traded for a worse failure mode. A FOURTH
// limit is the same gap reached more directly: replace the last record's
// line outright with a blank one (a two-character edit) instead of
// corrupting it — simpler than the third limit's shape, and it concedes
// nothing the SECOND limit (truncation) doesn't already concede, though
// it is the LOUDER of the two — truncation leaves UnparseableLines at 0,
// this leaves it at 1. It is silently accepted for the two reasons blank
// lines are tolerated at all (see the package's blank-line residue rule
// above: no content to conceal, and refusing would re-brick on an
// ordinary stray trailing newline) — NOT because it can't be told apart
// from crash-loop residue, which was this entry's stated reason until
// that claim was checked and found false (see above) and removed. The
// replaced record is gone with no error either way. None of these four is a
// defect in this package; a bare chain cannot detect tail forgery by
// construction, only interior tampering and gaps. Tail integrity comes
// from outside this package: snapshots are stamped with a feed-prefix
// hash (see PrefixHash), and a separate gate pins and compares against
// that value out of band — that external commitment, not this chain, is
// what actually protects the tail against any of the four.
package feed

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/hossainpazooki/meridian/internal/canon"
)

// Genesis is the prev of record 1.
const Genesis = "0000000000000000000000000000000000000000000000000000000000000000"

// Record is one decoded feed line plus its own line hash.
type Record struct {
	Seq       int64
	Type      string
	ID        string
	Effective string
	Payload   map[string]any
	Prev      string
	LineHash  string
}

// ChainError reports the first seq at which the feed is not a valid chain.
type ChainError struct {
	Seq    int64
	Reason string
}

func (e *ChainError) Error() string {
	return fmt.Sprintf("feed chain broken at seq %d: %s", e.Seq, e.Reason)
}

// ErrPoisoned is returned by Append once an earlier Append on the same
// handle has failed to write or fsync. Reopen the feed to get a handle
// whose state is re-derived from disk rather than trusted from memory.
var ErrPoisoned = errors.New("feed: handle poisoned by a previous write/fsync failure; reopen required")

// ErrMissingIdentity is returned by Append when typ or id is empty. A
// durable log that accepts an unidentifiable record isn't doing its one
// job: an empty id can never be told apart from any other empty id, and
// an empty type can never be routed to a fold handler. This was found via
// a real caller bug — a dropped CLI flag silently wrote
// {"effective":"","id":"","type":""} into an append-only log, permanent
// and surfacing only later, through a different subcommand's refusals.
// The CLI now also refuses this before ever calling Append; this check
// makes that defense in depth rather than the sole guard on one call
// path. effective is deliberately NOT checked here — see the package doc.
var ErrMissingIdentity = errors.New("feed: Append requires a non-empty type and id")

// fileHandle is the subset of *os.File that Feed needs once Open's initial
// scan is done. It exists so a test can substitute a fake that
// deterministically fails a Write or Sync call — real disk-full/EIO
// conditions aren't reliably triggerable in a unit test.
type fileHandle interface {
	io.Writer
	io.Closer
	Sync() error
}

// Feed is a single-writer, mutex-guarded handle on one feed file.
type Feed struct {
	mu           sync.Mutex
	f            fileHandle
	records      []Record
	needNewline  bool
	poisoned     error
	garbageLines int64
}

// Open creates the file if absent, full-scans it, and verifies seq
// contiguity and the prev-hash chain. Any number of unparseable lines are
// tolerated; only the last one in the file — whether it's an unterminated
// torn tail or a terminated line with nothing valid after it — decides
// whether Open succeeds. See the package doc for the exact rule. On
// success, UnparseableLines reports how many were tolerated.
func Open(path string) (*Feed, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	fd := &Feed{f: fh}
	if err := scan(fd, bufio.NewReaderSize(fh, 1<<20)); err != nil {
		fh.Close()
		return nil, err
	}
	return fd, nil
}

// scan performs Open's full verification pass over br, populating
// fd.records, fd.needNewline, and fd.garbageLines. Split out from Open so
// a test can drive it directly against a reader that deterministically
// fails partway through with a real (non-EOF) error — something a real
// *os.File cannot be made to do reliably in a unit test — to prove a
// genuine I/O error is hard-failed rather than silently treated as a torn
// tail. Never closes anything; that's Open's job on the single call site.
func scan(fd *Feed, br *bufio.Reader) error {
	prev := Genesis
	var lastByte byte
	saw := false

	// Bookkeeping for tolerated residue. garbageCount is the TOTAL residue
	// count exposed via UnparseableLines — every blank or unparseable line
	// increments it. But only NON-BLANK unparseable lines feed the
	// accept/refuse decision (lastGarbageTerminated / lastGarbageWant /
	// recordsSinceLastGarbage) — a blank line is handled separately, below,
	// precisely so it never can (see that branch for why). There is no cap
	// on how many are tolerated — only the LAST non-blank one found matters
	// for the accept/refuse decision, since a valid record after it is
	// automatically also after every earlier one (records only ever move
	// forward through the file). recordsSinceLastGarbage resets every time
	// a new non-blank residue line is found.
	var garbageCount int64
	var lastGarbageTerminated bool
	var lastGarbageWant int64
	var recordsSinceLastGarbage int64
	markResidue := func(terminated bool) {
		garbageCount++
		lastGarbageTerminated = terminated
		lastGarbageWant = int64(len(fd.records)) + 1
		recordsSinceLastGarbage = 0
	}

	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			saw = true
			lastByte = line[len(line)-1]
			complete := lastByte == '\n'
			stripped := canon.StripLine(line)
			if len(stripped) == 0 {
				// A blank line is not a record, and it is counted as
				// residue (see package doc) — but it does NOT participate
				// in the accept/refuse decision below, on either end. NOT
				// because a blank line can't be evidence of corruption —
				// it can: blanking a record's line silently drops it, and
				// that's an accepted, documented tail-integrity gap (see
				// the package doc's honest-limits section), not something
				// this branch is claiming to prevent. It's tolerated
				// because a blank line carries no content to CONCEAL
				// anything behind, and because refusing it re-bricks on an
				// ordinary stray trailing newline (see the package doc for
				// both reasons, and for why an earlier, false third reason
				// — that a blank line can't be told apart from crash-loop
				// residue — was removed rather than left standing). Only
				// count it here — never touch
				// lastGarbageTerminated/lastGarbageWant/
				// recordsSinceLastGarbage, which drive that decision.
				garbageCount++
				if !complete {
					if rerr != nil && rerr != io.EOF {
						return rerr
					}
					break
				}
				continue
			}
			v, derr := canon.Decode(stripped)
			if derr != nil {
				// A completed Append always writes syntactically valid
				// canonical JSON, so a line that fails to parse at all can
				// only be residue of an interrupted write. See the package
				// doc for why there is no cap on how many are tolerated,
				// and why only the last one's shape decides the outcome.
				markResidue(complete)
				if !complete {
					// Unterminated does NOT by itself mean "this is a torn
					// tail" — ReadBytes returns a line without its
					// delimiter both at true EOF AND when the underlying
					// reader hit a real error partway through (io.EOF is
					// not the only way this happens; a real error also
					// returns whatever partial data it read alongside a
					// non-nil, non-EOF err). Only true EOF is a genuine
					// torn tail; a real read error must hard-fail instead
					// of being silently treated as tolerable residue —
					// otherwise a real I/O error mid-file opens a
					// silently truncated feed.
					if rerr != nil && rerr != io.EOF {
						return rerr
					}
					break
				}
				continue
			}
			rec, perr := recordFromValue(v)
			if perr != nil {
				var ek *errExtraKeys
				if errors.As(perr, &ek) {
					// Every required key was present and correctly typed;
					// the only thing wrong is an extra one, which a torn
					// write can never produce (truncation only removes
					// bytes, never adds a whole extra key). Unconditional
					// hard failure, not residue — see recordFromValue.
					return &ChainError{Seq: rec.Seq, Reason: perr.Error()}
				}
				// Otherwise: decoded as JSON, but doesn't have the shape
				// of a record (missing/mistyped field) — the signature of
				// a genuinely interrupted write. Same residue treatment
				// as an outright parse failure — see above.
				markResidue(complete)
				if !complete {
					if rerr != nil && rerr != io.EOF {
						return rerr
					}
					break
				}
				continue
			}
			if !isLowerHex64(rec.Prev) {
				// Right length (recordFromValue already checked), wrong
				// character class. A torn write cannot produce a
				// complete, correctly-typed record with an internally
				// inconsistent field value — only editing can — so this
				// is unconditional too, not residue. (Deliberately split
				// from the length check: a wrong length is a more
				// generically "incomplete" shape and stays
				// residue-tolerable; see the package doc.)
				return &ChainError{Seq: rec.Seq, Reason: fmt.Sprintf("prev is not 64 lowercase hex characters: %q", rec.Prev)}
			}
			// Canonicality: re-marshaling the SAME decoded value must
			// reproduce the exact stored bytes. A mismatch means this line
			// has every required field but was never written by an honest
			// Append — see the package doc for what this does and does not
			// catch. This can only be deliberate tampering, never crash
			// residue (a genuine torn write never even parses), so it is
			// an unconditional hard failure, not routed through the
			// residue-tolerance above.
			if canonical, cerr := canon.Marshal(v); cerr != nil || !bytes.Equal(canonical, stripped) {
				// NOT "extra keys": the key-set check above already
				// refused those (it always runs first). Everything that
				// reaches here has exactly the required 6 keys, so a
				// mismatch here is reordering, a duplicated JSON key that
				// decode collapsed away, or some other non-canonical
				// encoding of a value that's present. Keep this message
				// in sync with what actually still reaches this line —
				// see recordFromValue and the package doc.
				reason := "line is not canonical JSON (reordered or duplicated keys, or another non-canonical encoding)"
				if cerr != nil {
					reason = "line failed re-canonicalization: " + cerr.Error()
				}
				return &ChainError{Seq: rec.Seq, Reason: reason}
			}
			want := int64(len(fd.records)) + 1
			last := want - 1
			if rec.Seq <= last {
				// A seq at or below one already accepted is a duplicate or
				// a regression, not a gap — distinct causes, distinct
				// message. (Before poisoning existed, a same-handle retry
				// after a Sync failure could produce exactly this; see
				// Append's package doc.)
				return &ChainError{Seq: rec.Seq, Reason: fmt.Sprintf("seq %d is a duplicate of or a regression from an already-accepted seq (have up to %d) — not a gap", rec.Seq, last)}
			}
			if rec.Seq != want {
				return &ChainError{Seq: rec.Seq, Reason: fmt.Sprintf("seq %d where %d expected (gap)", rec.Seq, want)}
			}
			if rec.Prev != prev {
				return &ChainError{Seq: rec.Seq, Reason: "prev hash does not match previous line"}
			}
			rec.LineHash = canon.SHA256Hex(stripped)
			prev = rec.LineHash
			fd.records = append(fd.records, rec)
			if garbageCount > 0 {
				recordsSinceLastGarbage++
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
	}

	if garbageCount > 0 && lastGarbageTerminated && recordsSinceLastGarbage == 0 {
		// lastGarbageTerminated can only have been set by a NON-BLANK
		// unparseable line (blank lines never touch it — see that branch
		// above), so reaching here means: the last non-blank residue line
		// in the file is newline-terminated and no valid record ever
		// followed it. Not a torn tail (a torn tail is either
		// unterminated, or terminated only because a later valid record's
		// recovery write closed it).
		// This is corruption of what was, or was meant to be, the last
		// committed record.
		return &ChainError{Seq: lastGarbageWant, Reason: "residue line with no valid record after it — not a torn tail"}
	}

	fd.needNewline = saw && lastByte != '\n'
	fd.garbageLines = garbageCount
	return nil
}

// recordFromValue extracts a Record from an already-decoded canon value
// (map[string]any / []any / json.Number / string / bool / nil). Split out
// from decoding so a caller that already has the decoded value — Open's
// canonicality check re-marshals that same value — never decodes the same
// bytes twice.
func recordFromValue(v any) (Record, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return Record{}, fmt.Errorf("record is not an object")
	}
	var r Record
	seq, ok := m["seq"].(json.Number)
	if !ok {
		return Record{}, fmt.Errorf("seq missing")
	}
	var err error
	if r.Seq, err = seq.Int64(); err != nil {
		return Record{}, err
	}
	if r.Type, ok = m["type"].(string); !ok {
		return Record{}, fmt.Errorf("type missing")
	}
	if r.ID, ok = m["id"].(string); !ok {
		return Record{}, fmt.Errorf("id missing")
	}
	if r.Effective, ok = m["effective"].(string); !ok {
		return Record{}, fmt.Errorf("effective missing")
	}
	if r.Prev, ok = m["prev"].(string); !ok || len(r.Prev) != 64 {
		// Character class (hex vs. not) is deliberately NOT checked here
		// — see Open's explicit, unconditional check right after
		// recordFromValue succeeds, and the package doc for why that
		// split exists.
		return Record{}, fmt.Errorf("prev missing or malformed")
	}
	if r.Payload, ok = m["payload"].(map[string]any); !ok {
		return Record{}, fmt.Errorf("payload missing")
	}
	if len(m) != knownRecordKeys {
		// Every required key is present and correctly typed at this
		// point, so a key count above knownRecordKeys means at least one
		// EXTRA key survived decode. A torn write can only ever produce a
		// record with keys missing (caught by the checks above) —
		// truncation cannot add bytes — so an extra key can only come
		// from deliberate editing. Distinct error type so Open can refuse
		// this unconditionally instead of folding it into ordinary
		// torn-tail residue tolerance.
		return r, &errExtraKeys{m: m}
	}
	return r, nil
}

// knownRecordKeys is the exact top-level key count of a record Append ever
// writes: effective, id, payload, prev, seq, type. No more, no fewer.
const knownRecordKeys = 6

// errExtraKeys signals a record that decoded with every required key
// present and correctly typed, but also at least one key beyond the known
// set. See recordFromValue and the package doc.
type errExtraKeys struct{ m map[string]any }

func (e *errExtraKeys) Error() string {
	known := map[string]bool{"effective": true, "id": true, "payload": true, "prev": true, "seq": true, "type": true}
	var extra []string
	for k := range e.m {
		if !known[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	return fmt.Sprintf("record has extra key(s) beyond {effective,id,payload,prev,seq,type}: %v", extra)
}

// isLowerHex64 reports whether s is exactly 64 lowercase hex characters —
// the shape of every hash this package ever produces (SHA256Hex and
// Genesis are both lowercase). A length-64 string with the wrong character
// class (e.g. "zz...") would otherwise pass a bare length check and only
// be caught later, incidentally, by a hash comparison that happens not to
// match.
func isLowerHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// deepCopyValue recursively copies a value out of the canon/JSON value
// space (map[string]any, []any, json.Number, string, bool, nil). Scalars
// are immutable in Go and returned as-is; only maps and slices need copying.
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = deepCopyValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = deepCopyValue(vv)
		}
		return out
	default:
		return val
	}
}

func deepCopyPayload(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

// Append writes one canonical record with the next seq and the chain prev,
// then fsyncs before returning. typ and id must be non-empty or Append
// refuses with ErrMissingIdentity before doing any work — see that var's
// doc for why, and for why effective is deliberately exempt: effective is
// a domain-meaningful field (what "effective" even means, and what values
// beyond "non-empty" are valid, differs per event type) that the
// fold/domain layer already validates and refuses when malformed, exactly
// like this package's deliberate choice not to constrain payload's keys —
// this durable-log layer only insists on the structural completeness
// (non-empty type/id) it has no other guard for and cannot function
// without; it stays agnostic about domain-field content it doesn't own.
// The caller's payload map is deep-copied on the way in, so a later
// mutation of it by the caller never reaches the Feed's own state or what
// was already written to disk. The RETURNED Record's Payload is a
// separate deep copy again, distinct from the one stored in the Feed —
// otherwise a caller mutating the returned record would silently mutate
// the Feed's own in-memory state too (they'd share the same underlying
// map), diverging from what's actually on disk with no error to show for
// it. On any write/fsync error nothing is recorded in memory, and the
// handle is poisoned — see the package doc.
func (fd *Feed) Append(typ, id, effective string, payload map[string]any) (Record, error) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	if fd.poisoned != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrPoisoned, fd.poisoned)
	}
	if typ == "" || id == "" {
		return Record{}, fmt.Errorf("%w: got type=%q id=%q", ErrMissingIdentity, typ, id)
	}
	payload = deepCopyPayload(payload)
	prev := Genesis
	if n := len(fd.records); n > 0 {
		prev = fd.records[n-1].LineHash
	}
	rec := Record{Seq: int64(len(fd.records)) + 1, Type: typ, ID: id, Effective: effective, Payload: payload, Prev: prev}
	line, err := canon.Marshal(map[string]any{
		"effective": effective, "id": id, "payload": payload, "prev": prev, "seq": rec.Seq, "type": typ,
	})
	if err != nil {
		return Record{}, err
	}
	var buf bytes.Buffer
	if fd.needNewline {
		buf.WriteByte('\n')
	}
	buf.Write(line)
	buf.WriteByte('\n')
	if _, err := fd.f.Write(buf.Bytes()); err != nil {
		// A short write can land a prefix of these bytes before failing;
		// we cannot know how much. Never let the next write glue onto an
		// unknown quantity of leftover bytes, and never trust this
		// handle's in-memory state again until a reopen re-derives it
		// from disk.
		fd.needNewline = true
		fd.poisoned = err
		return Record{}, err
	}
	if err := fd.f.Sync(); err != nil {
		// The Write above fully succeeded, so unlike the branch above
		// these bytes ARE the complete, valid record — a reopen will
		// find and accept it. What failed is durability, not the write
		// itself. Still poison and force a fresh line: this handle no
		// longer knows whether it can trust its own writes, and the only
		// bytes safe to trust are the ones a reopen re-verifies from disk.
		fd.needNewline = true
		fd.poisoned = err
		return Record{}, err
	}
	rec.LineHash = canon.SHA256Hex(line)
	fd.needNewline = false
	fd.records = append(fd.records, rec)
	returned := rec
	returned.Payload = deepCopyPayload(rec.Payload)
	return returned, nil
}

// Records returns a deep copy of all verified records in seq order.
// Mutating a returned Record's Payload — or a payload map the caller passed
// to Append — never affects the Feed's own state.
func (fd *Feed) Records() []Record {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	out := make([]Record, len(fd.records))
	for i, r := range fd.records {
		r.Payload = deepCopyPayload(r.Payload)
		out[i] = r
	}
	return out
}

// Len is the number of verified records (= the last seq).
func (fd *Feed) Len() int64 {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return int64(len(fd.records))
}

// UnparseableLines is the number of lines Open tolerated as residue (see
// the package doc) when this handle was opened. A caller that wants to
// notice accumulating garbage — e.g. to alert or compact — can poll this.
func (fd *Feed) UnparseableLines() int64 {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return fd.garbageLines
}

// PrefixHash returns "sha256:<hex>" of record seq — the chain head commits to
// the whole prefix [1..seq]. seq 0 is the empty prefix (Genesis).
func (fd *Feed) PrefixHash(seq int64) (string, error) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	if seq == 0 {
		return "sha256:" + Genesis, nil
	}
	if seq < 0 || seq > int64(len(fd.records)) {
		return "", fmt.Errorf("seq %d out of range [0,%d]", seq, len(fd.records))
	}
	return "sha256:" + fd.records[seq-1].LineHash, nil
}

// Close closes the file.
func (fd *Feed) Close() error {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return fd.f.Close()
}
