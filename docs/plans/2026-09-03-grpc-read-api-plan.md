# MERIDIAN gRPC Read API (P7) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the read-only gRPC API over Lane 1 (`Head`, `AsOf`, `Reconcile`) behind a Go `Reader` seam, and earn property P7 (wire fidelity) with one GREEN live cell and two RED twins with exact planted counts, so `sh gates/run.sh` ends `ok lane1 claimable=7/7` over 18 verdict rows.

**Architecture:** `internal/reader` defines `Reader` and the live `FeedReader` (per-request `feed.Open`, pure recompute). `internal/readgrpc` adapts any `Reader` to the generated `meridianv1.ReaderServer` with a fixed error-to-status mapping and offers an in-process `bufconn` client. The gate (`gates/p7_test.go`) is a gRPC client that re-derives everything locally; the two twins are misbehaving `Reader`s behind the same adapter, whose expected counts the Python generator measures into `manifest.json`.

**Tech Stack:** Go 1.26 (module `github.com/hossainpazooki/meridian`), `google.golang.org/grpc` v1.83.2, `google.golang.org/protobuf` v1.36.12, `buf` v1.72.0 + `protoc-gen-go` v1.36.12 + `protoc-gen-go-grpc` v1.6.2 as `go.mod` `tool` directives, Python 3.14 (generator), POSIX `sh` (runner).

**Spec:** `docs/2026-09-03-grpc-read-api-design.md`. Read it first; this plan implements it and nothing else.

## Global Constraints

- **Only the operator writes git history.** Every "commit" step below means: print the command block for the operator, do NOT run `git commit`/`git push`. A hook blocks it anyway.
- **No floats anywhere; no wallclock in the fold.** Unchanged packages stay unchanged in behaviour.
- **Snapshot bytes cross the wire verbatim.** Never re-serialise the snapshot into typed messages.
- **No write RPC.** The proto service has exactly three unary methods: `Head`, `AsOf`, `Reconcile`.
- **`evaluated` is the size of the universe a check examined**; `Emit` refuses `<= 0`.
- **Never weaken an expectation to make a guard pass.** If a generator guard fires, the planted structure is wrong.
- **STATUS.md is the state of record; README carries no counts or claim language** (`grep -oE 'CLAIMABLE|GREEN|RED\*' README.md | wc -l` stays 0).
- **A bare `go test` appends verdict rows**; only `sh gates/run.sh` clears `gates/out/`. After any `go test ./gates/...`, re-run `run.sh` before reading claimability.
- **Console is cp1252 on Windows**: keep Python `print()` ASCII.
- **Every generated file is LF** (`.gitattributes` has `* text=auto eol=lf`).
- Tool versions above are pinned; do not float them.
- Run all commands from the repo root `~/dev/meridian` unless a step says otherwise.

---

## File map

| Path | Task | Responsibility |
|---|---|---|
| `api/meridian/v1/read.proto` | 1 | The contract. |
| `api/meridian/v1/read.pb.go`, `read_grpc.pb.go` | 1 | Generated; committed. |
| `api/meridian/v1/read_test.go` | 1 | Descriptor test: exactly three unary methods. |
| `buf.yaml`, `buf.gen.yaml` | 1 | Codegen config, plugins via `go tool`. |
| `go.mod`, `go.sum` | 1 | grpc + protobuf deps; three `tool` directives. |
| `gates/run.sh` | 1, 7 | `== proto fresh` step; success line via claimability.py. |
| `.gitignore` | 1 | `/.protofresh/`. |
| `internal/reconcile/reconcile.go` | 2 | `LoadStatementBytes`. |
| `internal/asof/asof.go` | 3 | `ReadFrom(*feed.Feed, seq)`. |
| `internal/reader/reader.go`, `reader_test.go` | 3 | `Reader` interface, result structs, sentinels, `FeedReader`. |
| `internal/readgrpc/server.go`, `inprocess.go`, `server_test.go` | 4 | Adapter, status mapping, bufconn helper. |
| `cmd/meridian/main.go`, `main_test.go` | 5 | `serve` subcommand. |
| `fixtures/generate.py`, `fixtures/base/manifest.json` | 6 | `p7` section, measured footprints. |
| `gates/p7_test.go`, `gates/claimability.py` | 7 | The gate; 7 properties. |
| `STATUS.md`, `README.md`, `docs/learnings/*`, `docs/handoff/*` | 8 | State of record. |

---

### Task 1: Contract and codegen

**Files:**
- Create: `api/meridian/v1/read.proto`, `buf.yaml`, `buf.gen.yaml`, `api/meridian/v1/read_test.go`
- Generate: `api/meridian/v1/read.pb.go`, `api/meridian/v1/read_grpc.pb.go`
- Modify: `go.mod`, `go.sum`, `gates/run.sh` (after the `== import-pin` block, before `== go vet`), `.gitignore`

**Interfaces:**
- Produces: Go package `meridianv1` (`github.com/hossainpazooki/meridian/api/meridian/v1`) with `ReaderServer`, `ReaderClient`, `UnimplementedReaderServer`, `RegisterReaderServer`, `NewReaderClient`, messages `HeadRequest{}`, `HeadResponse{Records int64; PrefixHash string}`, `AsOfRequest{Seq int64}`, `AsOfResponse{Seq int64; PrefixHash, SnapshotHash string; Snapshot []byte}`, `ReconcileRequest{Seq int64; Statement []byte}`, `Mismatch{Instrument, Field string; Ledger, Custodian, Delta int64}`, `ReconcileResponse{Compared int64; Mismatches []*Mismatch}`, and the descriptor var `File_meridian_v1_read_proto`.

- [ ] **Step 1: Write the proto**

`api/meridian/v1/read.proto`:

```proto
// MERIDIAN read API. Read-only by construction: this service has exactly
// three unary methods and none of them writes. A write path would drag
// transport into P1's at-most-once claim (design Q5, 2026-08-31).
syntax = "proto3";

package meridian.v1;

option go_package = "github.com/hossainpazooki/meridian/api/meridian/v1;meridianv1";

service Reader {
  // Head returns the feed length and the prefix hash of the whole feed
  // (what `meridian replay` prints).
  rpc Head(HeadRequest) returns (HeadResponse);
  // AsOf replays the prefix [1..seq] and returns the canonical snapshot
  // bytes verbatim plus their sha256. seq < 0 means end of feed.
  rpc AsOf(AsOfRequest) returns (AsOfResponse);
  // Reconcile compares the snapshot at seq with a custodian statement
  // (the same JSON bytes `meridian reconcile --statement` loads).
  rpc Reconcile(ReconcileRequest) returns (ReconcileResponse);
}

message HeadRequest {}

message HeadResponse {
  int64 records = 1;
  string prefix_hash = 2;  // "sha256:<hex>"
}

message AsOfRequest {
  int64 seq = 1;  // < 0 means end of feed
}

message AsOfResponse {
  int64 seq = 1;            // resolved seq (a -1 request answers with the real end seq)
  string prefix_hash = 2;   // "sha256:<hex>" of the feed prefix [1..seq]
  string snapshot_hash = 3; // "sha256:<hex>" over `snapshot` exactly as sent
  bytes snapshot = 4;       // canonical JSON + '\n', byte-identical to `meridian asof`
}

message ReconcileRequest {
  int64 seq = 1;
  bytes statement = 2;  // custodian statement JSON object
}

message Mismatch {
  string instrument = 1;  // empty for the cash field
  string field = 2;       // "cash" | "quantity" | "cost_basis"
  int64 ledger = 3;
  int64 custodian = 4;
  int64 delta = 5;        // ledger - custodian
}

message ReconcileResponse {
  int64 compared = 1;
  repeated Mismatch mismatches = 2;
}
```

- [ ] **Step 2: Write buf config**

`buf.yaml`:

