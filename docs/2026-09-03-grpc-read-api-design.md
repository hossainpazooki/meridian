# MERIDIAN -- gRPC Read API (P7, wire fidelity): Design

*2026-09-03 -- output of the brainstorm that closed design question Q5 (deferred
in `2026-08-31-design.md` until P1-P3 were green; they are, since `f8825ba`).
Operator ruling 2026-09-03: build it, read-only, with its own gate and twins.
This is the validated design; nothing here is claimable until its gate is
green and every twin is red. The accessory becomes a property (P7) only
because the operator chose gate + twin over tests-only.*

---

## 0. Decisions

| Question | Decision |
|---|---|
| Surface | Three unary RPCs: `Head`, `AsOf`, `Reconcile`. No write RPC exists in the proto; a test asserts the service descriptor has exactly three methods. |
| Snapshot on the wire | Canonical snapshot **bytes** exactly as `asof.Read` produced them, plus the sha256 the server computed. Never re-serialised into typed messages: a second serialisation is a second thing a client could trust instead of rehashing. |
| Claim status | **Property P7, wire fidelity**, one live cell + two twins, surface `meridian-lane1-p7`. Not a fourth lane: it sits over Lane 1's feed and fold. |
| Seam | A Go `Reader` interface. The gRPC server adapts any `Reader`; the live one wraps `asof`/`reconcile`; the twins are misbehaving `Reader`s behind the same adapter. This is the "Reader protocol" `2026-08-31-design.md` names for Lane 2, made concrete. |
| Gate transport | In-process `bufconn`. No ports in a determinism repo. TCP is exercised by one CLI smoke test of `serve`, which emits no verdict row. |
| Codegen | `buf` and the two protoc plugins pinned as Go 1.26 `tool` directives; generated `*.pb.go` committed; `run.sh` gains a `proto fresh` step that regenerates and diffs. |
| Reader semantics | Per-request `feed.Open`: pure recompute, no cached state, no stale handle. Matches the CLI. |

## 1. Contract -- `api/meridian/v1/read.proto`

Package `meridian.v1`, Go package `github.com/hossainpazooki/meridian/api/meridian/v1;meridianv1`.

```proto
service Reader {
  rpc Head(HeadRequest) returns (HeadResponse);
  rpc AsOf(AsOfRequest) returns (AsOfResponse);
  rpc Reconcile(ReconcileRequest) returns (ReconcileResponse);
}
message HeadRequest {}
message HeadResponse { int64 records = 1; string prefix_hash = 2; }
message AsOfRequest  { int64 seq = 1; }            // < 0 means end of feed, as the CLI
message AsOfResponse { int64 seq = 1; string prefix_hash = 2; string snapshot_hash = 3; bytes snapshot = 4; }
message ReconcileRequest  { int64 seq = 1; bytes statement = 2; }   // custodian JSON, same bytes the CLI loads
message Mismatch { string instrument = 1; string field = 2; int64 ledger = 3; int64 custodian = 4; int64 delta = 5; }
message ReconcileResponse { int64 compared = 1; repeated Mismatch mismatches = 2; }
```

