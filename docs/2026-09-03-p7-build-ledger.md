# MERIDIAN P7 build ledger (rescued from .git/sdd/progress.md at session close, 2026-09-03)

Verbatim controller ledger of the subagent-driven build: task status, every reviewer finding and its triage, skeptic and integration evidence, operator commit blocks. Handoff: docs/handoff/2026-09-03-p7-grpc-read-api-built.md.

---

# MERIDIAN P7 (gRPC read API) -- subagent-driven-development progress ledger

Plan: `docs/plans/2026-09-03-grpc-read-api-plan.md` (UNCOMMITTED as of start)
Spec: `docs/2026-09-03-grpc-read-api-design.md` (committed `e189259`)
Branch: `p7-grpc-read-api`, forked from `e189259` on `main` (operator committed the
09-01 docs and the spec before the fork; only the plan is untracked).

## Standing deviations from the plan (controller decisions, 2026-09-03)

1. **No commits by any agent.** Operator's standing rule (hook-enforced):
   Claude never writes git history. Implementers build and test only; commit
   commands accumulate below for the operator. Review packages come from the
   WORKING TREE via `.git/sdd/pkg.sh`, not commit ranges.
2. No worktree: the plan and spec are untracked, so a worktree would not carry
   them. Work happens in the main checkout on the feature branch, as Lane 1 did.
3. Q5 (gRPC) ruled by the operator 2026-09-03: build, read-only, gate + twin.

## Task status

## Minor findings (for the final whole-branch review to triage)
- T1: `gates/run.sh` proto-fresh block leaves `.protofresh/` behind on the cmp-failure path (gitignored; wiped next run). A `trap` would be tidier.
- T1: go.mod carries ~70 indirect entries from buf's tool graph (known cost, accepted in the design).
- T3: `FeedReader` accepts ctx but never observes cancellation/deadline before file I/O (interface fixed by the brief).
- T4: `InProcess` error path calls srv.Stop() without explicit lis.Close() (equivalent; grpc closes listeners; plan-mandated code).
- T6: implementer report line numbers off by ~2 (report only).
- T8: learnings entry `commit:` reads `uncommitted (P7 gate not yet committed at time of writing)` -- operator should replace with the sha of the Task 7 commit.
- T8: pkg.sh listed six untouched 09-01 learnings files as MODIFIED with empty diffs (package artifact, not a change).
- T7: p7Check t.Fatal()s on transport errors (unevaluable, not a violation) -- correct, untested by any twin.
- T5 (plan-mandated): TestServeAnswersHeadOverTCP's first ReadString has no timeout; a hang before the listening line blocks the test instead of failing fast.
- T3: `reconcile.Reconcile`'s own error passes through unwrapped (reader.go ~227) while the parse error is wrapped in ErrBadStatement; flavor inconsistency only.

## Commit commands for the operator (run from `cd ~/dev/meridian` on `p7-grpc-read-api`)

```bash
# Task 1
git add api/ buf.yaml buf.gen.yaml go.mod go.sum gates/run.sh .gitignore
git commit -m "feat: read API proto, pinned codegen, proto-fresh gate step"
```
- Task 1: complete (working tree, review clean; go.sum verified by controller: `go mod verify` all modules verified, grpc v1.83.2 / protobuf v1.36.12 / buf v1.72.0 present)
- Task 2: complete (working tree, review clean)
- Task 3: complete (working tree, review clean)
- Task 4: dispatched (sonnet)

```bash
# Task 2
git add internal/reconcile/
git commit -m "refactor: reconcile.LoadStatementBytes for byte-borne statements"
```

```bash
# Task 3
git add internal/asof/ internal/reader/
git commit -m "feat: Reader seam and FeedReader over asof/reconcile"
```
- Task 6: complete (working tree, review clean; measured reconcile_matches_local=2, recompute=3, head=1, mislabeled rehash=3)
- Task 4: complete (working tree, review clean)
- Task 5: dispatched (sonnet)

```bash
# Task 6 (reconcile_matches_local measured 2: cash + CCC cost_basis)
git add fixtures/generate.py fixtures/base/manifest.json
git commit -m "feat: generator measures P7 twin footprints into the manifest"
```

```bash
# Task 4
git add internal/readgrpc/
git commit -m "feat: gRPC adapter over Reader with fixed status mapping and bufconn client"
```
- Task 5: complete (working tree, review clean)
- Task 7: dispatched (sonnet; gates/ only, parallel with Task 5 review)

```bash
# Task 5
git add cmd/meridian/
git commit -m "feat: meridian serve, read-only gRPC over the feed"
```
- Task 7: complete (working tree, review clean; run.sh `ok lane1 claimable=7/7`, 18 rows, first-run pass, no Emit refusal)
- Task 8: dispatched (sonnet; docs only, parallel with Task 7 review)

```bash
# Task 7
git add gates/p7_test.go gates/claimability.py
git commit -m "feat: P7 wire-fidelity gate, two twins, claimability to seven"
```
- Task 8: complete (working tree, review clean; factual STATUS claims checked against code by reviewer)
- Integration-runner: GREEN on working tree (generate_test ok, vet clean, run.sh `ok lane1 claimable=7/7` no WARN/FAIL, 18 rows x3 runs, go test 11 ok / 0 FAIL, diff --check clean)
- Skeptic: claim 1 SURVIVES (removing snapshot_matches_local_recompute -> Emit refuses `planted expectation (3) but was never computed`; removing snapshot_rehash_matches_claimed -> refused on twin_wrong_feed's 0-expectation first); claim 2 SURVIVES (blank line in read_grpc.pb.go -> `FAIL generated read_grpc.pb.go stale`, exit 1 before gates). Tree restored byte-identical (sha256 equal), status count 24 before/after.
  Gaps for honest limits: (a) Emit catches a MISSING check key, not a wired-but-dead check whose expectation is 0 -- P7 is safe because every one of its four checks has a nonzero expectation in at least one twin (1/3/3/2); (b) proto-fresh compares only the two named generated files; a buf.gen.yaml change emitting a third file is unchecked.
- Final whole-branch review (opus): MERGEABLE AFTER FIXES. Important: (1) AsOfResponse.prefix_hash never checked by any P7 check (spec gap, manifest-neutral one-liner); (2) STATUS honest-limits preamble says six cells / the fifth; (3) no handoff entry yet (controller's close step). New minors: proto-fresh hardcoded file list; mutated_rows:3 on mislabeled twin; bufconn in non-test graph uncommented; --listen help silent on exposure; as_of_seq parsed never compared (pre-existing); records/compared legs never driven non-zero. Recorded minors triaged: all leave except T8 commit: line (operator). Honest limits to add: feed opened O_CREATE|O_RDWR; exists-then-open race under serve; no MaxRecvMsgSize (4 MB default, unmeasured); proto-fresh only ever run on Windows.
- fix-final dispatched (sonnet) with brief .git/sdd/fix-final-brief.md covering Important 1+2, the new honest limits, and minors A-D; re-review by final-review to follow.

```bash
# Task 8 (after this: replace `commit: uncommitted ...` in docs/learnings/2026-09-03-eighteen-verdict-rows-after-p7.md with the Task 7 sha in a follow-up docs commit, or before committing Task 8)
git add STATUS.md README.md docs/learnings/
git commit -m "docs: P7 claimable, honest limit for in-process gate, 18-row learning"
git push -u origin p7-grpc-read-api
```

- Close: fix-final re-review by final-review: all findings FIXED; one cosmetic unbalanced `**` in STATUS.md fixed by controller; learnings x4 + handoff written; ledger rescued to docs/.
