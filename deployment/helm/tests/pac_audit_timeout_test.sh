#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

render() {
  helm template pac-timeout "$CHART_DIR" \
    --set istio.enabled=true \
    --set 'istio.hosts[0]=dcs.example' \
    --set 'istio.gateways[0]=external-gateway' \
    --show-only templates/deployment.yaml \
    --show-only templates/virtualservice.yaml \
    "$@"
}

rendered="$(render)"

grep -A1 'name: PAC_AUDIT_EXECUTOR_TIMEOUT' <<<"$rendered" | grep -q 'value: "10s"'
grep -A1 'name: PAC_AUDIT_EVIDENCE_TIMEOUT' <<<"$rendered" | grep -q 'value: "2m"'

pac_route_line="$(grep -n -m1 '    - name: pac-audit' <<<"$rendered" | cut -d: -f1)"
dcs_route_line="$(grep -n -m1 '    - name: dcs' <<<"$rendered" | cut -d: -f1)"
if ((pac_route_line >= dcs_route_line)); then
  echo "expected the PAC audit route before the DCS catch-all" >&2
  exit 1
fi

pac_route="$(sed -n '/    - name: pac-audit/,/    - name: dcs/p' <<<"$rendered")"
grep -q 'exact: POST' <<<"$pac_route"
grep -q 'exact: /digital-contracting-service/api/pac/audit' <<<"$pac_route"
grep -q 'timeout: "3m"' <<<"$pac_route"

override="$(render \
  --set-string auditExecutor.evidenceTimeout=2m45s \
  --set-string auditExecutor.requestTimeout=4m)"

grep -A1 'name: PAC_AUDIT_EXECUTOR_TIMEOUT' <<<"$override" | grep -q 'value: "10s"'
grep -A1 'name: PAC_AUDIT_EVIDENCE_TIMEOUT' <<<"$override" | grep -q 'value: "2m45s"'
override_pac_route="$(sed -n '/    - name: pac-audit/,/    - name: dcs/p' <<<"$override")"
grep -q 'timeout: "4m"' <<<"$override_pac_route"
