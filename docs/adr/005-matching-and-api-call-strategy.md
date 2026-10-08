# ADR-005: Server-side matching at preview time, and the API call strategy

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

- The preview shows each entry's match status and matched track details before a playlist is created (spec §6, PREV-2/4/7). Creating the playlist uses those matches (spec §7).
- The matching rules (spec §5) are the core of the product and must be thoroughly unit-testable.
- The server is stateless and stores nothing (ADR-003, ADR-004, spec GEN-5), so nothing about a preview can be held on the server for the create step.
- **setlist.fm API:** non-commercial, rate-limited per API key (the exact rate for our key is confirmed at the plan stage). It returns setlists 20 per page.
- **Spotify Web API** (development mode, since February 2026): search returns at most **10** results per request. Playlists are created with `POST /me/playlists`, and tracks are added with `/playlists/{id}/items`. Rate limits are applied over a rolling window, and "slow down" responses carry a `Retry-After` value.
- Cloud Run is capped at **one instance** (ADR-003).

## Decision

**When matching happens**

- Matching runs **on the server, when the preview is requested**. One API call ("preview show X") fetches the setlist, classifies the entries (spec §5.1–5.3), runs the Spotify searches, applies the matching rules (§5.4), and returns the preview rows: status, labels and matched track details, including track IDs.
- **Create doesn't re-run matching.** The UI sends the show ID and the ordered list of matched track IDs from the preview. The server fetches the show again (one setlist.fm call) to build the name and description (PL-2, PL-3), creates the private playlist and adds the tracks.
- The track list isn't signed or verified. A user who tampered with it could only change the contents of a playlist in **their own** library. This is accepted.

**Code structure**

- Entry classification and candidate selection are **pure Go code** with no I/O. They take setlist entries and Spotify candidate tracks as plain values and return decisions. The HTTP clients for setlist.fm and Spotify are separate, and are passed in where they are used, so the matching rules can be unit-tested without the network (testing ADR to follow).

**Spotify calls**

- One **field-filtered search per entry per pass** (`track:"…" artist:"…"`), with `limit=10` and `market=from_token`, so candidates are playable in the user's country. Covers can take two passes (MATCH-2).
- Up to **4 searches in parallel** per preview, a fixed, configurable bound.
- Spotify's "slow down" (HTTP 429) responses are honoured by waiting for `Retry-After` and retrying. Other failures surface as ERR-1.
- Tracks are added to the playlist in batches within Spotify's per-request limit.

**setlist.fm calls**

- **Sequential**, through a single **in-process rate limiter** set just below our key's rate. Because there is only one instance, this throttle covers the whole app.
- A rate-limit response is retried quietly after a short wait, a bounded number of times, and then surfaces as ERR-1 (ERR-2).
- **Show list (SHOW-1/2):** fetch pages of recent setlists, drop those with zero entries, and stop when there are 20 shows or after **3 pages**, whichever comes first.

**No caching** of setlist.fm or Spotify responses between requests. It isn't needed at this volume, and it keeps the app free of stored data.

## Alternatives considered

- **Match at create time only.** Fewer Spotify calls when a user backs out of a preview, but the preview could no longer show found/not-found or track details. That would go against the intent's journey and spec PREV-2/4/7, and users would never see unmatched songs. The calls saved are free and far below the limits. Rejected by the owner.
- **Re-run matching at create.** No trust in the client, but it doubles the Spotify calls, and results could differ from what the user previewed.
- **Signed preview result** (a server signature over the track list). This stops tampering, but the only possible victim is the tamperer, so it isn't worth the extra work.
- **Server-side cache** of setlists or matches. Avoids repeat calls, but it's state on a stateless service and unnecessary at this volume.
- **Unbounded parallel searches.** Faster, but it risks Spotify throttling in development mode for little gain on a roughly 20-song setlist.

## Consequences

- A preview costs about one Spotify search per matchable entry, plus a second for unmatched covers (roughly 20–30 calls for a typical show), and takes a few seconds behind "Matching songs…" (PREV-1).
- Create costs one setlist.fm call plus a few Spotify calls, and is fast.
- The preview result travels through the browser, so the API response for preview defines what create needs. Any future UI must send it back unchanged.
- The rate limiter's simplicity depends on the one-instance cap (ADR-003). Raising max instances would need this revisited.
- If many live versions push the studio version out of the 10 search results, a studio version may be missed (a live fallback or not found). If the matching test set (intent, success criterion 3) shows this happening, fetching a second page of results is the first remedy to try.
- Matching rules can be changed and tested in isolation from the HTTP and Spotify code.
