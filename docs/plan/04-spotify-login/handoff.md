# Step 04 handoff: Spotify login and session

## What was built

- **`internal/auth`**: Spotify login (Authorization Code flow with PKCE via `golang.org/x/oauth2`), the AES-GCM encrypted session cookie, the session and same-origin middleware, and the Spotify 401 helper.
- **Endpoints** (all in `docs/api.md`): `GET /auth/login`, `GET /auth/callback`, `POST /auth/logout`, `GET /api/session`.
- **Every `/api/` route except `/api/health` and `/api/session` requires a session**, including unknown paths.
- **Every request other than `GET`/`HEAD` must carry the app's own `Origin`**, on every path.
- New error codes `not_logged_in` (401) and `forbidden_origin` (403) in `internal/jsonapi` and `docs/api.md`.
- `BASE_URL` is now validated: `scheme://host[:port]`, `http` or `https`, no path, query or fragment (a trailing `/` is still trimmed).
- The placeholder page has a bare "Log in with Spotify" link, a "Log out" button, the session status and the `?login=` value, for manual testing only. Step 12 replaces it.
- **ADR-009** (see below), `docs/plan/04-spotify-login/setup.md` (owner checklist), README, `.env.example`.

## Decision taken in this step: ADR-009 (session cookie lifetime)

The brief and ADR-004 said the session cookie gets `Max-Age` = the token's remaining lifetime. A cookie with `Max-Age` is **persistent**: it survives closing the browser, which breaks spec AUTH-6 ("until the browser session ends or one hour has passed"). This was flagged to the owner, who chose to supersede that part of ADR-004:

- The session cookie has **no `Max-Age` and no `Expires`** (a browser-session cookie).
- The **one hour is enforced by the server** from the expiry inside the encrypted cookie.
- ADR-004's status line points to ADR-009; nothing else in ADR-004 changed. The step 04 brief and the step 12 brief's expiry check were updated.

## Cookies

| Cookie | Path | Lifetime | Content |
|---|---|---|---|
| `setlisted_session` (`auth.SessionCookie`) | `/` | browser session; server-side expiry ≤ 1 hour | `Session` |
| `setlisted_login` | `/auth` | `Max-Age=600` (10 minutes); deleted on return | state + PKCE verifier |

- Both are `HttpOnly`, `Secure`, `SameSite=Lax`, with no `Domain`.
- **Format:** `base64url-no-padding(nonce ‖ AES-256-GCM ciphertext)`. A random 12-byte nonce for each cookie. The key is `SESSION_KEY`. The plaintext is JSON. **The cookie name is the GCM additional data**, so a login cookie can't be passed off as a session cookie.
- Session JSON: `{"t": access token, "e": expiry (RFC 3339), "u": Spotify user ID}`. There is no refresh token: `Callback` drops it straight after the exchange.
- **Expiry** = login time + the token's `expires_in`, capped at one hour (`auth.MaxSessionAge`).
- A session is valid only if it decrypts with the current key, has a token and a user ID, and `now < expiry`. Otherwise the request counts as logged out, and the cookie is cleared in the response.

## How handlers use it

**Read the session** in a protected handler:

```go
s, ok := auth.SessionFromContext(r.Context()) // s.AccessToken, s.Expiry, s.UserID
```

`ok` is always true behind `RequireSession`. Handler unit tests can call `auth.ContextWithSession(ctx, s)` directly. Tests that go through `httpapi.NewHandler` get a valid cookie from `a.NewSessionCookie(auth.Session{...})` (see `loggedIn` in `internal/httpapi/server_test.go`).

**Protect a new route:** register it on the **`api`** sub-mux in `httpapi.NewHandler`, with a method:

```go
api.HandleFunc("GET /api/artists", h.searchArtists)
api.HandleFunc("POST /api/playlists", h.createPlaylist)
```

`api` sits behind `a.RequireSession`, so a route there can't forget the session check. Only routes registered on the outer `mux` are public. Don't add more public `/api/` routes without a spec reason. POSTs need nothing extra: `RequireSameOrigin` wraps the whole router.

**Spotify answers 401 to the user's token:** call `auth.SpotifyUnauthorized(w)` and return. It clears the cookie and writes `401 not_logged_in` (AUTH-8). Step 08's client should return a distinct "unauthorised" error so handlers know when to call it.

**Dependencies:** `httpapi.NewHandler(logger, static, a *auth.Auth)`. `main` builds `a` with `auth.New(auth.Config{...})`. `auth.Config` takes overridable Spotify URLs (`AuthURL`, `TokenURL`, `APIBaseURL`), an `HTTPClient` and `Now`, all used by tests. When later steps add the Spotify and setlist.fm clients, `NewHandler` will probably need a small dependencies struct (the step 01 handoff suggested one).

