# Step 06 handoff: entry classification rules

## What was built

- **`internal/setlist`**: the app's own model of a show (`Show`, `Header`, `Set`, `Entry`, `Kind`) and the classification rules ENT-1 to ENT-7, with the expected artists for MATCH-1/MATCH-2 and the PREV-3 headings and positions. Pure Go; its only internal import is `internal/normalize` (`go list -deps ./internal/setlist` lists no `net` or `net/http`).
- **`setlistfm.Setlist.Show()`** (`internal/setlistfm/show.go`): converts a fetched setlist into a classified `setlist.Show`.
- No new dependencies, endpoints, configuration or cloud resources. Step 05's types and `normalize.Name` are unchanged.

### Why the conversion lives in `setlistfm`

The brief asks for a conversion from step 05's types *and* for `internal/setlist` to have no HTTP dependencies. `setlistfm` imports `net/http`, so a conversion inside `setlist` would pull it in. The conversion is therefore a method on `setlistfm.Setlist`. `setlist.Build` takes plain input types (`SourceSet`, `SourceEntry`), which also lets step 09's fixtures or tests build shows directly.

## Public API

```go
package setlist

type Kind string
const KindSkipped, KindMedley, KindSong Kind = "skipped", "medley", "song"

type Header struct{ ID, Band, Date, Venue, City, Tour, URL string } // Date is DD Mon YYYY; Tour "" when none
type Show struct {
    Header
    Sets []Set // in setlist order; each set holds ALL its entries, skipped ones included
}
type Set struct {
    Heading string // "" for the first set; see the rule below
    Entries []Entry
}
type Entry struct {
    Position        int      // display number 1…n across the show; 0 for skipped entries
    Name            string   // as on setlist.fm
    Note            string   // setlist.fm's "info"; never affects matching (ENT-6)
    IsTape          bool
    OriginalArtist  string   // "" unless it's a cover
    Kind            Kind
    ExpectedArtists []string // [band] | [band, original] | empty for skipped and medleys
}
func (e Entry) IsCover() bool       // OriginalArtist != ""

type SourceSet   struct{ Name string; Encore int; Entries []SourceEntry }
type SourceEntry struct{ Name, Note string; Tape bool; CoverArtist string }

func Build(h Header, sets []SourceSet) Show
func Classify(name string) Kind     // the ENT-1/ENT-3 decision on a name alone
func (s Show) Entries() []Entry      // all entries flattened, in order, skipped included
func (s Show) Matchable() int        // count of KindSong entries (spec §1 "matchable")
```

From setlist.fm data: `show := sl.Show()` where `sl` is a `setlistfm.Setlist` (e.g. from `Client.Setlist`). The mapping:

| `setlist.Show` | from `setlistfm.Setlist` |
|---|---|
| `ID`, `URL` | `ID`, `URL` |
| `Band` | `Artist.Name` |
| `Date` | `DisplayDate()` (`DD Mon YYYY`, or the raw `eventDate` if it doesn't parse) |
| `Venue`, `City` | `Venue.Name`, `Venue.City.Name` |
| `Tour` | `Tour.Name`, or `""` when `Tour` is nil |
| set `Name`, `Encore` | `Set.Name`, `Set.Encore` |
| entry `Name`, `Note`, `Tape` | `Song.Name`, `Song.Info`, `Song.Tape` |
| entry `CoverArtist` | `Song.Cover.Name`, or `""` when `Cover` is nil |

## Rules

### Classification (spec §5, applied in order §5.1 → §5.2 → §5.3)

All checks run on `normalize.Name(name)` (case-folded, accents removed, trimmed, internal whitespace collapsed).

1. **Skipped (ENT-1)** if the normalised name is `""`, `solo`, `jam` or `improvisation`, or ends with ` solo` (a space, then `solo`).
2. **Medley (ENT-3)** if the normalised name contains ` / ` (space, slash, space), or has `medley` as a whole word. Words are runs of letters and digits, so `Medley:` and `(Medley)` count but `Remedley` and `Medleys` don't.
3. Otherwise **song (ENT-5)**: songs, tapes, covers, and entries with any note.

Only the name decides the kind. The note, the tape flag and the cover never change it (ENT-5, ENT-6). A tape or cover named "Drum Solo" is still skipped, and one named "A / B" is still a medley.

### Expected artists (MATCH-1, MATCH-2)

- `KindSong` with no original artist → `[band]` (normal song, tape without `cover`).
- `KindSong` with an original artist → `[band, original]` (cover, including a tape with `cover`). `IsTape` is kept as setlist.fm says.
- Skipped and medley → empty (`nil`).
- `band` is `Header.Band` exactly as setlist.fm gives it. The original artist is `cover.name` trimmed.

### Headings (PREV-3)

For the set at index `i` (0-based) of the show:

1. `i == 0` → `""`. The first set is the main set and never has a heading, **even if setlist.fm names it**.
2. Otherwise setlist.fm's set `name`, trimmed, if it isn't blank.
3. Otherwise `Encore N` from the `encore` number, if it's > 0.
4. Otherwise `Set N`, where **N is the set's 1-based position among all the show's sets** (so the third set is `Set 3`, whatever kinds of sets come before it).

### Positions (ENT-2, ENT-7, PREV-3)

- Numbered 1…n **across the whole show** (not restarting per set), in setlist order, counting every entry that isn't skipped. Medleys get a number.
- Skipped entries stay in `Set.Entries` with `Position: 0`, so the full setlist is still there, but they must not be shown.
- Order and duplicates are kept as setlist.fm gives them. Duplicates are separate entries with their own positions.

## Edge-case decisions

- **"Jam Session", "Drum Jam", "Improvisation No. 2"** are songs. ENT-1 needs the *whole* name to be `jam` or `improvisation`; only `solo` has the `<anything> solo` form. "Solo Flight" is a song too (`solo` isn't the last word).
- **Punctuation isn't stripped for ENT-1**: "Drum Solo." or "Drum Solo (Mario)" aren't skipped, because the whole name isn't a solo. setlist.fm normally puts the player in the note, which doesn't count. Step 09's fixtures will show if this matters.
- **A cover credited to the band itself** (normalised name equal to the band's) or a blank `cover.name` is treated as **not a cover**. setlist.fm's `cover` means "if different to the performing artist", and `[band, band]` would only repeat the same search.
- **A slash without spaces on both sides** ("AC/DC", "A/ B") isn't a medley separator.
- **Whitespace inside a name** is collapsed before the checks, so "A  /  B" is a medley and "Bass  Solo " is skipped.
- A name that is both skipped and a medley (e.g. "Medley Drum Solo") is **skipped**, because §5.1 comes first.
- **A set whose entries are all skipped, or that has none**, is kept in `Show.Sets` with its heading. Step 10 decides whether to show a heading with no rows under it (recommended: drop such sets from the response, because PREV-3 hides skipped entries and an empty heading would look like a bug).

## Notes for later steps

- **Step 07:** `Resolve` can take a `setlist.Entry` (name in `Entry.Name`, artists in `Entry.ExpectedArtists`, in pass order). It should only be called for `KindSong`. The band pass is always `ExpectedArtists[0]`; a match from `ExpectedArtists[1]` is the cover fallback. Note that `Entry.Note` must not be passed to matching (ENT-6).
- **Step 08:** iterate `show.Sets` and `set.Entries` to keep setlist order and get each entry's `Heading`. Skip `KindSkipped` (no result row), pass `KindMedley` through as `not_found_medley` without searching, and search only `KindSong`. `Entry.Position` is already the preview's row number.
- **Step 10:** `summary.matchable` is `show.Matchable()`. The header fields map one to one (`Band`, `Date`, `Venue`, `City`, `Tour`, `URL`). See the empty-set decision above.
- **Step 11:** the playlist name and description need the band, date, venue and city. `sl.Show().Header` or `sl.DisplayDate()` both give the same date string.

Briefs 07, 08 and 10 were updated with these notes in this PR.

## Verification

`gofmt -l .` (nothing), `go vet ./...` and `go test -race ./...` pass. Tests (`internal/setlist/setlist_test.go`, `internal/setlistfm/show_test.go`):

- `TestEntry_ENT1_Skipped`: `""`, `"  "`, tab, "Drum Solo", "guitar solo", "Bass  Solo ", "Solo", "SOLO", "Jam", "Improvisation" → skipped; "Solo Flight", "Jam Session", "Drum Jam", "Improvisation No. 2", "Desolo" → song. Also: skipped even when it's a tape or cover; skipped wins over medley.
- `TestEntry_ENT2_…`: skipped entries get position 0, later ones continue the count, and the matchable count leaves them out.
- `TestEntry_ENT3_Medley`: "Medley: A / B", "A / B", "A  /  B", "Medley", "Old Songs Medley", "MEDLEY (Part 2)" → medley; "Remedley", "Medleys", "AC/DC", "A/ B", "A /B" → song.
- `TestEntry_ENT4_…`: medleys have no expected artists, are numbered, and aren't matchable.
- `TestEntry_ENT5_…`: tape, cover, and "snippet"/"partial"/"acoustic" notes → song.
- `TestEntry_ENT6_…`: a note that looks like a rule ("with drum solo; medley / snippet; acoustic") is carried through and changes neither the kind nor the expected artists, for songs, covers, medleys and skipped entries.
- `TestEntry_ENT7_…`: the same song three times, across sets → each kept in place with its own position.
- `TestEntry_MATCH1_…` / `TestEntry_MATCH2_…`: as listed under "Expected artists", including the cover credited to the band and the blank cover name.
- `TestShow_PREV3_…`: headings (named first set has none, name beats encore number, `Encore 2`, `Set 4`, blank name → `Set 5`, trimmed name) and positions running across sets.
- `TestSetlistShow…`: a full setlist.fm JSON document decoded and converted, with every field checked; no tour, an unparseable date and no sets.

## Known gaps

- Not run against live setlist.fm data. Step 09's fixtures are the first real check of the rules (e.g. how setlist.fm writes solos and medleys in practice).
