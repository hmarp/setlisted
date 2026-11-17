# Setup runbooks

One-off owner setup for the infrastructure and the external services: what to follow to rebuild the Google Cloud project, CI/CD, the Spotify app or the setlist.fm key, or to rotate a secret.

They were written while building the MVP (steps 02–05, archived under [`mvp/plan/`](mvp/plan/)), so a few steps mention a step's PR or a check "with Claude". Those were one-off checks; the commands themselves are current.

- [Part 1: Google Cloud project and first deploy](#part-1-google-cloud-project-and-first-deploy)
- [Part 2: continuous deployment](#part-2-continuous-deployment)
- [Part 3: Spotify app and credentials](#part-3-spotify-app-and-credentials)
- [Part 4: setlist.fm API key](#part-4-setlistfm-api-key)

## Part 1: Google Cloud project and first deploy

One-off setup for hosting Setlisted on Cloud Run. The owner runs these steps. They can be repeated to rebuild the setup from scratch (for example in a new project).

"Console" means the **Google Cloud console**, the web UI at <https://console.cloud.google.com>, signed in with the same Google account as `gcloud auth login`. Everything else is a command.

Run the commands in **Git Bash** (on Windows, not the WSL `bash` in `System32`), or any bash on Linux or macOS. Set these variables first and keep the same shell open:

```bash
PROJECT_ID=setlisted-XXXX     # globally unique; pick your own suffix
REGION=europe-west1           # see "Choose the region" below
```

### 1. Install and log in

- [ ] Install the [Google Cloud CLI](https://cloud.google.com/sdk/docs/install). On Windows, let the installer add it to `PATH`, then open a new Git Bash window.
- [ ] Log in and check:

```bash
gcloud auth login
gcloud --version
```

### 2. Project and billing (console: card details)

- [ ] Create the project:

```bash
gcloud projects create "$PROJECT_ID" --name=setlisted
gcloud config set project "$PROJECT_ID"
```

- [ ] In the console, go to **Billing** (<https://console.cloud.google.com/billing>). Create a billing account if you don't have one (this is where the card details go), then **link it to the project** (Billing → Account management → My projects, or "Link a billing account" on the project's Billing page). The command-line equivalent, once the billing account exists:

```bash
gcloud billing accounts list
gcloud billing projects link "$PROJECT_ID" --billing-account=XXXXXX-XXXXXX-XXXXXX
```

### 3. £1 budget alert (console)

Budgets warn and don't stop spending. The hard cost ceiling is Cloud Run's max instances = 1 (set by the deploy script).

- [ ] Console → **Billing → Budgets & alerts → Create budget** (<https://console.cloud.google.com/billing/budgets>; pick the billing account if asked):
  - Name: `setlisted`
  - Scope: this project (or all projects on the billing account)
  - Amount: **specified amount, £1**. The currency is the billing account's. If it isn't GBP, use the equivalent.
  - Thresholds: **50%, 90% and 100% of actual spend** (optionally 100% forecasted too).
  - Notifications: keep "Email alerts to billing admins and users" ticked.

### 4. Enable the APIs

- [ ] Enable the APIs:

```bash
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  secretmanager.googleapis.com
```

### 5. Choose the region

Checked on 2026-10-27 against Google's [Cloud Run locations](https://cloud.google.com/run/docs/locations), [Cloud Run pricing](https://cloud.google.com/run/pricing) and the [free tier list](https://cloud.google.com/free/docs/free-cloud-features):

- The Cloud Run free allowance (2 million requests, 180,000 vCPU-seconds and 360,000 GiB-seconds a month on request-based billing) is **per billing account and applies in every region**. It isn't US-only, unlike the Compute Engine e2-micro free VM.
- Regions are priced in two tiers. Usage in a **Tier 2** region costs more per unit, so it uses up the free allowance faster. **`europe-west2` (London) is Tier 2.** `europe-west1` (Belgium), `europe-west4` (Netherlands) and `us-central1` (Iowa) are Tier 1.
- The free outbound data allowance (1 GB a month) counts only traffic from North America. Our responses are small JSON and a few static files, so outbound data from Europe should cost pennies at most.
- The Artifact Registry (0.5 GB), Cloud Build (2,500 build-minutes) and Secret Manager (6 active secret versions, 10,000 accesses) free tiers don't depend on the region.

**Recommendation: `europe-west1` (Belgium).** It's Tier 1, so it gets the same free allowance as a US region, and it's close to UK users (roughly 10–20 ms further than London). At a few friends' usage, any region would stay inside the free tier. Belgium keeps the most headroom without putting the service across the Atlantic. Choose `europe-west2` if London latency matters more than headroom.

- [ ] Choose the region and set `REGION` above. Record it in `scripts/deploy-config.sh`.

### 6. Artifact Registry repository

- [ ] Create the repository:

```bash
gcloud artifacts repositories create setlisted \
  --repository-format=docker \
  --location="$REGION" \
  --description="Setlisted container images"
```

Step 03 adds a clean-up policy.

### 7. Runtime service account

The identity the Cloud Run service runs as. It gets **no project-level roles**, only access to the three secrets (step 8).

- [ ] Create the service account:

```bash
gcloud iam service-accounts create setlisted-runtime \
  --display-name="Setlisted Cloud Run runtime"
RUNTIME_SA="setlisted-runtime@${PROJECT_ID}.iam.gserviceaccount.com"
```

### 8. Secrets

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

### 9. Deploy settings

- [ ] Work out the service URL. Cloud Run URLs are `https://<service>-<project-number>.<region>.run.app`:

```bash
PROJECT_NUMBER="$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')"
echo "https://setlisted-${PROJECT_NUMBER}.${REGION}.run.app"
```

- [ ] Fill in `PROJECT_ID`, `REGION` and `BASE_URL` in [`scripts/deploy-config.sh`](../scripts/deploy-config.sh) and commit it. None of these values is secret.

### 10. First deploy

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

### 11. Check

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

## Part 2: continuous deployment

One-off setup that lets GitHub Actions deploy to Cloud Run on every merge to `main` ([ADR-008](adr/008-continuous-deployment-on-merge.md)). The owner runs these steps. They can be repeated to rebuild the setup from scratch.

**Do sections 1–6 before merging the step 03 PR.** Merging it adds `.github/workflows/deploy.yml`, and that merge is the first automatic deploy. If the setup isn't there, the deploy job fails (nothing breaks, the live service is untouched) and you can re-run it from the Actions tab once the setup is done.

"Console" means the **Google Cloud console** (<https://console.cloud.google.com>); "GitHub" means the repository's pages on github.com. Everything else is a command.

Run the commands in **Git Bash** (on Windows, not the WSL `bash` in `System32`), logged in with `gcloud auth login` as the owner. Set these variables first and keep the same shell open:

```bash
PROJECT_ID=setlisted-2026
PROJECT_NUMBER=613938414647
REGION=europe-west1
GITHUB_REPO=hmarp/setlisted
RUNTIME_SA="setlisted-runtime@${PROJECT_ID}.iam.gserviceaccount.com"
DEPLOY_SA="setlisted-deployer@${PROJECT_ID}.iam.gserviceaccount.com"
gcloud config set project "$PROJECT_ID"
```

### 1. Repository is public

The repository was already **public** when this checklist was written (2026-10-27). Nothing to do unless that has changed.

- [ ] Check: `gh repo view hmarp/setlisted --json visibility` prints `"PUBLIC"`, or see the repository's [settings page](https://github.com/hmarp/setlisted/settings) (General → Danger Zone).

If it ever needs making public again, first check no secret is anywhere in the history (a `.env` file, or something that looks like a key):

```bash
git log --all --name-only --format= | sort -u | grep -E '(^|/)\.env($|\.)' | grep -v '\.env\.example$'
git log --all -p | grep -nE 'BEGIN (RSA |EC )?PRIVATE KEY|"private_key"|AIza[0-9A-Za-z_-]{35}' | head
```

Both should print nothing.

### 2. Enable the APIs

Workload Identity Federation needs the IAM, Security Token Service and IAM Credentials APIs.

- [ ] Enable them:

```bash
gcloud services enable \
  iam.googleapis.com \
  sts.googleapis.com \
  iamcredentials.googleapis.com
```

### 3. Workload Identity Federation

A **pool** (`github`) holds external identities. Its **provider** (`setlisted`) trusts GitHub's OIDC tokens, but only from `hmarp/setlisted` on `main`: tokens from any other repository or branch (including pull requests) are refused by the attribute condition.

- [ ] Create the pool:

```bash
gcloud iam workload-identity-pools create github \
  --location=global \
  --display-name="GitHub Actions"
```

- [ ] Create the provider:

```bash
gcloud iam workload-identity-pools providers create-oidc setlisted \
  --location=global \
  --workload-identity-pool=github \
  --display-name="hmarp/setlisted on main" \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.ref=assertion.ref" \
  --attribute-condition="assertion.repository == '${GITHUB_REPO}' && assertion.ref == 'refs/heads/main'"
```

- [ ] Note its full resource name (it goes into a GitHub variable in section 6):

```bash
gcloud iam workload-identity-pools providers describe setlisted \
  --location=global \
  --workload-identity-pool=github \
  --format='value(name)'
```

Expect `projects/613938414647/locations/global/workloadIdentityPools/github/providers/setlisted`.

### 4. Deploy service account

The identity the workflow acts as. It can push images to the `setlisted` repository, deploy the `setlisted` service, and deploy it *as* the runtime account. It gets **no project-level roles and no Secret Manager access**: Cloud Run checks that the runtime account can read the secrets, not the deployer. **Never create a key for it.**

- [ ] Create it:

```bash
gcloud iam service-accounts create setlisted-deployer \
  --display-name="Setlisted GitHub Actions deployer"
```

- [ ] Push images to the `setlisted` Artifact Registry repository:

```bash
gcloud artifacts repositories add-iam-policy-binding setlisted \
  --location="$REGION" \
  --member="serviceAccount:${DEPLOY_SA}" \
  --role=roles/artifactregistry.writer
```

- [ ] Deploy the `setlisted` Cloud Run service (and change its traffic, for `--to-latest`):

```bash
gcloud run services add-iam-policy-binding setlisted \
  --region="$REGION" \
  --member="serviceAccount:${DEPLOY_SA}" \
  --role=roles/run.developer
```

- [ ] Act as the runtime service account, and only that one:

```bash
gcloud iam service-accounts add-iam-policy-binding "$RUNTIME_SA" \
  --member="serviceAccount:${DEPLOY_SA}" \
  --role=roles/iam.serviceAccountUser
```

- [ ] Let workflows from `hmarp/setlisted` (that pass the provider's condition, so only `main`) impersonate the deploy account:

```bash
gcloud iam service-accounts add-iam-policy-binding "$DEPLOY_SA" \
  --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github/attribute.repository/${GITHUB_REPO}"
```

### 5. Artifact Registry clean-up policy

Keeps the 10 most recent images in the `setlisted` repository and deletes older ones. The policy is in [`scripts/artifact-cleanup-policy.json`](../scripts/artifact-cleanup-policy.json): a "keep" rule always wins over a "delete" rule, so "keep the 10 newest" plus "delete everything" means "delete all but the 10 newest". Clean-up runs in the background, roughly daily, so the count can briefly go above 10.

- [ ] Apply it (from the repository root):

```bash
gcloud artifacts repositories set-cleanup-policies setlisted \
  --location="$REGION" \
  --policy=scripts/artifact-cleanup-policy.json \
  --no-dry-run
```

- [ ] Check: `gcloud artifacts repositories list-cleanup-policies setlisted --location="$REGION"`.

### 6. GitHub repository variables

The workflow reads its settings from **variables**, not secrets: none of them is a credential.

| Variable | Value |
|---|---|
| `GCP_PROJECT_ID` | `setlisted-2026` |
| `GCP_REGION` | `europe-west1` |
| `GCP_WIF_PROVIDER` | `projects/613938414647/locations/global/workloadIdentityPools/github/providers/setlisted` |
| `GCP_DEPLOY_SERVICE_ACCOUNT` | `setlisted-deployer@setlisted-2026.iam.gserviceaccount.com` |

- [ ] Set them in GitHub → [Settings → Secrets and variables → Actions → Variables](https://github.com/hmarp/setlisted/settings/variables/actions) → New repository variable, or with the GitHub CLI:

```bash
gh variable set GCP_PROJECT_ID --repo "$GITHUB_REPO" --body "$PROJECT_ID"
gh variable set GCP_REGION --repo "$GITHUB_REPO" --body "$REGION"
gh variable set GCP_WIF_PROVIDER --repo "$GITHUB_REPO" --body "projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github/providers/setlisted"
gh variable set GCP_DEPLOY_SERVICE_ACCOUNT --repo "$GITHUB_REPO" --body "$DEPLOY_SA"
gh variable list --repo "$GITHUB_REPO"
```

### 7. Ruleset on `main`

PRs only, no force pushes or deletion, and CI must pass before merging. No approvals are required, because you can't approve your own PR; your review is still the gate. The bypass list stays **empty**, so the rules apply to you as well.

- [ ] GitHub → [Settings → Rules → Rulesets](https://github.com/hmarp/setlisted/settings/rules) → New ruleset → New branch ruleset:
  - Ruleset name: `main`; Enforcement status: **Active**; Bypass list: empty.
  - Target branches: Add target → **Include default branch**.
  - Tick **Restrict deletions** and **Block force pushes**.
  - Tick **Require a pull request before merging**, with **Required approvals: 0**.
  - Tick **Require status checks to pass** → Add checks → type `test` and pick the one from **GitHub Actions**. (It's the job in `ci.yml`; GitHub only suggests checks that have run in the last week. If it isn't suggested, add `test` anyway.) Leave "Require branches to be up to date" unticked: the deploy re-runs CI on the merged code.
  - Create.

Or with the GitHub CLI (`15368` is the GitHub Actions app, so only Actions can satisfy the check):

```bash
gh api --method POST "repos/${GITHUB_REPO}/rulesets" --input - <<'EOF'
{
  "name": "main",
  "target": "branch",
  "enforcement": "active",
  "bypass_actors": [],
  "conditions": { "ref_name": { "include": ["~DEFAULT_BRANCH"], "exclude": [] } },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": false,
        "required_status_checks": [{ "context": "test", "integration_id": 15368 }]
      }
    }
  ]
}
EOF
```

### 8. Check the setup

- [ ] No key exists for the deploy account (expect an empty list), and no GitHub secret holds a cloud credential (expect none at all):

```bash
gcloud iam service-accounts keys list --iam-account="$DEPLOY_SA" --managed-by=user
gh secret list --repo "$GITHUB_REPO"
```

- [ ] The deploy account has no project-level roles (expect no output):

```bash
gcloud projects get-iam-policy "$PROJECT_ID" \
  --flatten='bindings[].members' \
  --filter="bindings.members:serviceAccount:${DEPLOY_SA}" \
  --format='value(bindings.role)'
```

- [ ] A direct push to `main` is rejected (expect `GH013: Repository rule violations`):

```bash
git switch main && git pull
git commit --allow-empty -m "Test: direct push should be rejected"
git push origin main
git reset --hard origin/main   # drop the test commit
```

### 9. First automatic deploy (merge the step 03 PR)

- [ ] Merge the step 03 PR. Then open the [Actions tab](https://github.com/hmarp/setlisted/actions/workflows/deploy.yml) and watch the **Deploy** run: `ci / test`, then `deploy`, then `smoke`. The `deploy` job's summary shows the commit, image and URL.
- [ ] The image tagged with the merge commit's SHA is in Artifact Registry, and a new revision is serving:

```bash
gcloud artifacts docker images list "${REGION}-docker.pkg.dev/${PROJECT_ID}/setlisted/setlisted" --include-tags --limit=5 --sort-by=~UPDATE_TIME
gcloud run revisions list --service=setlisted --region="$REGION" --limit=3
curl https://setlisted-613938414647.europe-west1.run.app/api/health
```

- [ ] A PR with failing CI can't be merged: open a throwaway PR that breaks `gofmt` (e.g. add a badly indented line to a `.go` file), check the merge button is blocked, then close the PR without merging and delete its branch.

### 10. Practise a rollback

- [ ] List revisions (newest first) and pick the one before the latest:

```bash
gcloud run revisions list --service=setlisted --region="$REGION" --limit=5
```

- [ ] Route all traffic to it, and check:

```bash
gcloud run services update-traffic setlisted --region="$REGION" --to-revisions=PREVIOUS_REVISION=100
gcloud run services describe setlisted --region="$REGION" --format='yaml(status.traffic)'
curl https://setlisted-613938414647.europe-west1.run.app/api/health
```

- [ ] Route it back to the latest revision:

```bash
gcloud run services update-traffic setlisted --region="$REGION" --to-latest
gcloud run services describe setlisted --region="$REGION" --format='yaml(status.traffic)'
```

Record the revision names you used in `handoff.md` (or tell the session to).

### Troubleshooting

- **`auth` step fails with `invalid_target` or `unauthorized_client`:** the `GCP_WIF_PROVIDER` value is wrong, or the token didn't pass the attribute condition (the run isn't on `main` of `hmarp/setlisted`).
- **`auth` succeeds but a later step says `Permission 'iam.serviceAccounts.getAccessToken' denied`:** the `workloadIdentityUser` binding in section 4 is missing or has a typo in the `principalSet` (it uses the project **number**). IAM changes can take a few minutes to apply.
- **`docker push` is denied:** the `artifactregistry.writer` binding on the repository is missing.
- **`gcloud run deploy` is denied on `iam.serviceaccounts.actAs`:** the `serviceAccountUser` binding on the runtime account is missing.
- **`gcloud run deploy` is denied on the service itself** even though `run.developer` is bound on it: as a last resort, grant `roles/run.developer` on the project instead (`gcloud projects add-iam-policy-binding "$PROJECT_ID" --member="serviceAccount:${DEPLOY_SA}" --role=roles/run.developer`) and record that in `handoff.md`.
- **A failed run** can be re-run from its page in the Actions tab (Re-run jobs → Re-run failed jobs) once the cause is fixed.

## Part 3: Spotify app and credentials

The owner runs these steps. They need the owner's Spotify account and Google Cloud credentials. Claude doesn't run them.

- "Spotify Dashboard" means the **Spotify for Developers dashboard**, <https://developer.spotify.com/dashboard>, logged in with the owner's Spotify account.
- "Console" means the **Google Cloud console**, <https://console.cloud.google.com>, signed in with the same Google account as `gcloud auth login`.
- Commands run in **Git Bash** (on Windows, not the WSL `bash`), from the repository root.

Spotify's development-mode rules (February 2026): the app owner needs **Spotify Premium**, at most **5 users** can be allowlisted, and `GET /me` no longer returns email.

**Order:** sections 1–4 any time before the local check; section 5 (local check, with Claude) before the PR is merged; section 6 (Cloud Run settings) before **or** after the merge; section 7 after the merge has deployed.

### 1. Create the Spotify app

- [ ] In the Spotify Dashboard, click **Create app** (skip if the app already exists, and go to its **Settings** to check the values below).
  - **App name:** `Setlisted`
  - **App description:** "Turns a setlist.fm setlist into a private Spotify playlist."
  - **Website:** `https://setlisted-613938414647.europe-west1.run.app` (optional)
  - **Redirect URIs:** see section 2.
  - **Which API/SDKs are you planning to use?** tick **Web API** only.
  - Accept the Developer Terms and click **Save**.

One app serves both local development and production (ADR-007).

### 2. Register both redirect URIs

- [ ] In the app's **Settings → Basic Information → Edit**, under **Redirect URIs**, add each of these exactly (no trailing slash), clicking **Add** after each, then **Save**:

```text
http://127.0.0.1:8080/auth/callback
https://setlisted-613938414647.europe-west1.run.app/auth/callback
```

- Spotify doesn't accept `localhost`, so local development uses `127.0.0.1`. Open the app at `http://127.0.0.1:8080`, **not** `http://localhost:8080`: with `localhost` the redirect URI and the `Origin` check won't match.
- If you run locally on another port, register that port's URI too, and set `PORT` and `BASE_URL` to match.

### 3. Allowlist users

- [ ] In the app's **User Management** tab, add each person who may use Setlisted: their **name** and the **email address of their Spotify account**. Add yourself (the owner) first, then up to 4 friends.
- Anyone not on this list can get through Spotify's login screen, but Setlisted then shows the invite-only message and creates no session (AUTH-4).
- **The app owner's own account is always allowed**, listed or not. To test invite-only, use a second Spotify account that isn't on the list.

### 4. Client ID and secret into the local `.env`

- [ ] In **Settings → Basic Information**, copy the **Client ID**, and click **View client secret** to see the secret.
- [ ] In your local `.env` (copy `.env.example` if you don't have one), set:

```dotenv
BASE_URL=http://127.0.0.1:8080
SPOTIFY_CLIENT_ID=<client ID>
SPOTIFY_CLIENT_SECRET=<client secret>
```

`.env` is git-ignored. Never paste the secret into a commit, an issue, a PR or a chat.

- [x] **Send Claude the Client ID** (it isn't secret) so it can be committed in `scripts/deploy-config.sh` in this PR. Done: `2d7607fbbdd9462892990e71494eefd9`. If the app is ever recreated, update that file and section 6.

### 5. Local check (with Claude, before merging)

On the step 04 branch:

```bash
go run ./cmd/setlisted
```

- [ ] Open <http://127.0.0.1:8080/>. It says "Not logged in."
- [ ] Click **Log in with Spotify**, log in and accept. You're back on `/` and it says "Logged in."
- [ ] In the browser's developer tools (F12 → Application/Storage → Cookies → `http://127.0.0.1:8080`), the `setlisted_session` cookie is there with **HttpOnly** and **Secure** ticked, **SameSite** `Lax`, and **Expires** "Session". Note which browser and version you used: this confirms (or not) that the browser accepts a `Secure` cookie over plain HTTP on `127.0.0.1`.
- [ ] <http://127.0.0.1:8080/api/session> shows `{"loggedIn":true}`.
- [ ] Click **Log out**. It says "Not logged in." and the cookie is gone.
- [ ] Log in again, and click **Cancel** on Spotify's consent screen if it's shown (it's shown the first time, or after you remove the app under <https://www.spotify.com/account/apps/>). You land on `/?login=cancelled`.
- [ ] If a Spotify account that is **not** allowlisted is available (for example a second free account, in a private window): logging in with it lands on `/?login=invite_only`, and `/api/session` says `{"loggedIn":false}`. If none is available, say so; the handoff then records that this was covered by tests only.

Tell Claude the results, including the browser and version.

### 6. Cloud Run settings

Merges deploy **only a new image**: the service keeps its env vars and secret references (step 03 handoff). So the real client ID and secret are set once, by hand.

Set these in the shell first:

```bash
PROJECT_ID=setlisted-2026
REGION=europe-west1
gcloud config set project "$PROJECT_ID"
```

- [ ] **Client secret:** add a new version of `spotify-client-secret` (hidden prompt, nothing echoed or saved in history):

```bash
read -rs -p "Spotify client secret: " v && printf '%s' "$v" | gcloud secrets versions add spotify-client-secret --data-file=-; unset v; echo
gcloud secrets versions list spotify-client-secret
```

  Paste at the hidden prompt with **Shift+Insert** or a **right-click** (in Git Bash, Ctrl+V doesn't paste; it can insert an invisible character), then press Enter.

- [ ] **Check the stored value without showing it.** It should be 32 characters, with the same fingerprint as the secret in your local `.env`:

```bash
gcloud secrets versions access latest --secret=spotify-client-secret | wc -c      # expect 32
gcloud secrets versions access latest --secret=spotify-client-secret | sha256sum
grep '^SPOTIFY_CLIENT_SECRET=' .env | cut -d= -f2- | tr -d '\r\n' | sha256sum   # must match the line above
```

  If they don't match, add another version (the `read` command again), check again, then disable the bad version: `gcloud secrets versions disable <number> --secret=spotify-client-secret`.

- [ ] **Client ID:** set it on the live service. This starts a new revision of the image that's already deployed, which also reads the new secret version (`spotify-client-secret:latest`):

```bash
gcloud run services update setlisted --region="$REGION" \
  --update-env-vars SPOTIFY_CLIENT_ID=2d7607fbbdd9462892990e71494eefd9
```

  (Alternatively, once `scripts/deploy-config.sh` has the real ID and is committed, run `scripts/deploy.sh` once from a clean checkout of `main`. It re-applies every setting.)

- [ ] Check the service has the new ID and is healthy:

```bash
gcloud run services describe setlisted --region="$REGION" --format='value(spec.template.spec.containers[0].env)'
curl -s https://setlisted-613938414647.europe-west1.run.app/api/health
```

  The env list shows the real `SPOTIFY_CLIENT_ID`; health returns `{"status":"ok"}`.

- [ ] Disable the old placeholder version (keeps active versions within the free tier). Check its number in the list above first; it's normally `1`:

```bash
gcloud secrets versions disable 1 --secret=spotify-client-secret
```

  Do this only after the new revision is serving: a revision that is starting reads `latest`, so disabling the old version doesn't affect it.

In the console the same settings are on the [Cloud Run service page](https://console.cloud.google.com/run/detail/europe-west1/setlisted/revisions?project=setlisted-2026) (**Edit & deploy new revision → Variables & secrets**) and the [secret's page](https://console.cloud.google.com/security/secret-manager/secret/spotify-client-secret/versions?project=setlisted-2026).

### 7. Cloud Run check (after the merge deploys)

- [ ] The **Deploy** run for the merge commit passed ([Actions → Deploy](https://github.com/hmarp/setlisted/actions/workflows/deploy.yml)).
- [ ] Repeat section 5 on <https://setlisted-613938414647.europe-west1.run.app/>: log in, "Logged in.", cookie attributes, `/api/session`, log out. (The cancel and invite-only checks are optional here.)

### Troubleshooting

| What you see | Likely cause |
|---|---|
| Spotify says **"INVALID_CLIENT: Invalid redirect URI"** | The URI isn't registered exactly (scheme, host, port, path, no trailing slash), or you opened the app on `localhost` instead of `127.0.0.1`. |
| Spotify says **"INVALID_CLIENT: Invalid client"** | `SPOTIFY_CLIENT_ID` is wrong or still a placeholder (locally in `.env`; on Cloud Run, section 6). |
| You come back to `/?login=failed` | Often a wrong client secret (the token exchange is refused). The app logs `login failed` with a `reason` (and Spotify's status code). Locally it's on the terminal; on Cloud Run, in [Logs Explorer](https://console.cloud.google.com/logs/query?project=setlisted-2026). The app's own logs never contain tokens or codes. |
| `/?login=failed` straight after clicking login | The login cookie didn't survive the round trip: more than 10 minutes on Spotify's page, cookies blocked, or the app was opened on a different host than `BASE_URL`. |
| `/?login=invite_only` for an allowlisted user | The email in User Management doesn't match their Spotify account, or the app owner's Premium lapsed. |
| Log out does nothing; `POST /auth/logout` returns `403` | The page's origin isn't `BASE_URL` (for example `localhost` instead of `127.0.0.1`). |

## Part 4: setlist.fm API key

Owner tasks for step 05. Commands are for **Git Bash on Windows**, run from the repository root. Tick each box as you go.

Nothing here changes code. Sections 1–3 can be done before the PR merges, and section 3 **must** be done before the merge, so the revision the merge deploys reads the real key.

### 1. The API key and its rate limit

- [x] Open your [setlist.fm API settings](https://www.setlist.fm/settings/api) (log in to setlist.fm first). If there's no key yet, apply for one there: it's free for non-commercial use.
- [x] Note the **rate limit** shown for the key and send Claude the numbers (they aren't secret) so they go into the handoff. Done (2026-10-31): **max. 2.0 requests per second and max. 1,440 per day**. The limiter spaces calls **600 ms** apart (about 1.7 per second).
  - If your key's limit is different, tell Claude: the interval is one constant (`setlistfm.DefaultInterval`).
- [x] Copy the **API key**. Never paste it into a commit, an issue, a PR or a chat.

### 2. Key into the local `.env`

- [x] In your local `.env`, set:

```dotenv
SETLISTFM_API_KEY=<API key>
```

### 3. Key into Secret Manager (before the merge)

The `setlistfm-api-key` secret has a placeholder version from step 02. Merges deploy a new revision, which reads `setlistfm-api-key:latest` when it starts, so adding a version is all that's needed: no service update.

```bash
PROJECT_ID=setlisted-2026
gcloud config set project "$PROJECT_ID"
```

- [x] Add a new version (hidden prompt, nothing echoed or saved in history). Done by the owner in the console (2026-10-31):

```bash
read -rs -p "setlist.fm API key: " v && printf '%s' "$v" | gcloud secrets versions add setlistfm-api-key --data-file=-; unset v; echo
gcloud secrets versions list setlistfm-api-key
```

  Paste at the hidden prompt with **Shift+Insert** or a **right-click** (in Git Bash, Ctrl+V doesn't paste; it can insert an invisible character), then press Enter.

- [ ] **Check the stored value without showing it.** The fingerprint must match the key in your local `.env`:

```bash
gcloud secrets versions access latest --secret=setlistfm-api-key | wc -c      # the key's length, with no newline counted
gcloud secrets versions access latest --secret=setlistfm-api-key | sha256sum
grep '^SETLISTFM_API_KEY=' .env | cut -d= -f2- | tr -d '\r\n' | sha256sum   # must match the line above
```

  If they don't match, add another version (the `read` command again), check again, then disable the bad version: `gcloud secrets versions disable <number> --secret=setlistfm-api-key`.

- [ ] After the merge's revision is serving (section 5), disable the placeholder version (usually `1`; check the list):

```bash
gcloud secrets versions disable 1 --secret=setlistfm-api-key
```

In the console: the [secret's versions page](https://console.cloud.google.com/security/secret-manager/secret/setlistfm-api-key/versions?project=setlisted-2026).

### 4. Local check (with Claude, before merging)

On the step 05 branch, with the real key in `.env`:

```bash
go run ./cmd/setlisted
```

- [x] Open <http://127.0.0.1:8080/> and log in with Spotify.
- [x] In the same browser, open each of these and check the JSON looks sensible (tell Claude what you see; you don't need to paste it all): Done: searching works (2026-10-31).
  - <http://127.0.0.1:8080/api/artists?q=gojira>: `"match":"clear"` with one artist. Copy its `id`.
  - <http://127.0.0.1:8080/api/artists?q=ghost>: `"match":"picker"`, up to 10 artists, several called Ghost with different disambiguations.
  - <http://127.0.0.1:8080/api/artists?q=sword>: `"picker"` (The Sword isn't a clear match for `sword`).
  - <http://127.0.0.1:8080/api/artists?q=qwxzvbnm>: `"match":"none"`.
  - `http://127.0.0.1:8080/api/artists/<id>/shows` with the Gojira `id`: up to 20 shows, newest first, dates like `14 Mar 2026`, song counts above 0, no future (empty) shows.
- [ ] Logged out (Log out on `/`), the same URLs answer `401 not_logged_in`.
- [ ] The terminal's log lines show no search text and no key.

### 5. Cloud Run check (after the merge deploys)

- [ ] The **Deploy** run for the merge commit passed ([Actions → Deploy](https://github.com/hmarp/setlisted/actions/workflows/deploy.yml)).
- [ ] Log in at <https://setlisted-613938414647.europe-west1.run.app/>, then open <https://setlisted-613938414647.europe-west1.run.app/api/artists?q=gojira> and `https://setlisted-613938414647.europe-west1.run.app/api/artists/<id>/shows`. Same results as locally.
- [ ] If either returns `503 upstream_unavailable`, the revision probably has the placeholder key. Check the [service's logs](https://console.cloud.google.com/run/detail/europe-west1/setlisted/logs?project=setlisted-2026) for `setlist.fm call failed`: `status 401` or `status 403` means a bad key. Fix the secret (section 3), then start a new revision so it reads `latest`:

```bash
gcloud run services update setlisted --region=europe-west1 --update-labels=refreshed=$(date +%s)
```
