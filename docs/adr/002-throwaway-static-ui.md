# ADR-002: Throwaway static HTML/JS UI served by the Go backend

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

The MVP UI is to be **extremely basic** (spec GEN-2). The owner has big plans for the UI design, but those come **after** the MVP, and the MVP UI will be **replaced completely** by a more capable framework later.

So the MVP UI needs to:

- cover the spec's screens: login, search and artist picker, show list, preview, and confirmation and errors;
- work on phone and laptop browsers (GEN-1);
- be cheap to build and cheap to delete;
- exercise the backend's JSON API (ADR-001) fully, proving the API is complete before the real UI is built against it.

## Decision

- The MVP UI is a small set of **static HTML files with plain JavaScript and minimal CSS**: no framework, no build step, no npm.
- It talks to the backend **only through the JSON API**, using `fetch()`. It never calls setlist.fm or Spotify itself and never sees a secret or token.
- The **Go backend serves the static files** from the **same origin** as the API. As a result:
  - the session cookie (detailed in a later ADR) works without cross-origin configuration;
  - no CORS is needed;
  - there is one deployable.
- The UI lives in its own directory, so it can be deleted and replaced without touching backend code.
- Styling is limited to what's needed for readability and phone use (e.g. a viewport meta tag, readable font size, full-width controls). No visual design.

## Alternatives considered

- **Go HTML templates (server-rendered).** Simpler to start, but it ties the UI to the backend, and the future UI's JSON API would have to be built at that point.
- **htmx.** Productive for apps that stay server-rendered, but it relies on the server returning HTML fragments. That defeats the JSON API boundary (ADR-001) or doubles the endpoints. Using it with JSON goes against how it's designed.
- **Next.js or another framework now.** The owner wants a capable framework, but for the post-MVP design work. Bringing it in now adds tooling and a build pipeline to a UI that will be thrown away, and forces the frontend choice before the design needs are known.
- **Separately hosted frontend.** Adds a second deployment and cross-origin session handling for no MVP benefit.

## Consequences

- The UI is quick to write and easy to throw away. Its code quality matters less than the API's.
- Hand-written JS for four screens is some repetition, which is acceptable for a throwaway.
- The API gets exercised by a real client from the start.
- When the real UI arrives, a new ADR will decide its framework and whether it is served by Go (e.g. a static export) or hosted separately. That ADR will supersede this one.
