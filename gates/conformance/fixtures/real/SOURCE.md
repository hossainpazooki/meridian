# Real-row binding

One live and one twin row per governed repo, copied from that repo's
emitter output at a pinned commit and never edited (the hand-copy seam
between an emitter and this pack is hash-bound). Each row is a PASS fixture: `test.mjs`
recomputes every sha256 below over LF-normalized bytes, fails on any
listed file that is missing or altered, and fails on any row under this
directory that is not listed.

Layout: `<repo>/<file>.json`, bare rows (not case-wrapped). Re-copy on an
emitter change; do not patch. Record the source commit next to each repo.

## meridian

Source commit `97d815c9d95b9bac7a85f69b1cc66f6e5310f45e`, resolved from
meridian's own history, from `gates/out/`. Before copying, all
three rows present at that commit were confirmed `"gate_worktree": "clean"`
and `"gate_sha"` starting `97d815c`. One live row and one of the two twin
rows are bound here (`wrong_feed_served_as_base`, the earlier of the two by
`ran_at`); the second twin (`hash_field_mislabeled`) is not copied, per the
one-live-one-twin layout.

## sha256

97fc5b5a19f1f4913bf9b7d3d65d3c688eaaa235a7499450fe776562d3ff9293  meridian/meridian-lane1-p7-live-20260914T220041.287249Z.json
e592704e7da8604d3e626d0e2e17cefcb99a85d01ee19ac5ab51da0936cce442  meridian/meridian-lane1-p7-twin-20260914T220041.330525Z.json
