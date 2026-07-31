#!/usr/bin/env bash
set -euo pipefail

: "${KUBECTL_BIN:?KUBECTL_BIN is required}"
: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${SERVICE_NAME:?SERVICE_NAME is required}"
: "${PORT_MAPPING:?PORT_MAPPING is required}"

child_pid=""
cleanup() {
  if [[ -n "${child_pid}" ]]; then
    kill "${child_pid}" >/dev/null 2>&1 || true
    wait "${child_pid}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT TERM INT

# kubectl port-forward selects a backing pod even when the target is a
# Service. A rollout therefore ends that process; restart it until the BDD
# runner terminates this supervisor during cleanup.
while true; do
  "${KUBECTL_BIN}" -n "${K8S_NAMESPACE}" port-forward "svc/${SERVICE_NAME}" "${PORT_MAPPING}" &
  child_pid=$!
  wait "${child_pid}" || true
  child_pid=""
  sleep 1
done
