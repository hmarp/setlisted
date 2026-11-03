# Step 10 handoff: Preview API

## What was built

- **`GET /api/shows/{id}/preview`** (protected), in `internal/httpapi/preview.go`: fetches the setlist (step 05), converts and classifies it (`Setlist.Show()`, step 06), runs `pipeline.MatchShow` with the session's token and `market=from_token` (step 08), and returns the preview. Fully documented in `docs/api.md`.
- **`httpapi.Deps`**: `NewHandler(d Deps)` and `NewServer(addr, d Deps)` now take one dependencies struct (`Logger`, `Static`, `Auth`, `SetlistFM`, `Spotify`), as steps 01 and 05 suggested. `main` builds **one** `spotify.New(spotify.Config{})` and passes it in. Existing tests use a `testDeps(...)` helper.
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

- [ ] Local, on `http://127.0.0.1:8080`, logged in through the browser: a real show's preview returns `200` with sensible matches.
- [ ] After the merge deploys: the same on Cloud Run.
- [ ] Typical response time (the request log's `duration_ms`): ___ ms for a ___-song show (local), ___ ms (Cloud Run, warm).

## Notes for later steps

- **Step 11:**
  - `showId` is `show.id`; validate it with the same `setlistIDPattern` (`internal/httpapi/preview.go`) so both endpoints agree on what a show ID is, and map `setlistfm.ErrNotFound` to `404` the same way.
  - `trackIds` are Spotify track IDs (`track.id`, base62), in order, duplicates kept, already excluding not-found and medleys. The server only re-fetches the show for the name and description (`show := sl.Show()` gives `Band`, `Tour`, `City`, `Date` already as `DD Mon YYYY`, `Venue` and `URL`).
  - The Spotify client is `d.Spotify` in `httpapi.Deps`; add the playlist calls to `internal/spotify` (see step 08's note) and the route on the `api` mux.
  - Reuse the preview's pattern: one budget under the 60 s write timeout for the whole request, and `previewHandler.fail`'s client-gone check (`r.Context().Err() != nil` → 499, no WARN). For create, think about a client that goes away **after** the playlist exists: the playlist stays (no rollback, ERR-3), and nobody sees the response.
- **Step 12:** render from the tables above: no heading for `heading == ""`; `note` as secondary text when not `null`; "Cover → ‹originalArtist›" only when `status == "cover_fallback"`; "Live version" from `live`; "Tape" from `tape`; the summary from `summary`; disable "Create playlist" and show PREV-7's message when `canCreate` is false; `show.setlistUrl` for the setlist.fm link; post `show.id` and `trackIds` unchanged. A `401` means log in again; a `503` means show `message` with Retry (repeat the same GET).

## Known gaps

- **Not run against live setlist.fm or Spotify in this session** (no network from the sandbox). The owner's local and Cloud Run checks above are the first real runs with a user token and `from_token`.
- A preview's searches are bounded by the 50 s budget only; there's no shared back-off between parallel searches on a 429 (step 08's gap).
- A Spotify answer other than 401 to a user token (e.g. a `403` if Spotify ever refused `from_token`) is a generic "Couldn't reach Spotify", with the status only in the WARN log.
