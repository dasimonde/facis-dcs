#!/usr/bin/env bash
# Failure dump of a kind stack: the state of every pod in every namespace and
# the logs of each of its containers (init containers included). For a
# container that restarted, the log of the previous run follows, which holds
# the reason of a crash loop.
# Run: KUBECONFIG=tests/bdd/.tmp/kind-kubeconfig bash tests/bdd/scripts/dump_cluster_logs.sh
set -uo pipefail

KUBECTL_BIN="${KUBECTL_BIN:-kubectl}"
TAIL="${DUMP_LOG_TAIL:-500}"

"$KUBECTL_BIN" get pods -A -o wide
"$KUBECTL_BIN" get pods -A -o 'custom-columns=NAMESPACE:.metadata.namespace,POD:.metadata.name,CONTAINER:.status.containerStatuses[*].name,READY:.status.containerStatuses[*].ready,RESTARTS:.status.containerStatuses[*].restartCount'

# One line per container: namespace pod container restarts
"$KUBECTL_BIN" get pods -A -o json | python3 -c '
import json, sys
for pod in json.load(sys.stdin)["items"]:
    status = pod.get("status", {})
    for c in status.get("initContainerStatuses", []) + status.get("containerStatuses", []):
        print(pod["metadata"]["namespace"], pod["metadata"]["name"], c["name"], c.get("restartCount", 0))
' |
  while read -r ns pod container restarts; do
    echo "===== logs ${ns}/${pod} -c ${container} (restarts: ${restarts}) ====="
    "$KUBECTL_BIN" logs -n "$ns" "$pod" -c "$container" --tail="$TAIL" 2>&1
    if [[ "$restarts" != 0 ]]; then
      echo "===== previous logs ${ns}/${pod} -c ${container} ====="
      "$KUBECTL_BIN" logs -n "$ns" "$pod" -c "$container" --tail="$TAIL" --previous 2>&1
    fi
  done
exit 0
