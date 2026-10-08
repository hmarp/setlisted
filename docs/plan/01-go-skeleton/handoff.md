# Step 01 handoff: Go skeleton

## What was built

- Go module `github.com/hmarp/setlisted` with config loading, an HTTP server, JSON/error helpers, an embedded placeholder UI, a Dockerfile and CI.
- `GET /api/health` → `200 {"status":"ok"}`.
- `docs/api.md`, `.env.example`, and README sections for running and testing.

## Go version

- `go.mod` says **`go 1.27`** (Go 1.27 was the current stable release; 1.27.2 was the latest patch on 2026-10-08).
- The minor version is used on purpose, not an exact patch: CI's `actions/setup-go` (`go-version-file: go.mod`) then installs the latest 1.27.x, and the Dockerfile's `golang:1.27` image tracks the same line. An exact patch in `go.mod` broke the Docker build whenever an image lagged behind it.
- When moving to a new Go minor version, change `go.mod` and the `FROM golang:` line together.

## Layout

```
cmd/setlisted/main.go      main: loads .env (optional) and config, builds the server, listens, shuts down on SIGTERM
internal/config/           Load(getenv) → Config; validation of every variable
internal/jsonapi/          WriteJSON, WriteError, *Error and the error codes
internal/httpapi/          NewServer/NewHandler: routing, middleware (logging, panic recovery), handlers
web/web.go                 package web: embeds web/static/ and exposes it as web.Static() (an fs.FS)
web/static/index.html      placeholder page, served at /
Dockerfile, .dockerignore  multi-stage build → distroless static, non-root
.github/workflows/ci.yml   gofmt, go vet, go test; on pull_request to main and as workflow_call
```

Later steps add `internal/auth`, `internal/setlistfm`, `internal/spotify`, `internal/setlist` and `internal/matching` as planned.

### Refinement of the suggested layout

