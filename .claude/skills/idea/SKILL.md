---
name: idea
description: Interview the owner about a Setlisted issue or new idea and write its feature spec (docs/features/<issue#>-<slug>/spec.md). Use when the owner runs /idea or asks to spec, flesh out or think through an issue before it's built.
argument-hint: "<issue number, or a one-line idea>"
disable-model-invocation: true
---

# /idea: from an issue to a feature spec

Turn an issue (or a raw idea) into a spec that a fresh session can implement without this conversation. Input: `$ARGUMENTS`.

## 1. Get the issue

- **A number:** `gh issue view <n> --comments`.
- **A raw idea, or nothing:** ask the owner for the idea, draft an issue (title, problem, rough idea, label: `bug`, `feature`, `matching`, `refactor` or `idea`) and create it with `gh issue create` only after the owner approves the text. Issues are public: no personal details.

## 2. Decide whether it needs a spec

It needs one if it changes user-visible behaviour, changes a matching rule, adds or changes an endpoint, or adds a dependency (see `CLAUDE.md`). If it doesn't, say so, suggest `/implement <n>`, and stop.

## 3. Learn the current state first

Before asking anything, read what the idea touches: the relevant code and tests, `docs/api.md` and the relevant ADRs in `docs/adr/`. Don't read `docs/mvp/` except to look up an old requirement ID. Ask only what the code can't tell you.

## 4. Interview the owner

Ask in short rounds, one topic at a time, using AskUserQuestion with concrete options where possible. Cover, as relevant:

- the problem, and who hits it (owner, friends, phone, laptop);
- the behaviour wanted, step by step, including what the user sees;
- edge cases and failures (no results, a service down, rate limits, partial results);
- matching: real examples (bands, shows, songs), and what counts as right;
- API and UI changes;
- what's explicitly out of scope;
- how to check it's done: tests, quality-test changes, manual checks.

Push back on vague answers, and point out costs, simpler alternatives and conflicts with ADRs or constraints (Spotify development mode, setlist.fm's rate limit and non-commercial terms, no stored user data, Cloud Run's free tier). If the idea needs a stack or design change, it also needs a new ADR: say so and draft it with the owner.

## 5. Write the spec

Create the branch `<n>-<slug>` from the latest `origin/main` (`git fetch origin` first), and write `docs/features/<n>-<slug>/spec.md`:

```markdown
# <Title> (#<n>)

## Problem
## Behaviour
Numbered requirements, each testable (e.g. LINEUP-1, LINEUP-2).
## Edge cases and errors
## API changes
(or "None")
## UI changes
(or "None")
## Out of scope
## How to verify
Tests to add, quality-test changes, manual checks.
## Decisions
What was decided in the interview, with the reasons, and any open questions.
```

Add `plan.md` alongside only if the work spans several sessions or PRs: ordered steps, each one PR-sized, with how to verify each.

## 6. Finish

- Show the owner the spec and revise it until they approve.
- Commit it on the branch (`Spec for #<n>: <title>`). Don't push or comment on the issue unless the owner asks.
- Tell the owner to start a **fresh session** and run `/implement <n>`. The spec is the handoff; the new session won't see this conversation.
