# Step 04: Spotify login and session

- **Depends on:** step 03 (merges deploy automatically)
- **Branch:** `step-04-spotify-login`
- **Handoffs to read:** `docs/plan/01-go-skeleton/handoff.md`, `docs/plan/02-first-deploy/handoff.md`, `docs/plan/03-continuous-deployment/handoff.md`

## Goal

Users can log in with Spotify, stay logged in for up to an hour in a stateless encrypted-cookie session, and log out. Non-allowlisted accounts get the invite-only message. Every `/api/*` endpoint other than health and session status requires a session.

## Spec requirements (quoted from `docs/spec.md` §3)

- **AUTH-1** Before login, the only things a visitor can see are the app name, a one-line description, a **"Log in with Spotify"** action and the footer. No search, show or setlist data is available without login.
- **AUTH-2** Logging in goes through Spotify's own login and consent. The app asks only for the Spotify permissions it needs to read the user's identity, search the catalogue, and create a private playlist with tracks in the user's library.
- **AUTH-3** After a successful login, the user lands on the search screen.
- **AUTH-4** If Spotify refuses the user because their account isn't allowlisted, the app shows an **invite-only message**: "Setlisted is invite-only. Ask the owner to add your Spotify account." No raw Spotify error is shown.
- **AUTH-5** If the user cancels or declines on Spotify's consent screen, they're returned to the login screen with the message "Login was cancelled."
- **AUTH-6** A login lasts until **the browser session ends or one hour has passed since login, whichever comes first**. After that, the user must log in again. There is no silent renewal. (If Spotify remembers their consent, logging in again is usually one or two taps.)
- **AUTH-7** Every logged-in screen has a **"Log out"** action. Logging out discards the session completely and returns to the login screen.
- **AUTH-8** If the session has expired or become invalid at any point, the user is sent to the login screen. After logging in again they start at the search screen. The app does not restore their previous place.
- **GEN-5** The app stores **no user data**. Nothing about a user, their searches, matches or playlists is kept after their browser session ends or they log out. The only state held is what's needed to act on the user's behalf during their current session.
- **ERR-5** No error message shows technical details (status codes, stack traces, raw responses).

The UI screens and messages themselves are built in step 12. This step provides the endpoints, redirects and outcomes the UI needs. The placeholder page can show a bare login link for manual testing.

## Decisions this step relies on

**ADR-004: Spotify login with a stateless, encrypted-cookie session.**

- **Login flow:**
  - **Authorization Code flow with PKCE**, run entirely by Go using **`golang.org/x/oauth2`**.
  - Scope: only **`playlist-modify-private`**.
  - A random **`state`** and the **PKCE verifier** are held in a short-lived (a few minutes) encrypted `HttpOnly` cookie for the round trip, and deleted on return.
- **On return from Spotify:**
  - cancelled or declined → AUTH-5;
  - otherwise, exchange the code, then call **`GET /me`**. If Spotify refuses this because the user isn't allowlisted, show AUTH-4 and create no session. (In development mode a non-allowlisted user can complete Spotify's login screen, but their API calls are refused, typically with HTTP 403.)
  - **The refresh token is discarded immediately.** It is never stored, sent to the browser or logged.
- **Session cookie:**
  - It holds the **access token**, its **expiry** and the **Spotify user ID**, encrypted and authenticated with **AES-GCM** (`crypto/aes`, `crypto/cipher`) using `SESSION_KEY`.
  - Attributes: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`, and **`Max-Age` = the token's remaining lifetime** (at most one hour).
  - The server keeps **no session state**.
  - A missing, undecryptable or expired cookie, or Spotify rejecting the token, means **logged out**.
  - Logging out clears the cookie.
- **State-changing requests:** POSTs are protected by `SameSite=Lax`, **plus** a check that the `Origin` header equals the app's own origin (from `BASE_URL`). A POST with a missing or different `Origin` is rejected.
- **Key rotation:** changing `SESSION_KEY` logs everyone out. That's acceptable.
- **Local development:** Spotify doesn't accept `localhost`, so register `http://127.0.0.1:<port>/…`. **Confirm in this step** that browsers accept the `Secure` cookie over plain HTTP on `127.0.0.1`. If they don't, add a development-only setting (documented in `.env.example`, and never set in production) or use local HTTPS.
- **ADR-007:** a **single Spotify developer app** for local and production, with both redirect URLs registered.
- **Spotify (February 2026 development-mode rules):**
  - the app owner must have Premium;
  - up to 5 allowlisted users;
  - `GET /me` no longer returns email.