- **JSON and error helpers live in `internal/jsonapi`, not `internal/httpapi`.** `httpapi` wires up routes, so it will import `auth`, `setlistfm` and so on. If the helpers lived in `httpapi`, those packages (for example step 04's session middleware and Spotify 401 helper) couldn't use them without an import cycle. `jsonapi` imports nothing internal, so any package can use it.
- **Static files are in `web/static/`, not directly in `web/`.** `go:embed` needs a Go file in or above the embedded directory, and embedding a subdirectory keeps `web.go` out of the served files. Everything under `web/static/` is served at `/` (`web/static/app.js` → `/app.js`). New files are picked up automatically, with no code change.

## Error helper API (`internal/jsonapi`)

```go
type Code string
const CodeInvalidRequest, CodeNotFound, CodeInternalError Code = ...

type Error struct {
    Status  int    // HTTP status, not serialised
    Code    Code
    Message string // user-facing, no technical details (ERR-5)
    Service string // "setlist.fm" or "Spotify", omitted when empty
}
func (e *Error) Error() string        // so *Error can be returned as an error

func InvalidRequest(message string) *Error // 400
func NotFound() *Error                     // 404, "Not found."
func InternalError() *Error                // 500, "Something went wrong. Try again."

func WriteJSON(w http.ResponseWriter, status int, v any)
func WriteError(w http.ResponseWriter, e *Error)
```

To add a code: add a `Code` constant (and a constructor if it's used in several places), and add a row to the code table in `docs/api.md`. Errors involving a service set `Service`, e.g. `&jsonapi.Error{Status: 502, Code: "upstream_unavailable", Message: "Couldn't reach setlist.fm. Try again.", Service: "setlist.fm"}`.

`WriteJSON` sets `Content-Type: application/json; charset=utf-8`, `X-Content-Type-Options: nosniff` and `Cache-Control: no-store`.

## How routes are registered

All in `httpapi.NewHandler` (`internal/httpapi/server.go`), on a standard `http.ServeMux`, with Go's method-and-path patterns:

```go
mux.HandleFunc("GET /api/health", handleHealth)
mux.HandleFunc("/api/", ...)          // catch-all → JSON not_found
mux.Handle("/", http.FileServerFS(static))
```

- Always register API routes **with a method** (`"GET /api/…"`, `"POST /api/…"`). A request to a known path with another method falls through to the `/api/` catch-all and gets `404 not_found` rather than Go's plain-text 405.
- Non-API routes such as step 04's `/auth/login` and `/auth/logout` must be registered explicitly with a method, e.g. `"GET /auth/login"`. They don't conflict with the `/` catch-all. Unknown non-API paths get the file server's plain-text 404.
- The middleware chain is `logRequests(recoverPanics(mux))`. Middleware that applies only to some routes (e.g. step 04's session check for `/api/*`) can wrap individual handlers or a sub-mux. Step 04 decides.
- `NewHandler(logger, static fs.FS)` takes its dependencies as arguments. Handler tests use `httptest` with `slog.DiscardHandler` and `web.Static()` or `fstest.MapFS{}`. As later steps add dependencies (config, clients), extend the arguments, or turn them into a small struct if the list grows.

## Configuration

`config.Load(os.Getenv)` returns a `Config` or an error that lists **every** bad variable, each as `NAME: problem`, never the value. `main` exits with status 1 and logs it.

| Variable | Field | Rule |
|---|---|---|
| `PORT` | `Port string` | Optional, default `8080`, integer 1–65535 |
| `BASE_URL` | `BaseURL string` | Required. `http`/`https`, a host, and no path, query, fragment, credentials or trailing slash |
| `SPOTIFY_CLIENT_ID` | `SpotifyClientID` | Required |
| `SPOTIFY_CLIENT_SECRET` | `SpotifyClientSecret` | Required |
| `SETLISTFM_API_KEY` | `SetlistFMAPIKey` | Required |
| `SESSION_KEY` | `SessionKey []byte` | Required. Standard (padded) base64 of exactly 32 bytes, as `openssl rand -base64 32` produces. Stored decoded. |

- `BASE_URL` doesn't allow a path, because the app assumes it's served from the root (static files at `/`, redirect URL at `BASE_URL + "/auth/..."`).
- `Config` holds secrets. Never log it or pass it to `slog`.
- `.env` is loaded with `godotenv.Load()` from the working directory, only if it exists. Existing environment variables win.

## Logging

- `slog` JSON handler on stdout, with `level`/`msg` renamed to **`severity`/`message`** (with `WARN` → `WARNING`) so Cloud Logging picks up severity.
- One line per request: `method`, `path`, `status`, `duration_ms`. No query string, headers or bodies (GEN-5).
- Panics are logged with the panic value and the stack, and the client gets `500 internal_error`. If the handler already wrote a response, nothing more is written.
- `main` logs the port and `BASE_URL` at startup. Neither is secret.
- Later steps: never log tokens, secrets, cookie values, search terms or other user data. Log error categories, not upstream bodies.

## Server

- Timeouts: read header 5s, read 15s, **write 60s** (generous for the preview's many upstream calls; revisit in step 10 if it's not enough), idle 120s.
- SIGTERM/SIGINT → `Shutdown` with an 8s deadline (Cloud Run allows 10s).

## Docker

- `golang:1.27` builder, `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`, then `gcr.io/distroless/static-debian12:nonroot`. The image is about 16 MB.
- The binary listens on `$PORT`. `EXPOSE 8080` is documentation only.
- `.dockerignore` excludes `.git`, `.env*`, `docs` and Markdown files.
- Verified locally: build, then `/api/health` and `/` served with `PORT=9000`, and refusal to start with no configuration. (In the build sandbox Docker Hub was rate-limited and the Go module proxy was blocked inside the builder, so verification used a Docker Hub mirror and vendored modules. Neither is in the repo. A normal environment builds the Dockerfile as committed.)

## CI

- `.github/workflows/ci.yml`, one job (`test`): `actions/checkout@v5`, `actions/setup-go@v6` with `go-version-file: go.mod`, then `gofmt -l .` (fails if anything is listed), `go vet ./...`, `go test ./...`.
- Triggers: `pull_request` to `main`, and `workflow_call`. Step 03's deploy workflow calls it with `uses: ./.github/workflows/ci.yml`. The job is named `test`: the PR page shows it as "CI / test", and the check run name to require in step 03's ruleset is **`test`**.
- `permissions: contents: read`.

## Dependencies added

- `github.com/joho/godotenv` v1.5.1, for loading `.env` locally (chosen in ADR-007).

## Known gaps

- `http.FileServerFS` answers any method on non-API paths (e.g. `POST /` returns the page). It's harmless for static files. Step 04's `Origin` check covers state-changing routes.
- No security headers beyond `nosniff` on JSON (no CSP etc.). Not required by the spec. Could be added with the real UI.
- No `/api/` route has a session check yet (step 04).
