# Refusal reasons

The fixed vocabulary `check.mjs` refuses with. Each line is a prefix: a
refusal is `FAIL <file>: <reason>` and the reason starts with one entry
below, possibly followed by the field, key or group it names. A negative
fixture's `expect_reason` is matched against the emitted reason by prefix
(WebAssembly's `assert_malformed` rule); `test.mjs` fails if a refusal is
emitted that starts with none of these, and if any entry below is expected
by no fixture. Rule names in brackets are the entries of the checker's rule
table, the unit `test.mjs --mutate` disables one at a time.

## Row shape, from `schema/gate-verdict.v1.json` [schema]

- `schema: row must be object`
- `schema: missing required field` — followed by the dotted path
- `schema: unknown field` — additional properties are refused; the
  `parallax_sha` / `parallax_worktree` keys, carried over from before this
  pack existed, land here
- `schema: <path> must be integer` / `must be string` / `must be object`
- `schema: <path> must be one of` — enum (`schema`, `kind`, `cell`,
  `result`, `gate_worktree`); a row from an unknown schema version and the
  `UNEVALUABLE:<reason>` form both land here
- `schema: <path> must match` — pattern (`surface`, `content_hash`,
  `gate_sha`, `ran_at`, `runner`)
- `schema: <path> must be >=` — minimum (`lane`, counts, denominators,
  expectations, `rows`)
- `schema: <path> must have at least` — minProperties (`checks`)

## Cross-field, one row

- `unevaluable_reason required` — result is UNEVALUABLE and the reason is
  absent or empty [unevaluable_reason_iff]
- `unevaluable_reason forbidden` — result is not UNEVALUABLE and a reason is
  present [unevaluable_reason_iff]
- `planted required on twin` [planted_iff]
- `planted forbidden on live` [planted_iff]
- `evaluated keys must equal checks keys` [evaluated_keys]
- `evaluated zero forces UNEVALUABLE` — any denominator of 0 with a result
  other than UNEVALUABLE [evaluated_zero]
- `live with violations must be RED` — a live row with any non-zero check
  and result GREEN [live_violations_red]
- `twin with no violations must not be RED` [twin_zero_not_red]
- `twin RED does not match the plant` — `checks` compared with
  `planted.expected_violations` over the union of both key sets; a planted
  check never computed and a computed check never planted both land here
  [twin_plant_match]
- `status literal` — `CLAIMABLE`, `PARTIAL` or `UNCLAIMED` found as a whole
  word in any string anywhere in the row, object keys included
  [status_literal]

## Set, a directory of rows

Set rules run only over rows that passed every row rule.

- `second live cell for` — followed by `<surface>:lane<lane>` [set_one_live]
- `duplicate twin mutation` — followed by the mutation and the group
  [set_twin_unique]
- `live RED needs a human` — a real surface failing is refused, not derived
  into a status [set_live_red_human]
- `twin GREEN needs a human` — a plant the gate missed is refused, not
  derived into a status [set_twin_green_human]

## Expected set, `--expect <file>`

Run only when an expect file is given. The file is JSON,
`{"cells":[{"surface":"<s>","lane":<n>,"twins":["<mutation>",...]}]}`,
and names every (surface, lane) cell the rows directory must hold and the
exact set of twin mutations each must carry (order irrelevant; `[]` for a
live-only cell). A cell is present when any row names it, whether or not
that row passed the row rules. These rules never assert a status: statuses
stay derived.

- `unexpected cell` — followed by `<surface>:lane<lane>`: rows hold a cell
  the expect file does not name [expect_unexpected_cell]
- `expected cell has no rows` — followed by `<surface>:lane<lane>`: the
  expect file names a cell no row belongs to [expect_missing_cell]
- `twin mutation set differs for` — followed by `<surface>:lane<lane>` and
  both lists: the twin mutations the rows carry for a cell are not exactly
  the expected ones, one extra or one missing alike [expect_twin_set]

## Not refusals: exit 2, unevaluable

These are not in the vocabulary because they are not verdicts on a row.
`check.mjs` exits 2, never 0, when: the rows directory is missing or empty;
an entry in it is not a regular file -- a subdirectory, a symlink, a
junction -- named as `unevaluable: <name> is not a regular file` rather
than skipped, so a refusable row cannot hide behind its shape any more than
its extension; a file in it is not JSON; a file in it carries the same key
twice in one object, at any depth (`unevaluable: <file>: duplicate key
"<key>"`) -- a JSON parser keeps one of the two values and drops the other
without a word, so the row is not unambiguous JSON and is judged on
neither; the CLI is given more than one positional argument, or any flag
outside `--json`, `--verify-pin`, `--expect` (a plausible typo included) --
a usage line to stderr; or `--verify-pin` finds a vendored file missing,
unlisted or altered (an empty `PIN` included), or a `PIN` line that names
anything other than one of the pack's own files, once, under the name the
pack's own walk gives it (`listed but not a file of the pack` for a file
reached from outside the pack, `PIN line repeats` for a second line naming
the same file).
`--verify-pin` given alone, with no rows directory, is a complete
invocation: `ok pin verified` and exit 0 on a clean vendored tree, so CI can
verify the pin before running the vendored `test.mjs` at all, not after.

