# Step 10 handoff: Preview API

## What was built

- **`GET /api/shows/{id}/preview`** (protected), in `internal/httpapi/preview.go`: fetches the setlist (step 05), converts and classifies it (`Setlist.Show()`, step 06), runs `pipeline.MatchShow` with the session's token and `market=from_token` (step 08), and returns the preview. Fully documented in `docs/api.md`.
- **`httpapi.Deps`**: `NewHandler(d Deps)` and `NewServer(addr, d Deps)` now take one dependencies struct (`Logger`, `Static`, `Auth`, `SetlistFM`, `Spotify`), as steps 01 and 05 suggested. `main` builds **one** `spotify.New(spotify.Config{})` and passes it in. Existing tests use a `testDeps(...)` helper.
- **ADR-010**: login now also requests **`user-read-private`**, which `market=from_token` needs (found in the live check, below). `auth.Scope` became `auth.Scopes`; ADR-004's status line points to ADR-010.
- No new dependencies, configuration, error codes or cloud resources.

## Response shape

```json
{
  "show": { "id", "band", "date", "venue", "city", "tour": "…" | null, "setlistUrl" },
  "summary": { "found": 7, "matchable": 9 },
  "canCreate": true,
  "sets": [ { "heading": "" | "Encore 1" | …, "entries": [
    { "position": 1, "name": "…", "note": "…" | null, "tape": false,
      "status": "found" | "cover_fallback" | "live" | "not_found" | "not_found_medley",
      "live": false, "originalArtist": "…" | null,
      "track": { "id", "title", "artists": ["…"], "album" } | null } ] } ],
  "trackIds": ["…"]
}
```

- Every field is always present. **Absent text is `null`** (`tour`, `note`, `originalArtist`), matching the show list's `tour`; a blank or whitespace-only value counts as absent. `track` is `null` exactly when the status isn't a Found one.
- `summary.matchable` = `show.Matchable()`; `summary.found` = rows with a track; `canCreate` = `found > 0` (PREV-7).
- `sets`, `entries` and `trackIds` are `[]`, never `null`.

## Status and label mapping (PREV-4)

| `status` | `live` | Labels |
|---|---|---|
| `found` | false | — (Found) |
| `live` | true | "Live version" |
| `cover_fallback` | false | "Cover → ‹originalArtist›" |
| `cover_fallback` | true | "Cover → ‹originalArtist›" + "Live version" |
| `not_found` | false | Not found |
| `not_found_medley` | false | Not found (medley) |

Plus "Tape" whenever `tape` is true. `originalArtist` is set for **every** cover (including one found under the band, which is a plain `found`); the UI shows "Cover → …" only for `cover_fallback`.

## Decisions taken in this step

