# Learnings index

Pointers only; one fact per dated entry. A wrong entry is superseded by a new
entry with a `kills:` reference, never edited.

- [2026-09-01 -- baseline GATE_VERDICT schema](2026-09-01-baseline-gate-verdict-schema.md) -- the 16-key verdict row shape MERIDIAN's STATUS.md promises to emit.
- [2026-09-01 -- baseline hashes LF-normalized](2026-09-01-baseline-hashes-lf-normalized.md) -- registration-seam hashes are over CRLF->LF bytes; raw-byte hashing on Windows false-positives.
- [2026-09-01 -- baseline twin rows carry `planted`](2026-09-01-baseline-twin-rows-carry-planted.md) -- kills the "16 keys" wording: twin rows have a 17th key `planted` {mutation, mutated_rows, expected_violations}; live rows have 16.
- [2026-09-01 -- vacuity guard denominator](2026-09-01-vacuity-guard-denominator.md) -- a guard keyed on the want-set size refused the one check that catches an invented position; the denominator is the universe examined, not the thing looked for.
- [2026-09-01 -- CI measures cross-OS byte identity](2026-09-01-ci-measures-cross-os-byte-identity.md) -- the pinned snapshot hash and all seven fixture trees reproduce on ubuntu-24.04 as on Windows; a state-of-record written before a repo's first CI run understates its own evidence.
- [2026-09-01 -- bare `go test` appends verdict rows](2026-09-01-bare-go-test-appends-verdict-rows.md) -- only `run.sh` clears `gates/out/`; a low claimable count means check the row count (15) before suspecting a gate.
- [2026-09-03 -- eighteen verdict rows after P7](2026-09-03-eighteen-verdict-rows-after-p7.md) -- kills the "15 rows" figure: a clean run.sh now writes 18 (P7 = 1 live + 2 twins); the append-only warning in the killed entry still holds.
- [2026-09-03 -- Emit refuses a missing key, not a dead check](2026-09-03-emit-refuses-missing-key-not-dead-check.md) -- a wired check with a dead increment path passes whenever the twin plants 0; every P7 check has a nonzero expectation in some twin, which is what makes it safe.
- [2026-09-03 -- wire-fidelity check missed prefix_hash](2026-09-03-wire-fidelity-check-missed-prefix-hash.md) -- the P7 spec table compared seq + bytes but not AsOfResponse.prefix_hash; a correct snapshot under a fabricated prefix hash would have passed 7/7; fixed manifest-neutrally, negative control fires.
- [2026-09-03 -- proto-fresh hardcoded list fails open](2026-09-03-proto-fresh-hardcoded-list-fails-open.md) -- a freshness gate over a named file list never sees a file outside the list; replaced by enumerate-both-ways + refuse-on-empty.
- [2026-09-03 -- feed.Open is read-write](2026-09-03-feed-open-is-read-write.md) -- every read opens O_CREATE|O_RDWR, so `serve` needs write permission and the exists-then-open guard races under a long-lived server; stated, not fixed.
