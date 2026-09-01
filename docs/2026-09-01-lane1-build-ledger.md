# MERIDIAN Lane 1 — build ledger

*2026-09-01. The working record of the Lane 1 build: every defect found, who
found it, what was measured, and what was decided. Kept because the six
claimability cells are only worth what the process behind them was worth, and
this is that process — including the parts where the plan, the harness, and the
controller were wrong.*

**Read this if you are auditing the claims.** `STATUS.md` says what is claimable;
this says what it cost to be able to say so. Findings are numbered C1–C29 with
the measurement that established each, plus controller process errors (P1–P3)
and design decisions (D1–D2) recorded against the same standard.

Some entries describe transient states that no longer exist — a red suite
mid-change, a stale review package, a count later corrected. They are kept
deliberately: a ledger that only records outcomes cannot show whether a
conclusion was earned.

---

# MERIDIAN Lane 1 — subagent-driven-development progress ledger

Plan: `docs/plans/2026-09-01-lane1-implementation-plan.md` (committed `fbc19b3`)
Branch: `lane1-build`, forked from `fbc19b3` on `main`.

## Standing deviations from the plan (controller decisions, 2026-09-01)

1. **No commits by any agent.** The plan ends each task with a `git commit`
   step; the operator's standing rule (hook-enforced) is that Claude never
   writes git history. Implementers build and test only; the commit commands
   are collected in this ledger for the operator to run. Consequence: review
   packages are built from the WORKING TREE (`.git/sdd/pkg.sh`), not from
   commit ranges, and BASE/HEAD shas are not meaningful per task.
2. **Declared parallel lanes are used.** The plan's dependency lanes are
   honored ({T3,T9} alongside T2; T11-T15 after T10); two agents never touch
   the same file. This departs from the SDD skill's blanket "no parallel
   implementers" red flag, whose stated reason (file conflicts) does not
   apply to provably disjoint file sets.
3. Q7 (sequencing vs intent-workbench) was resolved by the operator invoking
   the build. No other deferred decision is treated as resolved.

## Task status

(one line appended per task on clean review)

- Task 1: dispatched (haiku)
- Task 9: dispatched (sonnet, parallel lane)

## Commit commands for the operator

(accumulated as tasks pass review; run from `cd ~/dev/meridian` on `lane1-build`)

## Controller decision D1 (2026-09-01, Task 1 review)

Reviewer found a REAL divergence: Go's encoder emits raw UTF-8 for non-ASCII
while the Python generator uses ensure_ascii=True (escapes to \uXXXX), so the
two languages hash non-ASCII strings differently. Reviewer proposed writing a
Python-compatible escaper in Go.

