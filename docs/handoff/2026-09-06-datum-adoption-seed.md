# Handoff -- DATUM adoption seed: replaces BASELINE registration

2026-09-06. Newest MERIDIAN commit this brief describes: **`a087ab2`**
(`docs: p7-closed handoff`, main, level with origin). Cross-repo anchors:
datum **`91be859`** (DATUM v2 governing text), baseline **`775978f`**.
Pick-up measures drift from `a087ab2`.

**Read this first.** The session that wrote this brief did not run, build,
or change anything in MERIDIAN. It read the tree and the rows, ran
BASELINE's checker over copies of the rows in a scratch directory, and
wrote this file plus one learnings entry. The gate-state claims below are
**reported** from a separate session's pick-up of `2026-09-03-p7-closed.md`
earlier the same day, pasted by the operator; they carry that session's
evidence and this session's re-verify lines, and were not re-run here.

## Current state

- **built (reported, not re-run here)** -- seven properties CLAIMABLE on
  `main` at `a087ab2`; the other session's fresh `sh gates/run.sh` exited
  0 on Go 1.26 / Python 3.14.2, ending `ok lane1 claimable=7/7`, 18 rows in
  `gates/out/`; CI run 33817051877 on `a087ab2` success.
  re-verify: `sh gates/run.sh 2>&1 | tail -1` -> `ok lane1 claimable=7/7`
  re-verify: `gh run list --branch main --limit 1 --json headSha,conclusion --jq '.[0] | "\(.headSha[0:7]) \(.conclusion)"'` -> `a087ab2 success`
- **built (elsewhere)** -- DATUM v2, the governing text for baseline,
  traverse and meridian, at datum `91be859`, with the pack design
  (`docs/2026-09-06-datum-design.md`) and the precedent research note.
  re-verify: `git -C ~/dev/datum log --oneline -1` -> `91be859 docs: DATUM v2, ...`
  re-verify: `grep -c "not a catalog" ~/dev/datum/DATUM.md` -> `1`
- **retired** -- BASELINE registration of the seven cells (item 1 of
  `2026-09-03-p7-closed.md`). Basis: BASELINE's checker refuses all 18 rows
  on two rules and its builder refuses a second surface; the operator then
  ruled BASELINE is not a catalog. Learnings entry
  `2026-09-06-baseline-checker-refuses-meridian-rows.md`.
  re-verify: `grep -n 'evaluated.no_future_accepted\|second ${row.cell} cell' ~/dev/baseline/scripts/lib/ledger.mjs` -> two hits
- **planned, blocked** -- the DATUM conformance pack (`schema/`,
  `conformance/check.mjs`, `test.mjs`, fixtures). Nothing executable
  exists in datum. MERIDIAN's adoption cannot start until it does.
  re-verify: `ls ~/dev/datum/conformance 2>&1` -> no such directory, until built
- **planned** -- MERIDIAN adoption, rollout step 2 in the design. Details
  under Open / next.
- **not started, unchanged from p7-closed** -- lanes 2-3; the three
  never-falsified checks; the optional third P7 twin driving `records` and
  `compared` non-zero; D2; generator RNG via getrandbits.

Reported drift from the other session's pick-up, not re-checked here: a
stray empty directory named like a mangled Windows path
(`C:Usershossadevmeridianinternalasof`) sits at the repo root, untracked and
invisible to `git status`. Verify it is empty before removing it.

## Locked decisions

1. **DATUM governs MERIDIAN; BASELINE is not a catalog.** Operator,
   2026-09-06. Reason: the shared thing is the discipline and the row, and
   the copy was measured to fail at the checker (learnings entry above).
   Ref: `~/dev/datum/DATUM.md`, "Scope: a discipline, not a catalog".
   Consequence for this repo: design `docs/2026-08-31-design.md` §7 and
   the p7-closed "next" item need a **dated amendment**, never a silent
   edit (DATUM rule 12).
