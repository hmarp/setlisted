# Step 06: Entry classification rules

- **Depends on:** step 05, which provides the setlist.fm types and `normalize.Name`.
- **Branch:** `step-06-entry-classification`
- **Handoffs to read:** `docs/plan/01-go-skeleton/handoff.md`, `docs/plan/05-setlistfm-search-shows/handoff.md`

## Goal

Pure Go code, with no network or HTTP, that turns a fetched setlist into an ordered list of **classified entries**, ready for matching (step 08) and the preview (step 10). Every rule is tested by spec ID.

## Spec requirements (quoted from `docs/spec.md`)

**Definitions (§1)**

- **Entry:** One line of a show's setlist, as recorded on setlist.fm. It may be in a set or an encore and may carry a note.
- **Cover:** An entry that setlist.fm attributes to an artist other than the band (the *original artist*).
- **Tape:** An entry setlist.fm flags as played from tape.
- **Matchable entry:** An entry that isn't skipped (§5.1) and isn't a medley (§5.2).
- **Normalised name:** A name compared ignoring letter case, accents and leading/trailing spaces (e.g. `motorhead` = `Motörhead`).

**§5: Entry handling.** Each entry in the chosen show is classified, in this order.

*5.1 Skipped entries*

- **ENT-1** An entry is **skipped** if it has no name, or if its whole name (normalised) is a solo, jam or improvisation: `solo`, `<anything> solo` (e.g. "Drum Solo", "Guitar Solo"), `jam` or `improvisation`.
- **ENT-2** Skipped entries are left out of both the preview and the playlist and are not counted as matchable.

*5.2 Medleys*

- **ENT-3** An entry is a **medley** if its name contains the word "medley" (any case) or combines several song names separated by ` / `.
- **ENT-4** Medleys are shown in the preview as **Not found (medley)** and are not added to the playlist. They are not counted as matchable.

*5.3 Everything else*

- **ENT-5** Every other entry is matched according to §5.4. This includes tapes, covers, and entries with notes such as "snippet", "partial" or "acoustic".
- **ENT-6** setlist.fm **notes** on an entry are shown in the preview but **never affect matching**.
- **ENT-7** **Duplicates** are kept. If the same song appears more than once, each occurrence is matched and placed in its own position.

**Expected artist (§5.4, used to prepare matching input)**

- **MATCH-1** For a normal entry, and for a tape that setlist.fm doesn't attribute to another artist, the expected artist is the **band**.
- **MATCH-2** For a **cover** (including a tape attributed to another artist), matching runs in two passes:
  1. Look for the **band's own recording** (expected artist = band).
  2. If there isn't one, look for the **original artist's** recording (expected artist = original artist). A match from this pass is labelled as a **cover fallback**.

**Preview structure (§6, the parts this step prepares)**

- **PREV-3** Entries are listed in setlist order. Each set after the main set is introduced by a heading using setlist.fm's set name (e.g. "Encore 1"). Skipped entries are not shown.

## Decisions this step relies on

- **ADR-005:** classification and matching are **pure Go code with no I/O**. They take setlist entries (and later Spotify candidates) as plain values and return decisions. HTTP clients are kept separate.
- **ADR-006:**
  - table-driven tests **named after spec IDs**, e.g. `TestEntry_ENT1_DrumSoloSkipped`, `TestEntry_ENT3_SlashMedley`;
  - every rule in §5.1–5.3 has at least one test, including its edge cases.
- **setlist.fm data model:**
  - a song's `cover` field is "the original Artist of this song, if different to the performing artist", so no `cover` means the song is credited to the band;
  - `tape` is a boolean;
  - `info` is the note.
  - Sets have an optional `name` and an optional `encore` number.

## What to build

1. **`internal/setlist`** (or similar), domain types independent of setlist.fm's JSON:
   - `Show`: ID, band name, date, venue, city, tour, setlist.fm URL, and sets.
   - `Set`: a heading. It's empty for the first or main set. Otherwise it's setlist.fm's set `name`; if there's no name, `Encore N` from the `encore` number; and if neither exists, `Set N`. Record this fallback rule in the handoff.
   - `Entry`:
     - setlist position;
     - name;
     - note;
     - `IsTape`;
     - `OriginalArtist` (empty unless it's a cover);
     - `Kind`: `skipped`, `medley` or `song`;
     - `ExpectedArtists`: an ordered list. It's `[band]` for a normal song or an unattributed tape, and `[band, original]` for a cover (MATCH-1, MATCH-2). It's empty for skipped entries and medleys.
2. **A conversion** from the step 05 setlist.fm types to `Show`.
3. **Classification**, applied in spec order (§5.1 → §5.2 → §5.3):
   - **ENT-1:** empty or whitespace-only name; or a normalised name that is exactly `solo`, `jam` or `improvisation`; or that ends with ` solo`.
   - **ENT-3:** the name contains `medley` as a word, case-insensitively, or contains ` / ` (space, slash, space).
4. **Display positions.** Number the entries 1…n in setlist order, **counting only non-skipped entries**, because skipped entries aren't shown (ENT-2, PREV-3). Medleys get a number. Keep the original order and duplicates (ENT-7).
5. **Matchable count.** A helper that counts entries of kind `song` (the "matchable" definition).

## Tests (named by spec ID)

- **ENT-1:**
  - `""` and `"  "` → skipped;
  - "Drum Solo", "guitar solo", "Solo", "Jam" and "Improvisation" → skipped;
  - "Solo Flight" (a hypothetical song) → **not** skipped, because "solo" isn't the whole name or the final word;
  - "Jam Session" → not skipped (the whole name must be `jam`). Document this decision.
- **ENT-2:** skipped entries are excluded from the display positions and the matchable count.
- **ENT-3:** "Medley: A / B" → medley; "A / B" → medley; "Medley" → medley; "Remedley" (not a word match) → not a medley.
- **ENT-4:** medleys have no expected artists and aren't counted as matchable.
- **ENT-5:** a tape, a cover and a song with a "snippet" note → kind `song`.
- **ENT-6:** the note is carried through but doesn't change the kind or expected artists.
- **ENT-7:** the same song twice → two entries in their original positions.
- **MATCH-1:** a normal song and a tape without `cover` → `[band]`.
- **MATCH-2:** a cover → `[band, original]`; a tape with `cover` → `[band, original]`, with `IsTape` true.
- **PREV-3 headings:** main set has no heading; a named encore uses its name; an unnamed encore becomes `Encore N`; and `Set N` when neither exists.

## Out of scope

- Spotify matching rules (step 07) and Spotify calls (step 08).
- API endpoints (step 10).

## Done when

- The tests pass in CI, covering every requirement listed above.
- The package has no network or HTTP dependencies.

## End of step (same PR)

- Write `handoff.md`, including:
  - the domain types and how to build them from setlist.fm data;
  - the heading fallback rule;
  - the position numbering rule;
  - edge-case decisions (e.g. "Jam Session").
- Mark step 06 done in `docs/plan.md`.
- Update `README.md` if needed.
