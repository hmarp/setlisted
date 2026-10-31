# Step 05 handoff: setlist.fm client, artist search and show list API

## What was built

- **`internal/normalize`**: `Name` and `Equal`, the spec's "normalised name".
- **`internal/setlistfm`**: a rate-limited client with typed responses: `SearchArtists`, `ArtistSetlists`, `Setlist` and `Artist`.
- **Endpoints** (both need a session; documented in `docs/api.md`):
  - `GET /api/artists?q=…`: artist search with the clear-match rule;
  - `GET /api/artists/{id}/shows`: up to 20 recent non-empty shows.
- New error code `upstream_unavailable` (503) in `internal/jsonapi` and `docs/api.md`, with `jsonapi.UpstreamUnavailable(service)` and the constants `jsonapi.ServiceSetlistFM` / `jsonapi.ServiceSpotify`.
- `docs/plan/05-setlistfm-search-shows/setup.md`: the owner's key, secret and check list.
- New dependency: **`golang.org/x/text`** v0.43.0 (Unicode decomposition for accent-insensitive names; allowed by the brief under ADR-001).

## Normalisation (`internal/normalize`)

```go
normalize.Name(s string) string // "  Blue Öyster  Cult " → "blue oyster cult"
normalize.Equal(a, b string) bool
```

