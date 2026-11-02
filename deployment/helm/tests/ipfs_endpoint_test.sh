#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

render_endpoint() {
  local rendered
  rendered="$(helm template ipfs-endpoint "$CHART_DIR" \
    --show-only templates/deployment.yaml "$@")" || return 1
  if [[ "$(grep -c 'name: IPFS_MFS_BASE_URL$' <<<"$rendered")" -ne 1 ]]; then
    echo 'expected exactly one IPFS_MFS_BASE_URL environment entry' >&2
    return 1
  fi
  awk '
      /- name: IPFS_MFS_BASE_URL$/ { found=1; print; next }
      found && /- name:/ { exit }
      found { print }
    ' <<<"$rendered"
}

assert_url() {
  local expected="$1"
  shift
  local endpoint
  endpoint="$(render_endpoint "$@")"
  [[ "$(grep -c 'name: IPFS_MFS_BASE_URL$' <<<"$endpoint")" -eq 1 ]]
  grep -Fqx "              value: \"$expected\"" <<<"$endpoint"
  ! grep -q 'valueFrom:' <<<"$endpoint"
}

for enabled in true false; do
  assert_url 'https://kubo.example/rpc' \
    --set "ipfs.enabled=$enabled" \
    --set-string ipfs.mfsBaseURL=https://kubo.example/rpc

  for url in '' 'https://ignored.example/rpc'; do
    endpoint="$(render_endpoint \
      --set "ipfs.enabled=$enabled" \
      --set-string "ipfs.mfsBaseURL=$url" \
      --set-string ipfs.mfsBaseURLSecretRef.name=kubo-credentials \
      --set-string ipfs.mfsBaseURLSecretRef.key=RPC_ENDPOINT)"
    [[ "$(grep -c 'name: IPFS_MFS_BASE_URL$' <<<"$endpoint")" -eq 1 ]]
    [[ "$(grep -c 'valueFrom:' <<<"$endpoint")" -eq 1 ]]
    grep -q 'secretKeyRef:' <<<"$endpoint"
    grep -q 'name: "kubo-credentials"' <<<"$endpoint"
    grep -q 'key: "RPC_ENDPOINT"' <<<"$endpoint"
    ! grep -q 'value:' <<<"$endpoint"
  done
done

# With in-cluster TLS (default) the backend calls the TLS proxy of the IPFS pod.
assert_url 'https://ipfs-endpoint-ipfs:8443' --set ipfs.enabled=true
assert_url 'http://ipfs-endpoint-ipfs:5001' --set ipfs.enabled=true --set global.internalTLS.enabled=false
assert_url 'http://ipfs-endpoint-ipfs:5101' \
  --set ipfs.enabled=true --set ipfs.service.apiPort=5101 --set global.internalTLS.enabled=false
assert_url '' --set ipfs.enabled=false --set replicaCount=0

if render_endpoint \
  --set-string ipfs.mfsBaseURLSecretRef.name=kubo-credentials \
  --set-string ipfs.mfsBaseURLSecretRef.key= >/dev/null 2>&1; then
  echo 'expected a named IPFS Secret with an empty key to fail rendering' >&2
  exit 1
fi

echo 'IPFS endpoint tests passed'
