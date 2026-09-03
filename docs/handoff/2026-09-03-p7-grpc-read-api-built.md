# Handoff -- P7 gRPC read API built, uncommitted, final review closed

2026-09-03. Newest commit this brief describes: `e189259` (`docs: gRPC read API
design (P7 wire fidelity)`) on `main`. **Everything below sits UNCOMMITTED on
branch `p7-grpc-read-api`, forked from `e189259`.** In this project only the
human operator writes git history; the build's commit commands are in the
"Open / next" section. Pick-up measures drift from the working tree, not from
a commit range; a clean checkout has none of this.

## Current state

- **built** -- Read-only gRPC API over Lane 1: `api/meridian/v1/read.proto`
  (service `Reader`: `Head`, `AsOf`, `Reconcile`, all unary), generated code
  committed-to-be under `api/meridian/v1/`, `internal/reader` (the `Reader`
  seam + live `FeedReader`), `internal/readgrpc` (adapter + `InProcess`
  bufconn client), `meridian serve --feed F --listen 127.0.0.1:0`.
  re-verify: `go test ./... -count=1 2>&1 | grep -c '^ok'` -> 11
  re-verify: `go test ./api/... -run TestReaderServiceIsReadOnly -count=1` -> ok
- **built** -- Property P7 (wire fidelity) CLAIMABLE: live GREEN, two twins RED
  as planted (`wrong_feed_served_as_base` 1/0/3/2, `hash_field_mislabeled`
  0/3/0/0 over head / rehash / recompute / reconcile).
  re-verify: `sh gates/run.sh` -> last line `ok lane1 claimable=7/7`, no `WARN`/`FAIL`
  re-verify: `sh gates/run.sh >/dev/null 2>&1 && ls gates/out/*.json | wc -l` -> 18
- **built** -- Codegen pinned as Go 1.26 `tool` directives (buf v1.72.0,
  protoc-gen-go v1.36.12, protoc-gen-go-grpc v1.6.2; grpc v1.83.2, protobuf
  v1.36.12) and a fail-closed `proto fresh` step in `gates/run.sh` that
  enumerates every generated file both ways and refuses an empty generation.
  re-verify: `sh gates/run.sh 2>&1 | grep -c '^ok proto fresh'` -> 1
  re-verify: `grep -c '^tool' go.mod` -> 1 (block header; three entries inside)
- **built** -- Generator measures the P7 footprints into `manifest.json` `p7`
  (guards die if the mutation would make a count meaningless).
  re-verify: `sh fixtures/generate_test.sh` -> `ok fixtures deterministic and fresh`
- **built** -- STATUS.md: 2026-09-03 entry, P7 row, honest limit with six
  measured sub-limits from the final review, deferred line resolved.
  re-verify: `grep -o CLAIMABLE STATUS.md | wc -l` -> 8
  re-verify: `grep -oE 'CLAIMABLE|GREEN|RED\*' README.md | wc -l` -> 0
- **built** -- Review chain: 8 task reviews clean; integration-runner GREEN;
  skeptic: both claims SURVIVE (Emit refuses a removed check; proto-fresh
  fires on drift); final whole-branch review (Opus) MERGEABLE AFTER FIXES ->
  one fix pass -> re-review: all findings FIXED, one cosmetic `**` fixed by
  the controller afterwards. Ledger: `docs/2026-09-03-p7-build-ledger.md`.
- **not verified** -- CI. The branch is unpushed; `go tool buf generate` has
  only ever run on Windows. The first CI run on `ubuntu-latest` is the
  measurement (cold module cache builds buf from source; no network is
  expected for a local buf module with no `deps:`).
- **not started** -- Lanes 2 and 3; the three pre-existing unfalsified checks;
  D2; generator RNG via getrandbits; BASELINE registration seam.

## Locked decisions

1. **Snapshot bytes cross the wire verbatim; no typed snapshot messages.**
   Reason: a second serialisation is a second thing a client could trust
   instead of rehashing. (`docs/2026-09-03-grpc-read-api-design.md` section 0.)
2. **P7 is a property with a gate and two twins, not an accessory.** Reason:
   operator ruling 2026-09-03; the seed's purpose was converting a gRPC gap
   into a scoped claim, and only a red twin makes it claimable.
3. **The gate runs over `bufconn`; TCP is a smoke test that emits no row.**
   Reason: no ports in a determinism repo. Stated as an honest limit.
