# Step 09 handoff: test setlist fixtures and matching-quality test

## What was built

- **`internal/quality`**: the fixture format, a fixture searcher implementing `pipeline.Searcher`, and `Evaluate`, which runs the pipeline offline on one fixture and compares the results with the expected outcomes.
- **`TestMatchingQuality`** (`internal/quality/quality_test.go`): a normal `go test` (no build tag, runs in CI, no network) over every setlist in `testdata/quality/`.
- **`cmd/recordfixtures`** (build tag `live`): records or refreshes fixtures from the live setlist.fm and Spotify APIs. Run by hand, never in CI.
- **Six owner-chosen setlists** recorded under `testdata/quality/<setlist-id>/`, each with an owner-approved `expected.json`.
- **Two matching rule changes** (see "Rule problems"): featured-artist credits are stripped from titles (an owner-approved spec change to MATCH-4), and "‹year› Recording" is a version marker.
- No new dependencies, endpoints, configuration or cloud resources.

## Decision: recorded API data in the public repo

The brief asked for Spotify's and setlist.fm's terms to be checked before committing fixtures. The build sandbox couldn't reach `developer.spotify.com` or `setlist.fm`, so the check relied on second-hand quotations found by web search:

- **Spotify Developer Terms** (v9, effective 8 May 2023, as quoted on the Spotify community forum): "Spotify Content" is *any content, data, information or material made available through the Spotify Platform*; section 4 forbids *"storing, aggregating, or building compilations or databases, other than as strictly necessary to operate your SDA. Do not store Spotify Content indefinitely."* The **Developer Policy** (effective 15 May 2025) forbids *offering metadata … as a standalone service or product*.
- **setlist.fm API terms** (as summarised by third-party projects): free for non-commercial use, results must link back to setlist.fm, and *persistent caching* is reportedly not allowed. Nothing specific on redistribution was found.

Committing even trimmed responses to a public repo isn't clearly allowed by that wording. Three options were put to the owner: (A) commit trimmed fixtures, (B) keep them git-ignored and local, and (C) keep them in a private repo fetched by CI. **The owner chose A (2026-11-03)**, accepting the risk for a small non-commercial project: B would have left the test out of CI and out of reach of cloud sessions, and C adds a repo and a secret. ADR-006 already proposes committed fixtures, so no new ADR was needed. Mitigations:

- fixtures hold only the fields the app reads: setlist.fm's setlist as `setlistfm.Setlist` decodes it, and per Spotify track the ID, title, artist names, album name, album type, release date and precision. No images, previews, popularity, URIs or user data;
- about 236 KB for six setlists; each setlist's `setlist.json` includes its setlist.fm `url`;
- re-recording (below) replaces the data, e.g. before step 13.

If Spotify or setlist.fm objects, the fallback is option B or C, recorded in a new ADR superseding the fixture part of ADR-006.

## Fixture format and layout

```
testdata/quality/<setlist-id>/
  setlist.json    the setlist.fm setlist (setlistfm.Setlist JSON)
  searches.json   every search the pipeline can make for it
  expected.json   the owner-reviewed expected outcome of each shown entry
```

`searches.json` (written by the recorder):

```json
{
  "market": "GB",
  "recorded": "2026-11-03",
  "searches": [
    {
      "track": "Lazarus", "artist": "Steven Wilson",
      "query": "track:\"Lazarus\" artist:\"Steven Wilson\"",
      "results": [
        {"id": "…", "title": "Lazarus (2015 Recording)", "artists": ["Steven Wilson"],
         "album": "Transience", "albumType": "album", "releaseDate": "2016-09-16", "releaseDatePrecision": "day"}
      ]
    }
  ]
}
```

- `track` and `artist` are the **raw pair the pipeline passes** (`Entry.Name` as setlist.fm gives it, and `Header.Band` or the trimmed cover name). `query` is the `q=` the live client sent, for reference only.
- **Both passes of every cover are recorded**, so the fallback data exists even when the band's pass finds a match. A repeated song is recorded once.
- `results` is in Spotify's order; `[]` means Spotify returned nothing.
- Files are decoded with unknown fields rejected, so a typo in a hand edit fails loudly.

