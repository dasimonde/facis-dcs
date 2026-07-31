#!/usr/bin/env bash
# Deploy one DCS instance with plain Helm.
#
# Environment-specific configuration (hostnames, DIDs, credentials) belongs in a
# values file kept OUTSIDE this repo and passed with --values. This script
# carries none of its own.
#
# The post-deploy checks exist because the failures that matter here are silent:
# an instance can be Running and still be unable to federate at all.
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

RELEASE="dcs"
NAMESPACE="dcs"
VALUES_FILES=()
PUBLIC_URL=""
TIMEOUT="15m"
SKIP_VERIFY=0
DRY_RUN=0

log()  { echo "[$(date '+%H:%M:%S')] $*"; }
fail() { echo "[$(date '+%H:%M:%S')] ERROR: $*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: deploy.sh --values FILE [--values FILE]... [options]

Required:
  --values FILE       Values file. Repeatable; later files override earlier ones.
                      The chart's own values.yaml is always applied first, so
                      pass only this environment's overrides.

Options:
  --release NAME      Helm release name (default: dcs). Subchart service names
                      derive from it, so values that reference e.g.
                      http://<release>-orce:1880 must agree with it.
  --namespace NS      Target namespace (default: dcs).
  --kubeconfig PATH   Kubeconfig to use (default: $KUBECONFIG, else ~/.kube/config).
  --public-url URL    Base https URL this instance is reachable at. When given,
                      the checks confirm a peer can actually resolve it.
  --timeout DURATION  Helm timeout (default: 15m).
  --dry-run           Render and validate against the cluster's API without
                      changing anything. Catches values/template errors and
                      missing dependencies before a real run.
  --skip-verify       Install only; run no checks.
  -h, --help          This text.

Example:
  ./deploy.sh --values ~/private/values.my-instance.yml \
              --namespace dcs --release dcs \
              --public-url https://dcs.example.org
EOF
  exit "${1:-1}"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --values)      [ "$#" -ge 2 ] || fail "--values needs a path";      VALUES_FILES+=("$2");    shift 2 ;;
    --release)     [ "$#" -ge 2 ] || fail "--release needs a name";     RELEASE="$2";            shift 2 ;;
    --namespace)   [ "$#" -ge 2 ] || fail "--namespace needs a name";   NAMESPACE="$2";          shift 2 ;;
    --kubeconfig)  [ "$#" -ge 2 ] || fail "--kubeconfig needs a path";  export KUBECONFIG="$2";  shift 2 ;;
    --public-url)  [ "$#" -ge 2 ] || fail "--public-url needs a URL";   PUBLIC_URL="${2%/}";     shift 2 ;;
    --timeout)     [ "$#" -ge 2 ] || fail "--timeout needs a duration"; TIMEOUT="$2";            shift 2 ;;
    --dry-run)     DRY_RUN=1; shift ;;
    --skip-verify) SKIP_VERIFY=1; shift ;;
    -h|--help)     usage 0 ;;
    *)             fail "unknown argument: $1 (see --help)" ;;
  esac
done

[ "${#VALUES_FILES[@]}" -gt 0 ] || fail "at least one --values file is required (see --help)"
command -v helm    >/dev/null || fail "helm not on PATH"
command -v kubectl >/dev/null || fail "kubectl not on PATH"

HELM_VALUES_ARGS=(--values "$CHART_DIR/values.yaml")
for f in "${VALUES_FILES[@]}"; do
  [ -r "$f" ] || fail "values file not readable: $f"
  HELM_VALUES_ARGS+=(--values "$f")
done

# Reachability only. Deliberately NOT `kubectl get namespace`: a namespace-scoped
# service account cannot read cluster-scoped objects, and those restricted
# credentials are a target case here. Helm reports a missing namespace itself.
kubectl version --request-timeout=15s >/dev/null 2>&1 \
  || fail "cannot reach the cluster API — check --kubeconfig"

BACKEND="deployment/${RELEASE}-digital-contracting-service"

