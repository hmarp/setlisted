# Step 08: Spotify search client and matching pipeline

- **Depends on:** step 04 (session and Spotify 401 helper), step 07 (matching rules)
- **Branch:** `step-08-spotify-search-pipeline`
- **Handoffs to read:** `docs/plan/04-spotify-login/handoff.md`, `docs/plan/06-entry-classification/handoff.md`, `docs/plan/07-matching-rules/handoff.md`

## Goal

A Spotify search client, plus a **pipeline** that takes a classified show (step 06), runs the needed searches, applies the matching rules (step 07), and returns a per-entry result ready for the preview API (step 10).

## Spec requirements (quoted from `docs/spec.md`)

- **MATCH-2** For a **cover** (including a tape attributed to another artist), matching runs in two passes:
  1. Look for the **band's own recording** (expected artist = band).
  2. If there isn't one, look for the **original artist's** recording (expected artist = original artist). A match from this pass is labelled as a **cover fallback**.
- **MATCH-9** If there is no candidate after the steps above, the entry is **Not found**. The app never falls back to a closest guess, a title-only search, or a different artist from the one expected.
- **ENT-7** **Duplicates** are kept. If the same song appears more than once, each occurrence is matched and placed in its own position.
- **ERR-1** If setlist.fm or Spotify can't be reached or returns an error, the app shows a plain message naming the service (e.g. "Couldn't reach setlist.fm. Try again."), with a **Retry** action that repeats the failed step.
- **ERR-5** No error message shows technical details (status codes, stack traces, raw responses).
- **AUTH-8** If the session has expired or become invalid at any point, the user is sent to the login screen. After logging in again they start at the search screen. The app does not restore their previous place.

## Decisions this step relies on

**ADR-005: Spotify calls.**

- One **field-filtered search per entry per pass**: `q=track:"…" artist:"…"`, `type=track`, **`limit=10`** (the development-mode maximum since February 2026), and **`market=from_token`**, so candidates are playable in the user's country.
- Covers can take two passes.
- Up to **4 searches in parallel** per preview, a fixed, configurable bound.
- Spotify **HTTP 429** responses are honoured by waiting for `Retry-After` and retrying. Other failures surface as ERR-1.
- No caching between requests.
- If the matching-quality test (step 09) shows studio versions being pushed out of the 10 results, fetching a second page is the first remedy. **Don't add that pre-emptively.**

**ADR-004:**
- the user's access token comes from the session in the request context;
- Spotify `401` → use step 04's helper (clear the cookie, `401 not_logged_in`).

**ADR-001:** standard library HTTP; no Spotify SDK.

**For step 09:** the pipeline must also work with a **market given explicitly** (e.g. `GB`) and a non-user token, because the fixture recorder uses an app token (client credentials), where `from_token` doesn't apply. Make the market a parameter.

## What to build

1. **`internal/spotify` search client:**
   - `SearchTracks(ctx, token, track, artist, market string) ([]matching.Candidate, error)`;
   - base URL configurable for tests;
   - timeouts;
   - **escape or strip double quotes** inside names before building the field filter, and document how;
   - map the JSON to `matching.Candidate`: `id`, `name`, `artists[].name`, `album.name`, `album.album_type`, `album.release_date`, `album.release_date_precision`;
   - also keep what the preview needs to display: track title, artist names, album name;
   - **429:** wait for `Retry-After` (with a sane cap), retry a bounded number of times, then error;
   - **401:** a distinct "unauthorised" error, so handlers can apply the step 04 helper;
   - other errors: a typed "Spotify unavailable" error.
2. **Pipeline** (e.g. `internal/preview` or `internal/pipeline`):
   - `MatchShow(ctx, show, searcher, market) ([]EntryResult, error)`;
   - for each entry of kind `song`, call step 07's `Resolve`, with candidates supplied by the searcher for each expected artist in turn. **Only search the original artist if the band pass found nothing.**
   - run at most 4 searches at once, and **keep the results in setlist order** whatever order they complete in;
   - skipped entries are omitted; medleys pass through as "not found (medley)" without searching;
   - each `EntryResult` holds:
     - the entry (position, set heading, name, note, tape flag, original artist);
     - the status: `found`, `cover_fallback`, `live`, `not_found` or `not_found_medley`;
     - the matched track's ID, title, artists and album, if any.

     A `live` match that came from the original artist's pass is both a cover fallback and live. Record how this combination is represented. The preview needs to show both labels.
   - **Errors:** if any search fails (other than "no candidates"), the whole match fails. Cancel outstanding searches and return the error. The user retries (ERR-1), so partial previews aren't shown.
   - **Searcher interface:** define a small interface for "search candidates for (track, artist)", so tests and step 09's recorded fixtures can stand in for the live client.

## Tests

- **Client (fake Spotify with `httptest`):**
  - the request carries the bearer token and `q=track:"…" artist:"…"`, `type=track`, `limit=10` and the market;
  - quotes in names are handled;
  - JSON maps to candidates;
  - a 429 with `Retry-After` is retried and then succeeds;
  - repeated 429s → error after the bound;
  - 401 → the unauthorised error;
  - 500 → the unavailable error.
- **Pipeline (fake searcher):**
  - results in setlist order despite out-of-order completion;
  - never more than 4 concurrent searches (count them in the fake);
  - a cover found under the band → a single search;
  - a cover not found under the band → a second search for the original artist, giving `cover_fallback`;
  - medleys and skipped entries aren't searched;
  - duplicates are each searched and kept;
  - one search error → the whole match errors, and outstanding searches are cancelled.

## Out of scope

- The HTTP endpoint for preview (step 10).
- Recording real fixtures and the quality test (step 09).

## Done when

- The tests pass in CI.
- A small manual check (a test behind a build tag, or a temporary local command not merged as a feature) against live Spotify with a real token returns sensible candidates for a known song. Record the result in the handoff.

## End of step (same PR)

- Write `handoff.md`, including:
  - the client and pipeline APIs;
  - the `EntryResult` shape and statuses;
  - the searcher interface (step 09 implements it from fixtures);
  - quote handling;
  - retry settings;
  - any Spotify quirks found.
- Mark step 08 done in `docs/plan.md`.
- Update `README.md` if needed.