`expected.json` (written by hand, never touched by the recorder):

```json
{
  "setlistId": "b4b714a",
  "comment": "…",
  "entries": [
    {"position": 9, "name": "Lazarus", "status": "found", "trackId": "1TUdpDRzVEp3SGAnmoZzJD", "comment": "…"},
    {"position": 1, "name": "War Pigs", "status": "cover_fallback", "tape": true, "trackId": "…"}
  ]
}
```

- One entry per **shown** entry (skipped entries have none, as in the preview), in setlist order.
- `status` is a `pipeline.Status` (`found`, `cover_fallback`, `live`, `not_found`, `not_found_medley`). `live` and `tape` are the PREV-4 labels (omitted when false). A live cover fallback has `"status": "cover_fallback", "live": true`.
- `trackId` is required exactly for the found statuses.
- `comment` explains the chosen track and flags judgement calls (`JUDGEMENT CALL:`).

## The fixture searcher

`quality.NewSearcher(searches)` returns a `*quality.Searcher`, a `pipeline.Searcher` that:

- looks up the exact `(track, artist)` pair (no normalisation, so it checks the pipeline searches with exactly what was recorded);
- returns a copy of the recorded candidates, `(nil, nil)` for a recorded empty result, and an error wrapping **`quality.ErrNoRecording`** for a pair that wasn't recorded. That error fails the whole match, so missing data can't pass as "not found";
- refuses a market other than the recorded one, and a pair recorded twice;
- is read-only after loading, so it's safe for the pipeline's 4 concurrent searches (tested under `-race`).

## The quality test

`TestMatchingQuality` loads every directory under `testdata/quality/` with a `setlist.json`, runs `pipeline.MatchShow` with the fixture searcher and the recorded market, and fails on:

- **any wrong-artist match**: a found track none of whose credited artists has the expected artist's normalised name (the band, or the original artist for a cover fallback). This is re-checked with `normalize.Name` directly, independently of `internal/matching`;
- **any difference from `expected.json`**: status, `live`, `tape` or track ID, a different entry count, or an entry whose position or name doesn't line up, plus problems in `expected.json` itself (a found status without a track ID, `live` status without `live: true`, a wrong `setlistId`);
- **overall found / matchable below 0.90** (spec §9 criterion 3). Found = any found status; matchable = `show.Matchable()` (not skipped, not a medley);
- a fixture that can't be run (missing recording, unreadable file) or no fixtures at all.

It logs a per-setlist and overall table with `t.Log` (run with `-v` to see it). `Evaluate` and the searcher have their own unit tests on a small in-memory fixture (`internal/quality/fixture_test.go`).

`-quality.draft=DIR` (e.g. `go test ./internal/quality -run TestMatchingQuality -quality.draft=/tmp/draft`) also writes each setlist's **actual** results as a draft `expected.json` into `DIR`, never into the fixtures. It's a starting point after re-recording; every entry still has to be reviewed against the raw searches and the spec. Pass it with a single package path; `go test ./...` would hand the flag to packages that don't define it.

## Recording and refreshing

The sandbox can't reach either API, so the owner records locally (Git Bash on Windows works), with the real keys in `.env`:

```bash
go run -tags live ./cmd/recordfixtures <setlist.fm setlist URL or ID>...
```

- Flags: `-market GB` (default), `-out testdata/quality` (default).
- Reads `SETLISTFM_API_KEY`, `SPOTIFY_CLIENT_ID` and `SPOTIFY_CLIENT_SECRET` from the environment or `.env`, and gets an **app token** with the client-credentials flow (no login). `market=from_token` doesn't work with an app token, hence `-market`.
- Uses the app's own clients (`setlistfm.Client` with its rate limiter, `spotify.Client.SearchTracks`), with a 150 ms pause between searches.
- Prints one line per setlist with its matchable, cover, tape, medley and skipped counts and the number of searches.
- Replaces `setlist.json` and `searches.json`; never touches `expected.json`. Never prints or writes keys or tokens. After recording, `grep -rniE "bearer|access_token|client_secret|x-api-key" testdata/quality` should print nothing.

