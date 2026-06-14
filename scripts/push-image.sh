#!/usr/bin/env bash
# ABOUTME: builds and pushes the Daylight Docker image to Docker Hub
# ABOUTME: tags with both :latest and the current git short SHA
set -euo pipefail

REPO="pmonierdev/daylight"
SHA=$(git rev-parse --short HEAD)

echo "Building $REPO:$SHA ..."
docker build -t "$REPO:latest" -t "$REPO:$SHA" .

echo "Pushing ..."
docker push "$REPO:latest"
docker push "$REPO:$SHA"

echo "Done: $REPO:latest ($SHA)"
