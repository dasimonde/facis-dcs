#!/bin/bash
# Full dev stack orchestration: Helm → pdf-core → Vite → Air
# Run from project root: bash dev-stack.sh
# Press Ctrl+C to stop everything
#
# Expected flow:
#   1. helm install dcs deployment/helm -f deployment/helm/values.dev.yml
#   2. bash dev-stack.sh   ← fetches certs, then starts pdf-core + frontend + backend

set -euo pipefail

cleanup() {
  local pids
  pids="$(jobs -p)"
  if [[ -n "$pids" ]]; then
    kill $pids 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

# Config
HELM_RELEASE="dcs"
HELM_CHART_PATH="deployment/helm"
HELM_VALUES_FILE="deployment/helm/values.dev.yml"
PDF_CORE_DIR="pdf-core"
PDF_CORE_DEV_ENV="$PDF_CORE_DIR/.dev.env"
PDF_CORE_ENV="$PDF_CORE_DIR/.env"
TSA_TRUST_CERT_FILE="backend/certs/dev/orce-tsa-cert.pem"
TSA_TRUST_SECRET="${HELM_RELEASE}-orce-tsa-material"

echo "=== Setting up dev environment ==="

# Helm: idempotent install-or-upgrade so this script works whether or not
# the release was already installed manually first.
echo "Building locked Helm dependencies and deploying to Kubernetes..."
helm dependency build --skip-refresh "$HELM_CHART_PATH"
helm upgrade --install "$HELM_RELEASE" "$HELM_CHART_PATH" -f "$HELM_VALUES_FILE"

# The backend's catalogue readiness gate makes exactly one verification call and
# exits if it fails — deliberately, so a broken catalogue is not papered over by
# retries. That leaves the ordering to whoever starts the backend: fc-service is
# a Spring Boot app behind Fuseki and answers nothing for a while after a
# deploy, so starting air straight away races it and kills the process:
#
#   federated catalogue functional verification failed: ... context deadline
#   exceeded (Client.Timeout exceeded while awaiting headers)
#
# Only wait when the values file actually deploys it.
if python3 -c "import sys,yaml; sys.exit(0 if (yaml.safe_load(open('$HELM_VALUES_FILE')) or {}).get('federatedCatalogue',{}).get('enabled') else 1)"; then
  echo "Waiting for the Federated Catalogue to answer before starting the backend..."
  kubectl wait --for=condition=ready pod \
    -l "app.kubernetes.io/instance=${HELM_RELEASE},app.kubernetes.io/name=federated-catalogue" \
    --timeout=10m
fi

# The host-side backend verifies every RFC 3161 token against the same
# certificate used by the in-cluster ORCE TSA. Keep the local trust anchor in
# sync with the release Secret on every stack start.
kubectl wait --for=create "secret/$TSA_TRUST_SECRET" --timeout=2m
kubectl get secret "$TSA_TRUST_SECRET" \
  -o jsonpath='{.data.tsa-cert\.pem}' | base64 --decode > "$TSA_TRUST_CERT_FILE"
echo "✓ ORCE TSA trust certificate exported"

echo "Waiting for statuslist-service..."
kubectl wait --for=condition=ready pod \
  -l "app.kubernetes.io/instance=${HELM_RELEASE},app.kubernetes.io/name=statuslist-service" \
  --timeout=5m

echo "Installing testWallet dependencies..."
make -C testWallet install

echo "Initializing statuslist for dev (NATS create when list is empty)..."
make -C testWallet ensure-statuslist

# Setup backend .env
cp backend/.env.dev1 backend/.env
echo "✓ .env updated from .env.dev1"

# Provision the SoftHSM2 token holding this instance's private keys and
# regenerate its DID document with the ECDSA P-256 token key.
HSM_TOKEN_DIR="$HOME/.dcs/softhsm-8991"
bash scripts/hsm-provision.sh "$HSM_TOKEN_DIR" dcs 1234 12345678
echo "SOFTHSM2_CONF=$HSM_TOKEN_DIR/softhsm2.conf" >> backend/.env
(
  cd backend
  SOFTHSM2_CONF="$HSM_TOKEN_DIR/softhsm2.conf" \
  PKCS11_MODULE_PATH=/usr/lib/softhsm/libsofthsm2.so \
  PKCS11_TOKEN_LABEL=dcs PKCS11_PIN=1234 \
  go run ./cmd/gendid -out certs/dev/did-8991.json \
    -did "did:web:localhost%3A8991" -endpoint "http://localhost:8991/api"
)
echo "✓ HSM token provisioned and did-8991.json regenerated"

# Issue the C2PA x5chain binding the dcs-c2pa token key so pdf-core can embed it
# in the COSE_Sign1 protected header (the signing itself runs in the backend).
bash scripts/c2pa-cert-provision.sh "$HSM_TOKEN_DIR" dcs 1234 \
  "$PDF_CORE_DIR/certs/dev/c2pa-x5chain-8991.pem"
echo "✓ C2PA x5chain provisioned for pdf-core"

# Publish an initial (empty) CRL for the dev signing CA so the leaf's
# crlDistributionPoints resolves to a fresh, valid list. crlcheck (ops) or the
# AC11 test path can later revoke the signing cert against this CA.
bash scripts/crl-provision.sh "$PDF_CORE_DIR/certs/dev" \
  "$PDF_CORE_DIR/certs/dev/dcs-c2pa.crl"
echo "✓ Dev CRL published"

# Copy .dev.env → .env so pdf-core main.go picks it up at startup.
cp "$PDF_CORE_DEV_ENV" "$PDF_CORE_ENV"

(cd "$PDF_CORE_DIR" && air) &> /tmp/pdf-core-live.log &
PDF_CORE_PID=$!
echo "✓ pdf-core started (PID $PDF_CORE_PID) — log: /tmp/pdf-core-live.log"

# Wait for pdf-core to be ready before starting the backend (the backend
# probes /version at startup and will Fatalf if pdf-core is unreachable).
echo "Waiting for pdf-core to become ready..."
for i in $(seq 1 15); do
  if curl -sf http://localhost:8080/version >/dev/null 2>&1; then
    echo "✓ pdf-core ready"
    break
  fi
  if [ "$i" -eq 15 ]; then
    echo "error: pdf-core did not become ready in time" >&2
    exit 1
  fi
  sleep 1
done

# Signing is wallet-driven (ADR-12): the DCS holds no signing key. It prepares
# the to-be-signed document, and validates + records whatever the signatory
# signs. Local signing therefore NEEDS a reachable EU DSS (the signature
# validator, DCS-FR-SM-18) and the test wallet (which signs with a per-signatory
# key, driving the DSS as its external SCA). Both are provisioned here by default
# so devs can always sign locally — DCS_DEV_DSS=0 opts out only if you know you
# don't need signing.
DSS_LOCAL_URL="http://localhost:18099"
WALLET_KEYS_DIR="$HOME/.dcs/wallet-keys"
if [ "${DCS_DEV_DSS:-1}" = "1" ]; then
  echo ""
  echo "=== Starting the local EU DSS (signature validation + test-wallet SCA) ==="
  docker rm -f dcs-dev-dss >/dev/null 2>&1 || true
  docker run -d --name dcs-dev-dss -p 18099:8080 \
    --entrypoint /dss/apache-tomcat-11.0.4/bin/catalina.sh \
    conectx/dss-demo:6.2.1 run >/dev/null
  echo "✓ DSS 6.2 demo webapp starting on $DSS_LOCAL_URL (boots in ~90s)"
  echo "DSS_URL=$DSS_LOCAL_URL" >> backend/.env
  mkdir -p "$WALLET_KEYS_DIR"
  echo "✓ backend .env wired to the DSS validator; test-wallet keys dir: $WALLET_KEYS_DIR"
  echo ""
  echo "  Sign a contract locally with the test wallet (plays wallet+QTSP):"
  echo "    make -C testWallet sign DCS_URL=http://localhost:8991/api \\"
  echo "      CONTRACT_DID=<did> FIELD=<field> USER=<signatory> TOKEN=<jwt>"
fi

echo ""
echo "=== Starting Vite dev server ==="
cd frontend/ClientApp
npm run dev &
VITE_PID=$!
cd ../..

sleep 2

cd backend
goa gen digital-contracting-service/design
echo ""
echo "=== Starting backend (air) ==="
air &> /tmp/backend-live.log &
BACKEND_PID=$!

# All three processes are long-lived. The first one that exits is therefore a
# terminal stack event, not something to hide while another dev server keeps
# running. In particular, propagate backend startup-gate failures immediately.
set +e
wait -n "$VITE_PID" "$PDF_CORE_PID" "$BACKEND_PID"
EXIT_STATUS=$?
set -e

if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
  echo "Backend exited; startup diagnostics:" >&2
  tail -n 200 /tmp/backend-live.log >&2 || true
elif ! kill -0 "$PDF_CORE_PID" 2>/dev/null; then
  echo "pdf-core exited; diagnostics:" >&2
  tail -n 200 /tmp/pdf-core-live.log >&2 || true
else
  echo "Vite dev server exited unexpectedly." >&2
fi

if [[ "$EXIT_STATUS" -eq 0 ]]; then
  exit 1
fi
exit "$EXIT_STATUS"
