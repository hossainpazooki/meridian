# Handoff -- conformance re-vendored: the pack moved, and both derivations credit per check

2026-09-15. Newest MERIDIAN commit this brief describes: **`97d815c`**
(`docs: conformance-pinned handoff`, main). Pick-up measures drift from
here. Everything below is **uncommitted** on top of it.

This entry supersedes the "Open / next" of `2026-09-10-conformance-pinned.md`.
Its step 1 landed: the pin was committed in `35142fa`, and CI run
34438961409, a push to `main` at `97d815c`, succeeded. Its step 2 is done:
the re-vendored pack's real-fixture corpus holds one P7 live and one P7 twin
row of this repo, byte-identical to rows emitted with `gate_sha` `97d815c`
and `gate_worktree` clean.

**The governing text** is DATUM, a private governing text for the operator's
data-platform repositories. This repo vendors its conformance pack and cites
it for nothing else; every claim below is checkable from this repo, its CI,
and the vendored pack.

## Current state

- **built, uncommitted** -- the pack vendored at `gates/conformance/` from
  an archive of a pushed commit of the governing text: `diff -r` against
  that archive found no difference, and the schema file, placed beside
  `check.mjs`, compared byte for byte on its own. `PIN` was written by
  `--write-pin`, with no redirect. The previous vendored directory is
  deleted from the working tree.
  re-verify: `node gates/conformance/check.mjs --verify-pin` -> `ok pin verified`
- **built, uncommitted** -- the vendored self-test passes in the shape this
  repo ships, `PIN` beside it.
  re-verify: `node gates/conformance/test.mjs --mutate | tail -1` -> `ok conformance: 19 positive, 50 negative, 2 real, 20 reasons, 16 rules mutated, 1 crediting rule mutated`
- **measured red, then green** -- over the 18 rows emitted at `97d815c`,
  before `gates/claimability.py` changed, the pack derived CLAIMABLE for P5
  and P7 only while `claimability.py` printed `ok lane1 claimable=7/7`. The
  new `--self-test`, run against the previous derivation, printed 9 `FAIL`
  lines and exited 1; `--agree` over the two `--json` outputs printed `FAIL
  conformance pack and claimability.py disagree` and exited 1. With the
  per-check rule in place both pass.
  re-verify: `python gates/claimability.py --self-test` -> `ok claimability self-test (all planted cases caught)`
- **built, uncommitted** -- `gates/claimability.py` credits per check
  (`unfalsified()`, used by `claimable()` and `derive()`), derives
  UNEVALUABLE with no list when any row of a property is UNEVALUABLE,
  prints `--json` as `[{surface, lane, status, unfalsified}]`, compares two
  `--json` outputs with `--agree`, and renders an Unfalsified column into
  STATUS.md's generated block.
  re-verify: `python gates/claimability.py gates/out | tail -1` -> `ok lane1 claimable=2/7`
- **built, uncommitted** -- `gates/expect.json` lists every surface with its
  exact twin mutations, checked against the rows. Before it existed, naming
  it was unevaluable (exit 2); a planted copy short one P6 twin and missing
  the P7 cell was refused (exit 1).
  re-verify: `node gates/conformance/check.mjs gates/out --verify-pin --expect gates/expect.json | tail -1` -> `ok 18 rows conform, 7 surfaces`
- **built, uncommitted** -- `gates/run.sh` order: the pin verified alone;
  the pack's self-test; `claimability.py --self-test`; the gates;
  `claimability.py --status --check`; the pack with `--verify-pin --expect
  gates/expect.json`; the agreement step, which fails printing both
  `--json` outputs on any per-property difference in status or unfalsified
  checks. `.github/workflows/gates.yml` changed in one comment only.
  re-verify: `sh gates/run.sh 2>&1 | tail -1` -> `ok conformance pack agrees: claimable=2`
- **built, uncommitted** -- STATUS.md: the generated block re-rendered; a
  2026-09-15 entry; the per-check half of the crediting rule; dated
  in-place corrections for the 2026-09-09 "Not yet" items, the
  never-falsified-checks bullet and the deferred-decisions line; a dated
  note on what P7's CLAIMABLE does not reach. README.md: the Conformance
  section and the property list, with no counts.
  re-verify: `python gates/claimability.py gates/out --check STATUS.md` -> `ok STATUS.md generated claimability block is fresh`
- **not run** -- CI on any of this; it runs on the operator's push.
- **stale, outside this change** -- comments in `gates/verdict.go` and
  `gates/verdict_test.go` still name the previous vendored directory. No
  gate reads them.
  re-verify: `git grep -n -E 'gate-verdict\.v1\.json|check\.mjs' -- gates/verdict.go gates/verdict_test.go` -> three comment lines, none naming `gates/conformance/`

## Locked decisions

1. **The vendored pack directory is `gates/conformance/`, in every governed
   repo.** Operator ruling. The reason it sits under `gates/` at all is
   unchanged: Go treats a top-level `vendor/` as module vendoring.
2. **Crediting is per check, in both derivations.** A live check that no
   twin RED as planted sets nonzero blocks CLAIMABLE; the property derives
   PARTIAL and names the check. A property with any UNEVALUABLE row, the
   live one included, derives UNEVALUABLE and names nothing. Operator
   ruling.
