# Step 11: Create playlist API

- **Depends on:** step 10
- **Branch:** `step-11-create-playlist-api`
- **Handoffs to read:** `docs/plan/04-spotify-login/handoff.md`, `docs/plan/05-setlistfm-search-shows/handoff.md`, `docs/plan/08-spotify-search-pipeline/handoff.md`, `docs/plan/10-preview-api/handoff.md`

## Goal

A protected endpoint that creates a **new private playlist** in the user's Spotify library from a show ID and the ordered track IDs returned by the preview. It names and describes the playlist as the spec says, and returns what the confirmation screen needs.

## Spec requirements (quoted from `docs/spec.md` §7 and §8)

- **PL-1** "Create playlist" creates a **new** playlist in the logged-in user's Spotify library every time. It never updates or reuses an existing playlist.
- **PL-2** **Name:**
  - With a tour name: `‹Band› – ‹Tour Name› (Setlist)`, e.g. `Gojira – Mea Culpa Tour (Setlist)`.
  - Without a tour name: `‹Band› – ‹City›, ‹DD Mon YYYY›`, e.g. `Gojira – Leipzig, 14 Mar 2026`.
  - The name can't be edited in the app.
- **PL-3** **Description:** `‹Venue›, ‹City›, ‹DD Mon YYYY›. Setlist: ‹setlist.fm URL of the show›. Created by Setlisted.`
- **PL-4** The playlist is **private** and keeps Spotify's default cover.
- **PL-5** **Contents:** every found entry (§6, any "Found" status), in setlist order, with duplicates kept. Not-found entries and medleys are left out. Set and encore boundaries are not reflected.
- **PL-6** "Create playlist" is disabled as soon as it's pressed. One press creates at most one playlist. *(UI, step 12.)*
- **PL-7** On success, the app shows: "Playlist created: ‹playlist name›, ‹n› tracks".
- **PL-8** The confirmation has an **"Open in Spotify"** link to the new playlist. On a phone with Spotify installed, it should open the Spotify app. Otherwise it opens Spotify on the web.
- **PL-9** The confirmation has a **"Search another band"** action, which returns to an empty search screen. The confirmation doesn't repeat the list of unmatched entries.
- **PREV-7** If no entries were found, "Create playlist" is **disabled** … An empty playlist is never created.
- **ERR-1** If setlist.fm or Spotify can't be reached or returns an error, the app shows a plain message naming the service (e.g. "Couldn't reach setlist.fm. Try again."), with a **Retry** action that repeats the failed step.
- **ERR-3** If playlist creation fails **after** the playlist has been created (e.g. adding tracks fails), the app says creation failed and provides a link to the partially created playlist. It does not delete or roll anything back. Retrying creates a **new** playlist (PL-1).
- **ERR-4** If playlist creation fails before anything is created, the app says so and offers Retry.
- **ERR-5** No error message shows technical details (status codes, stack traces, raw responses).

## Decisions this step relies on

- **ADR-005:**
  - **create doesn't re-run matching**. The request carries the show ID and the ordered track IDs from the preview;
  - the server fetches the show again (one setlist.fm call) to build the name and description;
  - it creates the private playlist and adds the tracks **in batches within Spotify's per-request limit**;
  - the track list is **not verified**: tampering only affects the user's own playlist.
- **Spotify (development mode since February 2026):**
  - create with **`POST /me/playlists`** (body: `name`, `description`, `public: false`);
  - add tracks with **`POST /playlists/{id}/items`**, with a `uris` array of `spotify:track:<id>`. Check the current per-request maximum (historically 100) in Spotify's docs and record it;
  - the `tracks` field and parameters were renamed to `items`.
- **ADR-004:**
  - POST is subject to the **`Origin` check**;
  - the scope is `playlist-modify-private`;
  - Spotify 401 → step 04's helper.

## What to build

1. **`POST /api/playlists`** (protected, `Origin`-checked). Body:

   `{"showId": "…", "trackIds": ["…", …]}`

   - **Validate:** `showId` is non-empty; `trackIds` is non-empty (PREV-7 server-side); every ID looks like a Spotify ID (base62, 22 characters); and set a sane maximum count (e.g. 200). Failures → `400 invalid_request`.
   - **Fetch the setlist** (step 05) and build the name (PL-2) and description (PL-3) with `DD Mon YYYY` formatting. The dash in the name is an en dash `–`.
   - **Check Spotify's documented length limits** for name and description, and truncate safely if needed. Record what you found.
   - **Create the playlist** (`public: false`).
   - **Add the tracks** in order, in batches. Duplicates are sent as they are (PL-5).
   - **Response `201`:**
     ```json
     { "playlist": { "id": "…", "name": "…", "trackCount": 18, "url": "https://open.spotify.com/playlist/…" } }
     ```
     `url` is Spotify's `external_urls.spotify` value. On phones with the app installed, this link opens the Spotify app (PL-8).
   - **Errors:**
     - setlist.fm unavailable or unknown show, or Spotify failing **before** the playlist exists → `503 upstream_unavailable` (or `404` for an unknown show), with nothing created (ERR-4);
     - adding tracks fails **after** creation → `502` with `code: "partial_create"`, a user-facing message, and `playlist.url` so the UI can link to it (ERR-3). **No rollback, no deletion**;
     - Spotify 401 → `401 not_logged_in`.
2. **`docs/api.md`:** document the request, response, validation and errors, including `partial_create`.

## Tests (`httptest`, with fake Spotify and setlist.fm)

- **Naming:** with a tour → `Band – Tour (Setlist)`; without → `Band – City, DD Mon YYYY`. The description format is exact.
- **Spotify request:** the created playlist has `public: false`; the right name and description are sent to `POST /me/playlists`.
- **Batching:** e.g. 150 tracks → two add requests, in order; duplicates are kept.
- **Validation:** empty `trackIds`, a malformed ID, a missing `showId` → `400`. A missing or foreign `Origin` → `403`. No session → `401`.
- **Failures:**
  - create fails → `503`, no add calls;
  - the second batch fails → `502 partial_create` with the URL;
  - Spotify 401 → `401` with the cookie cleared.

## Out of scope

- UI (step 12).
- Updating existing playlists, rollback or cleanup (excluded by the spec).

## Done when

- The tests pass in CI, and `docs/api.md` is updated.
- After the merge deploys, a logged-in owner can call preview and then create on Cloud Run, and a correctly named **private** playlist appears in their Spotify with the tracks in setlist order.

## End of step (same PR)

- Write `handoff.md`, including:
  - the endpoint shape;
  - the batch size and length limits found;
  - error codes the UI must handle (`partial_create` especially);
  - anything odd in Spotify's responses.
- Mark step 11 done in `docs/plan.md`.
- Update `README.md` (status: the API is feature-complete).