- Lower-cases (`strings.ToLower`), removes accents (NFD, drop combining marks `Mn`, then NFC), trims, and **collapses internal runs of whitespace to one space** (`strings.Fields`, so tabs and non-breaking spaces count). Collapsing isn't in the spec's definition; it was asked for by the brief and only makes more names equal that a person would call equal.
- Only letters that decompose lose their accent: `ö → o`, `é → e`, but **`ø`, `ł`, `æ` and `ß` are kept** (they aren't "letter + accent" in Unicode). `Ørkenkjøtt` → `ørkenkjøtt`. Step 07 can add a small mapping if fixtures show it matters.
- Punctuation is untouched: `Ghost` ≠ `Ghost B.C.`, `The Sword` ≠ `sword`. Step 07 extends this for titles (`&`/`and`, etc.); keep `Name` itself as is, because SRCH-3's strictness relies on it.
- Safe for concurrent use (a transformer per call).

## setlist.fm client (`internal/setlistfm`)

```go
c := setlistfm.New(setlistfm.Config{APIKey: key}) // other fields: BaseURL, HTTPClient, Interval, Retries, Backoff (tests)

c.SearchArtists(ctx, name) ([]Artist, error)              // GET /search/artists?artistName=…&sort=relevance&p=1
c.ArtistSetlists(ctx, mbid, page) (SetlistsPage, error)   // GET /artist/{mbid}/setlists?p=…
c.Setlist(ctx, setlistID) (Setlist, error)                // GET /setlist/{setlistId}
c.Artist(ctx, mbid) (Artist, error)                       // GET /artist/{mbid}
```

### Types

```go
type Artist struct{ MBID, Name, Disambiguation string }
type Setlist struct {
    ID, EventDate string // EventDate is dd-MM-yyyy
    Artist Artist
    Venue  Venue          // Venue.Name, Venue.City.Name
    Tour   *Tour          // nil when there's no tour
    Sets   Sets           // Sets.Set []Set
    URL    string         // the show's setlist.fm page (PL-3 needs it)
}
type Set  struct{ Name string; Encore int; Song []Song } // Encore: 0 = not an encore, 1, 2, …
type Song struct{ Name string; Tape bool; Cover *Artist; Info string }
type SetlistsPage struct{ Setlists []Setlist; Total, Page, ItemsPerPage int } // + HasMore()
```

Helpers: `Setlist.SongCount()` (all entries across all sets, including tapes and nameless entries), `Setlist.Date()` (parsed, midnight UTC), `Setlist.DisplayDate()` (`DD Mon YYYY`, or the raw `EventDate` if it doesn't parse), and `setlistfm.FormatDate(time.Time)`. **Step 11 should use `DisplayDate` for PL-2 and PL-3** rather than formatting dates again.

### Errors

- `setlistfm.ErrUnavailable` (wrapped, with a short cause): network failure, timeout, any non-200 other than 404/429, an undecodable body, or still `429` after the retries. Handlers map it to `jsonapi.UpstreamUnavailable(jsonapi.ServiceSetlistFM)`.
- `setlistfm.ErrNotFound`: a `404` from `Setlist` or `Artist`. Steps 10 and 11 map it to `404 not_found` for an unknown show.
- **`404` is not an error** for `SearchArtists` (empty slice) or `ArtistSetlists` (empty page whose `HasMore()` is false).
- Error text never holds the key, the URL (so no search text or IDs), or the response body: the `*url.Error` wrapper is stripped. It's safe to log, and the handlers do (`setlist.fm call failed`, with `op` and `err`).

### Rate limiter and retries

| Setting | Value | Where |
|---|---|---|
| Spacing between calls (process-wide) | **600 ms** (≈ 1.7 per second) | `setlistfm.DefaultInterval` |
| Retries on `429` | **2** (3 attempts in all) | `DefaultRetries` |
| Backoff | 1 s, then 2 s; a longer `Retry-After` (seconds) wins, capped at 5 s | `DefaultBackoff`, `maxBackoff` |
| Timeout per HTTP call | 10 s | `requestTimeout` |

- The limiter reserves a slot per call under a mutex and sleeps until it, so calls from concurrent requests are spaced too (ADR-005; one Cloud Run instance, ADR-003). Retries go through the limiter as well.
- **Rate limit (confirmed):** our key allows **max. 2.0 requests/second and max. 1,440/day**, as shown in the key's settings on setlist.fm (confirmed by the owner, 2026-10-31). This matches the standard-key figures in third-party documentation. The 600 ms spacing keeps us just below 2 per second.
- The daily limit isn't enforced in-process. A show list costs at most 3 calls and a search 1, so 1,440 a day is far beyond a few friends' use. Hitting it would show up as `429`s and then ERR-1.
- Worst case for one show list: about 30 s if all 3 page calls time out (timeouts aren't retried), or about 10 s if every call is rate-limited (3 pages × 3 s of backoff), both under the server's 60 s write timeout. A preview (step 10) needs only one setlist.fm call.

## Endpoints

Full shapes are in `docs/api.md`. Summary:

- `GET /api/artists?q=…` → `{"match": "clear"|"picker"|"none", "artists": [{"id", "name", "disambiguation"}]}`.
  - `q` is trimmed; empty → `400 invalid_request` "Enter a band name."; over 200 characters → `400` "That band name is too long." (a limit chosen in this step).
  - **Clear match** looks at **all of setlist.fm's first page** (up to 30, relevance order), not just the first 10: if exactly one has the normalised name, it's `clear`. Otherwise `picker` returns the first 10.
  - `disambiguation` is always present, `""` when absent.
- `GET /api/artists/{id}/shows` → `{"artist": {"id", "name"}, "shows": [{"id", "date", "city", "venue", "tour", "songCount"}]}`.
  - Pages 1, 2, 3 are fetched in order, stopping at 20 non-empty shows, at a page that has no more (`page × itemsPerPage ≥ total`, an empty page, or a 404), or after page 3.
  - `tour` is always present and is `null` when absent (a tour with an empty name counts as absent).
  - `artist.name` comes from the setlists. **If the artist has no setlists at all, one extra call (`GET /artist/{mbid}`) gets the name** for SHOW-5's message; a 404 there → `404 not_found`. An `{id}` that isn't a UUID → `404 not_found` without calling setlist.fm.
- Both are registered on the protected `api` mux in `httpapi.NewHandler`, which now takes `(logger, static, a *auth.Auth, sfm *setlistfm.Client)`. The handlers are methods on `setlistFMHandlers` in `internal/httpapi/artists.go`. When step 08 adds the Spotify client, consider turning `NewHandler`'s arguments into a small deps struct.

## setlist.fm quirks (from the API docs and client libraries; confirm in the live check)

- **`404` for "no results"**, on search and on setlist pages past the end or for artists with no setlists. Handled as empty.
- `eventDate` is `dd-MM-yyyy`. `sets` is nested: `"sets": {"set": [...]}`, and `set` may be missing or `[]` for announced shows. `encore` is a number. `tour`, `cover` and `info` are simply absent when empty.
- Entries can have an empty `name`. They count towards `songCount` (SHOW-3 counts entries); step 06 skips them (ENT-1).
- An artist search returns 30 per page; setlists come 20 per page.
- The API isn't reachable from Claude's sandbox, so the client was tested only against fakes. Anything different found in the live check should be added here.

## Logging

- No search text, artist names, MBIDs or the key in the app's own warning lines. Failures log `op` (`search_artists`, `artist_setlists`, `artist`) and the error category (e.g. `setlist.fm unavailable: status 403`).
- **The request line logs the path**, so `/api/artists/{mbid}/shows` puts the chosen artist's MBID in the logs (the search's `q` is in the query string, which isn't logged). An MBID is a public ID, not a name, and Cloud Run's own request logs record full URLs, query strings included, anyway (step 04 known gap). Accepted.

## Also fixed: `.env` parse errors leaked secrets (step 01 code)

During the local check, a `.env` with a value on its own line made the app exit with godotenv's parse error, which **quotes the file's contents**, including secrets, on stdout. `cmd/setlisted/main.go` now loads `.env` through `loadDotEnv`: a parse error becomes the generic `reading .env: the file couldn't be parsed; check that every line is NAME=value, …`, while file-system errors (which only name the path) are kept. A missing `.env` is still fine. Cloud Run has no `.env`, so production was never affected. Tested in `cmd/setlisted/main_test.go`.

## Verification

Done in this session: `gofmt -l .` (nothing), `go vet ./...`, `go test -race ./...` all pass. Tests cover:

- `normalize.Name`: case, accents (`Motörhead`, `Blue Öyster Cult`, pre-decomposed input), surrounding and internal whitespace, `ø`; `The Sword` ≠ `sword`.
- Clear-match classification: one exact match → `clear`; two → `picker`; none → `picker`; more than 10 → 10; empty → `none`.
- Show list against a fake setlist.fm: zero-entry shows dropped, stops at 20, follows up to 3 pages and no more, fewer than 20, a 404 page, date formatting, song count with tapes, missing tour, no setlists (artist lookup), only empty setlists (no lookup), unknown and malformed IDs.
- The client: `x-api-key` and `Accept` headers, query parameters, decoding of every field used, 404 handling, a `429` retried then succeeding, giving up after the bound, `Retry-After`, 5xx/403/bad JSON → `ErrUnavailable`, no URL or body in error text, limiter spacing (50 ms interval) and cancellation.
- The endpoints: `401` without a session, `400` for an empty/whitespace/too-long `q` with no upstream call, `503 upstream_unavailable` with the exact body, and no search text in logs.

To do with the owner (setup.md §1, §4, §5). Fill in:

- [x] Rate limit confirmed from the key's settings: max. 2.0/second and max. 1,440/day (2026-10-31).
- [x] Local check on `127.0.0.1` with the real key: the owner confirmed searching works (2026-10-31).
- [ ] After the merge deploys: the same on Cloud Run.

## Known gaps

- **Not checked against the live API** in this session (no network access to setlist.fm). Field names follow setlist.fm's documented JSON.
- Only `429` is retried. A `5xx` or timeout goes straight to ERR-1, and the user's Retry repeats it (ERR-1 asks for that).
- No caching between requests (ADR-005), so going back to a show list refetches it.
- A band with many announced (empty) shows at the top of its history can get fewer than 20 shows, because of the 3-page cap (ADR-005's trade-off).
