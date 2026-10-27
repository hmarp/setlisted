# Step 03 setup: continuous deployment (owner checklist)

One-off setup that lets GitHub Actions deploy to Cloud Run on every merge to `main` ([ADR-008](../../adr/008-continuous-deployment-on-merge.md)). The owner runs these steps. They can be repeated to rebuild the setup from scratch.

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

## 1. Repository is public

The repository was already **public** when this checklist was written (2026-10-27). Nothing to do unless that has changed.

- [ ] Check: `gh repo view hmarp/setlisted --json visibility` prints `"PUBLIC"`, or see the repository's [settings page](https://github.com/hmarp/setlisted/settings) (General → Danger Zone).

If it ever needs making public again, first check no secret is anywhere in the history (a `.env` file, or something that looks like a key):

```bash
git log --all --name-only --format= | sort -u | grep -E '(^|/)\.env($|\.)' | grep -v '\.env\.example$'
git log --all -p | grep -nE 'BEGIN (RSA |EC )?PRIVATE KEY|"private_key"|AIza[0-9A-Za-z_-]{35}' | head
```

Both should print nothing.

## 2. Enable the APIs

Workload Identity Federation needs the IAM, Security Token Service and IAM Credentials APIs.

- [ ] Enable them:

```bash
gcloud services enable \
  iam.googleapis.com \
  sts.googleapis.com \
  iamcredentials.googleapis.com
```

## 3. Workload Identity Federation

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

## 4. Deploy service account

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

## 5. Artifact Registry clean-up policy

Keeps the 10 most recent images in the `setlisted` repository and deletes older ones. The policy is in [`scripts/artifact-cleanup-policy.json`](../../../scripts/artifact-cleanup-policy.json): a "keep" rule always wins over a "delete" rule, so "keep the 10 newest" plus "delete everything" means "delete all but the 10 newest". Clean-up runs in the background, roughly daily, so the count can briefly go above 10.

- [ ] Apply it (from the repository root):

```bash
gcloud artifacts repositories set-cleanup-policies setlisted \
  --location="$REGION" \
  --policy=scripts/artifact-cleanup-policy.json \
  --no-dry-run
```

- [ ] Check: `gcloud artifacts repositories list-cleanup-policies setlisted --location="$REGION"`.

## 6. GitHub repository variables

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

## 7. Ruleset on `main`

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

## 8. Check the setup

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

## 9. First automatic deploy (merge the step 03 PR)

- [ ] Merge the step 03 PR. Then open the [Actions tab](https://github.com/hmarp/setlisted/actions/workflows/deploy.yml) and watch the **Deploy** run: `ci / test`, then `deploy`, then `smoke`. The `deploy` job's summary shows the commit, image and URL.
- [ ] The image tagged with the merge commit's SHA is in Artifact Registry, and a new revision is serving:

```bash
gcloud artifacts docker images list "${REGION}-docker.pkg.dev/${PROJECT_ID}/setlisted/setlisted" --include-tags --limit=5 --sort-by=~UPDATE_TIME
gcloud run revisions list --service=setlisted --region="$REGION" --limit=3
curl https://setlisted-613938414647.europe-west1.run.app/api/health
```

- [ ] A PR with failing CI can't be merged: open a throwaway PR that breaks `gofmt` (e.g. add a badly indented line to a `.go` file), check the merge button is blocked, then close the PR without merging and delete its branch.

## 10. Practise a rollback

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

## Troubleshooting

- **`auth` step fails with `invalid_target` or `unauthorized_client`:** the `GCP_WIF_PROVIDER` value is wrong, or the token didn't pass the attribute condition (the run isn't on `main` of `hmarp/setlisted`).
- **`auth` succeeds but a later step says `Permission 'iam.serviceAccounts.getAccessToken' denied`:** the `workloadIdentityUser` binding in section 4 is missing or has a typo in the `principalSet` (it uses the project **number**). IAM changes can take a few minutes to apply.
- **`docker push` is denied:** the `artifactregistry.writer` binding on the repository is missing.
- **`gcloud run deploy` is denied on `iam.serviceaccounts.actAs`:** the `serviceAccountUser` binding on the runtime account is missing.
- **`gcloud run deploy` is denied on the service itself** even though `run.developer` is bound on it: as a last resort, grant `roles/run.developer` on the project instead (`gcloud projects add-iam-policy-binding "$PROJECT_ID" --member="serviceAccount:${DEPLOY_SA}" --role=roles/run.developer`) and record that in `handoff.md`.
- **A failed run** can be re-run from its page in the Actions tab (Re-run jobs → Re-run failed jobs) once the cause is fixed.
