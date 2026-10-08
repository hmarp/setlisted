# Spec

Required behaviour for the Setlisted MVP. Derived from [`intent.md`](intent.md). If this document and the intent disagree, flag it and don't pick one silently.

The spec describes **what** the app does, not how. It makes no stack, hosting or architecture choices. Those belong in `docs/adr/`.

Requirements have IDs (e.g. `SRCH-3`) so the plan and PRs can refer to them.

## 1. Definitions

| Term | Meaning |
|---|---|
| **User** | A person who has logged in with a Spotify account on the owner's Spotify developer allowlist. |
| **Band** | The setlist.fm artist the user selected. Also called the *performing artist*. |
| **Show** | One setlist.fm event (setlist) for the band: a date, venue, city, optional tour name, and an ordered list of entries. |
| **Entry** | One line of a show's setlist, as recorded on setlist.fm. It may be in a set or an encore and may carry a note. |
| **Cover** | An entry that setlist.fm attributes to an artist other than the band (the *original artist*). |
| **Tape** | An entry setlist.fm flags as played from tape. |
| **Matchable entry** | An entry that isn't skipped (§5.1) and isn't a medley (§5.2). |
| **Normalised name** | A name compared ignoring letter case, accents and leading/trailing spaces (e.g. `motorhead` = `Motörhead`). |

## 2. General

- **GEN-1** The app is a website that works in current mobile and desktop browsers. All screens can be used on a phone-sized screen without zooming or horizontal scrolling.
- **GEN-2** The UI is extremely basic: plain, functional screens with no visual design work (see intent, Scope).
- **GEN-3** Every screen has a footer reading "Setlist data from setlist.fm", which links to setlist.fm.
- **GEN-4** The app is non-commercial: no ads, payments or donation links.
- **GEN-5** The app stores **no user data**. Nothing about a user, their searches, matches or playlists is kept after their browser session ends or they log out. The only state held is what's needed to act on the user's behalf during their current session.

## 3. Login and session

- **AUTH-1** Before login, the only things a visitor can see are the app name, a one-line description, a **"Log in with Spotify"** action and the footer. No search, show or setlist data is available without login.
- **AUTH-2** Logging in goes through Spotify's own login and consent. The app asks only for the Spotify permissions it needs to read the user's identity, search the catalogue, and create a private playlist with tracks in the user's library.
- **AUTH-3** After a successful login, the user lands on the search screen.
- **AUTH-4** If Spotify refuses the user because their account isn't allowlisted, the app shows an **invite-only message**: "Setlisted is invite-only. Ask the owner to add your Spotify account." No raw Spotify error is shown.
- **AUTH-5** If the user cancels or declines on Spotify's consent screen, they're returned to the login screen with the message "Login was cancelled."
- **AUTH-6** A login lasts until **the browser session ends or one hour has passed since login, whichever comes first**. After that, the user must log in again. There is no silent renewal. (If Spotify remembers their consent, logging in again is usually one or two taps.)
- **AUTH-7** Every logged-in screen has a **"Log out"** action. Logging out discards the session completely and returns to the login screen.
- **AUTH-8** If the session has expired or become invalid at any point, the user is sent to the login screen. After logging in again they start at the search screen. The app does not restore their previous place.

## 4. Search, artist and show selection

### 4.1 Artist search

- **SRCH-1** The search screen has one text box for the band name and a Search action. An empty or whitespace-only search can't be submitted.
- **SRCH-2** Search uses setlist.fm's artist search with the text the user entered.
- **SRCH-3 (clear match)** If **exactly one** result has a normalised name equal to the normalised search text, the app goes straight to that band's shows (§4.2) and skips the picker.
  - The rule is strict: `sword` does **not** match `The Sword`.
- **SRCH-4 (picker)** Otherwise, if there are results, the app shows an **artist picker**: up to **10** results in setlist.fm's order, each showing the artist name and setlist.fm's disambiguation text if there is any. There is no paging. Choosing an artist goes to its shows.
  - This covers both several exact-name matches (e.g. several bands called "Ghost") and no exact match (e.g. a typo).
- **SRCH-5 (no results)** If there are no results, the app shows "No artists found for '‹search text›' on setlist.fm." The search box keeps the text so the user can edit it.

### 4.2 Show list

