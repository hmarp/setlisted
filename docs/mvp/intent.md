# Intent

This document covers the MVP of Setlisted.

## Problem

The owner goes to a lot of heavy metal concerts. Before a gig, they look up the band's recent setlists on setlist.fm, choose one from the current or a recent tour, and build a Spotify playlist of it by hand. They listen to the playlist in the weeks before the gig to learn the likely set. Setlists are nearly identical across shows on the same tour, so one recent setlist is a good guide to what will be played.

Building the playlist by hand takes 10–15 minutes and is fiddly. Setlisted should turn "recent setlist on setlist.fm" into "playlist in my Spotify" in under a minute.

The main use is **before** a gig, to learn the set. Reliving a past gig is not the goal.

## Users

- The owner and a handful of friends.
- Each user signs in with **their own Spotify account**, and playlists are created in **their own** Spotify library.
- "Log in with Spotify" is the only way in. The app has no accounts, passwords or user management of its own.
- Access is controlled by the Spotify developer-dashboard allowlist, which the owner manages by hand outside the app.
- Users use the app on both laptops and phones, through a web browser.

## Core user journey

1. **Log in.** The user opens the app and logs in with Spotify. Nothing else, including search, is available before login. A Spotify account that isn't allowlisted sees a friendly invite-only message ("ask the owner for access") rather than a raw Spotify error.
2. **Search for a band.** The user types a band name.
   - If there is one clear match, the app goes straight to that band's shows.
   - Otherwise the user picks the right artist from a list of matches, including any disambiguation text setlist.fm provides.
3. **Pick a show.** The app lists the band's most recent shows (about 20, newest first). Each shows its date, city, venue, tour name (if any) and **song count**.
   - Shows with no songs, including announced future shows, are hidden or can't be selected.
   - If the band has no setlists at all, the app says so clearly.
   - The shows list is enough to confirm the right band was chosen. No extra artist details are needed.
   - The app does not try to detect partial setlists. The user judges this from the song counts.
4. **Preview.** The app shows the setlist in order, read-only. Each entry is marked as one of:
   - found on Spotify
   - found, but using the original artist's version of a cover (fallback)
   - tape entry (found or not found)
   - not found

   If the setlist looks thin or wrong, the user can go back and pick another show.
5. **Create.** The user presses "Create playlist". A new private playlist appears in their Spotify library, and the app confirms this with a link to it.

### Matching rules

- **A wrong match is worse than a missing one.** If the app isn't confident, it marks the entry "not found" rather than guessing.
- **Songs** match the **studio original by the performing artist**. Any edition or remaster is acceptable. Live, demo, remix, karaoke and tribute versions should be avoided, but a live version by the performing artist is an acceptable fallback if no studio version exists.
- **Covers** (as marked on setlist.fm) use the performing band's own recording if one exists. Otherwise they fall back to the original artist's version, which is labelled as a fallback in the preview.
- **Tape entries** (recorded music played over the PA, as marked on setlist.fm) are included and matched like normal songs, and labelled as tape in the preview.
- **Solos, jams and unnamed entries** (e.g. "Drum Solo") are skipped.
- **Medleys** are shown in the preview as not found.
- **Duplicates** are kept. If a song appears twice in the setlist, it appears twice in the playlist, in the same positions. The playlist is a faithful copy of the setlist's running order.

### Playlist contents and naming

- **Name:** `<Band> – <Tour Name> (Setlist)`. If the show has no tour name: `<Band> – <City>, <DD Mon YYYY>`.
  - e.g. `Gojira – Mea Culpa Tour (Setlist)`
- **Description:** the source show (venue, city, date), a link to the setlist on setlist.fm, and "Created by Setlisted".
- **Visibility:** private.
- **Cover image:** Spotify's default.
- **Every creation makes a new playlist.** Existing playlists are never updated or reused.
- The name is automatic and can't be edited in the app. Users can rename in Spotify.

## Scope

### In

