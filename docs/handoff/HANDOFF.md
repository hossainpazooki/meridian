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