```yaml
version: v2
modules:
  - path: api
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

`buf.gen.yaml`:

```yaml
version: v2
# Plugins run through `go tool`, pinned by go.mod's tool directives, so the
# generated bytes depend only on go.mod -- no global installs, no PATH.
plugins:
  - local: [go, tool, protoc-gen-go]
    out: api
    opt: paths=source_relative
  - local: [go, tool, protoc-gen-go-grpc]
    out: api
    opt: paths=source_relative
```

- [ ] **Step 3: Pin dependencies and tools**

Run:

```sh
go get google.golang.org/grpc@v1.83.2 google.golang.org/protobuf@v1.36.12
go get -tool github.com/bufbuild/buf/cmd/buf@v1.72.0
go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
go get -tool google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
```

Expected: `go.mod` now has `require` lines for grpc and protobuf and a `tool (...)` block naming the three commands. If `go get -tool` of buf v1.72.0 fails to build on Windows, fall back to `@v1.57.2` (verified to run here 2026-09-03 via `go run`) and record the version actually pinned in the commit message. Then:

```sh
go tool buf --version
go tool protoc-gen-go --version
go tool protoc-gen-go-grpc --version
```

Expected: `1.72.0` (or `1.57.2`), `protoc-gen-go v1.36.12`, `protoc-gen-go-grpc 1.6.2`.

- [ ] **Step 4: Generate**

Run: `go tool buf generate && go mod tidy && ls api/meridian/v1`

Expected: `read.pb.go  read.proto  read_grpc.pb.go`. `go mod tidy` must keep grpc and protobuf as direct requires (the generated code imports them). Run `go build ./...` -> exit 0.

- [ ] **Step 5: Write the descriptor test (fails until generation exists, passes after)**

`api/meridian/v1/read_test.go`:

```go
package meridianv1

import (
	"reflect"
	"sort"
	"testing"
)

// The read-only guarantee is a fact about the proto: exactly these three
// methods, all unary, none of them a write. Any future RPC added to the
// service fails this test until the design is amended.
func TestReaderServiceIsReadOnly(t *testing.T) {
	sd := File_meridian_v1_read_proto.Services().ByName("Reader")
	if sd == nil {
		t.Fatal("service Reader missing from descriptor")
	}
	want := []string{"AsOf", "Head", "Reconcile"}
	var got []string
	for i := 0; i < sd.Methods().Len(); i++ {
		m := sd.Methods().Get(i)
		got = append(got, string(m.Name()))
		if m.IsStreamingClient() || m.IsStreamingServer() {
			t.Fatalf("method %s is streaming; the read API is unary only", m.Name())
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reader methods = %v, want exactly %v", got, want)
	}
}
```

Run: `go test ./api/... -count=1`
Expected: `ok  	github.com/hossainpazooki/meridian/api/meridian/v1`

- [ ] **Step 6: Add the proto-fresh step to the runner and ignore its scratch dir**

Append `/.protofresh/` to `.gitignore` (own line).

In `gates/run.sh`, insert after the `"$PY" gates/importpin.py --self-test` line and before `echo "== go vet"`:

```sh
echo "== proto fresh"
# Regenerate into a scratch dir and byte-compare with the committed files:
# committed generated code must be exactly what the pinned tools produce.
rm -rf .protofresh && mkdir -p .protofresh
go tool buf generate -o .protofresh
for f in read.pb.go read_grpc.pb.go; do
  cmp ".protofresh/api/meridian/v1/$f" "api/meridian/v1/$f" || { echo "FAIL generated $f stale: run go tool buf generate"; exit 1; }
done
rm -rf .protofresh
echo "ok proto fresh"
```

Run: `sh gates/run.sh 2>&1 | grep -E "^ok proto fresh|^ok lane1"`
Expected: both lines print (`ok proto fresh`, `ok lane1 claimable=6/6`). Negative control: append a blank line to `api/meridian/v1/read.pb.go`, run `sh gates/run.sh 2>&1 | grep FAIL` -> `FAIL generated read.pb.go stale: run go tool buf generate`; then `go tool buf generate` to restore the file and confirm `run.sh` prints `ok proto fresh` again.

- [ ] **Step 7: Print the commit for the operator (do not run it)**

```bash
cd ~/dev/meridian
git add api/ buf.yaml buf.gen.yaml go.mod go.sum gates/run.sh .gitignore
git commit -m "feat: read API proto, pinned codegen, proto-fresh gate step"
```

---

### Task 2: `reconcile.LoadStatementBytes`

**Files:**
- Modify: `internal/reconcile/reconcile.go:38-84` (`LoadStatement`)
- Test: `internal/reconcile/reconcile_test.go` (append)

**Interfaces:**
- Produces: `func LoadStatementBytes(raw []byte) (Statement, error)`; `LoadStatement(path string)` keeps its signature and becomes `os.ReadFile` + `LoadStatementBytes`.

- [ ] **Step 1: Write the failing test**

Append to `internal/reconcile/reconcile_test.go`:

```go
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
```

Ensure the test file imports `os`, `path/filepath`, `reflect` (add if missing).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/reconcile/ -run LoadStatementBytes -count=1`
Expected: FAIL, `undefined: LoadStatementBytes`.

- [ ] **Step 3: Implement**

Replace the body of `LoadStatement` so it reads the file and delegates; move the parsing into `LoadStatementBytes`:

```go
// LoadStatement reads the custodian JSON format from a file.
func LoadStatement(path string) (Statement, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Statement{}, err
	}
	return LoadStatementBytes(raw)
}

// LoadStatementBytes parses the custodian JSON format from bytes. This is
// the one statement schema; the gRPC read API passes statements as bytes
// so it shares this parser rather than carrying a second one.
func LoadStatementBytes(raw []byte) (Statement, error) {
	v, err := canon.Decode(raw)
	if err != nil {
		return Statement{}, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Statement{}, fmt.Errorf("statement is not an object")
	}
	// ... the existing body from `var st Statement` to `return st, nil`, unchanged ...
}
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/reconcile/ -count=1`
Expected: `ok`.

- [ ] **Step 5: Print the commit for the operator**

```bash
cd ~/dev/meridian
git add internal/reconcile/
git commit -m "refactor: reconcile.LoadStatementBytes for byte-borne statements"
```

---

### Task 3: `internal/reader` and `asof.ReadFrom`

**Files:**
- Modify: `internal/asof/asof.go` (split `Read` into `Read` + `ReadFrom`)
- Test: `internal/asof/asof_test.go` (append)
- Create: `internal/reader/reader.go`, `internal/reader/reader_test.go`

**Interfaces:**
- Consumes: `reconcile.LoadStatementBytes` (Task 2), `feed.Open/Len/PrefixHash/Close`, `fold`, `snapshot`.
- Produces:
  ```go
  package reader
  var ErrNotFound, ErrSeqOutOfRange, ErrBadStatement error
  type Head struct{ Records int64; PrefixHash string }
  type AsOf struct{ Seq int64; PrefixHash, SnapshotHash string; Snapshot []byte }
  type Recon struct{ Compared int64; Mismatches []reconcile.Mismatch }
  type Reader interface {
      Head(ctx context.Context) (Head, error)
      AsOf(ctx context.Context, seq int64) (AsOf, error)
      Reconcile(ctx context.Context, seq int64, statement []byte) (Recon, error)
  }
  type FeedReader struct{ Path string }   // implements Reader
  ```
  and `func asof.ReadFrom(f *feed.Feed, seq int64) (asof.Result, error)`.

- [ ] **Step 1: Failing test for `asof.ReadFrom`**

Append to `internal/asof/asof_test.go`:

```go
func TestReadFromEqualsRead(t *testing.T) {
	path := filepath.Join("..", "..", "fixtures", "base", "feed.jsonl")
	viaPath, err := Read(path, -1)
	if err != nil {
		t.Fatal(err)
	}
	f, err := feed.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	viaFeed, err := ReadFrom(f, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(viaPath.Bytes, viaFeed.Bytes) || viaPath.Hash != viaFeed.Hash || viaPath.Seq != viaFeed.Seq {
		t.Fatalf("ReadFrom diverges from Read: %s vs %s", viaPath.Hash, viaFeed.Hash)
	}
	if _, err := ReadFrom(f, f.Len()+1); err == nil {
		t.Fatal("seq past end must error")
	}
}
```

Imports needed: `bytes`, `path/filepath`, and `github.com/hossainpazooki/meridian/internal/feed`.

Run: `go test ./internal/asof/ -run ReadFrom -count=1` -> FAIL `undefined: ReadFrom`.

- [ ] **Step 2: Implement `ReadFrom`**

Replace `Read` in `internal/asof/asof.go`:

```go
// Read opens feedPath (verifying its chain), folds [1..seq] and builds the
// snapshot. seq < 0 selects the last record.
func Read(feedPath string, seq int64) (Result, error) {
	f, err := feed.Open(feedPath)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	return ReadFrom(f, seq)
}

// ReadFrom folds [1..seq] of an already-open feed and builds the snapshot.
// seq < 0 selects the last record. It exists so a caller that has already
// opened the feed (to check its length, say) does not pay a second scan.
func ReadFrom(f *feed.Feed, seq int64) (Result, error) {
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
	doc, b, h, err := snapshot.Build(st, ph)
	if err != nil {
		return Result{}, err
	}
	return Result{State: st, Doc: doc, Bytes: b, Hash: h, PrefixHash: ph, Seq: seq}, nil
}
```

Run: `go test ./internal/asof/ -count=1` -> `ok`.

- [ ] **Step 3: Failing tests for `FeedReader`**

`internal/reader/reader_test.go`:

```go
package reader

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hossainpazooki/meridian/internal/asof"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reconcile"
)

var fixtures = filepath.Join("..", "..", "fixtures")

func base() string { return filepath.Join(fixtures, "base", "feed.jsonl") }

func TestFeedReaderHeadMatchesFeed(t *testing.T) {
	f, err := feed.Open(base())
	if err != nil {
		t.Fatal(err)
	}
	wantHash, _ := f.PrefixHash(f.Len())
	wantLen := f.Len()
	f.Close()
	h, err := FeedReader{Path: base()}.Head(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Records != wantLen || h.PrefixHash != wantHash {
		t.Fatalf("Head = %+v, want records=%d prefix=%s", h, wantLen, wantHash)
	}
}

func TestFeedReaderAsOfMatchesAsofRead(t *testing.T) {
	r := FeedReader{Path: base()}
	for _, seq := range []int64{-1, 1} {
		want, err := asof.Read(base(), seq)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.AsOf(context.Background(), seq)
		if err != nil {
			t.Fatalf("seq %d: %v", seq, err)
		}
		if got.Seq != want.Seq || got.PrefixHash != want.PrefixHash || got.SnapshotHash != want.Hash || !bytes.Equal(got.Snapshot, want.Bytes) {
			t.Fatalf("seq %d: AsOf diverges from asof.Read", seq)
		}
	}
}

func TestFeedReaderMissingPathIsNotFoundAndCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.jsonl")
	r := FeedReader{Path: path}
	ctx := context.Background()
	if _, err := r.Head(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Head: got %v, want ErrNotFound", err)
	}
	if _, err := r.AsOf(ctx, -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AsOf: got %v, want ErrNotFound", err)
	}
	if _, err := r.Reconcile(ctx, -1, []byte(`{"as_of_seq":0,"cash":0,"holdings":[]}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Reconcile: got %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a read created %s: feed.Open must never be reached for a missing path", path)
	}
}

