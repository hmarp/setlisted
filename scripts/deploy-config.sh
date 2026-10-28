# Non-secret deploy settings, read by scripts/deploy.sh.
# Nothing in this file is secret: secrets live in Secret Manager (ADR-007).
# See docs/plan/02-first-deploy/setup.md for how each value is chosen.

# Google Cloud project ID.
PROJECT_ID=setlisted-2026

# Cloud Run, Artifact Registry and Cloud Build region.
REGION=europe-west1

# Cloud Run service and Artifact Registry repository names.
SERVICE=setlisted
REPOSITORY=setlisted

# Runtime identity of the Cloud Run service. The only identity that can read the secrets.
RUNTIME_SERVICE_ACCOUNT="setlisted-runtime@${PROJECT_ID}.iam.gserviceaccount.com"

# Public URL of the service: https://<service>-<project-number>.<region>.run.app
BASE_URL=https://setlisted-613938414647.europe-west1.run.app

# Spotify app client ID (Spotify Dashboard → app → Settings). Not secret.
# Merges don't change env vars: after changing it, apply it once (see docs/plan/04-spotify-login/setup.md §6).
SPOTIFY_CLIENT_ID=2d7607fbbdd9462892990e71494eefd9
