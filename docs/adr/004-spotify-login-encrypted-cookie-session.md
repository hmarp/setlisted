# ADR-004: Spotify login with a stateless, encrypted-cookie session

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

- Users log in with Spotify only, and nothing is available before login (spec AUTH-1 to AUTH-5).
- A login lasts until the browser session ends or one hour passes, whichever comes first, with no silent renewal (AUTH-6). Logging out discards everything (AUTH-7). An expired or invalid session sends the user back to login (AUTH-8).
- The app stores **no user data** (GEN-5), and the server must be **stateless** because Cloud Run instances can stop at any time (ADR-003).
- The backend owns all Spotify calls, and the browser never sees secrets or tokens (ADR-001, ADR-002).
- In Spotify development mode, a non-allowlisted user can complete Spotify's login screen, but their subsequent API calls are refused (AUTH-4).
- Spotify access tokens are valid for one hour.

## Decision

**Login flow**

- Use the OAuth 2.0 **Authorization Code flow with PKCE**, run entirely by the Go backend using the official **`golang.org/x/oauth2`** package. This dependency is accepted under ADR-001.
- Request only the **`playlist-modify-private`** scope (AUTH-2).
- Protect the flow with a random **`state`** value and a **PKCE verifier**, held in a short-lived (a few minutes), encrypted, `HttpOnly` cookie for the round trip to Spotify and deleted on return.
- On return from Spotify:
  - if the user cancelled or declined, show the AUTH-5 message;
  - otherwise exchange the code for tokens, then call **`GET /me`**. If Spotify refuses this because the user isn't allowlisted, show the AUTH-4 invite-only message and create no session.
- The **refresh token is discarded immediately**. It is never stored, sent to the browser or logged.

**Session**

- The session is a single cookie holding the **access token** and its **expiry**, plus the minimum user identity needed to create playlists (the Spotify user ID). It is encrypted and authenticated with **AES-GCM** (Go standard library `crypto/aes`, `crypto/cipher`) using a server-side key supplied by configuration (secrets ADR to follow).
- Cookie attributes: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`, and **`Max-Age` = the access token's remaining lifetime** (at most one hour). Browsers also drop it when the browser session ends where they honour that, and the one-hour limit applies regardless.
- The server keeps **no session state**. Every request is authenticated by decrypting its cookie, so any instance can serve any request.
- A missing, undecryptable or expired cookie, or a Spotify rejection of the token, is treated as **logged out**. API calls return a "not logged in" response, and the UI sends the user to the login screen (AUTH-8).
- **Log out** clears the cookie (AUTH-7). There's nothing to revoke on the server.

**Protecting state-changing requests**

- State-changing API calls (e.g. create playlist) are `POST`s. `SameSite=Lax` prevents cross-site requests carrying the cookie, and in addition the backend rejects `POST`s whose `Origin` header isn't the app's own origin.

**Key rotation**

- Changing the encryption key logs everyone out, because existing cookies no longer decrypt. That's acceptable, and it's a simple way to force everyone to log in again.

## Alternatives considered

- **Silent token refresh** (keep the refresh token in the cookie and renew it). Logins would last beyond an hour, but it adds complexity, and it means a long-lived credential sits in the browser. The owner confirmed nobody uses the app for more than an hour at a time.
- **Server-memory sessions.** Breaks the statelessness rule (ADR-003). Sessions would vanish when instances stop or change.
- **A session store** (Firestore, Redis, a database). Stores user data, which goes against GEN-5, and adds a component and possible cost.
- **Tokens held by browser JavaScript** (localStorage, or the implicit/PKCE flow in the browser). Any script injection could leak the token, and it goes against ADR-001's rule that the backend owns Spotify calls.
- **Signed but not encrypted cookie** (e.g. a JWT). The access token would be readable by anyone holding the cookie, for example in logs or browser storage. Encrypting it costs nothing extra.
- **Third-party session libraries** (e.g. `gorilla/sessions`). Not needed: encrypting a cookie takes a few dozen lines with the standard library, and ADR-001 prefers standard-library code.

## Consequences

- Fully stateless and nothing persisted, matching GEN-5 and ADR-003.
- Users log in again at least once an hour and on each new browser session. This is accepted for the MVP.
- If a session expires partway through the journey, the user starts again (AUTH-8). There is no state to restore.
- The cookie encryption key becomes a critical secret. Leaking it would let someone forge or read sessions, though only for at most an hour per token. Its handling is covered by the secrets ADR.
- The cookie stays small (an access token plus a few fields), well within browser limits.
- Local development needs the Spotify redirect URL registered as `http://127.0.0.1:<port>/…`, because Spotify doesn't accept `localhost`. The plan must confirm that browsers accept the `Secure` cookie over plain HTTP on `127.0.0.1`. If they don't, local development uses a development-only setting or local HTTPS.
