# Setlisted

Search for a band's live performance and turn its setlist into a Spotify playlist.

Built on the [setlist.fm API](https://api.setlist.fm/docs/1.0/index.html) and the [Spotify Web API](https://developer.spotify.com/documentation/web-api). For personal use by a small group of friends.

## Status

In development. [Step 01](docs/plan/01-go-skeleton/handoff.md) (Go skeleton) is done: the server starts, validates its configuration, serves `GET /api/health` and a placeholder page. [Step 02](docs/plan/02-first-deploy/handoff.md) (first deploy) is done: the skeleton runs on Cloud Run at <https://setlisted-613938414647.europe-west1.run.app>, deployed manually with `scripts/deploy.sh`. [Step 03](docs/plan/03-continuous-deployment/handoff.md) (continuous deployment) is done: every merge to `main` deploys automatically through GitHub Actions. [Step 04](docs/plan/04-spotify-login/handoff.md) (Spotify login) is done: users log in with Spotify into a one-hour, browser-session cookie, and every `/api/` endpoint except health and session status needs a login. [Step 05](docs/plan/05-setlistfm-search-shows/handoff.md) (setlist.fm search and shows) is done: the API can search setlist.fm for a band (`GET /api/artists`) and list its 20 most recent shows with setlists (`GET /api/artists/{id}/shows`). [Step 06](docs/plan/06-entry-classification/handoff.md) (entry classification) is done: internal code (no endpoint yet) turns a setlist into numbered entries classified as songs, medleys or skipped, with the artists to match each against. [Step 07](docs/plan/07-matching-rules/handoff.md) (matching rules) is done: internal code (no endpoint yet) chooses the right Spotify track for an entry from search results, or decides there isn't one, following the spec's matching rules. [Step 08](docs/plan/08-spotify-search-pipeline/handoff.md) (Spotify search and matching pipeline) is done: internal code (no endpoint yet) searches Spotify for every song of a setlist, four searches at a time, and returns each entry's match or "not found", in setlist order. [Step 09](docs/plan/09-matching-quality-tests/handoff.md) (matching-quality tests) is done: six real setlists are recorded as test fixtures, and a test checks the matching against owner-approved expected outcomes in CI (95.2% found, no wrong artists). [Step 10](docs/plan/10-preview-api/handoff.md) (preview API) is done: `GET /api/shows/{id}/preview` fetches a show, matches every song on Spotify with the user's login and returns the preview, with the track IDs that playlist creation (step 11) will take. The placeholder page has bare login and logout controls for testing; there's no search screen or playlist feature yet. Progress is tracked in [`docs/plan.md`](docs/plan.md), and the API is documented in [`docs/api.md`](docs/api.md).

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
| `SETLISTFM_API_KEY` | yes | setlist.fm API key ([settings](https://www.setlist.fm/settings/api)). Free for non-commercial use. |
| `SESSION_KEY` | yes | Base64 of 32 random bytes (`openssl rand -base64 32`). |

The app starts with placeholder Spotify and setlist.fm values, but logging in needs the real Spotify client ID and secret (see [Spotify app](#spotify-app) below), and the search and show list endpoints need a real setlist.fm key ([step 05's setup](docs/plan/05-setlistfm-search-shows/setup.md)); with a placeholder they answer `503 upstream_unavailable`. If a variable is missing or invalid, the app refuses to start and names the variable (never its value).

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

**Logging in locally:** open the app at exactly `http://127.0.0.1:8080` (not `localhost`, or the Spotify redirect and the `Origin` check won't match), click **Log in with Spotify**, and log in with an allowlisted account. The page then says "Logged in.", and <http://127.0.0.1:8080/api/session> returns `{"loggedIn":true}`. **Log out** clears the session. While logged in, the same browser can try the setlist.fm endpoints, e.g. <http://127.0.0.1:8080/api/artists?q=gojira> then `http://127.0.0.1:8080/api/artists/<id>/shows` with the returned `id`, and then `http://127.0.0.1:8080/api/shows/<show id>/preview` with a show's `id` ([`docs/api.md`](docs/api.md)). The preview searches Spotify with the logged-in user's token, so it needs a real login as well as a real setlist.fm key. Chrome, Edge and Firefox accept the session's `Secure` cookie over plain HTTP on `127.0.0.1`; Safari may not. The login lasts until the browser is closed or an hour has passed.

### Test

```sh
gofmt -l .        # should print nothing
go vet ./...
go test ./...
```

**Manual check against live Spotify** (never run in CI): a test behind the `spotifylive` build tag searches Spotify with a real token and logs the candidates and matches. In Git Bash, with the real Spotify client ID and secret in `.env`, get an app token and run it:

```bash
set -a; source .env; set +a
export SPOTIFY_TEST_TOKEN=$(curl -s -X POST https://accounts.spotify.com/api/token -u "$SPOTIFY_CLIENT_ID:$SPOTIFY_CLIENT_SECRET" -d grant_type=client_credentials | sed -E 's/.*"access_token":"([^"]*)".*/\1/')
go test -tags spotifylive -run Live -v -count=1 ./internal/spotify/
unset SPOTIFY_TEST_TOKEN
```

The market defaults to `GB`; set `SPOTIFY_TEST_MARKET` to change it. The token is never printed.

### Matching-quality tests

`TestMatchingQuality` (`internal/quality`) runs the whole matching pipeline offline on real setlists recorded under [`testdata/quality/`](testdata/quality/) ([ADR-006](docs/adr/006-testing-strategy.md), spec §9 criterion 3). It's part of `go test ./...` and runs in CI. It fails if any found track isn't credited to the expected artist, if any entry's status, Live or Tape label or track differs from the owner-reviewed `expected.json`, or if found / matchable is below 90% overall. To see the per-setlist table:

```sh
go test ./internal/quality -run TestMatchingQuality -v
```

Each setlist folder holds `setlist.json` (from setlist.fm), `searches.json` (every Spotify search the pipeline makes for it, both passes for covers) and `expected.json` (written by hand). The recorded data is trimmed to the fields the app reads; the decision to commit it is in [step 09's handoff](docs/plan/09-matching-quality-tests/handoff.md).

**Recording or refreshing fixtures** (never in CI; needs the real setlist.fm key and Spotify client ID and secret in `.env`; works in Git Bash):

```bash
go run -tags live ./cmd/recordfixtures <setlist.fm setlist URL or ID>...
```

It uses a Spotify app token (no login) and market `GB` (`-market` to change it), writes `setlist.json` and `searches.json` to `testdata/quality/<setlist-id>/`, and never touches `expected.json` or writes keys or tokens. A new setlist needs an `expected.json`; after a refresh, review the test's differences and update the expectations (the owner approves them). `go test ./internal/quality -run TestMatchingQuality -quality.draft=<dir>` writes the pipeline's actual results as draft expectations to start from.

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
- **setlist.fm:** one rate-limited client, calls spaced 600 ms apart, no caching ([ADR-005](docs/adr/005-matching-and-api-call-strategy.md))
- **Spotify search:** one field-filtered search per entry and pass (`limit=10`), up to 4 at once, `Retry-After` honoured, no caching ([ADR-005](docs/adr/005-matching-and-api-call-strategy.md))
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
