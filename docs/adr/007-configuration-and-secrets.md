# ADR-007: Configuration and secrets

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

The app needs three **secrets**:

- the **setlist.fm API key**;
- the **Spotify client secret**;
- the **cookie encryption key** (ADR-004).

It also needs non-secret **configuration**: the Spotify client ID, the public base URL (from which the Spotify redirect URL is built), and the listening port (Cloud Run supplies `PORT`).

CLAUDE.md requires that secrets are never committed, that `.env` files are git-ignored, and that `.env.example` is kept up to date with variable names only. The app runs locally for development and on Cloud Run in production (ADR-003).

## Decision

**Environment variables only**

- The app reads all configuration and secrets from **environment variables**. Nothing secret is ever in the repo, a config file in production, or a container image.
- At startup, the app **validates** every required variable: it must be present and well-formed (e.g. the cookie key must decode to exactly 32 bytes, and the base URL must be absolute). If any is missing or invalid, the app **refuses to start** with a message naming the variable. A secret's value is never printed.

**Production**

- Secrets are stored in **Google Secret Manager** and exposed to the Cloud Run service as environment variables. Non-secret configuration is set directly on the Cloud Run service.
- Rotating the cookie key means adding a new secret version and redeploying. This logs all users out (ADR-004).

**Local development**

- Variables come from a git-ignored **`.env`** file, loaded with **`github.com/joho/godotenv`** at startup. The owner chose this dependency (ADR-001) over a hand-written loader: it's small and widely used, so we don't need to write and maintain our own parsing.
- The `.env` file is **optional**. If it doesn't exist, the app carries on with the environment as it is. Variables already set in the environment **take precedence** over `.env` values, so production (which has no `.env`) behaves the same way.
- **`.env.example`** lists every variable with a placeholder and a one-line description. It's updated in the same PR as any new or renamed variable.

**Spotify app**

- **One Spotify developer app** for both local and production use, with both redirect URLs registered: `http://127.0.0.1:<port>/…` for local and the Cloud Run HTTPS URL for production. This keeps the allowlist in one place and avoids the per-developer client ID limit introduced in 2026.

**Logging**

- Logs never contain secrets, access tokens, cookie values, or user data beyond what's needed to diagnose a failure (GEN-5, ADR-003).

## Alternatives considered

- **Hand-written `.env` loader.** No dependency, about 20 lines, but it's our own parsing to maintain. The owner preferred the established library.
- **No `.env` file** (variables set in the shell or IDE run configuration). Nothing to load, but it's less convenient, and setup is less repeatable on a new machine.
- **Config files** (YAML/JSON) for non-secret settings. Not needed for so few settings, and two configuration sources means more ways to get it wrong.
- **Secrets as plain Cloud Run environment variables** without Secret Manager. Simpler, but the values are visible in the service configuration to anyone who can view it, and there's no versioning. Secret Manager is free at this usage.
- **Separate Spotify apps for development and production.** Cleaner separation, but it doubles the allowlist management and runs into the 2026 client ID limits.

## Consequences

- Configuration is in one place, and production and local work the same way, apart from where the values come from.
- Misconfiguration is caught at startup rather than mid-journey.
- One small third-party dependency (`godotenv`), used only to load local configuration.
- Production deploys need Secret Manager access granted to the Cloud Run service account, which is a plan-stage task.
- A developer machine's `.env` holds live secrets and must be protected like any credentials file.
- The single Spotify app means local development uses real user accounts on the same allowlist as production. At this scale, that's acceptable.
