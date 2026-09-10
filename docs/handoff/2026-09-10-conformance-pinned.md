# Handoff -- conformance pinned: the pin was empty, CI went red, the pin is fixed

2026-09-10. Newest MERIDIAN commit this brief describes: **`cdee383`**
(`docs: conformance-adopted handoff`, main, level with origin). Pick-up
measures drift from here. One file is **uncommitted** on top of it:
`gates/datum/PIN`, rewritten from 0 bytes to a real pin.

This entry supersedes the "Open / next" of `2026-09-09-conformance-adopted.md`,
which said the pin was not written and the adoption was blocked on a commit
elsewhere. Both halves are now false: that commit landed, and the pin was
written -- empty -- and pushed.

**The governing text** is DATUM, a private governing text for the operator's
data-platform repositories. This repo vendors its conformance pack and cites
it for nothing else; every claim below is checkable from this repo, its CI,
and the vendored pack.

## Current state

- **built, committed** -- the pack vendored at `gates/datum/`, byte for byte
  identical to the governing text's repo at the commit its pin now names,
  across all 64 files, with no file on either side that the other lacks.
  re-verify: `head -1 gates/datum/PIN` -> `datum d4f8dbf95b0098e8a8a3b7ae384314dc9defa9ff`, and `node gates/datum/check.mjs gates/out --verify-pin >/dev/null && echo pinned` -> `pinned`
- **built, committed** -- `Emit` writes the shared row: `schema`,
  `gate_sha`, `gate_worktree`. `TestEmit` pins the 17/18-key list.
  re-verify: `go test ./gates -run TestEmit -count=1` -> `ok`
- **built, committed** -- `gates/run.sh` runs the pack's self-test before
  the gates, `claimability.py --status --check` after them, then the pack
  over `gates/out` with `--verify-pin`, then fails unless the pack and
  `claimability.py` agree on the CLAIMABLE count.
  re-verify: `sh gates/run.sh 2>&1 | tail -2` -> `ok 18 rows conform, 7 surfaces` then `ok conformance pack agrees: claimable=7`
- **built, committed** -- STATUS.md's claimability table is generated from
  the rows between markers and compared by `--check`.
  re-verify: `python gates/claimability.py gates/out --check STATUS.md` -> `ok STATUS.md generated claimability block is fresh`
- **broken on `main`, fixed locally, NOT committed** -- `gates/datum/PIN`
  was committed empty (0 bytes, blob `e69de29`, added in `b3d19ce`) and
  pushed, and CI run 34410348602 on `cdee383` failed at
  `== conformance pack over the rows`. The rest of that run was green,
  including the pack's own self-test and `ok lane1 claimable=7/7`. The pin
  in the working tree is now real: 65 lines, first line `datum <sha>`, one
  sha256 per vendored file.
  re-verify: `git cat-file -s $(git rev-parse cdee383:gates/datum/PIN)` -> `0`; `wc -l < gates/datum/PIN` -> `65`
- **red** -- CI on `main`. It stays red until the pin above is committed
  and pushed. Nothing was mis-credited by the red run: the pack refused
  with exit 2, unevaluable.
  re-verify: `gh run list --branch main --limit 1 --json headSha,conclusion --jq '.[0]|"\(.headSha[0:7]) \(.conclusion)"'` -> `cdee383 failure`, until the fix lands
- **not started** -- copying one live and one twin row into the pack's
  real-fixture corpus. Needs rows from a clean tree at a pushed sha;
  today's rows carry `gate_worktree: dirty` because the pin fix is
  uncommitted.
- **not started, unchanged** -- lanes 2-3; the three never-falsified
  checks (`fresh_process_identical`, `three_histories`,
  `unevaluable_match_golden`); the optional third P7 twin driving the
  `records` and `compared` legs non-zero; D2; generator RNG via
  `getrandbits`.

## Locked decisions

1. **Write the pin through a guard, never a bare shell redirect.** Reason:
   `--write-pin` streams to stdout, so a redirect creates the target
   before node runs and any failure leaves a 0-byte file that reads like a
   written pin -- which is exactly what was committed. Write to a temp
   file, require the first line to match `datum <40-hex>` and the line
   count to equal the vendored file count plus one, then move it into
   place. The durable fix belongs in the pack, not here; proposed there.
   Learnings: `docs/learnings/2026-09-10-empty-pin-committed-and-refused.md`.
