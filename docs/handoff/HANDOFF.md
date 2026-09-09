# Handoff index

Pointers only; evidence lives in the entries. Entries are immutable -- a later
session writes a new entry, never edits an old one.

- [2026-09-01 -- meridian-spec-design](2026-09-01-meridian-spec-design.md) --
  design locked + repo scaffolded at `a77110e`; build not started; Q7
  sequencing is the blocker.
- [2026-09-01 -- lane1-build](2026-09-01-lane1-build.md) -- Lane 1 built, all
  six properties CLAIMABLE (`ok lane1 claimable=6/6`, 15 verdict rows);
  everything UNCOMMITTED on `lane1-build` and CI has never run.
- [2026-09-01 -- lane1-merged](2026-09-01-lane1-merged.md) -- PR #1 merged
  at `f8825ba`; CI green on ubuntu-24.04, so P2's cross-OS leg is now MEASURED;
  gRPC read API unblocked; three checks still unfalsified.
- [2026-09-03 -- p7-grpc-read-api-built](2026-09-03-p7-grpc-read-api-built.md) --
  read-only gRPC API + P7 wire-fidelity gate built on `p7-grpc-read-api`
  (`ok lane1 claimable=7/7`, 18 rows), review chain closed MERGEABLE; ALL
  UNCOMMITTED, CI unmeasured; operator commit sequence inside.
- [2026-09-03 -- p7-merged](2026-09-03-p7-merged.md) -- PR #2 merged at
  `ecdd82d`, CI green on ubuntu-24.04 (run 33816126081); one docs commit
  `9898cba` still on the branch, lands with this entry.
- [2026-09-03 -- p7-closed](2026-09-03-p7-closed.md) -- everything landed on
  `main` at `014992e` (PRs #1-#3 merged); clean anchor for the next pick-up;
  next = BASELINE registration of the seven cells.
- [2026-09-06 -- datum-adoption-seed](2026-09-06-datum-adoption-seed.md) -- seed
  for another session, nothing run here: BASELINE registration RETIRED by
  operator ruling under the family's private governing text; next =
  design/STATUS amendments now, pack vendoring + `gate_sha` emitter change
  once the pack exists.
- [2026-09-09 -- conformance-adopted](2026-09-09-conformance-adopted.md) --
  pack vendored under `gates/datum/`, emitter writes `schema` /
  `gate_sha` / `gate_worktree`, checker red for exactly five reasons per
  row before and green (7 CLAIMABLE) after, STATUS table generated; ALL
  UNCOMMITTED, `PIN` waits on one commit in the governing text's repo.
