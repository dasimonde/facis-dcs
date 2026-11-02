#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  if [[ -f .tmp/port-forward-db.pid ]]; then
    kill "$(cat .tmp/port-forward-db.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-dcs.pid ]]; then
    kill "$(cat .tmp/port-forward-dcs.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-metrics.pid ]]; then
    kill "$(cat .tmp/port-forward-metrics.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-orce.pid ]]; then
    kill "$(cat .tmp/port-forward-orce.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-dss.pid ]]; then
    kill "$(cat .tmp/port-forward-dss.pid)" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

: "${VENV_PATH:?VENV_PATH is required}"
: "${FEATURES_PATH:?FEATURES_PATH is required}"
: "${KUBECTL_BIN:?KUBECTL_BIN is required}"
: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${DCS_DEPLOYMENT:?DCS_DEPLOYMENT is required}"
: "${BDD_DCS_BASE_URL:?BDD_DCS_BASE_URL is required}"
: "${PROJECT_ROOT:?PROJECT_ROOT is required}"
# Scopes every pod/label lookup below to THIS release: the two-instance BDD
# suite deploys a second DCS release (dcs2) into the SAME namespace
# (tests/bdd/Makefile's kind_deploy_b), and app.kubernetes.io/component=backend
# alone matches both releases' backend pods — an unscoped selector previously
# caused a wrong-pod log dump / signing-exec pick here.
: "${HELM_RELEASE:?HELM_RELEASE is required}"

BDD_PUBLIC_ORIGIN="${BDD_PUBLIC_ORIGIN:-http://localhost:18080}"
export BDD_PUBLIC_ORIGIN
# The status list every BDD credential names, served and signed by this
# release's ORCE issuer (ADR-34). It has to be the URL the BACKEND fetches,
# because the verifier requires the token's sub to equal the credential's URI —
# the ingress origin is reachable from both the host and the cluster.
export ISSUER_BASE_URL="${ISSUER_BASE_URL:-${BDD_PUBLIC_ORIGIN}/issuer}"

# BDD_DCS_BASE_URL_A / _B: the two-instance (@two-instance) peer-trust
# scenarios (steps/peer_trust/dcs_peer_trust_steps.py) address instance A and
# instance B independently of the single-instance BDD_DCS_BASE_URL used by
# every other scenario. Instance A is conventionally "the" default instance
# in this Helm/kind harness, so _A is just an alias for BDD_DCS_BASE_URL;
# _B defaults to the dcs2 release's public origin (values.bdd2.yml).
export BDD_DCS_BASE_URL_A="$BDD_DCS_BASE_URL"
export BDD_DCS_BASE_URL_B="${BDD_DCS_BASE_URL_B:-http://dcs-b.localhost:18080/digital-contracting-service/api}"

# Resolve a Running pod's name by label, waiting for one to appear: the one-shot
# get can race a rollout (a rollout-restart transiently has the old pod
# Terminating and the new one not yet phase=Running, i.e. zero Running matches),
# and jsonpath '{.items[0]...}' errors on an empty list under `set -e`.
wait_for_running_pod() {
  local ns="$1" selector="$2" name deadline
  deadline=$(( $(date +%s) + 120 ))
  while [[ "$(date +%s)" -lt "$deadline" ]]; do
    name="$("${KUBECTL_BIN}" -n "$ns" get pod -l "$selector" \
      --field-selector=status.phase=Running \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    if [[ -n "$name" ]]; then
      echo "$name"
      return 0
    fi
    sleep 3
  done
  echo "run_bdd_helm: timed out waiting for a Running pod matching [$selector] in namespace $ns" >&2
  return 1
}

# IPFS CID-swap tamper seam (steps/support/tamper_seam.py): several
# verify-shaped endpoints always re-fetch the SERVER'S OWN stored PDF from
# IPFS by CID, so tampered-artifact scenarios inject bytes as a NEW CID via
# `ipfs add` exec'd inside the shared IPFS pod, then repoint the owning row's
# CID column at it (via the existing context.db test-DB connection). The
# exec prefix must select the pod serving the tested contract's IPFS endpoint.
mkdir -p .tmp .reports/junit
REPORTS_JUNIT_DIR="$PWD/.reports/junit"
# A previous local run must not satisfy the fail-closed scenario-count gate
# below. Behave writes one report per feature, so remove only its generated
# JUnit XML artifacts before starting the selected suite.
# The load run keeps them: it follows the suite in the same CI job.
if [[ "${RUN_MODE:-bdd}" != "load" ]]; then
  find "$REPORTS_JUNIT_DIR" -maxdepth 1 -type f -name '*.xml' -delete
