# Manual test checklist

The whole journey, run by hand on the owner's **phone** and **laptop**, in the browsers they actually use ([ADR-006](adr/006-testing-strategy.md): there are no automated UI tests). Step 12 wrote it and step 13 runs it for acceptance. Run it after any change to the UI.

- **Where:** against Cloud Run, at exactly <https://setlisted-613938414647.europe-west1.run.app>. For a local run use `http://127.0.0.1:8080` (not `localhost`) in Chrome, Edge or Firefox.
- **What it creates:** sections 6–7 create real private playlists in your Spotify library. Delete them in Spotify afterwards, because the app never deletes anything.
- **Bands:** the examples are only suggestions. Any band that gives the described result will do. The example results come from setlist.fm today and may change.
- **Recording results:** tick each box, or note what happened instead. Copy the filled-in table at the end into the handoff or the step 13 results.

Each check names the spec requirement it covers ([`spec.md`](spec.md)).

## 0. Every screen

Check these on **every screen** you pass through (keep an eye on them throughout):

- [ ] The footer reads "Setlist data from setlist.fm" and "setlist.fm" links to <https://www.setlist.fm> (GEN-3).
- [ ] On the phone: there's no sideways scrolling and no need to zoom. Text is readable and buttons are easy to tap (GEN-1).
- [ ] Error messages never show status codes, technical text or raw responses (ERR-5). The words "rate limit" never appear (ERR-2).
- [ ] Laptop only: DevTools (F12) → Console shows no errors in red during the normal journey (sections 1–7). Red "Failed to load resource" lines are expected only in section 8, where failures are caused on purpose.

## 1. Logged out

1. [ ] In a fresh private/incognito window, open the app. You see only "Setlisted", the one-line description, **Log in with Spotify** and the footer. There's no Log out, search box or any band data (AUTH-1).
2. [ ] Open `…/#/artist/5a85c140-dcf8-4a4f-b8d6-1e5a2a4e0f4b` directly. You still see only the login screen, and the address bar goes back to `/` (AUTH-1).

## 2. Login

