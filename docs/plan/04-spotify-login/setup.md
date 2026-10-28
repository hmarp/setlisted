# Step 04 setup: Spotify app and credentials (owner checklist)

The owner runs these steps. They need the owner's Spotify account and Google Cloud credentials. Claude doesn't run them.

- "Spotify Dashboard" means the **Spotify for Developers dashboard**, <https://developer.spotify.com/dashboard>, logged in with the owner's Spotify account.
- "Console" means the **Google Cloud console**, <https://console.cloud.google.com>, signed in with the same Google account as `gcloud auth login`.
- Commands run in **Git Bash** (on Windows, not the WSL `bash`), from the repository root.

Spotify's development-mode rules (February 2026): the app owner needs **Spotify Premium**, at most **5 users** can be allowlisted, and `GET /me` no longer returns email.

**Order:** sections 1–4 any time before the local check; section 5 (local check, with Claude) before the PR is merged; section 6 (Cloud Run settings) before **or** after the merge; section 7 after the merge has deployed.

## 1. Create the Spotify app

- [ ] In the Spotify Dashboard, click **Create app** (skip if the app already exists, and go to its **Settings** to check the values below).
  - **App name:** `Setlisted`
  - **App description:** "Turns a setlist.fm setlist into a private Spotify playlist."
  - **Website:** `https://setlisted-613938414647.europe-west1.run.app` (optional)
  - **Redirect URIs:** see section 2.
  - **Which API/SDKs are you planning to use?** tick **Web API** only.
  - Accept the Developer Terms and click **Save**.

One app serves both local development and production (ADR-007).

## 2. Register both redirect URIs

- [ ] In the app's **Settings → Basic Information → Edit**, under **Redirect URIs**, add each of these exactly (no trailing slash), clicking **Add** after each, then **Save**:

```text
http://127.0.0.1:8080/auth/callback
https://setlisted-613938414647.europe-west1.run.app/auth/callback
```

- Spotify doesn't accept `localhost`, so local development uses `127.0.0.1`. Open the app at `http://127.0.0.1:8080`, **not** `http://localhost:8080`: with `localhost` the redirect URI and the `Origin` check won't match.
- If you run locally on another port, register that port's URI too, and set `PORT` and `BASE_URL` to match.

## 3. Allowlist users

- [ ] In the app's **User Management** tab, add each person who may use Setlisted: their **name** and the **email address of their Spotify account**. Add yourself (the owner) first, then up to 4 friends.
- Anyone not on this list can get through Spotify's login screen, but Setlisted then shows the invite-only message and creates no session (AUTH-4).
- **The app owner's own account is always allowed**, listed or not. To test invite-only, use a second Spotify account that isn't on the list.

## 4. Client ID and secret into the local `.env`

