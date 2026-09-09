# Handoff -- conformance adopted: the pack is vendored and the emitter writes the shared row

2026-09-09. Newest MERIDIAN commit this brief describes: **`3c454ed`**
(`docs: name the governing text once, drop private refs`, main, level with
origin). Everything below is **uncommitted** on top of it. Cross-repo
anchor: the governing text's repo (its pack pushed, CI green on Node 20 and
24) plus one uncommitted one-line fix there that this vendoring needs; the
commit the pin names will be the one that carries that fix. Pick-up
measures drift from `3c454ed`.

**The governing text** is DATUM, a private governing text for the
operator's data-platform repositories; this repo cites it for the ruling
and vendors its conformance pack, and nothing else. Below it is "the
governing text" and its checker "the pack".

## Current state

- **built, uncommitted** -- the pack vendored under `gates/datum/`
  (`check.mjs`, `test.mjs`, `reasons.md`, `fixtures/**`, a copy of
  `gate-verdict.v1.json`). Its self-test is green here on Node 24.
  re-verify: `node gates/datum/test.mjs --mutate` -> `ok conformance: 13 positive, 46 negative, 0 real, 17 reasons, 13 rules mutated`
- **measured red, then green** -- before the emitter change the pack
  refused all 18 rows with exactly five reasons per row (missing `schema`,
  `gate_sha`, `gate_worktree`; unknown `parallax_sha`, `parallax_worktree`);
  after it, the pack accepts all 18 and derives seven CLAIMABLE surfaces
  with twin counts 1/1/1/2/1/3/2. Learnings entry
  `2026-09-09-emitter-writes-the-shared-row.md`.
  re-verify: `sh gates/run.sh >/dev/null 2>&1; node gates/datum/check.mjs gates/out | tail -1` -> `ok 18 rows conform, 7 surfaces`
- **built, uncommitted** -- `Emit` writes `schema`, `gate_sha`,
  `gate_worktree` (`gates/verdict.go`); `TestEmit` pins the 17/18-key
  list and the schema id (`gates/verdict_test.go`); `go test ./gates` is
  green; `gofmt -l gates/` prints nothing.
  re-verify: `python -c "import json,glob; print(sorted(json.load(open(sorted(glob.glob('gates/out/*p7-live*'))[0]))))"` -> 17 keys including `gate_sha`, `gate_worktree`, `schema`
- **built, uncommitted** -- `gates/run.sh` runs the pack self-test before
  the gates, `claimability.py --status --check` after them, then the pack
  over `gates/out` with `--verify-pin`, and fails if the pack and
  `claimability.py` disagree on the CLAIMABLE count. `.github/workflows/gates.yml`
  gains a Node 20 setup step. `claimability.py` gains `--render` /
  `--check`; STATUS.md's claimability table is now the generated block.
  re-verify: `python gates/claimability.py gates/out --status STATUS.md --check STATUS.md | tail -2` -> `ok STATUS.md generated claimability block is fresh` then `ok lane1 claimable=7/7`
- **blocked on one commit elsewhere** -- `gates/datum/PIN` does not exist.
  The pin binds every vendored file to one commit of the governing text's
  repo, and the vendored `test.mjs` carries a fix (schema found beside
  itself, not at `../schema/`) that is not yet committed there. Until the
  pin exists `sh gates/run.sh` stops at `--verify-pin` with exit 2, which
  is the fail-closed reading, not a bug.
  re-verify: `ls gates/datum/PIN` -> no such file, until written
- **not started** -- copying one live and one twin row into the pack's
  real-fixture corpus and binding them. Needs rows emitted from a clean
  tree at a pushed sha (`gate_worktree: clean`), so it follows the commits.
- **unchanged from the 2026-09-06 seed** -- lanes 2-3; the three
  never-falsified checks; the optional third P7 twin; D2; generator RNG.

## Locked decisions

1. **The pack lives under `gates/datum/`, not `vendor/`.** Go treats
   `vendor/` as module vendoring and refuses to build without
   `vendor/modules.txt` (`go vet ./...`: "inconsistent vendoring").
   Recorded in the governing text's design §10. The contract is the file
   set, the pin and the CI order, not the path.
