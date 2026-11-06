#!/usr/bin/env bash
# Runs the unit tests of the ORCE flow functions (charts/orce/flows*/*.test.js).
# Needs node (node:test) and openssl.
# Run: bash deployment/helm/tests/orce_flow_test.sh
set -euo pipefail

ORCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../charts/orce" && pwd)"
node --test "$ORCE_DIR"/flows*/*.test.js
