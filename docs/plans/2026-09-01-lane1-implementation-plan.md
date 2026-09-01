# MERIDIAN Lane 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build MERIDIAN Lane 1 — the local Go event-sourced ledger — so all six properties (P1–P6) have a GREEN live cell and a RED twin cell with exact planted counts, emitted as BASELINE-schema `GATE_VERDICT` rows by one runner.

**Architecture:** One append-only hash-chained JSONL feed is the only durable input; every read is a pure recompute (fold) over the visible prefix `[1..V]`; snapshots are canonical-JSON, sha256-addressed artifacts stamped with the feed-prefix hash. A stdlib-only Python generator plants the ground truth (fills, actions, one amendment, every twin defect) and embeds an import-pinned naive fold that emits custodian statements and known-bad twin artifacts. Go gate tests check live artifacts (must pass) and twin artifacts (must fail with the planted counts) and write verdict rows.

**Tech Stack:** Go 1.26 (stdlib only, module `github.com/hossainpazooki/meridian`), Python 3.12+ stdlib only (`fixtures/generate.py`, `gates/*.py`), POSIX `sh` for the runner, GitHub Actions.

**Spec:** `docs/2026-08-31-design.md` (locked). Where this plan and the spec disagree, the spec wins — stop and flag.

## Global Constraints

Copied from the spec; every task's requirements include these.

- **No floats anywhere.** All amounts are integer minor units. Go: `int64` for state, `math/big` for the one division. Python: `int`.
- **No wallclock in the fold.** Feed sequence number is the only clock. `time.Now()` may appear only in verdict `ran_at` stamping.
- **One division point:** cost relieved on a sell = `round_half_even(total_cost * qty_sold / total_qty)`. Nowhere else divides.
- **The feed is the only input;** nothing derived is ever appended to it.
- **Pure recompute reads:** no materialized derived state on disk or in a long-lived process.
- **Event set is closed:** `fill`, `price`, `action` (split | dividend), `action_amendment`.
- **Naive fold is import-pinned:** `fixtures/generate.py` imports only Python stdlib and references no path under `internal/` or `cmd/`.
- **No performance vocabulary, no benchmarks** in code comments, docs, commit messages, or test names ("fast", "latency", "throughput", "benchmark" are banned).
- **STATUS.md is the state of record;** README never carries counts. A cell becomes CLAIMABLE only when live is GREEN *and* twin is RED with `checks == planted.expected_violations` exactly.
- **All fixture string fields are ASCII** matching `[A-Za-z0-9._:-]` so Go and Python canonical JSON are byte-identical.
- **Line endings:** `.gitattributes` forces LF; every reader strips a trailing `\r` before hashing so hashes are newline-insensitive (BASELINE lesson, `docs/learnings/2026-09-01-baseline-hashes-lf-normalized.md`).
- **Console output ASCII-only** (Windows cp1252 console).
- **Never write git history.** Each task ends with the commit command for the operator to run; keep working. (The commit commands below are shown as `git commit` lines for the operator — the executing agent prints them, it does not run them.)
- Twin gates live in the same test files as live gates; a twin that has never run RED proves nothing and the runner treats a twin that is not RED-with-planted-counts as a failed run.

## Decisions this plan makes beyond the spec (flag if you disagree; do not silently change)

1. **Verdict row keys are BASELINE's verbatim**, including `parallax_sha` / `parallax_worktree`, populated with MERIDIAN's own HEAD sha and worktree state. Renaming the key (to e.g. `emitter_sha`) is a BASELINE-side decision to take when the registration seam is designed; a copy-not-translation promise means we do not invent names here. Twin rows carry the 17th key `planted` exactly as BASELINE's twin row does (`{"mutation","mutated_rows","expected_violations"}`).
2. **Twin cells are known-bad *artifacts* or known-bad *feeds*, per property:** P1/P3/P4 twins are snapshots the naive fold emits in a deliberately broken mode (no-dedupe, leak, silent-zero/stale); P5's twin is a drifted statement; P2's and P6's twins are re-chained mutated feeds (plus one raw-tampered feed for the chain check). A gate's job is to reject a bad artifact, so the twin exercises the gate, not the ledger's own fold.
3. **Single portfolio, single cash balance.** No account field. Negative cash is allowed (no margin logic). Selling more than held is a fail-closed `oversell` refusal.
4. **Dividends go to cash and to `dividend_income`, not `realized_pnl`.** Realized P&L is sells only.
5. **Valuation price** = the `price` event for that instrument with the greatest `(effective, seq)` in the prefix. None in prefix ⇒ `unevaluable` record, `valuation: null`.
6. **Amendment semantics:** an `action_amendment` replaces the named action's terms (`ratio` or `rate`); the action still applies at its original `effective` date. At viewpoints before the amendment's seq the original terms apply. Amendment naming an unknown `action_id` ⇒ refusal `unknown_action`.
7. **Application order** within a visible prefix is `(effective, seq)` after dedupe and term resolution. Amendments are not applied as events; they only rewrite terms.
8. **Positions with `qty == 0` are removed** from the snapshot (relief of the final share is exact, so `total_cost` is 0 there by construction — asserted).
9. **`announced` / `processed` dates on an action are carried, not used.** Knowledge time is seq; the fields exist so the fixture reads like a real action record.
10. **Expected-view files use the snapshot schema key `feed_seq`, not `seq`.**
    Corrected 2026-09-01 during execution: the original text specified `seq`,
    which made `leaf_diff`/`snapshot.Diff` charge one phantom leaf on EVERY
    comparison (the key is absent from every snapshot document). That inflated
    the published twin footprints by one, made the generator's hollow-twin
    guards unreachable (`leaf_diff` had a hard floor of 1, so `if k == 0: die`
    could never fire), and would have made P1's and P3's live cells
    permanently RED. Measured: live diff 1 -> 0, k1 8 -> 7, k3 4 -> 3.
11. **`Diff` is one-sided by design, so every gate must ALSO assert key-set
    equality.** Added 2026-09-01 during execution. `snapshot.Diff` walks the
    golden's keys only, which is required — a twin document legitimately
    carries keys (`absorbed`, `refusals`, `unevaluable`, `feed_prefix_hash`,
    `unrealized_pnl`) that a reduced golden never has. The consequence,
    measured: a ledger that INVENTS a position, or that fails to mark an
    instrument `unevaluable`, scores 0 mismatches and passes every gate.
    Diff must NOT be made two-sided. Instead `fixtures/base/manifest.json`
    publishes `positions_at.{V1,V2,V3}` and `unevaluable_at.V3` (sorted
    instrument lists, computed and self-asserted by the generator), and each
    gate adds a check comparing the document's actual key sets against them.
    A gate that only diffs golden leaves is not complete.
12. **`snapshot.Build` returns an error; it must not panic.** Corrected
    2026-09-01 during execution. The plan had `Build` panic on a marshal
    error, justified as "doc is built from primitives only" — true until
    decision 10 hardened `canon.Marshal` to refuse non-ASCII strings. After
    that, a feed carrying a non-ASCII instrument name or event id reaches
    `Build` through `feed.Open` -> `canon.Decode` (which does not validate
    string content) -> `fold` (no ASCII check) and panics. Measured:
    `canon: non-ASCII rune 'É' in string "AAÉ" at .positions.AAÉ (key)`.
    A crash on hostile or corrupt data contradicts the fold's own "malformed
    becomes a refusal, never an error" invariant. Defended at three layers:
    `feed.Open` enforces canonicality at ingress, `fold` refuses
    uncanonicalizable payloads, and `Build` returns an error.
13. Plan lives at `docs/plans/` (committed) rather than the gitignored `docs/superpowers/plans/` because the executor may run from the web where only the remote exists.

---

## Shared contract (all tasks implement exactly this)

### Feed record (one JSONL line; canonical JSON = sorted keys, no whitespace, ASCII)

```json
{"effective":"2026-01-05","id":"ev-000001","payload":{...},"prev":"<64 hex>","seq":1,"type":"fill"}
```

- `seq`: 1-based, contiguous. `id`: unique ASCII string. `effective`: `YYYY-MM-DD`.
- `prev`: hex sha256 of the previous record's line bytes (line without its trailing `\n`, and with any trailing `\r` stripped). Genesis `prev` = 64 zeros.
- **Line hash** of record N = sha256 of its own line bytes (same stripping). **Feed prefix hash at V** = line hash of record V, written as `sha256:<hex>`.
- Payloads:
  - `fill`: `{"instrument":"AAA","price":1000,"qty":100,"side":"buy","trade_id":"T-000001","venue":"X"}` — `side` ∈ `buy|sell`; `price` is minor units per share.
  - `price`: `{"instrument":"AAA","price":800}`
  - `action`: `{"action_id":"CA-0001","announced":"2026-01-08","instrument":"AAA","kind":"split","processed":"2026-01-10","ratio":2}` or `{"action_id":"CA-0002","announced":"...","instrument":"BBB","kind":"dividend","processed":"...","rate":25}` — `ratio` = new shares per old share (whole), `rate` = minor units per share.
  - `action_amendment`: `{"action_id":"CA-0001","ratio":3}` or `{"action_id":"CA-0002","rate":30}`.

### Idempotency (P1)

- Key = sha256 hex of canonical JSON `{"trade_id":...,"venue":...}`; payload hash = sha256 hex of the canonical fill payload.
- First occurrence of a key wins. Later same key + same payload hash ⇒ **absorbed** (recorded, zero effect). Same key + different payload hash ⇒ **refusal** `collision` (original stands).

### Fold algorithm (Go `internal/fold` and Python naive fold must agree)

Given records `[1..V]`:
1. `terms[action_id] = action payload` for each `action`; for each `action_amendment` in seq order, overwrite `ratio`/`rate` in `terms[action_id]` (unknown id ⇒ refusal `unknown_action`, skip).
2. Dedupe fills in seq order by key (above) ⇒ kept fills, absorbed list, collision refusals.
3. `applicable` = kept fills + actions (resolved terms) + prices, stably sorted by `(effective, seq)`.
4. Apply in that order:
   - buy: `qty += q; total_cost += q*price; cash -= q*price`
   - sell: if `q > qty` ⇒ refusal `oversell`, skip. Else `relieved = rhe(total_cost*q, qty)`; `total_cost -= relieved; qty -= q; cash += q*price; realized_pnl += q*price - relieved`; if `qty == 0` delete position (assert `total_cost == 0`).
   - split: if position exists, `qty *= ratio` (cost unchanged). Else no-op.
   - dividend: if position exists and `qty > 0`: `d = qty*rate; cash += d; dividend_income += d`.
   - price: `last_price[instrument] = (price, seq)`.
5. Valuation: for each position, if `last_price` has it: `unrealized = qty*price - total_cost`, else `unevaluable {"instrument","reason":"no_price_in_prefix"}`. `unrealized_pnl` = sum over valued positions.

`rhe(n, d)` for `n >= 0, d > 0`: `quo, rem = divmod(n, d)`; `2*rem > d ⇒ quo+1`; `2*rem < d ⇒ quo`; tie ⇒ `quo` if `quo` even else `quo+1`.

### Snapshot document (canonical JSON + one trailing `\n`; file name `<sha256hex>.json`; `content_hash` = `sha256:<hex>` over those bytes)

```json
{"absorbed":[{"event_id":"ev-000031","key":"<hex>","seq":31}],
 "cash":-77354,
 "dividend_income":4500,
 "feed_prefix_hash":"sha256:<hex>",
 "feed_seq":58,
 "positions":{"AAA":{"qty":180,"total_cost":99030,"valuation":{"price":800,"price_seq":57,"unrealized":44970}},
              "CCC":{"qty":3,"total_cost":1500,"valuation":null}},
 "realized_pnl":18178,
 "refusals":[{"detail":"payload hash mismatch","event_id":"ev-000040","key":"<hex>","kind":"collision","seq":40}],
 "unevaluable":[{"instrument":"CCC","reason":"no_price_in_prefix"}],
 "unrealized_pnl":45368}
```
Lists are sorted by `seq` (absorbed, refusals) or by instrument (unevaluable). Empty lists encode as `[]`, never omitted. (Shown pretty-printed; on disk it is one line.)

### Fixture artifacts written by `fixtures/generate.py`

```
fixtures/
  generate.py
  base/feed.jsonl            seeded portfolio; contains planted dup + collision; CCC price withheld
  base/manifest.json         seeds, seqs, viewpoints, planted counts (schema in Task 9)
  base/expected/V1.json      naive-fold state at viewpoint V1  {"cash","dividend_income","positions":{I:{"qty","total_cost"}},"realized_pnl","feed_seq"}
  base/expected/V2.json, V3.json
  base/statement.json        custodian format {"as_of_seq":N,"cash":int,"holdings":[{"cost_basis":int,"instrument":"AAA","quantity":int}]}
  base/snapshot.sha256       PINNED by Go once (Task 11), not by the generator
  p1/twin/snapshot.json      naive fold, no-dedupe mode, at end seq (snapshot schema)
  p2/mutated/feed.jsonl      one fill price +1, re-chained
  p2/reordered/feed.jsonl    split action swapped with the AAA buy immediately before it, re-chained
  p2/tampered/feed.jsonl     base bytes with one payload digit changed, NOT re-chained
  p3/twin/V2.json            naive fold at V2 with amended terms leaked (snapshot schema)
  p4/twin/snapshot.json      naive fold at end: CCC valued at 0 silently, DDD price_seq points at BBB's price event
  p5/twin/statement.json     statement with cost_basis drift on one instrument
  p6/feed.jsonl              hand-scripted portfolio (Task 9 lists every event)
  p6/golden.json             HAND-COMPUTED, literal constants
  p6/twin-fill/feed.jsonl    AAA second buy qty 50 -> 51, re-chained
  p6/twin-price/feed.jsonl   AAA price 800 -> 801, re-chained
```

### Verdict row (BASELINE `GATE_VERDICT`; written by `gates/verdict.go` to `$MERIDIAN_VERDICT_DIR`, default `gates/out/`)

Keys, live cell (16): `kind, surface, lane, cell, result, checks, evaluated, rows, scope, params, parallax_sha, parallax_worktree, content_hash, content_hash_basis, ran_at, runner`. Twin cell adds `planted: {"mutation": <string>, "mutated_rows": <int>, "expected_violations": {check: int}}`. `surface` = `meridian-lane1-p<N>`; `lane` = 1; `cell` ∈ `live|twin`; `result` ∈ `GREEN|RED`; `checks` = violations per check; `evaluated` = rows examined per check; `content_hash` = the gated snapshot's hash (or the feed prefix hash when the gated artifact is a feed); `runner` = `$MERIDIAN_RUNNER` or `local`; `ran_at` = UTC RFC3339 with microseconds. File name: `meridian-lane1-p<N>-<cell>-<ran_at compact>.json`.

### File structure and ownership

| Path | Responsibility | Task |
|---|---|---|
| `go.mod`, `.gitattributes`, `.gitignore` | module, LF, ignore `gates/out/`, `bin/` | 1 |
| `internal/canon/canon.go` | canonical JSON bytes, sha256 helpers, `json.Number`-preserving decode | 1 |
| `internal/feed/feed.go` | Record, Open (chain verify), Append (fsync), Records, PrefixHash | 2 |
| `internal/fold/money.go` | `RelieveCost` (round-half-even, big.Int) | 3 |
| `internal/fold/fold.go` | event decode, dedupe, terms, apply, valuation → `State` | 4 |
| `internal/snapshot/snapshot.go` | `Build(state, prefixHash, seq)` → doc + bytes + hash; `Decode`; `Write` | 5 |
| `internal/snapshot/diff.go` | leaf-wise diff of a golden subset against a doc | 5 |
| `internal/asof/asof.go` | `Read(feedPath, V)` = open + fold + build | 6 |
| `internal/reconcile/reconcile.go` | statement load, field-level compare, mismatches with delta | 7 |
| `cmd/meridian/main.go` | CLI: append, replay, snapshot, asof, reconcile | 8 |
| `fixtures/generate.py` | generator + naive fold + all twins + manifest | 9 |
| `gates/verdict.go`, `gates/manifest.go` | verdict emission, manifest loader, git stamping | 10 |
| `gates/p1_test.go` … `gates/p6_test.go` | one property per file; live + twin | 10–15 |
| `fixtures/base/snapshot.sha256` | pinned Go snapshot hash | 11 |
| `gates/importpin.py` | import-pin check + negative control | 16 |
| `gates/run.sh`, `gates/claimability.py`, `.github/workflows/gates.yml` | runner, table, STATUS cross-check, CI | 17 |
| `STATUS.md`, `README.md`, `docs/handoff/` | state of record update, handoff | 18 |

### Dependency lanes (for the orchestrator)

```
T1 -> T2 -> T4 -> T5 -> T6 -> T8
      T3 -> T4        T5 -> T7 -> T8
T9 (Python) is independent of T1-T8; run it in parallel with T2-T7.
T10 needs T6+T7+T9. T11..T15 need T10 and own disjoint files -> may run in parallel.
T16 needs T9. T17 needs T11..T16. T18 last.
```
Parallel-safe groups: {T3, T9} alongside T2; {T11, T12, T13, T14, T15} after T10; T16 any time after T9.

---

### Task 1: Module bootstrap + canonical JSON

**Files:**
- Create: `go.mod`, `.gitattributes`, `.gitignore`
- Create: `internal/canon/canon.go`
- Test: `internal/canon/canon_test.go`

**Interfaces:**
- Produces: `canon.Marshal(v any) ([]byte, error)` — canonical JSON, sorted keys, no whitespace, no HTML escaping, no trailing newline. `canon.Decode(b []byte) (any, error)` — decodes with `UseNumber` so integers survive round-trips. `canon.SHA256Hex(b []byte) string`. `canon.StripLine(b []byte) []byte` — drops one trailing `\n` and then one trailing `\r`.

- [ ] **Step 1: Bootstrap files**

`go.mod`:
```
module github.com/hossainpazooki/meridian

go 1.26
```
`.gitattributes`:
```
* text=auto eol=lf
```
`.gitignore`:
```
/gates/out/
/bin/
/fixtures/.regen/
```

- [ ] **Step 2: Write the failing test**

