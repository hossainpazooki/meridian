#!/usr/bin/env python3
"""Claimability table from GATE_VERDICT rows, and a STATUS.md overclaim check.

This is the SECOND, independent enforcement of the crediting rule that the Go
harness (gates/verdict.go, func Emit) already applies when it writes a row.
It must not merely trust r["result"], which Emit already set: it re-derives
whether each twin row is RED-as-planted directly from r["checks"] vs
r["planted"]["expected_violations"], the same way Emit itself compares them
(dict equality over the union of keys). If this ever disagrees with what a
row's own "result" field claims, that is a real finding, not something to
paper over.

Crediting is per check. A property is CLAIMABLE only when its one live row is
GREEN, every twin row is RED as planted, and every check key the live row
reports is set nonzero by at least one of those twins. A live check that no
such twin sets nonzero has never been seen to fail, so its 0 in the live row
is not evidence: the property is not claimable, and the check is named as
unfalsified. An UNEVALUABLE row, the live one included, makes the property
UNEVALUABLE with no unfalsified list: a property that could not be evaluated
has reported no check to credit. The vendored conformance pack derives the
same table independently; gates/run.sh compares the two property by property
(--agree) and fails on any difference.

--self-test plants rows in memory and requires every crediting case, and
every kind of disagreement --agree exists to report, to be caught (a
derivation that has only ever said yes proves nothing).
"""
import argparse
import glob
import json
import os
import re
import sys

PROPS = [1, 2, 3, 4, 5, 6, 7]
LANE = 1

# Property names for the generated STATUS.md table. Names only; every other
# cell of that table is derived from the rows (status is never authored).
PROP_NAMES = {
    1: "At-most-once fill ingestion",
    2: "Deterministic replay, byte-identical snapshot",
    3: "PIT-correct corporate actions (incl. amendment)",
    4: "Fail-closed valuation",
    5: "Reconciliation proven able to fail",
    6: "Portfolio math (average cost, P&L)",
    7: "Wire fidelity of the gRPC read API",
}
BLOCK_BEGIN = "<!-- meridian:claimability:begin -->"
BLOCK_END = "<!-- meridian:claimability:end -->"


def surface(prop):
    return "meridian-lane1-p%d" % prop


def derive(cell):
    """(live_word, twin_word, status_word, unfalsified) for one property, from rows only.

    Vocabulary shared with the vendored conformance pack:
    UNEVALUABLE when any row is UNEVALUABLE, the live one included (no
    unfalsified list), or when the row set cannot be trusted (duplicate rows
    from concurrent writers); CLAIMABLE when claimable() holds; PARTIAL when
    exactly one cell kind is present, or when the live row is GREEN and every
    twin RED as planted but a live check is unfalsified; UNCLAIMED when there
    are no rows, or when the rows ran and did not qualify (a live row not
    GREEN, a twin not RED as planted), still naming any unfalsified check.
    """
    live, twins = cell["live"], cell["twin"]
    if len(live) > 1 or any(len(g) > 1 for g in _by_mutation(twins).values()):
        return ("-", "-", "UNEVALUABLE", [])
    live_word = live[0]["result"] if live else "-"
    twin_word = "RED" if twins and all(twin_ok(t) for t in twins) else (",".join(t["result"] for t in twins) or "-")
    if not live and not twins:
        return (live_word, twin_word, "UNCLAIMED", [])
    if unevaluable(cell):
        return (live_word, twin_word, "UNEVALUABLE", [])
    if claimable(cell):
        return (live_word, twin_word, "CLAIMABLE", [])
    unf = unfalsified(cell)
    if not live or not twins or (live[0]["result"] == "GREEN" and all(twin_ok(t) for t in twins)):
        return (live_word, twin_word, "PARTIAL", unf)
    return (live_word, twin_word, "UNCLAIMED", unf)


def unevaluable(cell):
    """True when any row of the property, the live one included, is UNEVALUABLE."""
    return any(r["result"] == "UNEVALUABLE" for r in cell["live"] + cell["twin"])


def unfalsified(cell):
    """Sorted live check keys that no twin RED as planted sets nonzero.

    Empty when there is no single live row to credit, or when the property is
    UNEVALUABLE (it reported no check, so none is named). A twin that went
    RED for the wrong reason (twin_ok false) covers nothing, whatever its
    counts: a check it sets nonzero has not been seen to fail as planted.
    """
    live = cell["live"]
    if len(live) != 1 or unevaluable(cell):
        return []
    as_planted = [t for t in cell["twin"] if twin_ok(t)]
    return sorted(k for k in live[0]["checks"] if not any(t["checks"].get(k, 0) > 0 for t in as_planted))