RESOLUTION (mine, not the reviewer's): refuse non-ASCII in canon.Marshal
instead. Reasons: (a) the plan's Global Constraints already restrict every
string in the system to ASCII, so the escaper would serve a domain the spec
excludes; (b) adding surrogate-pair escaping to the most load-bearing
primitive buys latent risk for no in-scope benefit; (c) fail-closed IS this
project's idiom -- it converts a documentary constraint into a mechanical one.
OPEN FOR HOSSAIN: if MERIDIAN should ever carry non-ASCII text (instrument
names, counterparties), the escaper is the right answer and this decision
flips. Recorded rather than silently settled.

## Task status detail

- **Task 3: COMPLETE** (2026-09-01). `internal/fold/money.go` + `money_test.go`.
  Review verdict Approved after 2 fix rounds. Verified by controller directly:
  gofmt clean, go vet clean, 11 half-even table cases + 5 panic-guard subtests
  all PASS. Uncommitted (operator writes history).
  Findings resolved: panic table expanded 1 -> 5 guard cases; the zero row was
  non-isolating ({10,5,0} also trips qtySold>totalQty) and is now {10,0,0};
  the reviewer's own suggested replacement {10,0,-1} was ALSO non-isolating
  (0 > -1) -- caught by the controller, confirmed by the reviewer.
  NOTED, not changed: reviewer observes the strictly-negative half of the
  `totalQty <= 0` clause can never independently gate anything (any
  non-negative qtySold already exceeds any negative totalQty), so the guard
  carries a logically redundant leg. Left as-is: `<= 0` reads as the intended
  invariant and costs nothing. Flagged for Hossain, not silently changed.

## Controller finding C1 (2026-09-01) -- PLAN DEFECT, mine

The T9 reviewer reported manifest k1=8/k3=4 vs true footprints 7/3. I
recomputed from the raw fixtures and found the cause, which is worse than a
wrong count:

`expected_view()` emits the key `seq`, but snapshot documents use `feed_seq`.
`leaf_diff`/`snapshot.Diff` therefore scores `.seq` as ABSENT on EVERY
comparison -- including the LIVE cells. Measured directly:
  live diff (expected/V3.json vs an honest snapshot doc) = 1   (must be 0)
  with seq -> feed_seq:                                    = 0
  corrected k1 = 7, corrected k3 = 3
Consequence had this shipped: P1's and P3's live cells could never be GREEN,
`Emit` would t.Fatal, and the build would have hard-stopped at Task 10 with a
failure whose cause sits two tasks upstream. P6 is unaffected (golden.json
already uses snapshot-schema keys).

Origin: the implementation plan itself (docs/plans/...), which specified the
expected-view schema as `{"cash","dividend_income","positions","realized_pnl",
"seq"}`. The generator implemented the brief faithfully. FIX: change the plan
AND the generator to `feed_seq`, regenerate fixtures, and let k1/k3 fall out
as measured -- never edit the manifest to match a wrong measurement.

## Controller finding C2 (2026-09-01) -- honest-limits item, not a bug

Verified directly against internal/feed: editing the payload of the LAST
record in a feed is NOT detected by Open(). Mid-feed garbling and mid-feed
deletion ARE both refused (`feed chain broken at seq 3: seq 3 where 2
expected`), but the tail record's own line hash is committed to by nothing --
record N's hash only appears in record N+1's `prev`, and there is no N+1.

This is inherent to a bare hash chain, not a defect in the implementation:
tail integrity comes from an EXTERNAL commitment, which this design does have
(snapshots are stamped with the feed-prefix hash, and P2's live cell compares
against the pinned `fixtures/base/snapshot.sha256`). The P2 tamper twin
targets a MIDDLE record (the split action), so the twin is unaffected.

ACTION: this must be stated in the repo rather than left implicit -- "the
chain detects any edit except to the final record; tail integrity comes from
the pinned snapshot hash." Belongs in feed.go's package comment and in
STATUS.md's honest-limits section at Task 18. Recorded so it cannot be lost.

## Controller process error P1 (2026-09-01) -- mine, no damage

I dispatched the same three canon round-3 findings TWICE: as a SendMessage
follow-up to the still-running fix-t1b AND as a fresh fix-t1c agent. Two
agents edited internal/canon/canon.go + canon_test.go concurrently -- exactly
the collision the SDD skill's red flag warns about. Their reported test counts
disagreed (19 vs 18) and fix-t1c believed canon.go was unchanged because
fix-t1b had already changed it.

Outcome checked directly, not assumed: 19 uniquely-named test functions, no
duplicated function names, both split error messages present exactly once,
gofmt/vet clean, 19/19 pass, and a controller-written probe confirms every
refusal class and the exact accepted-leaf byte output. The tree converged by
luck, not by design.

RULE for the rest of this build: a follow-up finding goes to the agent already
holding the file, or to a new agent -- never both. Check for an in-flight
agent on a path before dispatching to it.

## C1 corroboration (independent, 2026-09-01)

The T9 reviewer confirmed C1 from the opposite direction and pinned the branch
I could not: `feed_seq` is hardcoded in the ALREADY-BUILT task 5/6/7 briefs
(snapshot.Build, asof, reconcile), so moving the snapshot schema to `seq`
would be a retroactive change to completed work. Moving the golden is a
one-line change. The direction was not a judgment call; the other branch was
foreclosed.

Bonus the reviewer measured that I had not claimed: once the key names agree,
`feed_seq` is compared BY VALUE in every golden-vs-doc diff, so a document
folded to the WRONG VIEWPOINT now fails -- golden(V3) vs doc(V2) measures 11
mismatches where the mismatched name previously made that leaf invisible. The
reduced-golden design gains a viewpoint assertion for free.

Reviewer also withdrew its own proposed alternative ("write the twins through
expected_view") after measuring that p4/twin/snapshot.json needs six keys
expected_view does not carry -- the reduced-golden-vs-full-doc asymmetry is
intentional, which is why snapshot.Diff walks golden keys only.

- **Task 1: COMPLETE** (2026-09-01). go.mod, .gitattributes, .gitignore,
  internal/canon/. Approved after 3 fix rounds; final review found zero
  Critical/Important/Minor and independently re-ran gofmt/vet/tests (19/19)
  plus adversarial probes of the previously-broken class. Verified separately
  by the controller. Uncommitted.
  Journey: (1) Marshal silently diverged from the Python side on non-ASCII;
  (2) the first fix's walk was not total, so []string/map[string]string
  bypassed it entirely -- the doc comment promised a guarantee the code did
  not keep; (3) the rewrite made refusal STRUCTURAL via a type switch with a
  default branch, so an unhandled type cannot pass. Also split control-char
  from non-ASCII error messages, and pinned exact bytes for every accepted
  leaf type. See decision D1 and process error P1 above.

## Controller finding C3 (2026-09-01) -- internal/feed parse-skip, measured

The T2 implementer loosened Open() to skip ANY unparseable line at ANY
position (its justification was real: a torn tail followed by an append leaves
`garbage\n{record}\n`, so the torn line becomes a TERMINATED unparseable line
and a strict "unterminated final line only" rule rejects a legitimately
recovered file). The T2 reviewer flagged the loosening as unbounded. I probed
it directly:

  3 interior garbage lines inserted mid-feed  -> ACCEPTED, Len()=4   <- the defect
  tail truncated to 3 of 5 records + garbage  -> ACCEPTED, Len()=3
  tail truncated to 3 of 5, no garbage        -> ACCEPTED, Len()=3
  mid-feed line garbled to invalid JSON       -> REFUSED (seq leg)
  mid-feed record deleted                     -> REFUSED (seq leg)
  last record's payload edited                -> ACCEPTED (see C2)

Reading: interior junk cannot hide a deleted or altered RECORD (the seq leg
catches those), but it does mean the file may carry unattested content between
records -- "the feed is exactly this chain" is not enforced. Plain truncation
is NOT caused by the loosening: truncating a hash chain always yields a valid
shorter chain, so it is the same C2 class and needs the external commitment
(the pinned snapshot hash), which this design has.

Candidate fix: bound tolerance to at most ONE unparseable line total. That
closes the interior-junk hole while preserving the legitimate torn-tail
recovery, and record integrity stays backed by the seq and prev legs.
Reviewer asked for its view before I dispatch.

## Controller note (2026-09-01): disputed edit ordering -- UNRESOLVED, recorded both ways

I reported six round-2 items missing; fix-t9 says its own session history shows
five of them were already written before my check ran, and that its first
action on my message was a read-only grep showing them present.

My evidence: plain deterministic reads printing `positions_at present: False`,
`p6 ids unique: False`, and no `file=sys.stderr` in the file.
Its evidence: its own tool-call history plus a clean generator run in the turn
after my A-G messages.

I initially wrote here that its theory "does not hold" and that the gap was
real. I am withdrawing that: I cited its round-1 message as an admission, but
that message scoped out DIFFERENT items (P3 viewpoint literals, P6 twin_price),
not A-G, so it does not support the claim I made. Neither of us can see the
other's clock, and I cannot prove my read preceded its write. UNRESOLVED, and
recorded as such rather than settled in my own favour.

What is NOT in dispute, because I verified it directly after the fact:
`unevaluable_at` was genuinely the wrong shape and is now nested correctly; the
generator is at hash 79949655, exits 0, the determinism harness is green, and
three negative controls I ran myself show the guards firing with visible
reasons. The artifact is correct regardless of whose account of the ordering is
right, which is why I am not spending more of the build on it.


## Controller finding C4 (2026-09-01) -- MY dispatched fix was wrong, caught by review

I dispatched a three-part fix for the internal/feed parse-skip: needNewline on
write error, an EOF guard, and a hard BOUND OF ONE tolerated unparseable line.
The reviewer crash-tested it and the bound is a false-refusal that permanently
bricks the ledger after two ordinary crashes -- no attacker:

  the recovery append writes one buffer "\n" + record + "\n"; ANY non-empty
  partial write of it begins with the leading newline, terminating the existing
  residue and starting a second one. Two crashes => two unparseable lines =>
  bound-of-1 refuses => three successive opens all REFUSED, and there is no
  in-band repair because repair would mean rewriting bytes.

A restart loop or an OOM killer on the same append path produces it. I traded a
silent-loss bug for an unrecoverable-file bug, which for a ledger is worse.

CORRECTED RULE (dispatched): keep the EOF guard (an unparseable line is
tolerated only if followed by a valid record, or is the unterminated final
line); no cap; COUNT the residue and expose it. Record integrity does not
depend on a cap -- the seq and prev legs already refuse any deleted, altered or
reordered record, which I verified independently.

Lesson: I specified a rule from reasoning and did not crash-test it. The
reviewer's I4 is the real indictment -- the parse-failure policy has now been
changed three times (implementer, me, reviewer) with ZERO regression coverage
at any step. The crash-loop file is now a required test.

## Controller finding C5 (2026-09-01) -- the freshness harness proves the wrong thing

Recorded from the T9 review because it is a standing property of this repo's
design, not a bug to fix:

`fixtures/generate_test.sh` proves FIXTURES MATCH GENERATOR. It does NOT prove
PLANTS ARE STRONG. Every twin footprint lands in manifest.json bytes, so it is
tempting to think a weakened plant would break freshness. It would not:
whoever weakens a plant regenerates the fixtures and commits both, the
regenerated tree matches the new generator, and the harness goes green over a
quietly weaker twin.

The in-generator guards are therefore the ONLY defense against a hollow twin,
which is why a FLOORED guard (`if x == 0: die`) versus a PINNED one
(`if x != 3: die`) is a live distinction and not a stylistic one. Same reason
every guard now requires a negative control: a guard nobody has watched fire
is not known to work, which is this repo's own thesis applied to its ground
truth.

## Controller process error P2 (2026-09-01) -- dropped review item

I dispatched review items a-d in one batch and f-h in another, and item (e)
fell in the numbering gap and was never sent to the fixer -- despite being a
blocker in the reviewer's assessment. I also reused the label (h) for two
different findings. The reviewer caught both. Dispatched now as (e), with
(k) replacing my duplicate (h).

## Design decision D2 (2026-09-01) -- chain-covered residue: BETTER, deferred

The T2 reviewer built and measured a third feed-chain contract and I am
recording it as the INTENDED END-STATE while shipping the current one.

Proposal: redefine `prev` as sha256 of EVERYTHING SINCE THE LAST RECORD --
each residue line newline-terminated, followed by the previous record's line
bytes -- instead of just the previous record's line.

Measured properties (reviewer implemented it and ran the full matrix):
  - junk injection refused by the chain itself, no bound needed:
    `feed chain broken at seq 1: prev hash does not match previous line`
  - the crash loop RECOVERS (M1 opens clean, three successive open+append
    attempts all succeed) -- so finding C4 above cannot recur by construction
  - closes the LAZY TAIL FORGERY, which no other variant closed: not the
    shipped rule, not my bounded rule, not even the brief's strict rule
  - backward compatible: with no residue the definition collapses to sha256 of
    the previous record's line -- today's rule exactly. Migration, not rewrite.
  - all 5 brief tests pass, all 3 torn-tail orderings recover, vet clean.

NOT taken now, deliberately: `prev` is a CROSS-LANGUAGE contract. The Python
generator implements the same chain rule, and every fixture, footprint and
pinned hash derives from it; switching means changing two independent
implementations in lockstep and regenerating everything, while that generator
is mid-hardening with ~12 guards in flight. Lockstep coordination has already
produced two errors in this build today. And the harm it closes is narrow:
unattested bytes cannot corrupt what the ledger COMPUTES (the seq and prev legs
refuse every record-level change); what they cost is self-description to
readers other than Open().

FOR HOSSAIN: this is a real design improvement with measurements behind it, and
the backward-compatibility property makes it adoptable later without a format
break. Worth a decision after Lane 1 is green.

### D2 migration matrix (measured by the reviewer, both builds cross-read)

  clean feed, written by either build      -> BOTH open, len=3, identical head
                                              hash, and the two files are
                                              BYTE-IDENTICAL on disk (cmp)
  residue feed recovered under OLD rule    -> old: opens len=2 | new: REFUSED
  residue feed recovered under NEW rule    -> old: REFUSED     | new: opens len=2

So: any feed that has never recovered from a torn tail migrates for free --
existing files, fixtures, snapshot.sha256 pins and the Python generator's
output all stay valid under either implementation. The boundary is EXACTLY the
residue, and it breaks BACKWARD too: a feed that tears and recovers under the
new rule cannot be read by the old one, so rollback after any recovery is
unsafe.

Cutover rule that falls out: migrate a feed only when its residue count is 0.
The count we just dispatched is therefore doing double duty -- self-description
for non-Open readers, AND the mechanical migration-readiness gate. Nonzero must
be resolved first, and since bytes are never rewritten, resolving means rolling
to a fresh feed file at a pinned snapshot boundary. The dangerous window is a
crash DURING rollout (tear under old code, recover under new, or the reverse),
so per-feed cutover must be atomic; a time-boxed "accept either prev while
residue is non-empty" mode would restore two-way readability at the cost of
suspending injection detection for exactly those feeds.

Provenance caveat the reviewer flagged: its v3 measurements are bound-of-1 +
EOF guard + needNewline with the bound swapped for chain-covered residue. They
do NOT include the sticky poisoning or blank-lines-as-residue. Poisoning
changes one result -- its L1 probe fails under v3 only because it retries on the
same handle, which poisoning now forbids; after a reopen the torn-tail
orderings all pass. Anyone re-running this must apply poisoning first.

Deferred follow-ons if D2 is adopted: a residue-count report runnable over a
live feed directory (the cutover gate), and exposing residue OFFSETS as well as
the count (diagnostic -- tells you whether residue sits where a crash would
have put it).

## Controller verification of internal/feed after fix round 2 (2026-09-01)

Ran my own six-case matrix against the shipped code (the parse rule has now
changed three times, so I re-derived every case rather than trusting any
report):

  crash-loop: 2 garbage lines then valid record   -> OPENS, len=2   (C4 closed)
  mid-feed record DELETED                          -> REFUSED
  mid-feed record ALTERED                          -> REFUSED
  records REORDERED                                -> REFUSED
  terminated garbage at EOF                        -> REFUSED
  last record's "seq":3 retyped to "3"             -> REFUSED

So the durability defect and my own bricking regression are both closed, and
record-level integrity is intact under the unbounded-but-EOF-guarded rule.
Still true and documented, not a regression: a payload edit to the LAST record
and a plain truncation are both accepted (C2) -- inherent to a bare hash chain,
covered by the external pinned snapshot hash, and now stated in the package's
honest-limits comment.

impl-t2 also simplified the read rule: only the LAST unparseable line needs the
followed-by-a-valid-record check, since records only move forward, so a valid
record after the last garbage line is necessarily after every earlier one.

## Controller finding C6 (2026-09-01) -- P1 collision hole in the fold, measured

`PayloadHash` returns "" when canon.Marshal errors, so two DIFFERENT payloads
that both fail to canonicalize hash equal and the second is recorded as an
ABSORBED DUPLICATE instead of a collision refusal -- a direct hole in P1, the
at-most-once property. Measured against the shipped fold:

  PayloadHash(a)="" PayloadHash(b)="" equal=true
  FillKey(a)="94ae3f1d..." err=<nil>      <- key still resolves, so they collide
  absorbed=1 refusals=[] positions=map[AAÉ:{10 1000}]

with a and b differing in instrument, price AND quantity. FillKey succeeds
because it marshals only {trade_id, venue}; the payload hash is what fails.

Not reachable through feed.Append (which canon-marshals and would reject the
record) but reachable from a hand-written feed file, which Open parses without
re-marshalling. Fixed fail-closed: an uncanonicalizable payload becomes a
`malformed` refusal, because a fold that cannot compute the hash cannot tell a
duplicate from a collision and must refuse rather than guess.

Also dispatched: Fold's doc says "Seq <= upTo" but it slices by INDEX, so
Fold([]Record{{Seq:10},{Seq:20}}, 1) folds the seq-10 record. Benign today
because feed.Open guarantees Seq == index+1; the implementation is being moved
to select by seq so the contract holds for any caller.

NOT yet reproduced: the reviewer reports a panic reachable from a feed Append
itself can write. Five candidate sequences I constructed (sell-all after split,
stepwise sells to zero, zero-rate dividend, unit cost, split then remainder
sells) all completed cleanly. Awaiting the reviewer's exact repro; no fix
dispatched for it, because dispatching a fix for a defect I cannot reproduce is
how a real invariant gets papered over.

## Generator guard controls verified BY THE CONTROLLER (2026-09-01)

Five negative controls I wrote and ran myself against fixtures/generate.py,
each with stdout discarded exactly as generate_test.sh does it. A guard nobody
has watched fire is not known to work -- that is this repo's own thesis, and
these are the guards protecting its ground truth:

  neutered P1 twin (nodedupe -> honest)
    -> FAIL P1 twin has no footprint
  zeroed P5 drift (delta -> 0)
    -> FAIL P5 drift twin must change exactly one field, got 0: []
  P2 mutation forced back onto the duplicate's source event (mi = 0)
    -> FAIL P2 mutated twin corrupted the planted duplicate/collision
       structure at ['$.absorbed', '$.refusals']
  P6 fill twin footprint degraded (qty 51 -> 50)
    -> FAIL P6 fill twin footprint must be exactly 7, got 0: []
  written expected/V1.json perturbed by one minor unit
    -> FAIL viewpoint_V1 must be zero: honest fold disagrees with its own
       written expected/V1.json at ['$.cash']

All exit 1 with the reason on stderr. The last two are the guards added in the
final round -- the k6a pin (previously a >0 floor) and the disk-recompute that
replaced a tautological same-call check -- so both of the newest guards are
confirmed to discriminate, not merely to read a number.

Final manifest: k1=7, k3=3, k6a=7 (pinned), viewpoint_V1/V3=0 and
three_histories=0 (all measured from written artifacts, no longer literals),
p4.silent_zero=2, p4.stale_carry_forward=3, p5.field_mismatch=1,
positions_at 5 instruments x 3 viewpoints, unevaluable_at CCC at all three.

Standing limit, disclosed by the fixer and worth keeping visible: every
recompute check crosses a real disk boundary but still calls the same
naive_fold, so nothing INSIDE this file cross-checks the fold LOGIC. That is by
design -- the independent check on the fold is the Go implementation, which is
what P5 exists to measure -- but the disk round-trip must not be read as more
than it is.

## Controller finding C7 (2026-09-01) -- D1 made panics reachable FROM DATA

Cross-cutting consequence of my own decision D1 (canon.Marshal refuses
non-printable-ASCII strings and non-whitelisted types). Before D1, marshalling a
document built from primitives could not fail, so `panic(err)` on a marshal
error was genuinely unreachable. After D1 it is reachable from FEED DATA.
Measured by me against the shipped code:

  Build PANICKED on data-derived instrument name:
    canon: non-ASCII rune 'É' in string "AAÉ" at .positions.AAÉ (key)
  Build PANICKED on data-derived event id:
    canon: non-ASCII rune 'É' in string "ev-É" at .refusals[0].event_id

Path: feed.Open -> canon.Decode (which does NOT validate string content; only
canon.Marshal does) -> fold.getStr (no ASCII check) -> fold.State map keys and
refusal fields -> snapshot.Build -> canon.Marshal refuses -> panic. feed.Open
accepts any chain-consistent pre-existing file, not only ones this program's own
Append wrote -- and the snapshot package's own doc says it decodes
Python-emitted documents, so other producers exist.

This directly contradicts fold's documented invariant ("malformed input becomes
a refusal record, never an error") and turns a hostile or merely corrupt feed
into a crash instead of a refusal. Same root cause as C6.

THREE-LAYER FIX dispatched:
  ingress  - feed.Open enforces canonicality by re-marshalling each record and
             comparing to the stored bytes, so a non-canonical record is
             refused at open (already dispatched to impl-t2 as its item 4)
  fold     - refuses payloads that cannot be canonicalized, as a malformed
             refusal rather than a silent absorb (C6, dispatched)
  snapshot - Build must RETURN an error instead of panicking, so the contract
             stands on its own rather than resting on another package's
             validation -- which is precisely what I told impl-t4 not to do

PLAN CHANGE: the plan justified Build's panic as "doc is built from primitives
only". D1 invalidated that premise. The plan is corrected accordingly.

## Controller finding C8 (2026-09-01) -- int64 overflow breaks the fold's totality

The most serious defect found in this build. Reachable through the REAL API --
feed.Append writes it, feed.Open re-verifies the chain, Fold breaks. All four
shapes reproduced by me:

  buy qty=4e9 price=4e9, then sell 1 @ 1
    -> PANIC: fold: RelieveCost invariant violated
       (NOT the spec's one sanctioned panic -- it is RelieveCost's caller-bug
        guard in another file, reached because TotalCost went negative)
  two buys, each qty=1 price=MaxInt64 (each product individually IN RANGE)
    -> qty=2 totalCost=-2  refusals=0
  buy 4e9 @ 1, then a price event of 4e9
    -> unrealized=-2446744077709551616  refusals=0  unevaluable=0
       (no panic, no refusal -- nonsense P&L reported as fact)
  buy 100 @ 10, then split ratio=MaxInt64
    -> qty=-100  refusals=0   (a NEGATIVE POSITION, which the spec forbids)

Note the second shape defeats any per-event product check: the guard must be on
the RUNNING TOTAL. And the valuation shape never touches RelieveCost, so a fix
aimed only at the money path leaves three of four live.

Dispatched fix: overflow-check every arithmetic site (fill product, TotalCost /
Cash / RealizedPnL accumulations, split ratio, dividend rate) and refuse as
`malformed`; for the valuation path -- where the fill has already applied and
cannot be retroactively refused -- mark the position `unevaluable` with a
distinct reason rather than publishing a wrong number, which is the same
fail-closed discipline P4 applies to a missing price. RelieveCost's guard stays
exactly as it is: it caught a genuine caller bug.

## Controller finding C9 (2026-09-01) -- residue bypass, UNCLOSEABLE, documented instead

Measured on the shipped feed:

  terminated garbage as the last line        -> REFUSED
  same file + one unterminated junk byte     -> ACCEPTED, len=2, residue=2

So corrupting the final record and appending a single character with no
trailing newline silently drops that record. Neither the reviewer nor I can
close it: the bypass shape (record, garbage-terminated, garbage-unterminated)
is BYTE-IDENTICAL to the legitimate crash-loop shape that the round-2 fix
exists to accept. Any rule refusing one refuses the other, and refusing the
crash-loop shape is exactly the bricking bug (C4) we already fixed.

Resolution: document it as an honest limit next to the tail-forgery and
truncation limits, since protection comes from the external pinned snapshot
hash rather than from the chain. This is a third independent argument for D2
(chain-covered residue), which closes it by construction because the record
after the residue commits to the bytes it stepped over.

## Controller finding C10 (2026-09-01) -- canonicality check misses a sorted extra key

The canonicality re-marshal check DID land and catches reordered/duplicated
keys. It does not catch an ADDED key that happens to sort into place:

  key-reordered last line   -> REFUSED (line is not canonical JSON)
  extra key on last line    -> ACCEPTED

`{"backdoor":"yes","effective":...}` is still canonical -- `backdoor` sorts
before `effective` -- so decode-and-re-marshal reproduces the stored bytes
including the extra key. A round-trip cannot see a key it faithfully preserves.
Dispatched: validate the top-level key SET is exactly
{effective,id,payload,prev,seq,type}. The current error message already claims
to catch "extra keys" and does not, which is itself the message-overstates-the-
measurement defect class this build keeps finding.

Also verified by me, contradicting a stale review claim: blank-lines-as-residue,
the distinct duplicate-seq message, the mid-feed gap test, the deep copy (not
breakable at four levels of nesting) and the canonicality check had ALL landed.

- **Task 6: COMPLETE** (2026-09-01). `internal/asof/`. Reviewer verdict
  Approved, zero Critical/Important beyond plan-mandated coverage gaps.
  Confirmed by the reviewer independently: pure recompute (no package-level
  state, no memoization, fresh `*Feed` per call), byte-identity test genuinely
  re-derives rather than comparing a value to itself, handle closed on every
  path including `feed.Open`'s own error returns, `seq` beyond the feed errors
  rather than clamping.
  KNOWN COVERAGE GAPS, all plan-mandated (the brief supplied the test file) and
  all carried into the final review's triage list rather than fixed now:
  `seq == 0` (empty prefix / genesis PrefixHash) untested; empty feed with
  `seq < 0` untested; `feed.Open` failure path (a *ChainError) untested; and
  once `snapshot.Build` gains its error return, that fourth data-derived error
  path is untested too, since the fixture only uses ASCII instrument names.

- **Task 4: fix rounds 1-2 verified** (collision hole closed by routing dedupe
  through canon.Marshal inline so a canonicalization failure becomes a
  `malformed` refusal BEFORE the record enters `seen`; prefix now selected by
  Seq rather than index). Overflow work (C8) dispatched separately and NOT yet
  applied -- `grep overflow internal/fold/fold.go` returns 0. Task 4 is NOT
  complete.

## C7 resolution verified (2026-09-01)

snapshot.Build now returns an error instead of panicking; verified by me:

  instrument -> snapshot: build: canon: non-ASCII rune 'É' in string "AAÉ"
                at .positions.AAÉ (key)
  event id   -> snapshot: build: canon: non-ASCII rune 'É' in string "ev-É"
                at .refusals[0].event_id
  happy path unchanged

The implementer withdrew its original unreachability claim in place, naming
what it had missed: it conflated Go's TYPE system with canon.Marshal's runtime
CONTENT check -- nothing structurally kept non-ASCII out of fold.State's
string fields.

Valuable finding from tracing the real path: feed.Open's canonicality
re-marshal check ALREADY refuses non-ASCII content with a *feed.ChainError
before Build is ever reached. So the ingress layer works, and Build's error is
defense in depth rather than the primary gate. Consequence, flagged honestly
rather than hidden: the asof.Read test I asked for cannot literally exercise
Build's error today -- it proves the black-box behaviour (no panic, error
returned) but currently trips feed.Open's gate. Build's own error path is
covered by direct unit tests instead, which is the right place for it.

- **Task 5: fix round 1 verified.** Awaiting re-review.

## Carry-forward for the gate tasks (T10-T15) -- MUST be in their dispatches

Changes made during T1-T9 that the gate briefs do not know about:

1. `snapshot.Build` now returns `(Doc, []byte, string, error)`. Gates reach it
   via `asof.Read`, which already returns an error, so most gate code is
   unaffected -- but any direct Build call must handle four returns.
2. Gates must assert KEY-SET equality, not just golden-leaf diffs.
   `snapshot.Diff` is one-sided BY DESIGN (a twin document legitimately carries
   keys a reduced golden never has), so an INVENTED position or a MISSING
   `unevaluable` entry scores 0 mismatches and passes. The manifest now
   publishes `positions_at.{V1,V2,V3}` and `unevaluable_at.{V1,V2,V3}` for
   exactly this; each gate must compare the document's actual key sets against
   them. A gate that only diffs golden leaves is incomplete. (Plan decision 11.)
3. The expected-view files use `feed_seq`, NOT `seq` (plan decision 10). This
   also means feed_seq is compared BY VALUE, so a document folded to the wrong
   viewpoint now fails loudly -- a free viewpoint assertion.
4. New refusal kinds / `unevaluable` reasons may come out of the overflow fix
   (C8). If so, the generator's expectations and the gate assertions must both
   be updated to match; check `fixtures/base/manifest.json` at dispatch time
   rather than trusting the brief.
5. Twin verdict rows carry a 17th key `planted`; live rows have 16. The
   crediting rule is `checks == planted.expected_violations` EXACTLY, and a
   twin that never goes red must fail the run.
6. `feed.Open` now enforces canonicality and refuses non-canonical records, so
   a gate feeding a hand-built feed file must produce canonical lines.

- **Task 9: COMPLETE** (2026-09-01). `fixtures/` — generator, naive fold, all
  planted twins, manifest. Approved after 5 fix rounds. Final hash e675c0b5.
  Journey: shipped with a schema split that inflated two published footprints,
  made the hollow-twin guards LITERALLY UNREACHABLE, and would have kept P1's
  and P3's live cells from ever going GREEN; a P2 twin that corrupted P1's
  planted structure; and seven expectation values that were unverified
  literals. Now: every published expectation measured, ~12 live guards, five of
  which I personally watched fire, and a determinism harness plus negative
  controls at each round.

## CARRY-FORWARD FOR STATUS.md WORDING (Task 18) -- honesty constraint

The T9 reviewer's closing point, and it changes how P5 may be claimed:

**The naive fold is a SAME-CONTRACT REIMPLEMENTATION, not an independent
oracle.** Both it and the Go fold were written from the same written contract,
so their agreement is CROSS-IMPLEMENTATION AGREEMENT, not independent
confirmation of correctness — a contract-level misunderstanding would be
reproduced identically on both sides and the gate would stay green.
`fixtures/p6/golden.json` (P6_GOLDEN) is the ONLY artifact that breaks that
circularity, because its twelve leaves were hand-derived and independently
recomputed by hand twice (by me and by the reviewer).

So P5's claim must be worded "reconciles against an independently-implemented
fold" and must NOT imply "independently verified correct". The import-pin makes
the independence STRUCTURAL (no shared code) but not EPISTEMIC (shared spec).
Writing it any stronger would be exactly the correct-shaped lie this repo
exists to catch.

Second, smaller process finding from the same reviewer: my first review package
for T9 was STALE relative to the working tree, and the review was valid only
because the code happened not to have moved in between. Packages must be
generated immediately before dispatch, not reused.

## C8 resolution verified (2026-09-01) -- all four overflow shapes closed

Re-ran my own four probes through feed.Append -> feed.Open -> Fold:

  1 buy-overflow then sell   positions=map[] refusals=2 cash=0      (no panic)
  2 accumulation             qty=1 cost=9223372036854775807 refusals=1
                             (first buy applied, second REFUSED -- no negative)
  3 valuation overflow       unrealized=0
                             unevaluable=[{AAA valuation_overflow}]
  4 split ratio=MaxInt64     qty=100 refusals=1   (split refused, no negative)

Fail-closed at every site. The valuation case is the one I care most about: it
publishes an `unevaluable` record instead of a plausible-looking negative,
which is the same discipline P4 applies to a missing price.

CONTRACT CHANGE (additive): new `Unevaluable.Reason` value
`"valuation_overflow"`. No new `Refusal.Kind` -- overflow refusals reuse
`malformed`. The fixtures contain no overflow, so the generator's published
expectations are unaffected; the gate tasks must not assume `unevaluable`
reasons are limited to `no_price_in_prefix`.

- **Task 5: COMPLETE** (2026-09-01). `internal/snapshot/`. Approved after 1 fix
  round. The Critical was mine-adjacent: the implementer claimed Build's panic
  was unreachable "given fold.State's field types", conflating Go's TYPE system
  with canon.Marshal's runtime CONTENT check. Build now returns an error
  (wrapped with %w so errors.Is/As reach the canon error), asof propagates, and
  the doc comment no longer claims what it cannot deliver.
  The reviewer hand-traced both error strings against canon's own path-building
  rather than trusting the ones I quoted at it, and read the grown feed.go
  directly to verify the ingress gate instead of accepting my summary.
  OPEN, non-blocking coverage gaps carried to the final review's triage list:
  Decode's non-object-root error path, Write's I/O-failure path, and an
  automated test distinguishing `valuation: null` from an ABSENT key (the logic
  is correct on re-trace, just unpinned).

- **Task 7: built, review in flight.** `internal/reconcile/`. compared==5 and
  the 5-mismatch drift case hand-derived and matching the brief with no
  expectation adjusted.

## Feed final matrix verified by controller (2026-09-01, after fix round 4)

  extra key at sorted position, LAST line     -> REFUSED  (C10 closed)
  extra key at sorted position, MIDDLE line   -> REFUSED
  crash-loop: 2 residue lines + valid record  -> OPENS len=2 residue=2 (C4 stays closed)
  mid-feed record altered                     -> REFUSED
  clean feed                                  -> OPENS len=3 residue=0

The implementer distinguished the two failure classes properly: an EXTRA key is
deliberate tampering and hard-refuses unconditionally, while a missing or
mistyped field stays residue-tolerable because a torn write can produce it.
Same split applied to `prev`: length stays residue-tolerable (a torn write can
truncate it), character class is now unconditional (a torn write cannot produce
a complete record with a right-length wrong-charset value). That is a sharper
distinction than I asked for.

It also caught, on its own self-review, that adding the key-set check made the
canonicality error message AND an existing test's doc comment both stale --
each claiming to catch something the new check now intercepts first -- and
added a canonicality-only test (same six keys, reordered) so each check has a
test that isolates it. Payload keys deliberately NOT constrained, with the
reasoning stated in the doc: payload shape is event-type-specific and belongs
to the fold layer, not the durable-log layer.

## Controller finding C11 (2026-09-01) -- Reconcile FABRICATES mismatches

Measured against the shipped reconcile:

  document:  {"positions":{"AAA":{"qty":10}},"refusals":[]}   (no cash, no total_cost)
  statement: cash=-403, AAA quantity=10 cost_basis=1000
  ->  compared=3 mismatches=2
        instrument=""    field=cash       ledger=0 custodian=-403 delta=403
        instrument="AAA" field=cost_basis ledger=0 custodian=1000 delta=-1000

Neither mismatch is real. `cash, _ := asInt(doc["cash"])` discards the error and
yields 0; same for qty/total_cost via `pm, _ := p.(map[string]any)`. A schema or
decode failure is reported as a CUSTODIAN DISCREPANCY, complete with instrument,
field and signed amount -- the exact output this package exists to make
trustworthy.

Worse than a missed failure: for a property literally named "reconciliation
proven able to fail", a FABRICATED failure would credit the twin cell for the
wrong reason. Reachable, not theoretical -- snapshot.Decode accepts any
document in the schema including Python-emitted twins, and the P4 twin IS one.

Note the asymmetry the reviewer caught: LoadStatement (the custodian side) has
seven fail-closed branches, each a real informative error. Reconcile (the
ledger side) had none. Same package, opposite disciplines.

Dispatched: Reconcile returns an error, naming the offending instrument/field.
Signature becomes ([]Mismatch, int, error).

## Controller finding C12 (2026-09-01) -- CLI read commands accept AND CREATE a missing feed

  meridian replay --feed /tmp/does-not-exist.jsonl
    -> ok records=0 prefix_hash=sha256:000...000   exit=0
  meridian asof   --feed /tmp/does-not-exist-2.jsonl
    -> {"absorbed":[],"cash":0,...,"feed_seq":0,...}   exit=0
  and BOTH created the file on disk.

A mistyped path reports a clean chain over an empty ledger, prints a genesis
prefix hash, emits a well-formed zero-valued snapshot, and litters an empty
file. The advertised demo is "clone, replay, hash-compare, run a twin, watch it
go red" -- under a typo, replay says `ok`. A green result standing in for no
result is precisely the failure mode this project exists to prevent.

Root cause is layering, not feed: feed.Open's create-on-absent is CORRECT for
its own contract (append needs it); the CLI wrongly applied a write-path
constructor to read-only commands. Fixed at the CLI: replay/asof/snapshot/
reconcile stat first and exit 1; append keeps create-on-absent, pinned by its
own test so the distinction stays deliberate.

- **Task 8: built, fix round 1 dispatched.** The brief had a genuine compile
  collision (`func run` declared in both main.go and main_test.go); renamed to
  runCLI. asof stdout confirmed byte-identical to the written snapshot file by
  the implementer via cmp and md5sum, independently of the pinned test.

## Controller process note P3 (2026-09-01) -- two agents in cmd/meridian, benign

I told impl-t7 to update the reconcile call site in cmd/meridian/main.go "if it
exists", while impl-t8 owned that file and was concurrently adding
requireFeedExists. Both edits landed and survived; impl-t7 re-verified its own
edit and left the other agent's in-progress wiring untouched rather than
reformatting around it. Verified by me: builds, vets, cmd/meridian tests green.

Benign this time, and it is the second instance of my own one-agent-per-file
rule slipping (P1 was the first). The correct instruction would have been to
report the needed call-site change to me and let the file's owner apply it.

- **Task 7: fix round 1 verified, re-review in flight.** Reconcile fails closed
  on all four malformed doc shapes I probed; the well-formed case is unchanged
  at compared=3.

## Controller findings C13-C15 (2026-09-01) -- feed, two carried over + one I caused

C13. `Append`'s returned Record ALIASES the Feed's stored payload:
       feed reported 1002 before, 999999 after mutating the RETURNED record
     The ingress deep-copy exists, but the copy is shared between what Append
     returns and what the Feed keeps, so a caller mutating r.Payload silently
     changes what the Feed reports while disk holds the original. The existing
     copy test passes only because it mutates the CALLER's map, never the
     returned one.

C14. A non-EOF read error opens a SILENTLY TRUNCATED feed. The three
     `!complete` breaks exit before the `rerr` check, and bufio.ReadBytes
     returns partial data TOGETHER with a non-EOF error (reviewer measured:
     `data="{\"partial\":tru" err=simulated EIO isEOF=false`). A real I/O error
     mid-file whose partial bytes fail to parse is treated as a torn tail and
     Open returns SUCCESS with fewer records. The code comment justifying this
     asserts a false premise about ReadBytes.

C15. REGRESSION FROM MY OWN INSTRUCTION. Counting blank lines as residue (which
     I asked for) made one stray trailing newline brick the feed permanently:
       2-record feed, untouched          -> opens len=2 residue=0
       same feed + one extra "\n"        -> chain broken at seq 3
     Any editor or tool that "ensures a final newline" now bricks the file, and
     the never-rewrite rule leaves no repair path -- the same class as the
     bound-of-one bricking bug (C4), reintroduced by a change I requested.
     Fix: a blank line increments the residue COUNT but does not reset the
     last-garbage-terminated state, since a blank line carries no content and
     therefore cannot be a corrupted record.

PROCESS: C13 and C14 were reported to me as N2/N3 in the TRUNCATED tail of an
earlier review, and I never chased that tail before dispatching the next round.
Two real defects sat undispatched for several rounds because I acted on the
part of a review I could see. Chase every truncation before dispatching.

## Controller finding C16 (2026-09-01) -- last unguarded arithmetic site

The T4 reviewer enumerated all 26 arithmetic sites (21 in fold.go, 5 in
money.go) and found exactly one unguarded. I confirmed it:

  State.UnrealizedPnL()'s `t += v.Unrealized` accumulation
  sum of two MaxInt64 unrealized values = -2

Each per-position Unrealized is guarded at its own computation site, but the
SUM across positions is not -- the same running-total-versus-per-item
distinction that let shape two of C8 through a per-event check. It matters
because snapshot.Build publishes the result directly, so a silent wrap becomes
a published lie. Dispatched; the implementer chooses the shape (error channel
vs marking positions unevaluable so the aggregate cannot overflow by
construction) and must justify it.

Also verified by that reviewer, not taken on report: it reconstructed the
PRE-FIX fold.go and ran the four new tests against it, each failing for its own
stated reason rather than by compile error. The strongest is the two-buys-of-
1@MaxInt64 case, which can only pass if the check is on the running total.

- **Task 7: APPROVED** (2026-09-01). One non-blocking coverage gap dispatched
  (positions-key-absent branch is code-correct but unpinned). The reviewer
  verified by reconstructing the pre-fix body and running the new tests against
  it -- all three failed red, reproducing the exact fabricated mismatches.

- **Task 8: fix round 1 verified by implementer.** All four read commands now
  exit 1 with empty stdout, a path-naming stderr message, and no file created;
  append still creates. RED was produced by neutralizing the guard, and asof
  turned out to fail differently pre-fix (seq out of range, still creating the
  file), so the guard was not redundant there either.

- **Task 7: COMPLETE** (2026-09-01). `internal/reconcile/`. Approved, 5 tests.
  The Critical was that Reconcile FABRICATED mismatches -- a missing field
  became `Ledger: 0` and was reported as a confidently signed, instrument-named
  custodian discrepancy. For a property literally called "reconciliation proven
  able to fail", a fabricated failure would have credited the twin cell for the
  wrong reason. Now every ledger-side read fails closed with a named wrapped
  error, and a refusal carries compared=0 rather than a partial count a caller
  could mistake for a judgment.

## Task 9 round 6 (2026-09-01) -- twin key-set expectations, verified

Reopened briefly because the gate side surfaced that twins were not checked for
key-set divergence at all -- the same confinement gap closed for P2 earlier.
Manifest now publishes measured values (confirmed by me):

  p1 twin        positions 0  unevaluable 0
  p3 twin        positions 0  unevaluable 0
  p4 twin        positions 0  unevaluable 1   <- the planted defect, second way
  p6 twin_fill   positions 0  unevaluable 0
  p6 twin_price  positions 0  unevaluable 0
  p5, p2         excluded, with reasons

P4's 1 is the plant (CCC removed from unevaluable) caught independently of the
footprint counts, and the generator now die()s if it ever measures 0 -- a 0
would mean the plant stopped diverging from the manifest, which is the
hollow-twin failure in another mask.

Exclusions are reasoned, not convenient: P5's twin is a custodian STATEMENT
with no positions map and no unevaluable concept (publishing 0 would assert
"checked and matched" where the truth is "not applicable"); P2's twins are raw
re-chained FEEDS, not documents, and its confinement is already covered.

Definition: `len(set(document) ^ set(published))` -- SYMMETRIC difference, via a
single sym_diff_count() helper. A one-sided comparison here would recreate the
exact blind spot the checks exist to close.

NEW GAP the fixer found unprompted, carried to the gate author: measuring P6's
twins against the honest P6 fold's own sets also closes, for P6, the same
one-sided blind spot -- because P6_GOLDEN never declared an `unevaluable` key at
all, so nothing would have caught an invented one. That is in the single
artifact whose authority comes from being hand-derived.

## P1 fully credited (2026-09-01) -- first property with both cells earned

  meridian-lane1-p1 live GREEN 16 keys
    checks {collision_refused 0, duplicate_absorbed 0, position_after_dedupe 0,
            positions_match_manifest 0, unevaluable_match_manifest 0}
  meridian-lane1-p1 twin RED 17 keys
    checks   {collision_refused 1, duplicate_absorbed 1, position_after_dedupe 7,
              positions_match_manifest 0, unevaluable_match_manifest 0}
    expected {identical}

Twin checks == planted.expected_violations exactly. The live cell went GREEN on
its FIRST run, which is the payoff from the feed_seq schema fix: had that
defect shipped, this cell could never have gone green and the failure would
have surfaced two tasks downstream of its cause.

The gate author also tightened SetEquality from a sorted-multiset merge to true
map-based set semantics, so it matches the generator's len(set ^ set)
definition exactly rather than relying on an input property (no duplicate
instrument names) that nothing enforces.

## Source-of-truth guidance for the remaining gates (from the T10 author)

  P1/P3/P4  compare against manifest positions_at / unevaluable_at, KEYED BY
            VIEWPOINT (P3 needs V1/V2/V3, not just V3)
  P6        has its OWN feed and its own golden at its own end_seq (14) -- must
            compare against ITS OWN golden document's sets, NOT positions_at /
            unevaluable_at, which are scoped to fixtures/base and would
            silently compare against the wrong feed
  P5        no check: statement document, no positions or unevaluable concept
  P2        no check: raw feed twins, not snapshot documents

## Controller finding C17 (2026-09-01) -- CRITICAL: Emit can credit a dishonest cell

The T10 reviewer lifted Emit's enforcement block into an isolated module and
ran ten constructed rows. Emit CORRECTLY refuses: an all-zero twin, an
all-zero-expectations twin, a twin check missing from expected_violations, and
a twin disagreeing with a NON-ZERO expectation. Two holes remain:

  H1  a live row with an EMPTY or nil `checks` map is credited GREEN.
      `result` starts "GREEN" and only a non-zero entry flips it, so an empty
      map cannot flip it. A gate that examines NOTHING emits a GREEN row.
  H2  an expectation of value 0 whose check DOES NOT EXIST is silently
      accepted -- the forward loop reads Checks[k], which returns Go's zero
      value for an absent key, so 0 == want matches a check never computed.

H2 is live right now: the generator publishes eight zero-valued expectations
(positions_match_manifest / unevaluable_match_manifest on p1, p3, p6 twin_fill,
p6 twin_price) and Emit can enforce NONE of them. Only P4's value of 1 defends
itself, and only by accident of being non-zero. The structure cannot distinguish
a wired zero-check from an unwired one -- so wiring P1's twin checks and
forgetting P6's would pass silently.

DIVERGENCE the reviewer also caught: Task 17's claimability.py compares
`checks == planted.expected_violations` as full DICT equality, which is
symmetric and WOULD catch all eight. So the real enforcement currently lives in
a script that runs later, and a suite green locally could be refused at claim
time with no earlier signal. Two gates disagreeing about the crediting rule is
worse than either being wrong alone.

Neither hole is backstopped: claimability.py's claimable() inspects only
live[0]["result"], never its checks, so an EMPTY live row reaches CLAIMABLE and
then STATUS.md.

Dispatched: fail an empty live row; compare over the UNION of check and
expectation keys. Also asked whether a check with a missing or zero `evaluated`
entry should fail -- a check reporting 0 violations because it examined 0 rows
is a vacuous pass, and `evaluated` is the denominator a reader judges the zero by.

## Controller finding C18 (2026-09-01) -- append accepts empty required flags

  meridian append --feed f.jsonl --payload '{...}'   (no --type/--id/--effective)
  -> seq=1 hash=02c7ed2f...  exit=0
  -> {"effective":"","id":"","payload":{...},"prev":"000...","seq":1,"type":""}

A dropped flag silently writes a malformed record into an APPEND-ONLY log.
Worse than the read-side false success already fixed in this task: that one was
repeatable and harmless; this is permanent, since the record can never be
removed, only refused later. And it IS refused later -- asof reports
{"kind":"malformed","detail":"unknown event type "} -- so the mistake surfaces
through a DIFFERENT subcommand, at a different time, never as a non-zero exit
from the command that made it. Neither the CLI nor feed.Append enforces it.

## Controller finding C19 (2026-09-01) -- import-pin bypasses + overclaiming

Reviewer verified bypasses, all exiting 0 ("ok"):
  _e = exec ; _e("print(1)")
  f = getattr(__builtins__, "eval") ; f("1+1")
  imp = __import__ ; imp("os")
  x = "int" + "ernal/"        # real content "internal/"

The exec/eval class matters because those are BUILTINS -- no import needed, so
the allowlist forecloses nothing there. The check matched only a Call whose
func is the bare Name.

The import half survived: dotted imports, aliasing, relative imports,
conditional/nested/class-body imports all caught; f-strings and IMPLICIT
adjacent-literal concatenation caught for free (Python folds the latter into
one Constant at parse time).

Dispatched: flag any REFERENCE to exec/eval/__import__/compile anywhere plus
attribute access on __builtins__; constant-fold BinOp string addition before
the literal scan; make the checker itself fail CLOSED on a syntax error,
missing file, or non-ASCII target rather than dying with a traceback (a stack
trace is ambiguous between "refused" and "broken", and CI cannot tell them
apart); extend the single-injection self-test to more than one branch.

ALSO dispatched, and the part that matters more: state the THREAT MODEL in the
docstring. A static allowlist over a file we write ourselves cannot defend
against a determined author -- arbitrary Python obfuscates arbitrarily. What it
buys is protection against DRIFT and ACCIDENT: an edit that quietly adds a
dependency, a copied helper, a path string that creeps in. And it must not
imply epistemic independence, only structural -- same binding constraint as the
P5 wording.

## Controller finding C20 (2026-09-01) -- positions_match_manifest is UNCREDITED

The project's own thesis turned on its newest check. Measured values:
  p1 0/0, p3 0/0, p4 0/1, p6 twin_fill 0/0, p6 twin_price 0/0

`unevaluable_match_manifest` goes red exactly once (P4). `positions_match_manifest`
goes red NEVER. positions_at publishes the same five instruments at all three
viewpoints, and none of the five twin mutations changes the position KEY SET.

So by this repo's own crediting rule -- a gate that has never run red proves
nothing -- positions_match_manifest is an UNCREDITED check about to be counted
as evidence in five live rows. The exact hole it was added to close (an invented
or dropped position) is the one thing no twin shows it can see. The generator's
die()-on-zero guard protects P4's 1; it cannot protect a check that measures 0
everywhere by construction.

Dispatched: one twin, placed where the defect honestly falsifies that
property's claim, that drops or invents a single position -- with a measured
expectation, a die()-on-zero, and its own negative control.

NOTE ON PROVENANCE: the reviewer flagged that the five twin figures it reasoned
from were MINE, forwarded to it, not recomputed by it -- only the manifest facts
underneath were its own. I passed that caveat through unchanged and asked it to
verify the premise before acting. Forwarding my numbers into a review and
getting them back as a finding is a circularity worth naming.

## Further T10 findings dispatched (I3, I4, I5)

I4  P1's ledger-key assertion is SKIPPED when more than one record is absorbed:
    `if a := ...; len(a) == 1` makes the key comparison conditional on there
    being exactly one absorption. A fold emitting the correct duplicate PLUS a
    spurious one passes -- the id loop finds the planted id, the guard skips the
    key check, and `absorbed` is absent from the reduced golden so
    position_after_dedupe cannot see it either. An extra absorption IS an
    at-most-once violation, in the at-most-once property's own gate.
I3  SetEquality / PositionKeys / UnevaluableInstruments have no unit test and
    now execute 0/0 on both cells -- proving they work required a scratch module.
I5  json.MarshalIndent's error is discarded, so an unmarshalable Params value
    writes a ONE-BYTE verdict file: a corrupt row indistinguishable from a
    missing one, produced by a passing test.
I6  `evaluated` is enforced nowhere and is partly hardcoded -- a check with
    evaluated=0, and one with no evaluated entry at all, are both accepted.
    Asked for an explicit ruling: load-bearing and enforced, or documentation
    and labeled as such.

## C17 RESOLVED (2026-09-01) -- Emit now enforces the crediting rule

Verified by me against the fixed harness, all four constructed rows REFUSED:

  live with empty checks map                     refused=true
  live with nil maps                             refused=true
  twin, zero expectation for an UNWIRED check    refused=true
  twin with a zero evaluated denominator         refused=true

The implementer also ruled on `evaluated` explicitly rather than leaving it
incidental: any check present in Checks with a missing or <=0 Evaluated entry
now fails on BOTH cells -- deliberately stricter than a whole-map-empty guard,
because a gate with four real checks plus one silently-unwired one would slip
past the weaker form. It produced real RED first by restoring the pre-fix
emitWith: 3 of 4 new tests failed as expected, and it reported honestly that
the 4th passed even pre-fix because it was already caught indirectly.

The union comparison now matches claimability.py's dict equality, so the two
gates no longer disagree about the crediting rule.

## Controller finding C21 (2026-09-01) -- a correct rule resting on a false reason

feed.go's blank-line rule is right; its stated PREMISE is false. The comment
says a blank line "can never itself be the mangled remains of a corrupted
record". Blanking a record's line is a two-keystroke edit, and I measured:

  2-record feed, last record's line replaced with an empty line
  -> ACCEPTED len=1 residue=1     (record 2 silently gone)

The conclusion survives -- a blank line carries no content to CONCEAL anything
in, and refusing it would re-brick the crash-loop case (C4) -- but the
justification does not. Dispatched a comment fix plus an honest-limits note:
a tail record replaced by a blank line is dropped silently, the same
tail-integrity boundary reached by a different route.

Worth recording as a class: a correct rule resting on a false reason is one
edit away from being changed for the wrong reason. This is the same defect
family as a guard whose message overstates its measurement, applied to a
comment instead of an error string.

Also routed to internal/feed: Append should refuse an empty `type` or `id`
(both the CLI implementer and I think so -- a durable log that accepts an
unidentifiable record is not doing its one job), making the CLI's new
requireNonEmpty defense in depth rather than the sole guard.

## C19 RESOLVED (2026-09-01) -- import-pin hardened, verified by me

Nine cases, each run directly with exit codes checked separately from stdout
(my first pass piped through `head`, which swallowed the exit code -- worth
noting, since exit code IS the contract for a gate):

  aliased exec (_e = exec)          FAIL bare reference to forbidden name: exec       exit 1
  getattr(__builtins__, "eval")     FAIL getattr on __builtins__ is forbidden          exit 1
  "int" + "ernal/"                  FAIL string literal references the Go tree         exit 1
  import subprocess                 FAIL import subprocess not in allowlist            exit 1
  syntax error                      FAIL syntax error: '(' was never closed            exit 1
  non-ASCII file                    FAIL file is not ASCII: ...                        exit 1
  compile(...)                      FAIL bare reference to forbidden name: compile     exit 1
  missing file                      FAIL file not found: ...                           exit 1
  real fixtures/generate.py         ok import-pin                                      exit 0
  --self-test                       ok import-pin self-test (all negative controls caught)  exit 0

Every checker-failure mode now fails CLOSED with a FAIL line rather than a
traceback -- a stack trace would be ambiguous between "refused" and "broken",
which CI cannot distinguish. The self-test went from one injection to seven.

## C20 RESOLVED (2026-09-01) -- positions_match_manifest now has a red cell

New third P6 twin: fixtures/p6/twin-phantom/snapshot.json, mutation
`invented_untraded_position`, instrument ZZZ. Verified by me:

  phantom positions  ['AAA','BBB','CCC','ZZZ']
  golden positions   ['AAA','BBB','CCC']
  leaf_diff(GOLDEN, phantom) = []          <- structurally BLIND, as asserted
  symmetric difference       = 1
  manifest expectation: positions_match_manifest 1, unevaluable_match_manifest 0

The best part is not the twin, it is what the twin PROVES. The generator now
asserts (with a die()) that leaf_diff(P6_GOLDEN, phantom) is EMPTY -- because
leaf_diff walks the golden's keys only and never looks for "ZZZ". So
positions_match_manifest is demonstrably the ONLY thing in the build that
catches an invented position. The check's NECESSITY is now proven, not just its
function.

Placement reasoning (the fixer's, and I agree): "invents a position that never
traded" is a portfolio-math fabrication, and P6 is the only property validating
fold output against an independent HAND-VERIFIED golden rather than another
in-generator computation. It also cannot honestly be a feed event -- a real
fill would make the trade real -- so it is a document-level mutation, the same
pattern P4 already uses. Not bolted onto P1/P3, which would have corrupted
their confinement.

Guards: positions_match_manifest pinned EXACTLY at 1 (not merely non-zero), and
unevaluable_match_manifest must stay 0 for confinement. Three negative
controls: hollow plant -> "got a symmetric difference of 0"; degraded plant
(two instruments) -> "got a symmetric difference of 2", proving the exact pin
catches both directions; confinement break -> "not confined to the position
set". All reverted.

OPEN: p6.twin_phantom is a new manifest key and artifact the Go gate does not
consume yet. Generator-side guarantees are enforced; cross-language consumption
is NOT yet verified. The P6 gate (Task 15) must read it -- and if it does not,
the whole point is lost.

## C21 RESOLVED + feed identity guard (2026-09-01), verified

Comment fixed, code unchanged: the false premise ("a blank line can never be
the mangled remains of a corrupted record") is replaced with the true reasons --
a terminated blank line at EOF cannot be distinguished from the legitimate
crash-loop shape (same bytes, no distinguishing bit), and a blank line carries
no content to CONCEAL anything in. The implementer left the old wrong claim
NAMED in the comment rather than silently rewriting it away, and added a fourth
honest-limits entry for blank-line-replaces-tail-record.

feed.Append now refuses an empty type or id (ErrMissingIdentity). Verified:
  empty type      -> feed: Append requires a non-empty type and id: got type="" id="ev-1"
  empty id        -> feed: Append requires a non-empty type and id: got type="price" id=""
  empty effective -> accepted, seq=1     (deliberate exemption)
and a refusal neither poisons the handle nor consumes a seq -- both asserted.

`effective` stays exempt with the reasoning stated in the doc, not omitted:
it is domain-meaningful field CONTENT the fold already validates, whereas
type/id are structural identity this layer has no other guard for and cannot
function without. Same principle already applied to not constraining payload
keys in round 4. 28 tests, gofmt/vet clean.

## Controller finding C22 (2026-09-01) -- MY evaluated rule falsely refuses the P6 phantom twin

I asked for `evaluated` to be load-bearing without considering a legitimately
EMPTY denominator. SetEquality sets evaluated = len(WANT); when the correct
published set is empty that is 0, and the new guard discards the whole row:

  SetEquality([], [])     -> checks=0 evaluated=0   -> refused (correctly vacuous)
  SetEquality([ZZZ], [])  -> checks=1 evaluated=0   -> REFUSED, though the check
                                                       CAUGHT an invented entry

That is P6's exact shape: the phantom twin's document has `unevaluable: []`, the
golden has no unevaluable key, and positions_at/unevaluable_at are published
only at V1/V2/V3 (seq 21/47/71) while P6 runs at end_seq 14. So the honest want
is empty, evaluated is 0, and Emit would discard the phantom twin -- INCLUDING
positions_match_manifest: 1, the only check in the build that catches an
invented position. The guard I added to stop vacuous passes would have silently
destroyed the check that proves its own necessity.

Fix dispatched: denominator becomes the UNION size, not the want size. Empty vs
empty stays 0 (still refused, genuinely vacuous); empty-want vs invented-got
becomes 1 (credited). The P6 gate author was warned NOT to work around it --
injecting a fake evaluated, skipping the check, or switching to the base feed's
lists would each have hidden the harness bug behind a plausible-looking gate.

## Controller finding C23 (2026-09-01) -- my rule also made three existing tests vacuous

All three original TestEmitRejectsWrongCells cases now pass on the evaluated
guard rather than the rules they were written to test -- each builds
NewCounts("a") without setting Evaluated, so the new guard fires first:

  case 1 twin counts disagree   refused [EVALUATED]   <- wanted COUNT
  case 2 live with a violation  refused [EVALUATED]   <- wanted LIVE-RED
  case 3 twin never goes red    refused [EVALUATED]   <- wanted TWIN-NEVER-RED
  with Evaluated=5: refused [COUNT], [LIVE-RED], [TWIN-NEVER-RED] respectively

Three brief-mandated enforcement rules are correct but untested. Dispatched:
set a positive Evaluated in all three AND record the format string on fakeT so
each case asserts WHICH rule refused it. A test asserting only "it failed"
cannot tell you it failed for the right reason -- the same defect class this
project keeps finding in its own guards, this time in its own test suite.

## Task 16 APPROVED with one follow-up (2026-09-01)

Reviewer approved the import-pin. Remaining Important dispatched: `%` and
`.format()` string assembly still bypass the literal scan. The reviewer's
judgment, which I asked for and agree with: this is OUTSIDE the disclaimed
"determined author" boundary and therefore INSIDE what the gate promises --
its own docstring covers "a path string that creeps in", and %/format are the
two most ordinary ways anyone builds a path in Python. Nobody uses them to
evade a check. By contrast __builtins__["exec"], vars(__builtins__), bytes
literals and variable-mediated concatenation are all determined-author cases
the docstring correctly disclaims, and I told the implementer NOT to chase them
-- enumerating syntactic shapes forever is how a gate acquires the appearance
of rigor without the substance.

## Gate status (2026-09-01) -- four properties credited, P6 blocked on C22

  meridian-lane1-p1  live GREEN 16 keys | twin RED 17 keys  checks==planted
  meridian-lane1-p2  live GREEN 16 keys | twin RED 17 keys  checks==planted
  meridian-lane1-p3  live GREEN 16 keys | twin RED 17 keys  checks==planted
  meridian-lane1-p5  live GREEN 16 keys | twin RED 17 keys  checks==planted

P2, P3 and P5 all went GREEN on the FIRST run -- no Go-fold/naive-fold
disagreement anywhere. P2's pin
sha256:9d353431b4c4b627174682847e0a10d71d4e0f7c2a21459d53b40628033120ce was
cross-checked with sha256sum; its reordered twin genuinely diverges (not a
commuted no-op) and the tampered feed breaks at seq 23 exactly as the manifest
records, via errors.As. P3's three viewpoints give AAA 26 / 61 / 86 -- three
genuinely distinct histories. P5's expected delta is -4, derived by READING the
generator and diffing the two statement files (49531 vs 49535) rather than
assuming the sign.

P6 currently FAILS, and it is the predicted C22 harness defect, not a gate bug:

  p6_test.go:44: P6 live check "unevaluable_match_manifest" has no evaluated
                 denominator (evaluated=0)

The P6 author was warned not to work around it and did not -- no fake
denominator, no skipped check, no switch to the base feed's lists. That is the
right outcome: the gate is correct and the harness is wrong, and the failure
says so plainly instead of being hidden behind a plausible-looking gate.
Waiting on the union-denominator fix.

## Task 16 round 2 verified (2026-09-01)

  "%s/" % "internal"          FAIL string literal references the Go tree   exit 1
  "{}/".format("internal")    FAIL string literal references the Go tree   exit 1
  "%s/%s" % (os.sep, "ok")    ok    (non-literal operands, correctly unfoldable)
  real fixtures/generate.py   ok                                           exit 0
  --self-test (9 controls)    ok                                           exit 0

No false positive on the real generator, which legitimately uses %-formatting
with variables. The unfoldable case is honestly out of scope rather than
silently passed -- stated in the docstring as a static-analysis boundary.

## Controller finding C24 (2026-09-01) -- the fix for a false premise introduced another

Round 6 replaced the false blank-line premise. Its REPLACEMENT first reason is
also unsupported: feed.go now says refusing a blank line "would require telling
it apart from the trailing blank line a crash-loop recovery can legitimately
leave behind", citing TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly.
That test's file contains NO blank lines -- three physical lines, all non-blank.
The cited evidence disproves the claim it is cited for.

The reviewer then tried to CONSTRUCT the shape rather than assume it does not
exist: every partial-write size from 1 to 40 bytes across a two-crash sequence,
single-writer, reopening after each failure as poisoning requires. Zero produced
a blank line. The mechanism explains it: a recovery append prepends "\n" only
when needNewline is set, which happens only when the file ends MID-LINE, so the
prepended newline always TERMINATES non-blank residue rather than creating an
empty one. The only construction requires two concurrent handles with stale
needNewline -- excluded by the package's own doc, and refused by Open anyway.

Dispatched: delete the clause. The other two reasons stand on their own, and
the strongest TRUE reason was already written in the fourth honest-limits
entry -- blanking a tail record concedes nothing that DELETING it does not
already concede, and it leaves residue=1 where deletion leaves 0, so it is the
LOUDER of the two attacks.

Worth recording as a pattern: this is the second time in one file that a
correct rule carried an unsupported reason, and the second was introduced by
the fix for the first. Comments justifying a rule need the same evidentiary
standard as the rule itself -- and "cite a test" is not that standard unless
someone opens the test.

## MILESTONE (2026-09-01) -- all six properties credited, 14 verdict rows

  p1 live GREEN | twin RED    checks == planted
  p2 live GREEN | twin RED    checks == planted
  p3 live GREEN | twin RED    checks == planted
  p4 live GREEN | twin RED    checks == planted
  p5 live GREEN | twin RED    checks == planted
  p6 live GREEN | THREE twins RED (fill, price, phantom)  checks == planted

Every live cell green, every twin red for exactly its planted reason with exact
counts, verified by me from the emitted rows rather than from any agent report.

The union-denominator fix (C22) landed -- gates/manifest.go:196 is now
`len(union)` -- which is why the phantom twin's positions denominator is 4
({AAA,BBB,CCC} + ZZZ) rather than 3. The P6 author had flagged that as an
unexplained anomaly; it is the fix working, not a defect.

OPEN, and it is a real inconsistency rather than a cosmetic one: P6 carries a
LOCAL `unevaluableCheck` wrapper in p6_test.go because its unevaluable sets are
empty on BOTH sides, so even the union denominator is 0 and Emit refuses the
row as vacuous. The wrapper substitutes the position-universe size (3) as the
denominator. That is arguably the HONEST number -- the gate examined 3
positions and found none unevaluable, which is a real assertion, not a vacuous
one -- but it means P6 and the other gates now use different denominator
semantics for the same check NAME, decided in one gate's local code rather than
in the harness. Dispatching a harness-level fix so all five gates agree.

- **Task 16: COMPLETE** (2026-09-01). `gates/importpin.py`. Approved after 2
  rounds. Final reviewer pass independently reconfirmed the fold covers
  concatenation, `%` single-arg AND tuple-arg, `.format()` positional AND
  kwargs -- exercising two forms my own checks had missed -- and that
  unfoldable or malformed input degrades cleanly (returns "cannot fold")
  rather than crashing or false-flagging. No false positive on the real
  generator, which uses %-formatting with variables throughout.
  The docstring states STRUCTURAL-not-epistemic independence and the
  drift/accident-vs-determined-author boundary, and the reviewer confirmed the
  code matches the claim rather than the claim being aspirational.

- **Task 13 (P4): built, live GREEN first try.** Notable: the implementer
  DEPARTED from the brief's per-position hit-counting sketch, which would have
  produced silent_zero=1 and stale_carry_forward=1 against a manifest requiring
  2 and 3. Rather than hand-fitting a constant to the number, it decomposed
  each check into genuinely independent sub-conditions (price==0 and
  price_seq==0 as two separate silent_zero tests; price/price_seq/unrealized as
  three separate stale-carry tests) and verified the decomposition against the
  twin JSON and the generator's own leaf_diff before trusting it. It also
  recomputed each instrument's true last (price, seq) straight from the FEED
  rather than from any snapshot, so a shared bug could not cancel out.

- **Task 15 (P6): built, 4 rows.** The implementer corrected its own report:
  the evaluated=4 it had flagged as an unreconciled anomaly turned out to be
  the concurrent union fix working (union of a 4-member got with a 3-member
  want). It verified that with a throwaway debug test, removed it, and fixed
  the report before finalizing.

## C24 RESOLVED (2026-09-01) -- and the clause was in three places, not one

Verified: zero surviving occurrences, 28 tests green, gofmt clean. The
implementer checked the citation itself before acting -- read
TestCrashLoopTwoGarbageLinesThenValidRecordOpensCleanly and confirmed its file
is three NON-BLANK physical lines -- rather than taking my report on faith.

It found the clause in THREE places, not the one I quoted: the package-doc
paragraph, scan()'s inline comment, and the fourth honest-limits entry, which
independently repeated the false claim as its own justification. It grepped
after each edit to confirm none survived.

It also kept the correction VISIBLE in the comment -- naming the removed third
reason, the test it was checked against, and why it was false -- rather than
silently disappearing it. That is the right call: a reader who wonders why the
obvious symmetry argument is absent now finds out it was tried and refuted,
instead of re-adding it.

And it strengthened the surviving argument I had only gestured at: blanking a
tail record concedes nothing that TRUNCATION does not already concede, and is
the LOUDER of the two (residue=1 versus residue=0). It moved that ahead of the
weaker reasons so a reader meets the strong argument first.

## Denominator semantics settled (2026-09-01)

Two helpers, deliberately different, documented in gates/manifest.go:
  SetEquality(c,name,got,want)                   denominator = len(got ∪ want)
    -- correct for positions_match_manifest, where the compared sets ARE the
       assertion's full scope
  SetEqualityOverUniverse(c,name,got,want,universe)  denominator = len(universe)
    -- correct for unevaluable_match_manifest, whose real assertion is "every
       position in the ledger was checked for unevaluable-ness", not "the union
       of two possibly-empty unevaluable lists"

My proposal to put the universe INSIDE SetEquality was wrong and the harness
author said so: the helper cannot know a caller's meaningful scope, and baking
a guess in would have produced a denominator that looks principled and means
nothing. Two functions, not one changed behavior.

Notable: gates/p6_test.go had independently converged on the identical function
NAME and SIGNATURE before the harness helper existed. Two agents solving the
same problem the same way without coordination is evidence the shape is right --
though evidence about the shape, not the reasoning.

Propagated to P1, P3, P4 (six call sites still on the old semantics; harmless
today because their unevaluable_at lists are non-empty, but the same
one-name-two-meanings inconsistency).

## P6 source-of-truth question answered (2026-09-01)

Asked whether the generator should publish a P6-scoped positions_at /
unevaluable_at. The P6 author's answer, which I accept: NO, keep golden.json as
the sole source. Base's positions_at is fine because base's expected/V*.json is
ITSELF Python-fold-derived -- same authority level. P6's golden is deliberately
HAND-COMPUTED precisely to escape that cross-implementation agreement, so a
generator-published P6 list would be a second, LOWER-authority source
duplicating what golden.json's positions object already encodes (and its
unevaluable-emptiness by omission). Agreement between them would prove nothing
new; disagreement would be unarbitrable without redoing the hand computation.
Real cost, acknowledged and not new: any future change to fixtures/p6's traded
instruments needs the same hand-verification discipline golden.json already
requires.

- **Task 2: COMPLETE** (2026-09-01). `internal/feed/`. APPROVED after SEVEN
  rounds, no conditions. 28 tests.
  The reviewer's own summary of what mattered: the findings were not the ones
  either of us predicted. The tamper-detection framing I asked it to judge did
  not survive measurement; the real defect underneath was DURABILITY -- an
  acknowledged record silently lost, then a bound (mine) that turned that into
  an UNRECOVERABLE file, then an aliased payload, then a read error opening a
  truncated feed, and finally two correct rules resting on false reasons.
  Two things it would defend as much as the code: the honest-limits section
  names four gaps a bare chain cannot close and says plainly that the external
  pin, not this chain, protects the tail; and the corrections stayed VISIBLE,
  so the next reader who notices the obvious symmetry argument finds out it was
  already tested rather than re-adding it.

## Controller correction (2026-09-01): I asked for a duplicate helper

I asked impl-t10 to add `SetEqualityOver` without realising it had already
shipped `SetEqualityOverUniverse` in its previous message. It refused, correctly:
building a second function under my name "would recreate the exact 'two places
agreeing on one rule by accident' shape this whole thread is about". Only the
NAME differed. That is the second time an agent has protected the codebase from
an instruction of mine that was based on stale state -- the first being the P6
author declining to work around the harness bug. Both times the standing
instruction to push back rather than comply is what saved it.

It also did the genuinely new part: documented on `Counts` what `evaluated`
MEANS -- the size of the universe a check examined, not the size of the thing
it looked for; identical for simple checks, which is why it was easy to miss;
divergent for set-equality checks; and it named both defects that came from the
meaning being implicit.

## Carry-forward past Lane 1 (from the feed reviewer)

The residue-in-chain design (D2) remains the better end state and is still
measured: it closes junk injection and the lazy tail forgery with NO bound, and
is byte-identical for clean feeds so it migrates rather than rewrites. Its
blocker was always knowing whether a feed carries residue -- which
`UnparseableLines()` now measures. The reviewer's point: once the Python
generator is out of hardening, that is the cheapest it will ever be to do.

## Denominator propagation COMPLETE and verified (2026-09-01)

  14 rows | live GREEN 6/6 | twin RED 8/8 | twins mismatching planted: 0
  SetEqualityOverUniverse call sites: p1 3, p3 3, p4 2, p6 4

One apparent leftover checked and cleared: p4_test.go:91 still uses plain
SetEquality, but for `unevaluable_matches_planted` -- a DIFFERENT check that
compares the document's unevaluable set against the manifest's p4.withheld
list. Both sets are non-empty ({CCC} vs {CCC}), so the union genuinely IS its
universe and plain SetEquality is correct there. My grep was the false
positive, not the code.

The P1 author proved "only evaluated should change" EMPIRICALLY rather than
asserting it: ran the gate into one verdict dir before the edit, into a second
after, and diffed all four rows key by key. The only differing top-level key
besides ran_at (wall-clock) is `evaluated`, with unevaluable_match_manifest
going 1 -> 5. checks, result and planted byte-identical. 1 was the old union of
two single-element {"CCC"} lists; 5 is the real position universe P1 examines.

## P4 decomposition hand-verified (2026-09-01) -- answers the reverse-fit concern

I asked whether P4's departure from the brief's counting sketch (which would
give 1 and 1 against a manifest requiring 2 and 3) was a genuine independent
measurement or reverse-fitted to the numbers. The implementer redid it by hand
with real arithmetic against the RAW FEED, not the snapshot:

  AAA, BBB, EEE   true (price,seq) match the twin's stored values exactly;
                  qty*price - total_cost recomputed by hand equals stored
                  unrealized. 0 violations each.
  DDD             true (1650, 70) from the feed vs twin's stored (757, 69) --
                  price wrong, seq wrong, and true unrealized
                  77*1650 - 127982 = -932 vs stored -69693. All THREE legs
                  independently wrong -> stale_carry_forward = 3.
  CCC             stored price=0 AND price_seq=0, two independent hits
                  -> silent_zero = 2.

So 2 and 3 fall out of the arithmetic rather than being tuned to the manifest.
That was the highest-risk item in the gate batch and it holds.

Also fixed: mustInt64 now panics on a non-json.Number instead of defaulting to
0 -- a silent 0 inside a VALUATION check is the same defect class P4 exists to
catch.

## All denominator propagations landed and verified (2026-09-01)

P1, P3, P4, P6 all on SetEqualityOverUniverse for unevaluable_match_manifest;
each author diffed emitted rows before/after and confirmed only `evaluated`
moved (1 -> 5 for the base-feed gates, checks/result/planted byte-identical).
14 rows, 6/6 live GREEN, 8/8 twin RED, zero twins mismatching planted.

## MILESTONE (2026-09-01) -- `sh gates/run.sh` is green end to end

  prop | live  | twin(s)         | claimable
  P1   | GREEN | RED*            | YES
  P2   | GREEN | RED*            | YES
  P3   | GREEN | RED*            | YES
  P4   | GREEN | RED*            | YES
  P5   | GREEN | RED*            | YES
  P6   | GREEN | RED*,RED*,RED*  | YES
  + six WARNs that STATUS.md does not yet mark them CLAIMABLE (correct today)
  ok lane1 claimable=6/6

Run by me, not taken from a report. RED* means red WITH the planted counts
matched; a plain RED would not credit.

All three of the runner's negative controls fired and were reverted:
  corrupt a twin's checks vs planted -> that property drops to NO. Notably the
    independent re-derivation caught it even though the row's own `result`
    field still said RED -- claimability.py is genuinely re-deriving rather
    than trusting Emit's verdict, which is the whole point of having two.
  delete a property's rows            -> exit 1, and NO "ok .../6" line at all,
    so a missing gate can never read as "not claimable yet".
  hand-edit STATUS.md to overclaim    -> FAIL STATUS.md overclaims P1, exit 1,
    then restored byte-for-byte (md5 identical, git status clean).

claimability.py never disagreed with Emit's enforcement.

KNOWN ENVIRONMENTAL HAZARD, not a defect: all agents in this session share one
checkout, so MERIDIAN_VERDICT_DIR resolves to the same gates/out. Concurrent
`go test` runs collide and can double every row, which correctly drove
claimable to 0/6 rather than to a false positive -- but a low count during
concurrent activity may be contention rather than a real gate failure.
Hardening dispatched so the failure is diagnostic rather than confusing.

## Controller finding C25 (2026-09-01) -- P4 credits a NARROWER property than its name

The gate review's highest-value finding. P4 claims FAIL-CLOSED VALUATION. It
catches a fabricated zero and a stale carry-forward. It does NOT catch a SILENT
OMISSION -- a position whose valuation stops being produced without being
declared unevaluable -- which is the purest failure of the property and the
cheapest regression a fold can make. Verified by me:

  honest    unevaluable ['CCC']  null-valuation ['CCC']        biconditional OK
  tampered  unevaluable ['CCC']  null-valuation ['CCC','DDD']  BROKEN
  and every existing P4 check still reads 0 -> the gate goes GREEN

The reviewer built it rather than reasoned about it: nulling DDD's valuation
while leaving `unevaluable` alone produces BYTE-IDENTICAL verdicts. The missing
assertion is a biconditional -- a position has a null valuation IF AND ONLY IF
it is listed in `unevaluable`. The signal is already present and unread: the
denominators move (silent_zero 10->8, stale_carry_forward 12->9) and no check
consults them.

UNSOUND, not untidy: as scoped, P4 credits a narrower property than its name.

## Controller finding C26 (2026-09-01) -- silent_zero: 2 matches by COINCIDENCE

The gate counts failed SUB-CONDITIONS on one position (price==0, price_seq==0).
The generator counts differing LEAF PATHS ($.positions.CCC.valuation,
$.unevaluable). Both equal 2, measuring different things, neither a subset of
the other -- the gate never inspects `unevaluable` inside silent_zero, and the
generator never separates price from price_seq because it compares `valuation`
whole as one leaf.

Fragile in BOTH directions, demonstrated: leave CCC in unevaluable and fabricate
only the zero valuation -> generator 1, gate 2. Set price_seq to a nonzero stale
value alongside price:0 -> gate 1, generator 2. Either way Emit fails a plant
that is still a genuine silent zero.

The twin IS red for the planted reason, so the CREDIT survives; what fails is
the "exact counts" half of the crediting rule -- for this check the count
verifies nothing. One of the two sides has to move; I have asked the generator
to adopt the gate's sub-condition decomposition, since it is the more
meaningful measurement for a valuation check.

## Duplicate-row detection VERIFIED, and it caught a real collision (2026-09-01)

I ran claimability.py against what I believed was a clean gates/out and it
exited 1 with:

  P4   | -     | -   | NO (duplicate rows)
  FAIL duplicate verdict rows for meridian-lane1-p4 live: 2
       (concurrent runs writing the same MERIDIAN_VERDICT_DIR?)
  FAIL duplicate verdict rows for meridian-lane1-p4 twin
       mutation=silent_zero_and_stale_carry_forward: 2

16 rows, not 14: a sibling agent's concurrent `go test -run P4` had written a
second pair into the shared directory. So the hardening caught a REAL collision
within minutes of landing, and named the cause instead of collapsing the whole
table to a confusing 0/6. That is the difference between a red someone
investigates and a red someone learns to dismiss.

Clean re-run: run.sh exit 0, 14 rows, claimability exit 0, ok lane1 claimable=6/6.

The discriminator was chosen against real data rather than assumed: every
property's twin carries a distinct planted.mutation string, and P6's three are
fill_qty_plus_one / price_plus_one / invented_untraded_position -- so twins
group by mutation and live rows by count, generalizing without hardcoding
"P6 has three". The implementer also closed a gap I had not asked about:
duplicate-flagged properties are excluded from the STATUS.md "supported"
computation, since claimable() alone would have let a twin duplicated with
itself pass.

STANDING NOTE for anyone reading a low claimable count on this checkout while
agents are active: check the row count first. 14 is correct; more means
contention, and the FAIL line now says so.

## Controller finding C27 (2026-09-01) -- CI pins a Python the fixtures were never built on

.github/workflows/gates.yml pins python-version "3.12". The checked-in fixtures
were generated on Python 3.14, the only interpreter on this machine.
fixtures/generate.py drives its stream through random.Random(SEED) using
randint, choice and random(). CPython guarantees backward compatibility for
random() itself, but randint/choice go through _randbelow, whose implementation
is NOT contractually stable across versions.

If 3.12's stream differs, fixtures/generate_test.sh fails in CI on the freshness
diff -- and it will LOOK like fixture staleness rather than an interpreter
difference. Cannot be tested here: 3.14 is the only Python available.

Dispatched: pin CI to 3.14, with a comment saying WHY it is pinned rather than
floating -- the fixtures are byte-reproducible only on the interpreter that
produced them, and the freshness test is the detector. Pinning to a version
nobody has verified means CI checks a claim we have not made.

DURABLE ANSWER, recorded but NOT done now: the generator should stop depending
on random's implementation entirely -- an explicit spelled-out PRNG would make
the fixtures reproducible on any interpreter. That changes every planted value,
every footprint count and the pinned snapshot hash, so it is a deliberate future
change, not a now change. Recorded as a known constraint with a named detector
rather than papered over with an unexplained version pin.

## C27 RESOLVED + a durable-fix insight (2026-09-01)

CI pinned to Python 3.14, matching the interpreter that produced the fixtures,
with a comment stating WHY it is pinned rather than floating and what to do if
it is ever bumped (regenerate on the new interpreter and confirm
generate_test.sh still byte-matches FIRST).

Useful specific I did not have: `getrandbits` is the one call in random.Random
with a real cross-version guarantee. randint/choice instability is entirely
downstream of _randbelow, which exists only to bound getrandbits output. So the
durable fix -- routing the generator's stream through getrandbits directly -- is
smaller than "write an explicit PRNG", though it still regenerates every planted
value and the pinned snapshot hash, so still not a now-change.

## Expected transient red (2026-09-01) -- manifest-first coordination

  gates/p4_test.go:163: P4 twin check "undeclared_unpriced" has a planted
  expectation (0) but was never computed

This is Emit correctly refusing an inconsistent row, not a defect. The
generator landed the new manifest key before the gate wired the check, so there
is a window where the manifest plants something the gate does not compute.

Coordination lesson: manifest-first ALWAYS opens a red window, and gate-first
would open the mirror-image one (a check with no planted expectation, which
Emit also refuses). Either order is briefly red, which is correct behaviour --
the harness refuses to credit a row whose two sides disagree. What matters is
that the window is expected and announced, not that it is avoided. The T17
author reported it rather than waiting it out or touching files it did not own,
which is the right handling.

## Controller finding C28 (2026-09-01) -- three checks are never falsified anywhere

The project's thesis applied to its own gates. A check reading 0 in every cell
of every gate has no demonstrated ability to go RED, so its 0 is not evidence:

  fresh_process_identical (P2)  -- P2's HEADLINE check, the determinism claim
    itself. The twin exercises pinned_hash_match twice and chain_verifies once;
    nothing anywhere plants non-determinism. Credible (it compares two real
    subprocess runs, so not vacuous) but unfalsified.
  three_histories (P3)          -- 0 on live AND twin. Recomputed AAA
    quantities: live {26,61,86}, twin {26,87,86}. The leak moves V2 from 61 to
    87, still distinct from both, so the check CANNOT fire under the only twin
    P3 has. viewpoint_V2 (=3) does all the work of catching the leak.
  unevaluable_match_manifest (P6) -- 0 across all four cells, comparing [] to []
    because golden.json has no unevaluable key at all.

Dispatched, deliberately NOT as new twins: a demonstration that each comparator
DISCRIMINATES -- drive the same logic with knowingly different inputs and assert
it reports the difference, the way the SetEquality table test does. Planting
genuine non-determinism would mean deliberately breaking the thing under test.
Each must state plainly what that buys (the comparator can report a difference)
and what it does not (the ledger can produce one).

## Controller finding C29 (2026-09-01) -- P2's twin polarity is inverted

In every other gate a non-zero Checks value means the artifact FAILED. In P2's
twin it means the guard CORRECTLY fired: snapshot_hash_diverges_mutated = 1
when hm != pin, the identical predicate to the live cell's pinned_hash_match
read the opposite way, published under `expected_violations`.

Good consequence: because it IS the same predicate, P2's twin genuinely proves
the live check would fire on a defective feed -- sensitivity demonstrated, not
assumed, which is better than most gates manage.
Bad consequence: the emitted row files a SUCCESS as a VIOLATION. A reader sees
chain_break_detected: 1 beside result: RED and cannot tell from the row alone
that this means the check worked.

Routed as a presentation fix (a note in the row's `scope`), not a logic change,
since renaming would desynchronise manifest keys for a naming problem.

## Cross-language decomposition is SELF-CHECKING (2026-09-01)

The generator author flagged honestly that it matched the sub-condition
decomposition I DESCRIBED, not the Go source it never read. That concern is
real but self-limiting: if the two sides' decompositions disagree, the twin
row's checks will not equal planted.expected_violations and Emit refuses the
row. The harness catches exactly this class. Worth knowing rather than worrying
about -- and it is the reason the manifest-versus-gate agreement is meaningful
at all.

## C28/C29 partially resolved -- P2 (2026-09-01)

P2's headline check now has a falsifiability demonstration:
`replaysIdentical(h1,b1,h2,b2)` factored out of the inline comparison, plus
`TestReplaysIdenticalDiscriminates`, a 6-case table test proving the comparator
reports known-different artifacts as different. Its doc comment states BOTH
what that proves (the comparator discriminates) and what it does not (that the
ledger can produce two different replays) -- no non-determinism was planted
into the fold, deliberately.

Polarity handled as a presentation fix, logic untouched: the twin row's `scope`
now reads "NOTE polarity: unlike every other gate's twin, a non-zero check here
means the guard correctly DETECTED the planted defect (a pass), not that the
artifact failed it", with a matching source comment. Emitted rows diffed: checks
and evaluated byte-identical, only `scope` changed.

Worth noting how it verified under a broken tree: p3_test.go was mid-edit by
another agent and would not compile, so it COPIED the tree to a scratch dir,
removed the other agents' broken files THERE, and ran the suite -- never
touching the real repo. That is the right way to work around a concurrent
break; the wrong way would have been to "fix" a file it did not own.

## Round 9 partial (2026-09-01) -- P6 rename + golden landed; P4 key removal pending

Verified:
  p6.twin_fill    {golden_match 7, positions_match_golden 0, unevaluable_match_golden 0}
  p6.twin_price   {golden_match 3, positions_match_golden 0, unevaluable_match_golden 0}
  p6.twin_phantom {positions_match_golden 1, unevaluable_match_golden 0}
  fixtures/p6/golden.json now carries "unevaluable": []

Still present, so impl-t13 remains blocked: p4.twin.expected_violations still
publishes `unevaluable_matches_planted: 1`, the duplicate of
`unevaluable_match_manifest`. The gate author is correctly holding its code
change until the key is gone, because removing the check first would leave a
planted expectation with no computed check.

Current suite state is the expected mirror-image red:
  p6_test.go:52: P6 twin check "positions_match_golden" has a planted
  expectation (0) but was never computed
-- the manifest renamed first, so the gate must follow. Unblocked.

This is the third time this ordering has produced a brief, correct red. Worth
stating as a property rather than an annoyance: any change to a
cross-language expectation is red in one direction or the other while it lands,
because Emit refuses a row whose two sides disagree. The alternative -- a
harness that tolerated a mismatch during transitions -- is exactly the hole that
let eight zero-valued expectations go unenforced earlier in this build.

## MILESTONE (2026-09-01) -- all six credited again, now with the missing legs

  P1 GREEN | RED*                 P4 GREEN | RED*,RED*
  P2 GREEN | RED*                 P5 GREEN | RED*
  P3 GREEN | RED*                 P6 GREEN | RED*,RED*,RED*
  ok lane1 claimable=6/6      (15 rows)

Since the last milestone, the gate review added real evidence rather than
polish: P4 gained the silent-omission leg it was missing entirely (a document
that stops pricing a position without declaring it unevaluable now has a red
cell), P2's headline determinism check and P3's three_histories each gained a
demonstration that their comparator DISCRIMINATES, P6's key names stopped
claiming an authority they do not read, and golden.json's silence about
unevaluable became an explicit stated fact.

Both P2 and P3 stated in comments exactly what their new discrimination tests
prove (the comparator can report a difference) and what they do not (the ledger
can produce one) -- and P3 noted that a real collapse would be caught by
viewpoint_V1/V2/V3 anyway, so the gap is covered rather than merely admitted.

Two agents hit a concurrent build break mid-verification and neither touched a
file it did not own: one copied the tree to a scratch dir and removed the other
agents' broken files THERE; the other owned up to having caused it with a
mid-edit slip and fixed it immediately. Both are the right handling.

STILL OPEN: p4.twin.expected_violations still publishes
`unevaluable_matches_planted`, the duplicate of `unevaluable_match_manifest`.
The gate author is correctly holding until the manifest key is dropped.

## Round 9 COMPLETE (2026-09-01) -- all three changes, one regeneration

  p6 twins renamed to positions_match_golden / unevaluable_match_golden,
    values unchanged (0/0, 0/0, 1/0); helper docstring and P6's own die-messages
    updated too, so nothing calls P6's check "manifest" any more
  golden.json carries explicit "unevaluable": [] -- verified NOT decorative by
    an unprompted control: setting it wrong gives "FAIL naive fold disagrees
    with the hand-computed P6 golden at ['$.unevaluable']"
  p4.twin.expected_violations now has exactly five keys; the duplicate
    unevaluable_matches_planted is gone. p4.withheld itself retained -- it still
    drives the price-withholding and is still independently asserted -- so
    nothing was left unreferenced

P6 gate side confirmed the predicted numbers by re-running rather than assuming:
evaluated.golden_match 16 -> 17 (the new leaf), Checks["golden_match"] unchanged
at 0/7/3, all three twins RED under the new names. It also added the comment
naming the limit precisely: a fact about THIS golden and THESE two mutations,
not a structural guarantee, and what a future mutation touching unevaluable
status would have to account for.

## Gate suite (P2-P6): APPROVED (2026-09-01)

Reviewer recomputed every published number from source before approving:
P2's twin hashes and the seq-23 chain break; P3's six cells and three viewpoint
quantities; P4's trueLastPrices table straight from the feed; P5's compared
count of 11 and the derived -4 delta; P6's Leaves(golden)=16 and both twin
footprints of 7 and 3.

Its verdict on the three unfalsified checks is the wording STATUS.md now uses,
and the distinction is worth preserving exactly: a comparator-discrimination
test proves the CHECK can tell the two cases apart; a twin proves the LEDGER
can produce the defect. The first answers "is this check vacuous"; the second
answers "has this ever been seen to fail". Claiming the first while a reader
assumes the second IS the failure mode -- naming it in honest-limits forecloses
it.

### Remaining Minor items, carried to final triage (none affects a claim)

  p4_test.go:78-83  comment says stale_carry_forward's three legs are checked
                    "independently"; on the DDD twin leg 3 (unrealized) is an
                    ARITHMETIC CONSEQUENCE of leg 1 (price) -- verified:
                    unrealized -69693 == qty*doc_price - total_cost, so with
                    qty=77 a wrong price forces a wrong unrealized. The leg has
                    real power in general; the comment overstates it for this
                    twin. DISPATCHED as a comment fix.
  p4_test.go:100    a `valuation: {}` (empty object, not null) would reach
                    mustInt64(nil) and panic -- a gate crash rather than a
                    reported violation. DISPATCHED with the question of whether
                    that shape is possible at all.
  p2_test.go:46     strings.Fields(...)[0] with no length check
  p3_test.go:28     type assertions -- ALREADY hardened
  three scope notes

## Final Minor items closed (2026-09-01)

  stale_carry_forward comment: "independently" -> "independent in principle",
  with the DDD arithmetic stated -- stored unrealized (-69693) already equals
  qty*price - total_cost for its fabricated price (qty=77 != 0), so the
  unrealized hit is a CONSEQUENCE of the price hit. The comment now says the
  count of 3 on this twin is 2 independent failures plus 1 consequence. The leg
  is kept, because a document could carry a correct price with an independently
  wrong unrealized and this would still catch it.

  valuation:{} reaching mustInt64(nil): the author judged the shape POSSIBLE
  and said why -- snapshot.Build never emits a partial valuation, but twin
  fixtures are hand-built JSON from an independent Python generator and are not
  required to go through Build, so nothing rules {} out. Added int64OrZero
  returning (0,false) rather than panicking, applied to the three valuation
  sub-fields only. A missing price or price_seq now folds into the branch
  price==0 already takes, argued in the comment as "missing is if anything
  worse than an explicit zero"; a missing unrealized counts as a
  stale_carry_forward mismatch. mustInt64 (panic) kept for qty/total_cost,
  which are always present on a real position -- malformed there means the
  document is broken outside P4's scope. Emitted rows byte-identical: a
  robustness fix for a shape nothing in fixtures/ currently exercises.

## FINAL VERIFIED STATE (2026-09-01)

  sh gates/run.sh   -> ok lane1 claimable=6/6
  gates/out         -> 15 verdict rows
  go test ./...     -> 8 packages ok
