# Step 12: Throwaway UI

- **Depends on:** step 11 (the API is feature-complete)
- **Branch:** `step-12-throwaway-ui`
- **Handoffs to read:** `docs/plan/01-go-skeleton/handoff.md`, `docs/plan/04-spotify-login/handoff.md` (the `/?login=` values), `docs/plan/11-create-playlist-api/handoff.md`
- **Also read:** `docs/api.md`. It's the UI's only source for the API. If the UI needs something the API doesn't provide, **stop and flag it**, and don't work around it in the UI.

## Goal

An **extremely basic**, functional UI that covers the whole journey on phone and laptop, built only on the JSON API. It also proves the API is complete before the real UI replaces this one after the MVP.

## Spec requirements (quoted from `docs/spec.md`)

**General**

- **GEN-1** The app is a website that works in current mobile and desktop browsers. All screens can be used on a phone-sized screen without zooming or horizontal scrolling.
- **GEN-2** The UI is extremely basic: plain, functional screens with no visual design work.
- **GEN-3** Every screen has a footer reading "Setlist data from setlist.fm", which links to setlist.fm.

**Login**

- **AUTH-1** Before login, the only things a visitor can see are the app name, a one-line description, a **"Log in with Spotify"** action and the footer. No search, show or setlist data is available without login.
- **AUTH-3** After a successful login, the user lands on the search screen.
- **AUTH-4** If Spotify refuses the user because their account isn't allowlisted, the app shows an **invite-only message**: "Setlisted is invite-only. Ask the owner to add your Spotify account." No raw Spotify error is shown.
- **AUTH-5** If the user cancels or declines on Spotify's consent screen, they're returned to the login screen with the message "Login was cancelled."
- **AUTH-7** Every logged-in screen has a **"Log out"** action. Logging out discards the session completely and returns to the login screen.
- **AUTH-8** If the session has expired or become invalid at any point, the user is sent to the login screen. After logging in again they start at the search screen. The app does not restore their previous place.

**Search and show list**

- **SRCH-1** The search screen has one text box for the band name and a Search action. An empty or whitespace-only search can't be submitted.
- **SRCH-3** If exactly one result is a clear match, the app goes straight to that band's shows and skips the picker. *(The API returns `match: "clear"`.)*
- **SRCH-4** Otherwise, if there are results, the app shows an **artist picker**: up to **10** results in setlist.fm's order, each showing the artist name and setlist.fm's disambiguation text if there is any. There is no paging. Choosing an artist goes to its shows.
- **SRCH-5** If there are no results, the app shows "No artists found for '‹search text›' on setlist.fm." The search box keeps the text so the user can edit it.
- **SHOW-1** The show list is headed with the band's name and lists up to the **20 most recent shows that have at least one entry**, newest first.
- **SHOW-3** Each show displays: **date** (`DD Mon YYYY`), **city**, **venue**, **tour name** (if setlist.fm has one) and **song count**.
- **SHOW-5** If the band has no shows with entries, the app shows "No setlists found for ‹band› on setlist.fm."
- **SHOW-6** The show list has a "New search" action that returns to the search screen. Choosing a show goes to its preview.

**Preview**

- **PREV-1** While matching runs, the screen shows a simple "Matching songs…" message. There's no per-song progress.
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

**Create and confirmation**

- **PL-6** "Create playlist" is disabled as soon as it's pressed. One press creates at most one playlist.
- **PL-7** On success, the app shows: "Playlist created: ‹playlist name›, ‹n› tracks".
- **PL-8** The confirmation has an **"Open in Spotify"** link to the new playlist. On a phone with Spotify installed, it should open the Spotify app. Otherwise it opens Spotify on the web.
- **PL-9** The confirmation has a **"Search another band"** action, which returns to an empty search screen. The confirmation doesn't repeat the list of unmatched entries.

**Errors**

