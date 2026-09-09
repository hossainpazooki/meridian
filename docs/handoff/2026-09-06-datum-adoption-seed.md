# Handoff -- conformance seed: the governing text replaces BASELINE registration

2026-09-06. Newest MERIDIAN commit this brief describes: **`a087ab2`**
(`docs: p7-closed handoff`, main, level with origin). Cross-repo anchor:
baseline **`775978f`**. Pick-up measures drift from `a087ab2`.

Reworded 2026-09-09: the governing text named below is private, so this
entry no longer carries its location, revision, rule numbers or section
pointers; the ruling, the decisions and the plan are unchanged.

**Read this first.** The session that wrote this brief did not run, build,
or change anything in MERIDIAN. It read the tree and the rows, ran
BASELINE's checker over copies of the rows in a scratch directory, and
wrote this file plus one learnings entry. The gate-state claims below are
**reported** from a separate session's pick-up of `2026-09-03-p7-closed.md`
earlier the same day, pasted by the operator; they carry that session's
evidence and this session's re-verify lines, and were not re-run here.

**The governing text.** On 2026-09-06 the operator ruled that MERIDIAN is
governed by **DATUM**, a private governing text for the operator's
data-platform repositories: one statement of the discipline (a claim is a
row; a gate has three outcomes; a gate that has never gone red is not a
gate; status is derived, never authored; identity is content plus code plus
worktree; corrections are dated, never erased) and one row schema, enforced
by a conformance pack that each governed repo vendors at a pinned revision
with a sha256 per file. It is not public; nothing in this repo may cite it
as evidence. Below it is "the governing text", and its checker "the pack".

## Current state

- **built (reported, not re-run here)** -- seven properties CLAIMABLE on
  `main` at `a087ab2`; the other session's fresh `sh gates/run.sh` exited
  0 on Go 1.26 / Python 3.14.2, ending `ok lane1 claimable=7/7`, 18 rows in
  `gates/out/`; CI run 33817051877 on `a087ab2` success.
  re-verify: `sh gates/run.sh 2>&1 | tail -1` -> `ok lane1 claimable=7/7`
  re-verify: `gh run list --branch main --limit 1 --json headSha,conclusion --jq '.[0] | "\(.headSha[0:7]) \(.conclusion)"'` -> `a087ab2 success`
- **built (elsewhere, private)** -- the governing text, v2, with the pack
  design and a precedent research note. Its revision is recorded in its own
  repository, not here.
- **retired** -- BASELINE registration of the seven cells (item 1 of
  `2026-09-03-p7-closed.md`). Basis: BASELINE's checker refuses all 18 rows
  on two rules and its builder refuses a second surface; the operator then
  ruled BASELINE is not a catalog. Learnings entry
  `2026-09-06-baseline-checker-refuses-meridian-rows.md`.
  re-verify: `grep -n 'evaluated.no_future_accepted\|second ${row.cell} cell' ~/dev/baseline/scripts/lib/ledger.mjs` -> two hits
