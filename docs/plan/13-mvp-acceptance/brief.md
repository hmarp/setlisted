# Step 13: MVP acceptance

- **Depends on:** step 09 (quality test), step 12 (UI)
- **Branch:** `step-13-mvp-acceptance`
- **Handoffs to read:** `docs/plan/09-matching-quality-tests/handoff.md`, `docs/plan/12-throwaway-ui/handoff.md`
- **Also read:** `docs/manual-test-checklist.md`

## Goal

Check the deployed MVP against every success criterion, record the evidence, fix small issues, and declare the MVP done, or list exactly what's left.

## Success criteria (quoted from `docs/spec.md` §9; the source is `docs/intent.md`)

| Intent success criterion | How it is checked |
|---|---|
| 1. Under a minute end to end | The owner times the journey for a typical headliner: open the app (logged in), search, pick a show, preview, create, open in Spotify. Under 60 seconds. |
| 2. Friends can use it | At least 2 friends, unaided, log in and create a playlist in their own library. |
| 3. Matching quality | On the owner's ~10-setlist test set (including at least one with covers, one with a tape and one with a medley): found / matchable ≥ 90% overall; zero tracks credited to the wrong artist (MATCH-3); every cover-fallback, live, tape, not-found and medley entry labelled as in PREV-4. |
| 4. Phone and laptop | The full journey is completed on the owner's phone and laptop, in the browsers they actually use. |
| 5. Access control | A non-allowlisted account sees the AUTH-4 message. A logged-out visitor can't reach any search, show, preview or creation screen (AUTH-1). |
| 6. Cost | One month's running cost is within the intent's budget. |
| 7. Real use | The owner has used it to prepare for at least one real gig. |

**Budget (intent, Constraints):** £0/month is the target, and a few pounds a month is the most acceptable.

## Owner involvement

Most of this step is the owner using the app and reporting results:
- the timed journey;
- onboarding friends (adding them to the Spotify allowlist, ADR-004/007);
- using it for a real gig;
- checking the billing console.

The session prepares the record sheet, runs the technical checks, and fixes small issues.

## What to do

1. **Create `docs/plan/13-mvp-acceptance/results.md`**, a table with one row per criterion: status (pass / fail / pending), the evidence, the date and notes.
2. **Criterion 3:** confirm the step 09 quality test passes on `main` in CI, and copy its results table into the record.
3. **Criteria 4 and 5:**
   - run `docs/manual-test-checklist.md` on the owner's phone and laptop against Cloud Run;
   - for access control, also check that a logged-out request to every `/api/*` route except health and session returns `401` (e.g. with `curl`).
4. **Criterion 1:** the owner times three runs, and the record shows the times. If they're over 60 seconds, find where the time goes (most likely the preview) and fix it if that's small, e.g. a cold start can be excluded or noted. Otherwise flag it.
5. **Criterion 2:** onboard at least 2 friends, then record that they managed unaided, and any confusion they hit.
6. **Criterion 6:** record the billing console figure for the first full month. If the month isn't complete yet, mark it **pending** with the date it will be.
7. **Criterion 7:** record the gig and date when it happens, or mark it **pending**.
8. **Fixes:**
   - small issues found → fix them in this PR (or a follow-up PR, also referencing step 13);
   - larger issues, or ideas → list them as **post-MVP candidates** in the handoff, alongside the intent's out-of-scope list. **Multi-band / line-up playlists** are first, followed by the UI redesign.

## Out of scope

- New features.
- The UI redesign (post-MVP, new ADR).

## Done when

- `results.md` shows criteria 1–5 as pass, and 6–7 as pass or pending with a date.
- No open issues are classed as MVP-blocking.

## End of step (same PR)

- Write `handoff.md`:
  - the acceptance summary;
  - anything still pending and when to recheck it;
  - the prioritised post-MVP candidate list;
  - lessons learned from the process (useful for the portfolio write-up).
- Mark step 13 done in `docs/plan.md`, with the plan status "MVP complete" (or "MVP complete, pending criteria 6/7").
- Update `README.md` to its MVP state:
  - what it is;
  - how to use it;
  - architecture summary with links to the ADRs;
  - how to run, test and deploy;
  - the process (intent → spec → ADRs → plan → steps);
  - the licence.
