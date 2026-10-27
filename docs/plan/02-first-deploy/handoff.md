# Step 02 handoff: First deploy to Cloud Run

## What was built

- **`scripts/deploy.sh`**: manual, repeatable deploy. Builds with Cloud Build, deploys to Cloud Run, then checks `/api/health` on the live URL. Runs in Git Bash on Windows, Linux and macOS.
- **`scripts/deploy-config.sh`**: the non-secret settings the script sources (project, region, names, `BASE_URL`, `SPOTIFY_CLIENT_ID`). Committed.
- **`.gcloudignore`**: keeps `.git`, `.github`, `.claude`, `docs` and everything git-ignored (including `.env`) out of the Cloud Build upload. The Docker build then applies `.dockerignore` as before.
- **`docs/plan/02-first-deploy/setup.md`**: the owner's checklist for the one-off Google Cloud setup, repeatable from scratch.
- README "Deploying" section.

The owner ran setup.md end to end. The service is live and passes the checks below.

## Deployment facts

| What | Value |
|---|---|
| Project ID | `setlisted-2026` (project number `613938414647`) |
| Region | `europe-west1` (Belgium) |
| Cloud Run service | `setlisted` |
| URL (`BASE_URL`) | `https://setlisted-613938414647.europe-west1.run.app` |
| Artifact Registry repo | `europe-west1-docker.pkg.dev/setlisted-2026/setlisted` (images: `…/setlisted:<full commit SHA>`) |
| Runtime service account | `setlisted-runtime@setlisted-2026.iam.gserviceaccount.com`. No project-level roles, only `roles/secretmanager.secretAccessor` on each of the three secrets. |
| Scaling | max instances 1, min instances 0 |
| Access | `--allow-unauthenticated` (public site; access control is Spotify login, step 04) |
| Budget | £1/month on the billing account, alerts at 50%, 90% and 100% |

**Spotify redirect URL for step 04:** `https://setlisted-613938414647.europe-west1.run.app/auth/callback`.

### Why europe-west1

Checked against Google's documentation on 2026-10-27 (details in setup.md §5):

- Cloud Run's free allowance is per billing account and applies in every region.
- Regions are priced in two tiers, and Tier 2 usage uses up the free allowance faster. `europe-west2` (London) is Tier 2. `europe-west1` is Tier 1, the same as the US regions.
- Belgium therefore gets the same free headroom as a US region while staying close to UK users.

## Secrets

All three are in Secret Manager with automatic replication and are referenced by the service as `name:latest`:

| Secret | Env var | State |
|---|---|---|
| `session-key` | `SESSION_KEY` | Real random value (`openssl rand -base64 32`) |
| `setlistfm-api-key` | `SETLISTFM_API_KEY` | Placeholder, replaced in step 05 |
| `spotify-client-secret` | `SPOTIFY_CLIENT_SECRET` | Placeholder, replaced in step 04 |

`SPOTIFY_CLIENT_ID` is a non-secret placeholder (`placeholder-set-in-step-04`) in `scripts/deploy-config.sh`. Step 04 sets the real value there.

To replace a secret: add a version, disable the old one (the free tier covers 6 active versions), then redeploy. Secrets exposed as environment variables are read when an instance starts, so running instances keep the old value until a new revision starts. The commands are in setup.md §8.

## How to deploy

From a clean checkout, in Git Bash (not the WSL `bash` on Windows), logged in with `gcloud auth login` as the owner:

```bash
scripts/deploy.sh
```

The script refuses to run while there are uncommitted changes (the image tag is the commit SHA) or while any setting in `deploy-config.sh` still holds a `REPLACE_ME` placeholder. Running it again deploys a new revision. The owner checked this.

## Decisions taken in this step

- **Settings in a sourced bash file** (`scripts/deploy-config.sh`), not script variables, so step 03 and humans can read the same values.
- **The deploy is declarative.** `--set-env-vars` and `--set-secrets` replace the full set on every deploy, so the script is the single source of truth for the service's configuration.
- **Cloud Build uses the global default pool** (`gcloud builds submit` without `--region`), to avoid regional default-pool restrictions on new projects. The image still goes to the `europe-west1` Artifact Registry. Cloud Build creates a `setlisted-2026_cloudbuild` source bucket automatically.
- **The full 40-character SHA is the image tag**, matching `${{ github.sha }}` in step 03, so manual and automated images are named the same way.
- **The script strips a trailing `/` from `BASE_URL`.** Otherwise the health check would request `//api/health`.
- **Memory and CPU are left at Cloud Run's defaults.** Not tuned for this step.

## Notes for step 03

- `gcloud run deploy setlisted --image … --region europe-west1` with **only** `--image` keeps the existing service account, env vars, secrets and scaling. The workflow doesn't have to repeat them, but if it does, it must match `scripts/deploy-config.sh`.
- The deploy service account will need:
  - to push to the `setlisted` Artifact Registry repo;
  - to deploy the `setlisted` Cloud Run service;
  - `iam.serviceAccountUser` on `setlisted-runtime` (to deploy as it).

  It should **not** get Secret Manager access (ADR-008: only the runtime account reads secrets).
- Project ID and region for the GitHub repository variables: `setlisted-2026`, `europe-west1`.
- `scripts/deploy.sh` remains the documented fallback.

## Surprises

- **"Console" was ambiguous** in the first draft of setup.md. It now says it means the Google Cloud web console, and links the Billing and Budgets pages.
- **New projects don't show in the console automatically.** A project created with `gcloud` didn't appear until it was picked in the project picker (All tab, under "No organization").
- **`bash` on Windows** resolves to the WSL `bash.exe` in `System32` from PowerShell. Use Git Bash.

## Known gaps

- No automated deploys yet (step 03).
- No Artifact Registry clean-up policy yet (step 03).
- Two secrets and the Spotify client ID are placeholders (steps 04 and 05).
