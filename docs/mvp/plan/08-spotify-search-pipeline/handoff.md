# Step 08 handoff: Spotify search client and matching pipeline

## What was built

- **`internal/spotify`**: a standard-library Spotify Web API client (ADR-001) with one call, field-filtered track search (ADR-005).
- **`internal/pipeline`**: `MatchShow`, which takes a classified `setlist.Show` (step 06), runs the searches, applies `matching.Resolve` (step 07) and returns one `EntryResult` per shown entry, in setlist order. It defines the `Searcher` interface.
- **`internal/spotify/live_test.go`**: the manual check against live Spotify, behind the build tag `spotifylive`. It never runs in CI or in `go test ./...`.
- No new dependencies, endpoints, configuration or cloud resources. `internal/matching` and `internal/setlist` are unchanged and used as they are.

## Client API (`internal/spotify`)

```go
const DefaultBaseURL   = "https://api.spotify.com/v1"
const MarketFromToken  = "from_token" // user tokens only
const SearchLimit      = 10

var ErrUnauthorized // Spotify answered 401 → handler calls auth.SpotifyUnauthorized(w) (AUTH-8)
var ErrUnavailable  // anything else that failed → ERR-1 "Couldn't reach Spotify. Try again."

type Config struct {
    BaseURL       string        // default DefaultBaseURL
    HTTPClient    *http.Client  // default: 10 s timeout, never follows redirects
    Retries       int           // 429 retries; 0 → 3, negative → none
    Backoff       time.Duration // 0 → 1 s
    MaxRetryAfter time.Duration // 0 → 10 s
}
func New(cfg Config) *Client

func (c *Client) SearchTracks(ctx, token, track, artist, market string) ([]matching.Candidate, error)
func (c *Client) WithToken(token string) TokenSearcher // implements pipeline.Searcher
func Query(track, artist string) (q string, ok bool)   // the q= value; exported for step 09's recorder
```

- **Request:** `GET {base}/search?q=track:"<track>" artist:"<artist>"&type=track&limit=10&market=<market>`, `Authorization: Bearer <token>`, `Accept: application/json`. Exactly those four query parameters.
- **Market is required.** An empty market is an error (not `ErrUnavailable`). Step 10 passes `spotify.MarketFromToken`. Step 09's recorder, with an app token, passes a country code such as `GB`.
- **Mapping to `matching.Candidate`:** `id` → `ID`, `name` → `Title`, `artists[].name` (all, in order) → `Artists`, `album.name` → `Album`, `album.album_type` → `AlbumType`, `album.release_date` → `ReleaseDate`, `album.release_date_precision` → `ReleaseDatePrecision`. Results keep Spotify's order. `Title`, `Artists` and `Album` are what the preview displays, so nothing else is kept.
- **No results** is `(nil, nil)`.
- The client holds no token; each call takes one. It's safe for concurrent use.
- Error text never contains the token, the URL, the search text or the response body (tested), so it can be logged. Context cancellation is wrapped with `%w`, so `errors.Is(err, context.Canceled)` works and `errors.Is(err, ErrUnavailable)` is true as well.

### Quote handling

Spotify's search syntax has no escape for a double quote inside a quoted field value. **Each `"` in the track or artist name is replaced by a space**, then whitespace runs are collapsed and the ends trimmed: `The "Hellion"` → `track:"The Hellion"`, `"Heroes"` → `track:"Heroes"`. Other quote characters (`'`, `’`, `“`, `”`) are left alone. Spotify searches by words and ignores punctuation, so this shouldn't change the results; the matcher compares candidates with the **original** entry name, so the change can't make a wrong title match. A name that is empty after cleaning isn't searched and gives `(nil, nil)` (it would be MATCH-9 anyway).

### Retry settings

| Setting | Value |
|---|---|
| Retries after a 429 | 3 (so at most 4 requests per search) |
| Wait | `Retry-After` in seconds, as given |
| No or unparseable `Retry-After` (or `0`) | 1 s, doubling per retry (1 s, 2 s, 4 s), never more than 10 s (`MaxRetryAfter`), whatever `Backoff` and `Retries` are configured to |
| `Retry-After` over 10 s | give up at once with `ErrUnavailable` |
| Per-request timeout | 10 s, including the body |
| Response cap | 4 MB |

Worst case for one search is about 30 s of waiting, but only if Spotify keeps answering 429 with `Retry-After: 10`. HTTP-date `Retry-After` values aren't parsed (backoff is used).

### Status mapping

| Spotify | Error |
|---|---|
| 200 with valid JSON | candidates |
| 401 | `ErrUnauthorized` (not `ErrUnavailable`) |
| 429 | retried as above, then `ErrUnavailable` |
| anything else, including 403 and any 3xx (redirects aren't followed, so the bearer token only goes to the configured host) | `ErrUnavailable` |
| network error, timeout, undecodable JSON | `ErrUnavailable` |

## Pipeline API (`internal/pipeline`)

```go
const DefaultConcurrency = 4