`internal/canon/canon_test.go`:
```go
package canon

import (
	"encoding/json"
	"testing"
)

func TestMarshalSortsKeysAndOmitsWhitespace(t *testing.T) {
	in := map[string]any{"zeta": 1, "alpha": map[string]any{"y": "a<b>&", "x": []any{1, "s", nil}}}
	got, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"alpha":{"x":[1,"s",null],"y":"a<b>&"},"zeta":1}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestDecodePreservesIntegers(t *testing.T) {
	v, err := Decode([]byte(`{"n":123456789012345678,"p":{"q":-5}}`))
	if err != nil {
		t.Fatal(err)
	}
	n := v.(map[string]any)["n"].(json.Number)
	if n.String() != "123456789012345678" {
		t.Fatalf("integer not preserved: %s", n)
	}
	back, _ := Marshal(v)
	if string(back) != `{"n":123456789012345678,"p":{"q":-5}}` {
		t.Fatalf("round trip changed bytes: %s", back)
	}
}

func TestStripLine(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"abc\n", "abc"}, {"abc\r\n", "abc"}, {"abc", "abc"}, {"abc\r", "abc"},
	} {
		if got := string(StripLine([]byte(c.in))); got != c.want {
			t.Fatalf("StripLine(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestSHA256Hex(t *testing.T) {
	if got := SHA256Hex([]byte("")); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal(got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/canon/ -run . -v`
Expected: FAIL — `undefined: Marshal` (compile error).

- [ ] **Step 4: Implement**

`internal/canon/canon.go`:
```go
// Package canon is the single canonical-bytes authority: sorted-key,
// whitespace-free JSON with no HTML escaping, integer-preserving decode,
// sha256 hex, and newline-insensitive line stripping. Both the feed and the
// snapshot derive their hashes from these bytes; nothing else may marshal.
package canon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Marshal returns canonical JSON: encoding/json sorts map keys bytewise;
// SetEscapeHTML(false) keeps '<', '>', '&' literal. The trailing newline the
// Encoder adds is removed. Callers must pass maps/slices/json.Number/string/
// bool/nil/int64 only — struct field order is NOT canonical and structs are
// rejected by convention (not enforced).
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Decode parses JSON into map[string]any / []any / json.Number / string /
// bool / nil. Numbers stay json.Number so integers never pass through float64.
func Decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// SHA256Hex returns the lowercase hex sha256 of b.
func SHA256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// StripLine removes one trailing '\n' and then one trailing '\r'. Every
// hash over a feed line goes through this so CRLF checkouts hash identically.
func StripLine(b []byte) []byte {
	b = bytes.TrimSuffix(b, []byte("\n"))
	return bytes.TrimSuffix(b, []byte("\r"))
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go vet ./... && go test ./internal/canon/ -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Output commit for the operator**

```bash
cd ~/dev/meridian
git add go.mod .gitattributes .gitignore internal/canon
git commit -m "feat: module bootstrap + canonical JSON authority"
```

---

### Task 2: Append-only hash-chained feed

**Files:**
- Create: `internal/feed/feed.go`
- Test: `internal/feed/feed_test.go`

**Interfaces:**
- Consumes: `canon.Marshal`, `canon.Decode`, `canon.SHA256Hex`, `canon.StripLine`.
- Produces:
  ```go
  type Record struct {
      Seq       int64
      Type      string
      ID        string
      Effective string
      Payload   map[string]any // json.Number leaves
      Prev      string         // 64 hex
      LineHash  string         // 64 hex, sha256 of this record's stripped line bytes
  }
  type Feed struct{ /* unexported */ }
  func Open(path string) (*Feed, error)          // creates if absent; verifies chain + seq contiguity; error names the seq
  func (f *Feed) Append(typ, id, effective string, payload map[string]any) (Record, error) // canonical line + fsync
  func (f *Feed) Records() []Record              // copy, seq order
  func (f *Feed) Len() int64
  func (f *Feed) PrefixHash(seq int64) (string, error) // "sha256:<hex>" of record seq; seq 0 => "sha256:"+64 zeros
  func (f *Feed) Close() error
  const Genesis = "0000000000000000000000000000000000000000000000000000000000000000"
  type ChainError struct{ Seq int64; Reason string } // implements error
  ```

- [ ] **Step 1: Write the failing test**

`internal/feed/feed_test.go`:
```go
package feed

import (
	"encoding/json"
	"errors"
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
	r2, _ := f.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 1100))
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
	r3, _ := g.Append("price", "ev-3", "2026-01-07", pl("instrument", "AAA", "price", 1200))
	if r3.Seq != 3 || r3.Prev != r2.LineHash {
		t.Fatalf("append after reopen broke chain: %+v", r3)
	}
}

func TestCRLFCheckoutHashesIdentically(t *testing.T) {
	dir := t.TempDir()
	lf := filepath.Join(dir, "lf.jsonl")
	f, _ := Open(lf)
	f.Append("price", "ev-1", "2026-01-06", pl("instrument", "AAA", "price", 1100))
	f.Append("price", "ev-2", "2026-01-07", pl("instrument", "AAA", "price", 1200))
	f.Close()
	raw, _ := os.ReadFile(lf)
	crlf := filepath.Join(dir, "crlf.jsonl")
	os.WriteFile(crlf, []byte(strings.ReplaceAll(string(raw), "\n", "\r\n")), 0o644)
	g, err := Open(crlf)
	if err != nil {
		t.Fatalf("CRLF feed must verify: %v", err)
	}
	a, _ := g.PrefixHash(2)
	h, _ := Open(lf)
	b, _ := h.PrefixHash(2)
	if a != b {
		t.Fatalf("CRLF changed the prefix hash: %s vs %s", a, b)
	}
}

func TestTamperedRecordIsRefusedAtNextSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f, _ := Open(path)
	for i := 1; i <= 3; i++ {
		f.Append("price", "ev-"+fmtInt(i), "2026-01-0"+fmtInt(i+4), pl("instrument", "AAA", "price", 1000+i))
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
	f, _ := Open(path)
	f.Append("price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	f.Close()
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	os.WriteFile(path, []byte(lines[1]+"\n"), 0o644) // drop seq 1
	_, err := Open(path)
	var ce *ChainError
	if !errors.As(err, &ce) || ce.Seq != 2 {
		t.Fatalf("want ChainError at seq 2, got %v", err)
	}
}

func TestTornTailToleratedAndNextAppendStartsFreshLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.jsonl")
	f, _ := Open(path)
	f.Append("price", "ev-1", "2026-01-05", pl("instrument", "AAA", "price", 1))
	f.Close()
	fh, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fh.WriteString(`{"effective":"2026-01-06","id":"ev-2","pay`) // torn, no newline
	fh.Close()
	g, err := Open(path)
	if err != nil || g.Len() != 1 {
		t.Fatalf("torn tail must be ignored: err=%v len=%d", err, g.Len())
	}
	r, _ := g.Append("price", "ev-2", "2026-01-06", pl("instrument", "AAA", "price", 2))
	if r.Seq != 2 {
		t.Fatal(r)
	}
	g.Close()
	h, err := Open(path)
	if err != nil || h.Len() != 2 {
		t.Fatalf("append after torn tail must recover cleanly: err=%v len=%d", err, h.Len())
	}
}
```
(The `pl`/`itoa` helpers above are deliberately simple: replace `itoa` with `fmtInt` usage only — delete `itoa` if the compiler complains; `pl` must turn Go `int` into `json.Number`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/feed/ -v`
Expected: FAIL — `undefined: Open`.

- [ ] **Step 3: Implement**

`internal/feed/feed.go`:
```go
// Package feed is the only durable input: an append-only JSONL file, one
// canonical record per line, each carrying the sha256 of the previous line
// (hash chain), fsync'd on every append. Open verifies the whole chain and
// seq contiguity; a torn trailing line (no '\n') is tolerated and never
// rewritten. Seq is the only clock the ledger knows.
package feed

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func (e *ChainError) Error() string { return fmt.Sprintf("feed chain broken at seq %d: %s", e.Seq, e.Reason) }

// Feed is a single-writer, mutex-guarded handle on one feed file.
type Feed struct {
	mu          sync.Mutex
	f           *os.File
	records     []Record
	needNewline bool
}

// Open creates the file if absent, full-scans it, verifies prev-hash chain and
// contiguous seq starting at 1, and tolerates one torn trailing line.
func Open(path string) (*Feed, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	fd := &Feed{f: fh}
	br := bufio.NewReaderSize(fh, 1<<20)
	prev := Genesis
	var lastByte byte
	saw := false
	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			saw = true
			lastByte = line[len(line)-1]
			complete := lastByte == '\n'
			stripped := canon.StripLine(line)
			if len(stripped) == 0 {
				if !complete {
					break
				}
				continue
			}
			rec, perr := parse(stripped)
			if perr != nil {
				if !complete {
					break // torn tail: ignore, never rewrite
				}
				fh.Close()
				return nil, &ChainError{Seq: int64(len(fd.records)) + 1, Reason: perr.Error()}
			}
			want := int64(len(fd.records)) + 1
			if rec.Seq != want {
				fh.Close()
				return nil, &ChainError{Seq: rec.Seq, Reason: fmt.Sprintf("seq %d where %d expected", rec.Seq, want)}
			}
			if rec.Prev != prev {
				fh.Close()
				return nil, &ChainError{Seq: rec.Seq, Reason: "prev hash does not match previous line"}
			}
			rec.LineHash = canon.SHA256Hex(stripped)
			prev = rec.LineHash
			fd.records = append(fd.records, rec)
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			fh.Close()
			return nil, rerr
		}
	}
	fd.needNewline = saw && lastByte != '\n'
	return fd, nil
}

func parse(line []byte) (Record, error) {
	v, err := canon.Decode(line)
	if err != nil {
		return Record{}, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Record{}, fmt.Errorf("record is not an object")
	}
	var r Record
	seq, ok := m["seq"].(json.Number)
	if !ok {
		return Record{}, fmt.Errorf("seq missing")
	}
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
		return Record{}, fmt.Errorf("prev missing or malformed")
	}
	if r.Payload, ok = m["payload"].(map[string]any); !ok {
		return Record{}, fmt.Errorf("payload missing")
	}
	return r, nil
}

// Append writes one canonical record with the next seq and the chain prev,
// then fsyncs before returning. On any write/fsync error nothing is recorded.
func (fd *Feed) Append(typ, id, effective string, payload map[string]any) (Record, error) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
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
		return Record{}, err
	}
	if err := fd.f.Sync(); err != nil {
		return Record{}, err
	}
	rec.LineHash = canon.SHA256Hex(line)
	fd.needNewline = false
	fd.records = append(fd.records, rec)
	return rec, nil
}

// Records returns a copy of all verified records in seq order.
func (fd *Feed) Records() []Record {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	out := make([]Record, len(fd.records))
	copy(out, fd.records)
	return out
}

// Len is the number of verified records (= the last seq).
func (fd *Feed) Len() int64 { fd.mu.Lock(); defer fd.mu.Unlock(); return int64(len(fd.records)) }

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
func (fd *Feed) Close() error { fd.mu.Lock(); defer fd.mu.Unlock(); return fd.f.Close() }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go vet ./... && go test ./internal/feed/ -v`
Expected: PASS (5 tests). If `TestTamperedRecordIsRefusedAtNextSeq` reports seq 2 instead of 3, the `strings.Replace` hit record 2's price; the fixture prices are 1001, 1002, 1003, so `"price":1002` is record 2 and the break is detected at record **3** — the test is right; fix the implementation.

- [ ] **Step 5: Output commit for the operator**

```bash
cd ~/dev/meridian
git add internal/feed
git commit -m "feat: append-only hash-chained feed with chain verification"
```

---

### Task 3: The one division — round-half-even cost relief

**Files:**
- Create: `internal/fold/money.go`
- Test: `internal/fold/money_test.go`

**Interfaces:**
- Produces: `fold.RelieveCost(totalCost, qtySold, totalQty int64) int64` — `rhe(totalCost*qtySold, totalQty)`; panics on `totalQty <= 0`, `qtySold < 0`, `qtySold > totalQty`, or `totalCost < 0` (all are fold invariants violated upstream).

- [ ] **Step 1: Write the failing test**

`internal/fold/money_test.go`:
```go
package fold

import "testing"

func TestRelieveCostHalfEven(t *testing.T) {
	cases := []struct{ cost, sold, qty, want int64 }{
		{165050, 120, 300, 66020}, // exact
		{1001, 1, 2, 500},         // 500.5 -> 500 (even)
		{1003, 1, 2, 502},         // 501.5 -> 502 (even)
		{1005, 1, 2, 502},         // 502.5 -> 502 (even)
		{10, 1, 3, 3},             // 3.33 -> 3
		{20, 2, 3, 13},            // 13.33 -> 13
		{20, 1, 3, 7},             // 6.67 -> 7
		{99, 99, 99, 99},          // sell all -> whole cost
		{7, 0, 5, 0},
		{0, 3, 5, 0},
		{1 << 40, 1 << 20, 1 << 21, 1 << 39}, // product exceeds int64 -> big.Int path
	}
	for _, c := range cases {
		if got := RelieveCost(c.cost, c.sold, c.qty); got != c.want {
			t.Errorf("RelieveCost(%d,%d,%d)=%d want %d", c.cost, c.sold, c.qty, got, c.want)
		}
	}
}

func TestRelieveCostPanicsOnInvariantBreak(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on qtySold > totalQty")
		}
	}()
	RelieveCost(10, 6, 5)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fold/ -run RelieveCost -v`
Expected: FAIL — `undefined: RelieveCost`.

- [ ] **Step 3: Implement**

`internal/fold/money.go`:
```go
package fold

import "math/big"

// RelieveCost is the ledger's ONLY division: the cost relieved when qtySold of
// totalQty shares are sold from a position carrying totalCost, rounded
// half-to-even. All other money arithmetic is exact integer add/multiply.
// Uses big.Int so the product never overflows; the result always fits int64
// because it is <= totalCost.
func RelieveCost(totalCost, qtySold, totalQty int64) int64 {
	if totalQty <= 0 || qtySold < 0 || qtySold > totalQty || totalCost < 0 {
		panic("fold: RelieveCost invariant violated")
	}
	n := new(big.Int).Mul(big.NewInt(totalCost), big.NewInt(qtySold))
	d := big.NewInt(totalQty)
	quo, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	twice := new(big.Int).Lsh(rem, 1)
	switch twice.Cmp(d) {
	case 1:
		quo.Add(quo, big.NewInt(1))
	case 0:
		if quo.Bit(0) == 1 {
			quo.Add(quo, big.NewInt(1))
		}
	}
	return quo.Int64()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/fold/ -run RelieveCost -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Output commit for the operator**

```bash
cd ~/dev/meridian
git add internal/fold/money.go internal/fold/money_test.go
git commit -m "feat: round-half-even cost relief, the fold's single division"
```

---

### Task 4: The fold — dedupe, term resolution, application, valuation

**Files:**
- Create: `internal/fold/fold.go`
- Test: `internal/fold/fold_test.go`

**Interfaces:**
- Consumes: `feed.Record` (Task 2), `RelieveCost` (Task 3), `canon.Marshal`, `canon.SHA256Hex`.
- Produces:
  ```go
  type Position struct{ Qty, TotalCost int64 }
  type Valuation struct{ Price, PriceSeq, Unrealized int64 }
  type Absorbed struct{ Seq int64; EventID, Key string }
  type Refusal struct{ Seq int64; EventID, Key, Kind, Detail string } // Kind: collision | oversell | unknown_action | malformed
  type Unevaluable struct{ Instrument, Reason string }
  type State struct {
      Seq            int64
      Cash           int64
      RealizedPnL    int64
      DividendIncome int64
      Positions      map[string]Position
      Valuations     map[string]Valuation // only instruments that have a price
      Unevaluable    []Unevaluable        // sorted by instrument
      Absorbed       []Absorbed           // seq order
      Refusals       []Refusal            // seq order
  }
  func (s State) UnrealizedPnL() int64
  func Fold(records []feed.Record, upTo int64) (State, error) // folds records with Seq <= upTo; error only on upTo > len
  func FillKey(payload map[string]any) (string, error)         // sha256 hex of canonical {"trade_id","venue"}
  func PayloadHash(payload map[string]any) string               // sha256 hex of canonical payload
  ```
- A malformed event (missing/invalid field, wrong side/kind, non-positive qty/price/ratio, negative rate) becomes a `malformed` refusal and is skipped — never an error, never a panic. The fold must be total over any chain-valid feed.

- [ ] **Step 1: Write the failing test**

`internal/fold/fold_test.go`:
```go
package fold

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/hossainpazooki/meridian/internal/feed"
)

func num(i int) json.Number { return json.Number(strconv.Itoa(i)) }

type ev struct {
	typ, eff string
	p        map[string]any
}

func fill(eff, inst, side string, qty, price int, tid string) ev {
	return ev{"fill", eff, map[string]any{"instrument": inst, "price": num(price), "qty": num(qty), "side": side, "trade_id": tid, "venue": "X"}}
}
func price(eff, inst string, px int) ev {
	return ev{"price", eff, map[string]any{"instrument": inst, "price": num(px)}}
}
func split(eff, id, inst string, ratio int) ev {
	return ev{"action", eff, map[string]any{"action_id": id, "announced": eff, "instrument": inst, "kind": "split", "processed": eff, "ratio": num(ratio)}}
}
func dividend(eff, id, inst string, rate int) ev {
	return ev{"action", eff, map[string]any{"action_id": id, "announced": eff, "instrument": inst, "kind": "dividend", "processed": eff, "rate": num(rate)}}
}
func amendRatio(eff, id string, ratio int) ev {
	return ev{"action_amendment", eff, map[string]any{"action_id": id, "ratio": num(ratio)}}
}

func records(evs ...ev) []feed.Record {
	out := make([]feed.Record, len(evs))
	for i, e := range evs {
		out[i] = feed.Record{Seq: int64(i + 1), Type: e.typ, ID: "ev-" + strconv.Itoa(i+1), Effective: e.eff, Payload: e.p}
	}
	return out
}

