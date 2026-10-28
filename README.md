# Setlisted

Search for a band's live performance and turn its setlist into a Spotify playlist.

Built on the [setlist.fm API](https://api.setlist.fm/docs/1.0/index.html) and the [Spotify Web API](https://developer.spotify.com/documentation/web-api). For personal use by a small group of friends.

## Status

In development. [Step 01](docs/plan/01-go-skeleton/handoff.md) (Go skeleton) is done: the server starts, validates its configuration, serves `GET /api/health` and a placeholder page. [Step 02](docs/plan/02-first-deploy/handoff.md) (first deploy) is done: the skeleton runs on Cloud Run at <https://setlisted-613938414647.europe-west1.run.app>, deployed manually with `scripts/deploy.sh`. [Step 03](docs/plan/03-continuous-deployment/handoff.md) (continuous deployment) is done: every merge to `main` deploys automatically through GitHub Actions. [Step 04](docs/plan/04-spotify-login/handoff.md) (Spotify login) is done: users log in with Spotify into a one-hour, browser-session cookie, and every `/api/` endpoint except health and session status needs a login. The placeholder page has bare login and logout controls for testing; there are no search or playlist features yet. Progress is tracked in [`docs/plan.md`](docs/plan.md), and the API is documented in [`docs/api.md`](docs/api.md).

## Running locally

### Prerequisites

- [Go](https://go.dev/dl/) 1.27 or later (see `go.mod`)
- Optional: Docker, to build the container image

### Configuration

Settings come from environment variables only ([ADR-007](docs/adr/007-configuration-and-secrets.md)). For local development, put them in a `.env` file, which is git-ignored and optional. Variables already set in the environment take precedence.

```sh
cp .env.example .env
openssl rand -base64 32   # paste the output into SESSION_KEY
```

| Variable | Required | Description |
|---|---|---|
| `PORT` | no | Port to listen on. Default `8080`. |
| `BASE_URL` | yes | Public base URL, `scheme://host[:port]` with no path. Locally `http://127.0.0.1:8080` (not `localhost`). Used for the Spotify redirect URL and the `Origin` check. |
| `SPOTIFY_CLIENT_ID` | yes | Spotify app client ID. |
| `SPOTIFY_CLIENT_SECRET` | yes | Spotify app client secret. |
| `SETLISTFM_API_KEY` | yes | setlist.fm API key. |
| `SESSION_KEY` | yes | Base64 of 32 random bytes (`openssl rand -base64 32`). |

The app starts with placeholder Spotify values, but logging in needs the real client ID and secret (see [Spotify app](#spotify-app) below). The setlist.fm feature doesn't exist yet, so any non-empty placeholder works for `SETLISTFM_API_KEY` for now. If a variable is missing or invalid, the app refuses to start and names the variable (never its value).

### Spotify app

Login uses one Spotify developer app for local and production ([ADR-007](docs/adr/007-configuration-and-secrets.md)). The owner sets it up by following [`docs/plan/04-spotify-login/setup.md`](docs/plan/04-spotify-login/setup.md):

- redirect URIs `http://127.0.0.1:8080/auth/callback` and `https://setlisted-613938414647.europe-west1.run.app/auth/callback`;
- the users allowed to log in, under **User Management**. In development mode that's at most 5, and the owner needs Spotify Premium.

Anyone else gets the invite-only message.

### Run

```sh
go run ./cmd/setlisted
```

Then open <http://127.0.0.1:8080/> or check <http://127.0.0.1:8080/api/health>, which returns `{"status":"ok"}`. Logs are JSON on stdout.

**Logging in locally:** open the app at exactly `http://127.0.0.1:8080` (not `localhost`, or the Spotify redirect and the `Origin` check won't match), click **Log in with Spotify**, and log in with an allowlisted account. The page then says "Logged in.", and <http://127.0.0.1:8080/api/session> returns `{"loggedIn":true}`. **Log out** clears the session. Chrome, Edge and Firefox accept the session's `Secure` cookie over plain HTTP on `127.0.0.1`; Safari may not. The login lasts until the browser is closed or an hour has passed.

### Test

```sh
gofmt -l .        # should print nothing
go vet ./...
go test ./...
```

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs the same three checks on every pull request to `main`, and again on the merged code before each deploy. A PR can't be merged until its `test` check passes.

### Docker

```sh
docker build -t setlisted .
docker run --rm -p 8080:8080 --env-file .env setlisted
```

Use `BASE_URL=http://127.0.0.1:8080` in `.env` so the address matches the published port.

## Deploying

The app runs on Google Cloud Run at its `*.run.app` HTTPS address ([ADR-003](docs/adr/003-hosting-google-cloud-run.md)). Secrets are stored in Secret Manager and reach the service as environment variables. Non-secret settings are set on the service by the deploy script ([ADR-007](docs/adr/007-configuration-and-secrets.md)).

### One-off setup

The owner sets up:

- the Google Cloud project, billing, the £1 budget alert, Artifact Registry, the runtime service account and the secrets, by following [`docs/plan/02-first-deploy/setup.md`](docs/plan/02-first-deploy/setup.md);
- Workload Identity Federation, the deploy service account, the image clean-up policy, the GitHub repository variables and the `main` ruleset, by following [`docs/plan/03-continuous-deployment/setup.md`](docs/plan/03-continuous-deployment/setup.md).

### Merging deploys

`main` only changes through pull requests: direct pushes and force pushes are blocked, and the `test` CI check must pass before merging ([ADR-008](docs/adr/008-continuous-deployment-on-merge.md)). Every merge runs [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml), one at a time:

1. **`ci / test`**: the CI checks again, on the merged code.
2. **`deploy`**: builds the image, pushes it to Artifact Registry tagged with the commit SHA, deploys it to the `setlisted` service and sends all traffic to the new revision. Only the image changes; the service keeps its env vars, secrets, service account and scaling.
3. **`smoke`**: checks `/api/health` on the live URL. A failure flags the problem; it doesn't roll back.

If a job fails, the later ones don't run. GitHub authenticates to Google Cloud with Workload Identity Federation: there are no key files or stored cloud credentials.

**Watching a deploy:** [Actions → Deploy](https://github.com/hmarp/setlisted/actions/workflows/deploy.yml), then the run for the merge commit. The `deploy` job's summary shows the commit, image and URL. To see which revision is serving:

```bash
gcloud run services describe setlisted --region=europe-west1 --format='yaml(status.traffic)'
```

A failed run can be re-run from its page (Re-run jobs).

**Changing a non-secret setting** (such as `SPOTIFY_CLIENT_ID`): merges don't change env vars. Update [`scripts/deploy-config.sh`](scripts/deploy-config.sh) and apply it once with `scripts/deploy.sh` (or `gcloud run services update setlisted --region=europe-west1 --update-env-vars NAME=value`). A new **secret version** needs no deploy-config change: the next revision reads `latest`. The Spotify client ID and secret commands are in [step 04's setup](docs/plan/04-spotify-login/setup.md#6-cloud-run-settings).

### Rolling back

Route traffic back to an earlier revision (newest first in the list):

```bash
gcloud run revisions list --service=setlisted --region=europe-west1 --limit=5
gcloud run services update-traffic setlisted --region=europe-west1 --to-revisions=<previous-revision>=100
```

Return to the latest revision when ready:

```bash
gcloud run services update-traffic setlisted --region=europe-west1 --to-latest
```

The next merge also sends traffic to its new revision, so a rollback lasts until then. Only the 10 most recent images are kept, so roll back only to recent revisions.

### Manual deploy (fallback)

If GitHub Actions is unavailable, the owner can deploy from their machine. Requirements: the [Google Cloud CLI](https://cloud.google.com/sdk/docs/install), logged in as the owner (`gcloud auth login`), plus `git` and `curl`. On Windows, use Git Bash.

```bash
scripts/deploy.sh
```

The script:

- refuses to run if there are uncommitted changes;
- builds the image with Cloud Build (no local Docker needed), tagged with the commit SHA;
- deploys the `setlisted` service with max instances 1, min instances 0, the `setlisted-runtime` service account, the non-secret settings and the Secret Manager references;
- checks `/api/health` on the live URL.

Project, region, `BASE_URL` and `SPOTIFY_CLIENT_ID` come from [`scripts/deploy-config.sh`](scripts/deploy-config.sh), which holds no secrets. Unlike the workflow, the script re-applies every setting, and it doesn't move traffic after a rollback: run `update-traffic --to-latest` afterwards if traffic was pinned.

## Stack

- **Backend:** Go, standard library first, exposing a JSON API ([ADR-001](docs/adr/001-go-backend-json-api.md))
- **UI (MVP):** throwaway static HTML and plain JavaScript, served by the Go backend ([ADR-002](docs/adr/002-throwaway-static-ui.md))
- **Hosting:** Google Cloud Run ([ADR-003](docs/adr/003-hosting-google-cloud-run.md))
- **Login:** Spotify only, with a stateless encrypted-cookie session ([ADR-004](docs/adr/004-spotify-login-encrypted-cookie-session.md)), lasting for the browser session or one hour ([ADR-009](docs/adr/009-session-cookie-lifetime.md))
- **Delivery:** every merge to `main` deploys automatically via GitHub Actions ([ADR-008](docs/adr/008-continuous-deployment-on-merge.md))

## Workflow

1. **Intent**: idea-grilling session produces [`docs/intent.md`](docs/intent.md)
2. **Spec**: [`docs/spec.md`](docs/spec.md) is derived from the intent
3. **Architecture**: stack and design decisions are recorded as ADRs in [`docs/adr/`](docs/adr/)
4. **Plan**: [`docs/plan.md`](docs/plan.md) lists the implementation steps. Each step has a self-contained folder under `docs/plan/` with a brief for the session that implements it.
5. **Implement**: one step per branch/PR, reviewed manually before merging. Each step ends with a handoff note and an updated README.

## Licence

[MIT](LICENSE)