3. **The two derivations are compared per property, not by count.** Reason:
   a CLAIMABLE count can agree while the properties behind it differ, and
   the status alone can agree while the unfalsified checks differ. `run.sh`
   compares both, and prints both sides when they differ.
4. **`--expect` is part of the pack's run here.** `gates/expect.json` names
   every cell and its exact twin mutations and never asserts a status. A
   twin or property added to the gates changes this file in the same change.
5. **Stop rule: fix majors, list minors.** Minor findings go to a
   known-limits line, not another round.
6. **Carried, unchanged:** the decisions of `2026-09-10-conformance-pinned.md`,
   with its location decision replaced by decision 1 above and its
   count-only agreement replaced by decision 3, and the process lock -- say
   "one more commit coming" before pushing, because the operator merges
   within minutes.

## Reuse map

- `gates/conformance/check.mjs` -- `credit()` is the pack's per-check
  crediting; `--json`, `--expect`, `--verify-pin`, `--write-pin <sha>`.
  `verifyPin()` refuses a listed file missing or altered and an unlisted
  file present, so a re-vendor is: archive a pushed commit, empty
  `gates/conformance/`, copy, `--write-pin`, `--verify-pin` alone.
- `gates/conformance/reasons.md` -- the refusal vocabulary.
- `gates/claimability.py` -- `unfalsified()`, `derive()` (live word, twin
  word, status, unfalsified), `claimable()`, `to_json()`, `agree()`,
  `self_test()`; `PROP_NAMES` is still the only authored content in the
  generated table.
- `gates/run.sh` -- `== conformance pack pin`, `== conformance pack
  self-test`, `== claimability self-test`, `== conformance pack over the
  rows`, `== agreement`.
- `fixtures/generate.py`, the manifest block -- where each twin's
  `expected_violations` is measured. A twin that drives an unfalsified check
  starts there.
- `gates/p7_test.go`, `p7Check` -- where two P7 legs fold into one check key.

## Invariants

- **`run.sh` clears `gates/out/`; a bare `go test` appends.** Duplicates
  read as refusals from the pack and as `duplicate verdict rows` from
  `claimability.py`.
- **Disagreement between `Emit`, `claimability.py` and the pack is a
  finding.** Do not align by loosening any of the three.
- **Crediting reads one integer per check key.** Legs folded inside a key
  are invisible to it: P7's `records` leg of `head_matches_local` and
  `compared` leg of `reconcile_matches_local` stay undriven under a
  CLAIMABLE.
- **Exit 2 from the pack is unevaluable and is never coerced.** `run.sh`
  runs under `set -e`, and the pack's `--json` run happens only after a
  plain run over the same rows exited 0.
- **Nothing is CLAIMABLE anywhere unless STATUS.md says so**, and the
  generated block derives that from the rows.
- **The governing text is private; this repo cites only what a reader can
  check.** One naming per document; no paths, revisions or rule numbers.
- **Only the operator writes git history.**

## Open / next

1. **Commit and push; watch CI.** Stage the deleted previous vendored
   directory together with everything under `gates/` (`git add -A gates/`
   covers both), plus `.github/workflows/gates.yml`, `README.md`,
   `STATUS.md`, this entry and `docs/handoff/HANDOFF.md`.
2. **The unfalsified checks are what separates five properties from
   CLAIMABLE.** Per property, from `sh gates/run.sh` (excerpt, lines 3-6 and
   8 of its last 12):

       meridian-lane1-p1 lane1 PARTIAL (live GREEN, 1 twin; unfalsified: positions_match_manifest, unevaluable_match_manifest)
       meridian-lane1-p2 lane1 PARTIAL (live GREEN, 1 twin; unfalsified: chain_verifies, fresh_process_identical, pinned_hash_match)
       meridian-lane1-p3 lane1 PARTIAL (live GREEN, 1 twin; unfalsified: positions_match_manifest, three_histories, unevaluable_match_manifest, viewpoint_V1, viewpoint_V3)
       meridian-lane1-p4 lane1 PARTIAL (live GREEN, 2 twins; unfalsified: positions_match_manifest)
       meridian-lane1-p6 lane1 PARTIAL (live GREEN, 3 twins; unfalsified: unevaluable_match_golden)

   Each check needs either a twin, measured in `fixtures/generate.py` and
   emitted by its gate, that drives it nonzero for exactly its planted
   reason, or an operator ruling that the check is not part of what the
   property claims. Not started.
3. **Fix the stale comments** in `gates/verdict.go` and
   `gates/verdict_test.go` (outside this change's files).
4. **Unchanged.** The optional third P7 twin for the two folded legs;
   lanes 2-3; the chain-covered-residue decision; generator RNG via
   `getrandbits`.

**Known limits (minors, listed, not fixed):** `claimability.py --json`
reports lane 1 for every property because it reads only Lane 1 row files,
so a row carrying another lane surfaces as an agreement difference rather
than a refusal of its own; in the vendored copy the pack's commit-sha
wording sub-check is not evaluable and the self-test prints a note saying
so; a twin RED for the wrong reason derives UNCLAIMED in
`gates/claimability.py` while the pack refuses its row with exit 1, so the
agreement step never compares that case; older learnings entries and
handoffs keep the pre-move pack path in their re-verify lines, as immutable
records. No learnings entry was added in this change.
