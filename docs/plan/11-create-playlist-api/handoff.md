# Step 11 handoff: Create playlist API

## What was built

- **`POST /api/playlists`** (protected, `Origin`-checked), in `internal/httpapi/playlists.go`. It validates the body, fetches the show from setlist.fm again, builds PL-2's name and PL-3's description, creates a private playlist and adds the tracks in batches. Fully documented in `docs/api.md`.
- **`internal/spotify`**: `CreatePlaylist`, `AddTracks`, `ValidTrackID` and `MaxItemsPerRequest` (`playlist.go`). The GET-only `get`/`do` became `call`, which makes any request with a JSON body (`get` and `post` wrap it). Any 2xx is now success, because create and add answer `201`. The words that must never reach error text (`spotifyReason`) are now passed in by the caller: search passes the cleaned track and artist, as before, and create passes the name and description.
- **New error code `partial_create`** (502), with `jsonapi.PartialCreate()`. Its response is the only error body with a second top-level field, `playlist`.
- **`auth.EndSession(w)`**: clears the session cookie without writing a response (used for a 401 after the playlist exists, see below).
- **`failUpstream`** (`internal/httpapi/preview.go`): the preview's 503-or-499 helper, now shared with create, with `op` naming the endpoint in logs. The preview's behaviour and log lines are unchanged (`preview abandoned by the client`).
- No new dependencies, configuration, scopes or cloud resources.

## Endpoint shape

```
POST /api/playlists
{"showId": "63de4613", "trackIds": ["4uLU6hMCjMI75M1A2tKUQC", …]}

201 {"playlist": {"id", "name", "trackCount", "url"}}
502 {"error": {"code": "partial_create", "message", "service": "Spotify"}, "playlist": {"id", "name", "url"}}
```

**Validation** (all `400 invalid_request`, before any upstream call):

- the body must be one JSON object of at most 32 KB;
- `showId` must match step 10's `setlistIDPattern`;
- `trackIds` must have 1 to 200 entries (`maxTracks`), each 22 base62 characters (`spotify.ValidTrackID`).

An empty `trackIds` gets PREV-7's message, "None of these songs were found on Spotify."; anything else gets "The request isn't valid.", except too many tracks, which gets "That's too many songs for one playlist." Unknown fields are ignored.

## Name and description

- **Name (PL-2):** `Band – Tour (Setlist)` if `Header.Tour` isn't blank, else `Band – City, DD Mon YYYY`. It uses an en dash, U+2013.
- **Description (PL-3):** `Venue, City, DD Mon YYYY. Setlist: URL. Created by Setlisted.`
- The date is `Header.Date`, which `setlistfm.Setlist.DisplayDate` already formats.
- Blank parts (a missing city or venue) are left out rather than leaving `, ,`. setlist.fm always has them, so this is only a safety net.
- **Length limits:** the name is cut to **100** characters and the description to **300**. Cuts count runes, so a multi-byte character is never split, and the cut text ends in `…`. Text is cut, never refused. See below for how sure these figures are.

## Spotify facts, and what still needs confirming

The build sandbox can't reach `developer.spotify.com` or the API, so these are from the brief and from what I already knew, **not checked against the current docs**. The owner should confirm them (checklist below):

| Fact | Value used | Where |
|---|---|---|
| Create endpoint | `POST /me/playlists`, body `{"name", "description", "public": false}` → `201` with `id` and `external_urls.spotify` | `CreatePlaylist` |
| Add endpoint | `POST /playlists/{id}/items`, body `{"uris": ["spotify:track:<id>", …]}` → `201` with `snapshot_id` (ignored) | `AddTracks` |
| Max URIs per add request | **100** (historical value) | `spotify.MaxItemsPerRequest` |
| Name length | **100** characters (the limit Spotify's own apps enforce; I don't know of one in the Web API reference) | `maxNameLength` |
| Description length | **300** characters (the same) | `maxDescriptionLength` |

- A real setlist is about 10–40 tracks, so one batch covers it. 200 tracks (the cap) take 2 batches.
- If the docs give a different batch maximum, change only `MaxItemsPerRequest`.
- If a name or description is refused live, Spotify's reason is in the WARN log (`Spotify says "…"`), unless it repeats the name or description.

