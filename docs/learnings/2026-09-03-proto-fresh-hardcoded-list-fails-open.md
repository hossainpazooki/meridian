ts: 2026-09-03T22:21:58Z
commit: e189259 (P7 build present but uncommitted in the working tree at capture)
session: claude-code meridian p7 grpc read api (84f3dedb-209b-4c1e-843d-900eebd4177a), final whole-branch review
status: verified
fact: A generated-code freshness gate that `cmp`s a hardcoded list of output files fails OPEN on any generated file outside the list: the first `proto fresh` step in gates/run.sh compared only read.pb.go and read_grpc.pb.go, so a second .proto, a buf.gen.yaml change emitting a third file, or a committed generated file with no regenerated counterpart would all have passed unexamined. Replaced by enumerating every regenerated *.pb.go and comparing in both directions, plus refusing when buf generate produced nothing at all (the vacuous-pass case).
basis: negative controls on the replacement -- blank line appended to api/meridian/v1/read.pb.go -> `FAIL generated api/meridian/v1/read.pb.go stale: run go tool buf generate`, no `ok lane1` line, exit 1; read_grpc.pb.go moved away -> `FAIL generated api/meridian/v1/read_grpc.pb.go stale` (cmp against the missing file fires before the reverse loop); both restored, sha256 equal (fix-final report, 2026-09-03T22:29:21Z).
re-verify: grep -c 'produced no \*.pb.go' gates/run.sh
