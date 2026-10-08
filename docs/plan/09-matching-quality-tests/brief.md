# Step 09: Test setlist fixtures and matching-quality test

- **Depends on:** step 05 (setlist.fm client), step 08 (pipeline and searcher interface)
- **Branch:** `step-09-matching-quality-tests`
- **Handoffs to read:** `docs/plan/05-setlistfm-search-shows/handoff.md`, `docs/plan/06-entry-classification/handoff.md`, `docs/plan/07-matching-rules/handoff.md`, `docs/plan/08-spotify-search-pipeline/handoff.md`

## Goal

A repeatable, offline check of **matching quality on real data**, which is MVP success criterion 3. About 10 real setlists are recorded as fixtures, each entry gets an expected outcome reviewed by the owner, and a test fails on any wrong-artist match, a score below 90%, or a wrong label.

## Decide first: Spotify data in a public repo

The repository is **public**. **Before committing any recorded Spotify responses**, check Spotify's current Developer Terms and Developer Policy on storing and redistributing content obtained from the API.

- **Proposed approach:** trim the fixtures to **only the fields the matcher reads** (track ID, title, artist names, album name, album type, release date and precision), with no images, previews, popularity or other fields.
- **Present the finding to the owner and get a decision before committing fixtures.** If the terms don't allow committing even trimmed data, **stop and flag it**. A new ADR would then supersede the fixture part of ADR-006, for example with fixtures kept outside the repo (git-ignored and recorded locally), or with hand-written test data.
- setlist.fm data: check its API terms for redistribution too, and keep the setlist fixtures to the fields we use.

## Owner input needed

1. **Choose about 10 recent metal setlists** (setlist.fm URLs or IDs). They should include at least:
   - one with **covers**;
   - one with a **tape** intro or outro;
   - one with a **medley**.

   Ideally they cover a mix of eras and catalogue sizes (a band with many live albums tests MATCH-6 hard).
2. **Review the expected outcomes** drafted by the session (see below). This review is what makes the test meaningful. Pay particular attention to covers, live and studio choices, and re-recordings.

## Spec requirements and criteria (quoted)

- **Success criterion 3 (spec §9):** "On the owner's ~10-setlist test set (including at least one with covers, one with a tape and one with a medley): found / matchable ≥ 90% overall; zero tracks credited to the wrong artist (MATCH-3); every cover-fallback, live, tape, not-found and medley entry labelled as in PREV-4."
- **Matchable entry (§1):** "An entry that isn't skipped (§5.1) and isn't a medley (§5.2)."
- **MATCH-3 (artist):** "One of the track's credited artists has a normalised name equal to the expected artist's normalised name."
- **PREV-4 statuses:**
  - Found;
  - Found, cover fallback ("Cover → ‹original artist›");
  - Found, live version ("Live version");
  - Not found or Not found (medley);
  - and tape entries additionally carry a "Tape" label, whatever their status.

## Decisions this step relies on

**ADR-006: matching-quality tests.**

- The ~10 test setlists are stored as **recorded fixtures**, real API responses committed under a test-data directory (subject to the decision above), with public catalogue data only, **never tokens, keys or user data**.
- Each setlist has an **expected-outcomes file** giving, per entry: skipped, medley, found (with the expected Spotify track), cover fallback, live version, tape or not found.
- **Claude drafts** the expected outcomes, and the **owner reviews and corrects** them before they're treated as correct.
- The test runs the full pipeline **offline**. It:
  - **fails** on any wrong-artist match, with no exceptions;
  - **fails** if found / matchable is below 90%;
  - **fails** on any label that differs from the expected outcome;
  - **reports** per-setlist and overall results.
- **Live calls are opt-in only.** A separate command behind a **Go build tag** calls the live APIs (with keys from the local environment) to record or refresh fixtures. It's run by hand, never in CI.

**ADR-005:** if studio versions are being pushed out of the 10 search results, fetching a second page of results is the first remedy to try.

**Step 08:** the pipeline takes an explicit market and a searcher interface. The recorder uses an **app token** (Spotify client-credentials flow with `SPOTIFY_CLIENT_ID` and `SPOTIFY_CLIENT_SECRET`) and an explicit market (e.g. `GB`), so no user login is needed.

## What to build

1. **The recorder:** `cmd/recordfixtures` (or similar), with the build tag `live`. Given setlist IDs, it:
   - fetches each setlist from setlist.fm;
   - runs the **same searches the pipeline would make** (both passes for covers, so the fallback data exists even when the band pass finds a match);
   - saves trimmed JSON fixtures to `testdata/quality/<setlist-id>/`.

   It never writes tokens or keys. Usage goes in the README.
2. **A fixture searcher** that implements step 08's searcher interface from the recorded files, keyed by the exact query (track and artist). A query with no recording is a **test error**, not "no results", so missing data can't pass silently.
3. **Expected outcomes:** `testdata/quality/<setlist-id>/expected.json`. Per entry:
   - position;
   - entry name;
   - expected status;
   - expected Spotify track ID (for found statuses);
   - an optional `comment` (e.g. "studio original on *Master of Puppets*").

   Draft these from the recorded data and the spec rules. Where the draft is a judgement call, add a comment that flags it for the owner.
4. **The quality test** (a normal `go test`, no build tag, runs in CI):
   - runs the pipeline on every fixture;
   - compares each entry against the expected outcome;
   - fails on any matched track whose artists don't include the expected artist (re-checked independently of the matcher), on any status or label mismatch, or if the overall found / matchable is below 0.90;
   - prints a per-setlist and overall table (found, matchable, percentage, mismatches) with `t.Log`.
5. **If the test exposes rule problems:**
   - **flag them** to the owner, with examples;
   - fix them in this step **only if small and clearly within the spec**, e.g. a missing version marker that the spec's "such as" list covers;
   - anything that changes the spec's meaning is flagged for a spec change and not done quietly.

   The step is done when the test passes on the **owner-reviewed** expectations.

## Out of scope

- Preview and create endpoints (steps 10, 11).
- Changing the matching rules beyond small, spec-consistent fixes.

## Done when

- The decision about Spotify data in a public repo is recorded in the handoff (and in a new ADR if needed).
- About 10 owner-chosen setlists are recorded, with expectations reviewed and approved by the owner.
- The quality test passes in CI, with no network access, and reports ≥ 90% found / matchable and zero wrong-artist matches.

## End of step (same PR)

- Write `handoff.md`, including:
  - the fixture format;
  - how to record and refresh;
  - the decision about public data;
  - the quality results table;
  - rule issues found and how they were resolved or flagged.
- Mark step 09 done in `docs/plan.md`.
- Update `README.md` with a "Matching-quality tests" section: what they check, and how to refresh the fixtures.
