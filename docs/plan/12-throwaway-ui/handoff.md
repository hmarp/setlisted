# Step 12 handoff: Throwaway UI

## What was built

- The MVP's screens as static HTML, plain JavaScript and a little CSS in `web/static/` ([ADR-002](../../adr/002-throwaway-static-ui.md)). There's no framework, build step or npm. It's embedded in the binary and served at `/` as before (step 01).
- It covers login (with the cancel, invite-only and failed messages), search, the artist picker, the show list, the preview with "Matching songs…", creation and confirmation, errors with Retry, Log out and the setlist.fm footer.
- It's built only on [`docs/api.md`](../../api.md). **No API gaps were found**, and the backend is unchanged.
- [`docs/manual-test-checklist.md`](../../manual-test-checklist.md): the whole journey for phone and laptop, with the requirement IDs.

## File structure

```
web/static/index.html   the one page: header (name, Log out), one <section> per view, the shared
                        loading line and error panel, the footer
web/static/app.js       everything else: API calls, view switching, routing, rendering
web/static/style.css    readability and phone use only
```

`app.js` is in sections: `api()` and helpers, view switching, login and logout, routing, search and picker, show list, preview, create and confirmation, start-up.

## How views are switched

- **One section per view:** `view-login`, `view-search`, `view-shows`, `view-preview` and `view-done`. `show(view)` unhides one, hides the others, clears the error panel and the loading line, and shows **Log out** on every view except login.
- **Hash routing**, so the browser's Back and Forward work:

  | Hash | View |
  |---|---|
  | none, or `#/` | search, empty |
  | `#/search/<text>` | search results: the picker or "No artists found…" (a clear match replaces this entry with the artist's, so Back doesn't bounce) |
  | `#/artist/<mbid>` | the show list |
  | `#/artist/<mbid>/show/<setlist id>` | the preview |

  The artist's ID is in the preview's hash because the preview response has no artist ID, and **Back to shows** needs it. The confirmation has no hash: it only exists right after a create.
- **Start-up:** `GET /api/session`. Logged out → login view, with the `?login=` message if there is one. The parameter is always removed with `history.replaceState`. Logged in → the hash's view. After a login the callback redirects to `/`, so that's the search view (AUTH-3, AUTH-8).
- **Any `401`** (`api()` handles it centrally): forget the in-memory state, clear the hash, abort the request in flight and show the login view (AUTH-8). The login view then shows the API's message, "You're not logged in. Log in with Spotify." (see Decisions).
- **Stale answers:** every navigation increments `generation` and aborts the previous `fetch` (`AbortController`). An answer for an older generation is ignored. Leaving a preview while it's matching therefore cancels it (the server logs `499`).
- **In-memory caches:** the last successful search and the last show list, so Back from a show list or a preview doesn't call setlist.fm again. Only successes are cached; Retry always fetches. Nothing is written to browser storage (GEN-5).

## Behaviour

- **Search (SRCH-1):** **Search** is disabled while the box is empty or only spaces, and submitting is blocked as well. `maxlength="200"` matches the API's limit.
- **Create (PL-6):** the button is disabled as soon as it's pressed (a double press sent one `POST` in the check), and "Creating playlist…" shows. It's re-enabled only after a failure. The `POST` body is `{"showId": show.id, "trackIds": trackIds}` from the preview, unchanged. It's not aborted on navigation (the server finishes once Spotify is called), but its answer is ignored if the user has gone elsewhere.
- **Error panel:** the API's `message`, plus:
  - **Retry** for `503`, `500`, a `502` other than `partial_create`, and no answer at all. It repeats exactly the failed request (search, show list, preview, create with the same body, logout or session check).
  - No Retry for `400` and `404`, where repeating can't help.
  - **Back to shows** on preview and create errors, so there's always a way out.
  - **`partial_create` (ERR-3):** the message, "Open the playlist in Spotify" linking to `playlist.url`, and "Try again (creates a new playlist)".
- **Messages the UI words itself** (there's no API message for them):

  | When | Message |
  |---|---|
  | `?login=failed` (or an unknown value) | "Login didn't work. Try again." |
  | `fetch` fails (offline, server unreachable) | "Couldn't reach Setlisted. Check your connection and try again." |
  | an error answer that isn't the API's JSON (e.g. a proxy page) | "Something went wrong. Try again." (the same as `internal_error`) |

- **Preview labels** follow `docs/api.md`'s table: "Found", "Live version" (`status: live`), "Cover → ‹originalArtist›" only for `cover_fallback` (plus "Live version" when `live`), "Not found", "Not found (medley)", and "Tape" when `tape`. Track details are "Title — Artist, Artist · Album". The numbers are `entries[].position`, set as `<li value>`.
- **Open in Spotify** is a plain link to `playlist.url` in the same tab, as the brief says, so that phones can hand it to the Spotify app (PL-8).

## Decisions taken in this step

- **The login view shows the 401 message** ("You're not logged in. Log in with Spotify.") after a session ends mid-use, so the user knows why they're there. AUTH-1 lists what a logged-out visitor sees. This is a message like AUTH-4/5's and shows no data. Easy to drop if unwanted.
- **Hash routing puts the search text and the artist and show IDs in the address bar**, so they're in the browser's own history on that device. They're never sent to the server (fragments aren't part of requests), and the app stores nothing (GEN-5). If that's not acceptable, routing can be dropped: Back support is only nice-to-have.
- **No answer from the server** gets its own message (above), not one naming setlist.fm or Spotify, because the app can't tell which service would have failed.
- **The error panel sits at the top of the page.** On the preview, the Create button is near the top, so a create error appears right above it.

