# Step 01: Go skeleton (config, health, static serving, CI)

- **Depends on:** nothing. This is the first step.
- **Branch:** `step-01-go-skeleton`
- **Handoffs to read:** none.

## Goal

Create the Go service's foundations so that every later step only adds features:

- a module and project layout;
- validated configuration;
- an HTTP server that serves a health endpoint and the static UI folder;
- shared JSON and error helpers;
- a Dockerfile;
- CI on pull requests.

## Decisions this step relies on

- **ADR-001: Go backend with a JSON API.**
  - A single Go service using the current stable Go release.
  - The JSON API is shaped around the MVP journey.
  - **Standard library first**: `net/http` (including its method and path routing, e.g. `mux.HandleFunc("GET /api/health", …)`), `encoding/json`, `log/slog`, `testing`.
  - No web framework (Gin, Echo, etc.). If the router ever becomes limiting, `chi` is the preferred step up.
  - Any third-party dependency must be justified in the PR that adds it.
- **ADR-002: Throwaway static UI.**
  - Static HTML and plain JS in its own directory, served by the Go backend from the **same origin** as the API.
  - No framework, no build step, no npm.
- **ADR-003: Cloud Run.**
  - The app runs as a container listening on the port in the `PORT` environment variable.
  - It must be **stateless**.
  - Logs go to Cloud Logging and must never contain secrets, tokens or user data.
  - **Cloud Run reserves URL paths ending in `z`** (e.g. `/healthz` never reaches the app), so the health endpoint is `/api/health`.
- **ADR-006: Testing.**
  - Standard `testing` with table-driven tests, and `net/http/httptest` for HTTP.
  - No assertion or mocking libraries.
  - CI on GitHub Actions runs `gofmt`, `go vet` and `go test ./...` on every PR.
- **ADR-007: Configuration and secrets.**
  - Settings come from **environment variables only**.
  - At startup, every required variable is checked for presence and validity. If one is missing or invalid, the app **refuses to start** with a message naming the variable, **never printing its value**.
  - Locally, a git-ignored `.env` is loaded with **`github.com/joho/godotenv`**. The `.env` file is optional, and variables already in the environment take precedence.
  - `.env.example` lists every variable with a placeholder and a one-line description.

## Spec requirements touched

- **ERR-5:** "No error message shows technical details (status codes, stack traces, raw responses)."
- **GEN-5** (for logging): "The app stores **no user data**. Nothing about a user, their searches, matches or playlists is kept after their browser session ends or they log out."

## What to build

1. **Module and layout.** Initialise the Go module (`github.com/hmarp/setlisted`). Suggested layout, which you can refine and should record in the handoff:
   - `cmd/setlisted/`: `main`, which loads configuration, builds the server and listens.
   - `internal/config/`: loading and validation.
   - `internal/httpapi/`: server, routing, middleware, JSON and error helpers, handlers.
   - `web/`: the static UI. For now, a placeholder `index.html` that says "Setlisted" and has the footer "Setlist data from setlist.fm", linking to https://www.setlist.fm.
   - Later steps will add `internal/auth`, `internal/setlistfm`, `internal/spotify`, `internal/setlist` and `internal/matching`.
2. **Static files** are embedded in the binary with `embed.FS` and served at `/`. This keeps the container to a single file.
3. **Configuration.** Define and validate all the variables the MVP will need. Defining them now means step 02 can create every secret once.

   | Variable | Secret? | Validation |
   |---|---|---|
   | `PORT` | no | Optional, default `8080`. Must be numeric. |
   | `BASE_URL` | no | Required. An absolute `http(s)` URL with no trailing slash, e.g. `http://127.0.0.1:8080` locally. Used later for the Spotify redirect URL and the `Origin` check. |
   | `SPOTIFY_CLIENT_ID` | no | Required, non-empty. |
   | `SPOTIFY_CLIENT_SECRET` | yes | Required, non-empty. |
   | `SETLISTFM_API_KEY` | yes | Required, non-empty. |
   | `SESSION_KEY` | yes | Required. Base64 that decodes to **exactly 32 bytes**. |

   Validation errors name the variable and say what's wrong, never echoing the value. Load `.env` with `godotenv` only if the file exists.
4. **HTTP server.**
   - `GET /api/health` returns `200` with JSON `{"status":"ok"}`.
   - Sensible server timeouts.
   - Graceful shutdown on SIGTERM (Cloud Run sends it).
   - Request logging middleware using `slog` (JSON handler): method, path, status and duration only. No query strings, because they may contain search terms, which are user data.
   - Panic recovery middleware that returns the standard error response.
5. **JSON and error helpers.**
   - A helper to write JSON responses.
   - **A single API error format**, used by every later endpoint:
     ```json
     { "error": { "code": "upstream_unavailable", "message": "Couldn't reach setlist.fm. Try again.", "service": "setlist.fm" } }
     ```
     - `code` is a stable machine-readable string.
     - `message` is safe, user-facing text with no technical details (ERR-5).
     - `service` is included only when an external service is involved.
   - Define the initial codes: `invalid_request`, `not_found`, `internal_error`. Later steps add codes such as `not_logged_in`, `upstream_unavailable` and `forbidden_origin`.
   - Unknown `/api/*` paths return the JSON `not_found` error. Non-API paths are served from `web/`.
6. **`docs/api.md`.** Create the API reference with:
   - conventions: JSON over HTTPS, same origin, cookie session (added in step 04);
   - the error format and its code table;
   - the `GET /api/health` endpoint.

   Every later step that adds an endpoint documents it here.
7. **`.env.example`**, listing every variable above with a placeholder and a one-line description. Include a note on generating `SESSION_KEY`, e.g. `openssl rand -base64 32`.
8. **Dockerfile.**
   - Multi-stage build: a Go builder image, then a minimal final image (e.g. distroless static, non-root).
   - `CGO_ENABLED=0`.
   - The container listens on `$PORT`.
   - Add a `.dockerignore`.
9. **CI.** Add `.github/workflows/ci.yml`:
   - runs on `pull_request` to `main`;
   - uses `actions/setup-go` with `go-version-file: go.mod`;
   - fails if `gofmt -l .` lists any files, then runs `go vet ./...` and `go test ./...`.

   Make it **reusable** (`workflow_call`), because step 03's deploy workflow re-runs it.

## Tests

- Config: table-driven tests covering a missing required variable, invalid `BASE_URL`, `SESSION_KEY` of the wrong length or invalid base64, and a non-numeric `PORT`. Check that error messages name the variable and don't contain its value.
- HTTP: `/api/health` returns 200 and the JSON; an unknown `/api/x` returns the JSON `not_found` error; `/` serves the placeholder page; the recovery middleware turns a panic into `internal_error`.

## Out of scope

- Any Spotify, setlist.fm or session code (steps 04–05).
- Deployment (steps 02–03).
- UI beyond the placeholder page (step 12).

## Done when

- `go test ./...`, `go vet ./...` and `gofmt -l .` all pass locally and in CI on the PR.
- With a filled-in `.env`, `go run ./cmd/setlisted` starts and `http://127.0.0.1:8080/api/health` responds. With a variable missing, the app refuses to start with a clear message.
- `docker build` succeeds and the container serves `/api/health`.

## End of step (same PR)

- Write `handoff.md` in this folder. Include: the final layout, the error helper API, how routes are registered, config variable names, the Go version, and anything later steps should copy.
- Mark step 01 done in `docs/plan.md`.
- Update `README.md`: prerequisites, local setup (`.env`), how to run, how to test.