fi

# Emits `--resolve <host>:<port>:127.0.0.1` for a URL's host[:port], so
# *.localhost hostnames the host machine's own resolver may not know (e.g.
# dcs-b.localhost, which nothing registers anywhere) resolve to loopback for
# curl's own DNS resolution without /etc/hosts or sudo (USER CONSTRAINT: no
# /etc/hosts writes, no sudo anywhere in this harness). This is independent
# of environment.py's socket.getaddrinfo fallback, which only covers the
# Python behave process, not shell-level curl calls like the ones below.
resolve_args_for_url() {
  local url="$1" hostport host port
  hostport="${url#*://}"
  hostport="${hostport%%/*}"
  host="${hostport%%:*}"
  if [[ "$hostport" == *:* ]]; then
    port="${hostport##*:}"
  else
    case "$url" in
      https://*) port=443 ;;
      *) port=80 ;;
    esac
  fi
  printf '%s\n' "--resolve" "${host}:${port}:127.0.0.1"
}

DCS_HEALTH_URL="${BDD_DCS_BASE_URL%/}/auth/login"

verify_host_ingress() {
  local body http_code
  body=$(curl -s --max-time 10 -X POST "$DCS_HEALTH_URL" -H 'Content-Type: application/json' -d '{}' 2>/dev/null || true)
  http_code=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" -X POST "$DCS_HEALTH_URL" \
    -H 'Content-Type: application/json' -d '{}' 2>/dev/null || echo "000")
  if [[ "$body" == "404 page not found" ]] || [[ "$http_code" == "404" && "$body" == *"page not found"* ]]; then
    echo "Host port 18080 is not reaching the kind Traefik ingress (got Go default 404)."
    echo "Ensure kind exposes port 18080 and the BDD stack is deployed: make -C tests/bdd kind_up"
    return 1
  fi
  return 0
}

wait_for_dcs_http() {
  local http_code
  http_code=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" -X POST "$DCS_HEALTH_URL" \
    -H 'Content-Type: application/json' -d '{}' 2>/dev/null) || {
    echo "DCS pod is Ready but HTTP is unreachable at $DCS_HEALTH_URL"
    verify_host_ingress || true
    "$KUBECTL_BIN" get pods -n kube-system -l app.kubernetes.io/name=traefik -o wide || true
    return 1
  }
  if [[ ! "$http_code" =~ ^[24][0-9]{2}$ ]] || [[ "$http_code" == 404 ]]; then
    echo "DCS pod is Ready but HTTP returned $http_code at $DCS_HEALTH_URL"
    verify_host_ingress || true
    return 1
  fi
}

diagnose_fc_startup() {
  echo "Federated Catalogue startup diagnosis:" >&2
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" get deployment,job,pod \
    -l "app.kubernetes.io/instance=${HELM_RELEASE}" -o wide >&2 || true
  for workload in fc-service fc-fuseki fc-keycloak fc-postgres; do
    "$KUBECTL_BIN" -n "$K8S_NAMESPACE" describe "deployment/$workload" >&2 || true
    "$KUBECTL_BIN" -n "$K8S_NAMESPACE" logs "deployment/$workload" \
      --all-containers --tail=100 >&2 || true
  done
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" logs "deployment/$DCS_DEPLOYMENT" \
    --all-containers --tail=200 >&2 || true
}

echo "Waiting for DCS deployment ($DCS_DEPLOYMENT) to be available"
if ! "$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$DCS_DEPLOYMENT"; then
  diagnose_fc_startup
  exit 1
fi
echo "Waiting for DCS backend pod to accept traffic"
if ! "$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=ready pod \
  -l "app.kubernetes.io/name=digital-contracting-service,app.kubernetes.io/component=backend,app.kubernetes.io/instance=${HELM_RELEASE}" \
  --timeout=180s; then
  diagnose_fc_startup
  exit 1
