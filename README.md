# Setlisted

Search for a band's live performance and turn its setlist into a Spotify playlist.

Built on the [setlist.fm API](https://api.setlist.fm/docs/1.0/index.html) and the [Spotify Web API](https://developer.spotify.com/documentation/web-api). For personal use by a small group of friends.

## Status

Pre-development. Intent, spec, architecture decisions and the implementation plan are approved. Implementation starts with [step 01](docs/plan/01-go-skeleton/brief.md). See [`docs/`](docs/).

## Stack

- **Backend:** Go, standard library first, exposing a JSON API ([ADR-001](docs/adr/001-go-backend-json-api.md))
- **UI (MVP):** throwaway static HTML and plain JavaScript, served by the Go backend ([ADR-002](docs/adr/002-throwaway-static-ui.md))
- **Hosting:** Google Cloud Run ([ADR-003](docs/adr/003-hosting-google-cloud-run.md))
- **Login:** Spotify only, with a stateless encrypted-cookie session ([ADR-004](docs/adr/004-spotify-login-encrypted-cookie-session.md))
- **Delivery:** every merge to `main` deploys automatically via GitHub Actions ([ADR-008](docs/adr/008-continuous-deployment-on-merge.md))

## Workflow

1. **Intent**: idea-grilling session produces [`docs/intent.md`](docs/intent.md)
2. **Spec**: [`docs/spec.md`](docs/spec.md) is derived from the intent
3. **Architecture**: stack and design decisions are recorded as ADRs in [`docs/adr/`](docs/adr/)
4. **Plan**: [`docs/plan.md`](docs/plan.md) lists the implementation steps. Each step has a self-contained folder under `docs/plan/` with a brief for the session that implements it.
5. **Implement**: one step per branch/PR, reviewed manually before merging. Each step ends with a handoff note and an updated README.

## Licence

[MIT](LICENSE)
