# A vacuity guard keyed on the wrong denominator refuses real evidence

2026-09-01, during the Lane 1 build. A gate that fired wrong.

## What happened

`Emit` was hardened so a check with a missing or non-positive `evaluated`
denominator fails the row: a check reporting "0 violations" because it examined
0 things is a vacuous pass, and `evaluated` is the denominator a reader judges
the 0 by. Correct in intent.

The denominator was set to the size of the WANT set. `SetEquality(got, want)`
with a legitimately EMPTY want then produced `evaluated = 0`, and the guard
discarded the whole row:

    SetEquality([],    []) -> checks=0 evaluated=0  refused (correctly vacuous)
    SetEquality([ZZZ], []) -> checks=1 evaluated=0  REFUSED, though the check
                                                    CAUGHT an invented entry

The second line is P6's phantom twin exactly: `golden.json` declares no
unevaluable instruments, so the honest want is empty. The guard would have
silently discarded `positions_match_golden: 1` -- which a separate finding had
just established is the ONLY check in the entire build that catches an invented
position, since `leaf_diff` walks the golden's keys and is structurally blind
to a key the golden never had.

So a guard added to stop vacuous passes would have destroyed the one check that
proves its own necessity, and the row would have vanished rather than gone red.

## Why it fired wrong

The denominator answered "how big is the thing I looked FOR", when the honest
question is "how big is the universe I EXAMINED". Those coincide for simple
checks -- which is why the distinction was easy to miss -- and diverge exactly
when a set-equality check has an empty side, which is the case where catching an
invention matters most.

## The fix, and the shape of it

The denominator became the size of the UNION (`len(got | want)`), so empty-vs-
empty stays 0 and is still refused as genuinely vacuous, while empty-want vs
invented-got becomes 1 and is credited. A second helper,
`SetEqualityOverUniverse`, takes an explicit universe for checks whose real
assertion is "every position was examined for unevaluable-ness" rather than
"these two lists match". Two functions, not one function with changed behaviour:
a helper cannot know its caller's meaningful scope, and guessing produces a
denominator that looks principled and means nothing.

## What to carry forward

- **A guard that refuses is as dangerous as a guard that passes.** This one
  would have removed evidence, and the row's absence reads like a gate that was
  never wired -- quieter than a red.
- **Before adding a vacuity or non-emptiness guard, find the legitimately empty
  case.** If none is imagined, that is a reason to look harder, not a licence.
- The gate author who hit this was told NOT to work around it -- no fake
  denominator, no skipped check, no substituted list. It reported the harness as
  wrong and stopped. A plausible-looking local workaround would have hidden a
  harness defect behind a green gate, which is the failure this repo exists to
  catch.