// Hand-computed golden (see Task 9 p6): AAA buy 100@1000, buy 50@1301,
// split 2, sell 120@700, dividend 25, price 800.
func TestFoldGoldenAAA(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 100, 1000, "T-1"),
		fill("2026-01-06", "AAA", "buy", 50, 1301, "T-2"),
		split("2026-01-10", "CA-1", "AAA", 2),
		fill("2026-01-12", "AAA", "sell", 120, 700, "T-3"),
		dividend("2026-01-15", "CA-2", "AAA", 25),
		price("2026-01-16", "AAA", 800),
	)
	s, err := Fold(rs, 6)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Positions["AAA"]
	if p.Qty != 180 || p.TotalCost != 99030 {
		t.Fatalf("position %+v", p)
	}
	if s.Cash != -76550 || s.RealizedPnL != 17980 || s.DividendIncome != 4500 {
		t.Fatalf("cash %d realized %d div %d", s.Cash, s.RealizedPnL, s.DividendIncome)
	}
	v := s.Valuations["AAA"]
	if v.Price != 800 || v.PriceSeq != 6 || v.Unrealized != 44970 || s.UnrealizedPnL() != 44970 {
		t.Fatalf("valuation %+v total %d", v, s.UnrealizedPnL())
	}
	if len(s.Refusals) != 0 || len(s.Absorbed) != 0 || len(s.Unevaluable) != 0 {
		t.Fatalf("unexpected records: %+v %+v %+v", s.Refusals, s.Absorbed, s.Unevaluable)
	}
}

func TestFoldHalfEvenBothDirections(t *testing.T) {
	rs := records(
		fill("2026-01-05", "BBB", "buy", 1, 500, "T-1"),
		fill("2026-01-05", "BBB", "buy", 1, 503, "T-2"), // cost 1003
		fill("2026-01-06", "BBB", "sell", 1, 600, "T-3"), // 501.5 -> 502
		fill("2026-01-05", "CCC", "buy", 1, 500, "T-4"),
		fill("2026-01-05", "CCC", "buy", 1, 501, "T-5"), // cost 1001
		fill("2026-01-06", "CCC", "sell", 1, 600, "T-6"), // 500.5 -> 500
	)
	s, _ := Fold(rs, 6)
	if s.Positions["BBB"].TotalCost != 501 || s.Positions["CCC"].TotalCost != 501 {
		t.Fatalf("%+v", s.Positions)
	}
	if s.RealizedPnL != 98+100 {
		t.Fatalf("realized %d", s.RealizedPnL)
	}
}

func TestDuplicateAbsorbedCollisionRefused(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 10, 100, "T-1"),
		fill("2026-01-05", "AAA", "buy", 10, 100, "T-1"), // exact duplicate
		fill("2026-01-05", "AAA", "buy", 11, 100, "T-1"), // same key, different payload
	)
	s, _ := Fold(rs, 3)
	if s.Positions["AAA"].Qty != 10 {
		t.Fatalf("qty %d", s.Positions["AAA"].Qty)
	}
	if len(s.Absorbed) != 1 || s.Absorbed[0].Seq != 2 || s.Absorbed[0].EventID != "ev-2" {
		t.Fatalf("absorbed %+v", s.Absorbed)
	}
	if len(s.Refusals) != 1 || s.Refusals[0].Kind != "collision" || s.Refusals[0].Seq != 3 {
		t.Fatalf("refusals %+v", s.Refusals)
	}
	k, _ := FillKey(rs[0].Payload)
	if s.Absorbed[0].Key != k || s.Refusals[0].Key != k {
		t.Fatal("key mismatch")
	}
}

func TestOversellRefusedFailClosed(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 10, 100, "T-1"),
		fill("2026-01-06", "AAA", "sell", 11, 100, "T-2"),
	)
	s, _ := Fold(rs, 2)
	if s.Positions["AAA"].Qty != 10 || len(s.Refusals) != 1 || s.Refusals[0].Kind != "oversell" {
		t.Fatalf("%+v %+v", s.Positions, s.Refusals)
	}
}

func TestSellAllRemovesPosition(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 3, 333, "T-1"),
		fill("2026-01-06", "AAA", "sell", 3, 400, "T-2"),
	)
	s, _ := Fold(rs, 2)
	if _, ok := s.Positions["AAA"]; ok {
		t.Fatal("position should be removed at qty 0")
	}
	if s.RealizedPnL != 3*400-999 {
		t.Fatal(s.RealizedPnL)
	}
}

func TestAmendmentThreeViewpoints(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 100, 1000, "T-1"),
		split("2026-01-10", "CA-1", "AAA", 2), // seq 2
		fill("2026-01-11", "AAA", "sell", 50, 600, "T-2"),
		amendRatio("2026-01-10", "CA-1", 3), // seq 4: split was really 3-for-1
	)
	v1, _ := Fold(rs, 1)
	v2, _ := Fold(rs, 3)
	v3, _ := Fold(rs, 4)
	if v1.Positions["AAA"].Qty != 100 {
		t.Fatal(v1.Positions)
	}
	// V2: 200 shares cost 100000, sell 50 -> relieved 25000, qty 150, cost 75000
	if v2.Positions["AAA"].Qty != 150 || v2.Positions["AAA"].TotalCost != 75000 || v2.RealizedPnL != 30000-25000 {
		t.Fatalf("V2 %+v realized %d", v2.Positions, v2.RealizedPnL)
	}
	// V3: 300 shares cost 100000, sell 50 -> relieved rhe(5000000/300)=16667 (16666.67), qty 250, cost 83333
	if v3.Positions["AAA"].Qty != 250 || v3.Positions["AAA"].TotalCost != 83333 || v3.RealizedPnL != 30000-16667 {
		t.Fatalf("V3 %+v realized %d", v3.Positions, v3.RealizedPnL)
	}
}

func TestPriceLatestByEffectiveThenSeq(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 1, 100, "T-1"),
		price("2026-01-09", "AAA", 900), // later effective, earlier seq
		price("2026-01-08", "AAA", 800), // late-arriving older price must not win
	)
	s, _ := Fold(rs, 3)
	if s.Valuations["AAA"].Price != 900 || s.Valuations["AAA"].PriceSeq != 2 {
		t.Fatalf("%+v", s.Valuations)
	}
}

func TestMissingPriceIsUnevaluable(t *testing.T) {
	rs := records(
		fill("2026-01-05", "AAA", "buy", 1, 100, "T-1"),
		fill("2026-01-05", "CCC", "buy", 1, 100, "T-2"),
		price("2026-01-06", "AAA", 120),
	)
	s, _ := Fold(rs, 3)
	if len(s.Unevaluable) != 1 || s.Unevaluable[0].Instrument != "CCC" || s.Unevaluable[0].Reason != "no_price_in_prefix" {
		t.Fatalf("%+v", s.Unevaluable)
	}
	if _, ok := s.Valuations["CCC"]; ok {
		t.Fatal("CCC must not be valued")
	}
	if s.UnrealizedPnL() != 20 {
		t.Fatal(s.UnrealizedPnL())
	}
}

func TestMalformedAndUnknownActionAreRefusals(t *testing.T) {
	rs := records(
		ev{"fill", "2026-01-05", map[string]any{"instrument": "AAA", "price": num(100), "qty": num(0), "side": "buy", "trade_id": "T-1", "venue": "X"}},
		ev{"fill", "2026-01-05", map[string]any{"instrument": "AAA", "price": num(100), "qty": num(1), "side": "short", "trade_id": "T-2", "venue": "X"}},
		amendRatio("2026-01-05", "CA-404", 2),
		ev{"bogus", "2026-01-05", map[string]any{}},
	)
	s, _ := Fold(rs, 4)
	kinds := map[string]int{}
	for _, r := range s.Refusals {
		kinds[r.Kind]++
	}
	if kinds["malformed"] != 3 || kinds["unknown_action"] != 1 || len(s.Positions) != 0 {
		t.Fatalf("%+v", s.Refusals)
	}
}

func TestFoldUpToBeyondFeedIsError(t *testing.T) {
	if _, err := Fold(records(price("2026-01-05", "AAA", 1)), 2); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fold/ -v`
Expected: FAIL — `undefined: Fold` (and friends).

- [ ] **Step 3: Implement**

`internal/fold/fold.go`:
```go
// Package fold turns a feed prefix into ledger state. It is pure: no I/O, no
// clock, no floats; the same records in the same order always produce the
// same State. Malformed input becomes a refusal record, never an error.
package fold

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/feed"
)

type Position struct{ Qty, TotalCost int64 }
type Valuation struct{ Price, PriceSeq, Unrealized int64 }
type Absorbed struct {
	Seq          int64
	EventID, Key string
}
type Refusal struct {
	Seq                        int64
	EventID, Key, Kind, Detail string
}
type Unevaluable struct{ Instrument, Reason string }

// State is the complete fold output at Seq.
type State struct {
	Seq            int64
	Cash           int64
	RealizedPnL    int64
	DividendIncome int64
	Positions      map[string]Position
	Valuations     map[string]Valuation
	Unevaluable    []Unevaluable
	Absorbed       []Absorbed
	Refusals       []Refusal
}

// UnrealizedPnL sums valued positions only.
func (s State) UnrealizedPnL() int64 {
	var t int64
	for _, v := range s.Valuations {
		t += v.Unrealized
	}
	return t
}

// FillKey is the ledger-derived idempotency key: sha256 of canonical
// {"trade_id","venue"}. Never producer-supplied.
func FillKey(payload map[string]any) (string, error) {
	tid, ok1 := payload["trade_id"].(string)
	ven, ok2 := payload["venue"].(string)
	if !ok1 || !ok2 || tid == "" || ven == "" {
		return "", fmt.Errorf("fill identity fields missing")
	}
	b, err := canon.Marshal(map[string]any{"trade_id": tid, "venue": ven})
	if err != nil {
		return "", err
	}
	return canon.SHA256Hex(b), nil
}

// PayloadHash is sha256 of the canonical payload; "" if it cannot marshal.
func PayloadHash(payload map[string]any) string {
	b, err := canon.Marshal(payload)
	if err != nil {
		return ""
	}
	return canon.SHA256Hex(b)
}

func getInt(m map[string]any, k string) (int64, bool) {
	n, ok := m[k].(json.Number)
	if !ok {
		return 0, false
	}
	v, err := n.Int64()
	return v, err == nil
}

func getStr(m map[string]any, k string) (string, bool) {
	s, ok := m[k].(string)
	return s, ok && s != ""
}

// applicable is one resolved event ready to apply in (effective, seq) order.
type applicable struct {
	seq        int64
	effective  string
	kind       string // buy | sell | split | dividend | price
	instrument string
	qty, price int64 // fills
	ratio      int64 // split
	rate       int64 // dividend
}

// Fold folds records with Seq <= upTo per the shared contract.
func Fold(records []feed.Record, upTo int64) (State, error) {
	if upTo < 0 || upTo > int64(len(records)) {
		return State{}, fmt.Errorf("fold: upTo %d out of range [0,%d]", upTo, len(records))
	}
	st := State{Seq: upTo, Positions: map[string]Position{}, Valuations: map[string]Valuation{},
		Unevaluable: []Unevaluable{}, Absorbed: []Absorbed{}, Refusals: []Refusal{}}
	refuse := func(r feed.Record, key, kind, detail string) {
		st.Refusals = append(st.Refusals, Refusal{Seq: r.Seq, EventID: r.ID, Key: key, Kind: kind, Detail: detail})
	}

	// Pass 1: action terms, then amendments overwrite in seq order.
	type action struct {
		rec        feed.Record
		instrument string
		kind       string
		ratio      int64
		rate       int64
	}
	actions := map[string]*action{}
	for _, r := range records[:upTo] {
		if r.Type != "action" {
			continue
		}
		id, ok1 := getStr(r.Payload, "action_id")
		inst, ok2 := getStr(r.Payload, "instrument")
		kind, ok3 := getStr(r.Payload, "kind")
		if !ok1 || !ok2 || !ok3 {
			refuse(r, "", "malformed", "action missing action_id/instrument/kind")
			continue
		}
		if _, dup := actions[id]; dup {
			refuse(r, "", "malformed", "duplicate action_id "+id)
			continue
		}
		a := &action{rec: r, instrument: inst, kind: kind}
		switch kind {
		case "split":
			if v, ok := getInt(r.Payload, "ratio"); ok && v >= 1 {
				a.ratio = v
			} else {
				refuse(r, "", "malformed", "split ratio must be a whole number >= 1")
				continue
			}
		case "dividend":
			if v, ok := getInt(r.Payload, "rate"); ok && v >= 0 {
				a.rate = v
			} else {
				refuse(r, "", "malformed", "dividend rate must be an integer >= 0")
				continue
			}
		default:
			refuse(r, "", "malformed", "unknown action kind "+kind)
			continue
		}
		actions[id] = a
	}
	for _, r := range records[:upTo] {
		if r.Type != "action_amendment" {
			continue
		}
		id, ok := getStr(r.Payload, "action_id")
		if !ok {
			refuse(r, "", "malformed", "amendment missing action_id")
			continue
		}
		a, ok := actions[id]
		if !ok {
			refuse(r, "", "unknown_action", "amendment references unknown action "+id)
			continue
		}
		switch a.kind {
		case "split":
			if v, ok := getInt(r.Payload, "ratio"); ok && v >= 1 {
				a.ratio = v
			} else {
				refuse(r, "", "malformed", "amendment ratio must be a whole number >= 1")
			}
		case "dividend":
			if v, ok := getInt(r.Payload, "rate"); ok && v >= 0 {
				a.rate = v
			} else {
				refuse(r, "", "malformed", "amendment rate must be an integer >= 0")
			}
		}
	}

	// Pass 2: dedupe fills, decode prices, collect applicables.
	var apps []applicable
	seen := map[string]string{} // key -> payload hash
	for _, r := range records[:upTo] {
		switch r.Type {
		case "fill":
			key, err := FillKey(r.Payload)
			if err != nil {
				refuse(r, "", "malformed", err.Error())
				continue
			}
			ph := PayloadHash(r.Payload)
			if prev, dup := seen[key]; dup {
				if prev == ph {
					st.Absorbed = append(st.Absorbed, Absorbed{Seq: r.Seq, EventID: r.ID, Key: key})
				} else {
					refuse(r, key, "collision", "payload hash mismatch")
				}
				continue
			}
			inst, ok1 := getStr(r.Payload, "instrument")
			side, ok2 := getStr(r.Payload, "side")
			qty, ok3 := getInt(r.Payload, "qty")
			px, ok4 := getInt(r.Payload, "price")
			if !ok1 || !ok2 || !ok3 || !ok4 || qty <= 0 || px <= 0 || (side != "buy" && side != "sell") {
				refuse(r, key, "malformed", "fill fields invalid")
				continue
			}
			seen[key] = ph
			apps = append(apps, applicable{seq: r.Seq, effective: r.Effective, kind: side, instrument: inst, qty: qty, price: px})
		case "price":
			inst, ok1 := getStr(r.Payload, "instrument")
			px, ok2 := getInt(r.Payload, "price")
			if !ok1 || !ok2 || px <= 0 {
				refuse(r, "", "malformed", "price fields invalid")
				continue
			}
			apps = append(apps, applicable{seq: r.Seq, effective: r.Effective, kind: "price", instrument: inst, price: px})
		case "action", "action_amendment":
			// handled in pass 1
		default:
			refuse(r, "", "malformed", "unknown event type "+r.Type)
		}
	}
	for _, a := range actions {
		apps = append(apps, applicable{seq: a.rec.Seq, effective: a.rec.Effective, kind: a.kind, instrument: a.instrument, ratio: a.ratio, rate: a.rate})
	}
	sort.SliceStable(apps, func(i, j int) bool {
		if apps[i].effective != apps[j].effective {
			return apps[i].effective < apps[j].effective
		}
		return apps[i].seq < apps[j].seq
	})

	// Pass 3: apply.
	lastPrice := map[string]Valuation{}
	lastEff := map[string]string{}
	bySeq := map[int64]feed.Record{}
	for _, r := range records[:upTo] {
		bySeq[r.Seq] = r
	}
	for _, a := range apps {
		p := st.Positions[a.instrument]
		switch a.kind {
		case "buy":
			p.Qty += a.qty
			p.TotalCost += a.qty * a.price
			st.Cash -= a.qty * a.price
			st.Positions[a.instrument] = p
		case "sell":
			if a.qty > p.Qty {
				r := bySeq[a.seq]
				key, _ := FillKey(r.Payload)
				refuse(r, key, "oversell", fmt.Sprintf("sell %d exceeds held %d", a.qty, p.Qty))
				continue
			}
			relieved := RelieveCost(p.TotalCost, a.qty, p.Qty)
			p.TotalCost -= relieved
			p.Qty -= a.qty
			st.Cash += a.qty * a.price
			st.RealizedPnL += a.qty*a.price - relieved
			if p.Qty == 0 {
				if p.TotalCost != 0 {
					panic("fold: cost remains after full relief")
				}
				delete(st.Positions, a.instrument)
			} else {
				st.Positions[a.instrument] = p
			}
		case "split":
			if _, ok := st.Positions[a.instrument]; ok {
				p.Qty *= a.ratio
				st.Positions[a.instrument] = p
			}
		case "dividend":
			if _, ok := st.Positions[a.instrument]; ok && p.Qty > 0 {
				d := p.Qty * a.rate
				st.Cash += d
				st.DividendIncome += d
			}
		case "price":
			if prevEff, ok := lastEff[a.instrument]; !ok || a.effective > prevEff || (a.effective == prevEff && a.seq > lastPrice[a.instrument].PriceSeq) {
				lastPrice[a.instrument] = Valuation{Price: a.price, PriceSeq: a.seq}
				lastEff[a.instrument] = a.effective
			}
		}
	}

	// Pass 4: valuation.
	for inst, p := range st.Positions {
		if v, ok := lastPrice[inst]; ok {
			v.Unrealized = p.Qty*v.Price - p.TotalCost
			st.Valuations[inst] = v
		} else {
			st.Unevaluable = append(st.Unevaluable, Unevaluable{Instrument: inst, Reason: "no_price_in_prefix"})
		}
	}
	sort.Slice(st.Unevaluable, func(i, j int) bool { return st.Unevaluable[i].Instrument < st.Unevaluable[j].Instrument })
	sort.SliceStable(st.Refusals, func(i, j int) bool { return st.Refusals[i].Seq < st.Refusals[j].Seq })
	sort.SliceStable(st.Absorbed, func(i, j int) bool { return st.Absorbed[i].Seq < st.Absorbed[j].Seq })
	return st, nil
}
```
Note on the price rule: because `apps` is already sorted by `(effective, seq)`, the last `price` applied per instrument is the latest by that order; the explicit comparison is belt-and-braces and must agree with the sort.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go vet ./... && go test ./internal/fold/ -v`
Expected: PASS (11 tests). If `TestAmendmentThreeViewpoints` V3 fails on 16667: `rhe(100000*50, 300)` = 16666 rem 200/300 → 2*200=400 > 300 → 16667. The test is right.