fi

echo "Waiting for DCS HTTP via Traefik ingress at $DCS_HEALTH_URL ..."
if ! verify_host_ingress; then
  diagnose_fc_startup
  exit 1
fi
if ! wait_for_dcs_http; then
  diagnose_fc_startup
  exit 1
fi
echo "DCS is reachable at $DCS_HEALTH_URL"

# Resolve test seams only after the deployment startup gate is green. Doing
# this earlier hid terminal FC failures behind a generic "no Running pod"
# timeout and delayed the actual workload diagnostics.
# Sign did:web challenges through the in-cluster token: the Helm/kind harness
# has no local SoftHSM token (keys are non-extractable, PKCS#11-only). Exec
# through the deployment, not a pod name: the deployment selector holds the
# backend component label, and kubectl resolves the pod at each call, so a DCS
# restart (NFR-SF-01 scenario) does not leave a stale pod name here.
export BDD_HSMSIGN_EXEC="${KUBECTL_BIN} -n ${K8S_NAMESPACE} exec deployment/${DCS_DEPLOYMENT} -c digital-contracting-service --"

IPFS_POD="$(wait_for_running_pod "${K8S_NAMESPACE}" \
  "app.kubernetes.io/name=ipfs,app.kubernetes.io/instance=dcs")"
# -i/--stdin is required (not just harmless) here: `ipfs add -` reads its
# content from stdin, and without --stdin the API server may not have a
# stdin stream attached before the remote command starts reading — observed
# in practice as an intermittent race where `ipfs add` silently succeeds
# against an EMPTY stdin (producing the well-known empty-file CID
# Qmb...4Q7Vs-style hash) instead of the intended bytes, rather than a
# reliable failure.
# The Kubo RPC API requires its bearer token (charts/ipfs, api.auth). The
# wrapper drops the leading "ipfs" of the step's command and runs
# "ipfs --api-auth bearer:$IPFS_API_TOKEN <rest>" in the container.
export BDD_IPFS_EXEC="${KUBECTL_BIN} -n ${K8S_NAMESPACE} exec -i ${IPFS_POD} -c ipfs -- sh -c 'shift; exec ipfs --api-auth \"bearer:\$IPFS_API_TOKEN\" \"\$@\"' ipfs-auth"

# Instance B (dcs2, features/17_peer_trust @two-instance): only checked when
# the caller tells us it exists (DCS_DEPLOYMENT_B set AND actually present in
# this namespace) — never silently skipped without saying so, since a
# missing/unready instance B means every @two-instance scenario will fail
# with a much less obvious error later.
if [[ -n "${DCS_DEPLOYMENT_B:-}" ]] && "$KUBECTL_BIN" -n "$K8S_NAMESPACE" get "deployment/$DCS_DEPLOYMENT_B" >/dev/null 2>&1; then
  echo "Waiting for DCS deployment B ($DCS_DEPLOYMENT_B) to be available"
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=300s "deployment/$DCS_DEPLOYMENT_B"

  BDD_PUBLIC_ORIGIN_B="${BDD_PUBLIC_ORIGIN_B:-http://dcs-b.localhost:18080}"
  DCS_HEALTH_URL_B="${BDD_DCS_BASE_URL_B%/}/auth/login"
  mapfile -t CURL_RESOLVE_B < <(resolve_args_for_url "$BDD_PUBLIC_ORIGIN_B")

  echo "Waiting for DCS HTTP via Traefik ingress (instance B) at $DCS_HEALTH_URL_B ..."
  deadline_b=$(( $(date +%s) + 120 ))
  http_code_b=""
  until http_code_b=$(curl -s --max-time 10 "${CURL_RESOLVE_B[@]}" -o /dev/null -w "%{http_code}" -X POST "$DCS_HEALTH_URL_B" \
      -H 'Content-Type: application/json' -d '{}' 2>/dev/null) \
      && [[ "$http_code_b" =~ ^[24][0-9]{2}$ ]] && [[ "$http_code_b" != 404 ]]; do
    if [ "$(date +%s)" -gt "$deadline_b" ]; then
      echo "WARNING: timed out waiting for instance B's DCS HTTP on $DCS_HEALTH_URL_B — @two-instance scenarios will fail." >&2
      break
    fi
    sleep 2
  done
  if [[ "$http_code_b" =~ ^[24][0-9]{2}$ ]] && [[ "$http_code_b" != 404 ]]; then
    echo "DCS instance B is reachable at $DCS_HEALTH_URL_B"
  fi
