ts: 2026-09-03T22:21:25Z
commit: e189259 (P7 build present but uncommitted in the working tree at capture)
session: claude-code meridian p7 grpc read api (84f3dedb-209b-4c1e-843d-900eebd4177a), final whole-branch review
status: verified
fact: The P7 design's check table (docs/2026-09-03-grpc-read-api-design.md section 3, as committed at e189259) named the property "byte-for-byte what a local recompute produces" but its `snapshot_matches_local_recompute` check compared only `seq` and the snapshot bytes; `AsOfResponse.prefix_hash` reached the client and was compared against nothing. A server returning a correct snapshot under a fabricated prefix hash would have passed all four checks at 7/7. The gap was in the spec table, not in the implementer's transcription of it. Fixed manifest-neutrally: adding the prefix_hash leg leaves live 0, wrong-feed 3 (already violating on bytes at every viewpoint), mislabeled 0.
basis: negative control after the fix -- a twin that also flips PrefixHash's last hex char makes TestP7WireFidelity FAIL with `check "snapshot_matches_local_recompute" caught 3, planted 0`; reverted, sha256 equal; unmutated gate PASS, `sh gates/run.sh` -> `ok lane1 claimable=7/7`, 18 rows (fix-final report, 2026-09-03T22:29:21Z).
re-verify: grep -c 'resp.GetPrefixHash() != local.PrefixHash' gates/p7_test.go
