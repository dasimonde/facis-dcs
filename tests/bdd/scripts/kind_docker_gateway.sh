#!/usr/bin/env bash
set -euo pipefail

cluster_name="${1:?usage: kind_docker_gateway.sh KIND_CLUSTER_NAME}"
control_plane="${cluster_name}-control-plane"

# Discover the network from the node rather than assuming its name. Standard
# kind uses "kind"; this also supports clusters attached to a custom network.
mapfile -t attached_networks < <(
  docker inspect "${control_plane}" \
    --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{"\n"}}{{end}}'
)

ordered_networks=()
for network in "${attached_networks[@]}"; do
  [[ "${network}" == "kind" ]] && ordered_networks+=("${network}")
done
for network in "${attached_networks[@]}"; do
  [[ -n "${network}" && "${network}" != "kind" ]] && ordered_networks+=("${network}")
done

for network in "${ordered_networks[@]}"; do
  gateway="$({
    docker network inspect "${network}" \
      --format '{{range .IPAM.Config}}{{if .Gateway}}{{.Gateway}}{{"\n"}}{{end}}{{end}}'
  } | awk 'NF { print; exit }')"
  if [[ -n "${gateway}" ]]; then
    printf '%s\n' "${gateway}"
    exit 0
  fi
done

exit 1
