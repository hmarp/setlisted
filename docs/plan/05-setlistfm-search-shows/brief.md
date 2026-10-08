# Step 05: setlist.fm client, artist search and show list API

- **Depends on:** step 04
- **Branch:** `step-05-setlistfm-search-shows`
- **Handoffs to read:** `docs/plan/01-go-skeleton/handoff.md`, `docs/plan/04-spotify-login/handoff.md`

## Goal

A rate-limited setlist.fm client, plus two protected API endpoints: **artist search**, with the clear-match rule, and **show list**, with up to 20 recent non-empty shows. Also a client method to fetch a single setlist by ID, which steps 10 and 11 need.

## Spec requirements (quoted from `docs/spec.md`)

**Definitions (§1)**

- **Band:** The setlist.fm artist the user selected. Also called the *performing artist*.
- **Show:** One setlist.fm event (setlist) for the band: a date, venue, city, optional tour name, and an ordered list of entries.
- **Normalised name:** A name compared ignoring letter case, accents and leading/trailing spaces (e.g. `motorhead` = `Motörhead`).

**Artist search (§4.1)**

- **SRCH-1** The search screen has one text box for the band name and a Search action. An empty or whitespace-only search can't be submitted.
- **SRCH-2** Search uses setlist.fm's artist search with the text the user entered.
- **SRCH-3 (clear match)** If **exactly one** result has a normalised name equal to the normalised search text, the app goes straight to that band's shows (§4.2) and skips the picker.
  - The rule is strict: `sword` does **not** match `The Sword`.
- **SRCH-4 (picker)** Otherwise, if there are results, the app shows an **artist picker**: up to **10** results in setlist.fm's order, each showing the artist name and setlist.fm's disambiguation text if there is any. There is no paging. Choosing an artist goes to its shows.
  - This covers both several exact-name matches (e.g. several bands called "Ghost") and no exact match (e.g. a typo).
- **SRCH-5 (no results)** If there are no results, the app shows "No artists found for '‹search text›' on setlist.fm." The search box keeps the text so the user can edit it.

**Show list (§4.2)**

- **SHOW-1** The show list is headed with the band's name and lists up to the **20 most recent shows that have at least one entry**, newest first.
- **SHOW-2** Shows with **zero entries**, including announced future shows, are not listed.
- **SHOW-3** Each show displays: **date** (`DD Mon YYYY`), **city**, **venue**, **tour name** (if setlist.fm has one) and **song count**. Song count is the number of entries in the setlist, including tapes.
- **SHOW-4** The app does not try to detect or flag partial setlists. The user judges this from the song counts.
- **SHOW-5** If the band has no shows with entries, the app shows "No setlists found for ‹band› on setlist.fm."
- **SHOW-6** The show list has a "New search" action that returns to the search screen. Choosing a show goes to its preview (§6).

**Errors (§8)**

- **ERR-1** If setlist.fm or Spotify can't be reached or returns an error, the app shows a plain message naming the service (e.g. "Couldn't reach setlist.fm. Try again."), with a **Retry** action that repeats the failed step.
- **ERR-2** If setlist.fm's rate limit is hit, the app waits briefly and retries on its own. If that still fails, the user sees ERR-1's message. The words "rate limit" are never shown to the user.
- **ERR-5** No error message shows technical details (status codes, stack traces, raw responses).

The screens and messages are built in step 12. This step returns the data and outcomes they need. Validate SRCH-1 server-side too: an empty query → `400 invalid_request`.

## Decisions this step relies on

- **ADR-005: setlist.fm calls.**
  - Calls are **sequential**, through a single **in-process rate limiter** set just below our key's rate. That works because Cloud Run is capped at one instance (ADR-003).
  - A rate-limit response is retried quietly after a short wait, a **bounded** number of times, then surfaces as ERR-1.
  - **Show list:** fetch pages of recent setlists, drop those with zero entries, and stop when there are 20 shows or after **3 pages**, whichever comes first.
  - **No caching** between requests.
- **setlist.fm API:**
  - free for non-commercial use;
  - the key goes in the **`x-api-key`** header, with `Accept: application/json`;
  - setlists come **20 per page**, newest first.
  - **Confirm our key's actual rate limit** in this step (from the key's settings on setlist.fm, or the API docs) and record it in the handoff.
- **ADR-001:** standard library first. Accent-insensitive comparison needs Unicode decomposition, which isn't in the standard library, so **`golang.org/x/text`** (part of the official Go project) is acceptable here. Justify it in the PR.