func TestFeedReaderSeqPastEndIsOutOfRange(t *testing.T) {
	f, err := feed.Open(base())
	if err != nil {
		t.Fatal(err)
	}
	n := f.Len()
	f.Close()
	r := FeedReader{Path: base()}
	if _, err := r.AsOf(context.Background(), n+1); !errors.Is(err, ErrSeqOutOfRange) {
		t.Fatalf("got %v, want ErrSeqOutOfRange", err)
	}
	if _, err := r.AsOf(context.Background(), n); err != nil {
		t.Fatalf("seq == records must be valid: %v", err)
	}
}

func TestFeedReaderBadStatement(t *testing.T) {
	r := FeedReader{Path: base()}
	for _, raw := range []string{"[]", "{", ""} {
		if _, err := r.Reconcile(context.Background(), -1, []byte(raw)); !errors.Is(err, ErrBadStatement) {
			t.Fatalf("%q: got %v, want ErrBadStatement", raw, err)
		}
	}
}

func TestFeedReaderReconcileMatchesDirect(t *testing.T) {
	stPath := filepath.Join(fixtures, "base", "statement.json")
	raw, err := os.ReadFile(stPath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := reconcile.LoadStatementBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	res, err := asof.Read(base(), -1)
	if err != nil {
		t.Fatal(err)
	}
	wantMs, wantCompared, err := reconcile.Reconcile(res.Doc, st)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FeedReader{Path: base()}.Reconcile(context.Background(), -1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Compared != int64(wantCompared) || !reflect.DeepEqual(got.Mismatches, wantMs) {
		t.Fatalf("Reconcile = %+v, want compared=%d ms=%v", got, wantCompared, wantMs)
	}
	if wantCompared == 0 {
		t.Fatal("base statement compared nothing; the fixture is wrong")
	}
}

func TestFeedReaderChainErrorPassesThrough(t *testing.T) {
	tampered := filepath.Join(fixtures, "p2", "tampered", "feed.jsonl")
	_, err := FeedReader{Path: tampered}.Head(context.Background())
	var ce *feed.ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("got %v, want *feed.ChainError", err)
	}
}
```

Run: `go test ./internal/reader/ -count=1` -> FAIL (package does not build: `undefined: FeedReader` etc.).

- [ ] **Step 4: Implement `internal/reader/reader.go`**

```go
// Package reader is the read seam over Lane 1: the "Reader protocol" the
// design names for Lane 2. A Reader answers three questions -- what is the
// head of the feed, what did the ledger know at seq V, and how does that
// compare to a custodian statement -- and nothing else. The gRPC server
// adapts any Reader; the live one is FeedReader; the P7 twins are Readers
// that lie.
package reader

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hossainpazooki/meridian/internal/asof"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reconcile"
)

var (
	// ErrNotFound: the feed path does not exist. Checked BEFORE feed.Open,
	// which would otherwise create an empty-but-valid feed and answer a
	// clean empty ledger over nothing (the CLI's requireFeedExists rule).
	ErrNotFound = errors.New("feed does not exist")
	// ErrSeqOutOfRange: seq > number of records.
	ErrSeqOutOfRange = errors.New("seq out of range")
	// ErrBadStatement: the statement bytes are not the custodian JSON shape.
	ErrBadStatement = errors.New("bad statement")
)

type Head struct {
	Records    int64
	PrefixHash string
}

type AsOf struct {
	Seq                      int64
	PrefixHash, SnapshotHash string
	Snapshot                 []byte
}

type Recon struct {
	Compared   int64
	Mismatches []reconcile.Mismatch
}

type Reader interface {
	Head(ctx context.Context) (Head, error)
	AsOf(ctx context.Context, seq int64) (AsOf, error)
	Reconcile(ctx context.Context, seq int64, statement []byte) (Recon, error)
}

// FeedReader is the live Reader: every call opens the feed, verifies its
// chain, and recomputes. Nothing is cached, so an append made by another
// process is visible on the next call and a chain break surfaces on read.
type FeedReader struct{ Path string }

func (r FeedReader) exists() error {
	if _, err := os.Stat(r.Path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, r.Path)
		}
		return err
	}
	return nil
}

func (r FeedReader) Head(ctx context.Context) (Head, error) {
	if err := r.exists(); err != nil {
		return Head{}, err
	}
	f, err := feed.Open(r.Path)
	if err != nil {
		return Head{}, err
	}
	defer f.Close()
	h, err := f.PrefixHash(f.Len())
	if err != nil {
		return Head{}, err
	}
	return Head{Records: f.Len(), PrefixHash: h}, nil
}

