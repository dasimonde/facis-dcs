#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  if [[ -f .tmp/port-forward.pid ]]; then
    kill "$(cat .tmp/port-forward.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/keycloak-port-forward.pid ]]; then
    kill "$(cat .tmp/keycloak-port-forward.pid)" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

: "${VENV_PATH:?VENV_PATH is required}"
: "${FEATURES_PATH:?FEATURES_PATH is required}"
: "${KUBECTL_BIN:?KUBECTL_BIN is required}"
: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${DCS_DEPLOYMENT:?DCS_DEPLOYMENT is required}"
: "${DCS_SERVICE:?DCS_SERVICE is required}"
: "${LOCAL_FORWARD_PORT:?LOCAL_FORWARD_PORT is required}"
: "${SERVICE_PORT:?SERVICE_PORT is required}"
: "${DCS_API_BASE_PATH:?DCS_API_BASE_PATH is required}"
: "${KEYCLOAK_SERVICE:?KEYCLOAK_SERVICE is required}"
: "${KEYCLOAK_DEPLOYMENT:?KEYCLOAK_DEPLOYMENT is required}"
: "${KEYCLOAK_LOCAL_FORWARD_PORT:?KEYCLOAK_LOCAL_FORWARD_PORT is required}"
: "${KEYCLOAK_SERVICE_PORT:?KEYCLOAK_SERVICE_PORT is required}"
: "${BDD_KEYCLOAK_REALM:?BDD_KEYCLOAK_REALM is required}"
: "${BDD_RUN_MODE:?BDD_RUN_MODE is required (dev|all)}"
: "${PROJECT_ROOT:?PROJECT_ROOT is required}"

mkdir -p .tmp .reports/junit
REPORTS_JUNIT_DIR="$PWD/.reports/junit"
printf "%s localhost\n" "$KEYCLOAK_SERVICE" > .tmp/hostaliases
rm -f .tmp/port-forward.log .tmp/port-forward.pid .tmp/keycloak-port-forward.log .tmp/keycloak-port-forward.pid

echo "Waiting for DCS deployment ($DCS_DEPLOYMENT) to be available"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$DCS_DEPLOYMENT"

echo "Starting port-forward svc/$DCS_SERVICE $LOCAL_FORWARD_PORT:$SERVICE_PORT in namespace $K8S_NAMESPACE"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "svc/$DCS_SERVICE" "$LOCAL_FORWARD_PORT:$SERVICE_PORT" > .tmp/port-forward.log 2>&1 &
echo $! > .tmp/port-forward.pid

if [[ "${ENABLE_KEYCLOAK_PORT_FORWARD:-false}" == "true" ]]; then
  echo "Waiting for Keycloak deployment to be available"
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$KEYCLOAK_DEPLOYMENT"

  echo "Starting Keycloak port-forward svc/$KEYCLOAK_SERVICE $KEYCLOAK_LOCAL_FORWARD_PORT:$KEYCLOAK_SERVICE_PORT in namespace $K8S_NAMESPACE"
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "svc/$KEYCLOAK_SERVICE" "$KEYCLOAK_LOCAL_FORWARD_PORT:$KEYCLOAK_SERVICE_PORT" > .tmp/keycloak-port-forward.log 2>&1 &
  echo $! > .tmp/keycloak-port-forward.pid
  keycloak_pf_pid="$(cat .tmp/keycloak-port-forward.pid)"

  keycloak_ready=false
  for _ in $(seq 1 30); do
    if ! kill -0 "$keycloak_pf_pid" >/dev/null 2>&1; then
      echo "Keycloak port-forward process exited early (pid=$keycloak_pf_pid)." >&2
      echo "--- keycloak port-forward log ---" >&2
      cat .tmp/keycloak-port-forward.log >&2 || true
      exit 1
    fi

    if curl -fsS "http://127.0.0.1:$KEYCLOAK_LOCAL_FORWARD_PORT/realms/$BDD_KEYCLOAK_REALM/.well-known/openid-configuration" >/dev/null 2>&1; then
      keycloak_ready=true
      break
    fi
    sleep 1
  done

  if [[ "$keycloak_ready" != "true" ]]; then
    echo "Keycloak did not become ready on port-forward within timeout (localhost:$KEYCLOAK_LOCAL_FORWARD_PORT)." >&2
    echo "--- keycloak port-forward log ---" >&2
    cat .tmp/keycloak-port-forward.log >&2 || true
    exit 1
  fi
fi

sleep 2

source "$VENV_PATH/bin/activate"
export HOSTALIASES="$PWD/.tmp/hostaliases"
export NO_PROXY="$KEYCLOAK_SERVICE,127.0.0.1,localhost,${NO_PROXY:-}"
export BDD_DCS_BASE_URL="http://127.0.0.1:$LOCAL_FORWARD_PORT$DCS_API_BASE_PATH"
export BDD_KEYCLOAK_BASE_URL="http://$KEYCLOAK_SERVICE:$KEYCLOAK_LOCAL_FORWARD_PORT"
export BDD_KEYCLOAK_HOST_HEADER="$KEYCLOAK_SERVICE:$KEYCLOAK_SERVICE_PORT"
export BDD_KEYCLOAK_REALM
export BDD_KEYCLOAK_CLIENT_ID="${BDD_KEYCLOAK_CLIENT_ID:-dcs-client}"

# Canonical bdd-executor integration requires the package in the active environment.
python -c 'import eu.xfsc.bdd.core' >/dev/null

EXTRA_ARGS=()
if [[ -n "${ARG_BDD:-}" ]]; then
  # shellcheck disable=SC2206
  EXTRA_ARGS=(${ARG_BDD})
fi

JUNIT_ARGS=(--junit --junit-directory .reports/junit)
if [[ -n "${ARG_BDD_JUNIT:-}" ]]; then
  # shellcheck disable=SC2206
  JUNIT_ARGS=(${ARG_BDD_JUNIT})
fi

echo "Running BDD suite via bdd-executor environment"
if [[ "$BDD_RUN_MODE" == "all" ]]; then
  cd "$PROJECT_ROOT"
  "$VENV_PATH/bin/coverage" run --append -m behave "${JUNIT_ARGS[@]}" "$FEATURES_PATH" "${EXTRA_ARGS[@]}"
else
  cd "$PROJECT_ROOT"
  "$VENV_PATH/bin/coverage" run --append -m behave "${JUNIT_ARGS[@]}" "$FEATURES_PATH" "${EXTRA_ARGS[@]}"
fi

JUNIT_COUNT=$(find "$REPORTS_JUNIT_DIR" -name "*.xml" 2>/dev/null | wc -l || true)
echo "Generated $JUNIT_COUNT junit XML files in $REPORTS_JUNIT_DIR/"