- Log in with Spotify (the only login, and required before anything else)
- Invite-only message for accounts that aren't allowlisted
- Band search with artist disambiguation, skipped when there's one clear match
- List of recent shows with date, city, venue, tour and song count, with empty shows excluded
- Read-only setlist preview with each entry's match status
- Spotify matching according to the matching rules above
- Creating a private playlist in the user's own Spotify account, named and described as above
- setlist.fm attribution in the app
- Works in both mobile and laptop web browsers
- An **extremely basic UI**: plain, functional screens with no visual design work. Usable on a phone, but not polished.

### Out

1. Multi-band / full line-up playlists (headliner plus supports). **First candidate after the MVP.**
2. Editing the playlist in the app (removing, reordering, swapping tracks, renaming)
3. Choosing a different Spotify version of a matched track
4. Splitting medleys into their songs
5. A "typical setlist" built from several shows
6. Browsing or filtering older tours/shows beyond the recent list
7. Updating or syncing an existing playlist rather than creating a new one
8. A history of playlists created in the app (Spotify is the history)
9. Searching by venue, city, festival or date rather than by band
10. Streaming services other than Spotify
11. Native mobile apps (the web app must still work well in a mobile browser)
12. In-app user management (the allowlist is managed in the Spotify developer dashboard)
13. Visual design and UI polish (branding, styling, layout refinement). This comes after the functionality is complete.

## Constraints

- **Spotify development mode.** Only users the owner has allowlisted in the Spotify developer dashboard can log in. The allowlist's size limit is accepted as enough for this user base. Since February 2026, the app owner must keep an **active Spotify Premium subscription**, or the app stops working for all users (the owner has Premium). Getting out of development mode is not realistic for this project. No commercial use.
- **setlist.fm API.** Free for **non-commercial** use only. Requires **attribution** to setlist.fm in the app, with links back. Rate-limited per API key. Expected usage is far below the limits, but it rules out background or bulk fetching.
- **Permanently non-commercial.** No ads, no charging, no donation links.
- **No stored user data.** The app does not keep any data about users, their searches or their playlists. The only exception is whatever is strictly needed to keep a user logged in during a session.
- **Cost.** The target is £0/month, and a few pounds a month is the most acceptable. A custom domain is optional.
- **Availability.** No uptime target. "Works when I use it" is enough, and a slow first load after a period of inactivity is acceptable.
- **Usage.** A handful of playlists per user per month.
- **Devices.** It must be usable in current mobile and desktop browsers.

## Open questions

1. **What "stored" means for a login session.** Keeping a user logged in needs some session state, such as Spotify tokens held for the length of a session. The spec/ADRs need to decide the minimum that is acceptable under "no stored user data" (e.g. session-only, nothing persisted across sessions or in a database), and whether users have to log in again on each visit. *Resolved in the spec (AUTH-6 to AUTH-8): a login lasts for the browser session only, and nothing is persisted.*
2. **Matching test set.** The owner needs to pick about 10 real, recent metal setlists for success criterion 3. They should include at least one with covers, one with a tape intro and one with a medley.

## Success criteria

The MVP is done when:

1. **Under a minute from start to finish.** For a typical headliner, going from opening the app (already logged in) to having the playlist in Spotify takes under 60 seconds of the user's time.
2. **Friends can use it.** At least 2 friends have logged in with their own Spotify account and created a playlist in their own library without the owner's help.
3. **Matching quality.** On the owner's test set of about 10 recent metal setlists:
   - at least 90% of matchable songs are found
   - there are **no** wrong-artist matches
   - every unmatched, cover-fallback and tape entry is correctly labelled in the preview
4. **Works on the owner's phone and laptop**, in the browsers they actually use.
5. **Access control holds.** A non-allowlisted Spotify account sees the invite-only message, and nothing in the app is usable without logging in.
6. **Cost.** It has run for a month within budget.
7. **Real use.** The owner has used it to prepare for at least one actual gig instead of building the playlist by hand.
