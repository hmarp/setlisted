# CLAUDE.md

## Project

Setlisted is a web app that lets a user search for a band's live performance (via setlist.fm) and create a Spotify playlist from its setlist. It is used only by the owner and a few friends.

## Read first

Before doing any work, read the documents in `docs/` in this order:

1. `docs/intent.md`: what we're building and why
2. `docs/spec.md`: required behaviour
3. `docs/adr/`: architecture and stack decisions
4. `docs/plan.md`: the current implementation plan

If a document is still a stub, the work for that stage hasn't happened yet.

## Workflow rules

- Work follows the stages: intent → spec → ADRs → plan → implementation. Don't skip ahead.
- Don't write application code until `docs/plan.md` exists and has been approved.
- Don't choose or change the tech stack without an ADR in `docs/adr/`.
- Implement one plan step per branch/PR where practical, and reference the plan step in the PR description.
- If implementation reveals the spec or plan is wrong, stop and flag it rather than silently diverging.

## Secrets

- Never commit API keys, client secrets or tokens. Use environment variables or user secrets.
- `.env` files are git-ignored; keep `.env.example` up to date with variable names only.
