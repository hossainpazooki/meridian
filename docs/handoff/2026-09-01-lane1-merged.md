# Handoff -- lane1 merged to main, CI green

2026-09-01. Newest commit this brief describes: `f8825ba` (merge of PR #1,
`lane1-build` into `main`; local `main` clean and level with `origin/main`).
Pick-up measures drift from here.

This brief supersedes nothing in `2026-09-01-lane1-build.md` -- that entry
described the build while it was uncommitted and before CI had ever run. Both
of those facts have changed; this entry records what changed and what it
settled. Read that entry first for the build's own state of record.

## Current state

- **built** -- Lane 1 is merged. All six properties are CLAIMABLE: every live
  cell GREEN, every twin RED with `checks` equal to `planted.expected_violations`.
  P4 carries two twins, P6 three.
  re-verify: `sh gates/run.sh` -> last line `ok lane1 claimable=6/6`
- **built** -- 15 verdict rows per run (P1/P2/P3/P5 one twin each, P4 two, P6
  three). A different count means a re-run or contention, not a gate change --
  see the learnings entry on `gates/out/` accumulation.
  re-verify: `sh gates/run.sh >/dev/null 2>&1 && ls gates/out/*.json | wc -l` -> 15
- **built** -- Whole module green across 8 packages.
  re-verify: `go test ./... -count=1 2>&1 | grep -c '^ok'` -> 8
- **built** -- **CI has now run and passes on Linux**, which is new since the
  previous handoff. Run 33559490763 on `f8825ba`, `ubuntu-24.04`, Python 3.14.
  re-verify: `gh run view 33559490763 --log | grep -E "ubuntu-24.04|ok lane1 claimable"`
