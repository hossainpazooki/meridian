ts: 2026-09-01T21:27:55Z
commit: f8825ba
session: claude-code meridian lane1 build (925a8227-a1e8-4e15-b0f6-949ecb97006f)
status: verified
fact: A bare `go test ./gates/` APPENDS verdict rows to `gates/out/` rather than replacing them; only `gates/run.sh` clears the directory first. Running the suite twice therefore doubles every row, and `claimability.py` refuses the whole run with a duplicate-rows FAIL instead of a claimability count. Anyone reading a low or zero claimable count must check the ROW COUNT first: 15 is correct (P1/P2/P3/P5 one twin each, P4 two, P6 three); more means contention or a re-run, not a gate regression. This bit twice during the build -- once as a mysterious 16-row count I recorded as fact in the ledger, once as an apparent whole-suite collapse to 0/6 that was really two concurrent agents writing the same directory.
basis: `ls gates/out/*.json | wc -l` -> `15` immediately after `sh gates/run.sh`; then `go test ./gates/ -count=1` and the same command -> `30`; then `python gates/claimability.py gates/out` -> three `FAIL duplicate verdict rows for meridian-lane1-p6 twin mutation=... : 2 (concurrent runs writing the same MERIDIAN_VERDICT_DIR?)` lines, exit 1.
re-verify: sh gates/run.sh >/dev/null 2>&1 && ls gates/out/*.json | wc -l
