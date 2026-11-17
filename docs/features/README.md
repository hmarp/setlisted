# Feature specs

One folder per non-trivial issue: `<issue#>-<slug>/`, for example `21-lineup-playlists/`.

- **`spec.md`**: written with the owner in an idea session (`/idea <issue#>`), before any code. It describes the change from the app's current behaviour: the problem, the requirements, what's out of scope and how to check it.
- **`plan.md`**: only when the work spans several sessions or PRs.

A feature's folder is **frozen once the feature ships**. It records why a change was made, not how the app behaves today; for that, read the code and tests, [`api.md`](../api.md) and the [ADRs](../adr/). Don't update an old spec or rely on it for new work.

What counts as non-trivial, and the rest of the workflow, is in [`CLAUDE.md`](../../CLAUDE.md).
