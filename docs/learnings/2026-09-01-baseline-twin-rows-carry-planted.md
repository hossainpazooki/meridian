ts: 2026-09-01T17:55:21Z
commit: 2774c66
session: claude-code meridian-spec-design (925a8227-a1e8-4e15-b0f6-949ecb97006f)
status: verified
kills: 2026-09-01-baseline-gate-verdict-schema.md (the "exactly 16 keys" wording — true of LIVE rows only)
fact: BASELINE twin-cell GATE_VERDICT rows carry a 17th key, `planted`, an object with `mutation` (string), `mutated_rows` (int) and `expected_violations` (check-name -> int). The earlier entry sampled only the first file in sorted glob order, which is the live row. MERIDIAN's twin rows must emit `planted` in exactly this shape or the crediting rule (checks == planted.expected_violations) has nothing to compare against at registration.
basis: `python -c "import json,glob; f=[x for x in glob.glob(r'C:/Users/hossa/dev/baseline/ledger/verdicts/*.json') if 'twin' in x][0]; d=json.load(open(f)); print(sorted(d.keys())); print(sorted(d['planted'].keys()))"` -> 17 keys incl. 'planted'; planted keys ['expected_violations', 'mutated_rows', 'mutation'] (baseline@b465661, dirty worktree).
re-verify: python -c "import json,glob; f=[x for x in glob.glob(r'C:/Users/hossa/dev/baseline/ledger/verdicts/*.json') if 'twin' in x][0]; d=json.load(open(f)); print(sorted(d.keys())); print(sorted(d['planted'].keys()))"
