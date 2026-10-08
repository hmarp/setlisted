# Step 10: Preview API

- **Depends on:** step 05 (setlist.fm client, `Setlist` by ID), step 08 (matching pipeline)
- **Branch:** `step-10-preview-api`
- **Handoffs to read:** `docs/plan/04-spotify-login/handoff.md`, `docs/plan/05-setlistfm-search-shows/handoff.md`, `docs/plan/06-entry-classification/handoff.md`, `docs/plan/08-spotify-search-pipeline/handoff.md`

## Goal

A protected endpoint that, given a setlist ID, returns everything the preview screen needs: the show header, summary counts, entries grouped under set headings with their notes, statuses and labels, the matched track details, and the **track IDs that create (step 11) will send back**.

## Spec requirements (quoted from `docs/spec.md` §6)

- **PREV-1** While matching runs, the screen shows a simple "Matching songs…" message. There's no per-song progress. *(UI, step 12. This endpoint just returns when matching is done.)*
- **PREV-2** The top of the preview shows the band, show date, venue, city and tour name (if any). It also has a link to the show's page on setlist.fm, and a summary: "‹found› of ‹matchable› songs found".
- **PREV-3** Entries are listed in setlist order. Each set after the main set is introduced by a heading using setlist.fm's set name (e.g. "Encore 1"). Skipped entries are not shown.
- **PREV-4** Each row shows: position number, the entry name as on setlist.fm, any setlist.fm note (as secondary text), and its status:
  - **Found**: plus the matched Spotify track's title, artist(s) and album.
  - **Found, cover fallback**: labelled "Cover → ‹original artist›", plus the track details.
  - **Found, live version**: labelled "Live version", plus the track details.
  - **Not found** or **Not found (medley)**.
  - Tape entries additionally carry a **"Tape"** label, whatever their status.
- **PREV-5** The preview is **read-only**. Tracks can't be removed, reordered, swapped or renamed, and there are no audio previews.
- **PREV-6** The preview has a **"Back to shows"** action, which returns to the band's show list, and a **"Create playlist"** action.
- **PREV-7** If no entries were found, "Create playlist" is **disabled** and the preview says "None of these songs were found on Spotify." An empty playlist is never created.
- **ERR-1** If setlist.fm or Spotify can't be reached or returns an error, the app shows a plain message naming the service (e.g. "Couldn't reach setlist.fm. Try again."), with a **Retry** action that repeats the failed step.
- **ERR-2** If setlist.fm's rate limit is hit, the app waits briefly and retries on its own. If that still fails, the user sees ERR-1's message. The words "rate limit" are never shown to the user.
- **ERR-5** No error message shows technical details (status codes, stack traces, raw responses).
- **AUTH-8** If the session has expired or become invalid at any point, the user is sent to the login screen.
- **Found** means any of the three "Found" statuses. **Matchable** means entries that aren't skipped and aren't medleys.

## Decisions this step relies on

- **ADR-005:**
  - matching runs **on the server, when the preview is requested**, in one call: fetch the setlist, classify, search, match, return rows;
  - **create doesn't re-run matching.** The UI sends the show ID and the ordered list of matched track IDs from the preview;
  - the track list isn't signed or verified, because tampering would only affect the user's own playlist;
  - no caching.
- **ADR-001 / ADR-002:** this response is a lasting API contract for the throwaway UI and the future UI. Document it fully in `docs/api.md`.
- **ADR-004:** a protected route; Spotify 401 → step 04's helper.
- **Market:** use `from_token` for real user requests (step 08).

## What to build

1. **`GET /api/shows/{id}/preview`** (protected). It:
   - fetches the setlist (step 05 client), converts and classifies it (step 06), and runs `MatchShow` with the user's token and `market=from_token` (step 08);
   - returns, for example:
     ```json
     {
       "show": { "id": "…", "band": "Gojira", "date": "14 Mar 2026", "venue": "…", "city": "Leipzig", "tour": "Mea Culpa Tour", "setlistUrl": "https://www.setlist.fm/…" },
       "summary": { "found": 18, "matchable": 20 },
       "canCreate": true,
       "sets": [
         { "heading": "", "entries": [
           { "position": 1, "name": "…", "note": "…", "tape": true,
             "status": "found | cover_fallback | live | not_found | not_found_medley",
             "originalArtist": "… (covers only)",
             "track": { "id": "…", "title": "…", "artists": ["…"], "album": "…" } }
         ]},
         { "heading": "Encore 1", "entries": [ … ] }
       ],
       "trackIds": ["…", "…"]
     }
     ```
   - `trackIds` is the ordered list of matched track IDs, with duplicates kept. It's exactly what create (step 11) expects.
   - `canCreate` is `false` when `summary.found == 0` (PREV-7).
   - Represent a match that is both a cover fallback **and** live the way step 08's handoff describes (e.g. `status: "cover_fallback"` plus `"live": true`), and document it.
   - **Errors:**
     - setlist.fm unknown ID → `404 not_found`;
     - setlist.fm unavailable → `503 upstream_unavailable` with `service: "setlist.fm"`;
     - Spotify unavailable → `503 upstream_unavailable` with `service: "Spotify"` and the message "Couldn't reach Spotify. Try again.";
     - Spotify 401 → `401 not_logged_in`, with the cookie cleared.
2. **`docs/api.md`:** document the endpoint, every field, the status values and labels mapping (PREV-4), and the errors.

## Tests (`httptest`, with fake setlist.fm and Spotify, or fake clients)

- A show with a main set and an encore → headings correct, positions correct, skipped entries absent.
- The summary counts match the definition. `canCreate` is false when nothing is found.
- `trackIds` are in setlist order with duplicates, and exclude not-found and medley entries.
- Each status is rendered: found, cover fallback (with `originalArtist`), live, not found, medley, tape flag on a found entry and on a not-found one.
- Error mapping for setlist.fm 404 and unavailable, Spotify unavailable, Spotify 401, and no session (`401`).

## Out of scope

- Playlist creation (step 11).
- UI rendering (step 12).

## Done when

- The tests pass in CI, and `docs/api.md` documents the endpoint fully.
- After the merge deploys, a logged-in browser calling `/api/shows/<real id>/preview` on Cloud Run gets a sensible result within a few seconds.

## End of step (same PR)

- Write `handoff.md`, including:
  - the final response shape;
  - status and label mapping;
  - how `trackIds` is built;
  - typical response time observed.
- Mark step 10 done in `docs/plan.md`.
- Update `README.md` if needed.