## Verification in this session

- `gofmt -l .` prints nothing. `go vet ./...`, `go vet -tags live ./...`, `go vet -tags spotifylive ./...` and `go test -race ./...` pass, including step 09's `TestMatchingQuality`.
- **Headless Chromium against the real router** (`httpapi.NewHandler` with the embedded files, real `auth`, `setlistfm` and `spotify` clients) pointed at **fake setlist.fm, Spotify API and Spotify accounts servers**. The harness and the Playwright script weren't committed: the step forbids automated UI tests and backend additions. The login went through the real `/auth/login` → fake authorise page → real `/auth/callback`. At **375 × 740, mobile, touch**, 120 checks passed:
  - every view had the footer and `scrollWidth` ≤ 375, including a show with a 58-letter town name and an 80-character venue;
  - login: logged-out view only, cancel, invite-only and failed messages, `?login=` removed, a deep link without a session → login;
  - search: empty/whitespace blocked (button and Enter), no results with the text kept, a picker of 3 with disambiguation, Back → picker again, a clear match straight to shows;
  - shows: heading, a show with no songs left out, date, city, venue, tour and count, no setlists (SHOW-5), New search;
  - preview: "Matching songs…" alone while loading, the header, the setlist.fm link, "7 of 9 songs found", only the "Encore 2" heading, positions 1–10 with skipped entries absent, and every label combination (found + tape, note, live, medley, not found, cover + tape, cover + live, a cover found under the band as plain Found, tape + not found);
  - nothing found: the message and Create disabled; Back to shows;
  - create: disabled on press, one `POST` for a double press, a `503` with Retry and Create re-enabled, Retry → "Playlist created: Gojira – Mea Culpa Tour (Setlist), 7 tracks", the Open in Spotify link, no unmatched list, Search another band → empty box;
  - partial create: the message, the link and the "Try again (creates a new playlist)" wording;
  - setlist.fm `503` on search, show list and preview, and Spotify `503` on the preview, each with Retry working once the fake recovered;
  - offline → the connection message, and Retry once back online;
  - Spotify rejecting the token mid-preview → login with the hash cleared, then login again → search; a deleted cookie → login on the next action; Log out → login, with `/api/session` false afterwards.
  - **No JavaScript errors.** The only console lines were Chromium's own "Failed to load resource" for the failures caused on purpose.
- Also at 1280 px wide (the preview, checked by eye). `app.js` and `style.css` are served as `text/javascript` and `text/css`.

## Checklist results (owner, after the merge deploys)

To fill in from [`docs/manual-test-checklist.md`](../../manual-test-checklist.md):

- [ ] Local on `http://127.0.0.1:8080` (optional, before merging): browser ______, result ______
- [ ] Cloud Run, **laptop**: browser ______, date ______, result ______
- [ ] Cloud Run, **phone**: device and browser ______, date ______, result ______
- [ ] Open in Spotify on the phone opens the Spotify app (PL-8): ______
- [ ] No console errors during the journey (laptop): ______

Step 13 runs the checklist again for acceptance.

## Notes for step 13

- Every run of sections 6 and 8 of the checklist creates real playlists. Delete them in Spotify afterwards.
- The ERR-1 service messages and ERR-3 can't be caused on purpose against the live services. They were checked against fakes here.
- Invite-only needs a second Spotify account that isn't in User Management. The owner's own account is always let through (step 04's handoff).

## Known rough edges (for the post-MVP redesign)

- The confirmation isn't in the hash. Back from it goes to the show list (the preview is a history entry, so matching runs again if you go Forward). Reloading the confirmation re-runs the preview.
- A clear-match search isn't in history, so Back from its show list goes to the screen before the search, and the box is empty there.
- Labels in the preview are plain bordered text. Status is shown by words only, with no colour or icons.
- "Not found." is the API's message for a `404` (a show or artist that's gone from setlist.fm). It's accurate but terse.
- No "Matching songs…" progress or time estimate (none is wanted: PREV-1).
- Focus isn't moved on view changes, apart from the empty search box. A screen reader user would need to find the new content.
- Search text in the URL is percent-encoded, so non-ASCII band names look odd in the address bar.
