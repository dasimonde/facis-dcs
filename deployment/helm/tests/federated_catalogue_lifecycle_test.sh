#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$CHART_DIR/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "federated catalogue lifecycle test: $*" >&2
  exit 1
}

command -v helm >/dev/null || fail "helm is required"

lock_before="$(sha256sum "$CHART_DIR/Chart.lock")"
helm dependency build --skip-refresh "$CHART_DIR" >/dev/null
lock_after="$(sha256sum "$CHART_DIR/Chart.lock")"
[[ "$lock_before" == "$lock_after" ]] ||
  fail "helm dependency build changed the authoritative Chart.lock"

helm template lifecycle "$CHART_DIR" \
  -f "$CHART_DIR/values.bdd.yml" \
  --set fcservice.portal.enabled=true >"$TMP_DIR/enabled-a.yaml"
helm template lifecycle "$CHART_DIR" \
  -f "$CHART_DIR/values.bdd.yml" \
  --set fcservice.portal.enabled=true >"$TMP_DIR/enabled-b.yaml"
cmp "$TMP_DIR/enabled-a.yaml" "$TMP_DIR/enabled-b.yaml" ||
  fail "identical locked inputs rendered different manifests"

deployment_image() {
  local workload="$1"
  awk -v RS='---' -v name="$workload" '
    $0 ~ /kind: Deployment/ && $0 ~ ("name: " name "([[:space:]]|$)") {
      if (match($0, /image: "[^"]+"/)) {
        image=substr($0, RSTART + 8, RLENGTH - 9)
        print image
        exit
      }
    }
  ' "$TMP_DIR/enabled-a.yaml"
}

document_images() {
  local kind="$1"
  local workload="$2"
  awk -v RS='---' -v kind="$kind" -v name="$workload" '
    $0 ~ ("kind: " kind "([[:space:]]|$)") && $0 ~ ("name: " name "([[:space:]]|$)") {
      text=$0
      while (match(text, /image: "[^"]+"/)) {
        print substr(text, RSTART + 8, RLENGTH - 9)
        text=substr(text, RSTART + RLENGTH)
      }
      exit
    }
  ' "$TMP_DIR/enabled-a.yaml"
}

for workload in fc-service fc-fuseki fc-keycloak fc-postgres; do
  image="$(deployment_image "$workload")"
  [[ "$image" == *@sha256:* ]] ||
    fail "$workload image is not digest pinned: ${image:-missing}"
done

portal_image="$(deployment_image fc-demo-portal)"
if [[ -n "$portal_image" && "$portal_image" != *@sha256:* ]]; then
  fail "fc-demo-portal image is not digest pinned: $portal_image"
fi

mapfile -t provision_images < <(
  document_images Job lifecycle-digital-contracting-service-fc-realm-provision
)
[[ "${#provision_images[@]}" -eq 2 ]] ||
  fail "realm provision Job must render one readiness and one provision image"
for image in "${provision_images[@]}"; do
  [[ "$image" == alpine/k8s:1.33.4@sha256:* ]] ||
    fail "realm provision Job image is not the immutable provisioning image: $image"
done