func (r FeedReader) read(seq int64) (asof.Result, error) {
	if err := r.exists(); err != nil {
		return asof.Result{}, err
	}
	f, err := feed.Open(r.Path)
	if err != nil {
		return asof.Result{}, err
	}
	defer f.Close()
	if seq > f.Len() {
		return asof.Result{}, fmt.Errorf("%w: seq %d > records %d", ErrSeqOutOfRange, seq, f.Len())
	}
	return asof.ReadFrom(f, seq)
}

func (r FeedReader) AsOf(ctx context.Context, seq int64) (AsOf, error) {
	res, err := r.read(seq)
	if err != nil {
		return AsOf{}, err
	}
	return AsOf{Seq: res.Seq, PrefixHash: res.PrefixHash, SnapshotHash: res.Hash, Snapshot: res.Bytes}, nil
}

// Reconcile validates the statement first (no I/O) and then reads: a bad
// statement is refused as such even when the feed is also missing.
func (r FeedReader) Reconcile(ctx context.Context, seq int64, statement []byte) (Recon, error) {
	st, err := reconcile.LoadStatementBytes(statement)
	if err != nil {
		return Recon{}, fmt.Errorf("%w: %v", ErrBadStatement, err)
	}
	res, err := r.read(seq)
	if err != nil {
		return Recon{}, err
	}
	ms, compared, err := reconcile.Reconcile(res.Doc, st)
	if err != nil {
		return Recon{}, err
	}
	return Recon{Compared: int64(compared), Mismatches: ms}, nil
}
```

Note on `TestFeedReaderMissingPathIsNotFoundAndCreatesNothing`: the statement in that test is valid on purpose, so the error it sees is `ErrNotFound`, proving the order (statement, then feed) does not hide a missing feed.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/reader/ ./internal/asof/ -count=1`
Expected: both `ok`.

- [ ] **Step 6: Print the commit for the operator**

```bash
cd ~/dev/meridian
git add internal/asof/ internal/reader/
git commit -m "feat: Reader seam and FeedReader over asof/reconcile"
```

---

### Task 4: `internal/readgrpc` adapter and in-process client

**Files:**
- Create: `internal/readgrpc/server.go`, `internal/readgrpc/inprocess.go`, `internal/readgrpc/server_test.go`

**Interfaces:**
- Consumes: `reader.Reader`, `reader.Err*`, `feed.ChainError`, `meridianv1.*` (Task 1).
- Produces:
  ```go
  package readgrpc
  func New(r reader.Reader) *Server                 // *Server implements meridianv1.ReaderServer
  func InProcess(r reader.Reader) (meridianv1.ReaderClient, func(), error)  // bufconn; call stop() when done
  ```

- [ ] **Step 1: Failing tests**

`internal/readgrpc/server_test.go`:

```go
package readgrpc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reader"
)

var fixtures = filepath.Join("..", "..", "fixtures")

// stubReader returns a fixed error from every method, so each row of the
// status-code table can be exercised without a matching on-disk condition.
type stubReader struct{ err error }

func (s stubReader) Head(context.Context) (reader.Head, error)   { return reader.Head{}, s.err }
func (s stubReader) AsOf(context.Context, int64) (reader.AsOf, error) { return reader.AsOf{}, s.err }
func (s stubReader) Reconcile(context.Context, int64, []byte) (reader.Recon, error) {
	return reader.Recon{}, s.err
}

func TestStatusCodeTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"not found", reader.ErrNotFound, codes.NotFound},
		{"chain", &feed.ChainError{Seq: 3, Reason: "prev mismatch"}, codes.DataLoss},
		{"seq range", reader.ErrSeqOutOfRange, codes.InvalidArgument},
		{"bad statement", reader.ErrBadStatement, codes.InvalidArgument},
		{"other", errors.New("disk on fire"), codes.Internal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client, stop, err := InProcess(stubReader{err: c.err})
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			ctx := context.Background()
			_, errH := client.Head(ctx, &meridianv1.HeadRequest{})
			_, errA := client.AsOf(ctx, &meridianv1.AsOfRequest{Seq: -1})
			_, errR := client.Reconcile(ctx, &meridianv1.ReconcileRequest{Seq: -1, Statement: []byte("{}")})
			for _, e := range []error{errH, errA, errR} {
				if status.Code(e) != c.want {
					t.Fatalf("got %v (%v), want %v", status.Code(e), e, c.want)
				}
			}
		})
	}
}

func TestRoundTripMatchesFeedReader(t *testing.T) {
	base := filepath.Join(fixtures, "base", "feed.jsonl")
	fr := reader.FeedReader{Path: base}
	client, stop, err := InProcess(fr)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ctx := context.Background()

	wantH, _ := fr.Head(ctx)
	gotH, err := client.Head(ctx, &meridianv1.HeadRequest{})
	if err != nil || gotH.GetRecords() != wantH.Records || gotH.GetPrefixHash() != wantH.PrefixHash {
		t.Fatalf("Head over the wire = %v (%v), want %+v", gotH, err, wantH)
	}

	wantA, _ := fr.AsOf(ctx, -1)
	gotA, err := client.AsOf(ctx, &meridianv1.AsOfRequest{Seq: -1})
	if err != nil || gotA.GetSeq() != wantA.Seq || gotA.GetPrefixHash() != wantA.PrefixHash ||
		gotA.GetSnapshotHash() != wantA.SnapshotHash || !bytes.Equal(gotA.GetSnapshot(), wantA.Snapshot) {
		t.Fatalf("AsOf over the wire diverges from FeedReader (%v)", err)
	}

	raw, err := os.ReadFile(filepath.Join(fixtures, "base", "statement.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantR, _ := fr.Reconcile(ctx, -1, raw)
	gotR, err := client.Reconcile(ctx, &meridianv1.ReconcileRequest{Seq: -1, Statement: raw})
	if err != nil || gotR.GetCompared() != wantR.Compared || len(gotR.GetMismatches()) != len(wantR.Mismatches) {
		t.Fatalf("Reconcile over the wire = %v (%v), want %+v", gotR, err, wantR)
	}
}

// A cash mismatch has an empty instrument; the mapping must preserve it and
// every numeric field, not just the count.
type oneMismatch struct{ stubReader }

func (oneMismatch) Reconcile(context.Context, int64, []byte) (reader.Recon, error) {
	return reader.Recon{Compared: 1, Mismatches: []reconcile.Mismatch{{Field: "cash", Ledger: 5, Custodian: 7, Delta: -2}}}, nil
}

func TestMismatchMappingPreservesEveryField(t *testing.T) {
	client, stop, err := InProcess(oneMismatch{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	resp, err := client.Reconcile(context.Background(), &meridianv1.ReconcileRequest{Seq: -1, Statement: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCompared() != 1 || len(resp.GetMismatches()) != 1 {
		t.Fatalf("resp = %v", resp)
	}
	m := resp.GetMismatches()[0]
	if m.GetInstrument() != "" || m.GetField() != "cash" || m.GetLedger() != 5 || m.GetCustodian() != 7 || m.GetDelta() != -2 {
		t.Fatalf("mismatch mapped as %v", m)
	}
}
```

Add `"github.com/hossainpazooki/meridian/internal/reconcile"` to the test file's imports (used by `oneMismatch`).

Run: `go test ./internal/readgrpc/ -count=1` -> FAIL (`undefined: InProcess`, `New`).

- [ ] **Step 2: Implement `server.go`**

