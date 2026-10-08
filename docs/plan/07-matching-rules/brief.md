# Step 07: Matching rules (pure)

- **Depends on:** step 06
- **Branch:** `step-07-matching-rules`
- **Handoffs to read:** `docs/plan/05-setlistfm-search-shows/handoff.md` (normalisation), `docs/plan/06-entry-classification/handoff.md`

## Goal

Pure Go code that, given an entry, an expected artist and a list of Spotify candidate tracks, **chooses the right track or decides there isn't one**, exactly as spec §5.4 says. This is the heart of the product. A wrong match is worse than a missing one.

## Spec requirements (quoted from `docs/spec.md` §5.4)

**Who the expected artist is:**

- **MATCH-1** For a normal entry, and for a tape that setlist.fm doesn't attribute to another artist, the expected artist is the **band**.
- **MATCH-2** For a **cover** (including a tape attributed to another artist), matching runs in two passes:
  1. Look for the **band's own recording** (expected artist = band).
  2. If there isn't one, look for the **original artist's** recording (expected artist = original artist). A match from this pass is labelled as a **cover fallback**.

**What counts as a candidate.** A Spotify track is a candidate only if **both** of these are true:

- **MATCH-3 (artist)** One of the track's credited artists has a normalised name equal to the expected artist's normalised name.
- **MATCH-4 (title)** The track title equals the entry name after normalising both, which means:
  - ignoring case, accents and punctuation;
  - treating `&` and `and` as the same;
  - removing **version suffixes**, i.e. trailing parenthesised, bracketed or dash-separated markers such as "Remastered", "2015 Remaster", "Live", "Live at …", "Single Version", "Explicit", "Mono", "Stereo", "Deluxe", "‹year› Version".

  Any other difference means no candidate (e.g. "One" ≠ "One More Time").

**Exclusions:**

- **MATCH-5** A candidate is **excluded** if its version markers (title suffix or album title) identify it as a **demo, remix, instrumental, acoustic, karaoke or tribute** version.

**Choosing between candidates**, in order:

- **MATCH-6** Studio versions are preferred over **live** versions. A version is live if its title suffix contains "Live", or its album title contains "Live" as a word. A live version can be chosen only if no studio candidate exists, and it's labelled **Live version**.
- **MATCH-7** Among the remaining candidates: **album** releases come before **singles or EPs**, and singles or EPs come before **compilations**. (Spotify doesn't distinguish EPs from singles.)
- **MATCH-8** If there's still a tie, the **earliest release date** wins. This means re-recordings lose to originals but are used if they're the only version.

**No guesses:**

- **MATCH-9** If there is no candidate after the steps above, the entry is **Not found**. The app never falls back to a closest guess, a title-only search, or a different artist from the one expected.

*Known limitation (accepted):* an album whose title happens to contain "Live" (e.g. "Long Live Rock 'n' Roll") may have its tracks treated as live versions. This affects only which version is preferred, never whether the artist is correct.

**Definition (§1):** **Normalised name**: a name compared ignoring letter case, accents and leading/trailing spaces.

## Decisions this step relies on

- **ADR-005:** the matching rules are **pure Go with no I/O**. They take Spotify candidates as plain values. The Spotify client (step 08) is separate.
- **ADR-006:**
  - table-driven tests **named after spec IDs**, e.g. `TestMatch_MATCH6_StudioBeatsLive`;
  - every rule has at least one test, including the edge cases;
  - zero wrong-artist matches is a hard requirement of the later quality test (step 09).
- **Spotify track data available** (search results): track `id`, `name`, `artists[].name`, `album.name`, `album.album_type` (`album`, `single` or `compilation`), `album.release_date`, `album.release_date_precision` (`year`, `month` or `day`).

## What to build

**`internal/matching`:**

1. **`Candidate` type**, a plain value: ID, title, artist names, album name, album type, release date and precision.
2. **Title normalisation**, extending `normalize.Name` from step 05:
   - remove punctuation;
   - treat `&` and `and` as equal (e.g. normalise both to `and`);
   - **strip version suffixes**: trailing `(…)`, `[…]` or ` - …` segments, **only when** the segment contains a known version marker. Strip repeatedly, since there may be several. The marker list is: remaster, remastered, live, single version, explicit, mono, stereo, deluxe, a year followed by "version", radio edit, edit. The list lives in one place.

   A dash segment that isn't a version marker (e.g. a song genuinely called "Part 1 - The Beginning") is **kept**.

   **Live detection must use only the stripped suffixes and the album title**, never the core title, so "Live Wire" stays a studio song called "Live Wire".
3. **Version classification** of a candidate, from its stripped title suffixes and its album title (as whole words):
   - `excluded` if any marker is demo, remix, instrumental, acoustic, karaoke or tribute (MATCH-5);
   - otherwise `live` if a suffix contains "live", or the album title contains the word "live" (MATCH-6);
   - otherwise `studio`.
4. **`Choose(entryName, expectedArtist string, candidates []Candidate) (Candidate, isLive bool, ok bool)`:**
   - filter on MATCH-3 (normalised artist equality against **any** credited artist) and MATCH-4 (normalised title equality);
   - drop `excluded` (MATCH-5);
   - prefer `studio` over `live` (MATCH-6);
   - then album > single/EP > compilation (MATCH-7);
   - then the earliest release date (MATCH-8). Compare dates with year-only and month-only precisions treated as the start of that period. If still tied, keep the input order.
   - Nothing left → not found (MATCH-9).
5. **Two-pass helper** for MATCH-2, still pure:

   `Resolve(entry, candidatesFor func(expectedArtist) []Candidate) Result`

   It tries each expected artist in order and returns the first match. The result records whether the match came from the original artist (**cover fallback**) and whether it's **live**.

   The caller (step 08) supplies the candidates, so the network stays out of this package. If you prefer a different, equally pure, shape, record it in the handoff.

## Tests (named by spec ID; table-driven)

- **MATCH-3:**
  - the band among several credited artists → candidate;
  - a different artist with the same title → **not** a candidate;
  - accent and case differences (`Motorhead` vs `Motörhead`) → candidate;
  - `The Sword` vs `Sword` → not a candidate.
- **MATCH-4:**
  - "Seek & Destroy" vs "Seek and Destroy" → match;
  - "Blackened - Remastered 2021" → "Blackened";
  - "Battery (Live at Wembley)" → "Battery";
  - "Fade to Black (2016 Version)" → "Fade to Black";
  - "One" vs "One More Time" → no match;
  - "Live Wire" stays "Live Wire";
  - "Part 1 - The Beginning" keeps its dash segment;
  - punctuation and apostrophes ignored ("Don't" vs "Dont").
- **MATCH-5:** a demo, remix, instrumental, acoustic, karaoke or tribute marker in the title suffix or album → excluded; an excluded-only result → not found.
- **MATCH-6:**
  - studio beats live;
  - only live → live is chosen and flagged;
  - "Live" in a suffix → live;
  - album "Live After Death" → live;
  - album "Alive in Athens" → not live (whole-word rule);
  - "Long Live Rock 'n' Roll" → live (the documented limitation, tested so the behaviour is deliberate).
- **MATCH-7:** album beats single; single beats compilation.
- **MATCH-8:** earlier release wins; a re-recording loses to the original; a re-recording is used if it's the only one; mixed date precisions are compared correctly.
- **MATCH-9:** no candidates → not found; candidates only from other artists → not found.
- **MATCH-1 and MATCH-2 via `Resolve`:**
  - a normal song uses the band only;
  - a cover where the band has a recording → band's recording, not a fallback;
  - a cover with no band recording → original artist, cover fallback;
  - a cover with neither → not found.

## Out of scope

- Calling Spotify (step 08).
- Recorded real-data quality checks (step 09).
- API endpoints (step 10).

## Done when

- The tests pass in CI and cover MATCH-1 to MATCH-9 with the edge cases above.
- The package has no network or HTTP dependencies.
- The version-marker lists live in one documented place.

## End of step (same PR)

- Write `handoff.md`, including:
  - the public API of `internal/matching`;
  - the exact marker lists;
  - normalisation details;
  - any rule interpretation made (with spec ID);
  - anything step 08 must supply.
- Mark step 07 done in `docs/plan.md`.
- Update `README.md` if needed.
