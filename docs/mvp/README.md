# MVP archive

How the Setlisted MVP was specified, planned and built (October–November 2026). **Kept as a record only: not maintained, and not used for new work.**

| Document | What it was |
|---|---|
| [`intent.md`](intent.md) | What the MVP was for, what was in and out of scope, and the constraints |
| [`spec.md`](spec.md) | The MVP's required behaviour, with requirement IDs (`AUTH-1`, `MATCH-4`, …) |
| [`plan.md`](plan.md) | The 13 implementation steps |
| [`plan/NN-<slug>/`](plan/) | Each step's brief (what to build) and handoff (what was built and decided) |
| [`plan/13-mvp-acceptance/`](plan/13-mvp-acceptance/) | The acceptance results, the post-MVP candidate list and lessons learned |

Paths written as plain text inside these documents (such as `docs/plan.md`) are as they were at the time, before the archive moved here.

The requirement IDs still appear in test names and code comments. They refer to `spec.md` here, which describes the MVP's behaviour; later changes are described in their own feature specs under [`../features/`](../features/) and in the code and tests.

The setup runbooks from steps 02–05 moved to [`../setup.md`](../setup.md), because they're still in use.
