# Setlisted

Search for a band's live performance and turn its setlist into a Spotify playlist.

Built on the [setlist.fm API](https://api.setlist.fm/docs/1.0/index.html) and the [Spotify Web API](https://developer.spotify.com/documentation/web-api). For personal use by a small group of friends.

## Status

In development. [Step 01](docs/plan/01-go-skeleton/handoff.md) (Go skeleton) is done: the server starts, validates its configuration, serves `GET /api/health` and a placeholder page. Nothing is deployed yet. Progress is tracked in [`docs/plan.md`](docs/plan.md), and the API is documented in [`docs/api.md`](docs/api.md).

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
| `BASE_URL` | yes | Public base URL, no trailing slash. Locally `http://127.0.0.1:8080`. |
| `SPOTIFY_CLIENT_ID` | yes | Spotify app client ID. |
| `SPOTIFY_CLIENT_SECRET` | yes | Spotify app client secret. |
| `SETLISTFM_API_KEY` | yes | setlist.fm API key. |
| `SESSION_KEY` | yes | Base64 of 32 random bytes (`openssl rand -base64 32`). |

The Spotify and setlist.fm features don't exist yet, so any non-empty placeholder works for their variables for now. If a variable is missing or invalid, the app refuses to start and names the variable (never its value).

### Run

```sh
go run ./cmd/setlisted
```

Then open <http://127.0.0.1:8080/> or check <http://127.0.0.1:8080/api/health>, which returns `{"status":"ok"}`. Logs are JSON on stdout.

### Test

```sh
gofmt -l .        # should print nothing
go vet ./...
go test ./...
```

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs the same three checks on every pull request to `main`.

### Docker

```sh
docker build -t setlisted .
docker run --rm -p 8080:8080 --env-file .env setlisted
```

Use `BASE_URL=http://127.0.0.1:8080` in `.env` so the address matches the published port.

## Stack

- **Backend:** Go, standard library first, exposing a JSON API ([ADR-001](docs/adr/001-go-backend-json-api.md))
- **UI (MVP):** throwaway static HTML and plain JavaScript, served by the Go backend ([ADR-002](docs/adr/002-throwaway-static-ui.md))
- **Hosting:** Google Cloud Run ([ADR-003](docs/adr/003-hosting-google-cloud-run.md))
- **Login:** Spotify only, with a stateless encrypted-cookie session ([ADR-004](docs/adr/004-spotify-login-encrypted-cookie-session.md))
- **Delivery:** every merge to `main` deploys automatically via GitHub Actions ([ADR-008](docs/adr/008-continuous-deployment-on-merge.md))

## Workflow

1. **Intent**: idea-grilling session produces [`docs/intent.md`](docs/intent.md)
2. **Spec**: [`docs/spec.md`](docs/spec.md) is derived from the intent
3. **Architecture**: stack and design decisions are recorded as ADRs in [`docs/adr/`](docs/adr/)
4. **Plan**: [`docs/plan.md`](docs/plan.md) lists the implementation steps. Each step has a self-contained folder under `docs/plan/` with a brief for the session that implements it.
5. **Implement**: one step per branch/PR, reviewed manually before merging. Each step ends with a handoff note and an updated README.

## Licence

[MIT](LICENSE)
