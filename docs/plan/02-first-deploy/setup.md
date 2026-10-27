# Step 02 setup: Google Cloud (owner checklist)

One-off setup for hosting Setlisted on Cloud Run. The owner runs these steps. They can be repeated to rebuild the setup from scratch (for example in a new project).

Run the commands in **Git Bash** (on Windows, not the WSL `bash` in `System32`), or any bash on Linux or macOS. Set these variables first and keep the same shell open:

```bash
PROJECT_ID=setlisted-XXXX     # globally unique; pick your own suffix
REGION=europe-west1           # see "Choose the region" below
```

## 1. Install and log in

- [ ] Install the [Google Cloud CLI](https://cloud.google.com/sdk/docs/install). On Windows, let the installer add it to `PATH`, then open a new Git Bash window.
- [ ] Log in and check:

```bash
gcloud auth login
gcloud --version
```

## 2. Project and billing (console: card details)

- [ ] Create the project:

```bash
gcloud projects create "$PROJECT_ID" --name=setlisted
gcloud config set project "$PROJECT_ID"
```

- [ ] In the console, go to **Billing**. Create a billing account if you don't have one (this is where the card details go), then **link it to the project** (Billing → Account management → My projects, or "Link a billing account" on the project's Billing page). The command-line equivalent, once the billing account exists:

```bash
gcloud billing accounts list
gcloud billing projects link "$PROJECT_ID" --billing-account=XXXXXX-XXXXXX-XXXXXX
```

## 3. £1 budget alert (console)

Budgets warn and don't stop spending. The hard cost ceiling is Cloud Run's max instances = 1 (set by the deploy script).

- [ ] Console → **Billing → Budgets & alerts → Create budget**:
  - Name: `setlisted`
  - Scope: this project (or all projects on the billing account)
  - Amount: **specified amount, £1**. The currency is the billing account's. If it isn't GBP, use the equivalent.
  - Thresholds: **50%, 90% and 100% of actual spend** (optionally 100% forecasted too).
  - Notifications: keep "Email alerts to billing admins and users" ticked.

## 4. Enable the APIs

- [ ] Enable the APIs:

```bash
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  secretmanager.googleapis.com
```

## 5. Choose the region

Checked on 2026-10-27 against Google's [Cloud Run locations](https://cloud.google.com/run/docs/locations), [Cloud Run pricing](https://cloud.google.com/run/pricing) and the [free tier list](https://cloud.google.com/free/docs/free-cloud-features):

- The Cloud Run free allowance (2 million requests, 180,000 vCPU-seconds and 360,000 GiB-seconds a month on request-based billing) is **per billing account and applies in every region**. It isn't US-only, unlike the Compute Engine e2-micro free VM.
- Regions are priced in two tiers. Usage in a **Tier 2** region costs more per unit, so it uses up the free allowance faster. **`europe-west2` (London) is Tier 2.** `europe-west1` (Belgium), `europe-west4` (Netherlands) and `us-central1` (Iowa) are Tier 1.
- The free outbound data allowance (1 GB a month) counts only traffic from North America. Our responses are small JSON and a few static files, so outbound data from Europe should cost pennies at most.
- The Artifact Registry (0.5 GB), Cloud Build (2,500 build-minutes) and Secret Manager (6 active secret versions, 10,000 accesses) free tiers don't depend on the region.

**Recommendation: `europe-west1` (Belgium).** It's Tier 1, so it gets the same free allowance as a US region, and it's close to UK users (roughly 10–20 ms further than London). At a few friends' usage, any region would stay inside the free tier. Belgium keeps the most headroom without putting the service across the Atlantic. Choose `europe-west2` if London latency matters more than headroom.

- [ ] Choose the region and set `REGION` above. Record it in `scripts/deploy-config.sh`.

## 6. Artifact Registry repository

- [ ] Create the repository:

```bash
gcloud artifacts repositories create setlisted \
  --repository-format=docker \
  --location="$REGION" \
  --description="Setlisted container images"
```

Step 03 adds a clean-up policy.

## 7. Runtime service account

The identity the Cloud Run service runs as. It gets **no project-level roles**, only access to the three secrets (step 8).

- [ ] Create the service account:

```bash
gcloud iam service-accounts create setlisted-runtime \
  --display-name="Setlisted Cloud Run runtime"
RUNTIME_SA="setlisted-runtime@${PROJECT_ID}.iam.gserviceaccount.com"
```

## 8. Secrets

Secret values never go on the command line or into shell history: they're piped in from `openssl` or from a hidden prompt.

- [ ] `session-key`: a real random value:

```bash
openssl rand -base64 32 | tr -d '\n' | gcloud secrets create session-key --data-file=-
```

- [ ] `setlistfm-api-key` and `spotify-client-secret`: a real value if you have one (hidden prompt), otherwise a placeholder. Steps 04 and 05 replace placeholders.

```bash
# Real value: type or paste it at the prompt (nothing is echoed).
read -rs -p "setlist.fm API key: " v && printf '%s' "$v" | gcloud secrets create setlistfm-api-key --data-file=-; unset v; echo

# Or a placeholder:
printf '%s' placeholder-set-in-step-05 | gcloud secrets create setlistfm-api-key --data-file=-
printf '%s' placeholder-set-in-step-04 | gcloud secrets create spotify-client-secret --data-file=-
```

- [ ] Give the runtime service account access to each secret, and nothing else:

```bash
for s in session-key setlistfm-api-key spotify-client-secret; do
  gcloud secrets add-iam-policy-binding "$s" \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role=roles/secretmanager.secretAccessor
done
```

To replace a value later (steps 04 and 05), add a version, disable the old one (the free tier covers 6 active versions), then redeploy so new instances read it:

```bash
read -rs -p "New value: " v && printf '%s' "$v" | gcloud secrets versions add setlistfm-api-key --data-file=-; unset v; echo
gcloud secrets versions list setlistfm-api-key
gcloud secrets versions disable 1 --secret=setlistfm-api-key
```

## 9. Deploy settings

- [ ] Work out the service URL. Cloud Run URLs are `https://<service>-<project-number>.<region>.run.app`:

```bash
PROJECT_NUMBER="$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')"
echo "https://setlisted-${PROJECT_NUMBER}.${REGION}.run.app"
```

- [ ] Fill in `PROJECT_ID`, `REGION` and `BASE_URL` in [`scripts/deploy-config.sh`](../../../scripts/deploy-config.sh) and commit it. None of these values is secret.

## 10. First deploy

- [ ] From a clean checkout (no uncommitted changes), in Git Bash:

```bash
scripts/deploy.sh
```

The first Cloud Build run may ask to enable APIs or create a staging bucket: answer yes. If the build fails with a permission error on the build service account (for example it can't push to Artifact Registry or write logs), grant that account what it needs and run the script again:

```bash
BUILD_SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"
for role in roles/artifactregistry.writer roles/logging.logWriter roles/storage.objectViewer; do
  gcloud projects add-iam-policy-binding "$PROJECT_ID" --member="serviceAccount:${BUILD_SA}" --role="$role"
done
```

## 11. Check

- [ ] `curl https://<service-url>/api/health` returns `{"status":"ok"}`, and the URL in a browser shows the placeholder page over HTTPS. (The script checks `/api/health` itself.)
- [ ] The service has max instances 1, min 0, the runtime service account, and secrets referenced from Secret Manager:

```bash
gcloud run services describe setlisted --region="$REGION" \
  --format='yaml(spec.template.metadata.annotations,spec.template.spec.serviceAccountName,spec.template.spec.containers[0].env)'
```

  Expect `autoscaling.knative.dev/maxScale: '1'`, no `minScale` (or `'0'`), `setlisted-runtime@…`, and `SETLISTFM_API_KEY`, `SPOTIFY_CLIENT_SECRET` and `SESSION_KEY` as `valueFrom.secretKeyRef`, not `value`.
- [ ] The budget exists: Billing → Budgets & alerts.
- [ ] Commit something (or make an empty commit), run `scripts/deploy.sh` again, and check a new revision appears: `gcloud run revisions list --service=setlisted --region="$REGION"`.
- [ ] Logs show request lines (`method`, `path`, `status`, `duration_ms`) and no secret values: console → Cloud Run → setlisted → Logs, or:

```bash
gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="setlisted"' --limit=20
```
