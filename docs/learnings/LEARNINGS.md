# Learnings index

Pointers only; one fact per dated entry. A wrong entry is superseded by a new
entry with a `kills:` reference, never edited.

- [2026-09-01 -- baseline GATE_VERDICT schema](2026-09-01-baseline-gate-verdict-schema.md) -- the 16-key verdict row shape MERIDIAN's STATUS.md promises to emit.
- [2026-09-01 -- baseline hashes LF-normalized](2026-09-01-baseline-hashes-lf-normalized.md) -- registration-seam hashes are over CRLF->LF bytes; raw-byte hashing on Windows false-positives.
- [2026-09-01 -- baseline twin rows carry `planted`](2026-09-01-baseline-twin-rows-carry-planted.md) -- kills the "16 keys" wording: twin rows have a 17th key `planted` {mutation, mutated_rows, expected_violations}; live rows have 16.
- [2026-09-01 -- vacuity guard denominator](2026-09-01-vacuity-guard-denominator.md) -- a guard keyed on the want-set size refused the one check that catches an invented position; the denominator is the universe examined, not the thing looked for.
- [2026-09-01 -- CI measures cross-OS byte identity](2026-09-01-ci-measures-cross-os-byte-identity.md) -- the pinned snapshot hash and all seven fixture trees reproduce on ubuntu-24.04 as on Windows; a state-of-record written before a repo's first CI run understates its own evidence.
- [2026-09-01 -- bare `go test` appends verdict rows](2026-09-01-bare-go-test-appends-verdict-rows.md) -- only `run.sh` clears `gates/out/`; a low claimable count means check the row count (15) before suspecting a gate.
