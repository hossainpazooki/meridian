# STATUS

State of record for MERIDIAN. The README defers to this file; nothing is
claimable anywhere unless it is claimable here.

- **2026-08-31** -- Design locked (`docs/2026-08-31-design.md`); repo scaffolded.
  **Build not started** -- sequencing vs. a competing build is an open pipeline
  decision. No code, no fixtures, no gates existed. Every cell in the table
  below was UNCLAIMED as of this date.

- **2026-09-01** -- Lane 1 built. `sh gates/run.sh` ends
  `ok lane1 claimable=6/6`, over **15 verdict rows** in `gates/out/`
  (P1, P2, P3, P5: one live + one twin each; P4: one live + two twins; P6: one
  live + three twins). Every live cell GREEN, every twin cell RED for exactly
  its planted reason with exact counts.

  **The evidence is committed and pushed.** Corrected 2026-09-03 at
  pick-up: an earlier version of this paragraph said the build lived only
  in an untracked working tree on `lane1-build` and that nothing had been
  pushed. That was true when written. The build landed in four
  evidence-first commits (`527f571` core, `b7f8cf5` fixtures, `70479fe`
  gates + CI, `be1101d` docs) and merged to `main` as PR #1 at `f8825ba`;
  `main` is level with `origin/main`. In this project only the human
  operator writes git history.

  **CI has now run, and P2's cross-OS leg is measured.** Corrected
  2026-09-01 after the merge: an earlier version of this entry said CI had
  never run and that the cross-OS leg was unmeasured, which was true when it
  was written and is no longer. `gates` run
  [33559490763](https://github.com/hossainpazooki/meridian/actions/runs/33559490763)
  on the merge commit `f8825ba` succeeded on `ubuntu-24.04` under Python
  3.14, printing `ok fixtures deterministic and fresh`, `ok import-pin
  self-test (all negative controls caught)`, and `ok lane1 claimable=6/6`
  with the same six rows. So the pinned snapshot hash reproduces on Linux as
  well as Windows, and the fixtures regenerate byte-identically there.
  Two earlier runs (33558902926 push, 33559063650 pull_request) also passed.

  Verdict rows are **regenerated on every run and are not committed** --
  `/gates/out/` is in `.gitignore`. They are build output, not a record; the
  record is this file. Twin counts per property live in
  `fixtures/base/manifest.json` under `p<N>.twin.expected_violations`.

- **2026-09-03** -- P7 (wire fidelity of the gRPC read API) built. `sh
  gates/run.sh` ends `ok lane1 claimable=7/7` over **18 verdict rows** (the
  15 above plus P7: one live + two twins). The API is `Head` / `AsOf` /
  `Reconcile`, read-only by construction of `api/meridian/v1/read.proto`
  (a descriptor test pins exactly those three unary methods). The gate is a
  gRPC client over an in-process listener that rehashes the bytes it
  receives and recomputes locally; twin 1 serves `fixtures/p2/mutated` as
  if it were base (self-consistent hashes, wrong content), twin 2 serves the
  right bytes under the wrong `snapshot_hash`. Design:
  `docs/2026-09-03-grpc-read-api-design.md`.

## Crediting rule

A property is **CLAIMABLE** only when both halves hold, as mechanically
enforced by `gates/claimability.py` (which independently re-derives each
verdict from the row's own contents rather than trusting the row's `result`
label) and, at emit time, by `Emit` in `gates/verdict.go`:

- **Live** -- exactly one live row for the property, with `result` GREEN. A
  live row whose `checks` map is empty, or that carries any check with a
  missing or non-positive `evaluated` denominator, is refused rather than
  credited: a gate that examined nothing is not a gate that passed.
- **Twin** -- **every** twin row RED, with its `checks` map **exactly equal**
  to its `planted.expected_violations` (full dict equality, over the union of
  both key sets, so an expectation with no computed check and a computed check
  with no expectation are both refusals), and at least one check non-zero.

**P4 and P7 have two twins each and P6 has three, and all of them must hold** -- one red
twin does not credit a property that plants three defects. A gate that has
never run red proves nothing.

Verdicts are emitted as `GATE_VERDICT` rows in BASELINE's ledger schema
(surface, lane, cell, result, per-check planted-vs-caught counts, repo sha +
worktree state, content hash + basis, replay command; twin rows carry a 17th
key `planted`), so earned cells can register into the BASELINE catalog as a
copy, not a translation.

## Claimability -- Lane 1 (local, Go core)

| # | Property | Live | Twin | Status |
|---|---|---|---|---|
| P1 | At-most-once fill ingestion | GREEN | RED | CLAIMABLE |
| P2 | Deterministic replay, byte-identical snapshot | GREEN | RED | CLAIMABLE |
| P3 | PIT-correct corporate actions (incl. amendment) | GREEN | RED | CLAIMABLE |
| P4 | Fail-closed valuation (2 twins) | GREEN | RED | CLAIMABLE |
| P5 | Reconciliation proven able to fail | GREEN | RED | CLAIMABLE |
| P6 | Portfolio math (average cost, P&L) (3 twins) | GREEN | RED | CLAIMABLE |
| P7 | Wire fidelity of the gRPC read API (2 twins) | GREEN | RED | CLAIMABLE |

Every RED above is red **for its planted reason with its exact planted
counts**; a merely-red twin does not credit a cell.

The Twin column holds one word per property because that is the schema
`gates/claimability.py` parses, but **P4 and P7 have two twin rows each and P6
has three**, and a single RED there means **every** one of them went red as
planted -- the counts are in the dated entry above and the per-twin rows in
`gates/out/`.
P4's twins are `silent_zero_and_stale_carry_forward` and
`valuation_omitted_without_declaring_unevaluable`; P6's are `fill_qty_plus_one`,
`price_plus_one` and `invented_untraded_position`. P7's are
`wrong_feed_served_as_base` and `hash_field_mislabeled`.

## Honest limits

Measured facts about what the seven cells above do and do not establish. These
are not caveats added for modesty -- each one was found by measurement during
the build, and each bounds a claim that would otherwise be read as stronger
than the evidence. **The first four trace to an entry in
`docs/2026-09-01-lane1-build-ledger.md`, which records the measurement that
established each, and the fifth (P7) to the 2026-09-03 design and final
review** -- so they are checkable rather than merely asserted. The
sixth (no production claim) is not a finding at all: it is a scope wall
declared in `docs/2026-08-31-design.md` section 6 before any code existed, and
it is checkable a different way -- by the absence of anything in the repo that
would falsify it.

- **P5 demonstrates CROSS-IMPLEMENTATION AGREEMENT, not independent
  verification.** The Python naive fold is a **same-contract
  reimplementation**: it and the Go fold were written from the same written
  contract, so a contract-level misunderstanding is reproduced identically on
  both sides and the gate stays green. The import-pin
  (`gates/importpin.py`) gives **structural** independence -- no shared code --
  and never epistemic independence. `fixtures/p6/golden.json` is the only
  artifact in the build that escapes this circularity, because a human derived
  its twelve leaves from the arithmetic. P5's claim is "reconciles against an
  independently-implemented fold", never "independently verified correct".

- **The feed's hash chain does not protect its tail.** A payload edit to the
  **last** record, a truncation to a shorter valid prefix, and a tail record
  replaced by a blank line are all **accepted** by `feed.Open`. Tail integrity
  comes from the externally pinned snapshot hash
  (`fixtures/base/snapshot.sha256`), not from the chain. The package comment
  in `internal/feed/feed.go` states four such limits with the reasoning and the
  measured shapes; read it there rather than trusting a summary.

- **Three checks are never falsified anywhere in the build**:
  `fresh_process_identical` (P2 -- the determinism claim's own headline check),
  `three_histories` (P3), and `unevaluable_match_golden` (P6). Each reads 0 in
  every cell of every gate, so by this project's own rule its 0 is not
  evidence. Each now has a test proving that its **comparator discriminates**
  -- driven with knowingly different inputs, it reports the difference
  (`TestReplaysIdenticalDiscriminates`, `TestP3ThreeHistoriesDiscriminates`,
  `TestSetEqualityOverUniverseTable`). That is **not** the same as a twin
  proving the **ledger can produce the defect**. No non-determinism was
  planted into the fold, deliberately; what is demonstrated is that the check
  could report one, not that the system could commit one.

- **Fixture reproducibility is interpreter-dependent.** The fixtures were
  generated on **Python 3.14**, the only interpreter on the build machine, and
  CI pins that exact version. `fixtures/generate.py` drives its stream through
  `random.Random`, whose `randint` and `choice` are **not** contractually
  stable across CPython versions (only `getrandbits`/`random()` are). On a
  different interpreter MINOR the fixtures may not reproduce.
  `fixtures/generate_test.sh` is the detector -- and a failure there would
  present as fixture staleness, not as an interpreter difference.
  Narrowed 2026-09-01: the fixtures are now known to reproduce byte-identically
  on **two operating systems** under Python 3.14 (Windows locally, ubuntu-24.04
  in CI run 33559490763). What remains unverified is any OTHER Python minor;
  the OS leg is no longer in doubt.

- **P7 measures an in-process listener, not a network.** Its verdict rows
  come from a `bufconn` transport inside one test process. Nothing is
  claimed about TCP behaviour under load, TLS, authentication,
  authorisation, concurrency, or backpressure. `meridian serve` over
  loopback TCP is exercised by one smoke test (`Head` answered over the
  wire) that emits no row and ends the process with a kill, so graceful
  stop on SIGINT/SIGTERM is untested (Windows cannot deliver those signals
  to a child process). "Read-only" is a statement about the proto -- three
  methods, all reads, pinned by a descriptor test -- not about the serving
  process's filesystem permissions.
  Measured at final review 2026-09-03, none fixed, all stated: (1) a read
  opens the feed read-write -- `feed.Open` runs `os.MkdirAll` and opens with
  `O_CREATE|O_RDWR`, so `serve` needs write permission on the feed to answer
  reads and would fail on a read-only mount; (2) the exists-then-open guard
  in `FeedReader` has a race the one-shot CLI did not: delete the feed
  between the stat and the open and the server creates an empty feed and
  answers a clean empty ledger; (3) no message-size bound is set, so gRPC's
  4 MB client default caps the snapshot `AsOf` can deliver, and a ledger
  past it would fail with a code absent from the design's status table (the
  base feed is 16 KB; the boundary is unmeasured); (4) a statement's
  `as_of_seq` is parsed and never compared to the requested seq, in the
  gRPC path exactly as in the CLI -- pre-existing, now remotely reachable;
  (5) two comparison legs are never driven non-zero by any twin: the
  `records` leg of `head_matches_local` and the `compared` leg of
  `reconcile_matches_local` (both twins keep 71 records and compare 11
  fields), so their zeros are not evidence; (6) the `proto fresh` step has
  only ever run on Windows until this branch's first CI run.

- **No production claim.** Synthetic, versioned fixtures only. The ledger has
  run nowhere that matters, against no market data, no custodian, and no
  counterparty.

## Lanes 2-3 (empty on purpose)

| Lane | Surface | Live | Twin | Status |
|---|---|---|---|---|
| 2 | ClickHouse as-of read surface (behind Reader protocol) | UNCLAIMED | UNCLAIMED | not promised |
| 3 | Kafka feed transport (guarantees re-proven end-to-end) | UNCLAIMED | UNCLAIMED | not promised |

## Deferred decisions

- Build order vs. intent-workbench -- **resolved** 2026-09-01 by the operator
  invoking the Lane 1 build. No other deferred decision is treated as resolved.
- **D2, chain-covered residue** -- a measured, better feed-chain contract
  (`prev` covering everything since the last record) that closes junk
  injection and the lazy tail forgery with no bound, is byte-identical for
  clean feeds, and therefore migrates rather than rewrites. Deliberately **not
  taken** in Lane 1 because `prev` is a cross-language contract and the Python
  generator was mid-hardening. Awaiting an operator decision; see
  `docs/handoff/2026-09-01-lane1-build.md`.
- gRPC read API -- **resolved** 2026-09-03: built read-only as P7; see the
  dated entry above and `docs/2026-09-03-grpc-read-api-design.md`.
- Cross-language byte-identical twin -- v2 candidate once the snapshot format
  is stable.
- Whether `canon.Marshal` should ever accept non-ASCII (it refuses today, by
  decision, since the spec restricts every string to ASCII).