## Owner tasks

Put these in `docs/plan/04-spotify-login/setup.md`.

1. In the Spotify Developer Dashboard, create the app (if it doesn't exist).
2. Register the redirect URIs:
   - `http://127.0.0.1:8080/auth/callback`
   - `<BASE_URL from step 02>/auth/callback`
3. Allowlist the owner's own account and the friends' accounts (User Management).
4. Put the real `SPOTIFY_CLIENT_ID` into the Cloud Run service's environment, and the real secret into the `spotify-client-secret` secret (a new version). Update the local `.env`.

## What to build

1. **`internal/auth`:**
   - cookie encryption and decryption: AES-GCM, random nonce per cookie, base64url encoding;
   - the session type (access token, expiry, user ID);
   - helpers to set and clear the cookie;
   - middleware that puts the session into the request context, or answers `401 not_logged_in` for protected routes;
   - the `Origin`-check middleware for POSTs.
2. **Endpoints.** Document them all in `docs/api.md`.

   | Route | Behaviour |
   |---|---|
   | `GET /auth/login` | Create the state and verifier cookie, then redirect to Spotify's authorise URL. |
   | `GET /auth/callback` | Validate `state`. On `error=access_denied`, redirect to `/?login=cancelled`. Otherwise exchange the code, call `GET /me`, and on refusal redirect to `/?login=invite_only` with no session. On success, set the session cookie and redirect to `/` (the UI shows search, AUTH-3). Any other failure redirects to `/?login=failed`. |
   | `POST /auth/logout` | Clear the cookie and return `204`. Subject to the `Origin` check. |
   | `GET /api/session` | Not protected. Returns `{"loggedIn": true}` or `{"loggedIn": false}`. Don't return tokens. |

3. **Protected routes.** Apply the session middleware to every `/api/*` route except `/api/health` and `/api/session`. No such routes exist yet. Add a test-only route, or check the middleware directly, to prove protection works.
4. **Spotify 401 helper.** Add a small helper that later steps use whenever Spotify answers `401` to a user's token: clear the cookie and return `401 not_logged_in` (AUTH-8). Add the error codes `not_logged_in` and `forbidden_origin` to the error table in `docs/api.md`.
5. **Logging.** Never log tokens, codes, cookies or the Spotify user ID.

## Tests (`httptest`, with a fake Spotify accounts/API server)

- Login redirect includes the correct client ID, the redirect URI, scope `playlist-modify-private`, `state` and a PKCE challenge.
- Callback with:
  - a bad or missing `state` → rejected;
  - `access_denied` → `/?login=cancelled`;
  - `/me` returning 403 → `/?login=invite_only` and no session cookie;
  - success → a session cookie with the right attributes and a `Max-Age` of at most 3600.
- The refresh token doesn't appear in the cookie (decrypt and inspect it in the test).
- A tampered cookie, a cookie encrypted with a different key, and an expired session are all treated as logged out.
- The `Origin` check: a POST with no `Origin`, or a foreign one → `403 forbidden_origin`; a matching `Origin` → allowed.
- `GET /api/session` reports correctly in both states.

## Out of scope

- Calling Spotify search or creating playlists (steps 08, 11).
- UI screens (step 12).

## Done when

- The tests pass in CI.
- **Locally on `127.0.0.1`:** logging in with an allowlisted account sets the cookie, and `/api/session` reports logged in. Logging out clears it. The `Secure` cookie behaviour is confirmed and recorded.
- **On Cloud Run, after the merge deploys:** the same login and logout work.
- A non-allowlisted account (if one is available to test with) lands on `/?login=invite_only`. If none is available, record that this was covered by tests only.

## End of step (same PR)

- Write `handoff.md`, including:
  - the cookie name and format;
  - how handlers read the session (the context helper);
  - how to protect a new route;
  - the Spotify 401 helper;
  - the `Secure`/`127.0.0.1` finding;
  - the `/?login=` values the UI must handle.
- Mark step 04 done in `docs/plan.md`.
- Update `README.md` (Spotify app setup, login for local development) and `.env.example` if anything changed.
