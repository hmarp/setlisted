# CLAUDE.md

## Project

Setlisted is a web app that lets a user search for a band's live performance (via setlist.fm) and create a Spotify playlist from its setlist. It is used only by the owner and a few friends.

## Read first

What to read depends on the kind of session.

### Implementing a plan step (most sessions)

Read **only**:

1. `docs/plan.md`: the step list, statuses and dependencies. Confirm the step is next and its dependencies are done.
2. Everything in that step's folder, `docs/plan/NN-<slug>/`:
   - `brief.md`: what to build. It is self-contained: it quotes the spec requirements and summarises the ADR decisions the step relies on.
   - `handoff.md` files in the folders of the steps listed as dependencies, if the brief says to read them.

Don't read the rest of `docs/` unless the brief is unclear or seems to contradict the code. If that happens, check the spec section or ADR the brief cites. **The spec and ADRs are the source of truth, and briefs are copies taken from them.** If they disagree, stop and flag it.

### Working on intent, spec, ADRs or the plan itself

Read the documents in `docs/` in this order:

1. `docs/intent.md`: what we're building and why
2. `docs/spec.md`: required behaviour
3. `docs/adr/`: architecture and stack decisions
4. `docs/plan.md` and the step folders: the implementation plan

If a document is still a stub, the work for that stage hasn't happened yet.

## Workflow rules

- Work follows the stages: intent → spec → ADRs → plan → implementation. Don't skip ahead.
- Don't write application code until `docs/plan.md` exists and has been approved.
- Don't choose or change the tech stack without an ADR in `docs/adr/`. Accepted ADRs aren't edited. Supersede them with a new one.
- **One plan step per branch/PR.** Reference the step (e.g. "Step 04") in the branch name and PR description. Don't start work belonging to a later step.
- If implementation reveals the spec, an ADR, the plan or a brief is wrong, stop and flag it rather than silently diverging.
- **At the end of each step**, in the same PR:
  - write `handoff.md` in the step's folder: what was built, decisions made within the step, anything later steps need to know, and known gaps;
  - mark the step done in `docs/plan.md`;
  - update `README.md` so it reflects the project's current state (status, how to run, configuration, how to test, how to deploy, as applicable);
  - if the step changed anything that later, unstarted briefs rely on, update those briefs too.

## Secrets

- Never commit API keys, client secrets or tokens. Configuration comes from environment variables (ADR-007).
- `.env` files are git-ignored. Keep `.env.example` up to date with variable names and placeholders only.