def _by_mutation(twins):
    out = {}
    for t in twins:
        out.setdefault(t.get("planted", {}).get("mutation", "<no-mutation>"), []).append(t)
    return out


def render(rows):
    """The generated block for STATUS.md: markers, one line of provenance, the table."""
    lines = [
        BLOCK_BEGIN,
        "generated by `python gates/claimability.py gates/out --render` from the rows of one run; `gates/run.sh` compares it with `--check STATUS.md`",
        "",
        "| # | Property | Live | Twin | Status | Unfalsified |",
        "|---|---|---|---|---|---|",
    ]
    for p in PROPS:
        live_word, twin_word, status, unf = derive(rows[p])
        n = len(rows[p]["twin"])
        name = PROP_NAMES[p] + (" (%d twins)" % n if n > 1 else "")
        unf_cell = ", ".join("`%s`" % k for k in unf) or "-"
        lines.append("| P%d | %s | %s | %s | %s | %s |" % (p, name, live_word, twin_word, status, unf_cell))
    lines.append(BLOCK_END)
    return "\n".join(lines)


def to_json(rows):
    """[{surface, lane, status, unfalsified}] per property: the shape --agree compares."""
    out = []
    for p in PROPS:
        _, _, status, unf = derive(rows[p])
        out.append({"surface": surface(p), "lane": LANE, "status": status, "unfalsified": unf})
    return out


def agree(pack, mine):
    """Differences between the pack's --json and this script's --json; [] when they agree.

    Keyed by (surface, lane). Every property on either side must be present
    on both, with the same status and the same unfalsified checks. The pack
    leaves "unfalsified" out when it names none; an absent list reads as
    empty, on either side. An empty side is a difference, not agreement.
    """
    sides = []
    for name, xs in (("pack", pack), ("claimability.py", mine)):
        if not isinstance(xs, list) or not all(isinstance(x, dict) for x in xs):
            return ["%s: not a JSON array of objects" % name]
        if not xs:
            return ["%s: no properties" % name]
        by = {}
        for x in xs:
            k = (x.get("surface"), x.get("lane"))
            u = x.get("unfalsified", [])
            if not isinstance(u, list) or not all(isinstance(s, str) for s in u):
                return ["%s: %s lane%s: unfalsified is not a list of check names" % (name, k[0], k[1])]
            if k in by:
                return ["%s: %s lane%s listed twice" % (name, k[0], k[1])]
            by[k] = (x.get("status"), sorted(u))
        sides.append(by)
    a, b = sides
    diffs = []
    for k in sorted(set(a) | set(b), key=lambda k: (str(k[0]), str(k[1]))):
        where = "%s lane%s" % k
        if k not in a:
            diffs.append("%s: absent from the pack" % where)
        elif k not in b:
            diffs.append("%s: absent from claimability.py" % where)
        elif a[k] != b[k]:
            diffs.append("%s: pack %s unfalsified=%s, claimability.py %s unfalsified=%s"
                         % (where, a[k][0], a[k][1], b[k][0], b[k][1]))
    return diffs


def agree_main(pack_path, mine_path):
    """0 printing the CLAIMABLE count when both --json outputs agree; 1 printing both sides when not; 2 when one is unreadable."""
    loaded = []
    for name, path in (("pack", pack_path), ("claimability.py", mine_path)):
        try:
            with open(path, encoding="utf-8") as fh:
                text = fh.read()
            loaded.append((text, json.loads(text)))
        except (OSError, ValueError) as e:
            print("FAIL agreement: %s output %s is not readable JSON (%s)" % (name, path, e))
            return 2
    (pack_text, pack), (mine_text, mine) = loaded
    diffs = agree(pack, mine)
    if diffs:
        print("FAIL conformance pack and claimability.py disagree")
        for d in diffs:
            print("  " + d)
        print("--- pack --json (%s)" % pack_path)
        print(pack_text.rstrip("\n"))
        print("--- claimability.py --json (%s)" % mine_path)
        print(mine_text.rstrip("\n"))
        return 1
    print("ok conformance pack agrees: claimable=%d" % sum(1 for x in pack if x["status"] == "CLAIMABLE"))
    return 0


def check_block(path, fresh):
    """0 when STATUS.md's generated block equals a fresh rendering, else 1 with both printed."""
    with open(path, encoding="utf-8") as fh:
        text = fh.read().replace("\r\n", "\n")
    a, b = text.find(BLOCK_BEGIN), text.find(BLOCK_END)
    if a == -1 or b == -1 or b < a:
        print("FAIL STATUS.md has no generated claimability block (markers missing or out of order)")
        return 1
    current = text[a:b + len(BLOCK_END)]
    if current != fresh:
        print("FAIL STATUS.md generated claimability block differs from a fresh rendering")
        print("--- STATUS.md\n" + current + "\n--- fresh\n" + fresh)
        return 1
    print("ok STATUS.md generated claimability block is fresh")
    return 0