else
  echo "WARNING: DCS_DEPLOYMENT_B is not set or not present in namespace $K8S_NAMESPACE — instance B" >&2
  echo "readiness was NOT verified. @two-instance BDD scenarios will fail if they run. Deploy it with" >&2
  echo "'make -C tests/bdd kind_deploy_b' (or kind_up, which now includes it) if you need instance B." >&2
fi

# NetworkPolicy enforcement (DCS-NFR-SEC-03, DCS-NFR-SF-02, DCS-NFR-SEC-14).
# The default CNI of kind v0.24 and later (kindnetd with kube-network-policies)
# enforces the chart's policies, so the suite runs with them in force. The BDD
# values name the Traefik ingress (networkPolicy.ingressFrom), so the backend
# API admits the ingress, the pods of the release and the other BDD release. A
# pod with the release label reaches the backend API and is dropped at the IPFS
# RPC API, which admits only the backend; a pod without it is dropped at the
# backend API too. The API server proxy, which the load test uses to read
# /metrics, still reaches the backend: the CNI does not filter traffic that
# root processes on the node start (API server, kubelet probes, port-forwards).
echo "Checking NetworkPolicy enforcement"
ipfs_rpc_port="$("$KUBECTL_BIN" -n "$K8S_NAMESPACE" get service "${HELM_RELEASE}-ipfs" \
  -o jsonpath='{.spec.ports[?(@.name=="https")].port}')"
# The probe runs detached and its log is read after it ends: "kubectl run -i"
# attaches after the container starts and loses output written before that.
np_probe() { # np_probe <name> [labels]
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" run "$1" --image=curlimages/curl:8.11.1 ${2:+--labels="$2"} \
    --restart=Never --command -- sh -c '
      curl -s -o /dev/null --connect-timeout 5 --max-time 10 -w "backend=%{http_code}\n" "http://$1:$2/"; echo "backend_exit=$?"
      curl -sk -o /dev/null --connect-timeout 5 "https://$3:$4/api/v0/version"; echo "ipfs_exit=$?"' \
    probe "${DCS_SERVICE:-$DCS_DEPLOYMENT}" "${SERVICE_PORT:-8991}" "${HELM_RELEASE}-ipfs" "$ipfs_rpc_port" >/dev/null 2>&1 || true
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded "pod/$1" --timeout=120s >/dev/null 2>&1 || true
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" logs "pod/$1" 2>&1 || true
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" delete pod "$1" --wait=false >/dev/null 2>&1 || true
}
probe_out="$(np_probe "np-probe-$$" "app.kubernetes.io/instance=${HELM_RELEASE}")"
if ! grep -Eq '^backend=[1-5][0-9]{2}$' <<<"$probe_out"; then
  echo "NetworkPolicy check: a pod of the release did not reach the backend API, so the check proves nothing:" >&2
  echo "$probe_out" >&2
  exit 1
fi
# curl exit 28: the connection attempt timed out, the policy dropped it.
if ! grep -qx 'ipfs_exit=28' <<<"$probe_out"; then
  echo "NetworkPolicy check: a pod without the backend labels was not dropped at the IPFS RPC API." >&2
  echo "The cluster does not enforce NetworkPolicy (kind >= v0.24 required, see tests/bdd/kind-config.yaml):" >&2
  echo "$probe_out" >&2
  exit 1
fi
outsider_out="$(np_probe "np-outsider-$$")"
if ! grep -qx 'backend_exit=28' <<<"$outsider_out"; then
  echo "NetworkPolicy check: a pod outside the release and the ingress reached the backend API (networkPolicy.ingressFrom):" >&2
  echo "$outsider_out" >&2
  exit 1
