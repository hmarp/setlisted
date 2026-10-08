# Plan

- **Status:** Approved (2026-10-08)
- **Covers:** the MVP, as defined in [`intent.md`](intent.md) and [`spec.md`](spec.md), built according to ADR-001 to ADR-008.

## How this plan works

- The MVP is built in **numbered steps**. Each step is **one session and one branch/PR**, and is reviewed and merged before the next step starts.
- Each step has a folder, `docs/plan/NN-<slug>/`, containing:
  - **`brief.md`**, written at the plan stage. It's self-contained: goal, the spec requirements it implements (quoted), the ADR decisions it relies on (summarised), files and areas it touches, how to verify it, and what's out of scope.
  - **`handoff.md`**, written at the end of the step. It records what was built, decisions taken within the step, notes for later steps, and known gaps.
- An implementing session reads this file and the step's folder, and nothing else unless the brief says so (see CLAUDE.md).
- **`docs/api.md`** is the JSON API reference. It's created in step 01 and extended by every step that adds or changes an endpoint, because the future UI depends on it.
- Each step's PR also updates `README.md` and marks the step done below.

## Steps

| # | Step | Depends on | Status |
|---|---|---|---|
| 01 | [Go skeleton: config, health, static serving, CI](plan/01-go-skeleton/brief.md) | — | Not started |
| 02 | [First deploy to Cloud Run](plan/02-first-deploy/brief.md) | 01 | Not started |
| 03 | [Continuous deployment on merge](plan/03-continuous-deployment/brief.md) | 02 | Not started |
| 04 | [Spotify login and session](plan/04-spotify-login/brief.md) | 03 | Not started |
| 05 | [setlist.fm client: artist search and show list API](plan/05-setlistfm-search-shows/brief.md) | 04 | Not started |
| 06 | [Entry classification rules](plan/06-entry-classification/brief.md) | 05 | Not started |
| 07 | [Matching rules (pure)](plan/07-matching-rules/brief.md) | 06 | Not started |
| 08 | [Spotify search client and matching pipeline](plan/08-spotify-search-pipeline/brief.md) | 04, 07 | Not started |
| 09 | [Test setlist fixtures and matching-quality test](plan/09-matching-quality-tests/brief.md) | 05, 08 | Not started |
| 10 | [Preview API](plan/10-preview-api/brief.md) | 05, 08 | Not started |
| 11 | [Create playlist API](plan/11-create-playlist-api/brief.md) | 10 | Not started |
| 12 | [Throwaway UI](plan/12-throwaway-ui/brief.md) | 11 | Not started |
| 13 | [MVP acceptance](plan/13-mvp-acceptance/brief.md) | 09, 12 | Not started |

Steps 06 and 07 are pure code. They need only step 05's setlist.fm types and name normalisation, not live services.

### 01 Go skeleton: config, health, static serving, CI

- Go module, project layout (`cmd/`, `internal/…`, `web/` for the static UI).
- Configuration from environment variables, with startup validation and `godotenv` for local `.env` (ADR-007). Create `.env.example`.
- `net/http` server:
  - a `/api/health` endpoint;
  - serving `web/` (a placeholder page for now);
  - structured logging with `log/slog`;
  - shared JSON response and **API error format** helpers (the basis for spec §8).
- Create `docs/api.md` with the error format and the conventions.
- Dockerfile (small final image).
- GitHub Actions: `gofmt`, `go vet`, `go test ./...` on PRs (ADR-006).
- README: how to run locally.

### 02 First deploy to Cloud Run

