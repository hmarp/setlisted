# Step 07 handoff: matching rules (pure)

## What was built

- **`internal/matching`**: the matching rules MATCH-1 to MATCH-9 (spec §5.4). `Candidate`, title normalisation, version classification, `Choose` (one search's candidates → one track or none) and `Resolve` (the two passes over a step 06 `setlist.Entry`).
- Pure Go, no I/O (ADR-005). Its only internal imports are `internal/normalize` and `internal/setlist`. `go list -deps ./internal/matching` lists no `net` or `net/http` (`os` and `io` are there only because the standard library and `golang.org/x/text` pull them in, as for step 06).
- No new dependencies, endpoints, configuration or cloud resources. `normalize.Name` and `internal/setlist` are unchanged.
- Files: `matching.go` (types, `Classify`, `IsCandidate`, `Choose`, `Resolve`), `title.go` (title normalisation and suffix stripping), **`markers.go` (all marker lists, the one place to change them)**, `matching_test.go`.

## Public API

```go
package matching

type Candidate struct {
    ID, Title string
    Artists   []string // all credited artists' names
    Album     string
    AlbumType string   // "album" | "single" | "compilation" (AlbumTypeAlbum, …)
    ReleaseDate, ReleaseDatePrecision string // "1986-03-03","day" | "1986-03","month" | "1986","year"; precision is informational, never read
}

type Version string
const VersionStudio, VersionLive, VersionExcluded Version = "studio", "live", "excluded"

func Title(s string) string                                         // MATCH-4 normalised title
func Classify(c Candidate) Version                                  // MATCH-5 / MATCH-6
func IsCandidate(entryName, expectedArtist string, c Candidate) bool // MATCH-3 && MATCH-4
func Choose(entryName, expectedArtist string, candidates []Candidate) (c Candidate, isLive bool, ok bool)

type Result struct {
    Found         bool      // false = Not found (MATCH-9); other fields zero
    Track         Candidate
    Artist        string    // the expected artist of the pass that found Track
    CoverFallback bool      // found in the original artist's pass (MATCH-2)
    Live          bool      // live version (MATCH-6); can be true with CoverFallback
}
type CandidatesFunc func(expectedArtist string) ([]Candidate, error)

func Resolve(entry setlist.Entry, candidatesFor CandidatesFunc) (Result, error)
```

### Shape change from the brief: `Resolve` passes errors through

The brief proposed `Resolve(entry, candidatesFor func(expectedArtist) []Candidate) Result`. Step 08 must fail the whole match when a search fails (other than "no results"), so the function returns `([]Candidate, error)` and `Resolve` returns `(Result, error)`: a search error stops `Resolve` at once and is returned unwrapped (`errors.Is` works). Still pure: `Resolve` does no I/O itself. The caller's closure carries the context, token and market. The brief allowed an equally pure shape if recorded here; brief 08 is updated.

### `Resolve` behaviour

- Reads only `entry.Kind`, `entry.Name` and `entry.ExpectedArtists`. **`entry.Note` is never read** (ENT-6; tested with a note of "live acoustic demo").
- `entry.Kind != setlist.KindSong` → not found, **without calling `candidatesFor`**. Step 08 should still handle medleys itself (`not_found_medley`); this is just a guard.
- Passes are `ExpectedArtists` in order (`[band]` or `[band, original]`). `candidatesFor` is called **lazily**: the original artist is searched only if the band pass chose nothing (MATCH-2), so a cover found under the band costs one search. Any pass at index ≥ 1 sets `CoverFallback`.
- A band pass with candidates that are all excluded (e.g. only a demo) counts as "nothing chosen", so the original artist is tried.
- A band **live** version beats the original artist's studio version: MATCH-2's first pass found "the band's own recording", and MATCH-6 only ranks within a pass.

## Normalisation

**Artist (MATCH-3):** `normalize.Name` equality with **any** credited artist (case-folded, accents removed, trimmed, whitespace collapsed). Punctuation counts: `The Sword` ≠ `Sword`, `Ghost` ≠ `Ghost B.C.`. An empty expected artist matches nothing.

**Title (MATCH-4), `Title(s)`:**

1. **Strip version suffixes** from the raw title, repeatedly, from the end:
   - a suffix is a trailing `(…)` or `[…]` (balanced; nested brackets are fine), or the text after the **last** space–dash–space, where the dash is any of `-`, `‐` (U+2010), `‑` (U+2011), `‒` (U+2012), `–`, `—`, `―` (U+2015) or `−` (U+2212), the `dashes` constant in `title.go` (spaces on both sides; `Battery-Live` and `Song- Remastered` aren't split);
   - an unbalanced closing bracket at the end isn't a suffix, but an earlier dash suffix is still tried (`Fade to Black - Live (Remastered` → `fade to black`);
   - it's stripped **only if** the segment contains a marker (version **or** exclusion marker, below) as whole word(s), after the segment is normalised as in step 2;
   - stripping stops at the first segment without a marker, and never strips a segment that would leave nothing (`(Live)` stays `live`; `Live - Remastered` becomes `live`);
   - leftover spaces and dashes are trimmed (`Song - (Live)` → `Song`).
2. **Normalise the rest:** `normalize.Name`, then `&` → ` and `, apostrophes (`'`, `’`, `‘`, `ʼ`, `` ` ``, `´`) and full stops removed, any other Unicode punctuation or symbol → a space, whitespace collapsed.

Examples (all tested): `Seek & Destroy` = `Seek and Destroy` = `Seek&Destroy`; `Blackened - Remastered 2021` → `blackened`; `Battery (Live at Wembley)` → `battery`; `Battery - Live (Remastered 2016)` → `battery`; `Fade to Black (2016 Version)` → `fade to black`; `Live Wire` → `live wire`; `Part 1 - The Beginning` → `part 1 the beginning`; `Don't` = `Dont`; `T.N.T.` = `TNT`; `Hell-Bent` = `Hell Bent`; `Für Elise` = `Fur Elise`; `One` ≠ `One More Time`.

## Marker lists (`internal/matching/markers.go`)

All compared as whole words or whole runs of words of the normalised segment or album title.

| List | Markers | Used for |
|---|---|---|
| `versionMarkers` | `remaster`, `remastered`, `live` (the `liveMarker` constant, shared with the live check), `single version`, `explicit`, `mono`, `stereo`, `deluxe`, `radio edit`, `edit`, plus **a 4-digit number followed by `version`** (`isYearVersion`) | which trailing segments are stripped (MATCH-4) |
| `excludedMarkers` | `demo`, `demos`, `remix`, `remixes`, `remixed`, `instrumental`, `instrumentals`, `acoustic`, `karaoke`, `tribute` | exclusion (MATCH-5), from a stripped suffix **or the album title**; also stripped as suffixes |
| `liveMarker` | `live` | live (MATCH-6), from a stripped suffix **or the album title** |

## Classification and ranking

`Classify(c)` looks only at the **stripped suffixes and the album title**, never the core title (so `Live Wire` and `Acoustic Demo` as song names are studio):

1. `excluded` if any of them contains an exclusion marker (MATCH-5). Exclusion wins over live (`Battery (Live Acoustic)` is excluded).
2. else `live` if any of them contains the word `live` (MATCH-6). `Alive in Athens` isn't live; `Long Live Rock 'n' Roll` **is** (the spec's accepted limitation, tested on purpose).
3. else `studio`.

`Choose` keeps candidates passing MATCH-3 and MATCH-4, drops `excluded`, then sorts stably by:

1. studio before live (MATCH-6);
2. album type: `album` < `single` < `compilation` < anything else or empty (MATCH-7; case-insensitive);
3. release date, earliest first (MATCH-8). A date is read by its shape (`YYYY`, `YYYY-MM`, `YYYY-MM-DD`) as the start of that period, so `1986` < `1986-01-02` and `1986-03` < `1986-03-02`. The precision field isn't needed for that; a missing or unparseable date, or year 0 (Spotify's `0000` placeholder), sorts **after** every real date;
4. otherwise input order (Spotify's relevance order).

Nothing left → `ok == false` (MATCH-9). There's no fallback of any kind.

## Rule interpretations (by spec ID)

- **MATCH-4:** exclusion markers (demo, remix, …) are stripped as suffixes too. The spec calls them "version markers" (MATCH-5), and the brief's classification works "from its stripped title suffixes", which needs them stripped. Effect: `Battery (Demo)` is a candidate that is then excluded, rather than not a candidate; the outcome (never chosen) is the same.
- **MATCH-4:** the brief adds `radio edit` and `edit` to the spec's "such as" list; included. `Deluxe` also catches `Deluxe Edition`. A segment like `(feat. Someone)` is **not** a marker, so it's kept and the title doesn't match ("any other difference means no candidate"). Step 09 should check whether this matters for metal.
- **MATCH-4, punctuation:** apostrophes and full stops are removed (they sit inside words); other punctuation becomes a space. Known mismatch: `Rock'n'Roll` (`rocknroll`) ≠ `Rock 'n' Roll` (`rock n roll`). Step 09 will show whether it occurs.
- **MATCH-5:** plurals and `remixed` are included (an album called "The Demos" or "Remixes" identifies its tracks). Note that the album-title rule excludes any album with these words, e.g. Ozzy Osbourne's live album *Tribute* (1987).
- **MATCH-6:** the album title is checked as is (not suffix-stripped) for the word `live`; `Live!` counts.
- **MATCH-8:** ties after the date keep Spotify's order.
- **MATCH-2:** a pass whose candidates are all excluded counts as "no recording", so the fallback runs.

## What step 08 must supply

- `matching.Candidate` from each search result: `id`, `name` (→ `Title`), `artists[].name` (all of them, in order), `album.name`, `album.album_type`, `album.release_date`, `album.release_date_precision`. Pass the results **in Spotify's order** (ties keep it).
- A `CandidatesFunc` closure per entry, e.g.

  ```go
  r, err := matching.Resolve(entry, func(artist string) ([]matching.Candidate, error) {
      return searcher.Search(ctx, entry.Name, artist, market)
  })
  ```

  Return `(nil, nil)` for "no results"; return an error only for real failures, which `Resolve` passes back unchanged.
- Search with `entry.Name` as setlist.fm gives it; `Title` normalises it. Don't pass `entry.Note`.
- Status mapping for step 08's `EntryResult`: `!Found` → `not_found`; `CoverFallback` → `cover_fallback`; else `Live` → `live`; else `found`. `CoverFallback && Live` is possible and needs both labels (record the representation in step 08's handoff).
- `Resolve` searches one entry's passes **sequentially** (the fallback depends on the band pass). Step 08's 4-way parallelism goes across entries.

## Verification

`gofmt -l .` (nothing), `go vet ./...`, `go test -race ./...` all pass. `go list -deps ./internal/matching` has no `net` or `net/http`. Tests (`internal/matching/matching_test.go`), named by spec ID:

- `TestMatch_MATCH3_Artist`: band among several artists; other artist same title; `Motorhead`/`Motörhead` both ways; surrounding spaces; `The Sword` vs `Sword` both ways; `Ghost` vs `Ghost B.C.`; no artists; empty expected artist.
- `TestMatch_MATCH4_Title` and `…_TitleNormalisation`: every example from the brief, each marker, en and em dashes, several suffixes in either order, `Part 1 - The Beginning` with and without a remaster suffix, apostrophes (straight and curly), `!`, hyphens, `T.N.T.`, accents, `feat.`, an all-suffix title, brackets not at the end, unbalanced brackets (with a dash suffix before them), a dash without spaces, a year without `version`, `Take 2 Version`.
- `TestMatch_MATCH5_Excluded`: each marker in a suffix and in an album; excluded-only → not found; excluded loses even to a live compilation track; whole words only (`Demolition`, `Remixing History`, `Tributary`); a core title of `Acoustic Demo` is studio.
- `TestMatch_MATCH6_…`: studio (even a later compilation) beats live; only live → chosen and flagged; live suffix forms; `Live After Death`, `Live! In Paris` → live; `Alive in Athens` → studio; `Long Live Rock 'n' Roll` → live; `Live Wire` (also remastered) and `Live to Win` → studio.
- `TestMatch_MATCH7_AlbumType`: album > single > compilation > unknown; case-insensitive type.
- `TestMatch_MATCH8_EarliestRelease`: earlier wins; re-recording loses to the original and is used alone; year/month/day precisions mixed both ways; bad, empty and `0000` dates lose; ties keep input order.
- `TestMatch_MATCH9_NoGuesses`: no candidates; other artists only; near titles only; excluded only.
- `TestMatch_MATCH1_…`, `TestMatch_MATCH2_CoverTwoPasses`: band only for a song (one search, no retry elsewhere); cover found under the band (one search); fallback with `CoverFallback`; band with only a demo → fallback; band live beats original studio; fallback + live; neither → not found.
- `TestMatch_Resolve_Errors`, `…_OnlySongs`, `TestMatch_ENT6_NoteIgnored`, `TestMatch_EndToEnd_Show` (a step 06 `setlist.Build` show with a tape credited to another artist, a remastered title, a solo and a cover).

## Code review

A first fresh-context `/code-review` found 6 points. Fixed: a `0000` release date won MATCH-8 ties (now unknown); an unbalanced trailing bracket stopped an earlier dash suffix from being stripped; `live` was written twice in `markers.go` (now one constant). Recorded as known gaps rather than changed, because they follow the brief's and spec's rules: the stripping stops at an unmarked segment, and a marker anywhere in a segment strips it. Not changed: normalising the entry name once per `Choose` call instead of per candidate (at most 10 candidates; not worth the code).

A second fresh-context `/code-review` (on a different model) found 7 points. Fixed: only three dash characters were recognised as separators and the set was written twice (now the one `dashes` constant, with the hyphen, non-breaking hyphen, figure dash, horizontal bar and minus sign added); `ReleaseDatePrecision` looked meaningful but is never read (now documented as informational on the field, kept because the brief asks for it and step 08 maps it). Not changed:

- **A marker anywhere in a segment strips it** ("Heroes (Live Forever)"): already a known gap. The brief's rule is "contains a known version marker", which is what makes "Live at …" work; a whole-phrase rule would change the spec's meaning. MATCH-3 still applies, so the worst case is another song by the same band.
- **An unmarked trailing segment stops the stripping** ("(feat. X)", "(1999)"): the same point as the first review's; known gap; step 09 can add markers.
- **"edit" and "radio edit" aren't in the spec's list:** the brief adds them, and the spec's list is "such as". "radio edit" is redundant with "edit" but documents the intent.
- **Full stops and apostrophes deleted** ("3.14" = "314", "A.M. 180" = "AM 180"): only within one artist's tracks; deliberate so that "Don't" = "Dont" and "T.N.T." = "TNT".
- **Repeated normalisation in `Choose`:** declined again (at most 10 candidates).

## Known gaps

- Not run against real Spotify data; step 09 is the first real check. Things to watch there: `feat.` suffixes, `Rock'n'Roll`-style spacing, letters such as `ø` that `normalize.Name` keeps (step 05), live albums whose titles lack the word "live" (they're treated as studio), and studio albums with "Live" in the title.
- **Stripping stops at the first trailing segment without a marker** (the brief's rule), so "Battery (Live) [Bonus Track]" keeps both suffixes and doesn't match "Battery". Watch for "Bonus Track" and similar in step 09.
- **A marker anywhere in a segment strips it**, which is what makes "Live at …" work, but a real subtitle containing "live", "edit", "mono" or "stereo" is stripped too: "Rock and Roll (Live for Today)" would become "rock and roll" and count as live. The same kind of accepted limitation as "Long Live Rock 'n' Roll"; flag it if step 09 shows it.
- Only the markers in `markers.go` are recognised. Others the spec's "such as" covers can be added there (step 09).