fi
probe_backend_pod="$(wait_for_running_pod "$K8S_NAMESPACE" \
  "app.kubernetes.io/component=backend,app.kubernetes.io/instance=${HELM_RELEASE}")"
if ! "$KUBECTL_BIN" get --raw \
    "/api/v1/namespaces/${K8S_NAMESPACE}/pods/${probe_backend_pod}:9464/proxy/metrics" >/dev/null; then
  echo "NetworkPolicy check: the API server proxy to /metrics of ${probe_backend_pod} is blocked;" >&2
  echo "the load test reads it there. Admit the API server in networkPolicy.extraFrom of the BDD values." >&2
  exit 1
fi
echo "NetworkPolicy is enforced: backend API reachable for the release and dropped for an outsider, IPFS RPC API dropped, metrics proxy reachable"

# The harness owns these loopback ports. A survivor forward from an earlier
# run — possibly against a DIFFERENT cluster/kubeconfig — binds first, the
# nc readiness check below then passes against the squatter, and every
# DB/ORCE test seam silently talks to the wrong stack.
for harness_port in 5432 18991 19464 18880; do
  fuser -k -n tcp "$harness_port" >/dev/null 2>&1 || true
done
sleep 1

echo "Starting port-forward for PostgreSQL"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "svc/dcs-postgresql" 5432:5432 > .tmp/port-forward-db.log 2>&1 &
echo $! > .tmp/port-forward-db.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 5432 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for port-forward on 5432"
    cat .tmp/port-forward-db.log || true
    exit 1
  fi
  sleep 1
done
echo "Port-forward on 5432 is ready"

# Direct service access for endpoints Traefik does not route (endpoints the
# backend serves at its root, outside the API prefix).
DCS_SERVICE="${DCS_SERVICE:-$DCS_DEPLOYMENT}"
LOCAL_FORWARD_PORT="${LOCAL_FORWARD_PORT:-18991}"
SERVICE_PORT="${SERVICE_PORT:-8991}"
# keep_port_forward.sh restarts the forward when the DCS pod is replaced.
echo "Starting port-forward for DCS service ($DCS_SERVICE)"
KUBECTL_BIN="$KUBECTL_BIN" K8S_NAMESPACE="$K8S_NAMESPACE" \
  SERVICE_NAME="$DCS_SERVICE" PORT_MAPPING="$LOCAL_FORWARD_PORT:$SERVICE_PORT" \
  bash "$PWD/scripts/keep_port_forward.sh" > .tmp/port-forward-dcs.log 2>&1 &
echo $! > .tmp/port-forward-dcs.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$LOCAL_FORWARD_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for port-forward on $LOCAL_FORWARD_PORT"
    cat .tmp/port-forward-dcs.log || true
    exit 1
  fi
  sleep 1
done
export BDD_DCS_INTERNAL_ORIGIN="http://localhost:$LOCAL_FORWARD_PORT"
echo "Port-forward on $LOCAL_FORWARD_PORT is ready"

# /metrics is on its own ClusterIP Service, not on the API port.
METRICS_LOCAL_FORWARD_PORT="${METRICS_LOCAL_FORWARD_PORT:-19464}"
echo "Starting port-forward for DCS metrics (${DCS_SERVICE}-metrics)"
KUBECTL_BIN="$KUBECTL_BIN" K8S_NAMESPACE="$K8S_NAMESPACE" \
  SERVICE_NAME="${DCS_SERVICE}-metrics" PORT_MAPPING="$METRICS_LOCAL_FORWARD_PORT:9464" \
  bash "$PWD/scripts/keep_port_forward.sh" > .tmp/port-forward-metrics.log 2>&1 &
echo $! > .tmp/port-forward-metrics.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$METRICS_LOCAL_FORWARD_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for port-forward on $METRICS_LOCAL_FORWARD_PORT"
    cat .tmp/port-forward-metrics.log || true
    exit 1
  fi
  sleep 1
done
export BDD_DCS_METRICS_ORIGIN="http://localhost:$METRICS_LOCAL_FORWARD_PORT"
echo "Port-forward on $METRICS_LOCAL_FORWARD_PORT is ready"

