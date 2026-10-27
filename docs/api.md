# API reference

The JSON API served by the Go backend. The UI (`web/static/`) uses only what's documented here.

Every step that adds or changes an endpoint updates this file in the same PR.

## Conventions

- **JSON over HTTPS.** Request and response bodies are JSON (`Content-Type: application/json; charset=utf-8`). Locally the app runs over plain HTTP on `127.0.0.1`.
- **Same origin.** The API and the UI are served by the same Go service from the same origin, so there's no CORS.
- **Paths.** API endpoints live under `/api/`. Login endpoints live under `/auth/` (browser navigations and redirects, not JSON, except logout). Any other path is a static UI file. An unknown `/api/` path, or a known path with an unsupported method, returns the `not_found` error below (or `not_logged_in` first, if there's no session).
- **Session.** An encrypted, `HttpOnly` cookie (`setlisted_session`) set by the Spotify login. The UI can't read it and doesn't need to: the browser sends it automatically. It ends when the browser session ends or one hour after login, whichever comes first (spec AUTH-6, ADR-009).
- **Protected by default.** Every `/api/` endpoint **requires a session**, except `GET /api/health` and `GET /api/session`. Without a valid session it returns `401 not_logged_in`, and the UI shows the login screen (AUTH-8). The same happens when Spotify rejects the user's token.
- **Same-origin check.** Every request other than `GET` and `HEAD` must carry an `Origin` header equal to the app's own origin (`BASE_URL`), or it gets `403 forbidden_origin`. Browsers send it automatically with `fetch` from the app's own pages.
- **Caching.** JSON responses are sent with `Cache-Control: no-store`.
- **No technical details.** Error messages never contain status codes, stack traces or raw upstream responses (spec ERR-5).

## Errors

Every error response uses one format, with an HTTP status that matches the error:

```json
{ "error": { "code": "upstream_unavailable", "message": "Couldn't reach setlist.fm. Try again.", "service": "setlist.fm" } }
```

| Field | Always present? | Meaning |
|---|---|---|
| `code` | yes | Stable, machine-readable string. The UI can branch on it. |
| `message` | yes | Safe, user-facing text that the UI can show as-is. |
| `service` | no | The external service involved (`setlist.fm` or `Spotify`), only when there is one. |

### Codes

| Code | HTTP status | When |
|---|---|---|
| `invalid_request` | 400 | The request is malformed or a parameter is missing or invalid. |
| `not_found` | 404 | No such endpoint or resource. |
| `not_logged_in` | 401 | No valid session (missing, invalid or expired cookie), or Spotify rejected the user's token. The session cookie is cleared. Message: "You're not logged in. Log in with Spotify." |
| `forbidden_origin` | 403 | A request other than `GET`/`HEAD` with a missing or foreign `Origin` header. Message: "This request isn't allowed." |
| `internal_error` | 500 | An unexpected server error. Message: "Something went wrong. Try again." |

Later steps add codes (e.g. `upstream_unavailable`) to this table.

## Endpoints

### `GET /api/health`

Liveness check. Needs no session.

**Response `200`:**

```json
{ "status": "ok" }
```

### `GET /api/session`

Whether the browser has a valid session. Needs no session. Never returns tokens or the user's Spotify ID. An invalid or expired cookie is cleared.

**Response `200`:**

```json
{ "loggedIn": true }
```

or `{ "loggedIn": false }`.

## Login endpoints

These are browser navigations, not `fetch` calls (except logout).

### `GET /auth/login`

Starts the Spotify login: sets a short-lived login cookie (state and PKCE verifier, 10 minutes) and redirects (`302`) to Spotify's authorise page, asking only for `playlist-modify-private`. The UI links to it ("Log in with Spotify").

### `GET /auth/callback`

Where Spotify sends the user back (the registered redirect URI). Always redirects (`302`):

| Outcome | Redirect to | UI shows |
|---|---|---|
| Logged in: session cookie set | `/` | the search screen (AUTH-3) |
| The user cancelled or declined on Spotify | `/?login=cancelled` | "Login was cancelled." (AUTH-5) |
| Spotify refused the user (not allowlisted) | `/?login=invite_only` | "Setlisted is invite-only. Ask the owner to add your Spotify account." (AUTH-4) |
| Anything else (bad or missing state, expired login cookie, Spotify error) | `/?login=failed` | a generic login error (wording chosen in step 12; no technical details, ERR-5) |

No session cookie is set except on success.

### `POST /auth/logout`

Clears the session cookie (AUTH-7). Works whether or not the user is logged in. Subject to the same-origin check. No request body.

**Response `204`** (no body). Then show the login screen.