```go
// Package readgrpc adapts a reader.Reader to the generated gRPC service.
// It holds no logic: result structs become messages, sentinel errors become
// status codes, and that mapping lives here and nowhere else.
package readgrpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reader"
)

type Server struct {
	meridianv1.UnimplementedReaderServer
	r reader.Reader
}

func New(r reader.Reader) *Server { return &Server{r: r} }

func (s *Server) Head(ctx context.Context, _ *meridianv1.HeadRequest) (*meridianv1.HeadResponse, error) {
	h, err := s.r.Head(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &meridianv1.HeadResponse{Records: h.Records, PrefixHash: h.PrefixHash}, nil
}

func (s *Server) AsOf(ctx context.Context, req *meridianv1.AsOfRequest) (*meridianv1.AsOfResponse, error) {
	a, err := s.r.AsOf(ctx, req.GetSeq())
	if err != nil {
		return nil, toStatus(err)
	}
	return &meridianv1.AsOfResponse{Seq: a.Seq, PrefixHash: a.PrefixHash, SnapshotHash: a.SnapshotHash, Snapshot: a.Snapshot}, nil
}

func (s *Server) Reconcile(ctx context.Context, req *meridianv1.ReconcileRequest) (*meridianv1.ReconcileResponse, error) {
	rc, err := s.r.Reconcile(ctx, req.GetSeq(), req.GetStatement())
	if err != nil {
		return nil, toStatus(err)
	}
	out := &meridianv1.ReconcileResponse{Compared: rc.Compared}
	for _, m := range rc.Mismatches {
		out.Mismatches = append(out.Mismatches, &meridianv1.Mismatch{
			Instrument: m.Instrument, Field: m.Field, Ledger: m.Ledger, Custodian: m.Custodian, Delta: m.Delta,
		})
	}
	return out, nil
}

// toStatus is the whole error contract of the API (design section 1).
func toStatus(err error) error {
	var ce *feed.ChainError
	switch {
	case errors.Is(err, reader.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.As(err, &ce):
		return status.Error(codes.DataLoss, err.Error())
	case errors.Is(err, reader.ErrSeqOutOfRange), errors.Is(err, reader.ErrBadStatement):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
```

- [ ] **Step 3: Implement `inprocess.go`**

```go
package readgrpc

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/reader"
)

// InProcess serves r over an in-memory listener and returns a connected
// client. No port is opened. The P7 gate and the adapter's own tests use
// it so the transport is identical across the live cell and every twin.
func InProcess(r reader.Reader) (meridianv1.ReaderClient, func(), error) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	meridianv1.RegisterReaderServer(srv, New(r))
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		srv.Stop()
		return nil, nil, err
	}
	stop := func() { conn.Close(); srv.Stop(); lis.Close() }
	return meridianv1.NewReaderClient(conn), stop, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/readgrpc/ -count=1 -v 2>&1 | tail -15`
Expected: every subtest of `TestStatusCodeTable` PASS, `TestRoundTripMatchesFeedReader` PASS, `TestMismatchMappingPreservesEveryField` PASS, `ok`.

- [ ] **Step 5: Print the commit for the operator**

```bash
cd ~/dev/meridian
git add internal/readgrpc/
git commit -m "feat: gRPC adapter over Reader with fixed status mapping and bufconn client"
```

---

### Task 5: `meridian serve`

**Files:**
- Modify: `cmd/meridian/main.go` (imports; new `case "serve":` before `default:`; usage line)
- Test: `cmd/meridian/main_test.go` (append)

**Interfaces:**
- Consumes: `reader.FeedReader`, `readgrpc.New`, `meridianv1.RegisterReaderServer`.
- Produces: subcommand `serve --feed F [--listen 127.0.0.1:0]`; prints `listening addr=<host:port>` on stdout; exit 1 on missing `--feed` or missing feed file, creating nothing.

- [ ] **Step 1: Failing tests**

Append to `cmd/meridian/main_test.go`:

```go
func TestServeAnswersHeadOverTCP(t *testing.T) {
	bin := build(t)
	feedPath := filepath.Join("..", "..", "fixtures", "base", "feed.jsonl")
	cmd := exec.Command(bin, "serve", "--feed", feedPath, "--listen", "127.0.0.1:0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("no listening line: %v", err)
	}
	if !strings.HasPrefix(line, "listening addr=") {
		t.Fatalf("first stdout line = %q", line)
	}
	addr := strings.TrimSpace(strings.TrimPrefix(line, "listening addr="))
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, err := meridianv1.NewReaderClient(conn).Head(ctx, &meridianv1.HeadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := reader.FeedReader{Path: feedPath}.Head(ctx)
	if h.GetRecords() != want.Records || h.GetPrefixHash() != want.PrefixHash {
		t.Fatalf("Head over TCP = %v, want %+v", h, want)
	}
}

func TestServeRefusesMissingFeedAndCreatesNothing(t *testing.T) {
	bin := build(t)
	missing := filepath.Join(t.TempDir(), "nope.jsonl")
	stdout, stderr, code := runSplit(t, bin, "serve", "--feed", missing)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "feed does not exist") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("serve created the missing feed")
	}
	_, stderr, code = runSplit(t, bin, "serve")
	if code != 1 || !strings.Contains(stderr, "--feed") {
		t.Fatalf("serve without --feed: code=%d stderr=%q", code, stderr)
	}
}
```

Add imports to the test file: `bufio`, `context`, `time`, `google.golang.org/grpc`, `google.golang.org/grpc/credentials/insecure`, `meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"`, `"github.com/hossainpazooki/meridian/internal/reader"`.

Run: `go test ./cmd/meridian/ -run Serve -count=1` -> FAIL (`unknown command: serve`, exit 1, no listening line).

- [ ] **Step 2: Implement**

In `cmd/meridian/main.go`: add imports `context`, `net`, `os/signal`, `syscall`, `google.golang.org/grpc`, `meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"`, `"github.com/hossainpazooki/meridian/internal/reader"`, `"github.com/hossainpazooki/meridian/internal/readgrpc"`. Change the usage line to `usage: meridian <append|replay|snapshot|asof|reconcile|serve> [flags]`. Add before `default:`:

```go
	case "serve":
		// Read-only gRPC over the feed. Nothing here writes: the service
		// has three read methods (api/meridian/v1/read.proto) and this
		// command has no flag that mutates anything.
		listen := fs.String("listen", "127.0.0.1:0", "listen address (host:port; port 0 picks a free port)")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if *feedPath == "" {
			return fail(errors.New("missing required flag(s): --feed"))
		}
		if err := requireFeedExists(*feedPath); err != nil {
			return fail(err)
		}
		lis, err := net.Listen("tcp", *listen)
		if err != nil {
			return fail(err)
		}
		srv := grpc.NewServer()
		meridianv1.RegisterReaderServer(srv, readgrpc.New(reader.FeedReader{Path: *feedPath}))
		fmt.Printf("listening addr=%s\n", lis.Addr().String())
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		go func() {
			<-ctx.Done()
			srv.GracefulStop()
		}()
		if err := srv.Serve(lis); err != nil {
			return fail(err)
		}
```

Also update the package doc comment's first sentence to mention `serve`: "... reconcile compares, serve answers the three read RPCs over gRPC."

- [ ] **Step 3: Run the CLI tests**

Run: `go test ./cmd/meridian/ -count=1`
Expected: `ok` (all pre-existing CLI tests plus the two new ones). Known limit, to be stated in STATUS: the TCP test ends the process with `Kill`, so graceful stop on SIGINT/SIGTERM is not exercised by a test (Windows cannot deliver those signals to a child).

- [ ] **Step 4: Print the commit for the operator**

```bash
cd ~/dev/meridian
git add cmd/meridian/
git commit -m "feat: meridian serve, read-only gRPC over the feed"
```

---

### Task 6: Generator emits the `p7` manifest section

**Files:**
- Modify: `fixtures/generate.py` (in `main()`, immediately before `man = {`; and the `"p6"` entry gains a sibling `"p7"`)
- Regenerate: `fixtures/base/manifest.json` (the ONLY fixture file that may change)

