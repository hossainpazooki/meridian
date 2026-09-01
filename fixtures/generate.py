#!/usr/bin/env python3
"""MERIDIAN fixture generator + naive fold.

stdlib only; imports nothing from the Go tree; reads nothing but its own
constants. It plants the ground truth (fills, actions, one amendment, every
twin defect) and embeds an independent naive fold that emits the custodian
statement (P5), per-viewpoint expectations (P1/P3) and every known-bad twin
artifact. The Go ledger must recover what this file planted.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
import random
import sys

GENESIS = "0" * 64
SEED = 20260831
INSTRUMENTS = ["AAA", "BBB", "CCC", "DDD", "EEE"]
WITHHELD = ["CCC"]          # P4: no price event ever emitted for these
START = dt.date(2026, 1, 5)


# ---------- canonical bytes ----------
def canon(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def sha(s):
    if isinstance(s, str):
        s = s.encode("ascii")
    return hashlib.sha256(s).hexdigest()


def rhe(n, d):
    quo, rem = divmod(n, d)
    if 2 * rem > d:
        return quo + 1
    if 2 * rem < d:
        return quo
    return quo if quo % 2 == 0 else quo + 1


# ---------- feed construction ----------
class Ev:
    __slots__ = ("type", "id", "effective", "payload")

    def __init__(self, type_, id_, effective, payload):
        self.type, self.id, self.effective, self.payload = type_, id_, effective, payload


def chain(events):
    """Assign seq 1..n and prev hashes; return (lines, records)."""
    lines, records, prev = [], [], GENESIS
    for i, e in enumerate(events, 1):
        rec = {"effective": e.effective, "id": e.id, "payload": e.payload, "prev": prev, "seq": i, "type": e.type}
        line = canon(rec)
        lines.append(line)
        records.append(dict(rec, line_hash=sha(line)))
        prev = sha(line)
    return lines, records


def first_chain_break(lines):
    """Recompute the hash chain over raw JSONL line text (not the parsed
    records, so this is independent of the writer that produced them).
    Checks BOTH legs of the chain: the stored 'prev' hash-linkage AND that
    'seq' actually matches physical line position -- a feed with an intact
    hash chain but scrambled sequence numbers is just as broken as one with
    a hash mismatch, and only checking 'prev' would miss it. Returns the
    1-indexed line number of the first line that fails either check, or
    None if the chain holds end to end.
    """
    prev = GENESIS
    for i, ln in enumerate(lines, 1):
        rec = json.loads(ln)
        if rec["prev"] != prev or rec["seq"] != i:
            return i
        prev = sha(ln)
    return None


def chain_break_reason(lines, break_at):
    """Describe WHICH leg of first_chain_break's check failed at a given
    1-indexed line number. The return value is a line index either way, but
    a seq-leg failure means `seq` itself is the wrong field -- a message
    that calls that a "break at seq %r" mislabels a scrambled sequence
    number as a hash-chain break.
    """
    rec = json.loads(lines[break_at - 1])
    if rec["seq"] != break_at:
        return "seq %r does not match line position %d" % (rec["seq"], break_at)
    return "prev hash mismatch at seq %d" % break_at


def read_feed_records(path):
    """Re-parse a feed.jsonl the generator already WROTE, straight off disk,
    into full records (adding line_hash the same way chain() does). Used to
    make a recompute check genuinely independent of the in-memory value it
    is checking -- calling naive_fold(recs, v) a second time with the exact
    same `recs` object only proves Python is deterministic, since it can
    never disagree with itself. Reading the bytes actually written and
    re-parsing them crosses a real boundary (write_lines/canon roundtrip)
    that a serialization bug could break.
    """
    records = []
    with open(path, "r", newline="\n") as f:
        for ln in f:
            ln = ln.rstrip("\r\n")  # strip CR too: open(newline="\n") disables translation, so a CRLF checkout would otherwise leave \r in the hashed bytes
            records.append(dict(json.loads(ln), line_hash=sha(ln)))
    return records


def holdings_diff(a_holdings, b_holdings):
    """Pairwise diff of two statement `holdings` lists by instrument: return
    the list of (instrument, field) pairs that differ. `leaf_diff` cannot
    see inside `holdings` (a list, compared whole, one atomic leaf) -- this
    is the finer-grained walk P5's drift twin actually needs.
    """
    a_by = {h["instrument"]: h for h in a_holdings}
    b_by = {h["instrument"]: h for h in b_holdings}
    out = []
    for inst in sorted(set(a_by) | set(b_by)):
        ah, bh = a_by.get(inst), b_by.get(inst)
        if ah is None or bh is None:
            out.append((inst, "*presence*"))
            continue
        for k in sorted(set(ah) | set(bh)):
            if ah.get(k) != bh.get(k):
                out.append((inst, k))
    return out


def null_valued_instruments(doc):
    """Instruments in a snapshot document whose valuation is null (None)."""
    return sorted(i for i, p in doc["positions"].items() if p["valuation"] is None)


def undeclared_unpriced(doc):
    """P4's fail-closed-valuation claim rests on a biconditional: a position
    has a null valuation IF AND ONLY IF its instrument is listed in
    `unevaluable`. Returns the count of instruments violating it in EITHER
    direction (symmetric difference between the null-valued set and the
    declared-unevaluable set) -- a silent omission (null but undeclared) is
    the dangerous direction, but a declared-unevaluable entry that
    nonetheless carries a real valuation is also a contract violation, not
    just the omission case.
    """
    return sym_diff_count(null_valued_instruments(doc), [u["instrument"] for u in doc["unevaluable"]])


def count_true(conditions):
    """Count of True values in an explicit list of named boolean checks --
    used where the generator must measure the SAME sub-condition
    decomposition a Go gate measures (e.g. price == 0 and price_seq == 0
    for silent_zero), not a leaf-path footprint. A leaf-path count and a
    sub-condition count can coincide by accident on one seed while
    measuring genuinely different things; matching the gate's own
    decomposition means the count means the same thing on both sides.
    """
    return sum(1 for c in conditions if c)


def sym_diff_count(doc_instruments, published_instruments):
    """positions_match_manifest / unevaluable_match_manifest (P1/P3/P4,
    checked against the base feed's positions_at/unevaluable_at) and
    positions_match_golden / unevaluable_match_golden (P6, checked against
    P6_GOLDEN's own sets -- P6 has no viewpoint in the base feed's
    manifest, so its authority is a different artifact and its keys say so)
    are both defined once, here: the number of instruments in the
    SYMMETRIC DIFFERENCE between a twin document's instrument set and the
    corresponding reference set. Zero means the sets match exactly; a
    nonzero count means the twin invented, dropped, or (for unevaluable)
    wrongly cleared/kept an instrument relative to the reference -- exactly
    what leaf_diff's one-sided, golden-keys-only walk cannot see.
    """
    return len(set(doc_instruments) ^ set(published_instruments))


def assert_unique_ids(records, label):
    """The shared contract requires event `id` to be globally unique within
    a feed. Assert it mechanically for every feed the generator writes,
    rather than trusting each builder to get its own numbering right."""
    ids = [r["id"] for r in records]
    if len(ids) != len(set(ids)):
        dupes = sorted({i for i in ids if ids.count(i) > 1})
        die("%s has duplicate event ids: %r" % (label, dupes))


def write_lines(path, lines):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", newline="\n") as f:
        for ln in lines:
            f.write(ln + "\n")


def write_json(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", newline="\n") as f:
        f.write(canon(obj) + "\n")


def fill_key(p):
    return sha(canon({"trade_id": p["trade_id"], "venue": p["venue"]}))


# ---------- naive fold (independent of the Go tree) ----------
def naive_fold(records, up_to, mode="honest"):
    """Fold records[0:up_to] per the shared contract.

    mode: honest | nodedupe (P1 twin: apply every fill) | leak (P3 twin:
    resolve amendments from the WHOLE feed, not the visible prefix).
    Returns a full snapshot-schema dict.
    """
    vis = records[:up_to]
    src_terms = records if mode == "leak" else vis
    terms = {}
    for r in src_terms:
        if r["type"] == "action":
            terms[r["payload"]["action_id"]] = dict(r["payload"])
    # Amendment resolution ignores `effective` entirely and applies
    # amendments in FEED ORDER -- last-in-feed wins if an action_id ever had
    # more than one amendment. This fixture plants exactly one amendment, so
    # the tiebreak is untested; stating it explicitly here makes it a
    # decision this file makes, not an accident of iteration order.
    for r in src_terms:
        if r["type"] == "action_amendment":
            aid = r["payload"]["action_id"]
            if aid not in terms:
                die("action_amendment %s references action_id %s with no prior action in this prefix" % (r["id"], aid))
            t = terms[aid]
            for k in ("ratio", "rate"):
                if k in r["payload"]:
                    t[k] = r["payload"][k]
    absorbed, refusals, apps, seen = [], [], [], {}
    for r in vis:
        p = r["payload"]
        if r["type"] == "fill":
            key, ph = fill_key(p), sha(canon(p))
            if mode != "nodedupe" and key in seen:
                if seen[key] == ph:
                    absorbed.append({"event_id": r["id"], "key": key, "seq": r["seq"]})
                else:
                    refusals.append({"detail": "payload hash mismatch", "event_id": r["id"], "key": key, "kind": "collision", "seq": r["seq"]})
                continue
            seen[key] = ph
            apps.append((r["effective"], r["seq"], p["side"], p["instrument"], p["qty"], p["price"], 0, 0))
        elif r["type"] == "price":
            apps.append((r["effective"], r["seq"], "price", p["instrument"], 0, p["price"], 0, 0))
        elif r["type"] == "action":
            t = terms[p["action_id"]]
            apps.append((r["effective"], r["seq"], t["kind"], t["instrument"], 0, 0, t.get("ratio", 0), t.get("rate", 0)))
    apps.sort(key=lambda a: (a[0], a[1]))
    cash = realized = dividends = 0
    pos, last = {}, {}
    for eff, seq, kind, inst, qty, price, ratio, rate in apps:
        q, c = pos.get(inst, (0, 0))
        if kind == "buy":
            pos[inst] = (q + qty, c + qty * price)
            cash -= qty * price
        elif kind == "sell":
            if qty > q:
                refusals.append({"detail": "sell %d exceeds held %d" % (qty, q), "event_id": "", "key": "", "kind": "oversell", "seq": seq})
                continue
            rel = rhe(c * qty, q)
            q, c = q - qty, c - rel
            cash += qty * price
            realized += qty * price - rel
            if q == 0:
                # Provably unreachable today, not merely untested: when
                # qty == the pre-sell held quantity, rel = rhe(c*qty, qty)
                # == c exactly (integer division by itself is exact), so
                # c - rel is always precisely 0 here. Kept as a defensive
                # invariant against a future change to rhe() -- a reader
                # should not believe this is covering a live case now.
                if c != 0:
                    die("full liquidation of %s at seq %d left nonzero cost %d" % (inst, seq, c))
                del pos[inst]
            else:
                pos[inst] = (q, c)
        elif kind == "split":
            if inst in pos:
                pos[inst] = (q * ratio, c)
        elif kind == "dividend":
            if inst in pos and q > 0:
                cash += q * rate
                dividends += q * rate
        elif kind == "price":
            last[inst] = (price, seq)
    positions, unev, unreal = {}, [], 0
    for inst in sorted(pos):
        q, c = pos[inst]
        if inst in last:
            px, ps = last[inst]
            u = q * px - c
            unreal += u
            positions[inst] = {"qty": q, "total_cost": c, "valuation": {"price": px, "price_seq": ps, "unrealized": u}}
        else:
            positions[inst] = {"qty": q, "total_cost": c, "valuation": None}
            unev.append({"instrument": inst, "reason": "no_price_in_prefix"})
    refusals.sort(key=lambda r: r["seq"])
    prefix = ("sha256:" + records[up_to - 1]["line_hash"]) if up_to > 0 else "sha256:" + GENESIS
    return {"absorbed": absorbed, "cash": cash, "dividend_income": dividends, "feed_prefix_hash": prefix,
            "feed_seq": up_to, "positions": positions, "realized_pnl": realized, "refusals": refusals,
            "unevaluable": unev, "unrealized_pnl": unreal}


def expected_view(doc):
    return {"cash": doc["cash"], "dividend_income": doc["dividend_income"],
            "positions": {i: {"qty": p["qty"], "total_cost": p["total_cost"]} for i, p in doc["positions"].items()},
            "realized_pnl": doc["realized_pnl"], "feed_seq": doc["feed_seq"]}


def statement(doc):
    return {"as_of_seq": doc["feed_seq"], "cash": doc["cash"],
            "holdings": [{"cost_basis": p["total_cost"], "instrument": i, "quantity": p["qty"]} for i, p in sorted(doc["positions"].items())]}


def leaf_diff(golden, doc, path="$"):
    """Return the list of golden leaf paths the doc does not reproduce (same
    walk as Go snapshot.Diff). A footprint COUNT is len(leaf_diff(...)); a
    tripped guard should report the paths themselves, not just the count, so
    an operator can see WHICH leaves differed instead of decoding a bare
    integer.
    """
    if isinstance(golden, dict):
        out = []
        for k, v in golden.items():
            sub = doc.get(k) if isinstance(doc, dict) else None
            present = isinstance(doc, dict) and k in doc
            out.extend(leaf_diff(v, sub, path + "." + k) if present else count_leaves(v, path + "." + k))
        return out
    return [] if canon(golden) == canon(doc) else [path]


def count_leaves(v, path="$"):
    if isinstance(v, dict):
        out = []
        for k, x in v.items():
            out.extend(count_leaves(x, path + "." + k))
        return out
    return [path]


# ---------- base portfolio ----------
def build_base():
    rng = random.Random(SEED)
    evs, n = [], 0

    def nid():
        nonlocal n
        n += 1
        return "ev-%06d" % n

    tid = [0]

    def fill(day, inst, side, qty, price):
        tid[0] += 1
        return Ev("fill", nid(), day.isoformat(), {"instrument": inst, "price": price, "qty": qty, "side": side, "trade_id": "T-%06d" % tid[0], "venue": "X"})

    held = {i: 0 for i in INSTRUMENTS}   # feasibility under ORIGINAL split terms
    marks = {}
    day0 = START
    for inst in INSTRUMENTS:             # day 0: every instrument opens a position
        q, px = rng.randint(20, 80), rng.randint(500, 3000)
        evs.append(fill(day0, inst, "buy", q, px)); held[inst] += q
    for d in range(1, 40):
        day = day0 + dt.timedelta(days=d)
        if d == 12:                       # deterministic AAA buy right before the split (P2 reorder twin needs it)
            evs.append(fill(day, "AAA", "buy", 10, 1500)); held["AAA"] += 10
            marks["reorder_buy_index"] = len(evs) - 1
            evs.append(Ev("action", nid(), day.isoformat(), {"action_id": "CA-0001", "announced": (day - dt.timedelta(days=2)).isoformat(), "instrument": "AAA", "kind": "split", "processed": day.isoformat(), "ratio": 2}))
            marks["split_index"] = len(evs) - 1
            held["AAA"] *= 2
            continue
        if d == 14:                       # deterministic AAA sell after the split, before the amendment (makes the leak visible)
            evs.append(fill(day, "AAA", "sell", 30, 1400)); held["AAA"] -= 30
            continue
        if d == 18:
            evs.append(Ev("action", nid(), day.isoformat(), {"action_id": "CA-0002", "announced": (day - dt.timedelta(days=3)).isoformat(), "instrument": "BBB", "kind": "dividend", "processed": day.isoformat(), "rate": 25}))
            continue
        if d == 25:
            evs.append(Ev("action_amendment", nid(), (day0 + dt.timedelta(days=12)).isoformat(), {"action_id": "CA-0001", "ratio": 3}))
            marks["amendment_index"] = len(evs) - 1
            continue
        if d == 30:                       # P1 duplicate: exact copy of the first fill, new event id
            src = evs[0]
            evs.append(Ev("fill", nid(), src.effective, dict(src.payload)))
            marks["dup_index"] = len(evs) - 1
            continue
        if d == 31:                       # P1 collision: same identity, qty + 1
            src = evs[1]
            pl = dict(src.payload); pl["qty"] = pl["qty"] + 1
            evs.append(Ev("fill", nid(), src.effective, pl))
            marks["collision_index"] = len(evs) - 1
            continue
        if d in (10, 20, 35, 39):
            for inst in INSTRUMENTS:
                if inst not in WITHHELD:
                    evs.append(Ev("price", nid(), day.isoformat(), {"instrument": inst, "price": rng.randint(500, 3000)}))
            continue
        for _ in range(rng.randint(0, 3)):
            inst = rng.choice(INSTRUMENTS)
            if rng.random() < 0.6 or held[inst] < 2:  # float is RNG control flow only; never written to any fixture file
                q = rng.randint(1, 40)
                evs.append(fill(day, inst, "buy", q, rng.randint(500, 3000))); held[inst] += q
            else:
                q = rng.randint(1, held[inst] // 2)
                evs.append(fill(day, inst, "sell", q, rng.randint(500, 3000))); held[inst] -= q
    return evs, marks


def build_p6():
    # Event `id` is a single running counter over ALL 14 events (matching
    # final seq 1:1), independent of the fill trade counter `t` (which only
    # feeds the payload's `trade_id`, a separate field). The previous version
    # id'd fills by `t` and actions/prices by list position, so the action at
    # seq 7 and the fill at seq 8 (t=7) both got "p6-07" -- a real duplicate
    # id, and "p6-10" was never issued at all.
    n = [0]

    def nid():
        n[0] += 1
        return "p6-%02d" % n[0]

    def f(day, inst, side, q, px, t):
        return Ev("fill", nid(), day, {"instrument": inst, "price": px, "qty": q, "side": side, "trade_id": "T-%d" % t, "venue": "X"})

    def a(day, payload):
        return Ev("action", nid(), day, payload)

    def pr(day, payload):
        return Ev("price", nid(), day, payload)

    return [
        f("2026-01-05", "AAA", "buy", 100, 1000, 1),
        f("2026-01-06", "AAA", "buy", 50, 1301, 2),
        f("2026-01-07", "BBB", "buy", 1, 500, 3),
        f("2026-01-07", "BBB", "buy", 1, 503, 4),
        f("2026-01-08", "CCC", "buy", 1, 500, 5),
        f("2026-01-08", "CCC", "buy", 1, 501, 6),
        a("2026-01-10", {"action_id": "CA-1", "announced": "2026-01-08", "instrument": "AAA", "kind": "split", "processed": "2026-01-10", "ratio": 2}),
        f("2026-01-12", "AAA", "sell", 120, 700, 7),
        f("2026-01-13", "BBB", "sell", 1, 600, 8),
        f("2026-01-13", "CCC", "sell", 1, 600, 9),
        a("2026-01-15", {"action_id": "CA-2", "announced": "2026-01-13", "instrument": "AAA", "kind": "dividend", "processed": "2026-01-15", "rate": 25}),
        pr("2026-01-16", {"instrument": "AAA", "price": 800}),
        pr("2026-01-16", {"instrument": "BBB", "price": 700}),
        pr("2026-01-16", {"instrument": "CCC", "price": 700}),
    ]


# Hand-computed. AAA: 100@1000 + 50@1301 = cost 165050; split 2 -> 300 sh;
# sell 120@700: relieved rhe(165050*120/300) = 66020 exact, cost 99030, qty 180,
# realized 84000-66020 = 17980; dividend 180*25 = 4500; price 800 ->
# unrealized 144000-99030 = 44970. BBB: cost 1003, sell 1@600 relieved 502
# (501.5 half-even up), cost 501, realized 98, price 700 -> 199. CCC: cost
# 1001, relieved 500 (500.5 half-even down), cost 501, realized 100, 199.
# cash = -76550 - 403 - 401 = -77354.
#
# `unevaluable: []` is explicit, not an omission: reading the 14-line feed
# by hand (fills for AAA/BBB/CCC, two corporate actions on AAA, price
# events for all three at seq 12/13/14) supports "every instrument that
# ever traded is also priced by end_seq, so nothing is unevaluable" as a
# fact derived from the artifact -- the same hand-verification tier as
# every other value here. Leaving the key out would make the want an
# ABSENCE rather than a value: if some future twin's mutation caused a
# genuine unevaluable instrument that the fold's output ALSO omitted,
# silence would compare equal to silence and the check would pass on the
# shared error. An explicit `[]` cannot be silently right for the wrong
# reason the way an absent key can.
#
# This is a fact about the CURRENT twins, not a structural guarantee: none
# of twin-fill/twin-price/twin-phantom's mutations touch any instrument's
# price coverage, so this leaf never differs and `golden_match` needs no
# adjustment for it. A future P6 twin whose mutation COULD affect
# unevaluable status (e.g. withholding a price event) would need to check
# whether it also moves `golden_match`'s count.
P6_GOLDEN = {
    "cash": -77354, "dividend_income": 4500, "realized_pnl": 18178, "unrealized_pnl": 45368,
    "unevaluable": [],
    "positions": {
        "AAA": {"qty": 180, "total_cost": 99030, "valuation": {"price": 800, "unrealized": 44970}},
        "BBB": {"qty": 1, "total_cost": 501, "valuation": {"price": 700, "unrealized": 199}},
        "CCC": {"qty": 1, "total_cost": 501, "valuation": {"price": 700, "unrealized": 199}},
    },
}


def die(msg):
    # stderr, not stdout: generate_test.sh (and any caller) redirects the
    # generator's stdout to /dev/null to keep normal output quiet, which
    # would otherwise swallow the FAIL reason along with it and leave an
    # operator staring at a bare exit 1 with no idea which guard tripped.
    print("FAIL " + msg, file=sys.stderr)
    sys.exit(1)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=os.path.dirname(os.path.abspath(__file__)))
    out = ap.parse_args().out

    def P(*parts):
        return os.path.join(out, *parts)

    # ----- base -----
    evs, marks = build_base()
    lines, recs = chain(evs)
    N = len(recs)
    split_seq = marks["split_index"] + 1
    amend_seq = marks["amendment_index"] + 1
    V1, V2, V3 = split_seq - 1, amend_seq - 1, N
    assert_unique_ids(recs, "base feed")
    write_lines(P("base", "feed.jsonl"), lines)
    # first_chain_break's None branch was previously never exercised (its
    # only caller was the tampered-feed check, which always expects a break)
    # -- a stub that always returned a constant seq, or never returned None,
    # would have satisfied that lone caller. Assert it on every intact feed
    # this generator writes, so the helper is shown to discriminate a whole
    # chain from a broken one, not just shown to fire once.
    intact_break = first_chain_break(lines)
    if intact_break is not None:
        die("base feed does not chain end to end: %s" % chain_break_reason(lines, intact_break))
    honest = {v: naive_fold(recs, v) for v in (V1, V2, V3)}
    for name, v in (("V1", V1), ("V2", V2), ("V3", V3)):
        write_json(P("base", "expected", name + ".json"), expected_view(honest[v]))
    write_json(P("base", "statement.json"), statement(honest[V3]))
    q1, q2, q3 = (honest[v]["positions"]["AAA"]["qty"] for v in (V1, V2, V3))
    if len({q1, q2, q3}) != 3:
        die("three viewpoints must give three AAA quantities: %d %d %d" % (q1, q2, q3))
    if [u["instrument"] for u in honest[V3]["unevaluable"]] != WITHHELD:
        die("withheld set mismatch: %r" % honest[V3]["unevaluable"])
    # Structural invariant of naive_fold itself, not something that could
    # vary by seed: it appends to `unevaluable` exactly when it sets
    # `valuation: None`, and never in any other case -- so the honest fold
    # must always satisfy the null-valuation-iff-unevaluable biconditional.
    # Asserted here (cheap, and it should never fire) rather than assumed,
    # since P4's fail-closed-valuation claim depends on it holding for the
    # LIVE row, not just the twin.
    if undeclared_unpriced(honest[V3]) != 0:
        die("honest V3 fold violates the null-valuation-iff-unevaluable biconditional: null-valued %r vs unevaluable %r" %
            (null_valued_instruments(honest[V3]), [u["instrument"] for u in honest[V3]["unevaluable"]]))
    dup, col = recs[marks["dup_index"]], recs[marks["collision_index"]]
    if not any(a["seq"] == dup["seq"] for a in honest[V3]["absorbed"]):
        die("planted duplicate not absorbed by naive fold")
    if not any(r["seq"] == col["seq"] and r["kind"] == "collision" for r in honest[V3]["refusals"]):
        die("planted collision not refused by naive fold")
    # Derived, not hardcoded: the earliest other fill sharing each planted
    # event's dedupe key is its source event -- the same literal-instead-of-
    # derived brittleness class that caused the P2 mutated-twin defect.
    dup_of_seq = next(r["seq"] for r in recs if r["type"] == "fill" and r["seq"] != dup["seq"] and fill_key(r["payload"]) == fill_key(dup["payload"]))
    col_of_seq = next(r["seq"] for r in recs if r["type"] == "fill" and r["seq"] != col["seq"] and fill_key(r["payload"]) == fill_key(col["payload"]))

    # Sets a downstream Go gate can assert set-equality against: leaf_diff is
    # intentionally one-sided (reduced-golden-vs-full-document), so it cannot
    # by itself catch an invented position or a wrongly-omitted unevaluable
    # entry. Compute from the honest fold, then recompute from the feed FILE
    # this generator just wrote (a fresh disk read + fresh chain + fresh
    # fold), not from the retained in-memory `recs` -- naive_fold(recs, v)
    # called a second time on the SAME object is a tautology (it can only
    # ever agree with itself); reading the bytes actually on disk crosses a
    # real boundary that a write_lines/canon bug could break.
    # unevaluable_at covers all three viewpoints, not V3 alone: the nested
    # per-viewpoint shape (matching positions_at) would otherwise invite a
    # gate to look up unevaluable_at.V1, find it absent, and be unable to
    # tell that apart from "nothing is unevaluable at V1". CCC is withheld
    # from every price event, so it is unevaluable at all three viewpoints.
    positions_at = {name: sorted(honest[v]["positions"].keys()) for name, v in (("V1", V1), ("V2", V2), ("V3", V3))}
    unevaluable_at = {name: sorted(u["instrument"] for u in honest[v]["unevaluable"]) for name, v in (("V1", V1), ("V2", V2), ("V3", V3))}
    disk_recs = read_feed_records(P("base", "feed.jsonl"))
    for name, v in (("V1", V1), ("V2", V2), ("V3", V3)):
        disk_fold = naive_fold(disk_recs, v)
        recheck_pos = sorted(disk_fold["positions"].keys())
        if recheck_pos != positions_at[name]:
            die("positions_at.%s does not reproduce from the written feed.jsonl: %r vs %r" % (name, recheck_pos, positions_at[name]))
        recheck_unev = sorted(u["instrument"] for u in disk_fold["unevaluable"])
        if recheck_unev != unevaluable_at[name]:
            die("unevaluable_at.%s does not reproduce from the written feed.jsonl: %r vs %r" % (name, recheck_unev, unevaluable_at[name]))

    # ----- P1 twin: no-dedupe snapshot -----
    p1twin = naive_fold(recs, V3, mode="nodedupe")
    k1_paths = leaf_diff(expected_view(honest[V3]), p1twin)
    k1 = len(k1_paths)
    if k1 == 0:
        die("P1 twin has no footprint")
    p1_positions_match = sym_diff_count(p1twin["positions"].keys(), positions_at["V3"])
    p1_unevaluable_match = sym_diff_count((u["instrument"] for u in p1twin["unevaluable"]), unevaluable_at["V3"])
    write_json(P("p1", "twin", "snapshot.json"), p1twin)

    # ----- P2 twins -----
    mut = [Ev(e.type, e.id, e.effective, dict(e.payload)) for e in evs]
    # Mutation target chosen by PREDICATE (trade_id), not a hardcoded index:
    # the first fill whose trade_id is neither the P1 duplicate's source
    # (evs[0]) nor the P1 collision's source (evs[1]). The duplicate and
    # collision events themselves carry the same trade_id as their source, so
    # excluding by trade_id excludes them too, without naming their indices.
    # A positional exclusion set is exactly the class of bug this defect was:
    # it breaks again, silently, the moment day-0 construction order changes.
    dup_trade_id, col_trade_id = evs[0].payload["trade_id"], evs[1].payload["trade_id"]
    mi = next(i for i, e in enumerate(mut) if e.type == "fill" and e.payload["trade_id"] not in (dup_trade_id, col_trade_id))
    mut[mi].payload["price"] += 1
    mut_lines, mut_recs = chain(mut)
    assert_unique_ids(mut_recs, "P2 mutated feed")
    write_lines(P("p2", "mutated", "feed.jsonl"), mut_lines)
    mut_break = first_chain_break(mut_lines)
    if mut_break is not None:
        die("P2 mutated feed does not chain end to end: %s" % chain_break_reason(mut_lines, mut_break))
    mut_fold = naive_fold(mut_recs, N)
    # Footprint-confinement, not just non-zero-ness: the twin must diverge
    # ONLY in the snapshot values a price mutation should touch, and must
    # reproduce the honest fold's duplicate/collision structure exactly. This
    # is the property Defect 2 silently violated -- it must be mechanically
    # enforced here, not left to be noticed by eye again.
    if mut_fold["absorbed"] != honest[V3]["absorbed"] or mut_fold["refusals"] != honest[V3]["refusals"]:
        bad = leaf_diff({"absorbed": honest[V3]["absorbed"], "refusals": honest[V3]["refusals"]},
                         {"absorbed": mut_fold["absorbed"], "refusals": mut_fold["refusals"]})
        die("P2 mutated twin corrupted the planted duplicate/collision structure at %r" % bad)
    if canon(mut_fold) == canon(honest[V3]):
        die("P2 mutated twin snapshot does not diverge from base")
    # canon(mut_fold) always diverges from canon(honest[V3]) via
    # feed_prefix_hash alone (any re-chain changes it), so that comparison
    # is satisfied by the weakest possible divergence -- a mutation the fold
    # never even reads (e.g. an action's `announced` field) would pass it
    # while leaving the LEDGER byte-identical. Require divergence in the
    # ledger state itself: cash/positions/realized_pnl/feed_seq.
    if canon(expected_view(mut_fold)) == canon(expected_view(honest[V3])):
        die("P2 mutated twin does not diverge in ledger state (cash/dividend_income/positions/realized_pnl/feed_seq) -- only the chain hash differs")

    reo = list(evs)
    bi, si = marks["reorder_buy_index"], marks["split_index"]
    reo[bi], reo[si] = reo[si], reo[bi]
    reo_lines, reo_recs = chain(reo)
    assert_unique_ids(reo_recs, "P2 reordered feed")
    write_lines(P("p2", "reordered", "feed.jsonl"), reo_lines)
    reo_break = first_chain_break(reo_lines)
    if reo_break is not None:
        die("P2 reordered feed does not chain end to end: %s" % chain_break_reason(reo_lines, reo_break))
    reo_fold = naive_fold(reo_recs, N)
    # Same confinement check, scoped to `reordered` too (not just `mutated`):
    # the brief named only `mutated`, but the assertion is a verification-
    # time check independent of which twin's construction it was written
    # against, and today's reordered twin already satisfies it -- this makes
    # that fact enforced rather than incidental.
    if reo_fold["absorbed"] != honest[V3]["absorbed"] or reo_fold["refusals"] != honest[V3]["refusals"]:
        bad = leaf_diff({"absorbed": honest[V3]["absorbed"], "refusals": honest[V3]["refusals"]},
                         {"absorbed": reo_fold["absorbed"], "refusals": reo_fold["refusals"]})
        die("P2 reordered twin corrupted the planted duplicate/collision structure at %r" % bad)
    if canon(reo_fold) == canon(honest[V3]):
        die("P2 reordered twin snapshot does not diverge from base")
    if canon(expected_view(reo_fold)) == canon(expected_view(honest[V3])):
        die("P2 reordered twin does not diverge in ledger state (cash/dividend_income/positions/realized_pnl/feed_seq) -- only the chain hash differs")

    tam = list(lines)
    if '"ratio":2' not in tam[si]:
        die("tamper target not found")
    tam[si] = tam[si].replace('"ratio":2', '"ratio":3', 1)
    assert_unique_ids([json.loads(ln) for ln in tam], "P2 tampered feed")
    write_lines(P("p2", "tampered", "feed.jsonl"), tam)
    break_at = first_chain_break(tam)
    if break_at != si + 2:
        die("P2 tampered twin chain break expected at seq %d, detected %r" % (si + 2, break_at))

    # ----- P3 twin: leak -----
    leak = naive_fold(recs, V2, mode="leak")
    k3_paths = leaf_diff(expected_view(honest[V2]), leak)
    k3 = len(k3_paths)
    if k3 == 0:
        die("P3 leak twin has no footprint")
    # Confinement: the twin is labelled "confined to V2" -- assert it
    # actually is, by running the SAME leak mode at V1 and V3 and checking
    # it reproduces the honest fold exactly there. Not a blocker the way
    # P2's confinement check is (the only leaked artifact is
    # p3/twin/V2.json; the gate sources V1/V3 from the honest Go documents),
    # but the label costs one check to make self-verifying instead of assumed.
    leak_v1_paths = leaf_diff(expected_view(honest[V1]), naive_fold(recs, V1, mode="leak"))
    leak_v3_paths = leaf_diff(expected_view(honest[V3]), naive_fold(recs, V3, mode="leak"))
    if leak_v1_paths or leak_v3_paths:
        die("P3 leak twin is not confined to V2: V1 diff %r, V3 diff %r" % (leak_v1_paths, leak_v3_paths))
    p3_positions_match = sym_diff_count(leak["positions"].keys(), positions_at["V2"])
    p3_unevaluable_match = sym_diff_count((u["instrument"] for u in leak["unevaluable"]), unevaluable_at["V2"])
    write_json(P("p3", "twin", "V2.json"), leak)

    # The leak defect is specific to V2 (the one viewpoint where the fold's
    # visible prefix and the amendment-resolution scope diverge); V1 and V3
    # are the "live legs" that must show ZERO violation, and three_histories
    # asserts the three viewpoints stay genuinely distinct. Previously all
    # three were published as literal 0s nothing computed -- the same defect
    # class as the literal 1s already replaced, just harder to notice
    # because the correct value happens to be falsy.
    viewpoint_V1_paths = leaf_diff(expected_view(honest[V1]), json.load(open(P("base", "expected", "V1.json"))))
    viewpoint_V3_paths = leaf_diff(expected_view(honest[V3]), json.load(open(P("base", "expected", "V3.json"))))
    if viewpoint_V1_paths:
        die("viewpoint_V1 must be zero: honest fold disagrees with its own written expected/V1.json at %r" % viewpoint_V1_paths)
    if viewpoint_V3_paths:
        die("viewpoint_V3 must be zero: honest fold disagrees with its own written expected/V3.json at %r" % viewpoint_V3_paths)
    viewpoint_V1, viewpoint_V3 = len(viewpoint_V1_paths), len(viewpoint_V3_paths)
    three_histories = 3 - len({q1, q2, q3})

    # ----- P4 twin: silent zero + stale carry-forward -----
    p4 = json.loads(canon(honest[V3]))
    c = p4["positions"]["CCC"]
    c["valuation"] = {"price": 0, "price_seq": 0, "unrealized": -c["total_cost"]}
    p4["unevaluable"] = [u for u in p4["unevaluable"] if u["instrument"] != "CCC"]
    if any(u["instrument"] == "CCC" for u in p4["unevaluable"]):
        die("P4 twin still lists CCC as unevaluable; silent-zero plant failed")
    bbb = p4["positions"]["BBB"]["valuation"]
    ddd = p4["positions"]["DDD"]
    ddd_true_price = honest[V3]["positions"]["DDD"]["valuation"]["price"]
    ddd["valuation"] = {"price": bbb["price"], "price_seq": bbb["price_seq"], "unrealized": ddd["qty"] * bbb["price"] - ddd["total_cost"]}
    # Assert the property in the PLANT'S OWN TERMS, not a leaf-count pin: a
    # leaf count also changes for reasons unrelated to staleness (price_seq
    # alone already makes it nonzero), so a count would let this plant
    # silently weaken to "wrong price_seq, coincidentally-right price" if
    # DDD's own last price ever matched BBB's -- and never notice.
    if p4["positions"]["DDD"]["valuation"]["price"] == ddd_true_price:
        die("P4 stale-carry-forward plant is not actually stale: carried-forward price %d coincides with DDD's own correct price" % p4["positions"]["DDD"]["valuation"]["price"])
    p4["unrealized_pnl"] = sum(p["valuation"]["unrealized"] for p in p4["positions"].values() if p["valuation"])

    # silent_zero / stale_carry_forward measure the SAME sub-condition
    # decomposition the Go gate measures, not a leaf-path footprint. A
    # leaf-path count and the gate's sub-condition count can coincide by
    # accident on one seed (both landed on 2 here before this fix) while
    # measuring genuinely different things -- leaf_diff never inspects
    # `unevaluable` from inside a "silent_zero" walk the way the gate does,
    # and it treats a whole `valuation` dict as one leaf instead of the
    # gate's separate price/price_seq/unrealized sub-checks. Matching the
    # gate's own decomposition means the published count means the same
    # thing on both sides, not just the same number.
    silent_zero_footprint = count_true([
        p4["positions"]["CCC"]["valuation"]["price"] == 0,
        p4["positions"]["CCC"]["valuation"]["price_seq"] == 0,
    ])
    if silent_zero_footprint == 0:
        die("P4 silent-zero twin has no footprint")
    ddd_true_valuation = honest[V3]["positions"]["DDD"]["valuation"]
    stale_carry_footprint = count_true([
        p4["positions"]["DDD"]["valuation"]["price"] != ddd_true_valuation["price"],
        p4["positions"]["DDD"]["valuation"]["price_seq"] != ddd_true_valuation["price_seq"],
        p4["positions"]["DDD"]["valuation"]["unrealized"] != ddd_true_valuation["unrealized"],
    ])
    if stale_carry_footprint == 0:
        die("P4 stale-carry-forward twin has no footprint")
    p4_positions_match = sym_diff_count(p4["positions"].keys(), positions_at["V3"])
    p4_unevaluable_match = sym_diff_count((u["instrument"] for u in p4["unevaluable"]), unevaluable_at["V3"])
    # This one MUST be nonzero, not merely allowed to be: the silent-zero
    # plant removes CCC from `unevaluable` while the manifest still lists it
    # as unevaluable at V3, so the symmetric difference is exactly {"CCC"}.
    # A 0 here would mean the plant failed to diverge from the manifest --
    # the same defect the plant exists to create, measured a second way --
    # so it dies rather than publishing a surprising 0.
    if p4_unevaluable_match == 0:
        die("P4 unevaluable_match_manifest measured 0 -- the silent-zero plant should make CCC's removal visible against the manifest, not agree with it")
    # undeclared_unpriced must be 0 HERE: this twin FABRICATES a zero
    # (valuation is a real dict with price 0, not None), it does not omit a
    # valuation -- so it must not accidentally also trip the biconditional.
    # It is a different failure mode from the second twin below, and
    # publishing 0 here (rather than omitting the key) says "checked and
    # confined", not "not applicable".
    p4_undeclared_unpriced = undeclared_unpriced(p4)
    if p4_undeclared_unpriced != 0:
        die("P4 silent-zero-and-stale-carry twin unexpectedly trips the undeclared_unpriced biconditional: %d" % p4_undeclared_unpriced)
    write_json(P("p4", "twin", "snapshot.json"), p4)

    # ----- P4 second twin: silent omission (undeclared_unpriced) -----
    # The existing twin above tests fabrication (a fake zero, a stale
    # carry-forward) -- neither is a silent OMISSION, so undeclared_unpriced
    # measures 0 on every cell that exists today: an uncredited check is the
    # same problem positions_match_manifest had before the P6 phantom twin.
    # P4's stated claim is fail-closed valuation; silent omission -- a
    # position whose valuation simply stops being produced without being
    # declared unevaluable -- is the purest violation of that specific
    # claim, and the cheapest regression a fold can make. This plants it
    # directly: DDD's valuation is set to null while `unevaluable` is left
    # untouched, so DDD's own real valuation (still correct on every OTHER
    # field) simply vanishes with no declaration.
    p4b = json.loads(canon(honest[V3]))
    if p4b["positions"]["DDD"]["valuation"] is None:
        die("P4 silent-omission twin needs DDD to have a real valuation to omit")
    p4b["positions"]["DDD"]["valuation"] = None
    p4b["unrealized_pnl"] = sum(p["valuation"]["unrealized"] for p in p4b["positions"].values() if p["valuation"])
    p4b_undeclared_unpriced = undeclared_unpriced(p4b)
    # Pinned exactly (1 instrument silently omitted), same discipline as
    # every other exact-count guard in this file: a >0 check would let this
    # plant degrade (omit zero, or omit more than planned) and still ship.
    if p4b_undeclared_unpriced != 1:
        die("P4 silent-omission twin must undeclare exactly one instrument's valuation, got a symmetric difference of %d" % p4b_undeclared_unpriced)
    p4b_positions_match = sym_diff_count(p4b["positions"].keys(), positions_at["V3"])
    p4b_unevaluable_match = sym_diff_count((u["instrument"] for u in p4b["unevaluable"]), unevaluable_at["V3"])
    # Confinement: the only planted defect is DDD's vanished valuation --
    # this twin must not also invent/drop a position or touch `unevaluable`.
    if p4b_positions_match != 0 or p4b_unevaluable_match != 0:
        die("P4 silent-omission twin is not confined: positions_match_manifest=%d, unevaluable_match_manifest=%d, want 0/0" % (p4b_positions_match, p4b_unevaluable_match))
    write_json(P("p4", "twin-silent-omission", "snapshot.json"), p4b)

    # ----- P5 twin: drift -----
    honest_st = statement(honest[V3])
    st = statement(honest[V3])
    aaa = next(h for h in st["holdings"] if h["instrument"] == "AAA")
    drift = max(1, aaa["cost_basis"] // 10000)
    aaa["cost_basis"] += drift
    # leaf_diff cannot see inside `holdings`: it is a LIST, so leaf_diff
    # compares it whole -- a drift of one field and a corruption of every
    # field in every holding both collapse to the same single leaf
    # ['$.holdings'], len 1. Measure pairwise by (instrument, field) instead,
    # so the guard's claim ("exactly one field changed") matches what it
    # actually checks.
    field_mismatch_pairs = holdings_diff(honest_st["holdings"], st["holdings"])
    field_mismatch = len(field_mismatch_pairs)
    if field_mismatch != 1:
        die("P5 drift twin must change exactly one (instrument, field) pair, got %d: %r" % (field_mismatch, field_mismatch_pairs))
    if st["as_of_seq"] != honest_st["as_of_seq"] or st["cash"] != honest_st["cash"]:
        die("P5 drift twin touched as_of_seq or cash")
    # positions_match_manifest / unevaluable_match_manifest deliberately NOT
    # published here: a statement is not a snapshot. `holdings` is a list,
    # not an instrument-keyed map, and there is no `unevaluable` concept in
    # a custodian statement at all -- there is no field to measure it from.
    # Publishing a 0 anyway would claim a check that structurally cannot
    # apply to this document shape; omitting the keys says so honestly.
    write_json(P("p5", "twin", "statement.json"), st)

    # ----- P6 -----
    p6 = build_p6()
    l6, r6 = chain(p6)
    assert_unique_ids(r6, "P6 feed")
    write_lines(P("p6", "feed.jsonl"), l6)
    p6_honest = naive_fold(r6, len(r6))
    p6_check = leaf_diff(P6_GOLDEN, p6_honest)
    if p6_check:
        die("naive fold disagrees with the hand-computed P6 golden at %r" % p6_check)
    write_json(P("p6", "golden.json"), P6_GOLDEN)
    # P6 has no positions_at/unevaluable_at of its own (those are base-feed
    # viewpoint keys), so its "manifest" reference is the honest P6 fold
    # itself, measured here rather than assumed -- P6_GOLDEN declares
    # `positions` but no `unevaluable` key at all, so leaf_diff's golden-
    # keys-only walk above never even inspects doc["unevaluable"]; deriving
    # the expected unevaluable set from the honest fold closes that same
    # blind spot for P6 that positions_at/unevaluable_at closed for the base
    # feed.
    p6_expected_positions = sorted(p6_honest["positions"].keys())
    p6_expected_unevaluable = sorted(u["instrument"] for u in p6_honest["unevaluable"])
    tf = [Ev(e.type, e.id, e.effective, dict(e.payload)) for e in p6]
    tf[1].payload["qty"] = 51
    lf, rf = chain(tf)
    assert_unique_ids(rf, "P6 twin-fill feed")
    p6_fill_fold = naive_fold(rf, len(rf))
    k6a_paths = leaf_diff(P6_GOLDEN, p6_fill_fold)
    k6a = len(k6a_paths)
    # Pinned exactly, same as k6b below: both are single-event mutations on
    # the same 14-line hand-verified feed, so k6a is just as pinnable as k6b
    # -- an unpinned >0 check would let a fill-twin mutation degrade from 7
    # leaves to 1 and still ship silently.
    if k6a != 7:
        die("P6 fill twin footprint must be exactly 7, got %d: %r" % (k6a, k6a_paths))
    p6fill_positions_match = sym_diff_count(p6_fill_fold["positions"].keys(), p6_expected_positions)
    p6fill_unevaluable_match = sym_diff_count((u["instrument"] for u in p6_fill_fold["unevaluable"]), p6_expected_unevaluable)
    write_lines(P("p6", "twin-fill", "feed.jsonl"), lf)
    tp = [Ev(e.type, e.id, e.effective, dict(e.payload)) for e in p6]
    tp[11].payload["price"] = 801
    lp, rp = chain(tp)
    assert_unique_ids(rp, "P6 twin-price feed")
    p6_price_fold = naive_fold(rp, len(rp))
    k6b_paths = leaf_diff(P6_GOLDEN, p6_price_fold)
    k6b = len(k6b_paths)
    if k6b != 3:
        die("P6 price twin footprint must be exactly 3, got %d: %r" % (k6b, k6b_paths))
    p6price_positions_match = sym_diff_count(p6_price_fold["positions"].keys(), p6_expected_positions)
    p6price_unevaluable_match = sym_diff_count((u["instrument"] for u in p6_price_fold["unevaluable"]), p6_expected_unevaluable)
    write_lines(P("p6", "twin-price", "feed.jsonl"), lp)

    # ----- P6 twin: phantom position (fabricates a position with no fill
    # behind it) -----
    # Every twin so far mutates something the FEED actually contains: a
    # price, a qty, an amendment's visibility, a valuation swap. None of
    # them changes the POSITION KEY SET -- measured directly: p1/p3/p4
    # publish positions_match_manifest = 0 and p6-fill/p6-price publish
    # positions_match_golden = 0 (P6 checks against its own hand-verified
    # golden, not the base feed's manifest -- see the key rename below).
    # That is exactly the hole the check was added to close, left
    # uncredited by every twin that could have exercised it. This twin
    # plants that defect on purpose: a document-level mutation (no feed event backs
    # it, deliberately -- a real fill would make the trade real and
    # falsify the premise), following the same pattern P4 already uses to
    # mutate a copy of an honest fold rather than the underlying feed.
    #
    # It belongs here, not bolted onto P1 or P3 (which would corrupt
    # THEIR planted footprints and violate their own confinement checks),
    # and not as a new top-level P-number (no new gate/task was asked
    # for). "Invents a position that never traded" is a portfolio-math
    # fabrication defect -- the fold logic manufacturing a position with
    # no fill behind it -- which is P6's whole territory: it is the only
    # place in this file that validates fold correctness against an
    # independent, hand-verified golden. It is also the sharpest possible
    # demonstration of why positions_match_golden exists: leaf_diff
    # walks GOLDEN's keys only, so an instrument present in the document
    # but absent from golden is invisible to it by construction -- proven
    # below, not assumed.
    p6_phantom = json.loads(canon(p6_honest))
    if "ZZZ" in p6_phantom["positions"]:
        die("P6 phantom-position twin: ZZZ already a real position, pick a different instrument name")
    p6_phantom["positions"]["ZZZ"] = {"qty": 10, "total_cost": 5000, "valuation": {"price": 500, "price_seq": 14, "unrealized": 0}}
    # Confirm the blindness this twin exists to demonstrate, not assume it:
    # leaf_diff must NOT see the invented instrument (it never named "ZZZ"
    # as a golden leaf to compare), which is exactly the property
    # positions_match_manifest exists to catch instead.
    phantom_leaf_diff = leaf_diff(P6_GOLDEN, p6_phantom)
    if phantom_leaf_diff:
        die("P6 phantom-position twin unexpectedly visible to leaf_diff at %r -- the demonstration this twin exists for no longer holds" % phantom_leaf_diff)
    p6phantom_positions_match = sym_diff_count(p6_phantom["positions"].keys(), p6_expected_positions)
    # Pinned exactly (1 invented instrument), same discipline as k6a/k6b:
    # a footprint guard that only checks !=0 would let this plant degrade
    # (e.g. to inventing zero, or accidentally more than one) and still
    # ship silently.
    if p6phantom_positions_match != 1:
        die("P6 phantom-position twin must invent exactly one instrument, got a symmetric difference of %d" % p6phantom_positions_match)
    # Confinement: this twin's only planted defect is the invented
    # position: it must not also drift the unevaluable set.
    p6phantom_unevaluable_match = sym_diff_count((u["instrument"] for u in p6_phantom["unevaluable"]), p6_expected_unevaluable)
    if p6phantom_unevaluable_match != 0:
        die("P6 phantom-position twin is not confined to the position set: unevaluable_match_golden measured %d, want 0" % p6phantom_unevaluable_match)
    write_json(P("p6", "twin-phantom", "snapshot.json"), p6_phantom)

    # ----- manifest -----
    man = {
        "seed": SEED, "instruments": INSTRUMENTS, "end_seq": N,
        "viewpoints": {"V1": V1, "V2": V2, "V3": V3},
        "positions_at": positions_at, "unevaluable_at": unevaluable_at,
        "action": {"action_id": "CA-0001", "instrument": "AAA", "seq": split_seq, "amendment_seq": amend_seq, "original_ratio": 2, "amended_ratio": 3},
        "p1": {"duplicate": {"seq": dup["seq"], "event_id": dup["id"], "of_seq": dup_of_seq, "key": fill_key(dup["payload"])},
               "collision": {"seq": col["seq"], "event_id": col["id"], "of_seq": col_of_seq, "key": fill_key(col["payload"])},
               "twin": {"mutation": "naive_fold_no_dedupe", "mutated_rows": 2,
                        "expected_violations": {"duplicate_absorbed": 1, "collision_refused": 1, "position_after_dedupe": k1,
                                                 "positions_match_manifest": p1_positions_match, "unevaluable_match_manifest": p1_unevaluable_match}}},
        "p2": {"mutated": {"seq": mi + 1}, "reordered": {"seqs": [bi + 1, si + 1]}, "tampered": {"seq": si + 1, "break_at_seq": si + 2},
               "twin": {"mutation": "mutate_reorder_tamper", "mutated_rows": 3,
                        "expected_violations": {"snapshot_hash_diverges_mutated": 1, "snapshot_hash_diverges_reordered": 1, "chain_break_detected": 1}}},
        "p3": {"twin": {"mutation": "leak_amended_terms_at_V2", "mutated_rows": 1,
                        "expected_violations": {"viewpoint_V1": viewpoint_V1, "viewpoint_V2": k3, "viewpoint_V3": viewpoint_V3, "three_histories": three_histories,
                                                 "positions_match_manifest": p3_positions_match, "unevaluable_match_manifest": p3_unevaluable_match}}},
        "p4": {"withheld": WITHHELD, "stale_instrument": "DDD", "stale_from": "BBB",
               "twin": {"mutation": "silent_zero_and_stale_carry_forward", "mutated_rows": 2,
                        "expected_violations": {"silent_zero": silent_zero_footprint, "stale_carry_forward": stale_carry_footprint,
                                                 "positions_match_manifest": p4_positions_match, "unevaluable_match_manifest": p4_unevaluable_match,
                                                 "undeclared_unpriced": p4_undeclared_unpriced}},
               "twin_silent_omission": {"instrument": "DDD", "mutation": "valuation_omitted_without_declaring_unevaluable",
                                         "expected_violations": {"undeclared_unpriced": p4b_undeclared_unpriced,
                                                                  "positions_match_manifest": p4b_positions_match, "unevaluable_match_manifest": p4b_unevaluable_match}}},
        "p5": {"drift": {"instrument": "AAA", "field": "cost_basis", "delta": drift},
               "twin": {"mutation": "cost_basis_drift", "mutated_rows": 1, "expected_violations": {"field_mismatch": field_mismatch}}},
        "p6": {"end_seq": len(r6),
               "twin_fill": {"seq": 2, "mutation": "fill_qty_plus_one",
                             "expected_violations": {"golden_match": k6a, "positions_match_golden": p6fill_positions_match, "unevaluable_match_golden": p6fill_unevaluable_match}},
               "twin_price": {"seq": 12, "mutation": "price_plus_one",
                              "expected_violations": {"golden_match": 3, "positions_match_golden": p6price_positions_match, "unevaluable_match_golden": p6price_unevaluable_match}},
               "twin_phantom": {"instrument": "ZZZ", "mutation": "invented_untraded_position",
                                 "expected_violations": {"positions_match_golden": p6phantom_positions_match, "unevaluable_match_golden": p6phantom_unevaluable_match}}},
    }
    write_json(P("base", "manifest.json"), man)
    print("ok base_end_seq=%d p6_end_seq=%d" % (N, len(r6)))


if __name__ == "__main__":
    main()
