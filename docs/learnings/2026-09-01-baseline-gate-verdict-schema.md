ts: 2026-09-01T04:51:52Z
commit: a77110e
session: claude-code meridian-spec-design (925a8227-a1e8-4e15-b0f6-949ecb97006f)
status: verified
fact: BASELINE emits `GATE_VERDICT` verdict rows as JSON with exactly these 16 keys — cell, checks, content_hash, content_hash_basis, evaluated, kind, lane, params, parallax_sha, parallax_worktree, ran_at, result, rows, runner, scope, surface. MERIDIAN's STATUS.md promises this schema so that registration into the BASELINE catalog is a copy, not a translation; a gate-emitter that invents its own field names breaks that promise silently.
basis: `python -c "import json,glob; f=sorted(glob.glob(r'C:/Users/hossa/dev/baseline/ledger/verdicts/*.json'))[0]; d=json.load(open(f)); print(sorted(d.keys()))"` → `['cell', 'checks', 'content_hash', 'content_hash_basis', 'evaluated', 'kind', 'lane', 'parallax_sha', 'parallax_worktree', 'params', 'ran_at', 'result', 'rows', 'runner', 'scope', 'surface']` — read from baseline@b465661 with a dirty worktree (11 entries; the verdict rows there were freshly re-emitted 2026-08-31, filename ts 20260831T200954).
re-verify: python -c "import json,glob; f=sorted(glob.glob(r'C:/Users/hossa/dev/baseline/ledger/verdicts/*.json'))[0]; print(sorted(json.load(open(f)).keys()))"