**Interfaces:**
- Consumes (already in scope in `main()`): `recs`, `N`, `V1, V2, V3`, `mi` (0-based index of the P2-mutated event; mutated seq is `mi + 1`), `mut_recs`, `mut_fold`, `honest_st`, helpers `statement`, `holdings_diff`, `die`.
- Produces manifest keys read by Task 7:
  - `p7.viewpoints` = `["V1","V2","V3"]` (names into top-level `viewpoints`)
  - `p7.wrong_feed` = `"p2/mutated/feed.jsonl"`
  - `p7.twin_wrong_feed.{mutation, mutated_rows, expected_violations{head_matches_local, snapshot_rehash_matches_claimed, snapshot_matches_local_recompute, reconcile_matches_local}}`
  - `p7.twin_mislabeled.{...same four keys...}`

- [ ] **Step 1: Add the measurement block**

Insert before `man = {` in `fixtures/generate.py`:

```python
    # ----- P7 twins: wire fidelity -----
    # Twin 1 (wrong_feed_served_as_base) is the Go gate serving
    # fixtures/p2/mutated/feed.jsonl to a client that believes it is the base
    # feed. Every expectation below is MEASURED from the two feeds, never
    # asserted; each guard says why the number could not be anything else.
    p7_viewpoints = ["V1", "V2", "V3"]
    p7_vseqs = [V1, V2, V3]
    # head_matches_local examines two fields: records and prefix_hash. The
    # mutated feed keeps the record count and re-chains, so exactly one of
    # the two differs. Both halves are checked, not assumed.
    if len(mut_recs) != N:
        die("P7 wrong-feed twin changed the record count (%d vs %d); head_matches_local footprint assumes records equal" % (len(mut_recs), N))
    if mut_recs[-1]["line_hash"] == recs[-1]["line_hash"]:
        die("P7 wrong-feed twin has the base feed's prefix hash; head_matches_local would read 0")
    p7_head = 1
    # snapshot_matches_local_recompute: a viewpoint at or past the mutated
    # seq yields different bytes (feed_prefix_hash alone guarantees it); a
    # viewpoint BEFORE it would be byte-identical and must not be counted.
    p7_mutated_seq = mi + 1
    p7_recompute = sum(1 for v in p7_vseqs if v >= p7_mutated_seq)
    if p7_recompute != len(p7_vseqs):
        die("P7 wrong-feed twin: a viewpoint precedes the mutated seq %d, so snapshot_matches_local_recompute cannot cover every viewpoint" % p7_mutated_seq)
    # reconcile_matches_local: the Go server reconciles ITS snapshot (the
    # mutated fold at end_seq) against the BASE statement; the local side
    # reconciles the base snapshot against the same statement and finds
    # nothing (P5 live). The symmetric difference is therefore the server's
    # own mismatch list: one entry per (instrument, field) pair that differs,
    # plus one for cash if it differs. holdings_diff's "*presence*" marker
    # would map to TWO Go mismatches (quantity and cost_basis), so the
    # position sets must be equal for this count to mean what it says.
    mut_st = statement(mut_fold)
    p7_pairs = holdings_diff(honest_st["holdings"], mut_st["holdings"])
    if any(field == "*presence*" for _, field in p7_pairs):
        die("P7 wrong-feed twin changed the position set: %r" % p7_pairs)
    p7_reconcile = len(p7_pairs) + (1 if mut_st["cash"] != honest_st["cash"] else 0)
    if p7_reconcile == 0:
        die("P7 wrong-feed twin leaves cash and every holding untouched; reconcile_matches_local would prove nothing")
    # Twin 2 (hash_field_mislabeled) serves correct bytes with a wrong
    # snapshot_hash on every AsOf answer, so the rehash check fires once per
    # viewpoint and nothing else moves.
    p7_mislabeled = len(p7_vseqs)
```

And add to the `man` dict, after the `"p6"` entry:

```python
        "p7": {"viewpoints": p7_viewpoints, "wrong_feed": "p2/mutated/feed.jsonl",
               "twin_wrong_feed": {"mutation": "wrong_feed_served_as_base", "mutated_rows": 1,
                                   "expected_violations": {"head_matches_local": p7_head, "snapshot_rehash_matches_claimed": 0,
                                                            "snapshot_matches_local_recompute": p7_recompute, "reconcile_matches_local": p7_reconcile}},
               "twin_mislabeled": {"mutation": "hash_field_mislabeled", "mutated_rows": p7_mislabeled,
                                   "expected_violations": {"head_matches_local": 0, "snapshot_rehash_matches_claimed": p7_mislabeled,
                                                            "snapshot_matches_local_recompute": 0, "reconcile_matches_local": 0}}},
```

- [ ] **Step 2: Regenerate and confirm only the manifest moved**

Run:

```sh
python fixtures/generate.py
git status --short fixtures/
python -c "import json;print(json.dumps(json.load(open('fixtures/base/manifest.json'))['p7'],indent=1))"
```

Expected: `ok base_end_seq=71 p6_end_seq=...`; `git status` shows exactly ` M fixtures/base/manifest.json`; the printed `p7` block shows `snapshot_matches_local_recompute: 3`, `head_matches_local: 1`, `reconcile_matches_local` a positive integer (record its value in the commit message; 2 is expected: cash and CCC cost_basis both move when a buy price changes by 1, but the NUMBER IS WHAT THE GENERATOR PRINTS, not this sentence). If any other fixture file changed, STOP: the RNG stream was disturbed and the plan is wrong.

- [ ] **Step 3: Freshness gate**

Run: `sh fixtures/generate_test.sh`
Expected: `ok fixtures deterministic and fresh`.

- [ ] **Step 4: Negative control on one guard**

Temporarily insert `p7_recompute = 0` on the line after `p7_recompute = sum(...)`, then run `python fixtures/generate.py --out fixtures/.regen/ctl`. Expected: the `die(...)` line `P7 wrong-feed twin: a viewpoint precedes the mutated seq ...` on stderr and exit code 1. Revert the inserted line and `rm -rf fixtures/.regen`. This proves the guard is wired, not decorative.

- [ ] **Step 5: Print the commit for the operator**

```bash
cd ~/dev/meridian
git add fixtures/generate.py fixtures/base/manifest.json
git commit -m "feat: generator measures P7 twin footprints into the manifest"
```

---

### Task 7: The P7 gate and seven-property claimability

**Files:**
- Create: `gates/p7_test.go`
- Modify: `gates/claimability.py:20` (`PROPS`), `:180` (`ok lane1 claimable=%d/6` -> `/7`)

**Interfaces:**
- Consumes: `readgrpc.InProcess`, `reader.FeedReader`, `reader.Reader`, `reader.AsOf`, `meridianv1.*`, `reconcile.LoadStatementBytes/Reconcile`, gate helpers `LoadManifest`, `ReadFixture`, `NewCounts`, `Emit`, `Row`, `ptr`, `FixturesDir`, `canon.SHA256Hex`, manifest `p7.*` keys (Task 6).
- Produces: verdict rows `meridian-lane1-p7-live-*.json`, two `meridian-lane1-p7-twin-*.json`.

- [ ] **Step 1: Write the gate**

`gates/p7_test.go`:

```go
package gates

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reader"
	"github.com/hossainpazooki/meridian/internal/readgrpc"
	"github.com/hossainpazooki/meridian/internal/reconcile"
)

// P7, wire fidelity: what a gRPC client receives is byte-for-byte what a
// local recompute over fixtures/base produces, and the client can tell when
// it is not. Every cell drives the SAME adapter over the SAME in-process
// transport; only the Reader behind it differs.

// mismatchKey flattens a reconcile mismatch to a comparable string so the
// server's list and the local list can be set-differenced.
func mismatchKey(instrument, field string, ledger, custodian, delta int64) string {
	return strings.Join([]string{instrument, field, strconv.FormatInt(ledger, 10), strconv.FormatInt(custodian, 10), strconv.FormatInt(delta, 10)}, "|")
}

// symDiff counts entries present in one multiset but not the other.
func symDiff(a, b []string) int64 {
	count := map[string]int64{}
	for _, k := range a {
		count[k]++
	}
	for _, k := range b {
		count[k]--
	}
	var n int64
	for _, v := range count {
		if v < 0 {
			v = -v
		}
		n += v
	}
	return n
}

// p7Check drives one client and returns the counts plus the V3 snapshot
// hash the client received (the row's content hash).
func p7Check(t *testing.T, client meridianv1.ReaderClient, m Manifest) (Counts, string) {
	t.Helper()
	ctx := context.Background()
	c := NewCounts("head_matches_local", "snapshot_rehash_matches_claimed", "snapshot_matches_local_recompute", "reconcile_matches_local")
	basePath := filepath.Join(FixturesDir, "base", "feed.jsonl")

	// head: two fields examined against a direct open of the base feed.
	f, err := feed.Open(basePath)
	if err != nil {
		t.Fatal(err)
	}
	localRecords := f.Len()
	localPrefix, err := f.PrefixHash(localRecords)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	h, err := client.Head(ctx, &meridianv1.HeadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	c.Evaluated["head_matches_local"] = 2
	if h.GetRecords() != localRecords {
		c.Checks["head_matches_local"]++
	}
	if h.GetPrefixHash() != localPrefix {
		c.Checks["head_matches_local"]++
	}

	// snapshots: one AsOf per manifest viewpoint, in name order.
	names := m.Strs("p7", "viewpoints")
	sort.Strings(names)
	c.Evaluated["snapshot_rehash_matches_claimed"] = int64(len(names))
	c.Evaluated["snapshot_matches_local_recompute"] = int64(len(names))
	var lastHash string
	for _, name := range names {
		v := m.Int("viewpoints", name)
		resp, err := client.AsOf(ctx, &meridianv1.AsOfRequest{Seq: v})
		if err != nil {
			t.Fatalf("AsOf %s (%d): %v", name, v, err)
		}
		if "sha256:"+canon.SHA256Hex(resp.GetSnapshot()) != resp.GetSnapshotHash() {
			c.Checks["snapshot_rehash_matches_claimed"]++
		}
		local := ReadFixture(t, "base/feed.jsonl", v)
		if resp.GetSeq() != v || !bytes.Equal(resp.GetSnapshot(), local.Bytes) {
			c.Checks["snapshot_matches_local_recompute"]++
		}
		lastHash = resp.GetSnapshotHash()
	}

	// reconcile at end_seq against the base statement; the universe is
	// every field the LOCAL reconcile compared.
	endSeq := m.Int("end_seq")
	raw, err := os.ReadFile(filepath.Join(FixturesDir, "base", "statement.json"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := reconcile.LoadStatementBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	localMs, localCompared, err := reconcile.Reconcile(ReadFixture(t, "base/feed.jsonl", endSeq).Doc, st)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Reconcile(ctx, &meridianv1.ReconcileRequest{Seq: endSeq, Statement: raw})
	if err != nil {
		t.Fatal(err)
	}
	var localKeys, wireKeys []string
	for _, x := range localMs {
		localKeys = append(localKeys, mismatchKey(x.Instrument, x.Field, x.Ledger, x.Custodian, x.Delta))
	}
	for _, x := range resp.GetMismatches() {
		wireKeys = append(wireKeys, mismatchKey(x.GetInstrument(), x.GetField(), x.GetLedger(), x.GetCustodian(), x.GetDelta()))
	}
	c.Evaluated["reconcile_matches_local"] = int64(localCompared)
	c.Checks["reconcile_matches_local"] = symDiff(localKeys, wireKeys)
	if resp.GetCompared() != int64(localCompared) {
		c.Checks["reconcile_matches_local"]++
	}
	return c, lastHash
}

// mislabeledReader serves the truth with a lie on the label: correct bytes,
// wrong snapshot_hash. A client that trusts the hash field instead of
// rehashing the bytes it received cannot see this.
type mislabeledReader struct{ reader.Reader }

func (r mislabeledReader) AsOf(ctx context.Context, seq int64) (reader.AsOf, error) {
	a, err := r.Reader.AsOf(ctx, seq)
	if err != nil {
		return a, err
	}
	b := []byte(a.SnapshotHash)
	last := len(b) - 1
	if b[last] == '0' {
		b[last] = '1'
	} else {
		b[last] = '0'
	}
	a.SnapshotHash = string(b)
	return a, nil
}

func p7Params(m Manifest) map[string]any {
	names := m.Strs("p7", "viewpoints")
	vp := map[string]any{}
	for _, n := range names {
		vp[n] = m.Int("viewpoints", n)
	}
	return map[string]any{"viewpoints": vp, "end_seq": m.Int("end_seq"), "listener": "bufconn"}
}

func TestP7WireFidelity(t *testing.T) {
	m := LoadManifest(t)
	params := p7Params(m)
	basePath := filepath.Join(FixturesDir, "base", "feed.jsonl")
	rows := int64(len(ReadFixture(t, "base/feed.jsonl", -1).State.Positions))

	// live
	live := reader.FeedReader{Path: basePath}
	client, stop, err := readgrpc.InProcess(live)
	if err != nil {
		t.Fatal(err)
	}
	c, hash := p7Check(t, client, m)
	stop()
	Emit(t, Row{Prop: 7, Cell: "live", Scope: "gRPC client over bufconn vs local recompute, fixtures/base", ContentHash: hash,
		Basis: "sha256 of canonical snapshot bytes as received by the gRPC client", Rows: rows, Params: params, Counts: c})

	// twin 1: a valid, re-chained feed that is not the base feed.
	wrong := reader.FeedReader{Path: filepath.Join(FixturesDir, filepath.FromSlash(m.Str("p7", "wrong_feed")))}
	client, stop, err = readgrpc.InProcess(wrong)
	if err != nil {
		t.Fatal(err)
	}
	cw, hw := p7Check(t, client, m)
	stop()
	Emit(t, Row{Prop: 7, Cell: "twin", Scope: "gRPC client over bufconn vs local recompute; server reads fixtures/p2/mutated as if it were fixtures/base", ContentHash: hw,
		Basis: "sha256 of canonical snapshot bytes as received by the gRPC client (twin server)", Rows: rows, Params: params, Counts: cw, Planted: ptr(m.Planted("p7", "twin_wrong_feed"))})

	// twin 2: correct bytes, mislabeled hash.
	client, stop, err = readgrpc.InProcess(mislabeledReader{live})
	if err != nil {
		t.Fatal(err)
	}
	cm, hm := p7Check(t, client, m)
	stop()
	Emit(t, Row{Prop: 7, Cell: "twin", Scope: "gRPC client over bufconn vs local recompute; server mislabels snapshot_hash on every AsOf", ContentHash: hm,
		Basis: "snapshot_hash field as received (deliberately wrong); the bytes rehash to the base V3 hash", Rows: rows, Params: params, Counts: cm, Planted: ptr(m.Planted("p7", "twin_mislabeled"))})
}
```

- [ ] **Step 2: Run the gate alone and read the rows**

Run:

```sh
rm -rf gates/out && MERIDIAN_VERDICT_DIR="$PWD/gates/out" go test ./gates/ -run TestP7 -count=1 -v 2>&1 | tail -5
ls gates/out
```

Expected: `--- PASS: TestP7WireFidelity`, three files `meridian-lane1-p7-live-*.json`, `meridian-lane1-p7-twin-*.json` x2. If `Emit` refuses a twin with `caught N, planted M`, the generator's measurement and the Go check disagree: read both, decide which is wrong, and fix THAT side. Never edit `expected_violations` to match.

- [ ] **Step 3: Seven properties in claimability.py**

Change `PROPS = [1, 2, 3, 4, 5, 6]` to `PROPS = [1, 2, 3, 4, 5, 6, 7]` and `print("ok lane1 claimable=%d/6" % k)` to `print("ok lane1 claimable=%d/7" % k)`.

- [ ] **Step 4: Full runner**

Run: `sh gates/run.sh 2>&1 | tail -12 && ls gates/out/*.json | awk 'END{print "rows:", NR}'`
Expected: the table has a `P7   | GREEN | RED*,RED*          | YES` line, last line `ok lane1 claimable=7/7`, rows: 18. (`STATUS.md` is not yet updated, so claimability.py prints `WARN P7 is supported by verdicts but STATUS.md does not mark it CLAIMABLE`; that warning is expected here and disappears in Task 8.)