- [ ] **Step 5: Output commit for the operator**

```bash
cd ~/dev/meridian
git add internal/fold/fold.go internal/fold/fold_test.go
git commit -m "feat: pure fold - dedupe, term resolution, application, valuation"
```

---

### Task 5: Snapshot — canonical, content-addressed, diffable

**Files:**
- Create: `internal/snapshot/snapshot.go`, `internal/snapshot/diff.go`
- Test: `internal/snapshot/snapshot_test.go`

**Interfaces:**
- Consumes: `fold.State`, `canon.*`.
- Produces:
  ```go
  type Doc = map[string]any                                   // decoded snapshot (json.Number leaves)
  func Build(s fold.State, prefixHash string) (Doc, []byte, string) // doc, bytes (canonical + "\n"), "sha256:<hex>"
  func Decode(b []byte) (Doc, error)                          // any JSON object in snapshot schema (Python twins too)
  func Write(dir string, b []byte, hash string) (path string, err error) // <dir>/<hex>.json
  type Mismatch struct{ Path, Want, Got string }               // Path like "positions.AAA.qty"; Want/Got canonical text
  func Diff(golden, doc Doc) []Mismatch                        // every leaf in golden compared to doc; missing => Got "<absent>"
  func Leaves(doc Doc) int                                     // number of leaves (for `evaluated`)
  ```

- [ ] **Step 1: Write the failing test**

`internal/snapshot/snapshot_test.go`:
```go
package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hossainpazooki/meridian/internal/fold"
)

func sample() fold.State {
	return fold.State{Seq: 6, Cash: -76550, RealizedPnL: 17980, DividendIncome: 4500,
		Positions:   map[string]fold.Position{"AAA": {Qty: 180, TotalCost: 99030}, "CCC": {Qty: 3, TotalCost: 1500}},
		Valuations:  map[string]fold.Valuation{"AAA": {Price: 800, PriceSeq: 6, Unrealized: 44970}},
		Unevaluable: []fold.Unevaluable{{Instrument: "CCC", Reason: "no_price_in_prefix"}},
		Absorbed:    []fold.Absorbed{}, Refusals: []fold.Refusal{{Seq: 4, EventID: "ev-4", Key: "k", Kind: "collision", Detail: "payload hash mismatch"}}}
}

func TestBuildIsCanonicalAndStable(t *testing.T) {
	_, b1, h1 := Build(sample(), "sha256:abc")
	_, b2, h2 := Build(sample(), "sha256:abc")
	if string(b1) != string(b2) || h1 != h2 {
		t.Fatal("not stable")
	}
	want := `{"absorbed":[],"cash":-76550,"dividend_income":4500,"feed_prefix_hash":"sha256:abc","feed_seq":6,` +
		`"positions":{"AAA":{"qty":180,"total_cost":99030,"valuation":{"price":800,"price_seq":6,"unrealized":44970}},` +
		`"CCC":{"qty":3,"total_cost":1500,"valuation":null}},"realized_pnl":17980,` +
		`"refusals":[{"detail":"payload hash mismatch","event_id":"ev-4","key":"k","kind":"collision","seq":4}],` +
		`"unevaluable":[{"instrument":"CCC","reason":"no_price_in_prefix"}],"unrealized_pnl":44970}` + "\n"
	if string(b1) != want {
		t.Fatalf("bytes:\n%s\nwant:\n%s", b1, want)
	}
	if !strings.HasPrefix(h1, "sha256:") || len(h1) != 71 {
		t.Fatal(h1)
	}
	dir := t.TempDir()
	p, err := Write(dir, b1, h1)
	if err != nil || filepath.Base(p) != strings.TrimPrefix(h1, "sha256:")+".json" {
		t.Fatal(p, err)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != string(b1) {
		t.Fatal("written bytes differ")
	}
}

func TestDiffReportsLeafMismatchesOnly(t *testing.T) {
	doc, _, _ := Build(sample(), "sha256:abc")
	golden, _ := Decode([]byte(`{"cash":-76550,"positions":{"AAA":{"qty":181,"valuation":{"unrealized":44970}},"ZZZ":{"qty":1}},"realized_pnl":17980}`))
	ms := Diff(golden, doc)
	if len(ms) != 2 {
		t.Fatalf("%+v", ms)
	}
	if ms[0].Path != "positions.AAA.qty" || ms[0].Want != "181" || ms[0].Got != "180" {
		t.Fatalf("%+v", ms[0])
	}
	if ms[1].Path != "positions.ZZZ.qty" || ms[1].Got != "<absent>" {
		t.Fatalf("%+v", ms[1])
	}
	if Leaves(golden) != 5 {
		t.Fatal(Leaves(golden))
	}
	if n := Diff(doc, doc); len(n) != 0 {
		t.Fatal(n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/snapshot/ -v` → FAIL, `undefined: Build`.

- [ ] **Step 3: Implement**

`internal/snapshot/snapshot.go`:
```go
// Package snapshot serializes fold.State as a canonical, content-addressed
// artifact stamped with the feed prefix it derives from. Decode accepts any
// document in the same schema (including Python-emitted twins).
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/fold"
)

type Doc = map[string]any

func n(i int64) json.Number { return json.Number(strconv.FormatInt(i, 10)) }

// Build renders s. Bytes are canonical JSON plus one '\n'; hash is over those bytes.
func Build(s fold.State, prefixHash string) (Doc, []byte, string) {
	positions := map[string]any{}
	for inst, p := range s.Positions {
		var val any
		if v, ok := s.Valuations[inst]; ok {
			val = map[string]any{"price": n(v.Price), "price_seq": n(v.PriceSeq), "unrealized": n(v.Unrealized)}
		}
		positions[inst] = map[string]any{"qty": n(p.Qty), "total_cost": n(p.TotalCost), "valuation": val}
	}
	absorbed := make([]any, 0, len(s.Absorbed))
	for _, a := range s.Absorbed {
		absorbed = append(absorbed, map[string]any{"event_id": a.EventID, "key": a.Key, "seq": n(a.Seq)})
	}
	refusals := make([]any, 0, len(s.Refusals))
	for _, r := range s.Refusals {
		refusals = append(refusals, map[string]any{"detail": r.Detail, "event_id": r.EventID, "key": r.Key, "kind": r.Kind, "seq": n(r.Seq)})
	}
	unev := make([]any, 0, len(s.Unevaluable))
	for _, u := range s.Unevaluable {
		unev = append(unev, map[string]any{"instrument": u.Instrument, "reason": u.Reason})
	}
	doc := Doc{
		"absorbed": absorbed, "cash": n(s.Cash), "dividend_income": n(s.DividendIncome),
		"feed_prefix_hash": prefixHash, "feed_seq": n(s.Seq), "positions": positions,
		"realized_pnl": n(s.RealizedPnL), "refusals": refusals, "unevaluable": unev,
		"unrealized_pnl": n(s.UnrealizedPnL()),
	}
	b, err := canon.Marshal(doc)
	if err != nil {
		panic(err) // doc is built from primitives only
	}
	b = append(b, '\n')
	return doc, b, "sha256:" + canon.SHA256Hex(b)
}

// Decode parses a snapshot-schema JSON object.
func Decode(b []byte) (Doc, error) {
	v, err := canon.Decode(b)
	if err != nil {
		return nil, err
	}
	d, ok := v.(Doc)
	if !ok {
		return nil, fmt.Errorf("snapshot is not a JSON object")
	}
	return d, nil
}

// Write stores bytes as <dir>/<hex>.json and returns the path.
func Write(dir string, b []byte, hash string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, strings.TrimPrefix(hash, "sha256:")+".json")
	return p, os.WriteFile(p, b, 0o644)
}
```

`internal/snapshot/diff.go`:
```go
package snapshot

import (
	"sort"

	"github.com/hossainpazooki/meridian/internal/canon"
)

// Mismatch is one golden leaf that the doc does not reproduce.
type Mismatch struct{ Path, Want, Got string }

func leafText(v any) string {
	b, err := canon.Marshal(v)
	if err != nil {
		return "<unmarshalable>"
	}
	return string(b)
}

// Diff walks every leaf of golden and compares it to the same path in doc.
// Keys present only in doc are ignored (golden may be a subset). Output is
// sorted by Path.
func Diff(golden, doc Doc) []Mismatch {
	var out []Mismatch
	var walk func(path string, g, d any, present bool)
	walk = func(path string, g, d any, present bool) {
		if gm, ok := g.(map[string]any); ok {
			dm, _ := d.(map[string]any)
			for k, gv := range gm {
				dv, has := dm[k]
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(p, gv, dv, present && has)
			}
			return
		}
		if !present {
			out = append(out, Mismatch{Path: path, Want: leafText(g), Got: "<absent>"})
			return
		}
		if leafText(g) != leafText(d) {
			out = append(out, Mismatch{Path: path, Want: leafText(g), Got: leafText(d)})
		}
	}
	walk("", golden, doc, true)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Leaves counts non-object leaves in doc (lists count as one leaf).
func Leaves(doc Doc) int {
	c := 0
	var walk func(v any)
	walk = func(v any) {
		if m, ok := v.(map[string]any); ok {
			for _, x := range m {
				walk(x)
			}
			return
		}
		c++
	}
	walk(doc)
	return c
}
```
(`sort` is imported in snapshot.go only if used; remove the import if `go vet` flags it.)

- [ ] **Step 4: Run tests** — `go vet ./... && go test ./internal/snapshot/ -v` → PASS (2 tests).

- [ ] **Step 5: Commit for the operator**

```bash
cd ~/dev/meridian
git add internal/snapshot
git commit -m "feat: canonical content-addressed snapshot + leaf diff"
```

---

### Task 6: As-of read = pure recompute

**Files:**
- Create: `internal/asof/asof.go`
- Test: `internal/asof/asof_test.go`

**Interfaces:**
- Consumes: `feed.Open/Records/PrefixHash/Len`, `fold.Fold`, `snapshot.Build`.
- Produces:
  ```go
  type Result struct{ State fold.State; Doc snapshot.Doc; Bytes []byte; Hash, PrefixHash string; Seq int64 }
  func Read(feedPath string, seq int64) (Result, error)   // seq < 0 means "end of feed"
  ```

- [ ] **Step 1: Write the failing test**

`internal/asof/asof_test.go`:
```go
package asof

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/feed"
)

func TestReadAtViewpointsIsPureRecompute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feed.jsonl")
	f, _ := feed.Open(path)
	f.Append("fill", "e1", "2026-01-05", map[string]any{"instrument": "AAA", "price": json.Number("1000"), "qty": json.Number("100"), "side": "buy", "trade_id": "T-1", "venue": "X"})
	f.Append("action", "e2", "2026-01-10", map[string]any{"action_id": "CA-1", "announced": "2026-01-08", "instrument": "AAA", "kind": "split", "processed": "2026-01-10", "ratio": json.Number("2")})
	f.Append("action_amendment", "e3", "2026-01-10", map[string]any{"action_id": "CA-1", "ratio": json.Number("3")})
	f.Close()

	r1, err := Read(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := Read(path, 2)
	r3, _ := Read(path, -1)
	if r1.State.Positions["AAA"].Qty != 100 || r2.State.Positions["AAA"].Qty != 200 || r3.State.Positions["AAA"].Qty != 300 {
		t.Fatalf("%d %d %d", r1.State.Positions["AAA"].Qty, r2.State.Positions["AAA"].Qty, r3.State.Positions["AAA"].Qty)
	}
	if r3.Seq != 3 || r3.Doc["feed_seq"].(json.Number) != "3" || r3.Doc["feed_prefix_hash"] != r3.PrefixHash {
		t.Fatalf("%+v", r3)
	}
	again, _ := Read(path, -1)
	if again.Hash != r3.Hash || string(again.Bytes) != string(r3.Bytes) {
		t.Fatal("recompute not identical")
	}
	if _, err := Read(path, 4); err == nil {
		t.Fatal("seq beyond feed must error")
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/asof/ -v` → FAIL, `undefined: Read`.

- [ ] **Step 3: Implement**

`internal/asof/asof.go`:
```go
// Package asof answers "what did the ledger know at seq V" by replaying the
// prefix [1..V] through the fold on every call. Nothing is cached; nothing
// derived exists outside this call.
package asof

import (
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/fold"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

type Result struct {
	State            fold.State
	Doc              snapshot.Doc
	Bytes            []byte
	Hash, PrefixHash string
	Seq              int64
}

// Read opens feedPath (verifying its chain), folds [1..seq] and builds the
// snapshot. seq < 0 selects the last record.
func Read(feedPath string, seq int64) (Result, error) {
	f, err := feed.Open(feedPath)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	if seq < 0 {
		seq = f.Len()
	}
	ph, err := f.PrefixHash(seq)
	if err != nil {
		return Result{}, err
	}
	st, err := fold.Fold(f.Records(), seq)
	if err != nil {
		return Result{}, err
	}
	doc, b, h := snapshot.Build(st, ph)
	return Result{State: st, Doc: doc, Bytes: b, Hash: h, PrefixHash: ph, Seq: seq}, nil
}
```

- [ ] **Step 4: Run** `go test ./internal/asof/ -v` → PASS.

- [ ] **Step 5: Commit for the operator**

```bash
cd ~/dev/meridian
git add internal/asof
git commit -m "feat: as-of read as pure recompute over the feed prefix"
```

---

### Task 7: Field-level custodian reconciliation

**Files:**
- Create: `internal/reconcile/reconcile.go`
- Test: `internal/reconcile/reconcile_test.go`

**Interfaces:**
- Consumes: `snapshot.Doc`, `canon.Decode`.
- Produces:
  ```go
  type Holding struct{ Instrument string; Quantity, CostBasis int64 }
  type Statement struct{ AsOfSeq, Cash int64; Holdings []Holding }
  func LoadStatement(path string) (Statement, error)
  type Mismatch struct{ Instrument, Field string; Ledger, Custodian, Delta int64 } // Field: quantity | cost_basis | cash ; Instrument "" for cash; Delta = Ledger - Custodian
  func Reconcile(doc snapshot.Doc, st Statement) (mismatches []Mismatch, compared int)
  ```
- Rules: every holding in the statement must match a position (missing position ⇒ mismatches on both fields with Ledger 0); every position in the doc must appear in the statement (missing holding ⇒ mismatches with Custodian 0). `compared` = number of fields examined (2 per instrument in the union + 1 for cash). Output sorted by (Instrument, Field). Exact, zero tolerance.

- [ ] **Step 1: Write the failing test**

`internal/reconcile/reconcile_test.go`:
```go
package reconcile

import (
	"os"
	"path/filepath"
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
	ms, compared := Reconcile(d, st)
	if len(ms) != 0 || compared != 5 {
		t.Fatalf("%+v %d", ms, compared)
	}

	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"as_of_seq":3,"cash":-403,"holdings":[{"cost_basis":1001,"instrument":"AAA","quantity":10},{"cost_basis":501,"instrument":"CCC","quantity":1}]}`), 0o644)
	st2, _ := LoadStatement(bad)
	ms, _ = Reconcile(d, st2)
	// AAA cost_basis drift 1; BBB missing from statement (2 fields); CCC missing from ledger (2 fields)
	if len(ms) != 5 {
		t.Fatalf("%+v", ms)
	}
	if ms[0].Instrument != "AAA" || ms[0].Field != "cost_basis" || ms[0].Ledger != 1000 || ms[0].Custodian != 1001 || ms[0].Delta != -1 {
		t.Fatalf("%+v", ms[0])
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/reconcile/ -v` → FAIL.

- [ ] **Step 3: Implement**

`internal/reconcile/reconcile.go`:
```go
// Package reconcile compares a snapshot to a custodian statement field by
// field. The statement vocabulary (quantity, cost_basis, holdings) is
// deliberately not the snapshot's, so the mapping is explicit and a drift is
// named by instrument, field and signed amount.
package reconcile

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

type Holding struct {
	Instrument          string
	Quantity, CostBasis int64
}
type Statement struct {
	AsOfSeq, Cash int64
	Holdings      []Holding
}
type Mismatch struct {
	Instrument, Field        string
	Ledger, Custodian, Delta int64
}

func asInt(v any) (int64, error) {
	num, ok := v.(json.Number)
	if !ok {
		return 0, fmt.Errorf("not an integer: %v", v)
	}
	return num.Int64()
}

// LoadStatement reads the custodian JSON format.
func LoadStatement(path string) (Statement, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Statement{}, err
	}
	v, err := canon.Decode(raw)
	if err != nil {
		return Statement{}, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Statement{}, fmt.Errorf("statement is not an object")
	}
	var st Statement
	if st.AsOfSeq, err = asInt(m["as_of_seq"]); err != nil {
		return Statement{}, fmt.Errorf("as_of_seq: %w", err)
	}
	if st.Cash, err = asInt(m["cash"]); err != nil {
		return Statement{}, fmt.Errorf("cash: %w", err)
	}
	hs, ok := m["holdings"].([]any)
	if !ok {
		return Statement{}, fmt.Errorf("holdings missing")
	}
	for i, h := range hs {
		hm, ok := h.(map[string]any)
		if !ok {
			return Statement{}, fmt.Errorf("holding %d not an object", i)
		}
		inst, ok := hm["instrument"].(string)
		if !ok || inst == "" {
			return Statement{}, fmt.Errorf("holding %d instrument missing", i)
		}
		q, err := asInt(hm["quantity"])
		if err != nil {
			return Statement{}, fmt.Errorf("holding %s quantity: %w", inst, err)
		}
		c, err := asInt(hm["cost_basis"])
		if err != nil {
			return Statement{}, fmt.Errorf("holding %s cost_basis: %w", inst, err)
		}
		st.Holdings = append(st.Holdings, Holding{Instrument: inst, Quantity: q, CostBasis: c})
	}
	return st, nil
}

