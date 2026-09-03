ts: 2026-09-03T23:04:02Z
commit: 4d2b21d
session: claude-code meridian p7 grpc read api (84f3dedb-209b-4c1e-843d-900eebd4177a), post-push CI probe
status: verified
fact: The go.mod `tool`-directive codegen path (`go tool buf generate` with buf v1.72.0, protoc-gen-go v1.36.12, protoc-gen-go-grpc v1.6.2) reproduces the committed api/meridian/v1/*.pb.go byte-for-byte on ubuntu-24.04 / Go 1.26 / Python 3.14, and the seven-property gate is green there: `ok proto fresh` then `ok lane1 claimable=7/7` with the P7 row. On a cold runner the step downloads buf's module graph from the Go module proxy (`go: downloading github.com/bufbuild/buf` appears in the log) and builds buf from source, about 75 s from the import-pin line to the proto-fresh line; nothing in the repo pins or caches that beyond go.sum, so a proxy outage would fail the gate as a build error, not a drift.
basis: `gh run view 33815809956 --log` (push, 4d2b21d) -> `Image: ubuntu-24.04`, `Successfully set up Go version 1.26`, `python-version: 3.14`, `ok import-pin self-test` at 23:02:34.90Z, `ok proto fresh` at 23:03:48.93Z, `P7   | GREEN | RED*,RED*          | YES`, `ok lane1 claimable=7/7` at 23:04:02.63Z; run 33815870881 (pull_request, same sha) identical lines; `grep -c "go: downloading github.com/bufbuild/buf"` over the push log -> 1.
re-verify: gh run view 33815809956 --log | grep -E "ok proto fresh|ok lane1 claimable"
