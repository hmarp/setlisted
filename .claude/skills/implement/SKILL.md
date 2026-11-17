---
name: implement
description: Implement a Setlisted GitHub issue end to end (branch, plan, tests, code, checks, review, PR draft). Use when the owner runs /implement or asks to build, fix or work on a specific issue.
argument-hint: "<issue number>"
disable-model-invocation: true
---

# /implement: from an issue to a reviewed PR

Implement issue `$ARGUMENTS`, following `CLAUDE.md`.

## 1. Understand the work

- `gh issue view <n> --comments`.
- If `docs/features/<n>-*/spec.md` exists (on `main` or the branch `<n>-<slug>`), it's the requirements. Read its `plan.md` too, if there is one, and do only the next unfinished step.
- **If the issue is non-trivial** (user-visible behaviour, a matching rule, an endpoint or a new dependency) **and has no spec, stop** and suggest `/idea <n>`.
- Read the code and tests the change touches, and the ADRs that cover it. Don't read `docs/mvp/`.
- If the issue or spec contradicts an ADR or the code, stop and ask.

## 2. Branch

`git fetch origin`, then check out `<n>-<slug>` if it exists (the spec may be on it), or create it from `origin/main`.

## 3. Plan

- **Non-trivial:** present a short plan (the files, the approach, the tests) and wait for the owner's approval before writing code.
- **Trivial:** state the plan in a line or two and go ahead.

## 4. Tests first

Where the behaviour can be tested, write the failing test first and see it fail:
- table-driven unit tests, named after the spec's requirement IDs;
- `httptest` for handlers;
- for a matching change, a test case for the setlist entry it should fix. Changing `testdata/quality/*/expected.json` needs the owner's approval.

## 5. Implement

- Keep to the issue. List anything else you notice as a possible new issue; don't fix it here.
- Update `docs/api.md` for any endpoint change, `README.md` if running, configuring, testing or deploying changes, and `.env.example` for new configuration.
- Commit in logical steps, with messages ending in `(#<n>)`.

## 6. Check

```bash
gofmt -l .
go vet ./...
go vet -tags live ./...
go test ./...
```

`gofmt -l .` must print nothing, and the rest must pass, including `TestMatchingQuality`. Fix any failure before going on; never weaken a test to make it pass.

For a UI change, also check the page in the browser where you can, and tell the owner which sections of `docs/manual-test-checklist.md` to run.

## 7. Review

Run the `code-review` skill on the branch's diff. Fix the real findings, re-run the checks, and note the ones you decided not to fix, with the reason.

## 8. Hand over

- Draft the PR: a title, a summary of what changed and why, how it was verified, anything the owner must check by hand, and `Closes #<n>`.
- **Push and open the PR only when the owner asks.**
- Tell the owner what's done, what's left for them to check, and any follow-up issues you suggest (not created yet).
