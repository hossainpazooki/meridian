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

echo "== go vet"
go vet ./...

echo "== build"
mkdir -p bin
BIN="$PWD/bin/meridian"
case "$(uname -s 2>/dev/null || echo unknown)" in MINGW*|MSYS*|CYGWIN*) BIN="$BIN.exe" ;; esac
go build -o "$BIN" ./cmd/meridian

echo "== tests + gates"
rm -rf gates/out && mkdir -p gates/out
MERIDIAN_BIN="$BIN" MERIDIAN_VERDICT_DIR="$PWD/gates/out" MERIDIAN_RUNNER="${MERIDIAN_RUNNER:-local}" go test ./... -count=1

echo "== claimability"
"$PY" gates/claimability.py gates/out --status STATUS.md