2. **Two derivations, kept and diffed.** `claimability.py` and the pack
   both derive the table; `run.sh` fails when their CLAIMABLE counts
   differ. Agreement between independent derivations is evidence; one
   derivation is a claim. Neither is retired.
3. **STATUS.md's claimability table is generated.** `--render` writes it
   between markers from the rows; `--check` in `run.sh` refuses drift.
   The dated narrative above it stays hand-written and corrected in place.
4. **A negative count is a schema matter, not a violation.** The pack's
   live-with-violations rule counts positives, so one fixture plants one
   defect. This repo's `Emit` never writes a negative count.
5. **`UNCLAIMED` is never derived from a rows directory.** No rows, no
   group; the word belongs to a repo's own STATUS.md for a lane it has
   not run. `claimability.py` uses it for a property whose rows ran and
   did not qualify.
6. **Every regular file in a rows directory is a row.** The pack reads
   them all; a file that is not JSON makes the directory unevaluable
   (exit 2). Nothing is skipped by name. `run.sh` clears `gates/out/`
   before the gates, so only `Emit` writes there.
7. **Carried:** the nine decisions of `2026-09-06-datum-adoption-seed.md`
   and the p7-closed process lock (say "one more commit coming" before
   pushing).

## Reuse map

- `gates/verdict.go` row map (near line 188) -- the only place keys are
  written. `gates/verdict_test.go` line 35 area -- the sorted key list.
- `gates/claimability.py` -- `derive()` is the shared vocabulary
  (CLAIMABLE / PARTIAL / UNCLAIMED / UNEVALUABLE) for one property;
  `render()` and `check_block()` are the STATUS.md block; `PROP_NAMES`
  is the only authored content in the table.
- `gates/run.sh` -- `== conformance pack self-test`, `== claimability`,
  `== conformance pack over the rows`; the count cross-check is the last
  five lines.
- `gates/datum/reasons.md` -- the refusal vocabulary, one rule-table name
  per reason; read it before adding a check that could change a row's
  shape.
- `gates/datum/check.mjs --write-pin <sha>` -- writes `PIN` over every
  file beside it; `--verify-pin` refuses missing, altered or unlisted.
- `gates/datum/fixtures/real/SOURCE.md` -- empty binding in the vendored
  copy; the real-row copy goes into the governing text's repo, not here.

## Invariants

- **Pin to a commit that contains the bytes.** `PIN` names a commit of
  the governing text's repo; the vendored files must be byte-identical
  (LF-normalised) to that commit. Never pin to a working tree.
- **Fix the emitter and re-emit; never edit a row.** Unchanged.
- **Only `run.sh`-fresh rows go under the pack.** Bare `go test` appends;
  duplicates read as `second live cell` / `duplicate twin mutation`.
- **Disagreement between `Emit`, `claimability.py` and the pack is a
  finding.** Do not align by loosening any of the three.
- **Only the operator writes git history.**
- **The governing text is private; this repo cites only what a reader
  can check.** The pack is public by vendoring; its rule text is not.

## Open / next

1. **Operator, in the governing text's repo:** commit the `test.mjs`
   fix (and the design §10 / STATUS lines that record it). Then here:
   write the pin from that commit, run `sh gates/run.sh` to see the whole
   chain green, commit in the order below, push, watch CI.
2. **After the push, with a clean tree:** re-run `run.sh` at the pushed
   sha, copy one live and one twin row (P7 live and one P7 twin are the
   most informative) into the pack's real-fixture corpus for this repo,
   bind them in its `SOURCE.md`, re-render its STATUS block, commit there;
   then re-vendor here at that commit and rewrite `PIN` (the fixture set
   changed, so the pin must move).
3. **Unchanged.** Optional third P7 twin; lanes 2-3; the three
   never-falsified checks; the two P7 legs never driven non-zero.

**Learnings gate state:** `node ~/dev/rigor/scripts/check-learnings.mjs docs/learnings`
exits 1 on the pre-existing `2026-09-01-vacuity-guard-denominator.md`
only; the new entry passes.
