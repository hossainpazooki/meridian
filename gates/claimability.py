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
"""
import argparse
import glob
import json
import os
import re
import sys

PROPS = [1, 2, 3, 4, 5, 6]


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
    # EVERY twin row must independently check out RED-as-planted.
    return (
        len(live) == 1
        and live[0]["result"] == "GREEN"
        and len(twins) >= 1
        and all(twin_ok(t) for t in twins)
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


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("verdict_dir")
    ap.add_argument("--status")
    a = ap.parse_args()
    rows = load(a.verdict_dir)
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
        print("P%d   | %-5s | %-18s | %s" % (p, c["live"][0]["result"], tw, "YES" if ok else "NO"))
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
    print("ok lane1 claimable=%d/6" % k)
    return 0


if __name__ == "__main__":
    sys.exit(main())