## Decisions taken in this step

- **Spotify 401 while adding tracks (after the playlist exists) → `502 partial_create`, and the session cookie is cleared too.** ERR-3 says the user must get the partial playlist's link. A bare `401` would lose it, so the link wins. The next request then answers `401` and the user logs in again (AUTH-8). A 401 when *creating* is a plain `401 not_logged_in` (nothing exists yet). In practice a token can hardly expire in the second between the two calls.
- **The browser going away:**
  - **Before the Spotify calls** (during validation or the setlist.fm fetch): the request stops, with nothing created. It's logged as `499` at INFO, as in the preview.
  - **Once the Spotify calls start**, the browser's cancellation is ignored (`context.WithoutCancel`, still bounded by the same 50 s budget deadline). The playlist is created and **every batch added**, even with nobody waiting for the response, so a closed tab or a navigation never leaves an empty or half-filled playlist. This includes the create call itself, because cutting it off mid-way could leave a playlist on Spotify that we'd never fill.
  - Because of this, a Spotify failure after that point is always a real failure. It's logged at WARN with `client_gone: true|false`, never hidden as a 499.
- **One budget for the whole request: 50 s** (`createBudget`), as for the preview. An overrun before creation is a `503`, and after creation a `partial_create`. Either way the log says the budget was used up.
- **429s are retried for the POSTs too**, with the same rules as search (step 08): a 429 means Spotify didn't act on the request. Nothing else is retried. In particular a network error or timeout on create is **not** retried, because we can't know whether the playlist was created.
- **`trackCount`** in the `201` is the number of IDs sent (all of them were added). The `partial_create` body has no `trackCount`. How many were added is logged (`added`, `tracks`), not shown.
- **The `partial_create` message** is "Creating the playlist failed part-way: it was created on Spotify, but some songs may be missing." It's "may", because a failed batch might have been applied by Spotify before the error.

## Error codes the UI must handle (step 12)

