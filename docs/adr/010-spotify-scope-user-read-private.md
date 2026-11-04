# ADR-010: Request `user-read-private` for country-aware search

- **Status:** Accepted
- **Date:** 2026-11-04
- **Supersedes:** ADR-004's "Request only the `playlist-modify-private` scope". The rest of ADR-004 (and ADR-009) stands.

## Context

- ADR-005 searches Spotify with **`market=from_token`**, so that candidates are playable in the user's country. ADR-004 requests only **`playlist-modify-private`**.
- Step 08's live check used an app token with an explicit market (`GB`), so the combination of a **user token and `from_token`** was first exercised in step 10's local check (2026-11-04). Every search answered **`403` "Insufficient client scope"**, so no preview could be built.
- Two local experiments with the owner's account, the same day:
  1. the same login (`playlist-modify-private` only) with `market=GB` instead of `from_token` → **`200`**;
  2. `from_token` with the login also granting **`user-read-private`** → **`200`**.
- So searching with a user token needs no scope, but `from_token` does: Spotify has to read the user's country, which `user-read-private` grants.
- Spec AUTH-2 allows "the Spotify permissions it needs to read the user's identity, search the catalogue, and create a private playlist". Country-aware search is part of searching the catalogue, so the spec doesn't change.

## Decision

- Request **`playlist-modify-private user-read-private`** at login. Nothing else changes in the login flow, the session or ADR-005's search strategy (`market=from_token` stays).
- The app **doesn't read or keep** anything `user-read-private` exposes (country, subscription level, explicit-content setting). Only Spotify uses it to apply the market. The session still holds only the access token, its expiry and the user ID (GEN-5).

## Alternatives considered

- **A fixed market from configuration** (e.g. `SPOTIFY_MARKET=GB`), keeping one scope. Rejected: a friend outside that country could get tracks that aren't playable where they are, which is what ADR-005's `from_token` exists to avoid. It also adds a setting to keep right.
- **Omit `market`.** Without a market, Spotify doesn't filter by availability or relink tracks, so the same problem as a fixed market, and it wasn't tested.
- **Read the country from `GET /me` and pass it explicitly.** `/me` returns `country` only with `user-read-private` too, so it needs the same scope for more code.

## Consequences

- Spotify's consent screen lists one more permission (reading account details such as country and subscription type). Users who already consented are asked again at their next login.
- **Sessions created before the change carry a token without the new scope**, so their previews fail with "Couldn't reach Spotify" until they log in again. Sessions last at most an hour (ADR-009), so this clears itself within an hour of the deploy; logging out and in fixes it at once.
- Step 11's playlist calls are unaffected (`playlist-modify-private` is still requested).
