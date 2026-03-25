#!/usr/bin/env bash
set -euo pipefail

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

usage() {
  echo "Usage: $0 <kubeconfig> <private_key_path> <crt_path> <domain> <path> <oidc_issuer_url> <oidc_client_id>"
  echo "Example: $0 ~/.kube/config ./certs/dev.key ./certs/dev.crt xfsc.local dcs https://keycloak.xfsc.local/realms/dcs digital-contracting-service"
  echo ""
  echo "Optional environment variables:"
  echo "  OIDC_REDIRECT_URI - Redirect URI for OIDC flow (default: http://localhost:8991)"
  echo "  OIDC_LOGOUT_REDIRECT_URI - Logout redirect URI (default: same as frontend URL)"
  echo "  API_PATH_PREFIX - API path prefix forwarded by reverse proxy (default: empty)"
  echo "  FEDERATED_CATALOGUE_API_URL - Federated Catalogue API base URL (default: empty)"
  exit 1
}

# Input validation
[ "$#" -ne 7 ] && usage
export KUBECONFIG="$1"
KEY_FILE="$2"
CRT_FILE="$3"
DOMAIN="$4"
URL_PATH="$5"
OIDC_ISSUER_URL="$6"
OIDC_CLIENT_ID="$7"
OIDC_REDIRECT_URI="${OIDC_REDIRECT_URI:-http://localhost:8991}"
OIDC_LOGOUT_REDIRECT_URI="${OIDC_LOGOUT_REDIRECT_URI:-}"
API_PATH_PREFIX="${API_PATH_PREFIX:-}"
FEDERATED_CATALOGUE_API_URL="${FEDERATED_CATALOGUE_API_URL:-}"

# If OIDC_LOGOUT_REDIRECT_URI is not set, derive it from DOMAIN and PATH
if [[ -z "$OIDC_LOGOUT_REDIRECT_URI" ]]; then
  OIDC_LOGOUT_REDIRECT_URI="https://${DOMAIN}/${URL_PATH}/auth/logout-complete"
fi

# Image Registry Configuration
DOCKER_REGISTRY="${DOCKER_REGISTRY:-}"
DOCKER_REPO="${DOCKER_REPO:-}"
DOCKER_TAG="${DOCKER_TAG:-latest}"

IMAGE_NAME="digital-contracting-service"
if [[ -n "$DOCKER_REGISTRY" && -n "$DOCKER_REPO" ]]; then
  IMAGE_NAME="$DOCKER_REGISTRY/$DOCKER_REPO/digital-contracting-service"
fi
log "ℹ️ Image: $IMAGE_NAME:$DOCKER_TAG"

#----------------------------------------
# Custom CA Configuration
#----------------------------------------
CUSTOM_CA_ENABLED="${CUSTOM_CA_ENABLED:-false}"
CUSTOM_CA_CONFIGMAP="${CUSTOM_CA_CONFIGMAP:-dev-ca-cert}"
CUSTOM_CA_CERT_FILE="${CUSTOM_CA_CERT_FILE:-}"