# The wallet-driven signing scenarios call the EU DSS demonstration webapp
# (charts/dss) as the external SCA that computes getDataToSign/signDocument.
# It is an in-cluster ClusterIP service; the harness reaches it through a
# port-forward at the localhost:18099 default that BDD_DSS_URL points at. The
# DSS Tomcat bundle boots slowly (readiness initialDelaySeconds 90), so allow a
# generous availability timeout before forwarding.
DSS_DEPLOYMENT="${HELM_RELEASE}-dss"
DSS_SERVICE="${HELM_RELEASE}-dss"
DSS_LOCAL_FORWARD_PORT="${DSS_LOCAL_FORWARD_PORT:-18099}"
echo "Waiting for DSS deployment ($DSS_DEPLOYMENT) to be available"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=420s "deployment/$DSS_DEPLOYMENT"

echo "Starting port-forward for DSS service ($DSS_SERVICE)"
KUBECTL_BIN="$KUBECTL_BIN" K8S_NAMESPACE="$K8S_NAMESPACE" \
  SERVICE_NAME="$DSS_SERVICE" PORT_MAPPING="$DSS_LOCAL_FORWARD_PORT:8080" \
  bash "$PWD/scripts/keep_port_forward.sh" > .tmp/port-forward-dss.log 2>&1 &
echo $! > .tmp/port-forward-dss.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$DSS_LOCAL_FORWARD_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for DSS port-forward on $DSS_LOCAL_FORWARD_PORT"
    cat .tmp/port-forward-dss.log || true
    exit 1
  fi
  sleep 1
done
export BDD_DSS_URL="http://localhost:$DSS_LOCAL_FORWARD_PORT"
echo "DSS port-forward on $DSS_LOCAL_FORWARD_PORT is ready"

# Archive notary and audit-log endpoints are intentionally not exposed by the
# public ORCE ingress. Reach the release-scoped service directly and obtain the
# configured token from the running pod rather than duplicating it here.
ORCE_DEPLOYMENT="${HELM_RELEASE}-orce"
ORCE_SERVICE="${HELM_RELEASE}-orce"
ORCE_LOCAL_FORWARD_PORT="${ORCE_LOCAL_FORWARD_PORT:-18880}"
echo "Waiting for ORCE deployment ($ORCE_DEPLOYMENT) to be available"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$ORCE_DEPLOYMENT"
# During a rollout the terminating pod still reports phase Running while its
# containers are already gone — pick the newest running pod and retry the
# exec until it answers.
ORCE_TOKEN=""
deadline=$(( $(date +%s) + 120 ))
while [[ -z "$ORCE_TOKEN" ]]; do
  ORCE_POD="$("$KUBECTL_BIN" -n "$K8S_NAMESPACE" get pod \
    -l "app.kubernetes.io/name=orce,app.kubernetes.io/instance=${HELM_RELEASE}" \
    --field-selector=status.phase=Running \
    --sort-by=.metadata.creationTimestamp \
    -o jsonpath='{.items[-1:].metadata.name}' 2>/dev/null || true)"
  if [[ -n "$ORCE_POD" ]]; then
    ORCE_TOKEN="$("$KUBECTL_BIN" -n "$K8S_NAMESPACE" exec "$ORCE_POD" -c orce -- \
      printenv ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN 2>/dev/null || true)"
  fi
  if [[ -z "$ORCE_TOKEN" ]]; then
    if [ "$(date +%s)" -gt "$deadline" ]; then
      echo "ORCE archive audit token is not configured in pod ${ORCE_POD:-<none>}" >&2
      exit 1
    fi
    sleep 3
  fi
done

echo "Starting port-forward for ORCE service ($ORCE_SERVICE)"
KUBECTL_BIN="$KUBECTL_BIN" K8S_NAMESPACE="$K8S_NAMESPACE" \
  SERVICE_NAME="$ORCE_SERVICE" PORT_MAPPING="$ORCE_LOCAL_FORWARD_PORT:1880" \
  bash "$PWD/scripts/keep_port_forward.sh" > .tmp/port-forward-orce.log 2>&1 &
