# ADR-006: Testing strategy

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

- The spec's entry classification and matching rules (§5) are precise and numerous, and they're the core of the product.
- Success criterion 3 (intent; spec §9) needs a repeatable check of matching quality: on a test set of about 10 real recent metal setlists, **found / matchable ≥ 90%**, **zero wrong-artist matches**, and correct labels.
- Login, session, error and rate-limit behaviour (spec §3, §8; ADR-004, ADR-005) depend on responses from external services that are awkward to reproduce live.
- The MVP UI is throwaway (ADR-002).
- ADR-001 prefers the Go standard library.
- The repository is hosted on GitHub.

## Decision

**Tooling**

- Use Go's standard **`testing`** package with **table-driven tests**, and **`net/http/httptest`** for HTTP. No assertion or mocking libraries.

**1. Rule tests (unit)**

- The pure classification and matching code (ADR-005) has tests **named after spec requirement IDs**, e.g. `TestMatch_MATCH6_StudioBeatsLive`, `TestEntry_ENT3_MedleyDetection`.
- **Every rule in spec §5 has at least one test**, including its stated edge cases (e.g. `sword` vs `The Sword`, version-suffix removal, re-recordings, tape attribution).

**2. Matching-quality tests (success criterion 3)**

- The ~10 test setlists are stored as **recorded fixtures**: the real setlist.fm and Spotify JSON responses, committed under a test-data directory. The fixtures hold public catalogue data only, never tokens, keys or user data.
- Each setlist has an **expected-outcomes file** giving, per entry: skipped, medley, found (with the expected Spotify track), cover fallback, live version, tape or not found.
- Claude **drafts** the expected outcomes from the fixtures, and the owner **reviews and corrects** them before they're treated as correct.
- A test runs the full matching pipeline against the fixtures **offline** and:
  - **fails** on any wrong-artist match (MATCH-3), with no exceptions;
  - **fails** if found / matchable across the set is below 90%;
  - **fails** on any label that differs from the expected outcome;
  - reports per-setlist and overall results, so changes in quality show up in PRs.

**3. HTTP-level tests**

- API handlers and the setlist.fm and Spotify clients are tested against **`httptest` fake servers**. This covers:
  - the login and callback flow, including cancel (AUTH-5) and non-allowlisted refusal (AUTH-4);
  - cookie session behaviour: expiry, tampering and logout (AUTH-6 to AUTH-8; ADR-004);
  - the `Origin` check on POSTs;
  - error mapping (ERR-1 to ERR-5);
  - setlist.fm rate-limit retries and Spotify `Retry-After` handling (ADR-005);
  - show-list paging (SHOW-1/2).

**4. Live calls are opt-in only**

- Normal test runs make **no network calls**.
- A separate command, behind a Go **build tag**, calls the live APIs (with keys from the local environment) to record or refresh fixtures. It's run by hand, never in CI.

**5. UI**

- The throwaway UI has **no automated tests**. A short **manual test checklist** in the repo covers the journey on the owner's phone and laptop (success criterion 4) and is run before the MVP is declared done and after significant UI changes.

**6. CI**

- **GitHub Actions** runs on every pull request: formatting check (`gofmt`), `go vet`, and `go test ./...`, including the matching-quality tests. Required to pass before merging.

## Alternatives considered

- **testify or other assertion/mocking libraries.** Widely used and somewhat less verbose, but not needed. Standard-library tests are idiomatic and fit ADR-001.
- **Live API tests in CI.** They'd catch API changes, but they're slow and flaky, need secrets in CI, and use up the setlist.fm rate limit. The opt-in refresh command covers the same need by hand.
- **Owner writes all expected outcomes from scratch.** The most independent check, but tedious at about 200 entries. Drafting followed by owner review keeps the owner's judgement while saving most of the effort. The risk is that a reviewer anchors on the draft's assumptions, so review should be genuine, particularly for covers and versions.
- **Browser end-to-end tests** (e.g. Playwright) for the UI. Not worth it for a UI that will be deleted. Worth reconsidering for the real UI.

## Consequences

- Spec rules and tests can be checked against each other by ID. A spec change shows which tests need updating.
- Matching quality is measured on every PR, so a regression in one rule shows up as a drop in the score.
- Fixtures will drift from the live catalogue over time (e.g. tracks removed from Spotify). Refreshing them is a deliberate, manual step, and the expected outcomes may then need re-review.
- Committing real API responses adds some data to the repo (a few hundred KB). That's acceptable.
- The UI's correctness depends on manual checks only, which is acceptable for a throwaway UI.