# Host Aliases Configuration for local Keycloak access
# Extract hostname from OIDC_ISSUER_URL if it uses HTTPS
KEYCLOAK_HOSTNAME=""
if [[ "${OIDC_ISSUER_URL:-}" =~ ^https://([^/]+) ]]; then
  KEYCLOAK_HOSTNAME="${BASH_REMATCH[1]}"
  log "ℹ️ Detected HTTPS OIDC issuer, will configure host alias for: $KEYCLOAK_HOSTNAME"
  
  # Get Traefik ClusterIP for in-cluster resolution
  TRAEFIK_CLUSTER_IP=$(kubectl get svc -n kube-system traefik -o jsonpath='{.spec.clusterIP}' --kubeconfig "$KUBECONFIG" 2>/dev/null || echo "")
  if [[ -n "$TRAEFIK_CLUSTER_IP" ]]; then
    log "ℹ️ Traefik ClusterIP: $TRAEFIK_CLUSTER_IP"
    HOST_ALIAS_ENABLED="true"
  else
    log "⚠️ Could not detect Traefik ClusterIP, skipping host alias configuration"
    HOST_ALIAS_ENABLED="false"
  fi
else
  HOST_ALIAS_ENABLED="false"
fi

log "ℹ️ OIDC Configuration:"
log "  - Issuer URL (for backend): $OIDC_ISSUER_URL"
log "  - Client ID: $OIDC_CLIENT_ID"
log "  - Redirect URI: $OIDC_REDIRECT_URI"
log "  - Logout Redirect URI: $OIDC_LOGOUT_REDIRECT_URI"
log "  - API Path Prefix: ${API_PATH_PREFIX:-<empty>}"
log "  - Federated Catalogue API URL: ${FEDERATED_CATALOGUE_API_URL:-<empty>}"

if [[ ! -f "$KUBECONFIG" ]]; then
  log "❌ Kubeconfig file not found: $KUBECONFIG"
  exit 1
fi


# Cleanup local helm artifacts
if [ -f Chart.lock ]; then
  rm Chart.lock
  log "✅ Removed Chart.lock"
fi

if [ -d charts ]; then
  rm -rf charts
  log "✅ Removed charts/ directory"
fi

# Check dependencies
for cmd in kubectl helm jq curl sed trap; do
  if ! command -v "$cmd" &>/dev/null; then
    log "❌ '$cmd' is not installed. Please install it and retry."
    exit 1
  else
    log "✅ Found '$cmd'"
  fi
done

# Verify ingress class traefik is installed
log "ℹ Checking for ingressClass traefik..."
if ! kubectl get ingressclass traefik &>/dev/null; then
  log "❌ Ingress class traefik not found"
  exit 1
else
    log "✅ Ingress class traefik found"
fi

# Generate and validate namespace from path
NAMESPACE="digital-contracting-service-${URL_PATH}"
log "ℹ️ Using namespace: $NAMESPACE"

# Create namespace first
kubectl create namespace "$NAMESPACE" --kubeconfig "$KUBECONFIG" 2>/dev/null || true
log "✅ Namespace created or already exists"

#----------------------------------------
# Create Custom CA ConfigMap if enabled
#----------------------------------------
if [[ "$CUSTOM_CA_ENABLED" == "true" ]]; then
  if [[ -z "$CUSTOM_CA_CERT_FILE" ]]; then
    log "❌ CUSTOM_CA_ENABLED is true but CUSTOM_CA_CERT_FILE is not set"
    exit 1
  fi
  if [[ ! -f "$CUSTOM_CA_CERT_FILE" ]]; then
    log "❌ CA certificate file not found: $CUSTOM_CA_CERT_FILE"
    exit 1
  fi
  log "ℹ️ Creating ConfigMap '$CUSTOM_CA_CONFIGMAP' with CA certificate"
  kubectl create configmap "$CUSTOM_CA_CONFIGMAP" \
    --from-file=dev-ca.crt="$CUSTOM_CA_CERT_FILE" \
    -n "$NAMESPACE" \
    --kubeconfig "$KUBECONFIG" \
    --dry-run=client -o yaml | kubectl apply -f - --kubeconfig "$KUBECONFIG"
  log "✅ ConfigMap created or updated"
fi

# Prepare temporary values file
TMP_VALUES="$(mktemp -t values.XXXXXX.yaml)" || TMP_VALUES="/tmp/values-$$.yaml"
cp values.yaml "$TMP_VALUES"
log "ℹ️ Replacing placeholders in $TMP_VALUES"
sed -i \
  -e "s|\[domain-name\]|${DOMAIN}|g" \
  -e "s|\[path\]|${URL_PATH}|g" \
  -e "s|\[namespace\]|${NAMESPACE}|g" \
  -e "s|\[oidc-issuer-url\]|${OIDC_ISSUER_URL}|g" \
  -e "s|\[oidc-client-id\]|${OIDC_CLIENT_ID}|g" \
  -e "s|\[oidc-redirect-uri\]|${OIDC_REDIRECT_URI}|g" \
  -e "s|\[oidc-logout-redirect-uri\]|${OIDC_LOGOUT_REDIRECT_URI}|g" \
  -e "s|\[api-path-prefix\]|${API_PATH_PREFIX}|g" \
  -e "s|\[registry\]|${IMAGE_NAME}|g" \
  -e "s|tag: \"latest\"|tag: \"${DOCKER_TAG}\"|g" \
  -e "s|enabled: false|enabled: ${CUSTOM_CA_ENABLED}|g" \
  -e "s|configMapName: \"\"|configMapName: \"${CUSTOM_CA_CONFIGMAP}\"|g" \
  "$TMP_VALUES"

# Add hostAliases if HTTPS OIDC is configured
if [[ "$HOST_ALIAS_ENABLED" == "true" ]]; then
  log "ℹ️ Adding hostAlias: $KEYCLOAK_HOSTNAME -> $TRAEFIK_CLUSTER_IP"
  cat >> "$TMP_VALUES" <<EOF

# Auto-configured host alias for in-cluster OIDC access
hostAliases:
  - ip: "${TRAEFIK_CLUSTER_IP}"
    hostnames:
      - "${KEYCLOAK_HOSTNAME}"
EOF
fi

# Add Federated Catalogue API URL override if set.
if [[ -n "$FEDERATED_CATALOGUE_API_URL" ]]; then
  log "ℹ️ Setting Federated Catalogue API URL override"
  cat >> "$TMP_VALUES" <<EOF

federatedCatalogue:
  apiURL: "${FEDERATED_CATALOGUE_API_URL}"
EOF
fi

log "✅ Placeholders replaced in $TMP_VALUES"

# Helm dependency build & install
log "ℹ️ Running: helm dependency build"
helm dependency build . --kubeconfig "$KUBECONFIG"

log "ℹ️ Installing digital-contracting-service via Helm"
helm install digital-contracting-service . \
  --namespace "$NAMESPACE" \
  --create-namespace \
  --kubeconfig "$KUBECONFIG" \
  -f "$TMP_VALUES"
log "✅ digital-contracting-service Helm release deployed"

# Create TLS secret
log "ℹ️ Creating TLS secret 'certificates'"
kubectl create secret tls certificates \
  --namespace "$NAMESPACE" \
  --key "$KEY_FILE" \
  --cert "$CRT_FILE" \
  --kubeconfig "$KUBECONFIG"
log "✅ TLS secret created"

# Create shared TLS secret for ingress (dev-wildcard-tls)
log "ℹ️ Creating shared TLS secret 'dev-wildcard-tls' for ingress"
kubectl create secret tls dev-wildcard-tls \
  --cert="$CRT_FILE" \
  --key="$KEY_FILE" \
  -n "$NAMESPACE" \
  --kubeconfig "$KUBECONFIG" \
  --dry-run=client -o yaml | kubectl apply -f - --kubeconfig "$KUBECONFIG"
log "✅ Shared TLS secret created or updated"

# Wait for Deployment to be ready
log "ℹ️ Waiting for digital-contracting-service deployment to be ready (max 2m)..."
if ! kubectl rollout status deployment/digital-contracting-service \
     -n "$NAMESPACE" \
     --timeout=300s \
     --kubeconfig "$KUBECONFIG"; then
  log "❌ Timeout waiting for digital-contracting-service. Pod statuses:"
  kubectl get pods -n "$NAMESPACE" -o wide --kubeconfig "$KUBECONFIG"
  exit 1
fi
log "✅ digital-contracting-service deployment is ready"

# Final output
log "🎉 All operations completed successfully!"
echo
echo "🔹 DCS URL: https://${DOMAIN}/${URL_PATH}"
echo ""
log "ℹ️ Before accessing the service, ensure Keycloak is configured:"
log "   1. OIDC Issuer: ${OIDC_ISSUER_URL}"
log "   2. Client ID: ${OIDC_CLIENT_ID}"
log "   3. Valid Redirect URI: ${OIDC_REDIRECT_URI}"
log "   4. Valid post logout redirect URI: ${OIDC_LOGOUT_REDIRECT_URI}"
log "   5. Create users and assign roles in Keycloak admin console"
log ""
log "ℹ️ See README.md for detailed Keycloak setup instructions"
