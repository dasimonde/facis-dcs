#-- Frontend Build Stage --
FROM node:22-alpine AS frontend-builder

WORKDIR /frontend

# Copy frontend package files
COPY frontend/ClientApp/package*.json ./
COPY frontend/ClientApp/package-lock.json ./

# Install frontend dependencies
RUN npm ci

# Copy frontend source
COPY frontend/ClientApp/ ./

# Build frontend
RUN npm run build


#-- Go Build Stage --
FROM golang:1.25-alpine AS go-builder

WORKDIR /build

# Copy go mod files
COPY backend/go.mod backend/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY backend/ ./

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o dcs ./cmd/dcs


#-- Runtime Stage --
FROM debian:stable-slim

ARG DOCKER_USER=default_user

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    wget \
    gnupg && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -r "${DOCKER_USER}" && useradd -r -g "${DOCKER_USER}" "${DOCKER_USER}"

RUN mkdir -p /home/${DOCKER_USER}
RUN chmod 0777 /home/${DOCKER_USER}

RUN mkdir -p /app
RUN chown $DOCKER_USER:$DOCKER_USER /app

# Create directory for custom CA certificates
RUN mkdir -p /usr/local/share/ca-certificates/custom
RUN chmod 755 /usr/local/share/ca-certificates/custom

WORKDIR /app

USER $DOCKER_USER

# Copy compiled binary from go-builder stage
COPY --from=go-builder /build/dcs /app/dcs

# Copy generated OpenAPI spec from go-builder stage
COPY --from=go-builder /build/gen/http/openapi3.json /app/gen/http/openapi3.json

# Copy built frontend from frontend-builder stage
COPY --from=frontend-builder --chown=$DOCKER_USER:$DOCKER_USER /frontend/dist /app/web/dist

# Copy frontend config injection script
COPY --chown=$DOCKER_USER:$DOCKER_USER frontend/ClientApp/inject-config.sh /app/web/
RUN chmod +x /app/web/inject-config.sh

# Copy entrypoint script
COPY --chown=$DOCKER_USER:$DOCKER_USER backend/docker-entrypoint.sh /app/
RUN chmod +x /app/docker-entrypoint.sh

# Environment variable declarations (no defaults - must be provided at runtime)
ENV DATABASE_URL=""
ENV NATS_URL=""
ENV OIDC_ISSUER_URL=""
ENV OIDC_CLIENT_ID=""
ENV OIDC_REDIRECT_URI=""
ENV OIDC_LOGOUT_REDIRECT_URI=""
ENV FEDERATED_CATALOGUE_API_URL=""

# Frontend and Backend path configuration (production defaults)
ENV DCS_UI_PATH="/ui/"
ENV DCS_API_PATH="/"

EXPOSE 8991

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/dcs"]