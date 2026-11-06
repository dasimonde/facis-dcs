#!/usr/bin/env bash
# Render checks for the archive notary and the PAC executors: the production
# and acceptance profiles address the bundled ORCE, and a production-like
# release without ORCE does not render unless each URL is set (DCS-FR-CSA-03).
# Needs helm, python3 with PyYAML.
# Run: bash deployment/helm/tests/archive_notary_test.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

render() { helm template dcs "$CHART_DIR" -n dcs "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

check_env() {
  python3 -c '
import sys, yaml
want = dict(a.split("=", 1) for a in sys.argv[1:])
for d in yaml.safe_load_all(sys.stdin):
    if d and d["kind"] == "Deployment" and d["metadata"]["name"] == "dcs-digital-contracting-service":
        env = {e["name"]: e for e in d["spec"]["template"]["spec"]["containers"][0]["env"]}
        for name, value in want.items():
            assert env[name].get("value") == value, (name, env[name])
        assert env["ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN"]["valueFrom"]["secretKeyRef"]["key"] == "ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN"
        sys.exit(0)
sys.exit("backend Deployment not rendered")
' "$@"
}

for profile in prod acceptance; do
  values=(-f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml")
  render "${values[@]}" | check_env \
    ORCE_ARCHIVE_NOTARY_URL=https://dcs-orce:8443/archive/notary \
    PAC_AUDIT_EXECUTOR_URL=https://dcs-orce:8443/audit/run \
    PAC_WORKFLOW_GATE_EXECUTOR_URL=https://dcs-orce:8443/workflow-gate/run \
    || fail "$profile: the backend does not address the bundled ORCE"
  out="$(render "${values[@]}" --set orce.enabled=false 2>&1 || true)"
  for key in archiveNotary.baseURL auditExecutor.url workflowGateExecutor.url; do
    grep -q "orce.enabled or $key" <<<"$out" || fail "$profile renders without ORCE and without $key: $out"
  done
  render "${values[@]}" --set orce.enabled=false \
    --set archiveNotary.baseURL=https://notary.example.org \
    --set auditExecutor.url=https://executor.example.org/audit/run \
    --set workflowGateExecutor.url=https://executor.example.org/workflow-gate/run >/dev/null \
    || fail "$profile: external notary and executors are refused"
done

echo 'Archive notary render tests passed'