diagnose_fc() {
  echo "Federated Catalogue workload diagnosis:" >&2
  kubectl -n "$NAMESPACE" get deployment,job,pod -l \
    "app.kubernetes.io/instance=${RELEASE}" -o wide >&2 || true
  for workload in fc-service fc-fuseki fc-keycloak fc-postgres; do
    kubectl -n "$NAMESPACE" describe "deployment/$workload" >&2 || true
    kubectl -n "$NAMESPACE" logs "deployment/$workload" --all-containers --tail=100 >&2 || true
  done
  kubectl -n "$NAMESPACE" describe "job/${RELEASE}-digital-contracting-service-fc-realm-provision" >&2 || true
  kubectl -n "$NAMESPACE" logs "job/${RELEASE}-digital-contracting-service-fc-realm-provision" \
    --all-containers --tail=100 >&2 || true
}

# Subcharts are file:// dependencies, and Helm prefers a packaged charts/*.tgz
# over the source directory. A .tgz left from an earlier build therefore shadows
# edits to charts/<name>/ and deploys stale templates, so repackage every run.
log "Building dependencies from the committed Chart.lock"
helm dependency build --skip-refresh "$CHART_DIR" >/dev/null

if [ "$DRY_RUN" -eq 1 ]; then
  log "Validating release '$RELEASE' against namespace '$NAMESPACE' (dry run, no changes)"
  helm upgrade --install "$RELEASE" "$CHART_DIR" \
    --namespace "$NAMESPACE" \
    "${HELM_VALUES_ARGS[@]}" \
    --dry-run=server >/dev/null
  log "Dry run passed: values and templates are valid and the API accepted every manifest."
  exit 0
fi

log "Deploying release '$RELEASE' into namespace '$NAMESPACE'"
if ! helm upgrade --install "$RELEASE" "$CHART_DIR" \
  --namespace "$NAMESPACE" \
  "${HELM_VALUES_ARGS[@]}" \
  --timeout "$TIMEOUT"; then
  diagnose_fc
  fail "Helm deployment failed"
fi

if [ "$SKIP_VERIFY" -eq 1 ]; then
  log "Deployed. Verification skipped."
  exit 0
fi

fc_local=0
if kubectl -n "$NAMESPACE" get deployment/fc-service >/dev/null 2>&1; then
  fc_local=1
  log "Waiting for native Federated Catalogue health readiness"
  if ! kubectl -n "$NAMESPACE" rollout status deployment/fc-service --timeout="$TIMEOUT"; then
    diagnose_fc
    fail "co-deployed Federated Catalogue did not become healthy"
  fi
fi

log "Waiting for the backend startup gate"
if ! kubectl -n "$NAMESPACE" rollout status "$BACKEND" --timeout="$TIMEOUT"; then
  [ "$fc_local" -eq 0 ] || diagnose_fc
  kubectl -n "$NAMESPACE" logs "$BACKEND" --all-containers --tail=200 >&2 || true
  fail "backend startup gate failed"
fi

problems=0
note_problem() { echo "  FAIL: $*"; problems=$((problems + 1)); }

backend_env() {
  kubectl -n "$NAMESPACE" get "$BACKEND" \
    -o jsonpath='{range .spec.template.spec.containers[0].env[*]}{.name}={.value}{"\n"}{end}' 2>/dev/null \
    | sed -n "s/^$1=//p" | head -1
}

# Key material arrives from the provisioning hook only after the pod is
# scheduled, so a Running backend has not necessarily cleared its startup gates.
log "Checking the backend started cleanly"
backend_log="$(kubectl -n "$NAMESPACE" logs "$BACKEND" --tail=300 2>/dev/null || true)"
if grep -q "HTTP server listening" <<<"$backend_log"; then
  echo "  ok: reached its HTTP listener"
else
  note_problem "never logged 'HTTP server listening' — still bootstrapping or crash-looping"
fi
if grep -q "failed to sync federated catalogue" <<<"$backend_log"; then
  note_problem "federated catalogue sync failed — the backend treats this as fatal and will restart"
fi
if grep -q "federated catalogue readiness gate failed" <<<"$backend_log"; then
  note_problem "federated catalogue readiness gate failed — see the terminal diagnosis above"
fi

# The trust gate is fail-closed: unset or unreachable means federation is off,
# and a healthy-looking instance gives no other sign of it.
pdp_url="$(backend_env DCS_TRUST_PDP_URL)"
if [ -z "$pdp_url" ]; then
  echo "  note: DCS_TRUST_PDP_URL unset — this instance will not federate (see federation.trustPdpURL)"