- **planned, blocked** -- the pack (a row schema, a reference checker, the
  checker's own negative controls, a fixture corpus). Nothing executable
  exists yet. MERIDIAN's adoption cannot start until it does.
  re-verify: the governing text's repository has no `conformance/` directory until the pack is built
- **planned** -- MERIDIAN adoption, the governing text's rollout step for
  this repo. Details under Open / next.
- **not started, unchanged from p7-closed** -- lanes 2-3; the three
  never-falsified checks; the optional third P7 twin driving `records` and
  `compared` non-zero; D2; generator RNG via getrandbits.

Reported drift from the other session's pick-up, not re-checked here: a
stray empty directory named like a mangled Windows path
(`C:Usershossadevmeridianinternalasof`) sits at the repo root, untracked and
invisible to `git status`. Verify it is empty before removing it.

## Locked decisions

1. **The governing text governs MERIDIAN; BASELINE is not a catalog.**
   Operator, 2026-09-06. Reason: the shared thing is the discipline and the
   row, and the copy was measured to fail at the checker (learnings entry
   above). Consequence for this repo: design `docs/2026-08-31-design.md`
   §4 and §7 and the p7-closed "next" item need a **dated amendment**,
   never a silent edit.
2. **`gate_sha` / `gate_worktree` replace `parallax_sha` /
   `parallax_worktree`.** Operator, 2026-09-06. Reason: the emitter-neutral
   placeholder in the governing text's first draft was implemented nowhere,
   and `verdict.go` writes a MERIDIAN commit under a key named for another
   project (the misnomer `2026-09-01-lane1-build.md` item 6 already
   flagged). Both emitters (PARALLAX's too) change by the same diff.
3. **`result` is the exact enum; the reason is `unevaluable_reason`.**
   Operator, 2026-09-06. Reason: BASELINE's checker rejects a colon form;
   TRAVERSE's design used one. MERIDIAN today emits only GREEN/RED, so this
   is additive here.
4. **`schema` is a required row key**, holding the pack's schema id for
   the row version. Reason: the pack must be able to refuse a row from a
   version it does not know.
5. **Multi-twin crediting is the shared rule, and it is MERIDIAN's.** Read
   from `gates/claimability.py`. Reason: one red twin does not credit a
   property that plants three defects. No change to MERIDIAN's semantics;
   the change is that BASELINE's single-twin rule becomes the special case.
6. **STATUS.md house rule: hand-written dated log plus a generated block
   between markers, checked by `--check`.** Operator, 2026-09-06. Reason:
   MERIDIAN's hand-written record and BASELINE's generated page were two
   conventions; this keeps both in one file. MERIDIAN's claimability table
   becomes the generated block.
7. **Rollout order: the pack, then meridian, then baseline, then
   traverse.** Reason: MERIDIAN's rows regenerate, so nothing published
   breaks on a key rename; BASELINE must build `superseded_by` before
   re-emitting its rows.
8. **Rows stay uncommitted build output**, with one exception: one live and
   one twin row are copied into the pack's real-fixture corpus at a pinned
   MERIDIAN sha and hash-bound there. That is the only place a MERIDIAN row
   is ever committed, and it is a PASS fixture, re-copied on emitter
   change, never edited.
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
- `gates/run.sh` -- the CI entry; the pack's two commands go here, its
  self-test before the gate and its checker over `gates/out` after it.
- `.github/workflows/` -- the job that runs `run.sh` on ubuntu-24.04; Node
  20+ needs a setup step (the pack is Node, zero dependencies).
- `STATUS.md` claimability table -- becomes the generated block; the
  narrative entries above it stay hand-written.
- `docs/2026-08-31-design.md` §4 ("registration ... is a copy, not a
  translation") and §7 ("MERIDIAN's six claimability cells land as
  BASELINE catalog rows") -- the two paragraphs the amendment names.
- `docs/learnings/2026-09-01-baseline-gate-verdict-schema.md` and
  `2026-09-01-baseline-twin-rows-carry-planted.md` -- still true of
  BASELINE's rows today; the pack's schema supersedes their key names, so
  a new entry with `kills:` follows the emitter change, not before.
- The governing text and its pack design (private) -- the row schema, the
  crediting rule, the fixture format and expected-reason field, the
  vendoring contract (vendored directory, pin file, CI order), the rollout
  step for this repo, and the checker's exit codes.

## Invariants

- **Fix the emitter and re-emit; never edit a row.** Rows here are not
  committed, so the temptation is the pack's fixture copy: re-copy it, do
  not patch it.
- **Every check has a nonzero expectation in at least one twin** (p7-closed
  invariant). Adding `schema`/`gate_sha` keys adds no check, so this is
  unaffected; any new gate is not.
- **`Emit` refuses empty checks and a non-positive evaluated denominator**
  for both cells. The pack enforces the same rule from the outside;
  disagreement between the two is a finding, not something to align by
  loosening either.
- **`run.sh` clears `gates/out/`; bare `go test` appends** (learnings
  2026-09-01). Run the pack's checker only over a `run.sh`-fresh
  directory, or duplicate rows will read as a pack refusal.
- **Only the operator writes git history.** Emit commit commands, grouped
  by repo, one concern per commit.
- **Pack first, then emitter.** Vendor the pack and get its self-test green
  in CI before changing `verdict.go`, so the first checker run over real
  rows is a measured red (missing `schema`, unknown `parallax_sha` key)
  and the emitter change is what turns it green. A first run that is green
  proves the pack was never pointed at the rows.
- **STATUS.md is the record.** Nothing is CLAIMABLE anywhere unless it is
  CLAIMABLE there; the generated block does not change that, it derives it.
- **The governing text is private; this repo cites only what a reader can
  check.** Evidence here is BASELINE's public checker, this repo's gates,
  and this repo's ledgers. A claim that rests on the private text alone is
  not a claim in this repo.

## Open / next

1. **Docs first, no blocker.** Dated amendment to `docs/2026-08-31-design.md`
   §4 and §7 (decision 1), a STATUS.md entry dated 2026-09-06 recording that
   BASELINE registration is retired and conformance is planned, and the
   HANDOFF index row for this brief (already appended, uncommitted).
   Remove the stray directory after verifying it is empty.
2. **Blocked on the pack.** When the pack exists with its self-test green
   in its own CI: vendor it under `vendor/` with its pin file; add the
   Node setup and the two commands to `run.sh` and the workflow; confirm
   the measured red; change `verdict.go` (decisions 2-4); update
   `claimability.py` only if grep shows it reads the renamed keys; convert
   the STATUS.md table to the generated block with `--check`; add a README
   conformance section (pinned revision, the two commands, a rule-by-rule
   table with the authored cells labelled authored); copy one live and one
   twin row into the pack's real-fixture corpus and bind them; write the
   learnings entry that kills the two 2026-09-01 schema entries.
3. **Unchanged.** Optional third P7 twin; lanes 2-3; the three
   never-falsified checks.

Blocker for item 2 is the pack, built in its own session, not here. Item 1
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
git commit -m "docs: adoption seed, retires BASELINE registration"
git push
```
