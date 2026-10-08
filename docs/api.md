# API reference

The JSON API served by the Go backend. The UI (`web/static/`) uses only what's documented here.

Every step that adds or changes an endpoint updates this file in the same PR.

## Conventions

- **JSON over HTTPS.** Request and response bodies are JSON (`Content-Type: application/json; charset=utf-8`). Locally the app runs over plain HTTP on `127.0.0.1`.
- **Same origin.** The API and the UI are served by the same Go service from the same origin, so there's no CORS.
- **Paths.** API endpoints live under `/api/`. Any other path is a static UI file. An unknown `/api/` path, or a known path with an unsupported method, returns the `not_found` error below.
- **Session.** A cookie session, added in step 04.
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
| `internal_error` | 500 | An unexpected server error. Message: "Something went wrong. Try again." |

Later steps add codes (e.g. `not_logged_in`, `forbidden_origin`, `upstream_unavailable`) to this table.

## Endpoints

### `GET /api/health`

Liveness check. Needs no session.

**Response `200`:**

```json
{ "status": "ok" }
```
