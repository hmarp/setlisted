# Setlisted

Search for a band's live performance and turn its setlist into a Spotify playlist.

Log in with Spotify, type a band's name, pick one of its recent shows from setlist.fm and see which of its songs were found on Spotify (with covers, live versions and tapes labelled). Then create a private playlist of them in your library with one tap and open it in Spotify.

Built on the [setlist.fm API](https://api.setlist.fm/docs/1.0/index.html) and the [Spotify Web API](https://developer.spotify.com/documentation/web-api). For personal use by a small group of friends.

## Status

**MVP complete** (2026-11-10). It runs at <https://setlisted-613938414647.europe-west1.run.app>, for the owner and a few friends on the Spotify allowlist.

[Step 13's acceptance results](docs/mvp/plan/13-mvp-acceptance/results.md) against the spec's success criteria ([`spec.md`](docs/mvp/spec.md) §9) all pass:

- the journey takes under a minute;
- two friends used it unaided;
- matching quality: 95.2% found, with no wrong artists;
- it works on phone and laptop;
- access control;
- running cost is within budget;
- the owner has used it for a real gig.

Work since the MVP is tracked in [GitHub issues](https://github.com/hmarp/setlisted/issues): bugs, features and matching improvements. How the MVP was specified and built is archived in [`docs/mvp/`](docs/mvp/).

## Using it

1. Open the app and **Log in with Spotify**. Your Spotify account must be on the owner's allowlist; anyone else sees "Setlisted is invite-only".
2. Search for a band. If several artists share the name, pick yours.
3. Choose one of its 20 most recent shows that have a setlist.
4. Wait for "Matching songs…".
   - The preview lists every song in setlist order.
   - Each song's Spotify match is labelled: Found, Live version, Cover → original artist, Tape, Not found or Not found (medley).
5. **Create playlist** makes a new private playlist of the found songs in your Spotify library. **Open in Spotify** takes you to it.

The app stores nothing about you. The login lasts until you close the browser, or for one hour. **Log out** ends it straight away. Setlist data comes from [setlist.fm](https://www.setlist.fm).

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

The app starts with placeholder Spotify and setlist.fm values, but logging in needs the real Spotify client ID and secret (see [Spotify app](#spotify-app) below), and the search and show list endpoints need a real setlist.fm key ([step 05's setup](docs/setup.md#part-4-setlistfm-api-key)); with a placeholder they answer `503 upstream_unavailable`. If a variable is missing or invalid, the app refuses to start and names the variable (never its value).

### Spotify app

Login uses one Spotify developer app for local and production ([ADR-007](docs/adr/007-configuration-and-secrets.md)). The owner sets it up by following [part 3 of `docs/setup.md`](docs/setup.md#part-3-spotify-app-and-credentials):

- redirect URIs `http://127.0.0.1:8080/auth/callback` and `https://setlisted-613938414647.europe-west1.run.app/auth/callback`;
- the users allowed to log in, under **User Management**. In development mode that's at most 5, and the owner needs Spotify Premium.

Anyone else gets the invite-only message.

### Run

```sh
go run ./cmd/setlisted
```

Then open <http://127.0.0.1:8080/> or check <http://127.0.0.1:8080/api/health>, which returns `{"status":"ok"}`. Logs are JSON on stdout.

**Using it locally:** open the app at exactly `http://127.0.0.1:8080` (not `localhost`, or the Spotify redirect and the `Origin` check won't match), click **Log in with Spotify**, and log in with an allowlisted account. You land on the search screen: search for a band, choose a show, wait for "Matching songs…", then **Create playlist**. This **creates a real private playlist** in your Spotify library. It needs a real setlist.fm key and the real Spotify client ID and secret. **Log out** clears the session, and <http://127.0.0.1:8080/api/session> shows whether you're logged in. The JSON endpoints can also be opened directly in the logged-in browser ([`docs/api.md`](docs/api.md)), e.g. <http://127.0.0.1:8080/api/artists?q=gojira>.

Chrome, Edge and Firefox accept the session's `Secure` cookie over plain HTTP on `127.0.0.1`; Safari may not. The login lasts until the browser is closed or an hour has passed.

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

**Manual UI test:** the UI has no automated tests ([ADR-006](docs/adr/006-testing-strategy.md)). Run [`docs/manual-test-checklist.md`](docs/manual-test-checklist.md) on a phone and a laptop after any UI change.

### Matching-quality tests

`TestMatchingQuality` (`internal/quality`) runs the whole matching pipeline offline on real setlists recorded under [`testdata/quality/`](testdata/quality/) ([ADR-006](docs/adr/006-testing-strategy.md), spec §9 criterion 3). It's part of `go test ./...` and runs in CI. It fails if any found track isn't credited to the expected artist, if any entry's status, Live or Tape label or track differs from the owner-reviewed `expected.json`, or if found / matchable is below 90% overall. To see the per-setlist table:

```sh
go test ./internal/quality -run TestMatchingQuality -v
```

Each setlist folder holds `setlist.json` (from setlist.fm), `searches.json` (every Spotify search the pipeline makes for it, both passes for covers) and `expected.json` (written by hand). The recorded data is trimmed to the fields the app reads; the decision to commit it is in [step 09's handoff](docs/mvp/plan/09-matching-quality-tests/handoff.md).

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

- the Google Cloud project, billing, the £1 budget alert, Artifact Registry, the runtime service account and the secrets, by following [part 1 of `docs/setup.md`](docs/setup.md#part-1-google-cloud-project-and-first-deploy);
- Workload Identity Federation, the deploy service account, the image clean-up policy, the GitHub repository variables and the `main` ruleset, by following [part 2 of `docs/setup.md`](docs/setup.md#part-2-continuous-deployment).

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

**Changing a non-secret setting** (such as `SPOTIFY_CLIENT_ID`): merges don't change env vars. Update [`scripts/deploy-config.sh`](scripts/deploy-config.sh) and apply it once with `scripts/deploy.sh` (or `gcloud run services update setlisted --region=europe-west1 --update-env-vars NAME=value`). A new **secret version** needs no deploy-config change: the next revision reads `latest`. The Spotify client ID and secret commands are in [step 04's setup](docs/setup.md#6-cloud-run-settings).

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

## Architecture

One Go service serves a JSON API ([`docs/api.md`](docs/api.md)) and a static UI from the same origin, on Google Cloud Run. There's no database and nothing is cached.

```
browser ── static UI (web/static) ──► JSON API (/api/*) ──► setlist.fm API
                                         │   (Go, net/http)  └─► Spotify Web API
                                         └─ encrypted session cookie (Spotify token)
```

- **Backend:** Go, standard library first, exposing a JSON API ([ADR-001](docs/adr/001-go-backend-json-api.md))
- **UI (MVP):** throwaway static HTML and plain JavaScript in [`web/static/`](web/static/), served by the Go backend and using only the JSON API ([ADR-002](docs/adr/002-throwaway-static-ui.md))
- **Hosting:** Google Cloud Run, max 1 instance, scales to zero ([ADR-003](docs/adr/003-hosting-google-cloud-run.md))
- **Login:** Spotify only, with a stateless encrypted-cookie session ([ADR-004](docs/adr/004-spotify-login-encrypted-cookie-session.md)), lasting for the browser session or one hour ([ADR-009](docs/adr/009-session-cookie-lifetime.md)). Scopes are `playlist-modify-private` and `user-read-private` ([ADR-010](docs/adr/010-spotify-scope-user-read-private.md))
- **Matching and API calls** ([ADR-005](docs/adr/005-matching-and-api-call-strategy.md)):
  - setlist.fm: one rate-limited client, with calls spaced 600 ms apart;
  - Spotify: one field-filtered search per entry and pass (`limit=10`), up to 4 at once, with `Retry-After` honoured;
  - the matching rules are pure Go in `internal/setlist` and `internal/matching`.
- **Testing:** table-driven unit tests named after spec IDs, `httptest` for the HTTP layers, an offline matching-quality test on recorded setlists, and a manual UI checklist ([ADR-006](docs/adr/006-testing-strategy.md))
- **Configuration:** environment variables only, with secrets in Secret Manager ([ADR-007](docs/adr/007-configuration-and-secrets.md))
- **Delivery:** every merge to `main` deploys automatically via GitHub Actions ([ADR-008](docs/adr/008-continuous-deployment-on-merge.md))

## Workflow

Work is driven by [GitHub issues](https://github.com/hmarp/setlisted/issues) and built with AI coding sessions (Claude Code), following [`CLAUDE.md`](CLAUDE.md):

1. **Issue:** every bug, feature or idea starts as an issue, using the Bug or Feature templates. Labels: `bug`, `feature`, `matching`, `refactor`, `idea`.
2. **Spec (non-trivial changes only):** `/idea <issue#>` interviews the owner and writes `docs/features/<issue#>-<slug>/spec.md`, plus a `plan.md` for work spanning several PRs. Non-trivial means user-visible behaviour, a matching rule, an endpoint or a new dependency. Specs are frozen once the feature ships.
3. **Build:** `/implement <issue#>` in a fresh session: branch `<issue#>-<slug>`, plan, tests first, code, checks, a code review, and a PR with `Closes #<issue#>`.
4. **Merge:** CI must pass; the merge deploys automatically.

Decisions that outlast a feature (stack, hosting, security, testing) are ADRs in [`docs/adr/`](docs/adr/). Two hooks in [`.claude/settings.json`](.claude/settings.json) apply in every session: Go files are formatted with `gofmt` after each edit, and shell commands that name `.env` are blocked.

The MVP was built differently: intent → spec → ADRs → a 13-step plan, one step per PR, each with a brief and a handoff. That record is in [`docs/mvp/`](docs/mvp/), and the lessons from it are in [step 13's handoff](docs/mvp/plan/13-mvp-acceptance/handoff.md#lessons-learned-for-the-portfolio-write-up).

### Documentation

| File | For | Contents |
|---|---|---|
| `README.md` | Everyone | What it is, how to use, run, test and deploy it, and the workflow |
| [`CLAUDE.md`](CLAUDE.md) | AI sessions | The rules every session follows |
| [`docs/api.md`](docs/api.md) | API clients (the UI) | The JSON API contract |
| [`docs/adr/`](docs/adr/) | Anyone changing the design | Architecture decisions, with their reasons |
| [`docs/manual-test-checklist.md`](docs/manual-test-checklist.md) | The owner | The hand-run UI test, after any UI change |
| [`docs/setup.md`](docs/setup.md) | The owner | Runbooks for Google Cloud, CI/CD, the Spotify app and the setlist.fm key |
| [`docs/features/`](docs/features/) | AI sessions | One spec per non-trivial issue, frozen once shipped |
| [`docs/mvp/`](docs/mvp/) | History | The MVP's intent, spec, plan and step handoffs; not maintained |

## Licence

[MIT](LICENSE)
