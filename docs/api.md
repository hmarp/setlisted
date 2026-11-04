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
| `shows[].date` | Already formatted as `DD Mon YYYY` with English month abbreviations (SHOW-3). In the unlikely case setlist.fm sends a date that isn't `dd-MM-yyyy`, its text is passed through unchanged rather than the show being dropped. |
| `shows[].city`, `shows[].venue` | As setlist.fm has them. |
| `shows[].tour` | The tour name, or `null` when setlist.fm has none. |
| `shows[].songCount` | Entries across all sets, including tapes (SHOW-3). Always at least 1. |

- `shows` is `[]` when the band has no shows with entries: show "No setlists found for ‹artist.name› on setlist.fm." (SHOW-5).
- The server reads at most 3 pages (60 setlists) of setlist.fm's history to find the 20, so a band with many empty (e.g. announced) shows can get fewer than 20.

**Errors:** `404 not_found` for an ID that isn't an MBID or that setlist.fm doesn't know; `503 upstream_unavailable` (`service: "setlist.fm"`).

### `GET /api/shows/{id}/preview`

The preview of one show (PREV-2 to PREV-4, PREV-7): the setlist fetched from setlist.fm, classified, and every song matched on Spotify with the user's token (`market=from_token`). Needs a session. `{id}` is a show `id` from the show list. Matching runs on every call (no caching, ADR-005) and typically takes a few seconds; show "Matching songs…" while waiting (PREV-1).

**Response `200`:**

```json
{
  "show": { "id": "63de4613", "band": "Gojira", "date": "14 Mar 2026", "venue": "Haus Auensee", "city": "Leipzig",
            "tour": "Mea Culpa Tour", "setlistUrl": "https://www.setlist.fm/setlist/gojira/2026/haus-auensee-leipzig-germany-63de4613.html" },
  "summary": { "found": 7, "matchable": 9 },
  "canCreate": true,
  "sets": [
    { "heading": "", "entries": [
      { "position": 1, "name": "Intro", "note": null, "tape": true, "status": "found", "live": false, "originalArtist": null,
        "track": { "id": "4uLU6hMCjMI75M1A2tKUQC", "title": "Intro", "artists": ["Gojira"], "album": "Magma" } },
      { "position": 2, "name": "Silvera", "note": "extended outro", "tape": false, "status": "live", "live": true, "originalArtist": null,
        "track": { "id": "…", "title": "Silvera - Live", "artists": ["Gojira"], "album": "Live at Wacken" } },
      { "position": 3, "name": "Medley: Backbone / Remembrance", "note": null, "tape": false, "status": "not_found_medley", "live": false, "originalArtist": null, "track": null }
    ]},
    { "heading": "Encore 1", "entries": [
      { "position": 4, "name": "Hellion", "note": null, "tape": false, "status": "cover_fallback", "live": true, "originalArtist": "Judas Priest",
        "track": { "id": "…", "title": "Hellion - Live", "artists": ["Judas Priest"], "album": "Unleashed in the East (Live)" } }
    ]}
  ],
  "trackIds": ["4uLU6hMCjMI75M1A2tKUQC", "…", "…"]
}
```

| Field | Meaning |
|---|---|
| `show.id` | The setlist.fm setlist ID (the same as `{id}`). Create (step 11) takes it. |
| `show.band`, `show.venue`, `show.city` | As setlist.fm has them (PREV-2). |
| `show.date` | `DD Mon YYYY`, as in the show list (or setlist.fm's text if it doesn't parse). |
| `show.tour` | The tour name, or `null` when there's none. |
| `show.setlistUrl` | The show's page on setlist.fm, for PREV-2's link. |
| `summary.found` | Entries with any of the three Found statuses (`found`, `cover_fallback`, `live`). |
| `summary.matchable` | Entries that aren't skipped and aren't medleys (spec §1). Show "‹found› of ‹matchable› songs found" (PREV-2). |
| `canCreate` | `false` exactly when `summary.found` is `0`. Then disable "Create playlist" and show "None of these songs were found on Spotify." (PREV-7). |
| `sets` | In setlist order. The first is the main set, with `heading` `""` (no heading shown). Every later set has a non-empty `heading` to show above it: setlist.fm's set name, else `Encore N`, else `Set N` (PREV-3). A set whose entries are all skipped is left out, so no heading appears with nothing under it. `[]` if the show has nothing to show. |
| `entries[].position` | The row number, 1…n across the whole show (it doesn't restart per set). Skipped entries (solos, jams, nameless entries) aren't in the response at all and take no number. |
| `entries[].name` | The entry exactly as on setlist.fm. |
| `entries[].note` | setlist.fm's note, shown as secondary text, or `null`. |
| `entries[].tape` | `true` for a tape: add a **"Tape"** label, whatever the status (PREV-4). |
| `entries[].status` | One of the values below. |
| `entries[].live` | `true` when the chosen track is a live version: always with `status: "live"`, and possibly with `cover_fallback`. |
| `entries[].originalArtist` | The covered artist for **every cover** (setlist.fm's `cover`), else `null`. Only show "Cover → …" when `status` is `cover_fallback`: a cover found under the band itself is a plain Found. |
| `entries[].track` | The matched Spotify track (`id`, `title`, `artists` in Spotify's order, `album`), present exactly when the status is a Found one, else `null`. |
| `trackIds` | The ordered `track.id`s of every found entry, in setlist order, **duplicates kept** (ENT-7), not-found and medley entries left out. Send it unchanged with `show.id` to create (step 11). `[]` when nothing was found. |

**Status values and labels (PREV-4):**

| `status` | `live` | Row shows |
|---|---|---|
| `found` | `false` | Found, plus the track's title, artist(s) and album |
| `live` | `true` | "Live version", plus the track details |
| `cover_fallback` | `false` | "Cover → ‹originalArtist›", plus the track details |
| `cover_fallback` | `true` | "Cover → ‹originalArtist›" **and** "Live version", plus the track details |
| `not_found` | `false` | Not found |
| `not_found_medley` | `false` | Not found (medley) |

Add "Tape" to any of them when `tape` is `true`. The preview is read-only (PREV-5): nothing in the response is meant to be edited or reordered.

**Errors:**

| Status | Code | When | UI |
|---|---|---|---|
| `404` | `not_found` | `{id}` isn't a setlist ID (letters and digits only), or setlist.fm doesn't know it | — |
| `503` | `upstream_unavailable`, `service: "setlist.fm"` | setlist.fm failed, timed out or kept rate-limiting | "Couldn't reach setlist.fm. Try again." with Retry, which repeats the preview (ERR-1) |
| `503` | `upstream_unavailable`, `service: "Spotify"` | a Spotify search failed or kept rate-limiting, or matching didn't finish within the server's budget (50 s for the whole request) | "Couldn't reach Spotify. Try again." with Retry (ERR-1) |
| `401` | `not_logged_in` | no valid session, or Spotify rejected the user's token (the session cookie is cleared) | the login screen (AUTH-8) |

Any error means no partial preview: Retry fetches and matches the whole show again.

## Login endpoints

These are browser navigations, not `fetch` calls (except logout).

### `GET /auth/login`

Starts the Spotify login: sets a short-lived login cookie (state and PKCE verifier, 10 minutes) and redirects (`302`) to Spotify's authorise page, asking for `playlist-modify-private` and `user-read-private` (ADR-010: the second lets searches use the user's country). The UI links to it ("Log in with Spotify").

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