type Searcher interface {
    SearchTracks(ctx context.Context, track, artist, market string) ([]matching.Candidate, error)
}
type SearcherFunc func(ctx context.Context, track, artist, market string) ([]matching.Candidate, error)

func MatchShow(ctx, show setlist.Show, s Searcher, market string) ([]EntryResult, error)
func MatchShowLimit(ctx, show setlist.Show, s Searcher, market string, limit int) ([]EntryResult, error)
```

Live use: `pipeline.MatchShow(ctx, show, client.WithToken(session.AccessToken), spotify.MarketFromToken)`.

### `Searcher` (step 09 implements it from fixtures)

- Called with the **entry name exactly as setlist.fm gives it** (`Entry.Name`, never the note) and the **expected artist exactly as step 06 gives it** (`Header.Band` or the trimmed `cover.name`), plus the market passed to `MatchShow`. A fixture searcher can key on `(track, artist)` as received. Quote cleaning happens inside the live client, not before the searcher.
- Return candidates in Spotify's order; `(nil, nil)` for no results; an error only for a real failure. Step 09's "missing recording" should be an error, which fails the match.
- It's called concurrently (up to 4 at once), so it must be safe for concurrent use.

### `EntryResult`

```go
type Status string
const (
    StatusFound          = "found"
    StatusCoverFallback  = "cover_fallback"
    StatusLive           = "live"
    StatusNotFound       = "not_found"
    StatusNotFoundMedley = "not_found_medley"
)
func (s Status) Found() bool // found, cover_fallback or live

type Track struct {
    ID, Title string
    Artists   []string // all credited, Spotify's order
    Album     string
}

