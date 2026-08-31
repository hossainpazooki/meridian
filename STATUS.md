# STATUS

State of record for MERIDIAN. The README defers to this file; nothing is
claimable anywhere unless it is claimable here.

- **2026-08-31** — Design locked (`docs/2026-08-31-design.md`); repo scaffolded.
  **Build not started** — sequencing vs. a competing build is an open pipeline
  decision. No code, no fixtures, no gates exist. Every cell below is
  UNCLAIMED.

## Crediting rule

A property is **CLAIMABLE** only when its live cell is GREEN **and** its twin
cell is RED *for exactly the planted reason, with exact counts* — a gate that
has never run red proves nothing. Verdicts are emitted as `GATE_VERDICT` rows
in BASELINE's ledger schema (surface, lane, cell, result, per-check
planted-vs-caught counts, repo sha + worktree state, content hash + basis,
replay command), so earned cells can register into the BASELINE catalog as a
copy, not a translation.

## Claimability — Lane 1 (local, Go core)

| # | Property | Live | Twin | Status |
|---|---|---|---|---|
| P1 | At-most-once fill ingestion | UNCLAIMED | UNCLAIMED | not built |
| P2 | Deterministic replay, byte-identical snapshot | UNCLAIMED | UNCLAIMED | not built |
| P3 | PIT-correct corporate actions (incl. amendment) | UNCLAIMED | UNCLAIMED | not built |
| P4 | Fail-closed valuation | UNCLAIMED | UNCLAIMED | not built |
| P5 | Reconciliation proven able to fail | UNCLAIMED | UNCLAIMED | not built |
| P6 | Portfolio math (average cost, P&L) | UNCLAIMED | UNCLAIMED | not built |

## Lanes 2–3 (empty on purpose)

| Lane | Surface | Live | Twin | Status |
|---|---|---|---|---|
| 2 | ClickHouse as-of read surface (behind Reader protocol) | UNCLAIMED | UNCLAIMED | not promised |
| 3 | Kafka feed transport (guarantees re-proven end-to-end) | UNCLAIMED | UNCLAIMED | not promised |

## Deferred decisions

- Build order vs. intent-workbench (pipeline question; blocks the build).
- gRPC read API — revisit after P1–P3 are green; read-only if built.
- Cross-language byte-identical twin — v2 candidate once the snapshot format
  is stable.
