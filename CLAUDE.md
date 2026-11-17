# CLAUDE.md

## Project

Setlisted is a web app that lets a user search for a band's live performance (via setlist.fm) and create a Spotify playlist from its setlist. It is used only by the owner and a few friends. The MVP is complete and deployed; work now comes from GitHub issues.

## Where things are

- **How the app behaves today:** the code and its tests. `docs/api.md` is the API contract the UI uses.
- **Decisions that still hold:** `docs/adr/`. Read the relevant ADRs before changing anything they cover.
- **The work item:** the GitHub issue (`gh issue view <n>`), and its spec in `docs/features/<n>-<slug>/` if it has one.
- **Setup and deployment runbooks:** `docs/setup.md`; how to run, test and deploy: `README.md`.
- **`docs/mvp/` is an archive.** Don't read it or build on it, except to look up an old requirement ID (`MATCH-4`, …) or why something was done.

## Workflow

- **Every change starts from an issue. One issue per branch and PR:** branch `<issue#>-<slug>`, and `Closes #<n>` in the PR description.
- **Trivial changes** (bugs, refactors, small fixes, docs) go straight from issue to PR.
- **Non-trivial changes** need `docs/features/<n>-<slug>/spec.md`, written with the owner (`/idea <n>`), before any code. Non-trivial means a change in user-visible behaviour, a matching-rule change, a new or changed endpoint, or a new dependency. Add a `plan.md` only for work spanning several sessions or PRs.
- **Implement with `/implement <n>`.**
- **Feature specs are frozen once shipped.** Never edit an old one or rely on it for new work.
- **Stop and flag** if an issue or spec contradicts an ADR or the code, rather than silently diverging.
- Push, open PRs and create issues only when the owner asks.

## Rules that always apply

- **Stack and design:** no new framework, service, database or dependency without a new ADR. Accepted ADRs aren't edited; supersede them.
- **API:** any endpoint change updates `docs/api.md` in the same PR.
- **Matching:** `TestMatchingQuality` must keep passing. Change `testdata/quality/*/expected.json` only with the owner's approval.
- **UI:** after a UI change, the owner runs `docs/manual-test-checklist.md`. There are no automated UI tests (ADR-006).
- **README:** keep it current when running, configuring, testing or deploying changes.
- **Checks before a PR:** `gofmt -l .` prints nothing; `go vet ./...`, `go vet -tags live ./...` and `go test ./...` pass.

## Secrets

- Never commit API keys, client secrets, tokens or user data. Configuration comes from environment variables (ADR-007).
- Never read, print or stage `.env`; hooks block it. Keep `.env.example` current, with names and placeholders only.
- GitHub issues are public: no personal details in them.