for init_name in fc-realm-job-created fc-realm-ready; do
  backend_init_image="$(awk -v name="$init_name" '
    $1 == "-" && $2 == "name:" && $3 == name { found=1; next }
    found && $1 == "image:" { gsub(/"/, "", $2); print $2; exit }
  ' "$TMP_DIR/enabled-a.yaml")"
  [[ "$backend_init_image" == alpine/k8s:1.33.4@sha256:* ]] ||
    fail "backend $init_name image is not digest pinned: ${backend_init_image:-missing}"
done

grep -q -- '--for=condition=available' "$TMP_DIR/enabled-a.yaml" ||
  fail "realm provisioning does not use native Keycloak readiness"

realm_job_name="$(awk -v RS='---' '
  /kind: Job/ && /app.kubernetes.io\/component: fc-realm-provision/ {
    if (match($0, /metadata:[[:space:]]+name: [^[:space:]]+/)) {
      value=substr($0, RSTART, RLENGTH)
      sub(/^.*name: /, "", value)
      print value
      exit
    }
  }
' "$TMP_DIR/enabled-a.yaml")"
expected_realm_job="lifecycle-digital-contracting-service-fc-realm-provision"
[[ "$realm_job_name" == "$expected_realm_job" ]] ||
  fail "rendered hook Job name differs from the backend lifecycle target: ${realm_job_name:-missing}"

created_line="$(grep -n -m1 -- '- name: fc-realm-job-created' "$TMP_DIR/enabled-a.yaml" | cut -d: -f1)"
complete_line="$(grep -n -m1 -- '- name: fc-realm-ready' "$TMP_DIR/enabled-a.yaml" | cut -d: -f1)"
[[ -n "$created_line" && -n "$complete_line" && "$created_line" -lt "$complete_line" ]] ||
  fail "backend lifecycle init containers do not render in create-then-complete order"

created_gate="$(awk '
  /- name: fc-realm-job-created/ { found=1 }
  found { print }
  found && /job\/lifecycle-digital-contracting-service-fc-realm-provision/ { exit }
' "$TMP_DIR/enabled-a.yaml")"
complete_gate="$(awk '
  /- name: fc-realm-ready/ { found=1 }
  found && /^      containers:/ { exit }
  found { print }
' "$TMP_DIR/enabled-a.yaml")"
[[ "$(grep -c -- '--for=create' <<<"$created_gate")" -eq 1 ]] &&
  ! grep -q -- '--for=condition=complete' <<<"$created_gate" ||
  fail "first backend lifecycle gate must contain only the Job creation watch"
[[ "$(grep -c -- '--for=condition=complete --timeout=-1s' <<<"$complete_gate")" -eq 1 ]] &&
  [[ "$(grep -c -- '--for=condition=failed --timeout=-1s' <<<"$complete_gate")" -eq 1 ]] ||
  fail "terminal backend lifecycle gate does not start both native unbounded condition watches"
grep -q 'wait -n -p finished_pid' <<<"$complete_gate" ||
  fail "terminal backend lifecycle gate does not identify the first terminal condition"
grep -q 'kill "$other_pid"' <<<"$complete_gate" &&
  grep -q 'wait "$other_pid"' <<<"$complete_gate" ||
  fail "terminal backend lifecycle gate does not terminate and reap the losing watch"
if grep -Eq -- 'sleep|--watch-only|--resource-version|coproc|while ' <<<"$complete_gate"; then
  fail "terminal backend lifecycle gate contains polling or the unsupported raw watch mechanism"
fi
grep -q 'realm provisioning Job .* failed:' <<<"$complete_gate" ||
  fail "terminal backend lifecycle gate has no explicit Failed diagnostic"
grep -q 'exit 1' <<<"$complete_gate" ||
  fail "terminal backend lifecycle gate does not fail the init container for a failed Job"
[[ "$(grep -c -- "job/$expected_realm_job" <<<"$created_gate")" -eq 1 ]] &&
  [[ "$(grep -c -- "job=\"$expected_realm_job\"" <<<"$complete_gate")" -eq 1 ]] &&
  [[ "$(grep -cF '"job/$job"' <<<"$complete_gate")" -eq 3 ]] ||
  fail "backend lifecycle gates do not target the exact rendered hook Job"

command -v timeout >/dev/null || fail "timeout is required for lifecycle shell behavior tests"
terminal_gate_script="$TMP_DIR/fc-realm-ready.sh"
awk '
  /- name: fc-realm-ready/ { container=1 }
  container && /^            - \|$/ { script=1; next }
  script && /^      containers:/ { exit }
  script {
    sub(/^              /, "")
    print
  }
' "$TMP_DIR/enabled-a.yaml" >"$terminal_gate_script"
grep -q 'wait -n -p finished_pid' "$terminal_gate_script" ||
  fail "could not extract the rendered terminal lifecycle script"

fake_bin="$TMP_DIR/fake-bin"
mkdir -p "$fake_bin"
ln -s "$CHART_DIR/tests/fake-fc-lifecycle-kubectl" "$fake_bin/kubectl"

run_terminal_gate_case() {
  local name="$1"
  local initial="$2"
  local terminal="$3"
  local expected_status="$4"
  local case_dir="$TMP_DIR/$name"
  mkdir -p "$case_dir"
  mkfifo "$case_dir/block"
  : >"$case_dir/state"
  : >"$case_dir/kubectl.log"
  : >"$case_dir/pids"

  set +e
  timeout 5s env \
    PATH="$fake_bin:$PATH" \
    FC_FAKE_INITIAL="$initial" \
    FC_FAKE_TERMINAL="$terminal" \
    FC_FAKE_STATE="$case_dir/state" \
    FC_FAKE_KUBECTL_LOG="$case_dir/kubectl.log" \
    FC_FAKE_PID_LOG="$case_dir/pids" \
    FC_FAKE_BLOCK_FIFO="$case_dir/block" \
    bash "$terminal_gate_script" >"$case_dir/stdout" 2>"$case_dir/stderr"
  local status=$?
  set -e

  [[ "$status" -eq "$expected_status" ]] ||
    fail "$name terminal gate exited $status, expected $expected_status: $(<"$case_dir/stderr")"
  # The rendered script is checked above for both background wait commands.
  # At runtime the winning wait may finish before the scheduler lets the
  # losing fake reach its first log statement, so only require the winner to
  # have run and verify cleanup for every process that actually did start.
  [[ -s "$case_dir/pids" ]] ||
    fail "$name terminal condition winner did not start"
  while read -r pid _; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      fail "$name leaked kubectl wait process $pid"
    fi
  done <"$case_dir/pids"
}

run_terminal_gate_case already-complete Complete none 0

run_terminal_gate_case complete-event nonterminal Complete 0

run_terminal_gate_case failed-event nonterminal Failed 1
grep -q 'reason=BackoffLimitExceeded; message=realm import failed' \
  "$TMP_DIR/failed-event/stderr" ||
  fail "Failed event test did not emit the Job condition diagnosis"

backend_doc="$(awk '
  /^# Source: digital-contracting-service\/templates\/deployment.yaml$/ { found=1 }
  found && /^---$/ { exit }
  found { print }
' "$TMP_DIR/enabled-a.yaml")"
grep -q 'readinessProbe:' <<<"$backend_doc" &&
  grep -q 'path: /readyz' <<<"$backend_doc" &&
  grep -q 'port: http' <<<"$backend_doc" ||
  fail "backend Deployment is not gated by the application /readyz endpoint"

statuslist_proxy_doc="$(awk '
  /- name: statuslist-localhost-proxy$/ { found=1; print; next }
  found && /^        - name:/ { exit }
  found { print }
' <<<"$backend_doc")"
grep -q 'readinessProbe:' <<<"$statuslist_proxy_doc" &&
  grep -q 'httpGet:' <<<"$statuslist_proxy_doc" &&
  grep -q 'path: "/statuslist/v1/tenants/c2pa/status/1"' <<<"$statuslist_proxy_doc" &&
  grep -q 'port: sl-proxy' <<<"$statuslist_proxy_doc" ||
  fail "BDD status-list proxy readiness does not verify the required seeded list route"
if grep -q 'tcpSocket:' <<<"$statuslist_proxy_doc"; then
  fail "status-list proxy still reports ready from TCP reachability alone"
fi

helm template proxy-default "$CHART_DIR" \
  --set statusListLocalhostProxy.enabled=true \
  --set statusListLocalhostProxy.upstream=http://status-list-service:8080 \
  >"$TMP_DIR/proxy-default.yaml"
default_proxy_doc="$(awk '
  /- name: statuslist-localhost-proxy$/ { found=1; print; next }
  found && /^        - name:/ { exit }
  found { print }
' "$TMP_DIR/proxy-default.yaml")"
grep -q 'path: "/statuslist/health"' <<<"$default_proxy_doc" ||
  fail "status-list proxy default readiness path is not its functional health route"

# The backend needs the PKCS#11 material produced by this post-install hook
# before /readyz can become successful. The supported deploy path therefore
# must let Helm execute hooks before it waits for application readiness.
grep -q '"helm.sh/hook": post-install,post-upgrade' \
  "$CHART_DIR/templates/hsm-provision-job.yaml" ||
  fail "HSM provisioning no longer has the expected post-install hook ordering"
if grep -Eq '^[[:space:]]*[^#].*--wait([=[:space:]]|$)' "$CHART_DIR/deploy.sh"; then
  fail "deploy.sh uses helm --wait and deadlocks HSM post-install provisioning against /readyz"
fi

fc_service_doc="$(awk -v RS='---' '
  /kind: Deployment/ && /name: fc-service([[:space:]]|$)/ { print; exit }
' "$TMP_DIR/enabled-a.yaml")"
fc_cpu_request="$(awk '
  $1 == "requests:" { section="requests"; next }
  $1 == "limits:" { section="limits"; next }
  section == "requests" && $1 == "cpu:" { gsub(/"/, "", $2); print $2; exit }
' <<<"$fc_service_doc")"
fc_cpu_limit="$(awk '
  $1 == "requests:" { section="requests"; next }
  $1 == "limits:" { section="limits"; next }
  section == "limits" && $1 == "cpu:" { gsub(/"/, "", $2); print $2; exit }
' <<<"$fc_service_doc")"
cpu_millicores() {
  local quantity="$1"
  if [[ "$quantity" == *m ]]; then
    printf '%s\n' "${quantity%m}"
  else
    awk -v cores="$quantity" 'BEGIN { printf "%.0f\n", cores * 1000 }'
  fi
}
[[ -n "$fc_cpu_request" && "$(cpu_millicores "$fc_cpu_request")" -ge 500 ]] ||
  fail "fc-service CPU request is below the semantic verification floor: ${fc_cpu_request:-missing}"
[[ -n "$fc_cpu_limit" && "$(cpu_millicores "$fc_cpu_limit")" -ge 2000 ]] ||
  fail "fc-service CPU limit is below the semantic verification floor: ${fc_cpu_limit:-missing}"
grep -q 'readinessProbe:' <<<"$fc_service_doc" ||
  fail "fc-service has no native readiness probe"
if grep -Eq 'startupProbe:|livenessProbe:' <<<"$fc_service_doc"; then
  fail "fc-service cold start is still bounded by a restart-triggering probe"
fi
if awk -v RS='---' '
  /kind: Job/ && /fc-realm-provision/ && /(sleep [0-9]|for i in|until |while )/ { found=1 }
  END { exit !found }
' "$TMP_DIR/enabled-a.yaml"; then
  fail "realm provisioning retains polling, retry, or artificial sleep"
fi

sed '/^[[:space:]]*#/d' "$TMP_DIR/enabled-a.yaml" >"$TMP_DIR/enabled-runtime.yaml"
if grep -Eqi 'neo4j|n10s(\.graphconfig\.show)?' "$TMP_DIR/enabled-runtime.yaml"; then
  # The explicit false Spring health switch is evidence of removal.
  offenders="$(grep -Ei 'neo4j|n10s(\.graphconfig\.show)?' "$TMP_DIR/enabled-runtime.yaml" |
    grep -Eiv 'MANAGEMENT_HEALTH_NEO4J_ENABLED|value: "false"' || true)"
  [[ -z "$offenders" ]] || fail "rendered runtime still references Neo4j/n10s: $offenders"
fi

helm template lifecycle "$CHART_DIR" \
  -f "$CHART_DIR/values.dev.yml" >"$TMP_DIR/disabled.yaml"
if grep -q 'name: FEDERATED_CATALOGUE_API_URL' "$TMP_DIR/disabled.yaml"; then
  fail "disabled deployment still configures the backend FC startup gate"
fi

if helm template lifecycle "$CHART_DIR" \
  -f "$CHART_DIR/values.acceptance.yml" >/dev/null 2>"$TMP_DIR/remote-error"; then
  fail "remote ADMIN_ALL catalogue rendered without explicit ADR-18 acknowledgement"
fi
grep -q 'acknowledgeAdminAllTrustBoundary' "$TMP_DIR/remote-error" ||
  fail "remote FC rejection does not explain the required ADR-18 acknowledgement"
helm template lifecycle "$CHART_DIR" \
  -f "$CHART_DIR/values.acceptance.yml" \
  --set federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true \
  >"$TMP_DIR/remote-acknowledged.yaml"

if rg -n 'helm dependency update' \
  "$REPO_ROOT/dev-stack.sh" "$REPO_ROOT/dev-stack2.sh" "$CHART_DIR/deploy.sh" \
  "$CHART_DIR/values.dev.yml" "$CHART_DIR/values.dev2.yml" >/dev/null; then
  fail "an executable/documented development path bypasses Chart.lock"
fi
grep -q 'helm dependency build --skip-refresh "$HELM_CHART_PATH"' \
  "$REPO_ROOT/dev-stack2.sh" ||
  fail "second development entrypoint does not build dependencies from Chart.lock"
if rg -n 'Waiting for Federated Catalogue|app.kubernetes.io/name=federated-catalogue' \
  "$REPO_ROOT/dev-stack2.sh" >/dev/null; then
  fail "FC-disabled second development entrypoint still performs a blanket FC wait"
fi
if rg -n 'SyncWithRetry|FC_SCHEMA_SYNC_(MAX_WAIT|RETRY_INTERVAL)' \
  "$REPO_ROOT/backend" "$REPO_ROOT/dev-stack.sh" "$CHART_DIR" \
  --glob '!federated_catalogue_lifecycle_test.sh' >/dev/null; then
  fail "obsolete FC retry/warm-up configuration remains"
fi

echo "federated catalogue lifecycle tests passed"
