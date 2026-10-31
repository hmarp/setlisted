# Step 05 setup: setlist.fm API key (owner)

Owner tasks for step 05. Commands are for **Git Bash on Windows**, run from the repository root. Tick each box as you go.

Nothing here changes code. Sections 1–3 can be done before the PR merges, and section 3 **must** be done before the merge, so the revision the merge deploys reads the real key.

## 1. The API key and its rate limit

- [x] Open your [setlist.fm API settings](https://www.setlist.fm/settings/api) (log in to setlist.fm first). If there's no key yet, apply for one there: it's free for non-commercial use.
- [x] Note the **rate limit** shown for the key and send Claude the numbers (they aren't secret) so they go into the handoff. Done (2026-10-31): **max. 2.0 requests per second and max. 1,440 per day**. The limiter spaces calls **600 ms** apart (about 1.7 per second).
  - If your key's limit is different, tell Claude: the interval is one constant (`setlistfm.DefaultInterval`).
- [x] Copy the **API key**. Never paste it into a commit, an issue, a PR or a chat.

## 2. Key into the local `.env`

- [x] In your local `.env`, set:

```dotenv
SETLISTFM_API_KEY=<API key>
```

## 3. Key into Secret Manager (before the merge)

The `setlistfm-api-key` secret has a placeholder version from step 02. Merges deploy a new revision, which reads `setlistfm-api-key:latest` when it starts, so adding a version is all that's needed: no service update.

```bash
PROJECT_ID=setlisted-2026
gcloud config set project "$PROJECT_ID"
```

- [x] Add a new version (hidden prompt, nothing echoed or saved in history). Done by the owner in the console (2026-10-31):

```bash
read -rs -p "setlist.fm API key: " v && printf '%s' "$v" | gcloud secrets versions add setlistfm-api-key --data-file=-; unset v; echo
gcloud secrets versions list setlistfm-api-key
```

  Paste at the hidden prompt with **Shift+Insert** or a **right-click** (in Git Bash, Ctrl+V doesn't paste; it can insert an invisible character), then press Enter.

- [ ] **Check the stored value without showing it.** The fingerprint must match the key in your local `.env`:

```bash
gcloud secrets versions access latest --secret=setlistfm-api-key | wc -c      # the key's length, with no newline counted
gcloud secrets versions access latest --secret=setlistfm-api-key | sha256sum
grep '^SETLISTFM_API_KEY=' .env | cut -d= -f2- | tr -d '\r\n' | sha256sum   # must match the line above
```

  If they don't match, add another version (the `read` command again), check again, then disable the bad version: `gcloud secrets versions disable <number> --secret=setlistfm-api-key`.

- [ ] After the merge's revision is serving (section 5), disable the placeholder version (usually `1`; check the list):

```bash
gcloud secrets versions disable 1 --secret=setlistfm-api-key
```

In the console: the [secret's versions page](https://console.cloud.google.com/security/secret-manager/secret/setlistfm-api-key/versions?project=setlisted-2026).

## 4. Local check (with Claude, before merging)

On the step 05 branch, with the real key in `.env`:

```bash
go run ./cmd/setlisted
```

- [x] Open <http://127.0.0.1:8080/> and log in with Spotify.
- [x] In the same browser, open each of these and check the JSON looks sensible (tell Claude what you see; you don't need to paste it all): Done: searching works (2026-10-31).
  - <http://127.0.0.1:8080/api/artists?q=gojira>: `"match":"clear"` with one artist. Copy its `id`.
  - <http://127.0.0.1:8080/api/artists?q=ghost>: `"match":"picker"`, up to 10 artists, several called Ghost with different disambiguations.
  - <http://127.0.0.1:8080/api/artists?q=sword>: `"picker"` (The Sword isn't a clear match for `sword`).
  - <http://127.0.0.1:8080/api/artists?q=qwxzvbnm>: `"match":"none"`.
  - `http://127.0.0.1:8080/api/artists/<id>/shows` with the Gojira `id`: up to 20 shows, newest first, dates like `14 Mar 2026`, song counts above 0, no future (empty) shows.
- [ ] Logged out (Log out on `/`), the same URLs answer `401 not_logged_in`.
- [ ] The terminal's log lines show no search text and no key.

## 5. Cloud Run check (after the merge deploys)

- [ ] The **Deploy** run for the merge commit passed ([Actions → Deploy](https://github.com/hmarp/setlisted/actions/workflows/deploy.yml)).
- [ ] Log in at <https://setlisted-613938414647.europe-west1.run.app/>, then open <https://setlisted-613938414647.europe-west1.run.app/api/artists?q=gojira> and `https://setlisted-613938414647.europe-west1.run.app/api/artists/<id>/shows`. Same results as locally.
- [ ] If either returns `503 upstream_unavailable`, the revision probably has the placeholder key. Check the [service's logs](https://console.cloud.google.com/run/detail/europe-west1/setlisted/logs?project=setlisted-2026) for `setlist.fm call failed`: `status 401` or `status 403` means a bad key. Fix the secret (section 3), then start a new revision so it reads `latest`:

```bash
gcloud run services update setlisted --region=europe-west1 --update-labels=refreshed=$(date +%s)
```