- **Sets are cut along `show.Sets`, not by grouping equal `SetHeading`s.** Each set takes as many result rows as it has non-skipped entries; a set with none is dropped (step 06's recommendation, so no heading appears with nothing under it). This gives the same result as grouping consecutive headings, except that two adjacent sets with the same name (rare, but setlist.fm allows it) stay separate instead of being merged. No pipeline change was needed.
- **The setlist ID is checked before calling setlist.fm**: letters and digits, 1–32 characters (real IDs are 7–8 hex digits, e.g. `63de4613`). Anything else is `404 not_found` with no upstream call. `url.PathEscape` is still applied by the client.
- **`null` for absent text** (see above), rather than `""`, to match the show list.
- **One budget for the whole request: 50 s** (`previewBudget`), covering the setlist.fm fetch and all searches, leaving 10 s of the 60 s write timeout. A single budget rather than separate ones, because the setlist.fm part is one call and normally takes well under a second; if it's slow, Spotify gets what's left, and an overrun is a 503 either way.
- **A client that goes away** (tab closed, Retry clicked, navigation) is recorded as status **`499`** in the request log, with an INFO line `preview abandoned by the client` and **no** WARN. Nobody receives the 499. The check is `r.Context().Err() != nil`, so the budget running out (the request's own deadline, not the client's) is still a logged 503.
- **Spotify's reason is now logged (changes a step 08 rule).** The first live run got `spotify unavailable: status 403` with no way to tell why, so `internal/spotify` now adds Spotify's error `message` to the error text, e.g. `status 403, Spotify says "…"`. It's kept only if it's ≤ 120 characters of printable text and contains neither the track or artist name searched nor the token; otherwise it's left out as before. Users still see only "Couldn't reach Spotify. Try again." (ERR-5). Tested in `TestSearchTracks_SpotifyReasonLogged`; step 08's `…_ERR5_NoDetailsInErrors` (a non-JSON body) still passes. Step 11's playlist calls go through the same `do`, so they get it too.
- **Spotify 401** logs an INFO line (`Spotify rejected the session; logging out`) and calls `auth.SpotifyUnauthorized(w)`.

## Error mapping

| Situation | Response | Log |
|---|---|---|
| `{id}` malformed | `404 not_found`, no upstream call | request line only |
| setlist.fm `404` (`ErrNotFound`) | `404 not_found`, no Spotify call | request line only |
| setlist.fm anything else (`ErrUnavailable`, budget) | `503 upstream_unavailable`, `service: "setlist.fm"` | WARN `setlist.fm call failed`, `op: preview`, `err` |
| Spotify `ErrUnauthorized` (wins over other errors, step 08) | `401 not_logged_in`, session cookie cleared | INFO |
| Spotify anything else, or the budget used up during matching | `503 upstream_unavailable`, `service: "Spotify"`, "Couldn't reach Spotify. Try again." | WARN `Spotify call failed`, `op: preview`, `err` (budget overruns say so) |
| No session | `401 not_logged_in` (step 04's middleware) | request line only |
| Client gone during either call | `499` (logged only) | INFO `preview abandoned by the client` |

No partial preview is ever returned. Logs never contain names, notes, track details, search text or the token (tested); the request line has the path, so the setlist ID is logged (a public ID, like step 05's MBID).

## How `trackIds` is built

While building the rows, in setlist order: every row whose `track` is non-null appends `track.id`. So it's exactly the found entries (any of the three Found statuses), duplicates kept (ENT-7), not-found and medley entries left out (PL-5). The UI sends it back unchanged.

## Tests (`internal/httpapi/preview_test.go`)

Through the real router, with a fake setlist.fm (`httptest`) and the real `spotify.Client` pointed at a fake Spotify (`httptest`, results keyed on the `q` parameter):

- `TestPreview`: show header (all six fields + URL); summary 7 of 9; `canCreate`; `trackIds` in order with a duplicate; headings `""` and `Encore 2` (the all-skipped set between them dropped); positions 1–10 across sets; skipped entries absent; every search used `Bearer <session token>` and `market=from_token`.
- `TestPreview_PREV4_Statuses`: the exact JSON of all ten rows: found + tape, found + note, live, medley, not found, duplicate, cover fallback + tape, cover fallback + live, cover found under the band (`found` with `originalArtist`), not found + tape.
- `TestPreview_PREV7_NothingFound` (`canCreate` false, `trackIds` `[]`, `tour` null) and `TestPreview_NoSets` (`sets` `[]`, no Spotify calls).
- `TestPreviewErrors`: unknown show (no Spotify call), malformed ID (no setlist.fm call), setlist.fm 502, Spotify 500, Spotify 401 (cookie cleared), each with the exact error body.
- `TestPreviewNoSession`, `TestPreviewBudget` (slow Spotify → 503 within the budget, WARN logged), `TestPreviewClientGone` (during setlist.fm and during Spotify → 499, no WARN), `TestPreviewLogsNoUserData` (success, 503 and 401).

`gofmt -l .` (nothing), `go vet ./...`, `go vet -tags live ./...`, `go vet -tags spotifylive ./...` and `go test -race ./...` pass, including step 09's `TestMatchingQuality`.

## Live check (owner)

The build sandbox can't reach setlist.fm or Spotify, so this is the **first use of a user token with `market=from_token`**. Fill in:

- [x] Local, on `http://127.0.0.1:8080`, logged in through the browser: a real show's preview returns `200` with sensible matches.
  - 2026-11-04, first attempt (show `3b4890b8`): `503` "Couldn't reach Spotify" after 475 ms; the log had `spotify unavailable: status 403`. The owner's account **is** in User Management, and an app-token search (`market=GB`) answered `200` the same day, so Spotify refuses the **user** token's search. With the reason logged, Spotify said **"Insufficient client scope"**. Two temporary local experiments: the same login with `market=GB` → `200`; `from_token` with `user-read-private` added to the login → `200`. So `from_token` needs `user-read-private` (Spotify reads the user's country). The owner chose to request it (**ADR-010**) rather than fix the market in configuration.
  - [x] Re-check on this branch after logging in again with `user-read-private` (2026-11-04): **`200`**. Show `3b4890b8` (Gojira, Life Stage, Louisville, 20 Sep 2026, no tour): **11 of 11 found**, all `found`, all studio recordings credited to Gojira on the right albums; main set 1–10 and `Encore 1` with 11; the note "With Remembrance outro" carried through; `trackIds` has the 11 IDs in order. The show had no cover fallback, live match, tape, medley or skipped entry, so those are covered by the handler tests only.
  - setlist.fm quirk seen: "Mea culpa (Ah! Ça ira!)" has the cover artist **`[traditional]`**, so `originalArtist` is `"[traditional]"`. It was found under the band (plain `found`), so no "Cover →" label applies. Had the band pass failed, the fallback would have searched Spotify for the artist `[traditional]`, which finds nothing (a wasted search, not a wrong match). Worth treating setlist.fm's bracketed placeholders (`[traditional]`, `[unknown]`) as "not a cover" in step 06's rules later; not changed here.
- [ ] After the merge deploys: the same on Cloud Run.
- [x] Typical response time (the request log's `duration_ms`), local on `127.0.0.1`: **about 600–800 ms** across repeated loads of the 11-song show `3b4890b8` (one setlist.fm call and 11 searches, 4 at a time). Well inside the 50 s budget and spec §9's one-minute journey.
- [ ] Cloud Run, warm, after the merge deploys: ___ ms.

## Notes for later steps

- **Step 11:**
  - `showId` is `show.id`; validate it with the same `setlistIDPattern` (`internal/httpapi/preview.go`) so both endpoints agree on what a show ID is, and map `setlistfm.ErrNotFound` to `404` the same way.
  - `trackIds` are Spotify track IDs (`track.id`, base62), in order, duplicates kept, already excluding not-found and medleys. The server only re-fetches the show for the name and description (`show := sl.Show()` gives `Band`, `Tour`, `City`, `Date` already as `DD Mon YYYY`, `Venue` and `URL`).
  - The Spotify client is `d.Spotify` in `httpapi.Deps`; add the playlist calls to `internal/spotify` (see step 08's note) and the route on the `api` mux.
  - Reuse the preview's pattern: one budget under the 60 s write timeout for the whole request, and `previewHandler.fail`'s client-gone check (`r.Context().Err() != nil` → 499, no WARN). For create, think about a client that goes away **after** the playlist exists: the playlist stays (no rollback, ERR-3), and nobody sees the response.
- **Step 12:** render from the tables above: no heading for `heading == ""`; `note` as secondary text when not `null`; "Cover → ‹originalArtist›" only when `status == "cover_fallback"`; "Live version" from `live`; "Tape" from `tape`; the summary from `summary`; disable "Create playlist" and show PREV-7's message when `canCreate` is false; `show.setlistUrl` for the setlist.fm link; post `show.id` and `trackIds` unchanged. A `401` means log in again; a `503` means show `message` with Retry (repeat the same GET).

## Known gaps

- **Sessions from before ADR-010** hold a token without `user-read-private`, so their previews answer "Couldn't reach Spotify" (logged as `status 403, Spotify says "Insufficient client scope"`) until the user logs in again. Sessions last at most an hour, so this clears itself within an hour of the deploy. Not worth special handling for a few friends.

- **Not run against live setlist.fm or Spotify in this session** (no network from the sandbox). The owner's local and Cloud Run checks above are the first real runs with a user token and `from_token`.
- A preview's searches are bounded by the 50 s budget only; there's no shared back-off between parallel searches on a 429 (step 08's gap).
- A Spotify answer other than 401 to a user token (e.g. a `403` if Spotify ever refused `from_token`) is a generic "Couldn't reach Spotify", with the status only in the WARN log.
