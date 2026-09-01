# Learnings index

Pointers only; one fact per dated entry. A wrong entry is superseded by a new
entry with a `kills:` reference, never edited.

- [2026-09-01 — baseline GATE_VERDICT schema](2026-09-01-baseline-gate-verdict-schema.md) — the 16-key verdict row shape MERIDIAN's STATUS.md promises to emit.
- [2026-09-01 — baseline hashes LF-normalized](2026-09-01-baseline-hashes-lf-normalized.md) — registration-seam hashes are over CRLF→LF bytes; raw-byte hashing on Windows false-positives.
- [2026-09-01 — baseline twin rows carry `planted`](2026-09-01-baseline-twin-rows-carry-planted.md) — kills the "16 keys" wording: twin rows have a 17th key `planted` {mutation, mutated_rows, expected_violations}; live rows have 16.
