#!/bin/sh
# MERIDIAN Lane 1 gate runner. Every step must pass; nothing here is optional.
set -eu
cd "$(dirname "$0")/.."
PY="${PYTHON:-python}"

echo "== fixtures: deterministic + fresh"
sh fixtures/generate_test.sh

echo "== import-pin (+ negative control)"
"$PY" gates/importpin.py
"$PY" gates/importpin.py --self-test

echo "== proto fresh"
# Regenerate into a scratch dir and byte-compare with the committed files:
# committed generated code must be exactly what the pinned tools produce.
rm -rf .protofresh && mkdir -p .protofresh
go tool buf generate -o .protofresh
gen=$(find .protofresh/api -name '*.pb.go' | sort)
[ -n "$gen" ] || { echo "FAIL proto fresh: buf generate produced no *.pb.go"; exit 1; }
for g in $gen; do
  c="${g#.protofresh/}"
  cmp "$g" "$c" || { echo "FAIL generated $c stale: run go tool buf generate"; exit 1; }
done
for c in $(find api -name '*.pb.go' | sort); do
  [ -f ".protofresh/$c" ] || { echo "FAIL committed $c has no regenerated counterpart: run go tool buf generate"; exit 1; }
done
rm -rf .protofresh
echo "ok proto fresh"

echo "== go vet"
go vet ./...

echo "== build"
mkdir -p bin
BIN="$PWD/bin/meridian"
case "$(uname -s 2>/dev/null || echo unknown)" in MINGW*|MSYS*|CYGWIN*) BIN="$BIN.exe" ;; esac
go build -o "$BIN" ./cmd/meridian

echo "== conformance pack self-test"
# The vendored checker's own negative controls, mutation included, before it
# is trusted over anything (a checker that has only ever said yes proves
# nothing). Node 20+, no dependencies. gates/datum/PIN binds every vendored
# file by sha256 to the governing text's commit; --verify-pin below refuses
# a locally edited pack.
node gates/datum/test.mjs --mutate

echo "== tests + gates"
rm -rf gates/out && mkdir -p gates/out
MERIDIAN_BIN="$BIN" MERIDIAN_VERDICT_DIR="$PWD/gates/out" MERIDIAN_RUNNER="${MERIDIAN_RUNNER:-local}" go test ./... -count=1

echo "== claimability"
# --status: STATUS.md may not mark a property the rows do not support.
# --check: the claimability table in STATUS.md is generated from the rows
# (between markers) and must equal a fresh rendering; status is derived,
# never authored.
"$PY" gates/claimability.py gates/out --status STATUS.md --check STATUS.md

echo "== conformance pack over the rows"
# Two independent derivations of the same table: claimability.py (above) and
# the pack (here). Both must be green, and they must agree on how many
# surfaces are CLAIMABLE; disagreement is a finding, not something to align
# by loosening either.
node gates/datum/check.mjs gates/out --verify-pin
pack_claimable=$(node gates/datum/check.mjs gates/out --json | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).filter(x=>x.status==="CLAIMABLE").length))')
py_claimable=$("$PY" gates/claimability.py gates/out | sed -n 's/^ok lane1 claimable=\([0-9]*\)\/7$/\1/p')
[ -n "$pack_claimable" ] && [ -n "$py_claimable" ] || { echo "FAIL could not read a CLAIMABLE count from both derivations"; exit 1; }
[ "$pack_claimable" = "$py_claimable" ] || { echo "FAIL pack says $pack_claimable CLAIMABLE, claimability.py says $py_claimable"; exit 1; }
echo "ok conformance pack agrees: claimable=$pack_claimable"
