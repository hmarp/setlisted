# Setlisted

Search for a band's live performance and turn its setlist into a Spotify playlist.

Built on the [setlist.fm API](https://api.setlist.fm/docs/1.0/index.html) and the [Spotify Web API](https://developer.spotify.com/documentation/web-api). For personal use by a small group of friends.

## Status

Pre-development: working through intent → spec → plan. See [`docs/`](docs/).

## Workflow

1. **Intent**: idea-grilling session produces `docs/intent.md`
2. **Spec**: `docs/spec.md` is derived from the intent
3. **Architecture**: stack and design decisions are recorded as ADRs in `docs/adr/`
4. **Plan**: developer + AI review session produces `docs/plan.md`
5. **Implement**: work from the plan, open a PR, review manually before merging