After a refresh, run the quality test: Spotify's catalogue changes, so differences are expected. Review each one against the new `searches.json`, update `expected.json`, and get the owner's approval again.

## Results (owner-approved expectations, 2026-11-03)

Recorded 2026-11-03, market `GB`.

| Setlist | Show | Found | Matchable | % | Wrong artist | Mismatches |
|---|---|---|---|---|---|---|
| `3343e88d` | Judas Priest, Catton Hall, 09 Aug 2026 | 19 | 20 | 95.0 | 0 | 0 |
| `3b4e249c` | Slayer, Texas Motor Speedway, 24 Oct 2026 | 13 | 13 | 100.0 | 0 | 0 |
| `3b97d8a4` | Bring Me the Horizon, AFAS Live, 20 Nov 2018 | 15 | 15 | 100.0 | 0 | 0 |
| `4b48efd6` | Metallica, Sphere, 31 Oct 2026 | 17 | 18 | 94.4 | 0 | 0 |
| `b472d42` | Sanguisugabogg, Russell Industrial Center, 31 May 2025 | 7 | 7 | 100.0 | 0 | 0 |
| `b4b714a` | Steven Wilson, Royal Albert Hall, 29 Oct 2026 | 8 | 10 | 80.0 | 0 | 0 |
| **Overall** | | **79** | **83** | **95.2** | **0** | **0** |

Coverage: 6 covers (4 setlists), 6 tapes (3 setlists), 1 medley (Bring Me the Horizon), and examples of `found`, `cover_fallback` (with and without tape), `not_found` (with and without tape) and `not_found_medley`. There's **no `live` entry** in the final expectations (Lazarus was `live` before the marker fix); `live` is covered by the pipeline's and matcher's unit tests only.

The 4 not found: Judas Priest's "Faithkeepers Intro" tape (not on Spotify), Metallica's "Kirk and Rob Doodle" (an interlude, not a song), and Steven Wilson's "Home Invasion" and "Regret #9" (on Spotify only as one combined track).

**The test set is 6 setlists, not the spec's "~10".** The owner chose this (2026-11-03): five recent shows chosen for the mix of eras and catalogues, plus Bring Me the Horizon (2018) for a medley. Step 13 should note it when recording criterion 3.

## Rule problems found

### Fixed in this step

1. **Featured-artist credits** (owner-approved spec change). "Face Ripped Off (feat. Aaron Heard)" (Sanguisugabogg) and "wonderful life (feat. Dani Filth)" (Bring Me the Horizon) were Not found, because a `feat.` suffix isn't a version marker. MATCH-4 in `docs/spec.md` now also removes a trailing parenthesised, bracketed or dash-separated featured-artist credit ("feat. …", "ft. …", "featuring …"); MATCH-3 still applies. Code: `featuredMarkers` in `internal/matching/markers.go`, checked as the **first word** of a suffix segment (`isFeatured` in `title.go`), so "(Lift Me Up)" or "(Song ft Someone)" aren't stripped, nor is a bare "Song feat. X" with no bracket or dash. Tests in `TestMatch_MATCH4_Title`.
2. **"‹year› Recording"** is a version marker, like the spec's "‹year› Version" (`yearVersionWords` in `markers.go`). Steven Wilson's studio re-recording "Lazarus (2015 Recording)" wasn't a candidate, so his live version was chosen. Within the spec's "such as" list; no spec change.

### Flagged, not changed

