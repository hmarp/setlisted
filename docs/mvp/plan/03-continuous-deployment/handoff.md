# Step 03 handoff: Continuous deployment on merge

## What was built

- **`.github/workflows/deploy.yml`**: on every push to `main`, three jobs in order (a failure stops the rest):
  1. `ci`: calls `.github/workflows/ci.yml` (`gofmt`, `go vet`, `go test`). Shown as **`ci / test`**.
  2. `deploy`: authenticates with Workload Identity Federation, builds the image with Docker on the runner, pushes `europe-west1-docker.pkg.dev/setlisted-2026/setlisted/setlisted:<full commit SHA>`, runs `gcloud run deploy setlisted --image …`, routes 100% of traffic to the new revision (`--to-latest`) and writes the commit, image and URL to the job summary.
  3. `smoke`: `GET <service URL>/api/health` must return `{"status":"ok"}`, otherwise the run fails. It doesn't roll back.
- **`docs/plan/03-continuous-deployment/setup.md`**: the owner checklist (APIs, WIF pool and provider, deploy service account, clean-up policy, repository variables, ruleset, checks, first deploy, rollback practice, troubleshooting).
- **`scripts/artifact-cleanup-policy.json`**: keep the 10 most recent images, delete the rest.
- README "Deploying" section: automatic deploys, watching a deploy, rolling back, the manual fallback.

## Names and settings

| What | Value |
|---|---|
| Workload identity pool | `github` (global) |
| OIDC provider | `setlisted`, issuer `https://token.actions.githubusercontent.com` |
| Provider resource name | `projects/613938414647/locations/global/workloadIdentityPools/github/providers/setlisted` |
| Attribute mapping | `google.subject=assertion.sub`, `attribute.repository=assertion.repository`, `attribute.ref=assertion.ref` |
| Attribute condition | `assertion.repository == 'hmarp/setlisted' && assertion.ref == 'refs/heads/main'` |
| Deploy service account | `setlisted-deployer@setlisted-2026.iam.gserviceaccount.com` |
| Its roles | `artifactregistry.writer` on the `setlisted` repository; `run.developer` on the `setlisted` service; `iam.serviceAccountUser` on `setlisted-runtime` only. No project-level roles, no Secret Manager access, no keys. |
| Who can impersonate it | `principalSet://iam.googleapis.com/projects/613938414647/locations/global/workloadIdentityPools/github/attribute.repository/hmarp/setlisted` (`roles/iam.workloadIdentityUser`) |
| Repository variables | `GCP_PROJECT_ID`, `GCP_REGION`, `GCP_WIF_PROVIDER`, `GCP_DEPLOY_SERVICE_ACCOUNT`. No repository secrets. |
| Ruleset | `main`, active, on the default branch, empty bypass list: restrict deletions, block force pushes, require a PR (0 approvals), require status check `test` from GitHub Actions (not strict about being up to date). |
| Clean-up policy | keep 10 most recent versions; delete everything else (runs roughly daily) |

The repository was already public at the start of this step.

## Decisions taken in this step

- **Docker on the runner, not Cloud Build.** The deploy account then needs only `artifactregistry.writer` to publish an image. Cloud Build would also need Cloud Build and storage roles, plus `actAs` on the build service account. The Dockerfile is unchanged.
- **The workflow routes traffic to the latest revision after every deploy** (`gcloud run services update-traffic --to-latest`). After a rollback, traffic is pinned to an older revision, and `gcloud run deploy` alone would leave a new revision at 0%. Merging to `main` is taken to mean "serve `main`", so a rollback lasts until the next merge (usually the fix).
- **Smoke check as a separate job**, so a failed health check shows separately from a failed deploy in the Actions UI. It gets the URL from the deploy job (`status.url`), so it needs no cloud access.
- **Settings as repository variables** (as the brief says), with the service and repository names (`setlisted`) fixed in the workflow. The first deploy step fails with a clear message if a variable is missing.
- **Least-privilege job permissions:** the workflow defaults to `contents: read`; only the `deploy` job gets `id-token: write`.
- **The deploy job is skipped on forks** (`if: github.repository == 'hmarp/setlisted'`).
- Action versions: `actions/checkout@v5` (as in `ci.yml`), `google-github-actions/auth@v3`, `google-github-actions/setup-gcloud@v3` (latest majors on 2026-10-27).

## How a deploy looks

Merge a PR → Actions tab → **Deploy** run for the merge commit: `ci / test` (about a minute), then `deploy` (build, push, deploy; a few minutes), then `smoke`. The `deploy` job's summary lists the commit SHA, the image and the URL. `gcloud run revisions list --service=setlisted --region=europe-west1` shows a new revision.

## Rollback

```bash
gcloud run revisions list --service=setlisted --region=europe-west1 --limit=5
gcloud run services update-traffic setlisted --region=europe-west1 --to-revisions=<previous-revision>=100
# and back:
gcloud run services update-traffic setlisted --region=europe-west1 --to-latest
```

## Notes for later steps

- **Merges deploy only a new image.** The service keeps its env vars, secret references, service account and scaling. Secrets referenced as `:latest` are read by each new revision, so a new secret version is picked up by the next merge. **Non-secret env vars are not**: changing `SPOTIFY_CLIENT_ID` (step 04) means updating `scripts/deploy-config.sh` *and* either running `scripts/deploy.sh` once or `gcloud run services update setlisted --region=europe-west1 --update-env-vars SPOTIFY_CLIENT_ID=…`. The step 04 brief says so.
- **`scripts/deploy.sh` is declarative.** If it's run as the fallback, it re-applies everything in `deploy-config.sh`, so keep that file in step with the live service.
- **`scripts/deploy.sh` doesn't route traffic `--to-latest`.** After a rollback, a manual deploy leaves the new revision without traffic; run `update-traffic --to-latest` by hand.
- **New required checks:** if a later step adds CI jobs that should block merging, add them to the ruleset as well.

## Verification

Done in this session (locally): the workflow parses as YAML, `gofmt`, `go vet` and `go test` pass. `actionlint` wasn't available (downloading it was blocked), so the workflow wasn't linted.

To do with the owner once `setup.md` sections 1–8 are done, then the step 03 PR is merged (setup.md sections 9–10). Fill in:

- [ ] Deploy run for the merge commit passed (`ci / test`, `deploy`, `smoke`): <run URL>
- [ ] Image tagged with the merge commit SHA in Artifact Registry; new revision `<name>` serves `/api/health`.
- [ ] Direct push to `main` rejected; a PR with failing CI couldn't be merged.
- [ ] No user-managed key on `setlisted-deployer`; `gh secret list` empty.
- [ ] Rollback practised: traffic to `<previous revision>`, then `--to-latest`.

## Gotchas

- **GitHub keeps only one waiting run per concurrency group.** With three quick merges, the middle one's deploy is cancelled while waiting and the newest is deployed. Nothing is lost, because each image contains everything before it.
- **The `test` check must have run recently** for GitHub to suggest it when creating the ruleset. It can be typed in anyway.
- **Clean-up and rollbacks:** only the 10 most recent images are kept. A revision whose image has been deleted can't start new instances, and with min instances 0, rolling back to it would fail. Roll back only to recent revisions.
- **The attribute condition uses the repository name.** If `hmarp/setlisted` were ever renamed, transferred or deleted and the name reused, update the condition (or switch it to `assertion.repository_id`).

## Known gaps

- No automatic rollback on a failed smoke check (out of scope; ADR-008).
- The smoke check proves the service is healthy, not that it is running the new commit (`/api/health` doesn't report a version). The `--to-latest` routing makes the new revision the one serving.