1. [ ] **Cancel:** tap **Log in with Spotify**, then on Spotify's page tap **Cancel** (on the consent screen; if Spotify skips it because you've already agreed, remove the app under Spotify account → Apps first). You're back on the login screen with "Login was cancelled." The address bar shows no `?login=` (AUTH-5).
2. [ ] **Invite-only** (needs a Spotify account that isn't in the app's User Management list; skip if there isn't one, and write "not tested"): log in with it. You see "Setlisted is invite-only. Ask the owner to add your Spotify account." and nothing from Spotify's own error (AUTH-4).
3. [ ] **Log in** with your allowlisted account. You land on the search screen, with **Log out** at the top (AUTH-3, AUTH-7).

## 3. Search

1. [ ] With the box empty, or only spaces, **Search** is greyed out and pressing Enter does nothing (SRCH-1).
2. [ ] **No results:** search `qwxzqwxz`. You see "No artists found for 'qwxzqwxz' on setlist.fm." and the text is still in the box to edit (SRCH-5).
3. [ ] **Picker:** search a name shared by several artists (e.g. `Ghost`, or a word such as `Sword`). You see up to 10 artists in a list, each with setlist.fm's description under its name where there is one. There's no paging (SRCH-4).
4. [ ] Choose one. You go to its show list (SRCH-4).
5. [ ] Browser **Back** returns to the picker (back-button support, nice-to-have).
6. [ ] **Clear match:** search a band with one exact name, e.g. `Gojira`. You go straight to its show list, with no picker (SRCH-3).

## 4. Show list

1. [ ] It's headed with the band's name, and lists up to 20 shows, newest first (SHOW-1).
2. [ ] Each show has its date as `DD Mon YYYY`, the city, the venue, the tour name (when there is one) and the song count (SHOW-3).
3. [ ] **No setlists:** find a band with no setlists (e.g. a small local band from the picker whose shows have no songs). You see "No setlists found for ‹band› on setlist.fm." (SHOW-5). Note the band used: ______
4. [ ] **New search** returns to the search screen (SHOW-6).

## 5. Preview

Pick a show with **covers, a tape (intro or outro), a medley and an encore**. A big metal headliner's recent show usually has most of them; you can check on setlist.fm first. Note the show: ______

1. [ ] While it loads, the screen shows only "Matching songs…" with no per-song progress (PREV-1).
2. [ ] The top shows the band, date, venue, city and tour name (if any), a "This show on setlist.fm" link that opens the right show, and "‹found› of ‹matchable› songs found" (PREV-2).
3. [ ] Songs are in setlist order and numbered. The main set has no heading, and each later set has setlist.fm's name for it (e.g. "Encore 1"). Solos, jams and other skipped entries don't appear and don't take a number (PREV-3).
4. [ ] Each row shows its number, the name as on setlist.fm, any setlist.fm note in grey, and its status (PREV-4):
   - [ ] **Found**, with the Spotify track's title, artist(s) and album;
   - [ ] **Cover → ‹original artist›** with the track details, for a cover found under the original artist;
   - [ ] **Live version** with the track details, where only a live recording exists;
   - [ ] **Not found**, and **Not found (medley)** for a medley;
   - [ ] **Tape** as well, on tape entries, whatever their status.
5. [ ] Nothing can be edited, reordered or played (PREV-5).
6. [ ] **Back to shows** returns to the same band's show list (PREV-6).
7. [ ] **Nothing found** (hard to find live; skip if you can't, and write "not tested"): a show where no song is on Spotify says "None of these songs were found on Spotify." and **Create playlist** is greyed out (PREV-7).

## 6. Create a playlist

1. [ ] On the preview from section 5, tap **Create playlist** twice quickly. The button goes grey straight away and "Creating playlist…" shows (PL-6).
2. [ ] You see "Playlist created: ‹name›, ‹n› tracks" (PL-7). `n` equals the "found" number in the preview's summary.
3. [ ] In Spotify there is exactly **one** new playlist for that press (PL-6). It's private, with the name and description from the spec (PL-2 to PL-4), and its tracks are in setlist order.
4. [ ] The confirmation doesn't list the songs that weren't found (PL-9).
5. [ ] **Open in Spotify** (PL-8):
   - [ ] **phone** (Spotify app installed): it opens the playlist in the Spotify app;
   - [ ] **laptop:** it opens the playlist in Spotify on the web (or the desktop app, if the browser hands it over).
6. [ ] Go back to the app. **Search another band** shows the search screen with an empty box (PL-9).

## 7. Log out

1. [ ] Tap **Log out** on any screen. You're on the login screen (AUTH-7).
2. [ ] Open `…/api/session` in the same browser. It says `{"loggedIn":false}` (AUTH-7).

## 8. Errors and Retry

**Laptop** (Chrome or Edge DevTools; Firefox has the same under Network → block URL):

1. [ ] Log in and open a show list. In DevTools → Network, set throttling to **Offline**. Choose a show. You see "Couldn't reach Setlisted. Check your connection and try again." with **Retry** and **Back to shows**.
2. [ ] Set Network back to **No throttling** and tap **Retry**. The preview loads (ERR-1: Retry repeats the failed step).
3. [ ] On a preview, go **Offline** and tap **Create playlist**. You see the error with **Retry**, and **Create playlist** is enabled again. Go back online and tap **Retry**. Exactly one playlist is created (ERR-4, PL-6).
4. [ ] Search the same way while offline, then go online and use Retry. The search results appear.

**Phone:** on a show list, switch on aeroplane mode and choose a show. You see the connection message with **Retry**. Switch aeroplane mode off, tap **Retry**, and the preview loads.

The setlist.fm and Spotify messages ("Couldn't reach setlist.fm. Try again.", "Couldn't reach Spotify. Try again.") and the part-way failure (ERR-3: "Creating the playlist failed part-way…", a link to the playlist and "Try again (creates a new playlist)") can't be caused on purpose against the live services. They were checked against fake services in step 12 (see its handoff). If one happens for real, note what you saw.

## 9. Session expiry (AUTH-6, AUTH-8)

Do at least one of these:

1. [ ] **Delete the cookie** (laptop): DevTools → Application → Cookies → the app's address → delete `setlisted_session`. Then search for something. You're sent to the login screen. After logging in you start on the search screen, not where you were.
2. [ ] **Close the browser** completely (every window; on a phone, swipe the browser away), reopen the app. You see the login screen. (Some browsers restore session cookies with "continue where you left off". Note it if yours does: the one-hour limit still applies.)
3. [ ] **Wait over an hour** after logging in, then search or choose a show. You're sent to the login screen.

## Results

| Section | Phone (browser: ______) | Laptop (browser: ______) | Notes |
|---|---|---|---|
| 0 Every screen | | | |
| 1 Logged out | | | |
| 2 Login | | | |
| 3 Search | | | |
| 4 Show list | | | |
| 5 Preview | | | |
| 6 Create | | | |
| 7 Log out | | | |
| 8 Errors | | | |
| 9 Expiry | | | |

Date: ______ · App revision (`gcloud run services describe setlisted --region=europe-west1 --format='value(status.latestReadyRevisionName)'`): ______
