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
- `schema: unknown field` — additional properties are refused; the pre-DATUM
  `parallax_sha` / `parallax_worktree` keys land here
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

## Not refusals: exit 2, unevaluable

These are not in the vocabulary because they are not verdicts on a row.
`check.mjs` exits 2, never 0, when the rows directory is missing or empty,
when a file in it is not JSON, or when `--verify-pin` finds a vendored file
missing, unlisted or altered.
