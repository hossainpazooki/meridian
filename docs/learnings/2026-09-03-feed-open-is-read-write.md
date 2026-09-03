ts: 2026-09-03T22:22:19Z
commit: e189259 (P7 build present but uncommitted in the working tree at capture)
session: claude-code meridian p7 grpc read api (84f3dedb-209b-4c1e-843d-900eebd4177a), final whole-branch review
status: verified
fact: `feed.Open` (internal/feed/feed.go) runs `os.MkdirAll` and opens the feed with `O_CREATE|O_RDWR` on every call, so every READ through `FeedReader` -- and therefore every gRPC request served by `meridian serve` -- needs write permission on the feed path and would fail on a read-only mount. It also means the exists-then-open guard in `FeedReader` has a race the one-shot CLI never had: delete the feed between the `os.Stat` and the `Open` and a long-lived server creates an empty feed and answers a clean empty ledger, the exact failure the guard exists to prevent. Both are stated as P7 honest limits in STATUS.md, neither is fixed.
basis: internal/feed/feed.go lines 248-251 as read at final review (`os.MkdirAll` then `os.OpenFile(..., os.O_CREATE|os.O_RDWR, ...)`), and the Task 3 reviewer's independent reading of the same lines when confirming the guard is load-bearing.
re-verify: grep -n 'O_CREATE' internal/feed/feed.go