type EntryResult struct {
    Position       int    // the preview row number (step 06)
    SetHeading     string // "" for the main set
    Name           string
    Note           string
    IsTape         bool
    OriginalArtist string // "" unless a cover
    Status         Status
    Live           bool   // any live match, including a live cover fallback
    Track          *Track // non-nil exactly when Status.Found()
}
```

- **Status mapping from `matching.Result`:** `!Found` → `not_found`; `CoverFallback` → `cover_fallback`; else `Live` → `live`; else `found`. Medleys → `not_found_medley` without searching.
- **Cover fallback that is also live:** `Status: "cover_fallback"` **and** `Live: true`. `Live` is true for every live match (it's always true when `Status == "live"`), so the preview can show "Live version" from `Live` alone and "Cover → ‹OriginalArtist›" from `Status`. Step 10 should expose both (e.g. `"status": "cover_fallback", "live": true`).
- **Tape** is `IsTape` on every status (PREV-4's extra label).
- **A cover found under the band** has `Status: "found"` (or `live`) and still has `OriginalArtist` set; the preview shows it as a plain Found.
- **Sets:** results are a flat slice in setlist order with each row's `SetHeading`. Step 10 groups consecutive rows by heading. A set whose entries are all skipped yields no rows, so it disappears naturally.

### Behaviour

- Walks `show.Sets` → `set.Entries`. `KindSkipped` → no row. `KindMedley` → `not_found_medley`, no search. Songs → `matching.Resolve` with a closure over `ctx`, the searcher and the market. `Resolve` searches the original artist only if the band pass chose nothing, so a cover found under the band costs one search (tested).
- **Concurrency:** `limit` workers (default 4, capped at the number of songs) each resolve one entry at a time; an entry's two passes run in sequence. So at most 4 searches are in flight (tested by counting in the fake; the test also checks 4 are actually used).
- **Order:** each worker writes to its entry's slot, so the result order is the setlist order whatever the completion order (tested with earlier entries finishing last).
- **Duplicates (ENT-7):** each occurrence is searched and kept in its own row (no de-duplication or caching, ADR-005).
- **Errors:** the first search error cancels the context passed to the other searches, no new entries are started, and `MatchShow` returns an error unchanged and **no results** (tested: the other in-flight searches see cancellation, nothing more is searched). Which error:
  1. if the caller's context was cancelled or timed out, **the context's error** (`context.Cause`), even though the searches report it wrapped in `spotify.ErrUnavailable`. A user who closes the tab isn't logged as a Spotify outage;
  2. otherwise **`spotify.ErrUnauthorized` if any search returned it**, even if another search failed first (e.g. a 502 and a 401 at about the same time). An expired token always leads back to login (AUTH-8), rather than to a Retry that keeps failing;
  3. otherwise the first search error.

  Handlers map it with `errors.Is(err, spotify.ErrUnauthorized)` / `ErrUnavailable`. For rule 2, `internal/pipeline` imports `internal/spotify` (for the sentinel only). A fixture searcher in step 09 that never returns it is unaffected.

## Tests (by spec ID where one fits)

- **Client** (`internal/spotify/client_test.go`, `httptest`): request header and exact query; JSON mapping including a `null` item and all artists; explicit market and `WithToken`; market required; `Query` quote cases; quotes in the request; empty name not searched; `Retry-After` honoured then success (waits recorded, not slept); repeated 429 → `ErrUnavailable` after 4 calls; `Retry-After` too long → give up at once; `TestSearchTracks_AUTH8_Unauthorized`; `TestSearchTracks_ERR1_Unavailable` (500, 403, redirect, bad JSON); `TestSearchTracks_ERR5_NoDetailsInErrors`; context cancelled; the fallback backoff capped at `MaxRetryAfter` (defaults, a large `Backoff`, 70 retries without overflow); the body read to EOF so sequential searches reuse one connection.
- **Pipeline** (`internal/pipeline/pipeline_test.go`, fake searcher): every status and field in one show, including tape + cover fallback and live cover fallback; `…_ENT1_ENT4_SkippedAndMedleysNotSearched`; setlist order with out-of-order completion; concurrency bound (peak exactly 4, two searches per cover); limit 1; `…_MATCH2_CoverSearches` (one search vs two); `…_MATCH9_NotFound`; `…_ENT7_Duplicates`; `…_ERR1_SearchErrorCancels`; `…_AUTH8_UnauthorizedWins` (401 kept whether it comes first or second); parent cancelled before and during the searches; `SearcherFunc`.

`gofmt -l .` (nothing), `go vet ./...` and `go test -race ./...` pass. `go vet -tags spotifylive ./internal/spotify/` passes too.

## Manual check against live Spotify

The build sandbox can't reach Spotify (its proxy refuses `api.spotify.com` and `accounts.spotify.com`), so the owner runs it locally in Git Bash with an app token from the client-credentials flow:

```bash
set -a; source .env; set +a
export SPOTIFY_TEST_TOKEN=$(curl -s -X POST https://accounts.spotify.com/api/token -u "$SPOTIFY_CLIENT_ID:$SPOTIFY_CLIENT_SECRET" -d grant_type=client_credentials | sed -E 's/.*"access_token":"([^"]*)".*/\1/')
go test -tags spotifylive -run Live -v -count=1 ./internal/spotify/
unset SPOTIFY_TEST_TOKEN
```

It searches "Battery" by Metallica in `GB` (override with `SPOTIFY_TEST_MARKET`) and logs the 10 candidates, then runs `MatchShow` on a small show (a song, a cover found under the band, a cover needing Diamond Head, `"Heroes"` with quotes, a solo and a medley) and logs each row.

**Result (owner's machine, 2026-11-03, app token, market `GB`): passed.**

- "Battery" / Metallica returned 10 candidates, all credited to Metallica: the studio track on *Master Of Puppets (Remastered)* first, then a remastered box-set copy, seven live versions and a 1985 rough mix.
- `MatchShow` on the six-entry show took **696 ms** (5 searches, run up to 4 at a time):

| # | Entry | Status | Chosen track |
|---|---|---|---|
| 1 | Battery | `found` | "Battery", *Master Of Puppets (Remastered)* |
| 2 | Am I Evil? (cover of Diamond Head) | `found` (band's own recording, one search) | "Am I Evil? - Remastered", *Ride The Lightning (Deluxe / Remastered)* |
| 3 | Helpless (cover of Diamond Head) | `found` (band's own recording, one search) | "Helpless - Remastered", *The $5.98 EP - Garage Days Re-Revisited (Remastered)* |
| 4 | `"Heroes"` (cover of David Bowie) | `cover_fallback` | `"Heroes" - 2017 Remaster` by David Bowie, *"Heroes" (2017 Remaster)* |
| – | Drum Solo | dropped (skipped) | – |
| 5 | Medley: A / B | `not_found_medley` (not searched) | – |

All sensible. The quoted title was searched as `track:"Heroes"` and still found Bowie's `"Heroes"`, and the matcher accepted it, because MATCH-4 normalisation turns the quotes into spaces.

**Not checked live:** a user token with `market=from_token` (step 10 will exercise it on Cloud Run), and Spotify's 429 and 401 responses (covered by the `httptest` tests only).

## Spotify quirks

- Search results can contain `null` items; they're skipped (as are items with no ID).
- `market=from_token` only works with a user token. The client-credentials token step 09 uses needs an explicit country.
- In development mode `limit` is capped at 10 (ADR-005).

Seen in the live check (worth watching in step 09):

- **Remastered and deluxe reissues keep the original release date.** The *Master Of Puppets (Deluxe Box Set / Remastered)* copies are dated `1986-03-03`, so MATCH-8 can't tell them from the original. Live and rough-mix tracks that were added to a box set carry the box set's original date too. Studio-before-live (MATCH-6) still ranks them correctly.
- **`album_type` is inconsistent for box sets:** some deluxe box sets are `compilation`, others `album` (*ReLoad (Remastered Deluxe Box Set)*, dated 2026).
- **Placeholder dates:** some albums are dated 1 January with `day` precision (*S&M* `1999-01-01`, *Through The Never* `2013-01-01`), so the precision field can't be trusted to mean an exact day.
- **Live versions dominate the results for popular songs:** 7 of the 10 "Battery" results were live. The studio track was still first, but this is the case ADR-005's "fetch a second page" remedy is for, if step 09 finds a song whose studio version is pushed out.
- **Unmarked suffixes:** "Battery - October 1985 / Work In Progress Rough Mix" has no step 07 marker, so its suffix isn't stripped and it isn't a candidate. That's the right outcome here, but only by accident (`rough mix` isn't an exclusion marker).
- **Spotify's own titles carry quotes:** Bowie's track is literally `"Heroes" - 2017 Remaster`.

## Code review

A fresh-context `/code-review` found 7 points. Fixed, each with a test that fails on the old code:

1. A 401 could be hidden by another search's error that arrived first, so the user would see "Couldn't reach Spotify" instead of being sent to login (AUTH-8). `ErrUnauthorized` now wins.
2. A caller's cancellation during a search was returned as `ErrUnavailable` (a false Spotify outage in step 10's logs). The context's error is now returned.
3. The no-`Retry-After` backoff wasn't capped and could overflow with a large `Retries`. It's now capped at `MaxRetryAfter`.
4. The response body wasn't read to EOF after decoding, so connections weren't always reused. It's now drained.

Not changed:

5. **No back-off shared between the parallel searches**: ADR-005 and the brief ask only that each search honour `Retry-After`, and the live check saw no 429s. Kept as a known gap; add a shared "not before" time on the `Client` if step 09 or 10 hits rate limits.
6. **Retry code duplicated from `internal/setlistfm`**: sharing it means refactoring step 05's merged code, outside this step. The difference in waits is deliberate (Spotify's `Retry-After` is used as given; setlist.fm's is a minimum). Step 11, which adds the playlist calls, is the natural point to extract a shared helper.
7. **Hand-built work queue in `MatchShowLimit`** (instead of a channel): style only; the bound and the cancellation are tested under `-race`.

## Notes for later steps

- **Step 09:** implement `pipeline.Searcher` from fixtures keyed on the `(track, artist)` the pipeline passes (see above); pass an explicit market (e.g. `GB`) to `MatchShow`. The recorder can call `spotify.New(spotify.Config{}).SearchTracks(ctx, appToken, name, artist, "GB")` for **both** artists of every cover, and use `spotify.Query` if it wants to record the exact query. `live_test.go` shows how to get an app token.
- **Step 10:** build the client once in `main` (`spotify.New(spotify.Config{})`, with `BaseURL` overridable for handler tests), add it to the handler's dependencies, and call `pipeline.MatchShow(ctx, show, client.WithToken(s.AccessToken), spotify.MarketFromToken)`. `errors.Is(err, spotify.ErrUnauthorized)` → `auth.SpotifyUnauthorized(w)`; anything else → `503 upstream_unavailable`, `service: "Spotify"`. Handler tests can pass a `pipeline.SearcherFunc` if the handler takes a searcher factory, or point the client at an `httptest` server.
- **Step 11:** the client's `get`/`do` (bearer header, no redirects, 429 retry, 401 → `ErrUnauthorized`, error text without details) is the pattern to extend for `POST /me/playlists` and adding items. It currently only does `GET`.

## Known gaps

- **The live check used an app token and `GB`**, not a user token with `from_token`. Step 10's Cloud Run check covers that.
- A 429 is retried per search, not per show: 4 parallel searches hit by a rate limit each wait on their own. There's no shared back-off.
- `Retry-After` as an HTTP date isn't parsed (falls back to the backoff). Spotify sends seconds.