2. **The pack lives at `gates/datum/`, not `vendor/`.** Reason: Go treats a
   top-level `vendor/` as module vendoring and every build in the module
   fails with `inconsistent vendoring`. Measured 2026-09-09.
3. **Two derivations of the claimability table, kept and diffed.** Reason:
   agreement between independent derivations is evidence; one derivation
   is a claim. `run.sh` fails when `claimability.py` and the pack differ
   on the CLAIMABLE count. Neither is retired.
4. **STATUS.md's claimability table is generated; the dated narrative
   above it is hand-written and corrected in place.** Reason: status is
   derived, never authored -- but the record of what was true when is not
   derivable and must not be regenerated away.
5. **A pin names a commit, never a working tree.** Reason: the pin's whole
   job is to say which bytes were reviewed; a pin to an uncommitted tree
   names bytes nobody else can fetch.
6. **Carried, unchanged:** the nine decisions of
   `2026-09-06-datum-adoption-seed.md`, the seven of
   `2026-09-09-conformance-adopted.md`, the six build decisions of
   `2026-09-03-p7-grpc-read-api-built.md`, and the process lock -- say
   "one more commit coming" before pushing, because the operator merges
   within minutes.

## Reuse map

- `gates/datum/check.mjs --write-pin <sha>` / `--verify-pin` -- the pin.
  Read `verifyPin()` before changing anything about the vendored layout:
  it refuses a listed file that is missing or altered AND an unlisted file
  that is present, so adding a file to `gates/datum/` without repinning
  is a refusal, by design.
- `gates/datum/reasons.md` -- the refusal vocabulary. Read it before
  adding a gate that changes a row's shape.
- `gates/verdict.go`, the row map -- the only place row keys are written.
  `gates/verdict_test.go` -- the sorted key list, in one string per cell.
- `gates/claimability.py` -- `derive()` is the per-property vocabulary,
  `render()` / `check_block()` the STATUS.md block, `PROP_NAMES` the only
  authored content in that table.
- `gates/run.sh` -- three pack-related sections; the count cross-check is
  the last five lines.
- `docs/handoff/2026-09-09-conformance-adopted.md` -- the adoption's own
  record, still accurate except for its "Open / next".

## Invariants

- **`run.sh` clears `gates/out/`; a bare `go test` appends.** Duplicate
  rows read as `second live cell` or `duplicate twin mutation` from the
  pack and as `duplicate verdict rows` from `claimability.py`.
- **Every check has a nonzero expectation in at least one twin.** A check
  whose every twin plants 0 is unfalsified; `Emit` refuses a missing key,
  not a dead increment path.
- **Disagreement between `Emit`, `claimability.py` and the pack is a
  finding.** Do not align by loosening any of the three.
- **The pin must name a commit whose bytes match the vendored files.**
  Verify with a per-file diff against that commit before writing a pin,
  not after.
- **Nothing is CLAIMABLE anywhere unless STATUS.md says so**, and the
  generated block derives that from the rows rather than replacing it.
- **The governing text is private; this repo cites only what a reader can
  check.** One naming per document, no paths, revisions or rule numbers.
- **Only the operator writes git history.**

## Open / next

1. **Commit and push the pin; watch CI go green.** This is the whole of
   the next step, and it overrides everything below -- the gate on `main`
   is red and nothing should be built on it. The pin in the working tree
   is already verified against the vendored bytes and the full local gate
   ends `ok conformance pack agrees: claimable=7`, exit 0.
   `git add gates/datum/PIN && git commit -m "fix: write the pack pin, CI was red on an empty pin"`
2. **Then bind the real rows.** With a clean tree at the pushed sha,
   re-run `run.sh` so rows carry `gate_worktree: clean`, copy one P7 live
   and one P7 twin row into the pack's real-fixture corpus in the
   governing text's repo, hash-bind them there, and re-vendor and repin
   here if that changes the pack's files (it does -- the fixture set is
   inside the pinned tree).
3. **Unchanged.** The optional third P7 twin; lanes 2-3; the three
   never-falsified checks; the two P7 legs never driven non-zero.

**Learnings gate state:** `node ~/dev/rigor/scripts/check-learnings.mjs docs/learnings`
exits 1 on the pre-existing `2026-09-01-vacuity-guard-denominator.md` only
(prose form, all seven fields missing). Do not edit it; superseding it is
the operator's call.
