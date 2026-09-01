ts: 2026-09-01T04:51:58Z
commit: a77110e
session: claude-code meridian-spec-design (925a8227-a1e8-4e15-b0f6-949ecb97006f)
status: verified
fact: BASELINE's ledger copy-seam pins sha256 over LF-normalized bytes (CRLF → LF before hashing), and its `.gitattributes` forces `* text=auto eol=lf` so working trees match blobs. When MERIDIAN registers verdict rows into BASELINE, the binding hash must be computed post-normalization — a raw-byte hash computed on a Windows checkout will false-positive against the pinned value.
basis: `grep -n "LF-normalized" ~/dev/baseline/ledger/SOURCE.md` → `21:Hashes are over **LF-normalized bytes** (CRLF → LF before hashing) — the`; `cat ~/dev/baseline/.gitattributes` → `* text=auto eol=lf` (captured from baseline@b465661, dirty worktree, 11 entries).
re-verify: grep -n "LF-normalized" ~/dev/baseline/ledger/SOURCE.md && grep -n "eol=lf" ~/dev/baseline/.gitattributes
