# Real-row binding

One live and one twin row per governed repo, copied from that repo's
emitter output at a pinned commit and never edited (DATUM rule 8: the
hand-copy seam is hash-bound). Each row is a PASS fixture: `test.mjs`
recomputes every sha256 below over LF-normalized bytes, fails on any
listed file that is missing or altered, and fails on any row under this
directory that is not listed.

Layout: `<repo>/<file>.json`, bare rows (not case-wrapped). Re-copy on an
emitter change; do not patch. Record the source commit next to each repo.

Empty until the first governed repo adopts the pack.

## sha256