- Prove hosting, secrets and HTTPS early, while there's very little code to debug (ADR-003, ADR-007).
- **Owner-run console tasks** with a checklist:
  - Google Cloud project and billing;
  - **£1 budget alert**;
  - Artifact Registry;
  - Secret Manager secrets (with placeholder values where real ones don't exist yet);
  - Cloud Run service with **max instances 1, min 0**.
- **Decide the region** after checking how the free allowance applies per region.
- A repeatable **deploy script** in the repo (manual `gcloud` deploy). It remains the fallback once step 03 automates deployment.
- Verify: `/api/health` and the placeholder page work over the `*.run.app` HTTPS URL.
- README: how to deploy.

### 03 Continuous deployment on merge

- Implements ADR-008. **Owner-run tasks** with a checklist:
  - make the repo public (if it isn't yet);
  - Workload Identity Federation pool and provider, restricted to `hmarp/setlisted` on `main`;
  - deploy service account with minimal roles;
  - Artifact Registry clean-up policy (keep the last ~10 images);
  - **ruleset on `main`**: PRs only, no force-push, CI required.
- GitHub Actions deploy workflow on push to `main`:
  - re-run CI;
  - build and push an image tagged with the commit SHA;
  - deploy to Cloud Run;
  - one deploy at a time.
- Verify by merging a trivial PR and confirming the new revision serves `/api/health`. Practise a **rollback** once.
- README: how deployment works and how to roll back.

### 04 Spotify login and session

- Spotify developer app setup checklist (owner): both redirect URLs, allowlist.
- Authorization Code flow with PKCE using `x/oauth2`:
  - state and verifier cookie;
  - callback;
  - `GET /me` allowlist check;
  - refresh token discarded.

  Covers AUTH-1 to AUTH-8 (ADR-004).
- AES-GCM encrypted session cookie, middleware that requires a session for `/api/*`, `Origin` check on POSTs, logout.
- API: session status / current user, and logout.
- Tests: `httptest` coverage of the flow, cookie tamper/expiry and invite-only.
- Verify locally on `127.0.0.1` (confirm `Secure` cookie behaviour) **and** on Cloud Run, after the merge deploys it.

### 05 setlist.fm client: artist search and show list API

- Shared name normalisation (case, accents, whitespace) using `golang.org/x/text`, which steps 06–07 reuse.
- setlist.fm client: API key header, **in-process rate limiter**, quiet retry on rate limit, error mapping (ADR-005, ERR-1/2/5). Confirm our key's actual rate limit.
- API: **artist search** with the clear-match rule and picker (SRCH-1 to SRCH-5), and **show list** with up to 20 non-empty shows across at most 3 pages (SHOW-1 to SHOW-6).
- `httptest` tests for paging, empty shows, no results and rate-limit retry.

### 06 Entry classification rules

- Pure Go: setlist domain types and classification. Covers ENT-1 to ENT-7: skipped entries, medleys, covers, tapes, notes and duplicates.
- Table-driven tests named after spec IDs (ADR-006).

### 07 Matching rules (pure)

- Pure Go: name normalisation, version-suffix removal, candidate filtering (artist and title), exclusions, and ranking (studio before live, album before single/EP before compilation, earliest release). Covers MATCH-1 to MATCH-9, including the two-pass cover logic and the "no guesses" rule.
- Table-driven tests named after spec IDs.

### 08 Spotify search client and matching pipeline

- Spotify client:
  - field-filtered search (`limit=10`, `market=from_token`);
  - up to 4 searches in parallel;
  - `Retry-After` handling;
  - error mapping (ADR-005).
- Pipeline: classified entries → searches → matching rules → per-entry results, ready for the preview.
- Tests with fake Spotify responses.

### 09 Test setlist fixtures and matching-quality test

- **Owner input needed:** choose about 10 recent metal setlists, including at least one with covers, one with a tape and one with a medley.
- **Decide first: Spotify data in a public repo.** Check Spotify's developer terms on storing and redistributing API content before committing recorded Spotify responses. The proposed approach is to trim the fixtures to only the fields the matcher reads. If the terms don't allow it, supersede that part of ADR-006 (e.g. fixtures kept outside the repo, or test data written by hand).
- Opt-in recorder (Go build tag) that saves the live setlist.fm and Spotify responses as fixtures.
- Claude drafts the expected outcomes for each entry. **The owner reviews and corrects them.**
- Offline quality test: fails on any wrong-artist match, on found/matchable below 90%, or on any wrong label. It reports per setlist (ADR-006).
- If the test exposes rule problems, they're **flagged** (and fixed in this step only if small). The step is done when the test passes on the reviewed expectations.

### 10 Preview API

- API: preview a show. It returns the show header, summary counts, entries with set headings, notes, statuses and labels, and the matched track details and IDs (PREV-1 to PREV-7, ADR-005).
- Handler tests.

### 11 Create playlist API

- API: create a playlist from show ID plus ordered track IDs:
  - fetch the show again for name and description (PL-2, PL-3);
  - create a private playlist via `POST /me/playlists`;
  - add tracks via `/playlists/{id}/items` in batches;
  - return the name, track count and Spotify link.

  Covers PL-1 to PL-9 and ERR-3/ERR-4.
- Handler tests, including partial failure.

### 12 Throwaway UI

- Static HTML and plain JS in `web/`, using `docs/api.md` only. It covers every screen:
  - login and invite-only;
  - search and picker;
  - show list;
  - preview with "Matching songs…";
  - confirmation;
  - errors with Retry;
  - log out;
  - setlist.fm footer.

  Covers GEN-1 to GEN-3 and the spec's screen behaviour. It must be usable on a phone (ADR-002).
- Add the **manual test checklist** to the repo (ADR-006).
- After the merge deploys it, try it on phone and laptop.

### 13 MVP acceptance

- Run the manual checklist on the owner's phone and laptop.
- Check each success criterion (spec §9) and record the results:
  - timed journey;
  - two friends onboarded;
  - matching-quality test result;
  - access control;
  - first month's cost (this one may complete later);
  - real use for a gig.
- Fix small issues found. Larger ones are flagged as post-MVP work.
- Final README for the MVP.

## Decisions left to individual steps

- Cloud Run region (step 02)
- Exact project layout and API endpoint shapes (step 01 sets conventions, and later steps add endpoints to `docs/api.md`)
- setlist.fm key's actual rate limit (step 05)
- `Secure` cookie on `127.0.0.1` (step 04)
