# Handoff — meridian-spec-design

2026-09-01. Newest commit this brief describes: `a77110e` (commit zero,
`main`, 0 ahead / 0 behind `origin/main` at 2026-09-01T04:51:52Z) — pick-up
measures drift from here.

## Current state

- **built** — Design locked: 6 properties with twins, architecture, money
  rules, amended landing. All seven seed questions resolved; decisions table
  in the doc's §0.
  re-verify: `head -40 docs/2026-08-31-design.md` (the §0 table is the state)
- **built** — Seed archived verbatim with a superseded-note on its §6 (so the
  pair can't be read as contradicting).
  re-verify: `grep -n "superseded" docs/2026-08-31-seed-v2.md`
- **built** — README: premise (thesis #1), value framing, one mermaid
  diagram, six property one-liners, scope walls, DATUM lineage. Carries no
  counts by design.
  re-verify: `awk '/UNCLAIMED|GREEN/{n++} END{print n+0}' README.md` → expect 0
  (awk, not `grep -c`: grep exits 1 on the honest zero)
- **built** — STATUS.md as state of record: crediting rule, 12 Lane-1 cells
  UNCLAIMED, lanes 2–3 "not promised", deferred decisions.
  re-verify: `grep -o UNCLAIMED STATUS.md | wc -l` → expect 17 (12 lane-1
  cells + 4 lane-2/3 cells + 1 in the state-of-record prose)
- **built** — Commit zero pushed by operator to
  `github.com/hossainpazooki/meridian` (private/public choice was theirs).
  re-verify: `git status -sb` → `## main...origin/main`, clean, no ahead/behind
- **planned, NOT started** — All Go code, fixtures, gates, CI, implementation
  plan. Nothing beyond docs exists; every claimability cell is UNCLAIMED.
- **assumed, unverified** — The README mermaid block has never been rendered.
  Syntax is standard flowchart (incl. a `<-.->` bidirectional dotted edge);
  eyeball it on GitHub. First cheap task for any session.

## Locked decisions

Do not relitigate; pick-up may check whether each reason still holds.

1. **Name: MERIDIAN** — the reference line as-of reads are taken against;
   continues the VANTAGE/PARALLAX/BASELINE observational-reference family.
2. **Family umbrella: DATUM** — the geodetic reference frame points, angles,
   and lines are defined against, + singular of data. OBSERVATORY was
   explicitly rejected: a metaphor you must be told, not a description you can
   parse — the bar is intent-plane's clarity register. (design doc §7)
3. **BASELINE is the claim surface, not a displaced slot** — supersedes the
   seed's §6 "replaces BASELINE": the 08-31 BASELINE build made it the public
   catalog MERIDIAN registers into. MERIDIAN's verdicts use BASELINE's
   `GATE_VERDICT` schema from day one so registration is a copy, not a
   translation. (see learnings entry on the schema)
4. **P6 bounded** — average-cost basis + realized/unrealized P&L as fold
   state, snapshot-resident, never in the feed; no lot selection. Reason:
   derived records in the feed would need restatement-chasing; snapshot
   residency makes P3 interaction free. Lot selection is the scope cliff.
5. **Action set** — whole-share splits + cash dividends + **one amended
   action**. Reason: the amendment is the restatement-of-a-restatement demo
   (three viewpoints, three consistent histories) at near-zero cost; more
   action *types* add machinery without new concepts.
6. **Single currency** — FX doubles P4's surface and proves no new property
   class. Named non-goal.
7. **P5 = field-level custodian recon** — generator-embedded naive Python
   fold emits custodian statements; import-pinned (a gate asserts it imports
   nothing from the Go tree). Byte-identical cross-language twin deferred to
   v2. Reason: emitting a statement requires a second fold anyway; field-level
   is honest independence at ~50 lines.
8. **Pure recompute reads** — every as-of read replays the feed prefix; no
   materialized derived state anywhere. Reason: PIT correctness by
   construction; the restate-the-derived-table bug class cannot exist.
9. **Prices ride the feed** as events (closed set: `fill`, `price`, `action`,
   `action_amendment`). Reason: makes P4 PIT-native for free.
10. **Money** — integer minor units, no floats; state is `(total_cost, qty)`;
    division at exactly one point (sell cost relief, round-half-even).
    Reason: byte-identity with P6 math inside the fold.
11. **Thesis #1 load-bearing** (fold/recompute); #3 is the lineage sentence;
    #2 stays P1's section header (rhymes with /intent — can't lead twice).
12. **gRPC deferred** until P1–P3 green; **read API only** if built (a write
    path would drag transport into P1's claim).
13. **Build order vs. intent-workbench: deliberately undecided** — a pipeline
    question. The design does NOT authorize the build.

## Reuse map

- `docs/2026-08-31-design.md` — the spec; the implementation plan derives
  from it, not from memory.
- `~/dev/baseline/ledger/` — the verdict row schema to copy
  (`verdicts/*.json`, 16 keys), the copy-seam binding pattern (`SOURCE.md`:
  sha256 per file, LF-normalized bytes), and the gate that holds it
  (`scripts/check-ledger.mjs` + its negative controls).
- `~/dev/intent-plane` — append-only feed semantics, fsync discipline,
  derived idempotency keys, the import-pinned independent-verifier pattern.
- `~/dev/vantage` — no-lookahead gate with exact-planted-count discipline
  (P3's twin is this, ported).
- `~/dev/parallax` — Reader protocol (never a path/connection string), twin
  rule, bounded claims via STATUS.md.

## Invariants

- **The twin rule**: a property is CLAIMABLE only when live is GREEN and twin
  is RED for exactly the planted reason, exact counts. A gate that has never
  run red proves nothing. Violating this makes every downstream claim
  (STATUS.md, BASELINE registration, CV) dishonest at once.
- **STATUS.md is the state of record**; README never carries counts. Two
  sources of truth = guaranteed drift.
- **No performance vocabulary, no benchmarks** anywhere in the repo. The
  speed claim is "replayable and byte-identical," full stop.
- **The feed is the only input**; nothing derived is ever appended to it.
  Break this and restatement-of-derived-records enters the design.
- **No wallclock in the fold; no floats anywhere.** Either one silently kills
  P2's byte-identity.
- **The naive fold stays import-pinned** or P5's independence is decorative.
- **CV/site silence** until all six properties hold both cells.
- **Durable artifacts live in this repo, not `~/dev/briefs/`** — operator
  stated 2026-09-01 that briefs are not synced to any remote; anything left
  there is single-machine.

## Open / next

1. **First:** the Q7 sequencing call (MERIDIAN vs. intent-workbench) —
   operator decision, blocks the build. Nothing code-shaped happens before it.
2. **On greenlight:** write the implementation plan from the design doc
   (brainstorm's terminal step was deliberately parked on Q7).
3. **Cheap anytime:** verify the README mermaid renders on GitHub.
4. **Later seam to design once rows exist:** the MERIDIAN→BASELINE
   registration copy (SOURCE.md-style sha256 binding; note BASELINE hashes
   LF-normalized bytes — see learnings).
