#!/bin/bash
# Second-instance runner for the two-instance inter-org setup
# (features/17_peer_trust @two-instance scenarios).
#
# Starts a SECOND, independent DCS instance ("instance B") alongside the one
# started by dev-stack.sh ("instance A"): its own Helm release (Postgres,
# Keycloak, Hydra, NATS, statuslist-service, ORCE and IPFS — see
# deployment/helm/values.dev2.yml for the full NodePort map). The Federated
# Catalogue is NOT duplicated: instance B consumes instance A's, over the
# NodePorts dev-stack.sh already publishes (see backend/.env.dev2), so A must
# be up and its FC realm provisioned before B's backend starts.
# Backend on :8992, frontend on :5174. Instance B holds its own private keys in
# a separate SoftHSM2 token (provisioned below), never in the cluster.
#
# Run dev-stack.sh FIRST (instance A + the shared pdf-core process instance B
# also depends on, see PDF_CORE_URL in backend/.env.dev2), then run this
# script from the project root in a separate shell: bash dev-stack2.sh
# Press Ctrl+C to stop everything this script started (instance A/pdf-core
# are left running).
#
# NOTE on `air`: dev-stack.sh uses `air` for the backend's hot-reload dev
# loop, but this project's `air` is known not to reliably start the built
# binary under Windows (see CLAUDE.md / project runbook), and running two
# `air` instances against the same backend/ directory with the default
# .air.toml (shared ./tmp/main, clean_on_exit=true) would race with instance
# A's air process. Instance B therefore builds and runs the backend binary
# directly with `-env=.env.dev2` instead — see main.go's `-env` flag
# (cmd/dcs/dotenv.go) — which sidesteps both problems.

set -euo pipefail

trap 'kill $(jobs -p) 2>/dev/null; exit' INT TERM

# Config
HELM_RELEASE="dcs2"
HELM_CHART_PATH="deployment/helm"
HELM_VALUES_FILE="deployment/helm/values.dev2.yml"
BACKEND_ENV_FILE=".env.dev2"
BACKEND_BUILD_OUTPUT="tmp2/main2"

echo "=== Setting up dev environment for instance B (:8992 / :5174) ==="

echo "Building locked Helm dependencies and deploying to Kubernetes..."
helm dependency build --skip-refresh "$HELM_CHART_PATH"
helm upgrade --install "$HELM_RELEASE" "$HELM_CHART_PATH" -f "$HELM_VALUES_FILE"

# Still deployed for the C2PA provenance credentials this instance issues
# itself. The credentials a signatory presents name the shared issuer's own
# signed list instead (ADR-34).
echo "Waiting for statuslist-service..."
kubectl wait --for=condition=ready pod \
  -l "app.kubernetes.io/instance=${HELM_RELEASE},app.kubernetes.io/name=statuslist-service" \
  --timeout=5m

# The credential issuer is shared with instance A, like the Federated Catalogue:
# a Power of Attorney credential names one status list, and both instances must
# resolve that one URL. Instance B installs no issuer of its own and verifies A's
# against the same committed anchors (ADR-34).
echo "Checking the credential issuer (started by dev-stack.sh) is reachable..."
export ISSUER_BASE_URL="${ISSUER_BASE_URL:-http://localhost:30181}"
make -C testWallet check-status-list

# pdf-core is shared with instance A (same PDF_CORE_URL=http://localhost:8080
# in both backend/.env.dev1 and backend/.env.dev2) — verify it's already up
# rather than starting a second one on the same port.
echo "Checking shared pdf-core (started by dev-stack.sh) is reachable..."
if ! curl -sf http://localhost:8080/version >/dev/null 2>&1; then
  echo "error: pdf-core is not reachable at http://localhost:8080 — start instance A first (bash dev-stack.sh)" >&2
  exit 1
fi
echo "✓ pdf-core reachable"

echo ""
echo "=== Starting Vite dev server for instance B (:5174) ==="
cd frontend/ClientApp
npm run dev-dcs2 &
VITE_PID=$!
cd ../..

sleep 2

echo ""
echo "=== Provisioning instance B SoftHSM token and DID document ==="
# Instance B keeps its own token dir (separate from instance A's -8991), so the
# two instances' ECDSA DID keys differ — the genuine two-instance peer-trust
# breaking-change setup.
HSM_TOKEN_DIR_B="$HOME/.dcs/softhsm-8992"
bash scripts/hsm-provision.sh "$HSM_TOKEN_DIR_B" dcs 1234 12345678
export SOFTHSM2_CONF="$HSM_TOKEN_DIR_B/softhsm2.conf"
(
  cd backend
  PKCS11_MODULE_PATH=/usr/lib/softhsm/libsofthsm2.so \
  PKCS11_TOKEN_LABEL=dcs PKCS11_PIN=1234 \
  go run ./cmd/gendid -out certs/dev/did-8992.json \
    -did "did:web:localhost%3A8992" -endpoint "http://localhost:8992/api"
)
echo "✓ HSM token provisioned and did-8992.json regenerated"

echo ""
echo "=== Building and starting backend for instance B (:8992) ==="
cd backend
mkdir -p "$(dirname "$BACKEND_BUILD_OUTPUT")"
goa gen digital-contracting-service/design
go build -o "$BACKEND_BUILD_OUTPUT" ./cmd/dcs
"./$BACKEND_BUILD_OUTPUT" -env="$BACKEND_ENV_FILE" &> /tmp/backend-b-live.log &
BACKEND_PID=$!
cd ..
echo "✓ backend B started (PID $BACKEND_PID) — log: /tmp/backend-b-live.log"

echo ""
echo "Instance B: frontend http://localhost:5174/ui/  backend http://localhost:8992"
echo "did:web document: http://localhost:5174/api/.well-known/did.json (via Vite proxy) or http://localhost:8992/.well-known/did.json directly"

# Cleanup on exit
wait $VITE_PID 2>/dev/null || true
wait $BACKEND_PID 2>/dev/null || true