def load(dirpath):
    rows = {p: {"live": [], "twin": []} for p in PROPS}
    for f in sorted(glob.glob(os.path.join(dirpath, "meridian-lane1-p*-*.json"))):
        with open(f) as fh:
            r = json.load(fh)
        m = re.match(r"meridian-lane1-p(\d)", r["surface"])
        rows[int(m.group(1))][r["cell"]].append(r)
    return rows


def twin_ok(r):
    # Independently re-derive RED-as-planted from the row's own contents:
    # checks must equal planted.expected_violations over the union of keys
    # (plain dict equality does that), and at least one check must actually
    # be nonzero (a twin whose checks are all-zero never went RED, no matter
    # what "result" claims). r["result"] == "RED" is included as a
    # cross-check against Emit's own label, not as the source of truth.
    return (
        r["result"] == "RED"
        and r["checks"] == r["planted"]["expected_violations"]
        and any(v > 0 for v in r["checks"].values())
    )


def claimable(cell):
    live = cell["live"]
    twins = cell["twin"]
    # Exactly one live row, GREEN; one or more twin rows (P6 emits three:
    # twin_fill, twin_price, twin_phantom -- distinguished by scope/planted,
    # not by cell, since Emit only ever writes cell "live" or "twin"), and
    # EVERY twin row must independently check out RED-as-planted. And per
    # check: every key the live row reports must be set nonzero by at least
    # one of those twins (unfalsified() empty), or the live row's 0 on that
    # check has never been shown able to be anything else.
    return (
        len(live) == 1
        and live[0]["result"] == "GREEN"
        and len(twins) >= 1
        and all(twin_ok(t) for t in twins)
        and not unfalsified(cell)
    )


def duplicate_msgs(prop, cell):
    # A row that measured something twice is not a stronger claim than a row
    # that measured it once -- it is a sign that two writers hit the same
    # MERIDIAN_VERDICT_DIR (a concurrent `go test`/run.sh, most likely from
    # another agent/process sharing this working tree). Left undetected, the
    # extra rows silently break claimable()'s len(live) == 1 check and every
    # affected property reads as an ordinary NO -- indistinguishable from a
    # real regression in the gate itself. Name the condition instead.
    #
    # "live" has exactly one legitimate row per property (there is only one
    # live cell), so any count above 1 is a duplicate by construction -- no
    # further key is needed. "twin" can legitimately hold more than one row
    # (P6 emits three: twin_fill, twin_price, twin_phantom), all sharing the
    # same surface and cell, so cell alone cannot distinguish a genuine
    # second twin from a duplicated one. planted.mutation is the field that
    # actually varies across P6's three twins (verified against the emitted
    # rows and fixtures/base/manifest.json: fill_qty_plus_one,
    # price_plus_one, invented_untraded_position -- one distinct mutation
    # string per twin, for every property, not just P6) and Emit requires
    # Planted on every twin row, so it is always present. Grouping twin rows
    # by mutation and flagging any group of size > 1 catches a duplicated
    # twin without needing to know in advance how many twins a property
    # should have.
    surface = "meridian-lane1-p%d" % prop
    msgs = []
    if len(cell["live"]) > 1:
        msgs.append("FAIL duplicate verdict rows for %s live: %d (concurrent runs writing the same MERIDIAN_VERDICT_DIR?)"
                     % (surface, len(cell["live"])))
    by_mutation = {}
    for t in cell["twin"]:
        mut = t.get("planted", {}).get("mutation", "<no-mutation>")
        by_mutation.setdefault(mut, []).append(t)
    for mut, trows in sorted(by_mutation.items()):
        if len(trows) > 1:
            msgs.append("FAIL duplicate verdict rows for %s twin mutation=%s: %d (concurrent runs writing the same MERIDIAN_VERDICT_DIR?)"
                         % (surface, mut, len(trows)))
    return msgs


def status_cells(path):
    """Return {prop: (live_word, twin_word, status_text)} from the Lane 1 table in STATUS.md."""
    out = {}
    with open(path) as fh:
        for line in fh:
            m = re.match(r"\|\s*P(\d)\s*\|[^|]*\|\s*([A-Z]+)\s*\|\s*([A-Z]+)\s*\|\s*([^|]+?)\s*\|", line)
            if m:
                out[int(m.group(1))] = (m.group(2), m.group(3), m.group(4))
    return out


