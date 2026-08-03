#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../charts/orce" && pwd)"

rendered="$(
  helm template checkpoint-timeout "$CHART_DIR" \
    --set checkpointSink.enabled=true \
    --set-string checkpointSink.url=https://sink.example/checkpoints \
    --set-string checkpointSink.bearerToken=token \
    --set checkpointSink.timeoutSeconds=2 \
    --set checkpointSink.internalTimeoutSeconds=30
)"

grep -q 'name: CHECKPOINT_SINK_TIMEOUT_SECONDS' <<<"$rendered"
grep -A1 'name: CHECKPOINT_SINK_TIMEOUT_SECONDS' <<<"$rendered" | grep -q 'value: "2"'
grep -q 'name: CHECKPOINT_INTERNAL_TIMEOUT_SECONDS' <<<"$rendered"
grep -A1 'name: CHECKPOINT_INTERNAL_TIMEOUT_SECONDS' <<<"$rendered" | grep -q 'value: "30"'

if helm template checkpoint-timeout "$CHART_DIR" \
  --set checkpointSink.enabled=true \
  --set-string checkpointSink.url=https://sink.example/checkpoints \
  --set-string checkpointSink.bearerToken=token \
  --set checkpointSink.internalTimeoutSeconds=0 >/dev/null 2>&1; then
  echo "expected non-positive internal timeout to fail rendering" >&2
  exit 1
fi
