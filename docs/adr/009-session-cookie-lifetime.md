# ADR-009: Session cookie lifetime

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** the `Max-Age` part of ADR-004's cookie attributes. The rest of ADR-004 stands.

## Context

- Spec AUTH-6: a login lasts until **the browser session ends or one hour has passed since login, whichever comes first**. GEN-5 says nothing about a user is kept after their browser session ends.
- ADR-004 sets the session cookie's **`Max-Age` to the access token's remaining lifetime**, and says browsers "also drop it when the browser session ends".
- That isn't how cookies work. A cookie with `Max-Age` (or `Expires`) is a **persistent** cookie: browsers keep it across restarts until it expires. Only a cookie with neither attribute is a session cookie that ends with the browser session. Following ADR-004 would keep a user logged in for up to an hour after closing the browser, which breaks AUTH-6.
- This was found while implementing step 04, before any session code existed.

## Decision

- The session cookie is a **session cookie**: it has **no `Max-Age` and no `Expires`**. The browser drops it when the browser session ends.
- The **one-hour limit is enforced by the server**: the encrypted cookie holds the access token's expiry (at most one hour after login), and a cookie past that expiry is treated as logged out, as ADR-004 already requires.
- All other attributes are unchanged: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`.
- Clearing the cookie (log out, Spotify 401) still uses `Max-Age=0`.
- The short-lived state and PKCE verifier cookie for the login round trip keeps its own `Max-Age` of a few minutes. Outliving a browser restart within those minutes is harmless: it is single-use, holds no token and is deleted on return.

## Alternatives considered

- **Keep `Max-Age` and amend AUTH-6** to "one hour after login". Rejected: the browser-session limit is a deliberate spec requirement, and meeting it costs nothing.
- **`Max-Age` plus a client-side check.** The UI can't read an `HttpOnly` cookie and can't detect a browser restart reliably.

## Consequences

- AUTH-6 holds in both directions: closing the browser ends the login, and after one hour the server refuses the cookie even if the browser still sends it.
- An expired cookie may still be sent by the browser until the browser session ends. The server ignores it, and the UI sends the user to login (AUTH-8). The next login overwrites it.
- Some browsers restore session cookies when they "continue where you left off" after a restart. In that case the one-hour server-side limit still applies. This is accepted.
