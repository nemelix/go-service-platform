#!/usr/bin/env bash
# Deploys the given image tag of go-service-platform.
# Called via SSH forced command: the tag arrives in SSH_ORIGINAL_COMMAND.
set -euo pipefail

APP_DIR=/opt/go-service-platform
TAG="${SSH_ORIGINAL_COMMAND:-${1:-}}"

if [[ ! "$TAG" =~ ^[0-9a-f]{40}$ ]]; then
    echo "error: expected a 40-char commit sha, got '${TAG}'" >&2
    exit 1
fi

cd "$APP_DIR"
echo "APP_TAG=${TAG}" > .env

docker compose pull app
docker compose up -d app
docker compose exec -T nginx nginx -s reload
docker image prune -f > /dev/null

echo "deployed ${TAG}"