# Step 03: Continuous deployment on merge

- **Depends on:** step 02
- **Branch:** `step-03-continuous-deployment`
- **Handoffs to read:** `docs/plan/01-go-skeleton/handoff.md`, `docs/plan/02-first-deploy/handoff.md`

## Goal

Every merge to `main` deploys to Cloud Run automatically, after CI has passed on the merged code, with no stored cloud credentials. From this step on, all application code reaches production through reviewed PRs.

## Decisions this step relies on

**ADR-008: Continuous deployment on merge to main.**

- **Trigger and pipeline:**
  - A GitHub Actions workflow runs on **push to `main`**.
  - It **re-runs the full CI checks** (`gofmt`, `go vet`, `go test ./...`), then builds the container image, pushes it to Artifact Registry and deploys it to Cloud Run.
  - If any stage fails, nothing is deployed.
  - **Concurrency:** one deploy at a time. A later merge waits rather than racing.
- **Authentication:**
  - **Workload Identity Federation**, restricted to the repository **`hmarp/setlisted`** and the **`main`** branch.
  - **No service-account key files** are created or stored in GitHub.
  - A dedicated **deploy service account** has only the roles needed to push images to the project's Artifact Registry repository and deploy the Cloud Run service, including acting as the runtime service account.
  - The **runtime service account** is separate and is the only identity that can read the app's secrets. The deploy workflow never reads secret values.
- **Gate:** branch protection on `main` (a **ruleset**): PRs only, no direct or force pushes, and CI required before merging. The owner's PR review is the human gate.
- **Traceability and rollback:**
  - Images are tagged with the **commit SHA**.
  - Rollback means routing traffic back to the previous Cloud Run revision with one `gcloud` command, documented in the README.
- **Housekeeping:**
  - An Artifact Registry **clean-up policy** keeps the last ~10 images.
  - The **manual deploy script** (`scripts/deploy.sh`, from step 02) stays as a documented fallback.
- **The repository is public.** Workflow files and the Workload Identity configuration reveal names but no credentials.

## Owner tasks

Prepare exact commands in `docs/plan/03-continuous-deployment/setup.md`. The owner runs or clicks them.

1. **Make the repository public**, if it isn't already (GitHub → Settings → General → Danger Zone). First check there are no secrets anywhere in the git history.
2. **Workload Identity Federation:**
   - create a workload identity pool (e.g. `github`) and an OIDC provider for `https://token.actions.githubusercontent.com`;
   - map `attribute.repository` and `attribute.ref`;
   - add the **attribute condition** `assertion.repository == 'hmarp/setlisted' && assertion.ref == 'refs/heads/main'`.
3. **Deploy service account** (e.g. `setlisted-deployer`):
   - `roles/artifactregistry.writer` on the `setlisted` repository;
   - `roles/run.developer` on the `setlisted` service;
   - `roles/iam.serviceAccountUser` on the **runtime** service account only;
   - allow the pool's principal set for `hmarp/setlisted` to impersonate it (`roles/iam.workloadIdentityUser`).
4. **Artifact Registry clean-up policy:** keep the 10 most recent images, and delete older ones.
5. **GitHub repository variables** (not secrets, because none of these is a credential): project ID, region, Workload Identity provider resource name, deploy service account email.
6. **Ruleset on `main`:**
   - require a pull request before merging, with **0 required approvals**, because a solo owner can't approve their own PR (the review is still the gate);
   - block force pushes and deletion;
   - require the CI status check from the step 01 workflow to pass.

## What to build

1. **`.github/workflows/deploy.yml`:**
   - `on: push: branches: [main]`;
   - `concurrency: { group: deploy, cancel-in-progress: false }`;
   - permissions limited to `contents: read` and `id-token: write`.
   - Jobs:
     1. **ci**: call the reusable CI workflow from step 01;
     2. **deploy** (needs `ci`):
        - authenticate with `google-github-actions/auth` using Workload Identity Federation;
        - set up `gcloud`;
        - build the image (Docker on the runner, or Cloud Build; pick the simpler and note why);
        - tag it `<region>-docker.pkg.dev/<project>/setlisted/setlisted:${{ github.sha }}`, then push;
        - run `gcloud run deploy setlisted --image … --region …`.

        Deploying **only a new image** keeps the service's existing environment variables, secrets, service account and instance limits unchanged. Don't re-specify secrets in the workflow.
     3. **smoke check**: after deploying, call `<service-url>/api/health` and fail the job if it doesn't return 200. (This doesn't roll back automatically. It flags the problem.)
2. **README** "Deploying" section, updated with:
   - merging deploys;
   - how to watch a deploy in GitHub Actions;
   - **how to roll back**, e.g. `gcloud run services update-traffic setlisted --to-revisions=<previous-revision>=100 --region …`, and how to list revisions;
   - the manual script as the fallback.

## Out of scope

- Staging environments, release tags, automatic rollback.
- Any application features.

## Done when

- A trivial PR (e.g. the README change in this step), once merged, triggers the deploy workflow:
  - CI runs again;
  - an image tagged with the merge commit's SHA is pushed;
  - a new Cloud Run revision serves `/api/health`;
  - the smoke check passes.
- A direct push to `main` is rejected by the ruleset, and a PR with failing CI can't be merged.
- No service-account key exists, and no GitHub secret holds a cloud credential.
- **A rollback has been practised once**: traffic routed to the previous revision, then back to the latest. The commands are recorded in the README.

## End of step (same PR, with the workflow tested by merging it)

- Write `handoff.md`, including:
  - the pool, provider and service account names;
  - the repository variable names;
  - the ruleset details;
  - how a deploy looks;
  - the rollback commands as practised;
  - any gotchas.
- Mark step 03 done in `docs/plan.md`.
- Update `README.md` as described above.
