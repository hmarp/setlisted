# Step 13: MVP acceptance results

One row per success criterion from [`spec.md`](../../spec.md) §9 (the source is [`intent.md`](../../intent.md)). Status is **pass**, **fail** or **pending**. The details for each row are below the table.

| # | Criterion | Status | Evidence | Date | Notes |
|---|---|---|---|---|---|
| 1 | Under a minute end to end | **pass** | Owner's three timed runs, each under 60 s ([details](#1-under-a-minute-end-to-end)) | 2026-11-10 | Owner-reported; individual times to be added |
| 2 | Friends can use it | **pass** | Two friends logged in and created a playlist unaided ([details](#2-friends-can-use-it)) | 2026-11-10 | Owner-reported |
| 3 | Matching quality | **pass** | `TestMatchingQuality`: 79 / 83 = 95.2%, 0 wrong artists, 0 label or track mismatches. Passes locally and in CI on `main` ([details](#3-matching-quality)) | 2026-11-10 | Test set is **6 setlists, not ~10** (owner's choice, step 09) |
| 4 | Phone and laptop | **pass** | Manual checklist passed on the owner's phone and laptop ([details](#4-phone-and-laptop)) | 2026-11-10 | Owner-reported |
| 5 | Access control | **pass** | Every protected `/api/*` route returns `401` logged out, locally and on Cloud Run; a non-allowlisted account saw the AUTH-4 message; logged-out screens show only the login view ([details](#5-access-control)) | 2026-11-10 | Browser part owner-reported |
| 6 | Cost | pending until **2026-12-02** | Billing figure for November 2026, the first full month since the first deploy (2026-10-27) ([details](#6-cost)) | | Budget: £0 target, a few pounds at most |
| 7 | Real use | pending until the owner's next gig | A real gig ([details](#7-real-use)) | | Recheck at the next gig the owner goes to |

## 1. Under a minute end to end

A typical headliner: open the app (already logged in), search, pick a show, preview, create, open in Spotify. Timed by the owner on Cloud Run. Pass: each run under 60 s.

**Owner's report (2026-11-10): all three runs passed, each under 60 s.** The individual times weren't given; the table is for them.

| Run | Device and browser | Band and show | Total (s) | Preview wait (s) | Notes |
|---|---|---|---|---|---|
| 1 | | | | | |
| 2 | | | | | |
| 3 | | | | | |

## 2. Friends can use it

At least 2 friends, unaided, log in and create a playlist in their own library.

**Owner's report (2026-11-10): passed.** Two friends were added to User Management, logged in and created a playlist in their own library unaided. No confusion was reported. Friends are identified by letter only.

| Friend | Added to User Management | Logged in | Playlist created | Unaided? | Device and browser | Confusion or problems |
|---|---|---|---|---|---|---|
| A | | | | | | |
| B | | | | | | |

## 3. Matching quality

`go test ./internal/quality -run TestMatchingQuality -v`, run locally on 2026-11-10 on `origin/main` (`5336235`). The same test ran (and passed) in CI on `main` in the [Deploy run for `5336235`](https://github.com/hmarp/setlisted/actions/runs/42892255332) (`ci / test`, `ok github.com/hmarp/setlisted/internal/quality`).

| Setlist | Band | Date | Found | Matchable | % | Wrong artist | Mismatches |
|---|---|---|---|---|---|---|---|
| `3343e88d` | Judas Priest | 09 Aug 2026 | 19 | 20 | 95.0 | 0 | 0 |
| `3b4e249c` | Slayer | 24 Oct 2026 | 13 | 13 | 100.0 | 0 | 0 |
| `3b97d8a4` | Bring Me the Horizon | 20 Nov 2018 | 15 | 15 | 100.0 | 0 | 0 |
| `4b48efd6` | Metallica | 31 Oct 2026 | 17 | 18 | 94.4 | 0 | 0 |
| `b472d42` | Sanguisugabogg | 31 May 2025 | 7 | 7 | 100.0 | 0 | 0 |
| `b4b714a` | Steven Wilson | 29 Oct 2026 | 8 | 10 | 80.0 | 0 | 0 |
| **Overall** | | | **79** | **83** | **95.2** | **0** | **0** |

- **Pass:** found / matchable ≥ 90% overall (95.2%); zero wrong-artist matches (MATCH-3); every cover-fallback, live, tape, not-found and medley label as expected (PREV-4), against the owner-approved `expected.json` files (2026-11-03).
- **6 setlists, not the spec's ~10**, by the owner's choice (step 09): covers in 4 setlists, tapes in 3, one medley (Bring Me the Horizon). There's no `live` entry in the expectations; `live` is covered by unit tests only.
- The fixtures were recorded on 2026-11-03 and haven't been re-recorded, so the owner's approval still holds.

## 4. Phone and laptop

[`docs/manual-test-checklist.md`](../../manual-test-checklist.md) on Cloud Run, on the owner's phone and laptop in the browsers they actually use.

**Owner's report (2026-11-10): every section passed on both.** No issues were reported. The per-section table below wasn't filled in.

| Section | Phone (browser: ______) | Laptop (browser: ______) | Notes |
|---|---|---|---|
| 0 Every screen | | | |
| 1 Logged out | | | |
| 2 Login | | | |
| 3 Search | | | |
| 4 Show list | | | |
| 5 Preview | | | |
| 6 Create | | | |
| 7 Log out | | | |
| 8 Errors | | | |
| 9 Expiry | | | |

Date: ______ · App revision: ______

## 5. Access control

### Logged-out API requests (done 2026-11-10)

`curl` with no cookie against `http://127.0.0.1:8080` (the owner's local server) and <https://setlisted-613938414647.europe-west1.run.app> (revision serving `5336235`). `POST`s sent the app's own `Origin`, as a browser does. Both gave the same results:

| Request | Status |
|---|---|
| `GET /api/health` | 200 (public, as designed) |
| `GET /api/session` | 200 (public, as designed) |
| `GET /api/artists?q=Gojira` | **401** |
| `GET /api/artists/{id}/shows` | **401** |
| `GET /api/shows/{id}/preview` | **401** |
| `POST /api/playlists` | **401** |
| `GET /api/nonexistent` (unknown route) | **401** |
| `POST /api/artists` (wrong method) | **401** |
| `GET /api/artists?q=Gojira` with a forged `setlisted_session` cookie | **401** |

The `401` body is `{"error":{"code":"not_logged_in","message":"You're not logged in. Log in with Spotify."}}`. Unknown routes and wrong methods answer `401` rather than `404`/`405` when logged out, so the API's shape isn't revealed. A `POST` **without** an `Origin` header gets `403 forbidden_origin` before the session check, as [`docs/api.md`](../../api.md) documents. It's still refused, and a browser always sends `Origin`.

### In the browser (owner)

| Check | Result | Date |
|---|---|---|
| Non-allowlisted Spotify account sees "Setlisted is invite-only. Ask the owner to add your Spotify account." (AUTH-4) | pass (owner-reported) | 2026-11-10 |
| Logged-out visitor sees only the login screen, including on a deep link (AUTH-1; checklist section 1) | pass (owner-reported) | 2026-11-10 |

## 6. Cost

The first deploy was on 2026-10-27, so November 2026 is the first full month. Check the figure on or after **2026-12-02** (Google Cloud console → Billing → Reports, project `setlisted-2026`, November 2026). Pass: within the intent's budget (£0 target, a few pounds a month at most). The £1 budget alert is set (step 02).

| Month | Cost | Budget alerts received | Date checked |
|---|---|---|---|
| November 2026 | | | |

## 7. Real use

Pending: no gig yet. Record the gig the next time the owner uses Setlisted to prepare for one.

| Gig (band, venue) | Date | Playlist created | Notes |
|---|---|---|---|
| | | | |