4. **Codegen via `tool` directives, generated files committed.** Reason: the
   module cache is what CI caches; `go run pkg@version` is uncached. Cost
   (~70 indirect go.mod entries) accepted in the design.
5. **Per-request `feed.Open` in `FeedReader`.** Reason: pure recompute, no
   cached state; appends by another process are visible on the next call.
   Its cost -- read-write open, exists/open race -- is recorded, not hidden.
6. **The design doc was amended in place, dated, at final review** (the
   `prefix_hash` leg). Reason: the spec described a weaker check than the
   code after the fix; the repo's immutability rule covers handoff and
   learnings entries, not the design. The operator should know a committed
   spec moved.

## Reuse map

- `internal/reader/reader.go` -- the seam Lane 2 (ClickHouse) plugs into.
  Implement `Reader`, hand it to `readgrpc.New`; the P7 gate runs unchanged.
- `internal/readgrpc/inprocess.go` -- `InProcess(r)` for any in-process gate
  or test that needs a real gRPC client without a port.
- `gates/p7_test.go` -- reference for a gate whose twins are BEHAVIOURS (lying
  Readers) rather than artifacts; `mismatchKey`/`symDiff` for list-vs-list
  checks with an honest denominator.
- `gates/run.sh` `== proto fresh` -- the shape for any generated-code
  freshness gate: enumerate, compare both ways, refuse empty.
- `.git/sdd/pkg.sh` (also in `sdd-lane1/`) -- working-tree review packages
  when nothing is committed.
- `docs/2026-09-03-p7-build-ledger.md` -- the build's progress ledger with
  every reviewer finding and its triage.

## Invariants

- **A check whose every twin plants 0 is unfalsified.** Emit refuses a missing
  key, not a dead increment path. Each P7 check has a nonzero expectation in
  some twin; keep it that way when adding checks.
- **Never edit `expected_violations` to make a gate pass.** The generator
  measures; the gate must agree.
- **Read-only is a statement about the proto.** The descriptor test pins three
  unary methods; it says nothing about filesystem permissions (reads open the
  feed read-write).
- **`gates/out/` is cleared only by `run.sh`; a bare `go test` appends.** Row
  count is 18 now; check it before suspecting a gate.
- **README carries no counts or claim language; STATUS.md is the record.**
- **All new files are LF and ASCII** (`.gitattributes`; the design doc is CRLF
  in the working copy and will normalise on the next git write -- harmless).

## Open / next

1. **Operator: commit and push.** One commit per concern, in dependency
   order, then push and open the PR (Lane 1's pattern). The Task 8 learnings
   entry's `commit:` field should be updated to the Task 7 sha in the docs
   commit.

```bash
cd ~/dev/meridian   # on p7-grpc-read-api
git add api/ buf.yaml buf.gen.yaml go.mod go.sum gates/run.sh .gitignore
git commit -m "feat: read API proto, pinned codegen, proto-fresh gate step"
git add internal/reconcile/
git commit -m "refactor: reconcile.LoadStatementBytes for byte-borne statements"
git add internal/asof/ internal/reader/
git commit -m "feat: Reader seam and FeedReader over asof/reconcile"
git add internal/readgrpc/
git commit -m "feat: gRPC adapter over Reader with fixed status mapping and bufconn client"
git add cmd/meridian/
git commit -m "feat: meridian serve, read-only gRPC over the feed"
git add fixtures/generate.py fixtures/base/manifest.json
git commit -m "feat: generator measures P7 twin footprints into the manifest"
git add gates/p7_test.go gates/claimability.py
git commit -m "feat: P7 wire-fidelity gate, two twins, claimability to seven"
# docs last: claims after evidence
git add STATUS.md README.md docs/
git commit -m "docs: P7 claimable, honest limits, design amendment, learnings, handoff, build ledger"
git push -u origin p7-grpc-read-api
```

2. **Watch the first CI run.** `gh run list --limit 1` then `gh run view <id>
   --log | grep -E "ok proto fresh|ok lane1 claimable"`. If `go tool buf
   generate` fails on Linux, that is a red gate and precedes any claim.
3. **After merge:** BASELINE registration of seven cells (seam undesigned);
   the three pre-existing unfalsified checks; the two P7 legs never driven
   non-zero (`records`, `compared`) could get a third twin if anyone wants
   them evidenced.
