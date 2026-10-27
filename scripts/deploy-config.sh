# Non-secret deploy settings, read by scripts/deploy.sh.
# Nothing in this file is secret: secrets live in Secret Manager (ADR-007).
# See docs/plan/02-first-deploy/setup.md for how each value is chosen.

# Google Cloud project ID.
PROJECT_ID=REPLACE_ME

# Cloud Run, Artifact Registry and Cloud Build region.
REGION=europe-west1

# Cloud Run service and Artifact Registry repository names.
SERVICE=setlisted
REPOSITORY=setlisted

# Runtime identity of the Cloud Run service. The only identity that can read the secrets.
RUNTIME_SERVICE_ACCOUNT="setlisted-runtime@${PROJECT_ID}.iam.gserviceaccount.com"

# Public URL of the service: https://<service>-<project-number>.<region>.run.app
BASE_URL=REPLACE_ME

# Spotify app client ID. Not secret. Placeholder until step 04.
SPOTIFY_CLIENT_ID=placeholder-set-in-step-04
