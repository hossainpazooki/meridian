ts: 2026-09-03T22:19:02Z
commit: e189259 (P7 build present but uncommitted in the working tree at capture)
session: claude-code meridian p7 grpc read api (84f3dedb-209b-4c1e-843d-900eebd4177a), skeptic-verifier dispatch
status: verified
fact: Emit's twin rule (gates/verdict.go) refuses a check KEY that has a planted expectation but was never computed -- two-value map indexing, so a missing key cannot read as 0 -- but it cannot tell a wired check whose increment path is dead (present, stuck at 0) from a genuine zero when the planted expectation is also 0. Emit guarantees "every planted key was computed and matches", not "every check exercised its violation branch". P7 is safe from this only because each of its four checks carries a nonzero expectation in at least one twin (head 1, rehash 3, recompute 3, reconcile 2); a future gate whose every twin plants 0 for some check has an unfalsified check by construction.
basis: removing `snapshot_matches_local_recompute` from p7Check's Counts -> `p7_test.go:189: P7 twin check "snapshot_matches_local_recompute" has a planted expectation (3) but was never computed`; removing `snapshot_rehash_matches_claimed` -> `has a planted expectation (0) but was never computed` on the wrong-feed twin (first Emit call, expectation 0 there); file restored, sha256 equal before/after.
re-verify: grep -c 'was never computed' gates/verdict.go