## `/?login=` values the UI must handle

| Value | Meaning | Spec |
|---|---|---|
| (none, on `/` after a login) | logged in → show search | AUTH-3 |
| `cancelled` | the user cancelled or declined on Spotify | AUTH-5: "Login was cancelled." |
| `invite_only` | `GET /me` returned 403: not allowlisted. No session. | AUTH-4: "Setlisted is invite-only. Ask the owner to add your Spotify account." |
| `failed` | anything else: bad, missing or expired state, a refused token exchange, a `/me` failure, another Spotify `error` | not in the spec. Step 12 words a generic message (ERR-5). |

Constants: `auth.LoginCancelled`, `auth.LoginInviteOnly`, `auth.LoginFailed`. The UI should remove the parameter from the address bar (`history.replaceState`) after showing the message.

## Behaviour worth knowing

- **Unknown `/api/` paths need a session too.** They answer `401` without one and `404` with one. A `POST` to any path without the right `Origin` answers `403` before routing. `POST /api/health` (with `Origin` and a session) still answers `404 not_found`.
- **State is checked before `error`.** A cancel with a missing or expired login cookie (more than 10 minutes on Spotify's page) lands on `failed`, not `cancelled`.
- **The `/me` call doesn't use `oauth2`'s client.** It's a plain request with the bearer token, so nothing ever tries to refresh.
- **Spotify calls in the callback** have a 10-second timeout.
- **Logging:** `login failed` (WARNING) with a `reason` and, where there is one, `spotify_status`; `login cancelled` and `login succeeded` (INFO). Tokens, codes, state, cookies and the user ID are never logged. A test checks this. `oauth2` error text isn't logged, because it can include Spotify's response body.
- **Key rotation:** a new `SESSION_KEY` version logs everyone out at the next revision.

## `Secure` cookie on `127.0.0.1`

- **Chromium 153 (headless, in the build sandbox):** stores a `Secure`, `HttpOnly`, `SameSite=Lax` cookie set over plain `http://127.0.0.1` and sends it back. The real app's login cookie was stored the same way. Same-origin `fetch` POSTs carry `Origin`, so logout returned `204`.
- Chrome, Edge and Firefox treat loopback addresses as secure contexts. Safari has historically rejected `Secure` cookies over plain HTTP. For local development use a Chromium browser or Firefox.
- **No development-only setting was needed**, so none was added. The app always sets `Secure`.
- Owner's browser: <fill in from setup.md §5: browser and version, result>

## Verification

Done in this session: `gofmt`, `go vet`, `go test ./...` pass. The `httptest` tests (with a fake Spotify accounts and API server) cover:

- the login redirect parameters;
- the callback: success, short-lived token, missing, wrong or foreign state, no login cookie, cancel, other errors, refused code, `/me` 403 and 500;
- cookie attributes, and no `Max-Age`;
- no refresh token in the session, and no secrets in logs or headers;
- tampered, wrong-key, expired, empty and swapped cookies;
- the `Origin` check;
- `/api/session` in both states;
- logout and the 401 helper;
- protection of `/api/*` through the real router.

The app also ran locally in headless Chromium against the real router: status, `?login=` display, login redirect and cookie, logout `204`, and `403` without `Origin`.

To do with the owner (setup.md §5–7). Fill in:

- [ ] Local login on `127.0.0.1` with an allowlisted account: cookie set, `/api/session` logged in, log out clears it. Browser: <…>
- [ ] Cancel → `/?login=cancelled`.
- [ ] Non-allowlisted account → `/?login=invite_only`, or: "no test account available; covered by tests only".
- [ ] Cloud Run: real `SPOTIFY_CLIENT_ID` and secret version set (setup.md §6).
- [ ] After the merge deploys: the same login and logout work on `https://setlisted-613938414647.europe-west1.run.app`.

## Known gaps

- **AUTH-1, AUTH-3 and AUTH-7 screens** are step 12. The placeholder page shows only test controls.
- **No security headers** on HTML yet (still as in step 01).
- **Spotify can't be asked to revoke tokens**, so logging out only deletes the cookie. A copied cookie stays usable until its expiry (at most an hour). This is accepted in ADR-004.
- **Some browsers restore session cookies** when they "continue where you left off". The one-hour server-side limit still applies (ADR-009).
- **`SPOTIFY_CLIENT_ID` in `scripts/deploy-config.sh`** must hold the real ID before merging (it's `REPLACE_ME` until the owner provides it, and `scripts/deploy.sh` refuses to run until then).