echo $! > .tmp/port-forward-orce.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$ORCE_LOCAL_FORWARD_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for ORCE port-forward on $ORCE_LOCAL_FORWARD_PORT"
    cat .tmp/port-forward-orce.log || true
    exit 1
  fi
  sleep 1
done
export BDD_ORCE_ARCHIVE_NOTARY_URL="http://localhost:${ORCE_LOCAL_FORWARD_PORT}/archive/notary"
export BDD_ORCE_ARCHIVE_AUDIT_LOG_URL="http://localhost:${ORCE_LOCAL_FORWARD_PORT}/archive-audit-events.jsonl"
export BDD_ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN="$ORCE_TOKEN"
export BDD_ORCE_AUDIT_CONTROL_URL="http://localhost:${ORCE_LOCAL_FORWARD_PORT}/audit-executor/test"
# The reference executor endpoint itself, reached through the same port-forward
# as its control seam above. Its ingress-relative fallback (<origin>/orce/...)
# cannot be used: the DCS Ingress claims the /orce prefix on the same host for
# the webhook platform (backend/cmd/dcs/http.go mounts /orce/ on the service
# root), so an ingress-addressed /orce/audit/run is answered by the DCS mux
# with "404 page not found" and never reaches Node-RED.
export BDD_ORCE_AUDIT_EXECUTOR_URL="http://localhost:${ORCE_LOCAL_FORWARD_PORT}/audit/run"
export BDD_ORCE_NAMESPACE="$K8S_NAMESPACE"
export BDD_ORCE_DEPLOYMENT="$ORCE_DEPLOYMENT"
export BDD_KUBECTL="$KUBECTL_BIN"
# ADR-18 trust gate: both DCS instances point their DCS_TRUST_PDP_URL at the
# controllable ORCE trust-pdp flow; the steps steer it (allow/deny/silent)
# and read back the last captured request through this same port-forward.
export BDD_TRUST_PDP_CONTROL_URL="http://localhost:${ORCE_LOCAL_FORWARD_PORT}"
export BDD_TRUST_PDP_DEFAULT_FLOW_WIRED=1

echo "Waiting for authenticated ORCE archive audit-log endpoint"
deadline=$(( $(date +%s) + 60 ))
orce_archive_code=""
until orce_archive_code=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $ORCE_TOKEN" "$BDD_ORCE_ARCHIVE_AUDIT_LOG_URL" 2>/dev/null) \
    && [[ "$orce_archive_code" == "200" || "$orce_archive_code" == "404" ]]; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for ORCE archive audit log (last HTTP $orce_archive_code)"
    exit 1
  fi
  sleep 2
done
echo "ORCE archive endpoints are reachable"

# ORCE (Node-RED) hosts the contract-target-flow the deployment scenarios POST
# to directly; the BDD values route it through the shared Traefik ingress
# (orce.ingress in values.bdd.yml), so it is reachable at the public origin —
# same path locally and on CI, no port-forward. An empty POST must yield the
# flow's own 400 validation error; a Traefik 404 means the route is missing.
export BDD_ORCE_TARGET_URL="${BDD_ORCE_TARGET_URL:-${BDD_PUBLIC_ORIGIN}/contract-target/deploy}"
echo "Waiting for ORCE contract-target flow at $BDD_ORCE_TARGET_URL ..."
deadline=$(( $(date +%s) + 60 ))
until orce_code=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" -X POST "$BDD_ORCE_TARGET_URL" \
    -H 'Content-Type: application/json' -d '{}' 2>/dev/null) \
    && [[ "$orce_code" =~ ^[24][0-9]{2}$|^400$ ]] && [[ "$orce_code" != 404 ]]; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for ORCE contract-target flow at $BDD_ORCE_TARGET_URL (last HTTP $orce_code)"
    exit 1
  fi
  sleep 2
done
echo "ORCE contract-target flow is reachable (HTTP $orce_code); BDD_ORCE_TARGET_URL=$BDD_ORCE_TARGET_URL"

source "$VENV_PATH/bin/activate"
export BDD_DCS_BASE_URL

echo "Checking the issuer status list at $ISSUER_BASE_URL"
python "$PWD/scripts/check_status_list.py"