- **ERR-1** If setlist.fm or Spotify can't be reached or returns an error, the app shows a plain message naming the service (e.g. "Couldn't reach setlist.fm. Try again."), with a **Retry** action that repeats the failed step.
- **ERR-3** If playlist creation fails **after** the playlist has been created (e.g. adding tracks fails), the app says creation failed and provides a link to the partially created playlist. It does not delete or roll anything back. Retrying creates a **new** playlist.
- **ERR-4** If playlist creation fails before anything is created, the app says so and offers Retry.
- **ERR-5** No error message shows technical details (status codes, stack traces, raw responses).

## Decisions this step relies on

- **ADR-002: Throwaway static UI.**
  - **Static HTML with plain JavaScript and minimal CSS**: no framework, no build step, no npm, and **no htmx**.
  - It talks to the backend **only through the JSON API** using `fetch()`. It never calls setlist.fm or Spotify and never sees a token.
  - It's served by Go from the **same origin**, so the session cookie just works.
  - It lives in its own directory (`web/`), so it can be deleted and replaced without touching backend code.
  - Styling is limited to readability and phone use: a viewport meta tag, a readable font size, full-width controls.
  - It will be replaced after the MVP, so keep it simple. Its code quality matters less than the API's.
- **ADR-004:**
  - the session is an `HttpOnly` cookie the UI can't read, so use `GET /api/session` to decide whether to show login or search;
  - log out is `POST /auth/logout`, which needs the same-origin `Origin` header that `fetch` sends automatically;
  - login is a plain link to `/auth/login`.
- **ADR-006:** **no automated UI tests**. A **manual test checklist** goes in the repo instead, covering the whole journey on the owner's phone and laptop.

## What to build

1. **`web/`:** one `index.html`, one `app.js` and one small `style.css` (or split them further if that's clearer), plus anything else that's needed. A single page that switches between views is fine.
2. **Views:**
   - login (with the `/?login=cancelled | invite_only | failed` messages);
   - search;
   - artist picker;
   - show list;
   - preview, with "Matching songs…" while it loads;
   - confirmation;
   - an error panel with **Retry**, which repeats the failed request;
   - a footer on every view;
   - Log out on every logged-in view.
3. **Behaviour:**
   - any API `401` → login view (AUTH-8);
   - `match: "clear"` goes straight to shows;
   - empty search is blocked client-side (SRCH-1);
   - "Create playlist" is disabled on press (PL-6) and when `canCreate` is false (PREV-7);
   - create posts `showId` and the preview's `trackIds`, unchanged (ADR-005);
   - `partial_create` shows the failure message with a link to the partial playlist (ERR-3);
   - error text comes from the API's `message` field (already user-safe).
4. **Navigation:**
   - Back to shows;
   - New search;
   - Search another band (an empty search box).

   Browser back-button support is nice to have but not required. If it's cheap (e.g. hash routing), add it.
5. **Phone usability (GEN-1):** no horizontal scrolling at 375 px width; tap targets are comfortable.
6. **`docs/manual-test-checklist.md`:** a step-by-step journey with expected results, covering every requirement above. Include:
   - login, cancel, invite-only (if a test account is available), log out, and session expiry (wait an hour, or delete the cookie);
   - a clear match, the picker, no results;
   - a band with no setlists;
   - a preview with covers, tapes, medleys and encores;
   - creation, and Open in Spotify on both phone and laptop;
   - an error with Retry (e.g. by blocking the network in dev tools);
   - the footer.

## Out of scope

- Visual design, branding, a UI framework (post-MVP, with a new ADR superseding ADR-002).
- Automated browser tests.
- Any backend change other than small fixes that are flagged in the PR.

## Done when

- After the merge deploys, the manual checklist passes on the owner's **laptop and phone**, in the browsers they actually use, against Cloud Run.
- No console errors during the journey.

## End of step (same PR)

- Write `handoff.md`, including:
  - the file structure;
  - how views are switched;
  - API gaps found (if any);
  - the checklist results;
  - known UI rough edges, kept for the post-MVP redesign.
- Mark step 12 done in `docs/plan.md`.
- Update `README.md`: what the app does, with a screenshot if wanted, and the status "MVP feature-complete, in acceptance".
