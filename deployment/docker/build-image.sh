#!/bin/bash

# Parameters
# $1 - Optional tag for the Docker image (default: latest)

# Environment
# DOCKER_REGISTRY - Optional Docker registry URL
# DOCKER_REPO - Optional Docker repository name
# GIT_SHA - Optional commit SHA. Defaults to git rev-parse HEAD at the repo root.
# BUILD_TIME - Optional RFC3339 build timestamp. Defaults to UTC now when unset.

set -euo pipefail

# Set defaults if arguments not provided
TAG=${1:-latest}

# Set registry and repo from environment variables if available
REGISTRY=${DOCKER_REGISTRY:-}
REPO=${DOCKER_REPO:-}

IMAGE_NAME="digital-contracting-service:$TAG"

if [[ -n "$REGISTRY" && -n "$REPO" ]]; then
  IMAGE_NAME="$REGISTRY/$REPO/digital-contracting-service:$TAG"
fi

echo "Building $IMAGE_NAME..."
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
DOCKERFILE="$(dirname "$0")/Dockerfile"

GIT_SHA="${GIT_SHA:-$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || true)}"
GIT_SHA="${GIT_SHA:-unknown}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)}"

# Build context is the repo root to include both backend and frontend
docker build \
  -f "$DOCKERFILE" \
  --build-arg "GIT_SHA=$GIT_SHA" \
  --build-arg "BUILD_TIME=$BUILD_TIME" \
  -t "$IMAGE_NAME" \
  "$REPO_ROOT"

if [[ -n "$REGISTRY" && -n "$REPO" ]]; then
  echo "Tagging as latest..."
  docker tag "$IMAGE_NAME" "$REGISTRY/$REPO/digital-contracting-service:latest"
  
  echo "Pushing to $REGISTRY..."
  docker push "$IMAGE_NAME"
  docker push "$REGISTRY/$REPO/digital-contracting-service:latest"
else
  echo "Skipping push (REGISTRY and REPO aren't set)"
fi