- **built** -- STATUS.md marks six properties CLAIMABLE (7 occurrences: six
  table cells plus the crediting rule).
  re-verify: `grep -o CLAIMABLE STATUS.md | wc -l` -> 7
  (`grep -o ... | wc -l`, not `grep -c`: `grep -c` counts LINES and exits 1 on an
  honest zero. That exact mistake was made in this repo's first handoff.)
- **built** -- README carries no counts and no claim language; it defers to
  STATUS.md.
  re-verify: `grep -oE 'CLAIMABLE|GREEN|RED\*' README.md | wc -l` -> 0
- **built** -- The import-pin gate and its negative controls.
  re-verify: `python gates/importpin.py --self-test` -> `ok import-pin self-test (all negative controls caught)`
- **built** -- The build ledger, 77 sections, rescued out of `.git/sdd/` where
  it would have died with the checkout.
  re-verify: `grep -c '^## ' docs/2026-09-01-lane1-build-ledger.md` -> 77
- **not started** -- Lanes 2 (ClickHouse behind the Reader protocol) and 3
  (Kafka transport). Promised by no one; empty cells in STATUS.md.
- **not started** -- The gRPC read API, deferred until P1-P3 were green. They
  now are, so the deferral condition has been met and the decision is live
  again. Read-only if built.

## Locked decisions

Do not relitigate; pick-up may check whether each reason still holds.

1. **Only the operator writes git history.** Every agent in the build was
   blocked from `git commit`/`push` by a hook and emitted commit commands
   instead. Reason: the human owns the history. This is why the whole build
   arrived as one dirty worktree and was committed in four evidence-first
   commits.
2. **Evidence commits before claim commits.** Code, fixtures, gates and CI
   landed before `STATUS.md`. Reason: staging docs first would publish six
   CLAIMABLE cells while the evidence supporting them was still untracked -- a
   page claiming what the repository did not contain.
3. **P5 is cross-implementation agreement, never independent verification.**
   Both folds descend from one written contract, so a contract-level
   misreading reproduces on both sides and the gate stays green. The
   import-pin buys STRUCTURAL independence (no shared code), never epistemic.
   `fixtures/p6/golden.json` is the only artifact escaping the circularity,
   because a human derived its twelve leaves from the arithmetic.
4. **`snapshot.Diff` stays one-sided.** It walks the golden's keys only, which
   is required -- a twin document legitimately carries keys a reduced golden
   never has. The hole that leaves (an invented position scoring zero) is
   closed by separate key-set checks, not by making `Diff` two-sided.
5. **`evaluated` is the size of the universe a check EXAMINED**, not the size
   of the thing it looked for, and `Emit` refuses a check whose `evaluated` is
   missing or `<= 0`. Reason: a check reporting zero violations because it
   examined nothing is a vacuous pass.
6. **Two set-equality helpers, deliberately.** `SetEquality` uses the union as
   its denominator; `SetEqualityOverUniverse` takes the universe explicitly.
   Reason: a helper cannot know a caller's meaningful scope, and a legitimately
   empty published set makes the union vacuous even when the check examined a
   real universe.
7. **P6 compares against its own `golden.json`, not the manifest's lists,** and
   its check names say `_golden` rather than `_manifest`. Reason: P6 runs on
   its own feed at its own `end_seq`; the manifest's lists are scoped to the
   base feed and would silently compare against the wrong thing.
8. **CI pins Python 3.14**, the interpreter the fixtures were generated on.
   Reason: `randint`/`choice` are not contractually stable across CPython
   versions. Moving the pin means regenerating fixtures on the new interpreter
   and confirming `generate_test.sh` byte-matches FIRST.
9. **The chain-covered-residue redesign (D2) is deferred, not rejected.** It is
   measured, better, and backward compatible for clean feeds. Reason for
   deferring: `prev`'s definition is a cross-language contract and every
   fixture, footprint and pinned hash derives from it.

## Reuse map

- `docs/2026-09-01-lane1-build-ledger.md` -- the build's full working record,
  77 sections: every defect found (C1-C29), controller process errors, and
  design decisions, each with the measurement that established it. **Every
  honest limit in STATUS.md except the production-claim scope wall traces to an
  entry here.** Read this before re-deriving anything about why a gate is
  shaped the way it is.
- `gates/manifest.go` -- `SetEquality`, `SetEqualityOverUniverse`,
  `PositionKeys`, `UnevaluableInstruments`, `LoadManifest` and the typed
  accessors. Use these rather than hand-rolling set comparison in a new gate.
- `gates/verdict.go` -- `Emit` and the crediting rule. Read its `Counts` doc
  comment before adding a check; it explains what `evaluated` means and names
  the two defects that came from that being implicit.
- `gates/p1_test.go` -- the reference gate. `gates/p6_test.go` is the reference
  for a gate with multiple twins and a non-manifest authority.
- `gates/claimability.py` -- the SECOND, independent enforcement of the
  crediting rule. It re-derives from row contents rather than trusting
  `Emit`'s own `result` field; that independence caught a corrupted twin whose
  `result` still said RED.
- `fixtures/generate.py` -- the generator and its dozen guards. Every published
  expectation is measured; several guards prove their own necessity (the
  phantom twin asserts `leaf_diff` is blind to an invented position).
- `internal/feed/feed.go` -- its package comment states four honest limits about
  what a bare hash chain does and does not protect. Read it before assuming
  the chain covers the tail.

## Invariants

- **A gate that has never run red proves nothing.** A property is CLAIMABLE
  only when its live cell is GREEN and EVERY twin is RED with `checks` equal to
  `planted.expected_violations`. Both `Emit` and `claimability.py` enforce
  this, independently and deliberately.
- **Never weaken an expectation to make a guard pass.** If a generator guard
  fires, the planted structure is wrong and the fixture changes -- not the
  assertion.
- **A check's name and message must not claim more than its measurement.**
  This build corrected that failure six times, including twice where a correct
  rule rested on a false stated reason, and once in the honest-limits section's
  own summary line.
- **STATUS.md is the state of record; README carries no counts.** Two sources
  of truth guarantee drift.
- **No floats anywhere; no wallclock in the fold.** Either one silently kills
  byte-identical replay.
- **The feed is the only durable input.** Nothing derived is ever appended.
- **Any change to a cross-language expectation is briefly RED in one direction
  or the other** while it lands, because `Emit` refuses a row whose two sides
  disagree. That is correct; a harness tolerating mismatches during
  transitions is exactly the hole that let eight zero-valued expectations go
  unenforced during the build.
- **`gates/out/` is regenerated per run and gitignored.** Only `run.sh` clears
  it; a bare `go test` appends. Check the row count (15) before suspecting a
  gate.

## Open / next

1. **First, and it is now unblocked:** the gRPC read API was deferred until
   P1-P3 were green. They are. Decide whether to build it -- read-only if so, a
   write path would drag transport into P1's claim.
2. **Three checks remain unfalsified anywhere in the build**:
   `fresh_process_identical` (P2), `three_histories` (P3),
   `unevaluable_match_golden` (P6). Each has a test proving its comparator
   DISCRIMINATES, which is not a twin proving the ledger can produce the
   defect. Stated in STATUS.md's honest limits. Closing any of them means a new
   twin, and for P2 that would mean deliberately planting non-determinism --
   which is why it was not done.
3. **D2, chain-covered residue.** Measured, better, backward compatible for
   clean feeds; it closes junk injection and the lazy tail forgery with no
   bound and makes the crash-loop brick impossible by construction. Its cutover
   gate is `UnparseableLines() == 0` per feed. The ledger's D2 section carries
   the full migration matrix, including that it breaks BACKWARD for feeds
   carrying residue, so rollback after any recovery is unsafe.
4. **The generator's dependence on `random.Random` internals.** Routing its
   stream through `getrandbits` -- the one call with a cross-version guarantee --
   would make fixtures reproducible on any interpreter. Not done because it
   regenerates every planted value and the pinned snapshot hash.
5. **Six Minor gate items** listed at the end of the ledger, none affecting a
   claim.
6. **BASELINE registration.** The six earned cells are emitted in BASELINE's
   `GATE_VERDICT` schema so registration is a copy rather than a translation.
   The seam itself is undesigned; note BASELINE hashes LF-normalized bytes.