- **Medleys aren't matched** (Bring Me the Horizon's old-songs medley): by design. ENT-4 and the intent ("Splitting medleys into their songs" is out of scope). The owner would have liked it matched; kept for the MVP and listed as a post-MVP candidate.
- **Two setlist entries that are one Spotify track** ("Home Invasion" and "Regret #9" vs "Home Invasion / Regret #9"): Not found under MATCH-4 and MATCH-9. Matching them would need a new rule and a new preview status (e.g. "included in the previous track"). The owner would have liked them matched; kept for the MVP and listed as a post-MVP candidate.
- **Non-song interludes** ("Kirk and Rob Doodle"): ENT-1 skips only solo, jam and improvisation, so it's matchable and Not found. Adding "doodle" would be a spec change; not proposed for the MVP.
- **The band's own recording of a tape beats the original** (Metallica's "The Ecstasy of Gold": Metallica's cover on a Morricone tribute compilation is chosen, while the tape played is Morricone's). Correct under MATCH-2; accepted by the owner.
- **"‹anything› Version"** isn't a marker: "War Pigs - Charity Version" (Judas Priest with Ozzy Osbourne) isn't a candidate, so the fallback picks Black Sabbath's original, which is what's played. Stripping all "… Version" suffixes would change that; not proposed.
- **Ties after the release date** follow Spotify's order, which sometimes picks a box-set copy of the same recording (Metallica's "The Outlaw Torn", "Enter Sandman") or an "Expanded Edition" (Judas Priest). Same recording; no change.
- **ADR-005's second-page remedy isn't needed**: no studio version was pushed out of the 10 results in these setlists.

### Seen in the data, worth knowing

- An anniversary edition can be dated **earlier** than the original ("Hell Awaits (40th Anniversary Edition)" 1985-04-08 vs 1985-06-19). It's not a candidate only because "Anniversary Edition" isn't a marker; adding such a marker would make MATCH-8 choose it.
- Unmarked suffixes keep many unwanted versions out by accident: "Alternate Take", "Rough Mix", "Work In Progress", "From James' Riff Tapes", "Original 1976 Masters", "Orchestral Version". Adding markers needs care for that reason.
- setlist.fm uses `[traditional]` as a cover artist ("Whiskey in the Jar"); its normalised name doesn't match Spotify's "Traditional", so a fallback search for it can never succeed. Metallica's own recording was found first here.
- Steven Wilson's set names are "Set 1:" and "Set 2: The Overview" (step 06 shows the first set without a heading, and the second as "Set 2: The Overview").
- Spotify credits `(with X)` the same way as `feat.` on some tracks. Not handled (see known gaps).

## Notes for later steps

- **Step 10:** no change to the pipeline's API. MATCH-4 now also strips featured-artist credits, which is inside `internal/matching`.
- **Step 13:** criterion 3 is checked by `TestMatchingQuality` in CI. Copy the table above (or re-run with `go test ./internal/quality -run TestMatchingQuality -v`) into `results.md`, and note that the test set is 6 setlists by the owner's choice. Post-MVP candidates from this step: splitting medleys (intent Out #4), matching consecutive entries to one combined Spotify track, and skipping interludes such as "doodle". Brief 13 is updated.

## Verification

`gofmt -l .` (nothing), `go vet ./...`, `go vet -tags live ./...` and `go test -race ./...` all pass; `TestMatchingQuality` passes with the table above. The recorder's URL parsing is tested under the `live` tag (`go test -tags live ./cmd/recordfixtures`). The owner ran the recorder locally on 2026-11-03 (six setlists, 89 searches, no errors).

## Known gaps

- **The terms check used second-hand quotations**; the official pages couldn't be read from the sandbox. The owner accepted the risk (above).
- **6 setlists, not ~10** (owner's choice). Only one medley, and no live-only match in the final expectations.
- **`(with X)` credits aren't stripped**: "with" also starts real subtitles ("(With Love)"), so it wasn't added. A track titled "Song (with X)" is still Not found.
- The fixtures are a snapshot: Spotify's catalogue and setlist.fm's edits will drift from them. The test checks the rules against that snapshot, not against today's Spotify.
- `-quality.draft` writes drafts from the pipeline's own output, so a draft can't find a rule problem by itself; the review against the raw searches does that.