- [ ] In **Settings → Basic Information**, copy the **Client ID**, and click **View client secret** to see the secret.
- [ ] In your local `.env` (copy `.env.example` if you don't have one), set:

```dotenv
BASE_URL=http://127.0.0.1:8080
SPOTIFY_CLIENT_ID=<client ID>
SPOTIFY_CLIENT_SECRET=<client secret>
```

`.env` is git-ignored. Never paste the secret into a commit, an issue, a PR or a chat.

- [x] **Send Claude the Client ID** (it isn't secret) so it can be committed in `scripts/deploy-config.sh` in this PR. Done: `2d7607fbbdd9462892990e71494eefd9`. If the app is ever recreated, update that file and section 6.

## 5. Local check (with Claude, before merging)

On the step 04 branch:

```bash
go run ./cmd/setlisted
```

- [ ] Open <http://127.0.0.1:8080/>. It says "Not logged in."
- [ ] Click **Log in with Spotify**, log in and accept. You're back on `/` and it says "Logged in."
- [ ] In the browser's developer tools (F12 → Application/Storage → Cookies → `http://127.0.0.1:8080`), the `setlisted_session` cookie is there with **HttpOnly** and **Secure** ticked, **SameSite** `Lax`, and **Expires** "Session". Note which browser and version you used: this confirms (or not) that the browser accepts a `Secure` cookie over plain HTTP on `127.0.0.1`.
- [ ] <http://127.0.0.1:8080/api/session> shows `{"loggedIn":true}`.
- [ ] Click **Log out**. It says "Not logged in." and the cookie is gone.
- [ ] Log in again, and click **Cancel** on Spotify's consent screen if it's shown (it's shown the first time, or after you remove the app under <https://www.spotify.com/account/apps/>). You land on `/?login=cancelled`.
- [ ] If a Spotify account that is **not** allowlisted is available (for example a second free account, in a private window): logging in with it lands on `/?login=invite_only`, and `/api/session` says `{"loggedIn":false}`. If none is available, say so; the handoff then records that this was covered by tests only.

Tell Claude the results, including the browser and version.

## 6. Cloud Run settings

Merges deploy **only a new image**: the service keeps its env vars and secret references (step 03 handoff). So the real client ID and secret are set once, by hand.

Set these in the shell first:

```bash
PROJECT_ID=setlisted-2026
REGION=europe-west1
gcloud config set project "$PROJECT_ID"
```

- [ ] **Client secret:** add a new version of `spotify-client-secret` (hidden prompt, nothing echoed or saved in history):

```bash
read -rs -p "Spotify client secret: " v && printf '%s' "$v" | gcloud secrets versions add spotify-client-secret --data-file=-; unset v; echo
gcloud secrets versions list spotify-client-secret
```

  Paste at the hidden prompt with **Shift+Insert** or a **right-click** (in Git Bash, Ctrl+V doesn't paste; it can insert an invisible character), then press Enter.

- [ ] **Check the stored value without showing it.** It should be 32 characters, with the same fingerprint as the secret in your local `.env`:

```bash
gcloud secrets versions access latest --secret=spotify-client-secret | wc -c      # expect 32
gcloud secrets versions access latest --secret=spotify-client-secret | sha256sum
grep '^SPOTIFY_CLIENT_SECRET=' .env | cut -d= -f2- | tr -d '\r\n' | sha256sum   # must match the line above
```

  If they don't match, add another version (the `read` command again), check again, then disable the bad version: `gcloud secrets versions disable <number> --secret=spotify-client-secret`.

- [ ] **Client ID:** set it on the live service. This starts a new revision of the image that's already deployed, which also reads the new secret version (`spotify-client-secret:latest`):

```bash
gcloud run services update setlisted --region="$REGION" \
  --update-env-vars SPOTIFY_CLIENT_ID=2d7607fbbdd9462892990e71494eefd9
```

  (Alternatively, once `scripts/deploy-config.sh` has the real ID and is committed, run `scripts/deploy.sh` once from a clean checkout of `main`. It re-applies every setting.)

- [ ] Check the service has the new ID and is healthy:

```bash
gcloud run services describe setlisted --region="$REGION" --format='value(spec.template.spec.containers[0].env)'
curl -s https://setlisted-613938414647.europe-west1.run.app/api/health
```

  The env list shows the real `SPOTIFY_CLIENT_ID`; health returns `{"status":"ok"}`.

- [ ] Disable the old placeholder version (keeps active versions within the free tier). Check its number in the list above first; it's normally `1`:

```bash
gcloud secrets versions disable 1 --secret=spotify-client-secret
```

  Do this only after the new revision is serving: a revision that is starting reads `latest`, so disabling the old version doesn't affect it.

In the console the same settings are on the [Cloud Run service page](https://console.cloud.google.com/run/detail/europe-west1/setlisted/revisions?project=setlisted-2026) (**Edit & deploy new revision → Variables & secrets**) and the [secret's page](https://console.cloud.google.com/security/secret-manager/secret/spotify-client-secret/versions?project=setlisted-2026).

## 7. Cloud Run check (after the merge deploys)

- [ ] The **Deploy** run for the merge commit passed ([Actions → Deploy](https://github.com/hmarp/setlisted/actions/workflows/deploy.yml)).
- [ ] Repeat section 5 on <https://setlisted-613938414647.europe-west1.run.app/>: log in, "Logged in.", cookie attributes, `/api/session`, log out. (The cancel and invite-only checks are optional here.)

## Troubleshooting

| What you see | Likely cause |
|---|---|
| Spotify says **"INVALID_CLIENT: Invalid redirect URI"** | The URI isn't registered exactly (scheme, host, port, path, no trailing slash), or you opened the app on `localhost` instead of `127.0.0.1`. |
| Spotify says **"INVALID_CLIENT: Invalid client"** | `SPOTIFY_CLIENT_ID` is wrong or still a placeholder (locally in `.env`; on Cloud Run, section 6). |
| You come back to `/?login=failed` | Often a wrong client secret (the token exchange is refused). The app logs `login failed` with a `reason` (and Spotify's status code). Locally it's on the terminal; on Cloud Run, in [Logs Explorer](https://console.cloud.google.com/logs/query?project=setlisted-2026). The app's own logs never contain tokens or codes. |
| `/?login=failed` straight after clicking login | The login cookie didn't survive the round trip: more than 10 minutes on Spotify's page, cookies blocked, or the app was opened on a different host than `BASE_URL`. |
| `/?login=invite_only` for an allowlisted user | The email in User Management doesn't match their Spotify account, or the app owner's Premium lapsed. |
| Log out does nothing; `POST /auth/logout` returns `403` | The page's origin isn't `BASE_URL` (for example `localhost` instead of `127.0.0.1`). |
