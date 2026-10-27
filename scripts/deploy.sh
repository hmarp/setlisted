#!/usr/bin/env bash
# Manual deploy to Cloud Run: build the image with Cloud Build, deploy it,
# then check /api/health. Runs in Git Bash on Windows, Linux and macOS.
# Needs: gcloud (logged in as the owner), git, curl.
#
# Usage: scripts/deploy.sh
#
# Settings come from scripts/deploy-config.sh (non-secret). Secrets are
# referenced from Secret Manager and never pass through this script.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# shellcheck source=deploy-config.sh
source scripts/deploy-config.sh

for name in PROJECT_ID REGION SERVICE REPOSITORY RUNTIME_SERVICE_ACCOUNT BASE_URL SPOTIFY_CLIENT_ID; do
  value="${!name:-}"
  if [[ -z "$value" || "$value" == *REPLACE_ME* ]]; then
    echo "deploy: $name is not set in scripts/deploy-config.sh" >&2
    exit 1
  fi
done

for cmd in gcloud git curl; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "deploy: $cmd not found on PATH" >&2
    exit 1
  fi
done

# The image tag is the commit SHA, so the deployed code must be exactly that commit.
if [[ -n "$(git status --porcelain)" ]]; then
  echo "deploy: uncommitted changes. Commit or stash them first:" >&2
  git status --short >&2
  exit 1
fi

sha="$(git rev-parse HEAD)"
image="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/${SERVICE}:${sha}"

echo "deploy: building $image"
# No --region: the global default pool avoids per-region quotas on new projects.
gcloud builds submit . \
  --project "$PROJECT_ID" \
  --tag "$image" \
  --quiet

echo "deploy: deploying $SERVICE to $REGION"
gcloud run deploy "$SERVICE" \
  --project "$PROJECT_ID" \
  --region "$REGION" \
  --image "$image" \
  --service-account "$RUNTIME_SERVICE_ACCOUNT" \
  --allow-unauthenticated \
  --max-instances 1 \
  --min-instances 0 \
  --set-env-vars "BASE_URL=${BASE_URL},SPOTIFY_CLIENT_ID=${SPOTIFY_CLIENT_ID}" \
  --set-secrets "SETLISTFM_API_KEY=setlistfm-api-key:latest,SPOTIFY_CLIENT_SECRET=spotify-client-secret:latest,SESSION_KEY=session-key:latest" \
  --quiet

echo "deploy: checking ${BASE_URL}/api/health"
health="$(curl -fsS --retry 3 --retry-delay 2 "${BASE_URL}/api/health")"
if [[ "$health" != '{"status":"ok"}' ]]; then
  echo "deploy: unexpected health response: $health" >&2
  exit 1
fi
echo "deploy: done. $SERVICE is serving commit $sha at $BASE_URL"