## What to build

1. **`internal/normalize`** (or similar):
   - `Name(s string) string`: lower-case, remove accents (NFD decomposition, then drop combining marks), trim surrounding whitespace;
   - also collapse internal runs of whitespace (note this in the handoff).

   Step 07 reuses and extends this for song titles, so keep it small and well tested.
2. **`internal/setlistfm` client:**
   - base URL `https://api.setlist.fm/rest/1.0`, configurable for tests;
   - an `http.Client` with timeouts;
   - the rate limiter (stdlib only, e.g. a ticker or token bucket);
   - retry on `429` (bounded, with a short backoff);
   - errors mapped to a typed "service unavailable" error.
   - **setlist.fm answers `404` when a search has no results.** Treat that as an empty result, not an error.
   - Methods:
     - `SearchArtists(ctx, name)`: `GET /search/artists?artistName=…&sort=relevance&p=1`, returning MBID, name and disambiguation;
     - `ArtistSetlists(ctx, mbid, page)`: `GET /artist/{mbid}/setlists?p=…`;
     - `Setlist(ctx, setlistID)`: `GET /setlist/{setlistId}`.

     Typed structs cover the fields we use: `id`, `eventDate` (`dd-MM-yyyy`), `artist.name`, `venue.name`, `venue.city.name`, `tour.name`, `url`, and `sets.set[]` with `name`, `encore` and `song[]` (`name`, `tape`, `cover.name`, `info`).
3. **Endpoints.** Both are protected (session required). Document them in `docs/api.md`.
   - `GET /api/artists?q=…`:
     - returns `{"match": "clear" | "picker" | "none", "artists": [{"id", "name", "disambiguation"}]}`;
     - `clear` returns exactly the one matching artist (SRCH-3);
     - `picker` returns up to 10 in setlist.fm's order (SRCH-4);
     - `none` returns an empty list (SRCH-5).
   - `GET /api/artists/{id}/shows`:
     - returns `{"artist": {"id", "name"}, "shows": [{"id", "date", "city", "venue", "tour", "songCount"}]}`, newest first;
     - `date` is already formatted as `DD Mon YYYY` (e.g. `14 Mar 2026`), with English month abbreviations;
     - `tour` is omitted or null when absent;
     - `songCount` counts all entries across all sets, including tapes (SHOW-3);
     - zero-entry shows are excluded, with the 20-show / 3-page rule;
     - an empty `shows` list means SHOW-5.
   - setlist.fm failures → `503` with `code: "upstream_unavailable"`, `service: "setlist.fm"` and the message "Couldn't reach setlist.fm. Try again." (ERR-1, ERR-5).
4. **Logging.** Never log search text, artist names chosen by users, or the API key.

## Tests

- `normalize.Name`: case, accents (`Motörhead`, `Blue Öyster Cult`), surrounding spaces; `The Sword` ≠ `sword`.
- Clear-match classification:
  - exactly one exact match → `clear`;
  - two exact matches → `picker`;
  - no exact match but results → `picker`;
  - more than 10 results → only 10 returned;
  - 404 or empty → `none`.
- Show list (fake setlist.fm server):
  - zero-entry shows dropped;
  - stops at 20;
  - follows up to 3 pages and no more;
  - fewer than 20 available → returns what there is;
  - date formatting;
  - song count includes tapes;
  - missing tour.
- The client: sends `x-api-key`; retries a `429` and then succeeds; gives up after the bound and maps to `upstream_unavailable`; the rate limiter spaces out calls (test with a short interval).
- Endpoints reject requests without a session (`401`). An empty `q` → `400`.

## Out of scope

- Classifying entries for the preview (step 06).
- The preview and create endpoints (steps 10, 11).
- UI (step 12).

## Owner tasks

- Make sure the real setlist.fm API key is in the local `.env` and in the `setlistfm-api-key` secret (a new version) before the post-merge check.

## Done when

- The tests pass in CI.
- After the merge deploys, a logged-in browser can call `/api/artists?q=gojira` and `/api/artists/<mbid>/shows` on Cloud Run and get sensible results.
- Our key's rate limit is confirmed, and the limiter is configured just below it.

## End of step (same PR)

- Write `handoff.md`, including:
  - client API and types;
  - limiter settings and the confirmed rate limit;
  - normalisation behaviour;
  - the endpoint shapes;
  - setlist.fm quirks found (404-on-empty, field oddities).
- Mark step 05 done in `docs/plan.md`.
- Update `README.md` as needed.