# The database accepts TLS only (global.internalTLS). Through the port-forward
# the host name is localhost, which the server certificate does not name, so the
# harness verifies the chain against the release CA but not the name.
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" get secret dcs-internal-tls -o jsonpath='{.data.ca\.crt}' \
  | base64 --decode > "$PWD/.tmp/internal-ca.crt"
export DATABASE_URL="host=localhost port=5432 user=dcs password=dcs dbname=dcs sslmode=verify-ca sslrootcert=$PWD/.tmp/internal-ca.crt"

# Canonical bdd-executor integration requires the package in the active environment.
python -c 'import eu.xfsc.bdd.core' >/dev/null

# @isolated_stack scenarios arrange their own isolation (own namespace, own
# Helm release) inside the same cluster, so they belong in the sequential
# full-suite pass. The suite therefore runs unfiltered by default; a caller
# that wants a subset (run_bdd_audit_kind_once) passes ARG_BDD_TAGS.
# Note ARG_BDD_TAGS arrives from the Makefile recipe as set-but-empty, so the
# test must be on emptiness, not on being unset.
EXTRA_ARGS=()
if [[ -n "${ARG_BDD_TAGS:-}" ]]; then
  # shellcheck disable=SC2206
  EXTRA_ARGS+=(${ARG_BDD_TAGS})
fi
if [[ -n "${ARG_BDD:-}" ]]; then
  # shellcheck disable=SC2206
  EXTRA_ARGS+=(${ARG_BDD})
fi

JUNIT_ARGS=(--junit --junit-directory .reports/junit)
if [[ -n "${ARG_BDD_JUNIT:-}" ]]; then
  # shellcheck disable=SC2206
  JUNIT_ARGS=(${ARG_BDD_JUNIT})
fi

# The deployed stack + all its port-forwards (DSS 18099, ORCE, DB, instance B)
# are live at this point and stay alive until this script exits (trap cleanup).
# RUN_MODE selects what runs against them without tearing anything down:
#   bdd (default) — the behave suite via the bdd-executor environment;
#   e2e           — the Playwright suite (the built UI on vite preview + the venv-backed
#                   signing helpers), so the frontend E2E gets the same live
#                   two-instance stack + DSS forward the BDD suite uses.
#   load          — the k6 load test (tests/load/run_load.py, NFR-PER-01/02).
if [[ "${RUN_MODE:-bdd}" == "load" ]]; then
  echo "Running the load test against the deployed stack"
  cd "$PROJECT_ROOT"
  python tests/load/run_load.py
elif [[ "${RUN_MODE:-bdd}" == "e2e" ]]; then
  echo "Running Playwright E2E against the deployed stack"
  cd "$PROJECT_ROOT/frontend/ClientApp"
  # The suite runs against the production build (the same `build:docker` the
  # DCS image runs), served by `vite preview` (E2E_BUILT_UI, playwright.config.ts).
  npm run build:docker
  # DCS_HYDRA_TARGET routes the UI server's /oauth2 proxy at the deployed
  # Hydra through the same ingress everything else uses. Left unset it defaults
  # to localhost:4444, where nothing listens in CI, so anything reaching for a
  # token gets a proxy error instead of an answer.
  E2E_BUILT_UI=1 \
  E2E_DCS_API_BASE="${BDD_PUBLIC_ORIGIN}/digital-contracting-service/api" \
  E2E_BDD_PYTHON="$VENV_PATH/bin/python3" \
  DCS_HYDRA_TARGET="${BDD_PUBLIC_ORIGIN}" \
    npm run e2e
else
  echo "Running BDD suite via bdd-executor environment"
  cd "$PROJECT_ROOT"
  "$VENV_PATH/bin/coverage" run --append -m behave "${JUNIT_ARGS[@]}" "$FEATURES_PATH" "${EXTRA_ARGS[@]}"

  JUNIT_COUNT=$(find "$REPORTS_JUNIT_DIR" -name "*.xml" 2>/dev/null | wc -l || true)
  echo "Generated $JUNIT_COUNT junit XML files in $REPORTS_JUNIT_DIR/"
  python "$PWD/tests/bdd/scripts/assert_junit_scenarios.py" "$REPORTS_JUNIT_DIR"
fi