def self_test():
    """Planted cases, in memory. 0 when every one is caught, else 1 naming each miss."""
    def row(cell, result, checks, planted=None, mutation="m"):
        r = {"surface": surface(1), "lane": LANE, "cell": cell, "result": result, "checks": dict(checks)}
        if cell == "twin":
            r["planted"] = {"mutation": mutation, "expected_violations": dict(checks if planted is None else planted)}
        return r

    live_ab = row("live", "GREEN", {"a": 0, "b": 0})
    twin_a = row("twin", "RED", {"a": 1, "b": 0}, mutation="plant_a")
    twin_b = row("twin", "RED", {"a": 0, "b": 2}, mutation="plant_b")
    uncovered = {"live": [row("live", "GREEN", {"a": 0, "b": 0, "c": 0})], "twin": [twin_a, twin_b]}
    crediting = [
        # (case, cell, status, unfalsified, claimable)
        ("all keys covered -> claimable",
         {"live": [live_ab], "twin": [twin_a, twin_b]}, "CLAIMABLE", [], True),
        ("one key uncovered -> not claimable, naming it",
         uncovered, "PARTIAL", ["c"], False),
        ("a key set nonzero only by a wrong-reason twin -> not covered",
         {"live": [live_ab], "twin": [twin_a, row("twin", "RED", {"a": 0, "b": 1}, planted={"a": 0, "b": 2}, mutation="plant_b")]},
         "UNCLAIMED", ["b"], False),
        ("UNEVALUABLE live -> UNEVALUABLE, no unfalsified list",
         {"live": [row("live", "UNEVALUABLE", {"a": 0, "b": 0})], "twin": [twin_a, twin_b]}, "UNEVALUABLE", [], False),
        ("UNEVALUABLE twin beside a GREEN live -> UNEVALUABLE, no unfalsified list",
         {"live": [live_ab], "twin": [twin_a, row("twin", "UNEVALUABLE", {"a": 0, "b": 0}, mutation="plant_b")]},
         "UNEVALUABLE", [], False),
        ("a live with no twin -> PARTIAL, every live check unfalsified",
         {"live": [live_ab], "twin": []}, "PARTIAL", ["a", "b"], False),
    ]
    misses = []
    for case, cell, want_status, want_unf, want_claimable in crediting:
        _, _, status, unf = derive(cell)
        if status != want_status:
            misses.append("%s: status %s, want %s" % (case, status, want_status))
        if unf != want_unf:
            misses.append("%s: unfalsified %s, want %s" % (case, unf, want_unf))
        if claimable(cell) != want_claimable:
            misses.append("%s: claimable %s, want %s" % (case, claimable(cell), want_claimable))

    rows = {p: {"live": [], "twin": []} for p in PROPS}
    rows[1] = uncovered
    p1 = [l for l in render(rows).split("\n") if l.startswith("| P1 |")]
    if len(p1) != 1 or not p1[0].endswith("| PARTIAL | `c` |"):
        misses.append("render: the P1 line does not carry PARTIAL and its unfalsified check: %r" % p1)
    if to_json(rows)[0] != {"surface": surface(1), "lane": LANE, "status": "PARTIAL", "unfalsified": ["c"]}:
        misses.append("--json: the P1 element is %r" % (to_json(rows)[0],))

    pack = [{"surface": surface(1), "lane": 1, "status": "PARTIAL", "live": "GREEN", "twins": 2, "unfalsified": ["c"]},
            {"surface": surface(2), "lane": 1, "status": "CLAIMABLE", "live": "GREEN", "twins": 1}]
    mine = [{"surface": surface(1), "lane": 1, "status": "PARTIAL", "unfalsified": ["c"]},
            {"surface": surface(2), "lane": 1, "status": "CLAIMABLE", "unfalsified": []}]
    agreement = [
        # (case, pack side, claimability.py side, substring a difference must carry; None = must agree)
        ("identical tables agree, an absent unfalsified reading as empty", pack, mine, None),
        ("a status differs", pack, [mine[0], dict(mine[1], status="PARTIAL")],
         "%s lane1: pack CLAIMABLE unfalsified=[], claimability.py PARTIAL" % surface(2)),
        ("an unfalsified list differs", pack, [dict(mine[0], unfalsified=[]), mine[1]],
         "%s lane1: pack PARTIAL unfalsified=['c'], claimability.py PARTIAL unfalsified=[]" % surface(1)),
        ("a property absent from claimability.py", pack, mine[:1], "%s lane1: absent from claimability.py" % surface(2)),
        ("a property absent from the pack", pack[:1], mine, "%s lane1: absent from the pack" % surface(2)),
        ("an empty side", [], mine, "pack: no properties"),
    ]
    for case, a, b, want in agreement:
        got = agree(a, b)
        if want is None and got:
            misses.append("agree, %s: reported %r" % (case, got))
        elif want is not None and not any(want in d for d in got):
            misses.append("agree, %s: not caught, got %r" % (case, got))

    if misses:
        for m in misses:
            print("FAIL claimability self-test: %s" % m)
        return 1
    print("ok claimability self-test (all planted cases caught)")
    return 0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("verdict_dir", nargs="?")
    ap.add_argument("--status", help="STATUS.md to check for overclaims against the rows")
    ap.add_argument("--render", action="store_true", help="print the generated STATUS.md claimability block and exit")
    ap.add_argument("--check", metavar="STATUS_MD", help="compare STATUS.md's generated block with a fresh rendering")
    ap.add_argument("--json", action="store_true", help="print [{surface, lane, status, unfalsified}] per property and exit")
    ap.add_argument("--agree", nargs=2, metavar=("PACK_JSON", "CLAIMABILITY_JSON"),
                    help="compare the pack's --json with this script's --json property by property; exit 1 printing both sides on any difference")
    ap.add_argument("--self-test", action="store_true", help="run the planted crediting and agreement cases and exit")
    a = ap.parse_args()
    if a.self_test or a.agree:
        if a.verdict_dir or a.status or a.render or a.check or a.json or (a.self_test and a.agree):
            ap.error("--self-test and --agree each run alone")
        return self_test() if a.self_test else agree_main(*a.agree)
    if not a.verdict_dir:
        ap.error("a verdict directory is required")
    rows = load(a.verdict_dir)
    if a.render:
        print(render(rows))
        return 0
    if a.json:
        print(json.dumps(to_json(rows), indent=2))
        return 0
    if a.check:
        rc = check_block(a.check, render(rows))
        if rc:
            return rc
    k, bad = 0, False
    dup_msgs = []
    print("prop | live  | twin(s)            | claimable")
    for p in PROPS:
        c = rows[p]
        dups = duplicate_msgs(p, c)
        if dups:
            # Named and fatal, distinct from both an ordinary NO and a
            # missing-rows NO: this property's row count does not match
            # what a single honest run could have produced, so nothing
            # about it (including whether it "looks" claimable) can be
            # trusted until the duplicate is explained.
            dup_msgs.extend(dups)
            live_word = c["live"][0]["result"] if len(c["live"]) == 1 else "-"
            print("P%d   | %-5s | %-18s | NO (duplicate rows)" % (p, live_word, "-"))
            bad = True
            continue
        if not c["live"] or not c["twin"]:
            # Distinct from "ran but did not qualify": a missing gate must
            # never read the same as "not claimable yet". This is fatal.
            live_word = c["live"][0]["result"] if c["live"] else "-"
            print("P%d   | %-5s | %-18s | NO (missing rows)" % (p, live_word, "-"))
            bad = True
            continue
        ok = claimable(c)
        k += ok
        tw = ",".join(("RED*" if twin_ok(t) else t["result"]) for t in c["twin"])
        unf = derive(c)[3]
        verdict = "YES" if ok else ("NO (unfalsified: %s)" % ", ".join(unf) if unf else "NO")
        print("P%d   | %-5s | %-18s | %s" % (p, c["live"][0]["result"], tw, verdict))
    for m in dup_msgs:
        print(m)
    if a.status:
        st = status_cells(a.status)
        for p in PROPS:
            marked = p in st and st[p][2].upper().startswith("CLAIMABLE")
            # A property with duplicate rows is never "supported" here even
            # if claimable() would technically pass it (claimable() checks
            # "every twin present is RED-as-planted", not "no more rows than
            # expected" -- a twin duplicated with itself can pass that check
            # individually while still being a duplicate). Keeps this block
            # consistent with the FAIL already emitted for it above.
            supported = (bool(rows[p]["live"] and rows[p]["twin"])
                         and not duplicate_msgs(p, rows[p])
                         and claimable(rows[p]))
            if marked and not supported:
                print("FAIL STATUS.md overclaims P%d" % p)
                bad = True
            elif supported and not marked:
                print("WARN P%d is supported by verdicts but STATUS.md does not mark it CLAIMABLE" % p)
    if bad:
        return 1
    print("ok lane1 claimable=%d/7" % k)
    return 0


if __name__ == "__main__":
    sys.exit(main())