| Status | Code | Playlist exists? | UI |
|---|---|---|---|
| `201` | — | yes, complete | PL-7 confirmation with `playlist.name`, `playlist.trackCount`, "Open in Spotify" → `playlist.url` |
| `502` | **`partial_create`** | **yes**, partial | `message` and a link to `playlist.url` (ERR-3). A retry creates a second playlist. |
| `503` | `upstream_unavailable` (`setlist.fm` or `Spotify`) | no | `message` with Retry, which posts the same body (ERR-1, ERR-4) |
| `404` | `not_found` | no | the show has gone from setlist.fm |
| `401` | `not_logged_in` | no | login screen |
| `400` | `invalid_request` | no | `message` (shouldn't happen with an unchanged preview) |
| `403` | `forbidden_origin` | no | shouldn't happen from the app's own page |

The step 12 brief now has a *From step 11* note with this.

## Logging

- **Success:** the request line only. If the browser went away, there's also an INFO `create_playlist finished after the client went away`.
- **Failures:**
  - WARN `setlist.fm call failed` / `Spotify call failed` (`op: create_playlist`);
  - WARN `Spotify call failed after the playlist was created`, with `added`, `tracks` and `client_gone`;
  - INFO `Spotify rejected the session; logging out`.
- **Never logged:** names, venue, city, description, track IDs, the token, and the playlist's link or ID. A test checks this on success, a create failure, a 401 and a partial create. The request line has only the path, `/api/playlists`, and the show ID is in the body, so it isn't logged either.

## Tests

`internal/spotify/playlist_test.go` (`httptest`):

- the exact create body (`public: false`, no other fields), method, path and headers;
- the error mapping for 401, 403, 500, a missing ID or link, and bad JSON;
- a 429 retried with the same body;
- Spotify's reason dropped when it echoes the name or description;
- 250 tracks → batches of 100, 100 and 50, in order, with a duplicate across batches;
- exactly 100 tracks → 1 batch;
- a failed second batch stops there and reports 100 added (500 and 401);
- `ValidTrackID`.

`internal/httpapi/playlists_test.go`, through the real router with a fake setlist.fm and the real client pointed at a fake Spotify:

- **Success:** the exact `201` body. The exact create request: the PL-2 name with a tour and the exact PL-3 description. The URIs are in order with a duplicate kept, there's one setlist.fm call, and the session's token is used.
- **Naming and limits:** the name without a tour (through the router), a blank tour counted as none, and the en dash. Cuts at 100 and 300 characters on multi-byte text, and text exactly at the limit left alone.
- **Batching:** 150 tracks give 2 requests; 200 (the cap) give 2.
- **Validation:** 13 invalid bodies → `400`, with no upstream call.
- **`Origin` and session:** no `Origin` or a foreign one → `403`, no session → `401`, `GET` → `404`.
- **Errors before creation:**
  - unknown show → `404`;
  - setlist.fm `502` → `503`;
  - create `500` or `403` → `503`, with no add calls;
  - create `401` → `401` with the cookie cleared.
- **`TestCreatePlaylist_ERR3_PartialCreate`:**
  - the second batch fails with `500` or `401` → the exact `502` body with the link;
  - one create, two batches, and no other calls (no rollback);
  - the cookie is cleared only for the `401`;
  - a WARN is logged with `added: 100`.
- **Budget:** a slow create → `503` within the budget, logged as the budget being used up.
- **Client gone:** during setlist.fm, and just after setlist.fm answers → `499`, nothing created, no WARN. During the create call → still `201`, and all 150 tracks added.
- **Logs** contain no user data.

`gofmt -l .` prints nothing. `go vet ./...`, `go vet -tags live ./...`, `go vet -tags spotifylive ./...` and `go test -race ./...` pass, including step 09's `TestMatchingQuality`.

## Live check (owner)

Not run in this session: the sandbox has no network access to setlist.fm or Spotify. To fill in:

- [ ] **Confirm in Spotify's Web API reference**:
  - "Add Items to Playlist": the endpoint is `POST /playlists/{playlist_id}/items`, the body takes `uris`, the maximum per request is ___ (we use 100), and success is `201`.
  - "Create Playlist": `POST /me/playlists` with `name`, `description`, `public`, and success is `201`. Is there any documented name or description length limit? ___ (we cut at 100 and 300).
- [ ] **Local**, on `http://127.0.0.1:8080`, logged in through the browser: preview a real show, then create, from the console (see the README).
  - [ ] `201`, and `name` and `trackCount` as expected;
  - [ ] in Spotify the playlist is **private**, with the right name and the exact description including the setlist.fm link (check that Spotify didn't strip or escape the URL or the en dash), the tracks in setlist order, and the default cover;
  - [ ] "Open in Spotify": the `url` opens the playlist.
  - Anything odd in Spotify's responses: ___
- [ ] **After the merge deploys:** the same on Cloud Run, including opening `url` on a phone with the Spotify app (PL-8).
- [ ] Typical response time (`duration_ms`), local ___ ms, Cloud Run ___ ms.

## Notes for later steps

- **Step 12:** see the table above and the brief's *From step 11* note.
  - Disable "Create playlist" on press (PL-6) and re-enable it only after an error.
  - For `partial_create`, don't offer a plain Retry that looks harmless: it creates another playlist.
  - The confirmation doesn't list unmatched entries (PL-9).
- **Step 13:** each acceptance run creates real playlists. Delete the test playlists in Spotify by hand afterwards (the app never deletes anything).

## Not done (left as they were)

- **A shared retry helper for `internal/setlistfm` and `internal/spotify`** (step 08 review point 6). Merging them means changing step 05's client, and the two deliberately treat `Retry-After` differently. The Spotify side is now one `call` for every method, which was the part this step needed.
- **The artist endpoints still log a closed tab as a setlist.fm WARN** (step 10 review point 6). `failUpstream` is now shared and could replace `setlistFMHandlers.upstreamError`, but that's step 05's code and outside this step.

## Known gaps

- **A create that times out or loses its connection** gives `503`, but Spotify may already have created an empty playlist, and Retry then creates another. This can't be detected without listing the user's playlists, which would need another scope. It's rare, and the user can delete the empty one.
- **No idempotency across presses:** PL-6 (one press, at most one playlist) relies on the UI disabling the button. Two separate requests always make two playlists, as PL-1 requires.
- The Spotify facts table above hasn't been checked against the current docs.
- Only `201` responses were seen in the fakes. Real responses haven't been seen yet.
