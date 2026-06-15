#!/usr/bin/env bash
# ABOUTME: builds and pushes the Daylight Docker image to Docker Hub
# ABOUTME: tags with both :latest and the current git short SHA; builds linux/amd64+arm64
set -euo pipefail

REPO="pmonierdev/daylight"
SHA=$(git rev-parse --short HEAD)

echo "Building $REPO:$SHA ..."
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --push \
  -t "$REPO:latest" \
  -t "$REPO:$SHA" \
  .

echo "Done: $REPO:latest ($SHA)"