// Reconcile compares cash and every instrument in the union of both sides.
func Reconcile(doc snapshot.Doc, st Statement) ([]Mismatch, int) {
	var out []Mismatch
	compared := 0
	cash, _ := asInt(doc["cash"])
	compared++
	if cash != st.Cash {
		out = append(out, Mismatch{Field: "cash", Ledger: cash, Custodian: st.Cash, Delta: cash - st.Cash})
	}
	ledger := map[string][2]int64{}
	if pos, ok := doc["positions"].(map[string]any); ok {
		for inst, p := range pos {
			pm, _ := p.(map[string]any)
			q, _ := asInt(pm["qty"])
			c, _ := asInt(pm["total_cost"])
			ledger[inst] = [2]int64{q, c}
		}
	}
	cust := map[string][2]int64{}
	for _, h := range st.Holdings {
		cust[h.Instrument] = [2]int64{h.Quantity, h.CostBasis}
	}
	union := map[string]bool{}
	for k := range ledger {
		union[k] = true
	}
	for k := range cust {
		union[k] = true
	}
	for inst := range union {
		l, c := ledger[inst], cust[inst]
		compared += 2
		if l[0] != c[0] {
			out = append(out, Mismatch{Instrument: inst, Field: "quantity", Ledger: l[0], Custodian: c[0], Delta: l[0] - c[0]})
		}
		if l[1] != c[1] {
			out = append(out, Mismatch{Instrument: inst, Field: "cost_basis", Ledger: l[1], Custodian: c[1], Delta: l[1] - c[1]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Instrument != out[j].Instrument {
			return out[i].Instrument < out[j].Instrument
		}
		return out[i].Field < out[j].Field
	})
	return out, compared
}
```

- [ ] **Step 4: Run** `go vet ./... && go test ./internal/reconcile/ -v` → PASS.

- [ ] **Step 5: Commit for the operator**

```bash
cd ~/dev/meridian
git add internal/reconcile
git commit -m "feat: field-level custodian reconciliation with named drift"
```

---

### Task 8: CLI

**Files:**
- Create: `cmd/meridian/main.go`
- Test: `cmd/meridian/main_test.go`

**Interfaces:**
- Consumes: `feed`, `asof`, `snapshot`, `reconcile`.
- Produces the binary `meridian` with subcommands:
  - `append --feed F --type T --id ID --effective D --payload JSON` → prints `seq=<n> hash=<hex>`; exit 1 on error.
  - `replay --feed F` → verifies chain, prints `ok records=<n> prefix_hash=sha256:<hex>`; exit 2 on `ChainError` (message on stderr names the seq).
  - `snapshot --feed F [--seq V] --out DIR` → writes `<DIR>/<hex>.json`, prints `<hash> <path>`.
  - `asof --feed F --seq V` → prints the snapshot bytes to stdout.
  - `reconcile --feed F --statement S [--seq V]` → prints one line per mismatch `instrument=<i> field=<f> ledger=<l> custodian=<c> delta=<d>`, then `mismatches=<n> compared=<m>`; exit 1 if `n > 0`.
- All output ASCII. Exit codes: 0 ok, 1 usage/mismatch/other, 2 chain error.

- [ ] **Step 1: Write the failing test**

`cmd/meridian/main_test.go`:
```go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "meridian")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func run(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

func TestCLIRoundTrip(t *testing.T) {
	bin := build(t)
	dir := t.TempDir()
	feedPath := filepath.Join(dir, "feed.jsonl")
	out, code := run(t, bin, "append", "--feed", feedPath, "--type", "fill", "--id", "e1", "--effective", "2026-01-05",
		"--payload", `{"instrument":"AAA","price":1000,"qty":100,"side":"buy","trade_id":"T-1","venue":"X"}`)
	if code != 0 || !strings.HasPrefix(out, "seq=1 hash=") {
		t.Fatalf("%d %s", code, out)
	}
	run(t, bin, "append", "--feed", feedPath, "--type", "price", "--id", "e2", "--effective", "2026-01-06", "--payload", `{"instrument":"AAA","price":1100}`)
	out, code = run(t, bin, "replay", "--feed", feedPath)
	if code != 0 || !strings.HasPrefix(out, "ok records=2 prefix_hash=sha256:") {
		t.Fatalf("%d %s", code, out)
	}
	outDir := filepath.Join(dir, "snap")
	out, code = run(t, bin, "snapshot", "--feed", feedPath, "--out", outDir)
	if code != 0 {
		t.Fatal(out)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 || !strings.HasPrefix(fields[0], "sha256:") {
		t.Fatal(out)
	}
	raw, _ := os.ReadFile(fields[1])
	asofOut, _ := run(t, bin, "asof", "--feed", feedPath, "--seq", "2")
	if string(raw) != asofOut {
		t.Fatal("asof output must equal the snapshot bytes")
	}
	stmt := filepath.Join(dir, "s.json")
	os.WriteFile(stmt, []byte(`{"as_of_seq":2,"cash":-100000,"holdings":[{"cost_basis":100000,"instrument":"AAA","quantity":100}]}`), 0o644)
	out, code = run(t, bin, "reconcile", "--feed", feedPath, "--statement", stmt)
	if code != 0 || !strings.Contains(out, "mismatches=0 compared=3") {
		t.Fatalf("%d %s", code, out)
	}
	os.WriteFile(stmt, []byte(`{"as_of_seq":2,"cash":-100000,"holdings":[{"cost_basis":100001,"instrument":"AAA","quantity":100}]}`), 0o644)
	out, code = run(t, bin, "reconcile", "--feed", feedPath, "--statement", stmt)
	if code != 1 || !strings.Contains(out, "instrument=AAA field=cost_basis ledger=100000 custodian=100001 delta=-1") {
		t.Fatalf("%d %s", code, out)
	}
	// tamper -> replay exit 2
	rawFeed, _ := os.ReadFile(feedPath)
	os.WriteFile(feedPath, []byte(strings.Replace(string(rawFeed), `"price":1000`, `"price":1001`, 1)), 0o644)
	_, code = run(t, bin, "replay", "--feed", feedPath)
	if code != 2 {
		t.Fatalf("tampered feed must exit 2, got %d", code)
	}
}
```

- [ ] **Step 2: Run** `go test ./cmd/meridian/ -v` → FAIL (no main package / build fails).

- [ ] **Step 3: Implement**

`cmd/meridian/main.go`:
```go
// Command meridian is the thin CLI over the internal packages. It holds no
// logic of its own: append writes to the feed, replay verifies it, snapshot /
// asof recompute, reconcile compares. Exit codes: 0 ok, 1 usage or mismatch
// or other error, 2 feed chain error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/hossainpazooki/meridian/internal/asof"
	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reconcile"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

func main() { os.Exit(run(os.Args[1:])) }

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	var ce *feed.ChainError
	if errors.As(err, &ce) {
		return 2
	}
	return 1
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: meridian <append|replay|snapshot|asof|reconcile> [flags]")
		return 1
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	feedPath := fs.String("feed", "", "feed path")
	seq := fs.Int64("seq", -1, "viewpoint seq (default: end)")
	switch args[0] {
	case "append":
		typ := fs.String("type", "", "event type")
		id := fs.String("id", "", "event id")
		eff := fs.String("effective", "", "effective date YYYY-MM-DD")
		payload := fs.String("payload", "", "payload JSON object")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		v, err := canon.Decode([]byte(*payload))
		if err != nil {
			return fail(err)
		}
		pm, ok := v.(map[string]any)
		if !ok {
			return fail(errors.New("payload must be a JSON object"))
		}
		f, err := feed.Open(*feedPath)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		rec, err := f.Append(*typ, *id, *eff, pm)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("seq=%d hash=%s\n", rec.Seq, rec.LineHash)
	case "replay":
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		f, err := feed.Open(*feedPath)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		h, _ := f.PrefixHash(f.Len())
		fmt.Printf("ok records=%d prefix_hash=%s\n", f.Len(), h)
	case "snapshot":
		out := fs.String("out", "", "output directory")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		r, err := asof.Read(*feedPath, *seq)
		if err != nil {
			return fail(err)
		}
		p, err := snapshot.Write(*out, r.Bytes, r.Hash)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s %s\n", r.Hash, p)
	case "asof":
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		r, err := asof.Read(*feedPath, *seq)
		if err != nil {
			return fail(err)
		}
		os.Stdout.Write(r.Bytes)
	case "reconcile":
		stmt := fs.String("statement", "", "custodian statement JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		r, err := asof.Read(*feedPath, *seq)
		if err != nil {
			return fail(err)
		}
		st, err := reconcile.LoadStatement(*stmt)
		if err != nil {
			return fail(err)
		}
		ms, compared := reconcile.Reconcile(r.Doc, st)
		for _, m := range ms {
			fmt.Printf("instrument=%s field=%s ledger=%d custodian=%d delta=%d\n", m.Instrument, m.Field, m.Ledger, m.Custodian, m.Delta)
		}
		fmt.Printf("mismatches=%d compared=%d\n", len(ms), compared)
		if len(ms) > 0 {
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", args[0])
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run** `go vet ./... && go test ./... -count=1` → all PASS.

- [ ] **Step 5: Commit for the operator**

```bash
cd ~/dev/meridian
git add cmd/meridian
git commit -m "feat: meridian CLI - append, replay, snapshot, asof, reconcile"
```

---

### Task 9: Fixture generator with the import-pinned naive fold

**Files:**
- Create: `fixtures/generate.py`
- Create (generated, checked in): everything under `fixtures/base/`, `fixtures/p1..p6/` listed in the shared contract (except `base/snapshot.sha256`, Task 11)
- Test: `fixtures/generate_test.sh` (determinism: two runs, identical bytes) — plus the generator's own self-checks (P6 golden agreement, planted-count sanity) which abort with exit 1.

**Interfaces:**
- Consumes: nothing from the Go tree (import-pinned; Task 16 enforces).
- Produces: the fixture files; `python fixtures/generate.py --out <dir>` (default `fixtures`), exit 0 and prints `ok base_end_seq=<n> p6_end_seq=14`.
- Manifest schema (`fixtures/base/manifest.json`), consumed by Tasks 10–15 exactly as named:

```json
{"seed":20260831,"instruments":["AAA","BBB","CCC","DDD","EEE"],"end_seq":N,
 "viewpoints":{"V1":n1,"V2":n2,"V3":N},
 "action":{"action_id":"CA-0001","instrument":"AAA","seq":s,"amendment_seq":a,"original_ratio":2,"amended_ratio":3},
 "p1":{"duplicate":{"seq":d,"event_id":"ev-..","of_seq":o,"key":"<hex>"},
       "collision":{"seq":c,"event_id":"ev-..","key":"<hex>"},
       "twin":{"mutation":"naive_fold_no_dedupe","mutated_rows":2,
               "expected_violations":{"duplicate_absorbed":1,"collision_refused":1,"position_after_dedupe":K1}}},
 "p2":{"mutated":{"seq":m},"reordered":{"seqs":[s-1,s]},"tampered":{"seq":s,"break_at_seq":s+1},
       "twin":{"mutation":"mutate_reorder_tamper","mutated_rows":3,
               "expected_violations":{"snapshot_hash_diverges_mutated":1,"snapshot_hash_diverges_reordered":1,"chain_break_detected":1}}},
 "p3":{"twin":{"mutation":"leak_amended_terms_at_V2","mutated_rows":1,
               "expected_violations":{"viewpoint_V1":0,"viewpoint_V2":K3,"viewpoint_V3":0,"three_histories":0}}},
 "p4":{"withheld":["CCC"],"stale_instrument":"DDD","stale_from":"BBB",
       "twin":{"mutation":"silent_zero_and_stale_carry_forward","mutated_rows":2,
               "expected_violations":{"unevaluable_matches_planted":1,"silent_zero":1,"stale_carry_forward":1}}},
 "p5":{"drift":{"instrument":"AAA","field":"cost_basis","delta":D},
       "twin":{"mutation":"cost_basis_drift","mutated_rows":1,"expected_violations":{"field_mismatch":1}}},
 "p6":{"end_seq":14,
       "twin_fill":{"seq":2,"mutation":"fill_qty_plus_one","expected_violations":{"golden_match":K6a}},
       "twin_price":{"seq":12,"mutation":"price_plus_one","expected_violations":{"golden_match":3}}}}
```
`K1`, `K3`, `K6a` are computed by the generator by diffing its own honest vs broken output over the golden/expected leaves (the planter records its footprint). `K6a` must be > 0; the generator asserts `twin_price` = 3 exactly (`positions.AAA.valuation.price`, `positions.AAA.valuation.unrealized`, `unrealized_pnl`).

- [ ] **Step 1: Write the determinism test**

`fixtures/generate_test.sh`:
```sh
#!/bin/sh
# Two runs into fresh dirs must be byte-identical, and must equal the checked-in fixtures.
set -eu
cd "$(dirname "$0")/.."
rm -rf fixtures/.regen && mkdir -p fixtures/.regen/a fixtures/.regen/b
python fixtures/generate.py --out fixtures/.regen/a >/dev/null
python fixtures/generate.py --out fixtures/.regen/b >/dev/null
diff -r fixtures/.regen/a fixtures/.regen/b >/dev/null || { echo "FAIL generator not deterministic"; exit 1; }
for d in base p1 p2 p3 p4 p5 p6; do
  diff -r -x snapshot.sha256 "fixtures/.regen/a/$d" "fixtures/$d" >/dev/null || { echo "FAIL checked-in fixtures/$d stale: rerun python fixtures/generate.py"; exit 1; }
done
rm -rf fixtures/.regen
echo "ok fixtures deterministic and fresh"
```

- [ ] **Step 2: Run it to verify it fails**

Run: `sh fixtures/generate_test.sh` → fails (`generate.py` absent).

- [ ] **Step 3: Write the generator**

`fixtures/generate.py` (complete; keep stdout ASCII):
```python
#!/usr/bin/env python3
"""MERIDIAN fixture generator + naive fold.

stdlib only; imports nothing from the Go tree; reads nothing but its own
constants. It plants the ground truth (fills, actions, one amendment, every
twin defect) and embeds an independent naive fold that emits the custodian
statement (P5), per-viewpoint expectations (P1/P3) and every known-bad twin
artifact. The Go ledger must recover what this file planted.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
import random
import sys

GENESIS = "0" * 64
SEED = 20260831
INSTRUMENTS = ["AAA", "BBB", "CCC", "DDD", "EEE"]
WITHHELD = ["CCC"]          # P4: no price event ever emitted for these
START = dt.date(2026, 1, 5)


# ---------- canonical bytes ----------
def canon(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def sha(s):
    if isinstance(s, str):
        s = s.encode("ascii")
    return hashlib.sha256(s).hexdigest()


def rhe(n, d):
    quo, rem = divmod(n, d)
    if 2 * rem > d:
        return quo + 1
    if 2 * rem < d:
        return quo
    return quo if quo % 2 == 0 else quo + 1


# ---------- feed construction ----------
class Ev:
    __slots__ = ("type", "id", "effective", "payload")

    def __init__(self, type_, id_, effective, payload):
        self.type, self.id, self.effective, self.payload = type_, id_, effective, payload


def chain(events):
    """Assign seq 1..n and prev hashes; return (lines, records)."""
    lines, records, prev = [], [], GENESIS
    for i, e in enumerate(events, 1):
        rec = {"effective": e.effective, "id": e.id, "payload": e.payload, "prev": prev, "seq": i, "type": e.type}
        line = canon(rec)
        lines.append(line)
        records.append(dict(rec, line_hash=sha(line)))
        prev = sha(line)
    return lines, records


def write_lines(path, lines):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", newline="\n") as f:
        for ln in lines:
            f.write(ln + "\n")


def write_json(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", newline="\n") as f:
        f.write(canon(obj) + "\n")


def fill_key(p):
    return sha(canon({"trade_id": p["trade_id"], "venue": p["venue"]}))


# ---------- naive fold (independent of the Go tree) ----------
def naive_fold(records, up_to, mode="honest"):
    """Fold records[0:up_to] per the shared contract.

    mode: honest | nodedupe (P1 twin: apply every fill) | leak (P3 twin:
    resolve amendments from the WHOLE feed, not the visible prefix).
    Returns a full snapshot-schema dict.
    """
    vis = records[:up_to]
    src_terms = records if mode == "leak" else vis
    terms = {}
    for r in src_terms:
        if r["type"] == "action":
            terms[r["payload"]["action_id"]] = dict(r["payload"])
    for r in src_terms:
        if r["type"] == "action_amendment":
            t = terms[r["payload"]["action_id"]]
            for k in ("ratio", "rate"):
                if k in r["payload"]:
                    t[k] = r["payload"][k]
    absorbed, refusals, apps, seen = [], [], [], {}
    for r in vis:
        p = r["payload"]
        if r["type"] == "fill":
            key, ph = fill_key(p), sha(canon(p))
            if mode != "nodedupe" and key in seen:
                if seen[key] == ph:
                    absorbed.append({"event_id": r["id"], "key": key, "seq": r["seq"]})
                else:
                    refusals.append({"detail": "payload hash mismatch", "event_id": r["id"], "key": key, "kind": "collision", "seq": r["seq"]})
                continue
            seen[key] = ph
            apps.append((r["effective"], r["seq"], p["side"], p["instrument"], p["qty"], p["price"], 0, 0))
        elif r["type"] == "price":
            apps.append((r["effective"], r["seq"], "price", p["instrument"], 0, p["price"], 0, 0))
        elif r["type"] == "action":
            t = terms[p["action_id"]]
            apps.append((r["effective"], r["seq"], t["kind"], t["instrument"], 0, 0, t.get("ratio", 0), t.get("rate", 0)))
    apps.sort(key=lambda a: (a[0], a[1]))
    cash = realized = dividends = 0
    pos, last = {}, {}
    for eff, seq, kind, inst, qty, price, ratio, rate in apps:
        q, c = pos.get(inst, (0, 0))
        if kind == "buy":
            pos[inst] = (q + qty, c + qty * price)
            cash -= qty * price
        elif kind == "sell":
            if qty > q:
                refusals.append({"detail": "sell %d exceeds held %d" % (qty, q), "event_id": "", "key": "", "kind": "oversell", "seq": seq})
                continue
            rel = rhe(c * qty, q)
            q, c = q - qty, c - rel
            cash += qty * price
            realized += qty * price - rel
            if q == 0:
                assert c == 0
                del pos[inst]
            else:
                pos[inst] = (q, c)
        elif kind == "split":
            if inst in pos:
                pos[inst] = (q * ratio, c)
        elif kind == "dividend":
            if inst in pos and q > 0:
                cash += q * rate
                dividends += q * rate
        elif kind == "price":
            last[inst] = (price, seq)
    positions, unev, unreal = {}, [], 0
    for inst in sorted(pos):
        q, c = pos[inst]
        if inst in last:
            px, ps = last[inst]
            u = q * px - c
            unreal += u
            positions[inst] = {"qty": q, "total_cost": c, "valuation": {"price": px, "price_seq": ps, "unrealized": u}}
        else:
            positions[inst] = {"qty": q, "total_cost": c, "valuation": None}
            unev.append({"instrument": inst, "reason": "no_price_in_prefix"})
    refusals.sort(key=lambda r: r["seq"])
    prefix = ("sha256:" + records[up_to - 1]["line_hash"]) if up_to > 0 else "sha256:" + GENESIS
    return {"absorbed": absorbed, "cash": cash, "dividend_income": dividends, "feed_prefix_hash": prefix,
            "feed_seq": up_to, "positions": positions, "realized_pnl": realized, "refusals": refusals,
            "unevaluable": unev, "unrealized_pnl": unreal}


def expected_view(doc):
    return {"cash": doc["cash"], "dividend_income": doc["dividend_income"],
            "positions": {i: {"qty": p["qty"], "total_cost": p["total_cost"]} for i, p in doc["positions"].items()},
            "realized_pnl": doc["realized_pnl"], "feed_seq": doc["feed_seq"]}


def statement(doc):
    return {"as_of_seq": doc["feed_seq"], "cash": doc["cash"],
            "holdings": [{"cost_basis": p["total_cost"], "instrument": i, "quantity": p["qty"]} for i, p in sorted(doc["positions"].items())]}


def leaf_diff(golden, doc, path=""):
    """Count golden leaves the doc does not reproduce (same walk as Go snapshot.Diff)."""
    if isinstance(golden, dict):
        n = 0
        for k, v in golden.items():
            sub = doc.get(k) if isinstance(doc, dict) else None
            present = isinstance(doc, dict) and k in doc
            n += leaf_diff(v, sub, path + "." + k) if present else count_leaves(v)
        return n
    return 0 if canon(golden) == canon(doc) else 1


def count_leaves(v):
    return sum(count_leaves(x) for x in v.values()) if isinstance(v, dict) else 1


# ---------- base portfolio ----------
def build_base():
    rng = random.Random(SEED)
    evs, n = [], 0

    def nid():
        nonlocal n
        n += 1
        return "ev-%06d" % n

    tid = [0]

    def fill(day, inst, side, qty, price):
        tid[0] += 1
        return Ev("fill", nid(), day.isoformat(), {"instrument": inst, "price": price, "qty": qty, "side": side, "trade_id": "T-%06d" % tid[0], "venue": "X"})

    held = {i: 0 for i in INSTRUMENTS}   # feasibility under ORIGINAL split terms
    marks = {}
    day0 = START
    for inst in INSTRUMENTS:             # day 0: every instrument opens a position
        q, px = rng.randint(20, 80), rng.randint(500, 3000)
        evs.append(fill(day0, inst, "buy", q, px)); held[inst] += q
    for d in range(1, 40):
        day = day0 + dt.timedelta(days=d)
        if d == 12:                       # deterministic AAA buy right before the split (P2 reorder twin needs it)
            evs.append(fill(day, "AAA", "buy", 10, 1500)); held["AAA"] += 10
            marks["reorder_buy_index"] = len(evs) - 1
            evs.append(Ev("action", nid(), day.isoformat(), {"action_id": "CA-0001", "announced": (day - dt.timedelta(days=2)).isoformat(), "instrument": "AAA", "kind": "split", "processed": day.isoformat(), "ratio": 2}))
            marks["split_index"] = len(evs) - 1
            held["AAA"] *= 2
            continue
        if d == 14:                       # deterministic AAA sell after the split, before the amendment (makes the leak visible)
            evs.append(fill(day, "AAA", "sell", 30, 1400)); held["AAA"] -= 30
            continue
        if d == 18:
            evs.append(Ev("action", nid(), day.isoformat(), {"action_id": "CA-0002", "announced": (day - dt.timedelta(days=3)).isoformat(), "instrument": "BBB", "kind": "dividend", "processed": day.isoformat(), "rate": 25}))
            continue
        if d == 25:
            evs.append(Ev("action_amendment", nid(), (day0 + dt.timedelta(days=12)).isoformat(), {"action_id": "CA-0001", "ratio": 3}))
            marks["amendment_index"] = len(evs) - 1
            continue
        if d == 30:                       # P1 duplicate: exact copy of the first fill, new event id
            src = evs[0]
            evs.append(Ev("fill", nid(), src.effective, dict(src.payload)))
            marks["dup_index"] = len(evs) - 1
            continue
        if d == 31:                       # P1 collision: same identity, qty + 1
            src = evs[1]
            pl = dict(src.payload); pl["qty"] = pl["qty"] + 1
            evs.append(Ev("fill", nid(), src.effective, pl))
            marks["collision_index"] = len(evs) - 1
            continue
        if d in (10, 20, 35, 39):
            for inst in INSTRUMENTS:
                if inst not in WITHHELD:
                    evs.append(Ev("price", nid(), day.isoformat(), {"instrument": inst, "price": rng.randint(500, 3000)}))
            continue
        for _ in range(rng.randint(0, 3)):
            inst = rng.choice(INSTRUMENTS)
            if rng.random() < 0.6 or held[inst] < 2:
                q = rng.randint(1, 40)
                evs.append(fill(day, inst, "buy", q, rng.randint(500, 3000))); held[inst] += q
            else:
                q = rng.randint(1, held[inst] // 2)
                evs.append(fill(day, inst, "sell", q, rng.randint(500, 3000))); held[inst] -= q
    return evs, marks


def build_p6():
    def f(day, inst, side, q, px, t):
        return Ev("fill", "p6-%02d" % t, day, {"instrument": inst, "price": px, "qty": q, "side": side, "trade_id": "T-%d" % t, "venue": "X"})
    return [
        f("2026-01-05", "AAA", "buy", 100, 1000, 1),
        f("2026-01-06", "AAA", "buy", 50, 1301, 2),
        f("2026-01-07", "BBB", "buy", 1, 500, 3),
        f("2026-01-07", "BBB", "buy", 1, 503, 4),
        f("2026-01-08", "CCC", "buy", 1, 500, 5),
        f("2026-01-08", "CCC", "buy", 1, 501, 6),
        Ev("action", "p6-07", "2026-01-10", {"action_id": "CA-1", "announced": "2026-01-08", "instrument": "AAA", "kind": "split", "processed": "2026-01-10", "ratio": 2}),
        f("2026-01-12", "AAA", "sell", 120, 700, 7),
        f("2026-01-13", "BBB", "sell", 1, 600, 8),
        f("2026-01-13", "CCC", "sell", 1, 600, 9),
        Ev("action", "p6-11", "2026-01-15", {"action_id": "CA-2", "announced": "2026-01-13", "instrument": "AAA", "kind": "dividend", "processed": "2026-01-15", "rate": 25}),
        Ev("price", "p6-12", "2026-01-16", {"instrument": "AAA", "price": 800}),
        Ev("price", "p6-13", "2026-01-16", {"instrument": "BBB", "price": 700}),
        Ev("price", "p6-14", "2026-01-16", {"instrument": "CCC", "price": 700}),
    ]


# Hand-computed. AAA: 100@1000 + 50@1301 = cost 165050; split 2 -> 300 sh;
# sell 120@700: relieved rhe(165050*120/300) = 66020 exact, cost 99030, qty 180,
# realized 84000-66020 = 17980; dividend 180*25 = 4500; price 800 ->
# unrealized 144000-99030 = 44970. BBB: cost 1003, sell 1@600 relieved 502
# (501.5 half-even up), cost 501, realized 98, price 700 -> 199. CCC: cost
# 1001, relieved 500 (500.5 half-even down), cost 501, realized 100, 199.
# cash = -76550 - 403 - 401 = -77354.
P6_GOLDEN = {
    "cash": -77354, "dividend_income": 4500, "realized_pnl": 18178, "unrealized_pnl": 45368,
    "positions": {
        "AAA": {"qty": 180, "total_cost": 99030, "valuation": {"price": 800, "unrealized": 44970}},
        "BBB": {"qty": 1, "total_cost": 501, "valuation": {"price": 700, "unrealized": 199}},
        "CCC": {"qty": 1, "total_cost": 501, "valuation": {"price": 700, "unrealized": 199}},
    },
}


def die(msg):
    print("FAIL " + msg)
    sys.exit(1)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=os.path.dirname(os.path.abspath(__file__)))
    out = ap.parse_args().out
    P = lambda *parts: os.path.join(out, *parts)

    # ----- base -----
    evs, marks = build_base()
    lines, recs = chain(evs)
    N = len(recs)
    split_seq = marks["split_index"] + 1
    amend_seq = marks["amendment_index"] + 1
    V1, V2, V3 = split_seq - 1, amend_seq - 1, N
    write_lines(P("base", "feed.jsonl"), lines)
    honest = {v: naive_fold(recs, v) for v in (V1, V2, V3)}
    for name, v in (("V1", V1), ("V2", V2), ("V3", V3)):
        write_json(P("base", "expected", name + ".json"), expected_view(honest[v]))
    write_json(P("base", "statement.json"), statement(honest[V3]))
    q1, q2, q3 = (honest[v]["positions"]["AAA"]["qty"] for v in (V1, V2, V3))
    if len({q1, q2, q3}) != 3:
        die("three viewpoints must give three AAA quantities: %d %d %d" % (q1, q2, q3))
    if [u["instrument"] for u in honest[V3]["unevaluable"]] != WITHHELD:
        die("withheld set mismatch: %r" % honest[V3]["unevaluable"])
    dup, col = recs[marks["dup_index"]], recs[marks["collision_index"]]
    if not any(a["seq"] == dup["seq"] for a in honest[V3]["absorbed"]):
        die("planted duplicate not absorbed by naive fold")
    if not any(r["seq"] == col["seq"] and r["kind"] == "collision" for r in honest[V3]["refusals"]):
        die("planted collision not refused by naive fold")

    # ----- P1 twin: no-dedupe snapshot -----
    p1twin = naive_fold(recs, V3, mode="nodedupe")
    k1 = leaf_diff(expected_view(honest[V3]), p1twin)
    if k1 == 0:
        die("P1 twin has no footprint")
    write_json(P("p1", "twin", "snapshot.json"), p1twin)

    # ----- P2 twins -----
    mut = [Ev(e.type, e.id, e.effective, dict(e.payload)) for e in evs]
    mi = next(i for i, e in enumerate(mut) if e.type == "fill")
    mut[mi].payload["price"] += 1
    write_lines(P("p2", "mutated", "feed.jsonl"), chain(mut)[0])
    reo = list(evs)
    bi, si = marks["reorder_buy_index"], marks["split_index"]
    reo[bi], reo[si] = reo[si], reo[bi]
    write_lines(P("p2", "reordered", "feed.jsonl"), chain(reo)[0])
    tam = list(lines)
    if '"ratio":2' not in tam[si]:
        die("tamper target not found")
    tam[si] = tam[si].replace('"ratio":2', '"ratio":3', 1)
    write_lines(P("p2", "tampered", "feed.jsonl"), tam)

    # ----- P3 twin: leak -----
    leak = naive_fold(recs, V2, mode="leak")
    k3 = leaf_diff(expected_view(honest[V2]), leak)
    if k3 == 0:
        die("P3 leak twin has no footprint")
    write_json(P("p3", "twin", "V2.json"), leak)

    # ----- P4 twin: silent zero + stale carry-forward -----
    p4 = json.loads(canon(honest[V3]))
    c = p4["positions"]["CCC"]
    c["valuation"] = {"price": 0, "price_seq": 0, "unrealized": -c["total_cost"]}
    p4["unevaluable"] = [u for u in p4["unevaluable"] if u["instrument"] != "CCC"]
    bbb = p4["positions"]["BBB"]["valuation"]
    ddd = p4["positions"]["DDD"]
    ddd["valuation"] = {"price": bbb["price"], "price_seq": bbb["price_seq"], "unrealized": ddd["qty"] * bbb["price"] - ddd["total_cost"]}
    p4["unrealized_pnl"] = sum(p["valuation"]["unrealized"] for p in p4["positions"].values() if p["valuation"])
    write_json(P("p4", "twin", "snapshot.json"), p4)

    # ----- P5 twin: drift -----
    st = statement(honest[V3])
    aaa = next(h for h in st["holdings"] if h["instrument"] == "AAA")
    drift = max(1, aaa["cost_basis"] // 10000)
    aaa["cost_basis"] += drift
    write_json(P("p5", "twin", "statement.json"), st)

    # ----- P6 -----
    p6 = build_p6()
    l6, r6 = chain(p6)
    write_lines(P("p6", "feed.jsonl"), l6)
    if leaf_diff(P6_GOLDEN, naive_fold(r6, len(r6))) != 0:
        die("naive fold disagrees with the hand-computed P6 golden")
    write_json(P("p6", "golden.json"), P6_GOLDEN)
    tf = [Ev(e.type, e.id, e.effective, dict(e.payload)) for e in p6]
    tf[1].payload["qty"] = 51
    lf, rf = chain(tf)
    k6a = leaf_diff(P6_GOLDEN, naive_fold(rf, len(rf)))
    if k6a == 0:
        die("P6 fill twin has no footprint")
    write_lines(P("p6", "twin-fill", "feed.jsonl"), lf)
    tp = [Ev(e.type, e.id, e.effective, dict(e.payload)) for e in p6]
    tp[11].payload["price"] = 801
    lp, rp = chain(tp)
    k6b = leaf_diff(P6_GOLDEN, naive_fold(rp, len(rp)))
    if k6b != 3:
        die("P6 price twin footprint must be exactly 3, got %d" % k6b)
    write_lines(P("p6", "twin-price", "feed.jsonl"), lp)

    # ----- manifest -----
    man = {
        "seed": SEED, "instruments": INSTRUMENTS, "end_seq": N,
        "viewpoints": {"V1": V1, "V2": V2, "V3": V3},
        "action": {"action_id": "CA-0001", "instrument": "AAA", "seq": split_seq, "amendment_seq": amend_seq, "original_ratio": 2, "amended_ratio": 3},
        "p1": {"duplicate": {"seq": dup["seq"], "event_id": dup["id"], "of_seq": 1, "key": fill_key(dup["payload"])},
               "collision": {"seq": col["seq"], "event_id": col["id"], "key": fill_key(col["payload"])},
               "twin": {"mutation": "naive_fold_no_dedupe", "mutated_rows": 2,
                        "expected_violations": {"duplicate_absorbed": 1, "collision_refused": 1, "position_after_dedupe": k1}}},
        "p2": {"mutated": {"seq": mi + 1}, "reordered": {"seqs": [bi + 1, si + 1]}, "tampered": {"seq": si + 1, "break_at_seq": si + 2},
               "twin": {"mutation": "mutate_reorder_tamper", "mutated_rows": 3,
                        "expected_violations": {"snapshot_hash_diverges_mutated": 1, "snapshot_hash_diverges_reordered": 1, "chain_break_detected": 1}}},
        "p3": {"twin": {"mutation": "leak_amended_terms_at_V2", "mutated_rows": 1,
                        "expected_violations": {"viewpoint_V1": 0, "viewpoint_V2": k3, "viewpoint_V3": 0, "three_histories": 0}}},
        "p4": {"withheld": WITHHELD, "stale_instrument": "DDD", "stale_from": "BBB",
               "twin": {"mutation": "silent_zero_and_stale_carry_forward", "mutated_rows": 2,
                        "expected_violations": {"unevaluable_matches_planted": 1, "silent_zero": 1, "stale_carry_forward": 1}}},
        "p5": {"drift": {"instrument": "AAA", "field": "cost_basis", "delta": drift},
               "twin": {"mutation": "cost_basis_drift", "mutated_rows": 1, "expected_violations": {"field_mismatch": 1}}},
        "p6": {"end_seq": len(r6),
               "twin_fill": {"seq": 2, "mutation": "fill_qty_plus_one", "expected_violations": {"golden_match": k6a}},
               "twin_price": {"seq": 12, "mutation": "price_plus_one", "expected_violations": {"golden_match": 3}}},
    }
    write_json(P("base", "manifest.json"), man)
    print("ok base_end_seq=%d p6_end_seq=%d" % (N, len(r6)))


if __name__ == "__main__":
    main()
```

- [ ] **Step 4: Generate, then run the determinism test**

Run: `python fixtures/generate.py && sh fixtures/generate_test.sh`
Expected: `ok base_end_seq=<N> p6_end_seq=14` then `ok fixtures deterministic and fresh`. If the generator dies with a `FAIL` line, the planted structure is wrong — fix the generator, never the assertion. If `P("p4")` fails because `DDD` or `BBB` has no position at the end (all sold), the seeded stream sold it out — sells are capped at `held // 2` so this cannot happen from the random path; check `held` bookkeeping.

Sanity check by hand (must hold or the fixture is wrong): `grep -c '"type":"fill"' fixtures/base/feed.jsonl` is between 40 and 130; `grep -c '"type":"price"' fixtures/base/feed.jsonl` = 16 (4 days × 4 non-withheld instruments); `grep -c CCC fixtures/base/feed.jsonl | ` shows CCC fills only, never a CCC price: `grep '"type":"price"' fixtures/base/feed.jsonl | grep -c CCC` → 0.

- [ ] **Step 5: Commit for the operator**

```bash
cd ~/dev/meridian
git add fixtures
git commit -m "feat: fixture generator with import-pinned naive fold and planted twins"
```

---

### Task 10: Gate harness — manifest loader, verdict emission, P1

**Files:**
- Create: `gates/manifest.go`, `gates/verdict.go`, `gates/p1_test.go`
- Test: `gates/verdict_test.go`

**Interfaces:**
- Consumes: `asof.Read`, `snapshot.Decode/Diff/Leaves`, `feed.Open`, `canon.*`.
- Produces (package `gates`, all tests in this package use these):
  ```go
  const FixturesDir = "../fixtures"
  type Manifest map[string]any
  func LoadManifest(t *testing.T) Manifest
  func (m Manifest) Int(path ...string) int64        // t.Fatal-free: panics on missing (tests must load real manifest)
  func (m Manifest) Str(path ...string) string
  func (m Manifest) Strs(path ...string) []string
  func (m Manifest) Ints(path ...string) []int64
  func (m Manifest) Planted(prop string, twinKey ...string) Planted // twinKey defaults to "twin"; reads mutation, mutated_rows, expected_violations
  type Planted struct{ Mutation string; MutatedRows int64; ExpectedViolations map[string]int64 }
  type Counts struct{ Checks, Evaluated map[string]int64 }
  func NewCounts(names ...string) Counts               // zeroed maps for every named check
  type Row struct{ Prop int; Cell, Scope, ContentHash, Basis string; Rows int64; Params map[string]any; Counts Counts; Planted *Planted }
  func Emit(t *testing.T, r Row)                       // writes verdict JSON; live: t.Fatal if any check != 0; twin: t.Fatal unless every check == expected and at least one > 0
  func LoadDoc(t *testing.T, path string) snapshot.Doc // decode a snapshot-schema JSON file
  func ReadFixture(t *testing.T, rel string, seq int64) asof.Result // asof.Read over FixturesDir/rel
  ```
- Verdict file path: `$MERIDIAN_VERDICT_DIR` (default `out`) `/meridian-lane1-p<N>-<cell>-<YYYYMMDDTHHMMSS.ffffffZ>.json`. Row keys exactly as the shared contract; `parallax_sha` from `git rev-parse HEAD`, `parallax_worktree` = `clean` if `git status --porcelain` is empty else `dirty`; git failure ⇒ `t.Fatal` (a row with an unknown sha is a lie). `runner` = `$MERIDIAN_RUNNER` or `local`.

- [ ] **Step 1: Write the failing test**

`gates/verdict_test.go`:
```go
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
	// A twin whose violations do not equal the planted counts must fail the test that emits it.
	ft := &fakeT{T: t}
	c := NewCounts("a")
	c.Checks["a"] = 2
	emitWith(ft, Row{Prop: 9, Cell: "twin", Counts: c, Params: map[string]any{}, Planted: &Planted{ExpectedViolations: map[string]int64{"a": 1}}})
	if !ft.failed {
		t.Fatal("twin with wrong counts must fail")
	}
	ft = &fakeT{T: t}
	c = NewCounts("a")
	c.Checks["a"] = 1
	emitWith(ft, Row{Prop: 9, Cell: "live", Counts: c, Params: map[string]any{}})
	if !ft.failed {
		t.Fatal("live with violations must fail")
	}
	ft = &fakeT{T: t}
	c = NewCounts("a")
	emitWith(ft, Row{Prop: 9, Cell: "twin", Counts: c, Params: map[string]any{}, Planted: &Planted{ExpectedViolations: map[string]int64{"a": 0}}})
	if !ft.failed {
		t.Fatal("twin that never goes red must fail")
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
```

- [ ] **Step 2: Run** `go test ./gates/ -run 'Emit|Manifest' -v` → FAIL (undefined).

- [ ] **Step 3: Implement**

`gates/manifest.go`:
```go
// Package gates holds the property gates. Each p<N>_test.go runs its live
// cell (must be GREEN) and its twin cell (must be RED with exactly the
// planted counts) and emits BASELINE-schema verdict rows.
package gates

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
```

`gates/verdict.go`:
```go
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
	failed bool
}

func (f *fakeT) Fatalf(format string, a ...any) { f.failed = true }

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
	result := "GREEN"
	names := make([]string, 0, len(r.Counts.Checks))
	for k, v := range r.Counts.Checks {
		names = append(names, k)
		if v != 0 {
			result = "RED"
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
		for k, want := range r.Planted.ExpectedViolations {
			if got := r.Counts.Checks[k]; got != want {
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
	b, _ := json.MarshalIndent(row, "", "  ")
	name := fmt.Sprintf("meridian-lane1-p%d-%s-%s.json", r.Prop, r.Cell, now.Format("20060102T150405.000000Z"))
	if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
}
```

- [ ] **Step 4: Run** `go test ./gates/ -run 'Emit|Manifest' -v` → PASS (3 tests; `TestManifestLoads` needs Task 9's fixtures present).

- [ ] **Step 5: Write P1's gate (failing first)**

`gates/p1_test.go`:
```go
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
	// Belt and braces: the absorbed record must carry the ledger-derived key.
	if a := live.Doc["absorbed"].([]any); len(a) == 1 {
		if a[0].(map[string]any)["key"] != m.Str("p1", "duplicate", "key") {
			c.Checks["duplicate_absorbed"]++
		}
	}
	Emit(t, Row{Prop: 1, Cell: "live", Scope: "fixtures/base end-of-feed snapshot", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: params, Counts: c})

	twin := LoadDoc(t, filepath.Join(FixturesDir, "p1", "twin", "snapshot.json"))
	ct := p1Check(twin, expected, dupID, colID)
	raw, _ := json.Marshal(twin)
	Emit(t, Row{Prop: 1, Cell: "twin", Scope: "naive no-dedupe snapshot over fixtures/base", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the twin document as decoded and re-marshaled", Rows: int64(len(twin["positions"].(map[string]any))), Params: params, Counts: ct, Planted: ptr(m.Planted("p1"))})
}
```
Add to `gates/manifest.go` (bottom):
```go
func ptr(p Planted) *Planted { return &p }

func sha256Hex(b []byte) string { return canon.SHA256Hex(b) }
```

- [ ] **Step 6: Run** `go test ./gates/ -run P1 -v` → PASS; `ls gates/out/` shows `meridian-lane1-p1-live-*.json` and `-twin-`. If the live cell fails on `position_after_dedupe`, the Go fold and the naive fold disagree — that is a real defect in one of them; diff `expected/V3.json` against `meridian asof --feed fixtures/base/feed.jsonl --seq <end>` and fix the wrong side per the shared contract (do not "fix" by editing expectations).

- [ ] **Step 7: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/manifest.go gates/verdict.go gates/verdict_test.go gates/p1_test.go
git commit -m "feat: gate harness with BASELINE verdict rows; P1 live+twin"
```

---

### Task 11: P2 — deterministic replay (+ pin the snapshot hash)

**Files:**
- Create: `gates/p2_test.go`, `fixtures/base/snapshot.sha256`

- [ ] **Step 1: Write the gate**

`gates/p2_test.go`:
```go
package gates

import (
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
	c.Evaluated["fresh_process_identical"], c.Evaluated["pinned_hash_match"], c.Evaluated["chain_verifies"] = 2, 1, m.Int("end_seq")
	if h1 != h2 || string(b1) != string(b2) {
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

	ct := NewCounts("snapshot_hash_diverges_mutated", "snapshot_hash_diverges_reordered", "chain_break_detected")
	ct.Evaluated["snapshot_hash_diverges_mutated"], ct.Evaluated["snapshot_hash_diverges_reordered"], ct.Evaluated["chain_break_detected"] = 1, 1, 1
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
	Emit(t, Row{Prop: 2, Cell: "twin", Scope: "fixtures/p2 mutated, reordered, tampered feeds vs the pin", ContentHash: pin,
		Basis: "pinned sha256 of the base snapshot the twins must diverge from", Rows: 3, Params: tp, Counts: ct, Planted: ptr(m.Planted("p2"))})
}
```

- [ ] **Step 2: Run** `go test ./gates/ -run P2 -v` → FAIL: pin missing.

- [ ] **Step 3: Pin the hash**

Run: `go run ./cmd/meridian snapshot --feed fixtures/base/feed.jsonl --out fixtures/.regen/pin` → prints `sha256:<hex> <path>`. Write **only the hash** (with the `sha256:` prefix, one line, LF) to `fixtures/base/snapshot.sha256`; delete `fixtures/.regen`. Cross-check: `sha256sum <path>` (or `certutil -hashfile <path> SHA256` on Windows) equals the hex.

- [ ] **Step 4: Run** `go test ./gates/ -run P2 -v` → PASS. The reordered twin **must** diverge: if `snapshot_hash_diverges_reordered` is 0, the swapped pair commuted — check that the buy before the split is on `AAA` (manifest `reordered.seqs`) and fix the generator.

- [ ] **Step 5: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/p2_test.go fixtures/base/snapshot.sha256
git commit -m "feat: P2 deterministic replay gate + pinned base snapshot hash"
```

---

### Task 12: P3 — point-in-time corporate actions

**Files:**
- Create: `gates/p3_test.go`

- [ ] **Step 1: Write the gate**

`gates/p3_test.go`:
```go
package gates

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/snapshot"
)

func p3Check(docs map[string]snapshot.Doc, expected map[string]snapshot.Doc) Counts {
	c := NewCounts("viewpoint_V1", "viewpoint_V2", "viewpoint_V3", "three_histories")
	qtys := map[string]bool{}
	for _, v := range []string{"V1", "V2", "V3"} {
		ms := snapshot.Diff(expected[v], docs[v])
		c.Evaluated["viewpoint_"+v] = int64(snapshot.Leaves(expected[v]))
		c.Checks["viewpoint_"+v] = int64(len(ms))
		pos, _ := docs[v]["positions"].(map[string]any)
		aaa, _ := pos["AAA"].(map[string]any)
		qtys[string(aaa["qty"].(json.Number))] = true
	}
	c.Evaluated["three_histories"] = 1
	if len(qtys) != 3 {
		c.Checks["three_histories"] = 1
	}
	return c
}

func TestP3PointInTimeActions(t *testing.T) {
	m := LoadManifest(t)
	vp := map[string]int64{"V1": m.Int("viewpoints", "V1"), "V2": m.Int("viewpoints", "V2"), "V3": m.Int("viewpoints", "V3")}
	expected, docs := map[string]snapshot.Doc{}, map[string]snapshot.Doc{}
	var hashV3 string
	for v, seq := range vp {
		expected[v] = LoadDoc(t, filepath.Join(FixturesDir, "base", "expected", v+".json"))
		r := ReadFixture(t, "base/feed.jsonl", seq)
		docs[v] = r.Doc
		if v == "V3" {
			hashV3 = r.Hash
		}
	}
	params := map[string]any{"viewpoints": vp, "action_seq": m.Int("action", "seq"), "amendment_seq": m.Int("action", "amendment_seq"),
		"original_ratio": m.Int("action", "original_ratio"), "amended_ratio": m.Int("action", "amended_ratio")}
	c := p3Check(docs, expected)
	Emit(t, Row{Prop: 3, Cell: "live", Scope: "fixtures/base at three viewpoints around the amended split", ContentHash: hashV3,
		Basis: "sha256 of canonical snapshot bytes at V3", Rows: 3, Params: params, Counts: c})

	leaked := map[string]snapshot.Doc{"V1": docs["V1"], "V2": LoadDoc(t, filepath.Join(FixturesDir, "p3", "twin", "V2.json")), "V3": docs["V3"]}
	ct := p3Check(leaked, expected)
	raw, _ := json.Marshal(leaked["V2"])
	Emit(t, Row{Prop: 3, Cell: "twin", Scope: "V2 replaced by a snapshot that leaked the amended terms", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the leaked V2 document as decoded and re-marshaled", Rows: 3, Params: params, Counts: ct, Planted: ptr(m.Planted("p3"))})
}
```

- [ ] **Step 2: Run** `go test ./gates/ -run P3 -v` → PASS. A live failure on `viewpoint_V2` or `viewpoint_V3` means the Go fold and the naive fold disagree on amendment application; reread the shared contract's step 1 and Decision 6 and fix the wrong side. `three_histories` failing means the fixture's AAA quantities coincide — the generator's self-check should have caught this.

- [ ] **Step 3: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/p3_test.go
git commit -m "feat: P3 point-in-time corporate actions gate"
```

---

### Task 13: P4 — fail-closed valuation

**Files:**
- Create: `gates/p4_test.go`

- [ ] **Step 1: Write the gate**

`gates/p4_test.go`:
```go
package gates

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

// priceIndex maps "instrument|seq" -> price for every price event in [1..upTo].
func priceIndex(t *testing.T, records []feed.Record, upTo int64) map[string]string {
	idx := map[string]string{}
	for _, r := range records {
		if r.Seq > upTo || r.Type != "price" {
			continue
		}
		idx[r.Payload["instrument"].(string)+"|"+strconv.FormatInt(r.Seq, 10)] = string(r.Payload["price"].(json.Number))
	}
	return idx
}

func p4Check(doc snapshot.Doc, prices map[string]string, withheld []string) Counts {
	c := NewCounts("unevaluable_matches_planted", "silent_zero", "stale_carry_forward")
	got := []string{}
	for _, u := range doc["unevaluable"].([]any) {
		got = append(got, u.(map[string]any)["instrument"].(string))
	}
	sort.Strings(got)
	want := append([]string{}, withheld...)
	sort.Strings(want)
	c.Evaluated["unevaluable_matches_planted"] = int64(len(want))
	c.Checks["unevaluable_matches_planted"] = int64(symDiff(got, want))
	pos := doc["positions"].(map[string]any)
	for inst, p := range pos {
		val, _ := p.(map[string]any)["valuation"].(map[string]any)
		if val == nil {
			continue
		}
		c.Evaluated["silent_zero"]++
		c.Evaluated["stale_carry_forward"]++
		price, seq := string(val["price"].(json.Number)), string(val["price_seq"].(json.Number))
		if price == "0" || seq == "0" {
			c.Checks["silent_zero"]++
			continue
		}
		if prices[inst+"|"+seq] != price {
			c.Checks["stale_carry_forward"]++
		}
	}
	return c
}

func symDiff(a, b []string) int {
	set := map[string]int{}
	for _, x := range a {
		set[x]++
	}
	for _, x := range b {
		set[x]--
	}
	n := 0
	for _, v := range set {
		if v != 0 {
			n++
		}
	}
	return n
}

func TestP4FailClosedValuation(t *testing.T) {
	m := LoadManifest(t)
	withheld := m.Strs("p4", "withheld")
	f, err := feed.Open(filepath.Join(FixturesDir, "base", "feed.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	records := f.Records()
	f.Close()
	end := m.Int("end_seq")
	prices := priceIndex(t, records, end)
	params := map[string]any{"withheld": withheld, "viewpoint": end}

	live := ReadFixture(t, "base/feed.jsonl", -1)
	c := p4Check(live.Doc, prices, withheld)
	Emit(t, Row{Prop: 4, Cell: "live", Scope: "fixtures/base end-of-feed snapshot; CCC price withheld by the generator", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: params, Counts: c})

	twin := LoadDoc(t, filepath.Join(FixturesDir, "p4", "twin", "snapshot.json"))
	ct := p4Check(twin, prices, withheld)
	raw, _ := json.Marshal(twin)
	Emit(t, Row{Prop: 4, Cell: "twin", Scope: "snapshot with a silent zero (CCC) and a stale carry-forward (DDD from BBB)", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the twin document as decoded and re-marshaled", Rows: int64(len(twin["positions"].(map[string]any))), Params: params, Counts: ct, Planted: ptr(m.Planted("p4"))})
}
```

- [ ] **Step 2: Run** `go test ./gates/ -run P4 -v` → PASS. If live `stale_carry_forward` > 0, the fold's `PriceSeq` does not point at the price event it used — fix the fold.

- [ ] **Step 3: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/p4_test.go
git commit -m "feat: P4 fail-closed valuation gate"
```

---

### Task 14: P5 — reconciliation proven able to fail

**Files:**
- Create: `gates/p5_test.go`

- [ ] **Step 1: Write the gate**

`gates/p5_test.go`:
```go
package gates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/reconcile"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

func p5Check(t *testing.T, doc snapshot.Doc, statementPath string) (Counts, []reconcile.Mismatch) {
	st, err := reconcile.LoadStatement(statementPath)
	if err != nil {
		t.Fatal(err)
	}
	ms, compared := reconcile.Reconcile(doc, st)
	c := NewCounts("field_mismatch")
	c.Evaluated["field_mismatch"] = int64(compared)
	c.Checks["field_mismatch"] = int64(len(ms))
	return c, ms
}

func TestP5ReconciliationCanFail(t *testing.T) {
	m := LoadManifest(t)
	live := ReadFixture(t, "base/feed.jsonl", -1)
	params := map[string]any{"viewpoint": m.Int("end_seq")}
	c, _ := p5Check(t, live.Doc, filepath.Join(FixturesDir, "base", "statement.json"))
	Emit(t, Row{Prop: 5, Cell: "live", Scope: "fixtures/base snapshot vs naive-fold custodian statement, field by field", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: params, Counts: c})

	twinPath := filepath.Join(FixturesDir, "p5", "twin", "statement.json")
	ct, ms := p5Check(t, live.Doc, twinPath)
	// The gate must NAME the instrument and the amount, not just count.
	if len(ms) == 1 {
		if ms[0].Instrument != m.Str("p5", "drift", "instrument") || ms[0].Field != m.Str("p5", "drift", "field") || ms[0].Delta != -m.Int("p5", "drift", "delta") {
			t.Fatalf("drift not named correctly: %+v", ms[0])
		}
	}
	raw, _ := os.ReadFile(twinPath)
	tp := map[string]any{"viewpoint": m.Int("end_seq"), "drift_instrument": m.Str("p5", "drift", "instrument"), "drift_delta": m.Int("p5", "drift", "delta")}
	Emit(t, Row{Prop: 5, Cell: "twin", Scope: "same snapshot vs a statement with one planted cost_basis drift", ContentHash: "sha256:" + sha256Hex(raw),
		Basis: "sha256 of the twin statement file bytes", Rows: int64(len(live.State.Positions)), Params: tp, Counts: ct, Planted: ptr(m.Planted("p5"))})
}
```

- [ ] **Step 2: Run** `go test ./gates/ -run P5 -v` → PASS.

- [ ] **Step 3: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/p5_test.go
git commit -m "feat: P5 reconciliation gate with named drift"
```

---

### Task 15: P6 — portfolio math against the hand golden

**Files:**
- Create: `gates/p6_test.go`

- [ ] **Step 1: Write the gate**

`gates/p6_test.go`:
```go
package gates

import (
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/snapshot"
)

func p6Check(doc, golden snapshot.Doc) (Counts, []snapshot.Mismatch) {
	ms := snapshot.Diff(golden, doc)
	c := NewCounts("golden_match")
	c.Evaluated["golden_match"] = int64(snapshot.Leaves(golden))
	c.Checks["golden_match"] = int64(len(ms))
	return c, ms
}

func TestP6PortfolioMath(t *testing.T) {
	m := LoadManifest(t)
	golden := LoadDoc(t, filepath.Join(FixturesDir, "p6", "golden.json"))
	live := ReadFixture(t, "p6/feed.jsonl", -1)
	c, _ := p6Check(live.Doc, golden)
	params := map[string]any{"viewpoint": m.Int("p6", "end_seq"), "golden": "fixtures/p6/golden.json (hand-computed)"}
	Emit(t, Row{Prop: 6, Cell: "live", Scope: "fixtures/p6 hand-scripted portfolio vs hand-computed golden, to the minor unit", ContentHash: live.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(live.State.Positions)), Params: params, Counts: c})

	fill := ReadFixture(t, "p6/twin-fill/feed.jsonl", -1)
	cf, _ := p6Check(fill.Doc, golden)
	Emit(t, Row{Prop: 6, Cell: "twin", Scope: "fixtures/p6 with one fill quantity mutated", ContentHash: fill.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(fill.State.Positions)), Params: map[string]any{"mutated_seq": m.Int("p6", "twin_fill", "seq")},
		Counts: cf, Planted: ptr(m.Planted("p6", "twin_fill"))})

	price := ReadFixture(t, "p6/twin-price/feed.jsonl", -1)
	cp, ms := p6Check(price.Doc, golden)
	// Exactly the dependent unrealized fields move; everything else byte-stable.
	for _, x := range ms {
		switch x.Path {
		case "positions.AAA.valuation.price", "positions.AAA.valuation.unrealized", "unrealized_pnl":
		default:
			t.Fatalf("price perturbation moved an unrelated field: %+v", x)
		}
	}
	Emit(t, Row{Prop: 6, Cell: "twin", Scope: "fixtures/p6 with one price perturbed by one minor unit", ContentHash: price.Hash,
		Basis: "sha256 of canonical snapshot bytes", Rows: int64(len(price.State.Positions)), Params: map[string]any{"mutated_seq": m.Int("p6", "twin_price", "seq")},
		Counts: cp, Planted: ptr(m.Planted("p6", "twin_price"))})
}
```
Note `Manifest.Planted("p6", "twin_fill")` reads `p6.twin_fill` — it has `mutation` and `expected_violations`; `mutated_rows` is absent and stays 0. Add `"mutated_rows": 1` to both p6 twin blocks in the generator's manifest for completeness (regenerate fixtures; the P6 twin rows then carry `mutated_rows: 1`).

- [ ] **Step 2: Run** `go test ./gates/ -run P6 -v` → PASS. P6 emits **two** twin rows (`twin_fill`, `twin_price`); both files will exist in `gates/out/` with distinct `ran_at`. The claimability table (Task 17) requires *every* twin row of a property to be RED-as-planted.

- [ ] **Step 3: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/p6_test.go fixtures
git commit -m "feat: P6 portfolio math gate against the hand golden"
```

---

### Task 16: Import-pin gate for the naive fold

**Files:**
- Create: `gates/importpin.py`

**Interfaces:**
- Produces: `python gates/importpin.py [path]` → exit 0 and `ok import-pin <path>` when every `import` in the file is in the stdlib allowlist and no string literal references `internal/`, `cmd/`, `.go`, or `go run`; exit 1 with `FAIL import-pin: <reason>` otherwise. `--self-test` runs the negative control (a temp copy with `import internal.fold` injected must FAIL) and exits 1 if the control passes.

- [ ] **Step 1: Write the negative control first (it is the test)**

Run: `python gates/importpin.py --self-test` → must fail now with "No such file".

- [ ] **Step 2: Implement**

`gates/importpin.py`:
```python
#!/usr/bin/env python3
"""Import-pin: the naive fold in fixtures/generate.py may import only the
Python stdlib names below and may not reference the Go tree. This makes P5's
independence claim structural. --self-test injects a forbidden import into a
temp copy and requires the check to FAIL on it (a gate that cannot go red
proves nothing)."""
import ast
import os
import sys
import tempfile

ALLOWED = {"argparse", "datetime", "hashlib", "json", "os", "random", "sys"}
FORBIDDEN_LITERALS = ("internal/", "cmd/", ".go", "go run", "go build")
DEFAULT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "fixtures", "generate.py")


def check(path):
    with open(path, encoding="ascii") as f:
        src = f.read()
    tree = ast.parse(src, filename=path)
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for a in node.names:
                root = a.name.split(".")[0]
                if root not in ALLOWED:
                    return "import %s not in allowlist" % a.name
        elif isinstance(node, ast.ImportFrom):
            root = (node.module or "").split(".")[0]
            if root not in ALLOWED:
                return "from %s import ... not in allowlist" % node.module
        elif isinstance(node, ast.Call) and getattr(node.func, "id", "") in ("__import__", "exec", "eval"):
            return "dynamic import/exec is forbidden"
        elif isinstance(node, ast.Constant) and isinstance(node.value, str):
            for lit in FORBIDDEN_LITERALS:
                if lit in node.value:
                    return "string literal references the Go tree: %r" % node.value
    return None


def main(argv):
    if argv and argv[0] == "--self-test":
        with open(DEFAULT, encoding="ascii") as f:
            src = f.read()
        with tempfile.NamedTemporaryFile("w", suffix=".py", delete=False, encoding="ascii") as tmp:
            tmp.write("import internal.fold\n" + src)
            name = tmp.name
        try:
            reason = check(name)
        finally:
            os.unlink(name)
        if reason is None:
            print("FAIL import-pin self-test: forbidden import was not caught")
            return 1
        print("ok import-pin self-test (negative control caught: %s)" % reason)
        return 0
    path = argv[0] if argv else DEFAULT
    reason = check(path)
    if reason:
        print("FAIL import-pin: " + reason)
        return 1
    print("ok import-pin " + os.path.relpath(path))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
```

- [ ] **Step 3: Run both directions**

Run: `python gates/importpin.py && python gates/importpin.py --self-test`
Expected: `ok import-pin fixtures/generate.py` (or the relative path) then `ok import-pin self-test (negative control caught: import internal.fold not in allowlist)`. If the first line FAILs on a literal, the generator's docstring mentions the Go tree — reword it (the docstring in Task 9 says "Go tree" without a slash, which is allowed).

- [ ] **Step 4: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/importpin.py
git commit -m "feat: import-pin gate for the naive fold, with negative control"
```

---

### Task 17: Runner, claimability table, STATUS cross-check, CI

**Files:**
- Create: `gates/run.sh`, `gates/claimability.py`, `.github/workflows/gates.yml`

**Interfaces:**
- `sh gates/run.sh` (from repo root; POSIX sh): regenerates fixtures into a scratch dir and diffs against checked-in (freshness + determinism), runs the import-pin and its self-test, `go vet`, builds `bin/meridian`, runs `go test ./... -count=1` with `MERIDIAN_BIN` and a fresh `MERIDIAN_VERDICT_DIR=gates/out`, then `python gates/claimability.py gates/out --status STATUS.md`. Any step failing ⇒ exit 1. Last line on success: `ok lane1 claimable=<k>/6`.
- `python gates/claimability.py <verdict_dir> [--status STATUS.md]`: reads every `meridian-lane1-p*-*.json`, groups by property, prints an ASCII table (`P# | live | twin | CLAIMABLE?`), decides CLAIMABLE = exactly one live row GREEN **and** ≥1 twin rows, every twin RED with `checks == planted.expected_violations`. With `--status`, parses the Lane-1 table in STATUS.md: any row marked `CLAIMABLE` in STATUS.md whose verdicts do not support it ⇒ exit 1 (`FAIL STATUS.md overclaims P<N>`); a supported property not yet marked is a `WARN` (operator lag, not dishonesty). Exit 1 also if any property has no rows at all (the runner did not run every gate).

- [ ] **Step 1: Write claimability.py**

```python
#!/usr/bin/env python3
"""Claimability table from GATE_VERDICT rows, and a STATUS.md overclaim check."""
import argparse
import glob
import json
import os
import re
import sys

PROPS = [1, 2, 3, 4, 5, 6]


def load(dirpath):
    rows = {p: {"live": [], "twin": []} for p in PROPS}
    for f in sorted(glob.glob(os.path.join(dirpath, "meridian-lane1-p*-*.json"))):
        with open(f) as fh:
            r = json.load(fh)
        m = re.match(r"meridian-lane1-p(\d)", r["surface"])
        rows[int(m.group(1))][r["cell"]].append(r)
    return rows


def twin_ok(r):
    return r["result"] == "RED" and r["checks"] == r["planted"]["expected_violations"] and any(v > 0 for v in r["checks"].values())


def claimable(cell):
    live = cell["live"]
    twins = cell["twin"]
    return len(live) == 1 and live[0]["result"] == "GREEN" and len(twins) >= 1 and all(twin_ok(t) for t in twins)


def status_cells(path):
    """Return {prop: status_word} from the Lane 1 table in STATUS.md."""
    out = {}
    with open(path) as fh:
        for line in fh:
            m = re.match(r"\|\s*P(\d)\s*\|[^|]*\|\s*([A-Z]+)\s*\|\s*([A-Z]+)\s*\|\s*([^|]+?)\s*\|", line)
            if m:
                out[int(m.group(1))] = (m.group(2), m.group(3), m.group(4))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("verdict_dir")
    ap.add_argument("--status")
    a = ap.parse_args()
    rows = load(a.verdict_dir)
    k, bad = 0, False
    print("prop | live  | twin(s)            | claimable")
    for p in PROPS:
        c = rows[p]
        if not c["live"] or not c["twin"]:
            print("P%d   | %-5s | %-18s | NO (missing rows)" % (p, "-" if not c["live"] else c["live"][0]["result"], "-"))
            bad = True
            continue
        ok = claimable(c)
        k += ok
        tw = ",".join(("RED*" if twin_ok(t) else t["result"]) for t in c["twin"])
        print("P%d   | %-5s | %-18s | %s" % (p, c["live"][0]["result"], tw, "YES" if ok else "NO"))
    if a.status:
        st = status_cells(a.status)
        for p in PROPS:
            marked = p in st and st[p][2].upper().startswith("CLAIMABLE")
            supported = bool(rows[p]["live"] and rows[p]["twin"]) and claimable(rows[p])
            if marked and not supported:
                print("FAIL STATUS.md overclaims P%d" % p)
                bad = True
            elif supported and not marked:
                print("WARN P%d is supported by verdicts but STATUS.md does not mark it CLAIMABLE" % p)
    if bad:
        return 1
    print("ok lane1 claimable=%d/6" % k)
    return 0


if __name__ == "__main__":
    sys.exit(main())
```
(`RED*` in the table means RED with the planted counts matched.)

- [ ] **Step 2: Write run.sh**

```sh
#!/bin/sh
# MERIDIAN Lane 1 gate runner. Every step must pass; nothing here is optional.
set -eu
cd "$(dirname "$0")/.."
PY="${PYTHON:-python}"

echo "== fixtures: deterministic + fresh"
sh fixtures/generate_test.sh

echo "== import-pin (+ negative control)"
"$PY" gates/importpin.py
"$PY" gates/importpin.py --self-test

echo "== go vet"
go vet ./...

echo "== build"
mkdir -p bin
BIN="$PWD/bin/meridian"
case "$(uname -s 2>/dev/null || echo unknown)" in MINGW*|MSYS*|CYGWIN*) BIN="$BIN.exe" ;; esac
go build -o "$BIN" ./cmd/meridian

echo "== tests + gates"
rm -rf gates/out && mkdir -p gates/out
MERIDIAN_BIN="$BIN" MERIDIAN_VERDICT_DIR="$PWD/gates/out" MERIDIAN_RUNNER="${MERIDIAN_RUNNER:-local}" go test ./... -count=1

echo "== claimability"
"$PY" gates/claimability.py gates/out --status STATUS.md
```

- [ ] **Step 3: Write the CI workflow**

`.github/workflows/gates.yml`:
```yaml
name: gates
on:
  push:
  pull_request:
jobs:
  lane1:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - name: run gates
        env:
          MERIDIAN_RUNNER: github-actions
        run: sh gates/run.sh
      - name: upload verdict rows
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: verdicts
          path: gates/out/
```

- [ ] **Step 4: Run the whole runner**

Run: `sh gates/run.sh`
Expected: each `==` section passes; final table shows six `YES` rows and `ok lane1 claimable=6/6`. Six `WARN ... STATUS.md does not mark it CLAIMABLE` lines are expected at this point (STATUS.md still says UNCLAIMED) — they are warnings, exit code 0. Any `FAIL` means a gate is not honest; do not proceed to Task 18 until this prints `ok`.

Check the Linux path too if on Windows: the CI run on push is the cross-platform evidence for P2 (`pinned_hash_match` on ubuntu proves the snapshot bytes are identical across OSes — that is the byte-identity claim, so **do not** mark P2 CLAIMABLE until CI is green).

- [ ] **Step 5: Commit for the operator**

```bash
cd ~/dev/meridian
git add gates/run.sh gates/claimability.py .github/workflows/gates.yml
git commit -m "feat: lane-1 gate runner, claimability table, STATUS overclaim check, CI"
git push   # CI must go green before Task 18 flips any cell
```

---

### Task 18: State of record, docs, handoff

**Files:**
- Modify: `STATUS.md` (Lane 1 table + a dated state-of-record entry), `README.md` (one line: how to run the gates), `docs/handoff/HANDOFF.md` + new `docs/handoff/<date>-lane1-build.md`
- Do **not** touch `docs/2026-08-31-design.md` except to append a one-line "Built: see STATUS.md" note under §8 if the operator asks.

- [ ] **Step 1: Verify before claiming (this is the gate for this task)**

Run locally: `sh gates/run.sh` → `ok lane1 claimable=6/6`. Then confirm CI: `gh run list --workflow gates --limit 1` shows `completed success` for the pushed sha. Record both the local `parallax_sha` from a `gates/out/*-p2-live-*.json` row and the CI run id. If CI is not green, stop: no cell flips; write the handoff describing the red instead.

- [ ] **Step 2: Update STATUS.md**

Replace the Lane 1 table rows — for each property with a supported verdict:

```
| P1 | At-most-once fill ingestion | GREEN | RED | CLAIMABLE |
```
Keep any unsupported property `UNCLAIMED | UNCLAIMED | not claimable — <reason>`. Add a dated entry above the crediting rule:

```
- **<YYYY-MM-DD>** — Lane 1 built. `sh gates/run.sh` → `ok lane1 claimable=<k>/6` at
  commit `<sha>` (worktree clean); CI run <id> green on ubuntu (cross-OS
  byte-identity for P2). Verdict rows: `gates/out/` (regenerated per run, not
  committed). Twin counts per property are in `fixtures/base/manifest.json`
  under `p<N>.twin.expected_violations`.
```
Keep "Deferred decisions"; strike the sequencing line only if the operator has ruled on it.

- [ ] **Step 3: README — one pointer, no counts**

Add under "Scope walls" or as a short "Run the gates" section:
```
## Run the gates

    python fixtures/generate.py   # regenerate fixtures (deterministic; CI checks freshness)
    sh gates/run.sh               # every live gate green, every twin red for its planted reason

Claim state is in [STATUS.md](STATUS.md).
```

- [ ] **Step 4: Re-run the overclaim check**

Run: `python gates/claimability.py gates/out --status STATUS.md` → no `FAIL`, no `WARN`, `ok lane1 claimable=<k>/6`.

- [ ] **Step 5: Handoff brief**

Use the `rigor:handoff` skill (or follow `docs/handoff/2026-09-01-meridian-spec-design.md` as the template): current state with re-verify lines (`sh gates/run.sh` last line; `grep -c CLAIMABLE STATUS.md` → k; CI run id), decisions taken during the build (anything that deviated from this plan, with the reason), open items (gRPC read API, cross-language byte-identical twin, BASELINE registration seam incl. the `parallax_sha` key-name question, lanes 2–3). Add a learnings entry for any gate that fired wrong during the build (rigor `learn-from-misfire`).

- [ ] **Step 6: Commit for the operator**

```bash
cd ~/dev/meridian
git add STATUS.md README.md docs/handoff docs/learnings
git commit -m "docs: lane 1 state of record, run instructions, build handoff"
git push
```

---

## Self-review (done at plan time; executor re-checks at Task 18)

**Spec coverage** — design §1: feed (T2), fold+time (T4), money (T3), snapshot (T5), P1 mechanics (T4), as-of pure recompute (T6), CLI incl. `reconcile --statement` (T8). §2: P1 (T10), P2 (T11), P3 (T12), P4 (T13), P5 (T14), P6 (T15) — each with live + twin and exact counts via `Emit`. §3: generator + naive fold + import-pin (T9, T16). §4: runner emits the claimability table; STATUS.md state of record with BASELINE row schema (T10 verdict.go, T17, T18); CI on every push (T17). §5: repo layout matches (no `internal/gates`; gates live in `gates/` per spec). §6 scope walls: no floats (T3/T4, big.Int), no perf vocabulary (constraint), import-pin (T16), lanes 2–3 untouched. §7/§8: not in scope; the `parallax_sha` key question is carried as an open item in T18.

**Known soft spots the executor must watch:**
1. T2's test helper `pl` must produce `json.Number`; if the executor "simplifies" it to `int`, canonical bytes change and `TestAppendThenReopenVerifiesChain` line-1 comparison fails — the test is right.
2. Generator randomness: the seeded stream must leave BBB and DDD with positions at end for the P4 twin; the `held // 2` cap guarantees it. If Python's `random` changes its algorithm across versions the fixtures drift — CI pins 3.12; local 3.14 must produce identical bytes (`generate_test.sh` catches it; if it fires, pin the generator to an explicit LCG instead of `random.Random`).
3. P3's live `viewpoint_V2` compares Go vs naive fold on the amended-split path; a disagreement there is the most likely real bug in either fold — resolve against the shared contract, never by editing `expected/`.
4. Windows: `go test` runs from `gates/`, so `FixturesDir = "../fixtures"` and `out/` are relative to it; `run.sh` passes an absolute `MERIDIAN_VERDICT_DIR`.
5. `Emit` uses `git`; in a detached CI checkout `rev-parse HEAD` works. In a worktree-isolated session it works too. If the executor runs from a non-git copy, every gate fails loudly — intended.

**Type consistency check:** `Counts`/`NewCounts`/`Row`/`Emit`/`Planted`/`ptr`/`sha256Hex`/`LoadDoc`/`ReadFixture`/`LoadManifest`/`Manifest.Int|Str|Strs|Ints|Planted` are defined in T10 and used identically in T11–T15. `snapshot.Diff/Leaves/Decode/Doc` from T5; `reconcile.LoadStatement/Reconcile/Mismatch{Instrument,Field,Ledger,Custodian,Delta}` from T7; `feed.Open/Records/Len/PrefixHash/ChainError{Seq}` from T2; `asof.Read/Result{State,Doc,Bytes,Hash,PrefixHash,Seq}` from T6.