`--write-pin` is a form of its own, exactly `--write-pin <sha>`: given any
other argument or flag beside it -- a rows directory, `--json`, a second
`--write-pin`, a typo -- it exits 2 with a usage line and writes nothing.
It also exits 2 -- leaving a pre-existing `PIN` byte-unchanged and no
temporary file behind -- when: the sha argument is not a 40-hex commit sha;
the generated content fails its own shape guard (first line not the pack
name and the sha, or the line count not equal to the hashed file count
plus one); the pack directory cannot be listed to look for a leftover temp
file; a `.PIN.tmp.*` file is already present beside `check.mjs` -- a prior
run's leftover -- refused outright rather than raced with this run's own
temp file; or the write or the rename itself fails, in which case the temp
file this run created is removed before returning. If that removal fails
as well, the temp file stays and would refuse every later run as a
leftover, so the message names it for removal by hand. Measured on Windows:
`node check.mjs --write-pin <sha> > PIN` makes the shell's redirect target
the rename destination, so the rename throws `EPERM`; file operations run
through an injectable fs-like object so the same refusals are forced and
asserted on any platform, not only where the fault reproduces for real.
What no program can do is protect `PIN` from that redirect: the shell
truncates the file before `check.mjs` starts, so it ends empty, and
`--verify-pin` refuses it. Never redirect `--write-pin`'s output.
None of these are verdicts on a row, so none are part of the vocabulary
above.

`--expect <file>` exits 2 as well, never 1: when `--expect` has no value
(or its value is itself a flag), is given twice, or is given with no rows
directory -- a usage line to stderr; and when the expect file cannot be
read, is not JSON, or is malformed -- `unevaluable: expect file <file>`
followed by what is wrong with it, one line per problem. Malformed means:
not an object with exactly the key `cells`; `cells` not an array; a cell
that is not an object with exactly `surface` (matching the row schema's
surface pattern), `lane` (an integer from 1) and `twins` (an array of
non-empty strings, none repeated); two cells with the same surface and
lane; the same key twice in one JSON object, at either level (`unevaluable:
expect file <file>: duplicate key "<key>"`) -- a JSON parser keeps the last
of two equal keys without a word, so the first authored value would be
dropped silently, the very loss the expect file exists to refuse. A cell
carrying `status`, or any other key, is malformed: an expected set never
asserts a status.

## Not refusals: crediting

Crediting is derived from a set that already conforms, and never changes
the exit code. A (surface, lane) group whose live row reports a check that
no twin RED as planted sets nonzero derives PARTIAL, never CLAIMABLE, and
names those checks: `--json` adds `"unfalsified": [<check keys>]` to the
group's element (sorted), and the text line adds `; unfalsified: <keys>`
[credit_live_checks_falsified]. A live row with no twin at all lists every
one of its checks. A group any of whose rows is UNEVALUABLE -- its live row
included -- derives UNEVALUABLE, never PARTIAL, and no crediting rule runs
on it: per-check crediting speaks about checks a GREEN live reported.
`test.mjs --mutate` disables each crediting rule in turn and requires a
PASS fixture's `expect_credit` to notice.
