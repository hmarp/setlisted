# Step 13 handoff: MVP acceptance

## Acceptance summary

The deployed MVP was checked against every success criterion in [`spec.md`](../../spec.md) §9. The evidence is in [`results.md`](results.md).

| # | Criterion | Status |
|---|---|---|
| 1 | Under a minute end to end | **pass** (owner, 2026-11-10: three runs, each under 60 s) |
| 2 | Friends can use it | **pass** (owner, 2026-11-10: two friends, unaided) |
| 3 | Matching quality | **pass** (`TestMatchingQuality`: 79 / 83 = 95.2%, 0 wrong artists, 0 mismatches; in CI on `main`) |
| 4 | Phone and laptop | **pass** (owner, 2026-11-10: the manual checklist on both) |
| 5 | Access control | **pass** (`curl`: every protected `/api/*` route `401` logged out, locally and on Cloud Run; owner: AUTH-4 message and logged-out screens) |
| 6 | Cost | **pass**, owner-accepted (2026-11-10, on a partial month; no figure recorded) |
| 7 | Real use | **pass** (owner, 2026-11-10; gig details not recorded) |

**The MVP is complete** (owner's decision, 2026-11-10). All seven criteria pass. No open issue is MVP-blocking. No code changed in this step: the checks found nothing to fix.

## Criteria 6 and 7

Both were first recorded as pending (PR #16). The owner then reported both as fine and asked for the MVP to be declared complete, without recording the figures (2026-11-10):

- **Cost:** checked before November 2026, the first full month, had ended. The £1 budget alert (step 02) stays as the safeguard. Worth a glance at December's bill.
- **Real use:** the owner used it for a real gig.

Criteria 1, 2 and 4 were also reported as passed without the individual figures.

## Decisions taken in this step

- **Owner-reported results are recorded as such.** Criteria 1, 2, 4 and the browser part of 5 rest on the owner's report of 2026-11-10. Criteria 3 and the API part of 5 were checked by the session.
- **`POST` without `Origin` answers `403`, not `401`.** The same-origin check runs before the session check, as [`docs/api.md`](../../api.md) documents. The request is refused either way, and a browser always sends `Origin`, so this meets AUTH-1. No change.
- **Logged out, unknown `/api/` routes and wrong methods answer `401`**, not `404`/`405` (the session middleware wraps the whole `/api/` tree). Recorded as intended: it doesn't reveal the API's shape.
- **The test set stays at 6 setlists** (owner's choice, step 09), recorded in the notes for criterion 3.

## Post-MVP candidates (prioritised)

Each needs the usual stages (spec change, ADR where the stack or design changes, plan).

1. **Multi-band / full line-up playlists** (headliner plus supports). Intent Out #1, the first candidate after the MVP.
2. **UI redesign** (intent Out #13), with a new ADR superseding [ADR-002](../../adr/002-throwaway-static-ui.md). Fold in step 12's rough edges:
   - the confirmation isn't in the hash;
   - a clear-match search isn't in history;
   - labels are text only;
   - focus isn't moved on view changes;
   - search text is percent-encoded in the address bar.
3. **Matching ideas from step 09:**
   - splitting medleys into their songs (intent Out #4);
   - matching consecutive setlist entries to one combined Spotify track ("Home Invasion" + "Regret #9"), which needs a new rule and preview status;
   - skipping non-song interludes such as "Kirk and Rob Doodle" (an ENT-1 change);
   - stripping `(with X)` credits safely;
   - a `[traditional]` cover artist that can never match Spotify's "Traditional".
4. **A bigger quality test set** (~10 setlists as the spec says, with a `live` entry and more medleys), and a periodic re-record to catch Spotify catalogue drift.
5. **The intent's other out-of-scope items**, unordered:
   - editing the playlist;
   - choosing a different version of a track;
   - a "typical setlist";
   - older shows;
   - syncing an existing playlist;
   - search by venue, city, festival or date;
   - other streaming services.

   History, native apps and in-app user management stay out by design.
6. **Housekeeping:**
   - add `-race` to CI's `go test` (it runs without it today; Windows machines without a C compiler can't run it locally);
   - read the official Spotify and setlist.fm terms pages directly. Step 09 used second-hand quotations.

## Lessons learned (for the portfolio write-up)

- **Intent → spec → ADRs → plan → steps worked.** Self-contained briefs let each session start cold, and handoffs carried decisions forward. The cost was keeping briefs in step with the spec when a step changed a rule (MATCH-4 in step 09).
- **Deploying the skeleton first (step 02) and automating deploys early (step 03)** meant hosting, secrets and HTTPS were never in the way later.
- **Pure rules before clients (steps 06–07), then a fixture-based quality test (step 09),** turned "is the matching good?" into a number in CI. The test found two real rule problems that unit tests had missed.
- **External constraints shaped the design more than code did:** Spotify's development mode (allowlist, Premium), its terms on storing data, and setlist.fm's non-commercial terms and rate limit.
- **Acceptance needs a human.** Timing, friends and real use can't be automated, and owner-reported results need recording with the same care as test output.

## Verification in this session

- `go test ./internal/quality -run TestMatchingQuality -v` passes locally; `TestMatchingQuality` passed in the `ci / test` job of the Deploy run for `5336235` on `main`.
- `curl` checks of every `/api/*` route without a session, against `http://127.0.0.1:8080` and the Cloud Run URL (table in `results.md` §5).
- `gofmt -l .` prints nothing; `go vet ./...`, `go vet -tags live ./...` and `go test ./...` pass. **`go test -race ./...` couldn't run locally:** there's no C compiler, and Docker wasn't running. CI doesn't use `-race` either, and this step changed no Go code.

## Known gaps

- Criteria 6 and 7 were accepted by the owner without recorded figures; criterion 6 was checked before a full month had elapsed.
- Criteria 1, 2 and 4 are owner-reported without individual times or per-section details.
- `-race` not run in this session.