- **SHOW-1** The show list is headed with the band's name and lists up to the **20 most recent shows that have at least one entry**, newest first.
- **SHOW-2** Shows with **zero entries**, including announced future shows, are not listed.
- **SHOW-3** Each show displays: **date** (`DD Mon YYYY`), **city**, **venue**, **tour name** (if setlist.fm has one) and **song count**. Song count is the number of entries in the setlist, including tapes.
- **SHOW-4** The app does not try to detect or flag partial setlists. The user judges this from the song counts.
- **SHOW-5** If the band has no shows with entries, the app shows "No setlists found for ‹band› on setlist.fm."
- **SHOW-6** The show list has a "New search" action that returns to the search screen. Choosing a show goes to its preview (§6).

## 5. Entry handling

Each entry in the chosen show is classified, in this order:

### 5.1 Skipped entries

- **ENT-1** An entry is **skipped** if it has no name, or if its whole name (normalised) is a solo, jam or improvisation: `solo`, `<anything> solo` (e.g. "Drum Solo", "Guitar Solo"), `jam` or `improvisation`.
- **ENT-2** Skipped entries are left out of both the preview and the playlist and are not counted as matchable.

### 5.2 Medleys

- **ENT-3** An entry is a **medley** if its name contains the word "medley" (any case) or combines several song names separated by ` / `.
- **ENT-4** Medleys are shown in the preview as **Not found (medley)** and are not added to the playlist. They are not counted as matchable.

### 5.3 Everything else

- **ENT-5** Every other entry is matched according to §5.4. This includes tapes, covers, and entries with notes such as "snippet", "partial" or "acoustic".
- **ENT-6** setlist.fm **notes** on an entry are shown in the preview but **never affect matching**.
- **ENT-7** **Duplicates** are kept. If the same song appears more than once, each occurrence is matched and placed in its own position.

### 5.4 Spotify matching

**Who the expected artist is:**

- **MATCH-1** For a normal entry, and for a tape that setlist.fm doesn't attribute to another artist, the expected artist is the **band**.
- **MATCH-2** For a **cover** (including a tape attributed to another artist), matching runs in two passes:
  1. Look for the **band's own recording** (expected artist = band).
  2. If there isn't one, look for the **original artist's** recording (expected artist = original artist). A match from this pass is labelled as a **cover fallback**.

**What counts as a candidate.** A Spotify track is a candidate only if **both** of these are true:

- **MATCH-3 (artist)** One of the track's credited artists has a normalised name equal to the expected artist's normalised name.
- **MATCH-4 (title)** The track title equals the entry name after normalising both, which means:
  - ignoring case, accents and punctuation;
  - treating `&` and `and` as the same;
  - removing **version suffixes**, i.e. trailing parenthesised, bracketed or dash-separated markers such as "Remastered", "2015 Remaster", "Live", "Live at …", "Single Version", "Explicit", "Mono", "Stereo", "Deluxe", "‹year› Version".

  Any other difference means no candidate (e.g. "One" ≠ "One More Time").

**Exclusions:**

- **MATCH-5** A candidate is **excluded** if its version markers (title suffix or album title) identify it as a **demo, remix, instrumental, acoustic, karaoke or tribute** version.

**Choosing between candidates**, in order:

- **MATCH-6** Studio versions are preferred over **live** versions. A version is live if its title suffix contains "Live", or its album title contains "Live" as a word. A live version can be chosen only if no studio candidate exists, and it's labelled **Live version**.
- **MATCH-7** Among the remaining candidates: **album** releases come before **singles or EPs**, and singles or EPs come before **compilations**. (Spotify doesn't distinguish EPs from singles.)
- **MATCH-8** If there's still a tie, the **earliest release date** wins. This means re-recordings lose to originals but are used if they're the only version.

**No guesses:**

- **MATCH-9** If there is no candidate after the steps above, the entry is **Not found**. The app never falls back to a closest guess, a title-only search, or a different artist from the one expected.

*Known limitation:* an album whose title happens to contain "Live" (e.g. "Long Live Rock 'n' Roll") may have its tracks treated as live versions. This affects only which version is preferred, never whether the artist is correct, and is accepted for the MVP.

## 6. Preview

- **PREV-1** While matching runs, the screen shows a simple "Matching songs…" message. There's no per-song progress.
- **PREV-2** The top of the preview shows the band, show date, venue, city and tour name (if any). It also has a link to the show's page on setlist.fm, and a summary: "‹found› of ‹matchable› songs found".
- **PREV-3** Entries are listed in setlist order. Each set after the main set is introduced by a heading using setlist.fm's set name (e.g. "Encore 1"). Skipped entries are not shown.
- **PREV-4** Each row shows: position number, the entry name as on setlist.fm, any setlist.fm note (as secondary text), and its status:
  - **Found**: plus the matched Spotify track's title, artist(s) and album.
  - **Found, cover fallback**: labelled "Cover → ‹original artist›", plus the track details.
  - **Found, live version**: labelled "Live version", plus the track details.
  - **Not found** or **Not found (medley)**.
  - Tape entries additionally carry a **"Tape"** label, whatever their status.
- **PREV-5** The preview is **read-only**. Tracks can't be removed, reordered, swapped or renamed, and there are no audio previews.
- **PREV-6** The preview has a **"Back to shows"** action, which returns to the band's show list, and a **"Create playlist"** action.
- **PREV-7** If no entries were found, "Create playlist" is **disabled** and the preview says "None of these songs were found on Spotify." An empty playlist is never created.

## 7. Playlist creation

- **PL-1** "Create playlist" creates a **new** playlist in the logged-in user's Spotify library every time. It never updates or reuses an existing playlist.
- **PL-2** **Name:**
  - With a tour name: `‹Band› – ‹Tour Name› (Setlist)`, e.g. `Gojira – Mea Culpa Tour (Setlist)`.
  - Without a tour name: `‹Band› – ‹City›, ‹DD Mon YYYY›`, e.g. `Gojira – Leipzig, 14 Mar 2026`.
  - The name can't be edited in the app.
- **PL-3** **Description:** `‹Venue›, ‹City›, ‹DD Mon YYYY›. Setlist: ‹setlist.fm URL of the show›. Created by Setlisted.`
- **PL-4** The playlist is **private** and keeps Spotify's default cover.
- **PL-5** **Contents:** every found entry (§6, any "Found" status), in setlist order, with duplicates kept. Not-found entries and medleys are left out. Set and encore boundaries are not reflected.
- **PL-6** "Create playlist" is disabled as soon as it's pressed. One press creates at most one playlist.

### 7.1 Confirmation

- **PL-7** On success, the app shows: "Playlist created: ‹playlist name›, ‹n› tracks".
- **PL-8** The confirmation has an **"Open in Spotify"** link to the new playlist. On a phone with Spotify installed, it should open the Spotify app. Otherwise it opens Spotify on the web.
- **PL-9** The confirmation has a **"Search another band"** action, which returns to an empty search screen. The confirmation doesn't repeat the list of unmatched entries.

## 8. Errors

- **ERR-1** If setlist.fm or Spotify can't be reached or returns an error, the app shows a plain message naming the service (e.g. "Couldn't reach setlist.fm. Try again."), with a **Retry** action that repeats the failed step.
- **ERR-2** If setlist.fm's rate limit is hit, the app waits briefly and retries on its own. If that still fails, the user sees ERR-1's message. The words "rate limit" are never shown to the user.
- **ERR-3** If playlist creation fails **after** the playlist has been created (e.g. adding tracks fails), the app says creation failed and provides a link to the partially created playlist. It does not delete or roll anything back. Retrying creates a **new** playlist (PL-1).
- **ERR-4** If playlist creation fails before anything is created, the app says so and offers Retry.
- **ERR-5** No error message shows technical details (status codes, stack traces, raw responses).

## 9. Acceptance

The MVP meets this spec when every requirement above is true. The intent's success criteria are checked as follows:

| Intent success criterion | How it is checked |
|---|---|
| 1. Under a minute end to end | The owner times the journey for a typical headliner: open the app (logged in), search, pick a show, preview, create, open in Spotify. Under 60 seconds. |
| 2. Friends can use it | At least 2 friends, unaided, log in and create a playlist in their own library. |
| 3. Matching quality | On the owner's ~10-setlist test set (including at least one with covers, one with a tape and one with a medley): found / matchable ≥ 90% overall; zero tracks credited to the wrong artist (MATCH-3); every cover-fallback, live, tape, not-found and medley entry labelled as in PREV-4. |
| 4. Phone and laptop | The full journey is completed on the owner's phone and laptop, in the browsers they actually use. |
| 5. Access control | A non-allowlisted account sees the AUTH-4 message. A logged-out visitor can't reach any search, show, preview or creation screen (AUTH-1). |
| 6. Cost | One month's running cost is within the intent's budget. |
| 7. Real use | The owner has used it to prepare for at least one real gig. |