- [ ] **Step 5: Print the commit for the operator**

```bash
cd ~/dev/meridian
git add gates/p7_test.go gates/claimability.py
git commit -m "feat: P7 wire-fidelity gate, two twins, claimability to seven"
```

---

### Task 8: State of record

**Files:**
- Modify: `STATUS.md` (new dated entry after the 2026-09-01 entry, before `## Crediting rule`; table row after `| P6 |` at line 78; twins paragraph at line 87; new honest limit before `- **No production claim.**` at line 148; deferred line 170)
- Modify: `README.md` (`## Run the gates` block)
- Create: `docs/learnings/2026-09-03-eighteen-verdict-rows-after-p7.md`; append pointer to `docs/learnings/LEARNINGS.md`
- The handoff entry is written at session close via the `rigor:handoff` skill, not here.

- [ ] **Step 1: STATUS.md dated entry**

Insert after the 2026-09-01 entry's last paragraph (the one ending `under `p<N>.twin.expected_violations`.`):

```markdown
- **2026-09-03** -- P7 (wire fidelity of the gRPC read API) built. `sh
  gates/run.sh` ends `ok lane1 claimable=7/7` over **18 verdict rows** (the
  15 above plus P7: one live + two twins). The API is `Head` / `AsOf` /
  `Reconcile`, read-only by construction of `api/meridian/v1/read.proto`
  (a descriptor test pins exactly those three unary methods). The gate is a
  gRPC client over an in-process listener that rehashes the bytes it
  receives and recomputes locally; twin 1 serves `fixtures/p2/mutated` as
  if it were base (self-consistent hashes, wrong content), twin 2 serves the
  right bytes under the wrong `snapshot_hash`. Design:
  `docs/2026-09-03-grpc-read-api-design.md`.
```

- [ ] **Step 2: Table row, twins paragraph, deferred line**

After the P6 row add:

```markdown
| P7 | Wire fidelity of the gRPC read API (2 twins) | GREEN | RED | CLAIMABLE |
```

In the paragraph beginning `The Twin column holds one word per property`, change `**P4 has two twin rows and P6 has three**` to `**P4 and P7 have two twin rows each and P6 has three**`, and append after the P6 twins sentence: `P7's are `wrong_feed_served_as_base` and `hash_field_mislabeled`.`

Also change the crediting-rule sentence `**P4 has two twins and P6 has three, and all of them must hold**` to `**P4 and P7 have two twins each and P6 has three, and all of them must hold**`.

Replace the deferred line `- gRPC read API -- revisit after P1-P3 are green; read-only if built.` with:

```markdown
- gRPC read API -- **resolved** 2026-09-03: built read-only as P7; see the
  dated entry above and `docs/2026-09-03-grpc-read-api-design.md`.
```

- [ ] **Step 3: Honest limit**

Insert before `- **No production claim.**`:

```markdown
- **P7 measures an in-process listener, not a network.** Its verdict rows
  come from a `bufconn` transport inside one test process. Nothing is
  claimed about TCP behaviour under load, TLS, authentication,
  authorisation, concurrency, or backpressure. `meridian serve` over
  loopback TCP is exercised by one smoke test (`Head` answered over the
  wire) that emits no row and ends the process with a kill, so graceful
  stop on SIGINT/SIGTERM is untested (Windows cannot deliver those signals
  to a child process). "Read-only" is a statement about the proto -- three
  methods, all reads, pinned by a descriptor test -- not about the serving
  process's filesystem permissions.
```

- [ ] **Step 4: README line**

In `## Run the gates`, after the `sh gates/run.sh` line add:

```
    bin/meridian serve --feed fixtures/base/feed.jsonl   # read-only gRPC: Head / AsOf / Reconcile (api/meridian/v1/read.proto)
```

Then run `grep -oE 'CLAIMABLE|GREEN|RED\*' README.md | wc -l` -> `0`.

- [ ] **Step 5: Learnings entry**

`docs/learnings/2026-09-03-eighteen-verdict-rows-after-p7.md`:

```
ts: <UTC now, e.g. 2026-09-03T18:00:00Z>
commit: <sha of the P7 gate commit once the operator has made it; "uncommitted" until then>
session: claude-code meridian grpc read api
status: verified
kills: 2026-09-01-bare-go-test-appends-verdict-rows.md (the row count only)
fact: A clean `sh gates/run.sh` now produces 18 verdict rows, not 15: P7 adds one live row and two twin rows (wrong_feed_served_as_base, hash_field_mislabeled). The rest of the killed entry stands -- a bare `go test` still appends, only run.sh clears -- but anyone checking the row count before suspecting a gate must now expect 18.
basis: `sh gates/run.sh >/dev/null 2>&1 && ls gates/out/*.json | wc -l` -> `18`; `python gates/claimability.py gates/out --status STATUS.md` last line `ok lane1 claimable=7/7`.
re-verify: sh gates/run.sh >/dev/null 2>&1 && ls gates/out/*.json | wc -l
```

Append to `docs/learnings/LEARNINGS.md`:

```markdown
- [2026-09-03 -- eighteen verdict rows after P7](2026-09-03-eighteen-verdict-rows-after-p7.md) -- kills the "15 rows" figure: a clean run.sh now writes 18 (P7 = 1 live + 2 twins); the append-only warning in the killed entry still holds.
```

- [ ] **Step 6: The whole gate, from clean**

Run:

```sh
sh gates/run.sh 2>&1 | tail -12
ls gates/out/*.json | awk 'END{print "rows:", NR}'
go test ./... -count=1 2>&1 | awk '/^ok/{n++} END{print "ok packages:", n+0}'
grep -o CLAIMABLE STATUS.md | awk 'END{print "CLAIMABLE:", NR}'
git diff --check && echo "diff-check clean"
```

Expected: `ok lane1 claimable=7/7` with NO `WARN`/`FAIL` line; rows: 18; ok packages: 11 (the 8 existing + `api/meridian/v1`, `internal/reader`, `internal/readgrpc`); CLAIMABLE: 8 (seven table cells + the crediting rule; the 09-03 entry says `claimable=7/7` in lowercase, which the case-sensitive grep does not count); diff-check clean. Then re-run `sh gates/run.sh` once more because the `go test ./...` above appended rows.

- [ ] **Step 7: Print the commit for the operator**

```bash
cd ~/dev/meridian
git add STATUS.md README.md docs/learnings/
git commit -m "docs: P7 claimable, honest limit for in-process gate, 18-row learning"
git push
```

After the push, verify CI: `gh run list --limit 1 --json conclusion,headSha --jq '.[0]'` -> `success` on the pushed sha, and `gh run view <id> --log | grep -E "ok proto fresh|ok lane1 claimable"` -> both lines. CI's first run downloads the tool modules; if `go tool buf generate` fails there for a platform reason, that is a red gate and the fix precedes any claim.

---

## Integration gate (terminal step)

Dispatch `rigor:integration-runner` (or run it yourself) with exactly this brief: "From a clean tree at the last commit, run `sh fixtures/generate_test.sh`, `sh gates/run.sh`, `go test ./... -count=1`, `go vet ./...`; report the last line of run.sh, the row count in `gates/out/`, the ok-package count, and the CI conclusion for the pushed sha. Iterate until run.sh ends `ok lane1 claimable=7/7` with no WARN/FAIL. Then dispatch `rigor:skeptic-verifier` on two claims: (1) 'the P7 twins would go GREEN if their check were removed' -- verify by temporarily deleting `snapshot_matches_local_recompute` from twin 1's counts and `snapshot_rehash_matches_claimed` from twin 2's and confirming `Emit` refuses (planted expectation never computed); (2) 'the proto-fresh step fires on drift' -- verify by editing a generated file and watching run.sh FAIL." Nothing in this plan is claimable until that report exists.
