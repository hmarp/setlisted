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
| `upstream_unavailable` | 503 | setlist.fm (or, from step 08, Spotify) couldn't be reached, returned an error, or was still rate-limiting after the server's quiet retries. `service` names it. Message: "Couldn't reach setlist.fm. Try again." (or "…Spotify…"). The UI shows it with a **Retry** action that repeats the same request (ERR-1, ERR-2). |
| `internal_error` | 500 | An unexpected server error. Message: "Something went wrong. Try again." |

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

### `GET /api/artists?q=…`

Searches setlist.fm for artists named `q` (SRCH-2) and applies the clear-match rule. Needs a session.

| Parameter | Rule |
|---|---|
| `q` | Required. The band name as typed. Surrounding spaces are ignored. Empty or whitespace-only → `400 invalid_request` ("Enter a band name."), as is anything over 200 characters ("That band name is too long."). |

**Response `200`:**

```json
{
  "match": "clear",
  "artists": [
    { "id": "5a85c140-dcf8-4a4f-b8d6-1e5a2a4e0f4b", "name": "Gojira", "disambiguation": "French metal band" }
  ]
}
```

| `match` | Meaning | `artists` | UI |
|---|---|---|---|
| `clear` | Exactly one result's normalised name equals the normalised `q` (case, accents and spaces ignored; `sword` ≠ `The Sword`). | that one artist | go straight to its shows (SRCH-3) |
| `picker` | Results, but no single exact match (none, or several). | up to 10, in setlist.fm's relevance order | the artist picker: name, plus `disambiguation` when it isn't empty (SRCH-4) |
| `none` | No results. | `[]` | "No artists found for '‹q›' on setlist.fm.", keeping the text in the box (SRCH-5) |

- `id` is the artist's MusicBrainz ID (MBID). Use it in the show list path.
- `disambiguation` is always present and is `""` when setlist.fm has none.

**Errors:** `400 invalid_request`, `503 upstream_unavailable` (`service: "setlist.fm"`).

### `GET /api/artists/{id}/shows`

The band's up to 20 most recent shows that have at least one entry, newest first (SHOW-1, SHOW-2). Needs a session. `{id}` is an artist `id` from the search.

**Response `200`:**

```json
{
  "artist": { "id": "5a85c140-dcf8-4a4f-b8d6-1e5a2a4e0f4b", "name": "Gojira" },
  "shows": [
    { "id": "63de4613", "date": "14 Mar 2026", "city": "Leipzig", "venue": "Quarterback Immobilien Arena", "tour": "European Tour 2026", "songCount": 17 },
    { "id": "2bd6a4f2", "date": "02 Mar 2026", "city": "Paris", "venue": "Zénith", "tour": null, "songCount": 15 }
  ]
}
```

| Field | Meaning |
|---|---|
| `artist.name` | The band's name, for the heading (SHOW-1) and SHOW-5's message. |
| `shows[].id` | The setlist.fm setlist ID. The preview (step 10) takes it. |
| `shows[].date` | Already formatted as `DD Mon YYYY` with English month abbreviations (SHOW-3). |
| `shows[].city`, `shows[].venue` | As setlist.fm has them. |
| `shows[].tour` | The tour name, or `null` when setlist.fm has none. |
| `shows[].songCount` | Entries across all sets, including tapes (SHOW-3). Always at least 1. |

- `shows` is `[]` when the band has no shows with entries: show "No setlists found for ‹artist.name› on setlist.fm." (SHOW-5).
- The server reads at most 3 pages (60 setlists) of setlist.fm's history to find the 20, so a band with many empty (e.g. announced) shows can get fewer than 20.

**Errors:** `404 not_found` for an ID that isn't an MBID or that setlist.fm doesn't know; `503 upstream_unavailable` (`service: "setlist.fm"`).

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
| Anything else (bad or missing state, expired login cookie, other Spotify error) | `/?login=failed` | a generic login error (wording chosen in step 12; no technical details, ERR-5) |

No session cookie is set except on success. Any other outcome also clears an existing session cookie, so a refused login never leaves a previous user logged in. `HEAD` on `/auth/login` and `/auth/callback` answers `405` and does nothing.

### `POST /auth/logout`

Clears the session cookie (AUTH-7). Works whether or not the user is logged in. Subject to the same-origin check. No request body.

**Response `204`** (no body). Then show the login screen.
