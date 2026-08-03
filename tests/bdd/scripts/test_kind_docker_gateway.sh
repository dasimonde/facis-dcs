#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
resolver="${script_dir}/kind_docker_gateway.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

mkdir -p "${tmp_dir}/bin"
cat >"${tmp_dir}/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$1 $2" in
  "inspect dcs-bdd-control-plane")
    printf '%s\n' bridge kind
    ;;
  "inspect custom-control-plane")
    printf '%s\n' custom-kind
    ;;
  "network inspect")
    case "$3" in
      kind) printf '%s\n' 172.18.0.1 ;;
      custom-kind) printf '%s\n' 172.29.0.1 ;;
      bridge) printf '%s\n' 172.17.0.1 ;;
      *) exit 1 ;;
    esac
    ;;
  *) exit 1 ;;
esac
EOF
chmod +x "${tmp_dir}/bin/docker"

actual="$(PATH="${tmp_dir}/bin:${PATH}" bash "${resolver}" dcs-bdd)"
[[ "${actual}" == "172.18.0.1" ]] || {
  echo "expected standard kind gateway 172.18.0.1, got ${actual}" >&2
  exit 1
}

actual="$(PATH="${tmp_dir}/bin:${PATH}" bash "${resolver}" custom)"
[[ "${actual}" == "172.29.0.1" ]] || {
  echo "expected custom kind gateway 172.29.0.1, got ${actual}" >&2
  exit 1
}

echo "kind Docker gateway resolver tests passed"