2. **`gate_sha` / `gate_worktree` replace `parallax_sha` /
   `parallax_worktree`.** Operator, 2026-09-06. Reason: the emitter-neutral
   placeholder in DATUM v1 was implemented nowhere, and `verdict.go`
   writes a MERIDIAN commit under a key named for another project (the
   misnomer `2026-09-01-lane1-build.md` item 6 already flagged). Ref:
   DATUM rule 6. Both emitters (PARALLAX's too) change by the same diff.
3. **`result` is the exact enum; the reason is `unevaluable_reason`.**
   Operator, 2026-09-06. Reason: BASELINE's checker rejects a colon form;
   TRAVERSE's design used one. Ref: DATUM rule 2. MERIDIAN today emits
   only GREEN/RED, so this is additive here.
4. **`schema: "datum/gate-verdict/1"` is a required row key.** Design §2.
   Reason: the pack must be able to refuse a row from a version it does
   not know.
5. **Multi-twin crediting is the shared rule, and it is MERIDIAN's.**
   DATUM rule 3, read from `gates/claimability.py`. Reason: one red twin
   does not credit a property that plants three defects. No change to
   MERIDIAN's semantics; the change is that BASELINE's single-twin rule
   becomes the special case.
6. **STATUS.md house rule: hand-written dated log plus a generated block
   between markers, checked by `--check`.** Operator, 2026-09-06, DATUM
   rule 5. Reason: MERIDIAN's hand-written record and BASELINE's generated
   page were two conventions; this keeps both in one file. MERIDIAN's
   claimability table becomes the generated block.
7. **Rollout order: datum pack, then meridian, then baseline, then
   traverse.** Design §6. Reason: MERIDIAN's rows regenerate, so nothing
   published breaks on a key rename; BASELINE must build `superseded_by`
   before re-emitting its rows.
8. **Rows stay uncommitted build output** (DATUM rule 8, second case),
   with one exception: one live and one twin row are copied into datum's
   `conformance/fixtures/real/meridian/` at a pinned MERIDIAN sha and
   hash-bound there. That is the only place a MERIDIAN row is ever
   committed, and it is a PASS fixture, re-copied on emitter change, never
   edited.
9. **Carried from `2026-09-03-p7-closed.md`:** its six build decisions and
   the process lock -- say "one more commit coming" before pushing, since
   the operator merges within minutes.

## Reuse map

- `gates/verdict.go`, the row map near line 186 -- the only place the
  keys are written; rename two, add `schema`.
- `gates/claimability.py` -- `load()` keys on the `surface` regex and
  `cell`; `twin_ok()` and `duplicate_msgs()` read `planted`. It never reads
  the sha or worktree keys, so it should not need to change for decision
  2; confirm by grep before assuming.
- `gates/run.sh` -- the CI entry; the pack's two commands go here, `test.mjs`
  before the gate and `check.mjs gates/out` after it.
- `.github/workflows/` -- the job that runs `run.sh` on ubuntu-24.04; Node
  20+ needs a setup step (the pack is Node, zero dependencies).
- `STATUS.md` claimability table -- becomes the generated block; the
  narrative entries above it stay hand-written.
- `docs/2026-08-31-design.md` §4 ("registration ... is a copy, not a
  translation") and §7 ("MERIDIAN's six claimability cells land as
  BASELINE catalog rows") -- the two paragraphs the amendment names.
- `docs/learnings/2026-09-01-baseline-gate-verdict-schema.md` and
  `2026-09-01-baseline-twin-rows-carry-planted.md` -- still true of
  BASELINE's rows today; schema v1 supersedes their key names, so a new
  entry with `kills:` follows the emitter change, not before.
- `~/dev/datum/docs/2026-09-06-datum-design.md` -- §2 the schema, §3
  crediting, §4 fixture format and `expect_reason`, §5 the vendoring
  contract (`vendor/datum/`, `PIN` file, CI order), §6 step 2, §7 exit codes.
- `~/dev/datum/DATUM.md` -- rules 2, 3, 6, 8 are the ones this adoption
  touches.

## Invariants

- **Fix the emitter and re-emit; never edit a row.** Rows here are not
  committed, so the temptation is the datum fixture copy: re-copy it, do
  not patch it.
- **Every check has a nonzero expectation in at least one twin** (p7-closed
  invariant). Adding `schema`/`gate_sha` keys adds no check, so this is
  unaffected; any new gate is not.
- **`Emit` refuses empty checks and a non-positive evaluated denominator**
  for both cells. The pack enforces the same rule from the outside;
  disagreement between the two is a finding, not something to align by
  loosening either.
- **`run.sh` clears `gates/out/`; bare `go test` appends** (learnings
  2026-09-01). Run the pack's `check.mjs` only over a `run.sh`-fresh
  directory, or duplicate rows will read as a pack refusal.
- **Only the operator writes git history.** Emit commit commands, grouped
  by repo, one concern per commit.
- **Pack first, then emitter.** Vendor the pack and get `test.mjs` green
  in CI before changing `verdict.go`, so the first `check.mjs` run over
  real rows is a measured red (missing `schema`, unknown `parallax_sha`
  key) and the emitter change is what turns it green. A first run that is
  green proves the pack was never pointed at the rows.
- **STATUS.md is the record.** Nothing is CLAIMABLE anywhere unless it is
  CLAIMABLE there; the generated block does not change that, it derives it.

## Open / next

1. **Docs first, no blocker.** Dated amendment to `docs/2026-08-31-design.md`
   §4 and §7 (decision 1), a STATUS.md entry dated 2026-09-06 recording that
   BASELINE registration is retired and DATUM adoption is planned, and the
   HANDOFF index row for this brief (already appended, uncommitted).
   Remove the stray directory after verifying it is empty.
2. **Blocked on the datum pack.** When `~/dev/datum/conformance/` exists
   with `test.mjs` green in datum's CI: vendor it into `vendor/datum/` with
   the `PIN` file; add the Node setup and the two commands to `run.sh` and
   the workflow; confirm the measured red; change `verdict.go` (decisions
   2-4); update `claimability.py` only if grep shows it reads the renamed
   keys; convert the STATUS.md table to the generated block with `--check`;
   add the README DATUM section (pinned sha, two commands, rule-by-rule
   table with the authored cells labelled authored); copy one live and one
   twin row into datum's `fixtures/real/meridian/` and bind them; write the
   learnings entry that kills the two 2026-09-01 schema entries.
3. **Unchanged.** Optional third P7 twin; lanes 2-3; the three
   never-falsified checks.

Blocker for item 2 is the pack, built in a datum session, not here. Item 1
needs nothing.

**Learnings gate state, measured at write time:** `node
~/dev/rigor/scripts/check-learnings.mjs docs/learnings` exits 1 on one
PRE-EXISTING entry, `2026-09-01-vacuity-guard-denominator.md` (prose form,
all seven fields missing). The new entry passes. Do not edit the old entry;
the ledger rule is a new dated entry that supersedes it, and that is the
operator's call.

**Untracked at handoff:** this brief, its index row, and the learnings entry
`2026-09-06-baseline-checker-refuses-meridian-rows.md` with its index row.
Commit commands:

```bash
cd ~/dev/meridian
git add docs/learnings/2026-09-06-baseline-checker-refuses-meridian-rows.md docs/learnings/LEARNINGS.md
git commit -m "docs: learning, BASELINE checker refuses MERIDIAN rows"
git add docs/handoff/2026-09-06-datum-adoption-seed.md docs/handoff/HANDOFF.md
git commit -m "docs: DATUM adoption seed, retires BASELINE registration"
git push
```
