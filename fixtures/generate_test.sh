#!/bin/sh
# Two runs into fresh dirs must be byte-identical, and must equal the checked-in fixtures.
set -eu
cd "$(dirname "$0")/.."
rm -rf fixtures/.regen && mkdir -p fixtures/.regen/a fixtures/.regen/b
python fixtures/generate.py --out fixtures/.regen/a >/dev/null
python fixtures/generate.py --out fixtures/.regen/b >/dev/null
diff -r fixtures/.regen/a fixtures/.regen/b >/dev/null || { echo "FAIL generator not deterministic"; exit 1; }
for d in base p1 p2 p3 p4 p5 p6; do
  diff -r -x snapshot.sha256 "fixtures/.regen/a/$d" "fixtures/$d" >/dev/null || { echo "FAIL checked-in fixtures/$d stale: rerun python fixtures/generate.py"; exit 1; }
done
rm -rf fixtures/.regen
echo "ok fixtures deterministic and fresh"