else
  code="$(kubectl -n "$NAMESPACE" exec "$BACKEND" -- sh -c \
    "curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
     -d '{\"peerDID\":\"did:web:deploy-sh-probe\",\"direction\":\"inbound\"}' '$pdp_url'" 2>/dev/null || echo 000)"
  case "$code" in
    2*) echo "  ok: trust policy endpoint answered $code" ;;
    *)  note_problem "trust policy endpoint $pdp_url answered $code — federation stays closed until it returns 2xx" ;;
  esac
fi

if [ -z "$(backend_env DSS_URL)" ]; then
  echo "  note: DSS_URL unset — no validator, so externally produced signatures are refused"
fi

# At-rest encryption (DCS-NFR-SEC-14) rides on the StorageClass of these PVCs.
# A claim keeps the class it was born with, so a values change after first
# install silently does nothing — assert the bound class against the class the
# rendered manifest asked for. No configured class means the cluster default
# was used: that is an operator prerequisite this script cannot assert.
manifest_pvc_storageclass() {
  helm -n "$NAMESPACE" get manifest "$RELEASE" 2>/dev/null | awk -v pvc="$1" '
    function flush() { if (kind == "PersistentVolumeClaim" && name == pvc) { gsub(/"/, "", sc); print sc } }
    /^---/   { flush(); kind = ""; name = ""; sc = ""; next }
    /^kind:/ { kind = $2 }
    /^  name:/ { if (name == "") name = $2 }
    /^[[:space:]]*storageClassName:/ { sc = $2 }
    END      { flush() }'
}

storageclass_check() {
  local label="$1" pvc="$2" desired actual
  kubectl -n "$NAMESPACE" get pvc "$pvc" >/dev/null 2>&1 || return 0
  desired="$(manifest_pvc_storageclass "$pvc")"
  actual="$(kubectl -n "$NAMESPACE" get pvc "$pvc" -o jsonpath='{.spec.storageClassName}' 2>/dev/null)"
  if [ -z "$desired" ]; then
    echo "  note: $label PVC has no StorageClass configured (bound: ${actual:-cluster default}) — encrypted storage is an operator prerequisite, not asserted"
  elif [ "$actual" = "$desired" ]; then
    echo "  ok: $label PVC bound to configured StorageClass '$desired'"
  else
    note_problem "$label PVC is bound to StorageClass '${actual:-<none>}' but '$desired' is configured — a pre-existing claim keeps its class; recreate the PVC to move it"
  fi
}

log "Checking StorageClass of the at-rest-relevant PVCs"
storageclass_check "postgres"  "${RELEASE}-postgresql"
storageclass_check "ipfs"      "${RELEASE}-ipfs"
storageclass_check "hsm-token" "${RELEASE}-digital-contracting-service-hsm-token"

# A peer resolves this instance by appending /.well-known/did.json to the bare
# hostname. Anything else claiming /.well-known on the ingress (Hydra's OIDC
# discovery) shadows these documents, and nothing inside the cluster notices.
if [ -n "$PUBLIC_URL" ]; then
  log "Checking federation documents resolve at $PUBLIC_URL"
  for path in /.well-known/did.json \
              /.well-known/dcs-agreement-credential.json \
              /.well-known/dcs-federation-rules.md; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "${PUBLIC_URL}${path}" 2>/dev/null || echo 000)"
    if [ "$code" = "200" ]; then
      echo "  ok: $path"
    else
      note_problem "$path answered $code — a peer cannot resolve this instance"
    fi
  done

  expected_did="$(backend_env ISSUER_DID)"
  # Match each "id" separately and keep the one without a fragment: that is the
  # document's own id, whatever order the keys are serialised in. A single
  # s/.*"id"…/ over the whole document anchors on the LAST one instead, because
  # .* is greedy — it reports a verification method and fails a valid document.
  served_did="$(curl -sS --max-time 20 "${PUBLIC_URL}/.well-known/did.json" 2>/dev/null \
    | grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' \
    | sed -n 's/.*"\([^"]*\)"$/\1/p' | grep -v '#' | head -1)"
  if [ -n "$expected_did" ] && [ -n "$served_did" ] && [ "$expected_did" != "$served_did" ]; then
    note_problem "serves '$served_did' but ISSUER_DID is '$expected_did' — peers will reject it"
  elif [ -n "$served_did" ]; then
    echo "  ok: serving $served_did"
  fi
else
  echo "  note: --public-url not given, skipped the external federation checks"
fi

echo
if [ "$problems" -gt 0 ]; then
  log "Deployed, but $problems check(s) failed — see above."
  exit 1
fi
log "Deployed and verified."
