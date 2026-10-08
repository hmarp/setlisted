# ADR-008: Continuous deployment on merge to main

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

- The owner wants this workflow: develop locally on a feature branch, open a PR into `main`, the owner reviews it manually, and **merging deploys automatically** to Cloud Run.
- ADR-003 hosts the app on Cloud Run and left the deployment mechanism open. ADR-006 runs CI on GitHub Actions for every PR. ADR-007 keeps secrets in Google Secret Manager.
- The repository is on GitHub and will be **public**, as a portfolio project, so no credential may live in the repo, and anything stored in GitHub must be treated with care.
- Deploying from GitHub needs GitHub to be granted access to the Google Cloud project, which is a security decision.

## Decision

**Trigger and pipeline**

- A GitHub Actions workflow runs on **push to `main`**, which is what a merged PR is.
- It runs the **full CI checks again** on the merged code (`gofmt`, `go vet`, `go test ./...`), then builds the container image, pushes it to Artifact Registry and deploys it to the Cloud Run service. If any stage fails, nothing is deployed.
- **Concurrency:** one deploy runs at a time. A later merge waits rather than racing an earlier one.

**Authentication to Google Cloud**

- **Workload Identity Federation**: Google Cloud trusts GitHub's short-lived identity tokens, restricted to the repository **`hmarp/setlisted`** and the **`main`** branch.
- **No long-lived credentials** (service-account key files) are created or stored in GitHub.
- A dedicated **deploy service account** has only the roles needed to push images to the project's Artifact Registry repository and deploy the Cloud Run service (including acting as the runtime service account).
- The **runtime service account** (used by the running app) is separate and is the only identity that can read the app's secrets. The deploy workflow never reads secret values.

**Gate**

- **Branch protection on `main`** (a ruleset): changes only through pull requests, no direct pushes or force-pushes, and CI must pass before merging. The owner's PR review is the human gate. With these rules, "merge" means "reviewed and tested".

**Traceability and rollback**

- Images are tagged with the **commit SHA**, so every Cloud Run revision traces back to a commit.
- **Rollback** means routing traffic back to the previous Cloud Run revision (one `gcloud` command, documented in the README). A fix then goes through a normal PR.

**Housekeeping**

- An Artifact Registry **clean-up policy** keeps the last ~10 images, to stay within the free storage allowance.
- The **manual deploy script** (plan step 02) stays as a documented fallback, e.g. if GitHub Actions is unavailable.

## Alternatives considered

- **Manual deploys only** (the draft plan's original approach). Simpler to set up, but every merge needs a separate manual step, and production can quietly drift from `main`. The owner wants merge to mean deploy.
- **Service-account key stored as a GitHub secret.** Quick to set up, but it's a long-lived credential that could leak, and Google discourages it. That risk matters more in a public repository.
- **Google Cloud Build triggers** (Google building from the GitHub repo). Also keyless, but CI would be split across two systems (tests in GitHub Actions, deploy in Cloud Build). Keeping both in GitHub Actions puts the whole pipeline in one place, visible in the repo.
- **Deploy on tag or release** instead of every merge. More control over releases, but an extra manual step the owner doesn't want for a small personal app.
- **Staging environment before production.** A safer rollout, but double the infrastructure and cost for six users. Fast rollback covers the risk.

## Consequences

- Every merged PR reaches production within minutes, with no manual step. `main` always matches what is deployed (or, after a rollback, is ahead of it).
- Mistakes reach users quickly too. Mitigations: the owner reviews each PR, CI is required before merge, and rollback is quick.
- One-off setup in Google Cloud (a Workload Identity pool and provider, a deploy service account and its roles) and GitHub (a ruleset). This setup is owner-run and documented.
- The repository being public is compatible with this design: the workflow file and the Workload Identity configuration reveal names but no credentials, and access is bound to this repo's `main` branch.
- GitHub Actions minutes are free for public repositories.
