ts: 2026-09-03T22:09:27Z
commit: uncommitted (P7 gate not yet committed at time of writing)
session: claude-code meridian grpc read api
status: verified
kills: 2026-09-01-bare-go-test-appends-verdict-rows.md (the row count only)
fact: A clean `sh gates/run.sh` now produces 18 verdict rows, not 15: P7 adds one live row and two twin rows (wrong_feed_served_as_base, hash_field_mislabeled). The rest of the killed entry stands -- a bare `go test` still appends, only run.sh clears -- but anyone checking the row count before suspecting a gate must now expect 18.
basis: `sh gates/run.sh >/dev/null 2>&1 && ls gates/out/*.json | wc -l` -> `18`; `python gates/claimability.py gates/out --status STATUS.md` last line `ok lane1 claimable=7/7`.
re-verify: sh gates/run.sh >/dev/null 2>&1 && ls gates/out/*.json | wc -l
