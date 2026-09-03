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

echo "== tests + gates"
rm -rf gates/out && mkdir -p gates/out
MERIDIAN_BIN="$BIN" MERIDIAN_VERDICT_DIR="$PWD/gates/out" MERIDIAN_RUNNER="${MERIDIAN_RUNNER:-local}" go test ./... -count=1

echo "== claimability"
"$PY" gates/claimability.py gates/out --status STATUS.md
