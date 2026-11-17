# Step 02: First deploy to Cloud Run

- **Depends on:** step 01
- **Branch:** `step-02-first-deploy`
- **Handoffs to read:** `docs/plan/01-go-skeleton/handoff.md`

## Goal

Prove hosting, secrets and HTTPS **early**, while there's very little code to debug. At the end of this step, the step 01 skeleton runs on Cloud Run at its `*.run.app` HTTPS address, gets its configuration from Secret Manager, and can be redeployed with one script.

## Decisions this step relies on

- **ADR-003: Cloud Run.**
  - The app runs as a container on Cloud Run, scaling to zero, at the default `*.run.app` HTTPS address. No custom domain in the MVP.
  - **Cost guards:**
    - a **budget alert at £1/month** (alerts warn, they don't stop spending);
    - **maximum instances = 1** as a hard ceiling on compute;
    - **minimum instances = 0**.
  - **The region is decided in this step**, after confirming how the always-free allowance applies per region. The trade-off is a US region (the most generous free allowance) against a European one (e.g. `europe-west2`, London, closer to the users).
  - The service is stateless.
  - The deployment mechanism is decided at the plan stage. The plan says: a manual, repeatable deploy script now, and automated deploys in step 03, with this script kept as the fallback.
- **ADR-007: Configuration and secrets.**
  - Secrets are stored in **Google Secret Manager** and exposed to Cloud Run as environment variables.
  - Non-secret configuration is set directly on the Cloud Run service.
  - The Cloud Run **runtime service account** needs access to the secrets.
  - Nothing secret goes in the repo or the image.
- **ADR-008 (preparing for step 03).** The runtime service account is separate from the deploy service account created in step 03, and is the only identity that can read the secrets.

## Owner tasks

The session prepares exact commands and instructions. **The owner does the steps that involve billing or account access.** Put the checklist in `docs/plan/02-first-deploy/setup.md` so it can be repeated.

1. Install the Google Cloud CLI and run `gcloud auth login`, if not already done.
2. Create a Google Cloud project (e.g. `setlisted`) and link a billing account. **The owner enters the card details.**
3. Create a **budget of £1/month** on the billing account with email alerts (e.g. at 50%, 90% and 100%).
4. Enable the APIs: Cloud Run, Artifact Registry, Cloud Build, Secret Manager.
5. **Decide the region.** The session checks Google's current Cloud Run pricing and free-tier documentation and recommends one. The owner chooses, and the handoff records the choice and why.
6. Create an Artifact Registry Docker repository (e.g. `setlisted`) in that region.
7. Create a runtime service account (e.g. `setlisted-runtime`) with only `roles/secretmanager.secretAccessor` on the app's secrets.
8. Create the secrets `setlistfm-api-key`, `spotify-client-secret` and `session-key`:
   - `session-key`: a real random value (`openssl rand -base64 32`);
   - the other two: real values if the owner already has them, otherwise placeholders, replaced in steps 04 and 05.

## What to build

1. **`scripts/deploy.sh`**, a repeatable manual deploy. It must run in Git Bash on Windows as well as on Linux and macOS. It should:
   - build the image with **Cloud Build** (`gcloud builds submit --tag <region>-docker.pkg.dev/<project>/setlisted/setlisted:<tag>`), so no local Docker is needed. Tag with the current commit SHA, and refuse to deploy if there are uncommitted changes;
   - deploy with `gcloud run deploy setlisted`, passing:
     - `--image <that tag>`
     - `--region <region>`
     - `--service-account setlisted-runtime@…`
     - `--allow-unauthenticated` (it's a public website, and access control happens at Spotify login)
     - `--max-instances 1`
     - `--min-instances 0`
     - `--set-env-vars BASE_URL=…,SPOTIFY_CLIENT_ID=…`
     - `--set-secrets SETLISTFM_API_KEY=setlistfm-api-key:latest,SPOTIFY_CLIENT_SECRET=spotify-client-secret:latest,SESSION_KEY=session-key:latest`

   Read the project, region and non-secret values from script variables or a small **non-secret** config file committed to the repo, never from secrets.
2. **`BASE_URL`.** Cloud Run service URLs follow the pattern `https://<service>-<project-number>.<region>.run.app`, so the URL can be worked out before the first deploy. If it can't, deploy once, read the URL, then set `BASE_URL`. Record the final URL in the handoff. It's needed for the Spotify redirect URL in step 04.
3. If `SPOTIFY_CLIENT_ID` doesn't exist yet, use a placeholder, which is fine for this step. Step 04 sets the real value.

## Out of scope

- Automated deployment from GitHub (step 03).
- Spotify developer app setup (step 04).
- Any application features.

## Done when

- `https://<service-url>/api/health` returns `{"status":"ok"}` over HTTPS, and `/` serves the placeholder page.
- The Cloud Run service shows max instances 1, min 0, the runtime service account, and secrets referenced from Secret Manager (not plain environment values).
- The budget alert exists.
- Running `scripts/deploy.sh` again deploys a new revision.
- Cloud Logging shows request logs with no secret values.

## End of step (same PR)

- Write `handoff.md`, including:
  - the project ID, region and reason, service name and URL;
  - the Artifact Registry repo path;
  - the runtime service account;
  - the secret names, and which are still placeholders;
  - how to run the deploy script;
  - anything surprising.
- Mark step 02 done in `docs/plan.md`.
- Update `README.md` with a "Deploying" section: the one-off setup (linking to `setup.md`) and the deploy script.
