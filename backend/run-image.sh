#!/usr/bin/env bash
TAG=${1:-latest}
SERVICE_PREFIX=${2:-}  # Optional path prefix (e.g., /production/dcs or empty for /)
REPO=${DOCKER_REPO:-facis}
REGISTRY=${DOCKER_REGISTRY:-h6s71ks6.c1.de1.container-registry.ovh.net}

# Derived paths from SERVICE_PREFIX
DCS_API_PATH="${SERVICE_PREFIX}/api"
DCS_UI_PATH="${SERVICE_PREFIX}/ui"

docker run -d \
  --name dcs \
  -p 8991:8991 \
  -v ../deployment/certs/dev.crt:/usr/local/share/ca-certificates/custom/dev.crt:ro \
  -e HYDRA_CLIENT_ID=dcs-client \
  -e HYDRA_REDIRECT_URI=http://localhost:8991${DCS_API_PATH}/auth/callback \
  -e HYDRA_POST_LOGOUT_REDIRECT_URI=http://localhost:8991${DCS_API_PATH}/auth/logout-complete \
  -e DATABASE_URL="host=host.docker.internal port=5432 user=dcs password=dcs dbname=dcs sslmode=disable" \
  -e NATS_URL=nats://host.docker.internal:4222 \
  -e HYDRA_PUBLIC_ISSUER_URL=http://localhost:5173 \
  -e HYDRA_INTERNAL_ISSUER_URL=http://localhost:30444 \
  -e HYDRA_CLIENT_SECRET=dcs-secret \
  -e HYDRA_ADMIN_URL=http://localhost:30085 \
  -e FC_KEYCLOAK_REALM_URL=https://keycloak.xfsc.local/realms/gaia-x \
  -e DCS_API_PATH=${DCS_API_PATH} \
  -e DCS_UI_PATH=${DCS_UI_PATH} \
  --add-host host.docker.internal:host-gateway \
  --user default_user \
  ${REGISTRY}/${REPO}/digital-contracting-service:${TAG}