`AsOfResponse.seq` is the **resolved** seq (so a `-1` request answers with the
actual end seq). `prefix_hash` and `snapshot_hash` carry the `sha256:` prefix
exactly as the CLI prints them. `statement` is bytes, not a typed message, so
there is one statement schema (`reconcile.LoadStatement`'s), not two.
`Mismatch` mirrors `reconcile.Mismatch` field for field; the empty
`instrument` on a cash mismatch is preserved.

Status codes, fixed and tested:

| Condition | Code |
|---|---|
| feed path does not exist | `NOT_FOUND` |
| `feed.ChainError` | `DATA_LOSS` |
| `seq` > records, or statement not valid JSON / not the statement shape | `INVALID_ARGUMENT` |
| anything else | `INTERNAL` |

## 2. Server side

- **`internal/reader`** -- `type Reader interface { Head(ctx) (Head, error); AsOf(ctx, seq int64) (AsOf, error); Reconcile(ctx, seq int64, statement []byte) (Recon, error) }` with plain Go result structs (no protobuf types leak into `internal/`). `FeedReader{Path}` implements it: `Head` opens the feed and returns `Len()` + `PrefixHash(Len())`; `AsOf` is `asof.Read`; `Reconcile` is `asof.Read` + `reconcile.LoadStatementBytes` + `reconcile.Reconcile`. A missing path is `reader.ErrNotFound` (the CLI's `requireFeedExists` rule, moved where both can use it -- `feed.Open` would otherwise create a clean empty feed and answer green over nothing). Sentinel errors: `ErrNotFound`, `ErrSeqOutOfRange`, `ErrBadStatement`.
- **`internal/reconcile`** -- gains `LoadStatementBytes([]byte)`; `LoadStatement(path)` becomes a thin wrapper. No behaviour change.
- **`internal/readgrpc`** -- `New(r reader.Reader) meridianv1.ReaderServer`. Pure mapping: result structs to messages, sentinel/`ChainError` to the codes above. No logic.
- **`cmd/meridian serve`** -- flags `--feed` (required, must exist) and `--listen` (default `127.0.0.1:0`). Prints `listening addr=<host:port>` on stdout once bound, serves until SIGINT/SIGTERM, exits 0. The four existing commands are untouched. `serve` has no flag that writes.
- **Codegen** -- `buf.yaml` (module root `api`), `buf.gen.yaml` with `protoc-gen-go` and `protoc-gen-go-grpc` invoked as `go tool ...`; `go.mod` gains `google.golang.org/grpc`, `google.golang.org/protobuf` as deps and `github.com/bufbuild/buf/cmd/buf`, `google.golang.org/protobuf/cmd/protoc-gen-go`, `google.golang.org/grpc/cmd/protoc-gen-go-grpc` as `tool` directives, versions pinned by `go.mod`. Generated files are committed and LF-normalised by the existing `.gitattributes`. `gates/run.sh` step `== proto fresh` runs `go tool buf generate --output <tmp>` and diffs against the committed files; drift fails the run. CI needs no new setup step: `setup-go` restores the module cache including the tools.

Known cost: the `tool` directives pull buf's dependency graph into `go.sum`
(not into any binary). Chosen over `go run pkg@version` because the module
cache is what CI caches.

## 3. The P7 gate -- `gates/p7_test.go`

**Property.** What a gRPC client receives is byte-for-byte what a local
recompute produces, and the client can tell when it is not.

**Harness.** One helper starts `readgrpc.New(r)` on a `bufconn` listener and
returns a connected client; the live cell and both twins use it, so the
transport is identical across cells and only the `Reader` differs.

**Checks** (names are the verdict-row keys; `evaluated` is the universe
examined, per locked decision 5):

| Check | Evaluated | Violation counts |
|---|---|---|
| `head_matches_local` | 2 (records, prefix_hash) | each field of `Head` that differs from `feed.Open` on the base feed |
| `snapshot_rehash_matches_claimed` | 3 (V1, V2, V3) | viewpoints where `sha256(snapshot bytes) != snapshot_hash` |
| `snapshot_matches_local_recompute` | 3 (V1, V2, V3) | viewpoints where the received bytes are not byte-equal to `asof.Read(base, V).Bytes` (bytes, not hashes; `seq` and `prefix_hash` must also equal the local values). Amended 2026-09-03 at final review: the `prefix_hash` leg was missing from the original table. |
| `reconcile_matches_local` | local `compared` (11 on the base fixture: cash + 2 fields x 5 instruments) | size of the symmetric difference between the server's mismatch set and the local one over `fixtures/base/statement.json` at `end_seq`, plus 1 if `compared` differs |

Live cell: `FeedReader{fixtures/base/feed.jsonl}`. Every check 0. Scope
`"gRPC client over bufconn vs local recompute, fixtures/base"`; content hash =
the V3 snapshot hash received over the wire; basis
`"sha256 of canonical snapshot bytes as received by the gRPC client"`.

**Twin 1 -- `wrong_feed_served_as_base`** (manifest key `p7.twin_wrong_feed`).
`FeedReader{fixtures/p2/mutated/feed.jsonl}`: a valid chain (verified: 71
records, opens cleanly) whose seq-3 fill carries price 2363 instead of 2362,
served to a client that believes it is the base feed. Hashes are
self-consistent; the content is wrong. Expected: `head_matches_local` 1
(prefix_hash differs, records equal), `snapshot_rehash_matches_claimed` 0,
`snapshot_matches_local_recompute` 3 (all viewpoints are >= 3), and
`reconcile_matches_local` = the number of `(instrument, field)` pairs on which
the mutated feed's naive fold differs from the base statement -- **measured by
`generate.py`** via `holdings_diff` plus the cash comparison, never
hard-coded. This twin shows the recompute check is load-bearing.

**Twin 2 -- `hash_field_mislabeled`** (manifest key `p7.twin_mislabeled`). A
`Reader` wrapping the live `FeedReader` that returns the correct bytes and a
`snapshot_hash` that is the sha256 of the bytes with one hex digit changed.
Expected: `snapshot_rehash_matches_claimed` = number of viewpoints (derived in
the generator as `len(viewpoints)`, 3 today), every other check 0. This twin
shows the rehash check is load-bearing and that trusting the hash field alone
is a hole.

Both twins are Go `Reader` implementations in the gate's test file (the
misbehaviour) whose **expected counts live in `fixtures/base/manifest.json`**
(the authority), so `Emit` refuses any drift between the two, exactly as for
P1-P6. Generator guards: the wrong-feed reconcile footprint must be > 0 (a
price mutation that left cash and cost basis untouched would mean the twin
proves nothing), and the mutated feed's viewpoint V1 must be >= the mutated
seq (otherwise `snapshot_matches_local_recompute` cannot be `len(viewpoints)`
and the generator must say so rather than emit 3).

**Row count.** 1 live + 2 twins = 3 new rows; a clean `run.sh` now produces
**18** rows.

## 4. State of record

- `gates/claimability.py`: `PROPS = [1..7]`, success line `ok lane1 claimable=N/7`.
- `STATUS.md`: dated 2026-09-03 entry; P7 row `| P7 | Wire fidelity of the gRPC read API (2 twins) | GREEN | RED | CLAIMABLE |` only once both cells are earned and CI is green; the "gRPC read API" line under deferred decisions moves to resolved; twins named in the paragraph that names P4's and P6's.
- New honest limit: **P7 measures an in-process listener.** The verdict rows come from `bufconn`; nothing is claimed about a network, TLS, authentication, authorisation, concurrency, or backpressure. `serve` over loopback TCP is exercised by a smoke test that emits no row. The API is read-only by construction of the proto (three methods, all reads), which the descriptor test enforces -- and that is a statement about the proto, not about the process's filesystem permissions.
- `README.md`: one sentence under the run section naming `serve`; no counts, no claim language (the existing grep stays at 0).
- Learnings entry: the "15 rows" figure in `2026-09-01-bare-go-test-appends-verdict-rows.md` is superseded by a new entry with `kills:` -- 18 after P7.
- Handoff entry at close, per `docs/handoff/HANDOFF.md` convention.

## 5. Testing

- `internal/reader`: `FeedReader` against the base fixture (Head, AsOf at each viewpoint incl. `-1`, Reconcile with the base statement), missing path -> `ErrNotFound` and **no file created**, seq past end -> `ErrSeqOutOfRange`, malformed statement -> `ErrBadStatement`, chain error passes through as `*feed.ChainError`.
- `internal/readgrpc`: every row of the status-code table over `bufconn` with a stub `Reader`; descriptor has exactly `Head`, `AsOf`, `Reconcile`.
- `cmd/meridian`: `serve` binds `127.0.0.1:0`, prints the address, answers one `Head` over TCP, stops on signal; `serve` without `--feed` or with a missing feed exits 1 and creates nothing.
- `gates/p7_test.go`: live + two twins, emitted through `Emit`.
- `gates/verdict_test.go`: unchanged. `claimability.py` has no self-test today and gains none here; its enforcement is the `run.sh` success line and the STATUS.md overclaim check, both of which must now count seven.
- `fixtures/generate_test.sh`: still byte-identical after the manifest gains `p7`.

## 6. Non-goals (stated so they are not read as omissions)

- No streaming, no pagination, no watch/subscribe.
- No write RPC, ever, on this service. A feed-append transport is Lane 3's
  question and would drag transport into P1's claim.
- No auth, TLS, or reflection. No performance vocabulary.
- No typed snapshot schema on the wire.
- No change to the snapshot format, the feed format, or any P1-P6 gate.

*Rule carried from every prior seed: if a property can't get a twin, it isn't
a property -- it's a hope. P7 has two.*
