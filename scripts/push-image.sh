#!/usr/bin/env bash
# ABOUTME: builds and pushes the Daylight Docker image to Docker Hub
# ABOUTME: tags with :latest and an optional version (default: git short SHA); builds linux/amd64+arm64
set -euo pipefail

REPO="daylightreview/daylight"
SHA=$(git rev-parse --short HEAD)
VERSION="${1:-$SHA}"

echo "Building $REPO:$VERSION ..."
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --push \
  -t "$REPO:latest" \
  -t "$REPO:$VERSION" \
  .

echo "Done: $REPO:latest ($VERSION)"
