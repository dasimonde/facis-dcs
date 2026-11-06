#!/usr/bin/env bash
# Runs every chart test in this directory. Run: bash deployment/helm/tests/run_all.sh
#
# helm dependency build (deploy.sh, dev-stack.sh, the BDD Makefile) packs each
# local subchart into charts/<name>-<version>.tgz. Helm then loads the copy and
# the directory, and a stale copy can hide an edit of the directory. The copies
# are removed first; the remote dependencies stay.
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$(cd "$TESTS_DIR/.." && pwd)"

for dir in "$CHART_DIR"/charts/*/; do
  rm -f "$CHART_DIR/charts/$(basename "$dir")"-*.tgz
done

failed=0
for t in "$TESTS_DIR"/*_test.sh; do
  echo "== $(basename "$t")"
  bash "$t" || failed=1
done
exit "$failed"
