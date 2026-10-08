# ADR-001: Go backend exposing a JSON API

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

Setlisted needs server-side code no matter which UI is used:

- The **setlist.fm API key** and the **Spotify client secret** must never reach the browser (intent, Constraints; CLAUDE.md, Secrets).
- The Spotify login, the session (spec §3) and all calls to setlist.fm and Spotify have to happen somewhere trusted.
- The **matching rules** (spec §5) are the core of the product and need thorough automated tests.

The owner plans substantial UI design work **after** the MVP and intends to replace the MVP UI completely (spec GEN-2; ADR-002). The backend must therefore expose a clean boundary that a future UI can use without changes to the backend.

The owner explicitly wants to **learn a new stack** with this project, rather than use their most familiar one (.NET). That is a valid reason for this choice and is recorded here as one.

## Decision

- The backend is written in **Go** (current stable release) as a **single service**.
- It exposes a **JSON API** for the UI. The API is shaped around the MVP's journey (search artists, list shows, preview a show's matches, create a playlist, session/login status). It is not a general-purpose public API.
- The backend owns **everything that holds a secret or a rule**: the Spotify login flow and session, all setlist.fm and Spotify calls, entry classification and matching, and playlist creation. The UI only displays data and sends user choices.
- **Standard library first.** We use `net/http` (including its built-in method and path routing), `encoding/json`, `log/slog`, `testing` and so on. A third-party dependency is added only when it clearly earns its place, and the reason is noted in the PR that adds it. A dependency that is expected now is the official `golang.org/x/oauth2` package for Spotify login (detailed in a later ADR). No web framework such as Gin, Echo or Fiber.

## Alternatives considered

- **ASP.NET Core (C#), server-rendered.** This is the owner's familiar stack and the fastest path to a working tool. Rejected because the owner wants to learn something new, not for technical reasons.
- **Next.js full-stack (TypeScript)**, using its route handlers as the backend. This would be one codebase, but there would be no Go, and the backend would be tied to the UI framework the owner wants freedom to choose and change later.
- **Separate Go API and Next.js app now.** Rejected for the MVP: two deployments, two cold starts on free hosting, and login/session handling across two origins, all for a deliberately basic UI. It may be revisited when the real UI is built.
- **Go rendering HTML** (templates or htmx fragments). Rejected because the backend would produce HTML rather than JSON. The future UI would then need a JSON API built at that point, or every endpoint would have to be built twice.
- **Gin** (or Echo/Fiber). It's popular, and it adds request binding and validation, JSON helpers and ready-made middleware. Rejected because:
  - the standard library router now covers methods and path parameters, which was the main reason to use a framework;
  - Gin's own handler type (`*gin.Context`) ties handlers, middleware and tests to Gin rather than to the standard `http.Handler` types the rest of the Go ecosystem uses;
  - its extras would replace only a small amount of helper code at this size;
  - learning the standard types first is the better foundation for this project's learning goal.

  Learning Gin specifically wasn't a goal.
- **chi.** A lightweight router built on the standard handler types. It's not needed now, but if the standard router becomes limiting (e.g. route groups with per-group middleware), chi is the preferred step up. It can be adopted without rewriting handlers.

## Consequences

- One codebase and one deployable for the backend. The language is new to the owner, so early progress will be slower.
- Secrets and matching logic live in one place and can be unit-tested without a UI.
- The future UI can be built against an API that already works, without changing the backend.
- The API has to be designed deliberately (request/response shapes, error format for spec §8), because it's a lasting contract and not an internal detail.
- With only the standard library, we write some small utilities ourselves (e.g. JSON response helpers, middleware). That's acceptable at this size.
