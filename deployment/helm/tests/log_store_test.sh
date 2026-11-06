#!/usr/bin/env bash
# Render checks for the log store (DCS-NFR-SEC-09): retention, TLS, pushes only
# from the collector and queries only from Grafana. Needs helm, python3 with PyYAML.
# Run: bash deployment/helm/tests/log_store_test.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ACK=(--set federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true)
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

render() { helm template dcs "$CHART_DIR" -n dcs "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Prints one field of one rendered object: obj <file> <kind> <name> <python expression on x>
obj() {
  python3 - "$@" <<'EOF'
import sys, yaml
f, kind, name, expr = sys.argv[1:5]
for x in yaml.safe_load_all(open(f)):
    if x and x['kind'] == kind and x['metadata']['name'] == name:
        print(eval(expr))
        break
else:
    sys.exit('no %s %s' % (kind, name))
EOF
}
B=dcs-digital-contracting-service
render "${ACK[@]}" | grep -q 'name: dcs-loki' && fail "log store rendered while logging.enabled is false"
for profile in prod acceptance; do
  render -f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml" >"$TMP/$profile.yaml"
  p="$TMP/$profile.yaml"
  [[ "$(obj "$p" StatefulSet dcs-loki "x['spec']['replicas']")" == 1 ]] || fail "$profile has no Loki"
  cfg="$(obj "$p" ConfigMap dcs-loki "x['data']['loki.yaml']")"
  grep -q 'retention_period: 2160h' <<<"$cfg" || fail "$profile: Loki without retention period"
  grep -q 'retention_enabled: true' <<<"$cfg" || fail "$profile: Loki compactor does not delete expired logs"
  grep -q 'http_listen_address: 127.0.0.1' <<<"$cfg" || fail "$profile: Loki listens outside its pod"
  gw="$(obj "$p" ConfigMap dcs-loki-gateway "x['data']['nginx.conf']")"
  grep -q 'ssl_protocols TLSv1.3;' <<<"$gw" || fail "$profile: Loki gateway without TLS 1.3"
  # The push port takes only pushes, the query port only queries: the delete
  # and every other API fall to the 403 location.
  python3 - "$gw" <<'EOF' || fail "$profile: Loki gateway admits more than push and query"
import re, sys
conf = sys.argv[1]
servers = re.split(r"\n\s*server \{", conf)[1:]
push = next(s for s in servers if "listen 3101" in s)
query = next(s for s in servers if "listen 3100" in s)
assert re.findall(r"location ([^{]+)\{", push) == ["= /loki/api/v1/push ", "/ "], push
assert "return 403" in push.split("location / ")[1]
assert "/loki/api/v1/push" not in query and "delete" not in conf, query
assert "return 403" in query.split("location / ")[1]
EOF
  [[ "$(obj "$p" Service dcs-loki "x['spec']['type']")" == ClusterIP ]] || fail "$profile: Loki exposed outside the cluster"
  [[ "$(obj "$p" NetworkPolicy dcs-loki "[([p['port'] for p in r['ports']], [f.get('podSelector', {}).get('matchLabels', {}).get('app.kubernetes.io/name') for f in r['from']]) for r in x['spec']['ingress']]")" == "[([3101], ['log-collector']), ([3100], ['grafana'])]" ]] \
    || fail "$profile: Loki push or query reachable by other pods"
  ds="$(obj "$p" ConfigMap $B-loki-datasource "x['metadata']['labels']['grafana_datasource'] + ' ' + x['data']['loki-datasource.yaml']")"
  grep -q '^1 ' <<<"$ds" || fail "$profile: data source not labelled for the Grafana sidecar"
  grep -q 'url: https://dcs-loki.dcs.svc:3100' <<<"$ds" || fail "$profile: Grafana reads Loki without TLS"
  obj "$p" Secret dcs-internal-tls "__import__('base64').b64decode(x['data']['ca.crt']).decode().strip()" >"$TMP/pca"
  python3 -c "import sys,yaml; d=yaml.safe_load(sys.stdin.read().split(' ',1)[1]); print(d['datasources'][0]['secureJsonData']['tlsCACert'].strip())" <<<"$ds" >"$TMP/dca"
  cmp -s "$TMP/pca" "$TMP/dca" || fail "$profile: Grafana data source trusts another CA than the server certificate's"
  alloy="$(obj "$p" ConfigMap dcs-log-collector "x['data']['config.alloy']")"
  grep -q 'url = "https://dcs-loki:3101/loki/api/v1/push"' <<<"$alloy" || fail "$profile: collector pushes without TLS"
  grep -q 'names = \["dcs"\]' <<<"$alloy" || fail "$profile: collector not limited to the release namespace"
  [[ "$(obj "$p" RoleBinding dcs-log-collector "x['roleRef']['kind']")" == Role ]] || fail "$profile: collector has cluster-wide rights"
done
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --set logging.retention=720h --show-only charts/logging/templates/loki-config.yaml \
  | grep -q 'retention_period: 720h' || fail "logging.retention is not used"

echo 'log store render tests passed'
