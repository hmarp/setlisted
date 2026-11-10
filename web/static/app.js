// Setlisted throwaway UI (ADR-002). Plain JS on the JSON API in docs/api.md.
//
// One page, one <section> per view; show() unhides one of them. The address
// bar's hash says where the user is, so the browser's Back button works:
//
//   #/                         search (empty)
//   #/search/<text>            search results (picker or "no artists")
//   #/artist/<id>              the artist's shows
//   #/artist/<id>/show/<id>    the show's preview
//
// The confirmation isn't in the hash: it only exists right after a create.
"use strict";

const $ = (id) => document.getElementById(id);

const LOGIN_MESSAGES = {
  cancelled: "Login was cancelled.",
  invite_only: "Setlisted is invite-only. Ask the owner to add your Spotify account.",
  failed: "Login didn't work. Try again.",
};
// No answer at all (offline, server down): there's no API message to show.
const NETWORK_MESSAGE = "Couldn't reach Setlisted. Check your connection and try again.";
// An answer that isn't the API's JSON (e.g. a proxy error page).
const GENERIC_MESSAGE = "Something went wrong. Try again.";

const VIEWS = ["login", "search", "shows", "preview", "done"];

let loggedIn = false;
// Each navigation gets a new generation; answers to older ones are ignored,
// and their fetches aborted.
let generation = 0;
let controller = null;
// The last successful search and show list, so Back doesn't re-ask setlist.fm.
let lastSearch = null; // {q, data}
let lastShows = null; // {artistId, data}
// The preview on screen, for Create playlist.
let preview = null;

// LoggedOut is thrown by api() after a 401 has switched to the login view.
class LoggedOut extends Error {}

function isAbort(e) {
  return e && e.name === "AbortError";
}

// newRequest starts a navigation: it aborts the previous one's fetch.
function newRequest() {
  generation++;
  if (controller) controller.abort();
  controller = new AbortController();
  const mine = generation;
  return { signal: controller.signal, current: () => mine === generation };
}

// api calls the JSON API. It resolves to {ok, status, data, error}, where
// error is the API's {code, message, service} on failure. A 401 shows the
// login view and throws LoggedOut (AUTH-8). An abort throws AbortError.
async function api(path, { method = "GET", body, signal } = {}) {
  const init = { method, signal, headers: {} };
  if (body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(path, init);
  } catch (e) {
    if (isAbort(e)) throw e;
    return { ok: false, status: 0, data: null, error: { code: "", message: NETWORK_MESSAGE } };
  }
  let data = null;
  try {
    if (res.status !== 204) data = await res.json();
  } catch (e) {
    if (isAbort(e)) throw e;
  }
  if (res.status === 401) {
    const msg = data && data.error && data.error.message;
    endSession(msg || null);
    throw new LoggedOut();
  }
  if (res.ok) return { ok: true, status: res.status, data, error: null };
  const error = data && data.error && typeof data.error.message === "string"
    ? data.error
    : { code: "", message: GENERIC_MESSAGE };
  return { ok: false, status: res.status, data, error };
}

// retryable: worth repeating as is (no answer, or a server/upstream error).
function retryable(res) {
  return res.status === 0 || res.status >= 500;
}

// ---- View switching ----

function show(view) {
  for (const v of VIEWS) $("view-" + v).hidden = v !== view;
  $("logout").hidden = view === "login";
  hideError();
  setLoading(null);
}

function setLoading(text) {
  $("loading").textContent = text || "";
  $("loading").hidden = !text;
}

function hideError() {
  $("error").hidden = true;
}

// showError shows the error panel. retry is {label, run} or null; nav is
// {label, run} or null (a way out, e.g. "New search"); link is {text, href}
// or null.
function showError(message, { retry = null, nav = null, link = null } = {}) {
  setLoading(null);
  $("error-message").textContent = message;
  $("error-link").hidden = !link;
  if (link) {
    $("error-link-a").textContent = link.text;
    $("error-link-a").href = link.href;
  }
  for (const [id, action] of [["error-retry", retry], ["error-nav", nav]]) {
    const b = $(id);
    b.hidden = !action;
    b.onclick = null;
    if (action) {
      b.textContent = action.label;
      b.onclick = () => {
        hideError();
        action.run();
      };
    }
  }
  $("error").hidden = false;
}

function setHash(hash, { replace = false } = {}) {
  if (location.hash === hash || (hash === "#/" && location.hash === "")) {
    route(); // no hashchange event for the same hash
  } else if (replace) {
    location.replace(hash);
  } else {
    location.hash = hash;
  }
}

function clearHash() {
  history.replaceState(null, "", location.pathname);
}

function el(tag, className, text) {
  const e = document.createElement(tag);
  if (className) e.className = className;
  if (text !== undefined && text !== null) e.textContent = text;
  return e;
}

// ---- Login and logout ----

function showLogin(message) {
  show("login");
  $("login-message").textContent = message || "";
  $("login-message").hidden = !message;
}

// endSession is for a 401: forget everything and show the login view.
function endSession(message) {
  loggedIn = false;
  generation++;
  if (controller) controller.abort();
  lastSearch = lastShows = preview = null;
  clearHash();
  showLogin(message);
}

async function logout() {
  hideError();
  $("logout").disabled = true;
  let res;
  try {
    res = await api("/auth/logout", { method: "POST" });
  } catch (e) {
    return; // LoggedOut: already on the login view
  } finally {
    $("logout").disabled = false;
  }
  if (!res.ok) {
    showError(res.error.message, { retry: { label: "Retry", run: logout } });
    return;
  }
  endSession(null);
}

// ---- Routing ----

function route() {
  if (!loggedIn) {
    clearHash();
    showLogin(null);
    return;
  }
  const parts = location.hash.replace(/^#\/?/, "").split("/");
  try {
    if (parts[0] === "search" && parts.length >= 2) {
      return runSearch(decodeURIComponent(parts.slice(1).join("/")));
    }
    if (parts[0] === "artist" && parts[1]) {
      const artistId = decodeURIComponent(parts[1]);
      if (parts[2] === "show" && parts[3]) return loadPreview(artistId, decodeURIComponent(parts[3]));
      return loadShows(artistId);
    }
  } catch (e) {
    // A malformed hash (bad %-escape): fall through to the search view.
  }
  showSearch("");
}

// ---- Search and picker (SRCH-1, SRCH-3 to SRCH-5) ----

function showSearch(text) {
  newRequest();
  show("search");
  $("search-input").value = text;
  updateSearchButton();
  $("search-message").hidden = true;
  $("picker").hidden = true;
  $("picker").replaceChildren();
  if (text === "") $("search-input").focus();
}

function updateSearchButton() {
  $("search-button").disabled = $("search-input").value.trim() === "";
}

function submitSearch(event) {
  event.preventDefault();
  const q = $("search-input").value.trim();
  if (q === "") return; // SRCH-1
  setHash("#/search/" + encodeURIComponent(q));
}

async function runSearch(q) {
  showSearch(q);
  if (q.trim() === "") return;
  if (lastSearch && lastSearch.q === q) return showSearchResult(q, lastSearch.data);
  const req = newRequest();
  setLoading("Searching…");
  let res;
  try {
    res = await api("/api/artists?q=" + encodeURIComponent(q), { signal: req.signal });
  } catch (e) {
    return;
  }
  if (!req.current()) return;
  setLoading(null);
  if (!res.ok) {
    showError(res.error.message, { retry: retryable(res) ? { label: "Retry", run: () => runSearch(q) } : null });
    return;
  }
  lastSearch = { q, data: res.data };
  showSearchResult(q, res.data);
}

function showSearchResult(q, data) {
  if (data.match === "clear" && data.artists.length === 1) {
    // SRCH-3: straight to the shows. Replace, so Back doesn't bounce here.
    setHash("#/artist/" + encodeURIComponent(data.artists[0].id), { replace: true });
    return;
  }
  if (data.artists.length === 0) {
    // SRCH-5; the box keeps the text.
    $("search-message").textContent = "No artists found for '" + q + "' on setlist.fm.";
    $("search-message").hidden = false;
    return;
  }
  // SRCH-4: up to 10, in setlist.fm's order.
  const list = $("picker");
  list.replaceChildren();
  for (const a of data.artists.slice(0, 10)) {
    const b = el("button", "choice");
    b.type = "button";
    b.append(el("span", "name", a.name));
    if (a.disambiguation) b.append(el("span", "secondary", a.disambiguation));
    b.onclick = () => setHash("#/artist/" + encodeURIComponent(a.id));
    const li = el("li");
    li.append(b);
    list.append(li);
  }
  $("search-message").textContent = "Choose an artist:";
  $("search-message").hidden = false;
  list.hidden = false;
}

// ---- Show list (SHOW-1, SHOW-3, SHOW-5, SHOW-6) ----

async function loadShows(artistId) {
  const req = newRequest();
  show("shows");
  $("shows-title").textContent = "";
  $("shows-empty").hidden = true;
  $("shows-list").replaceChildren();
  if (lastShows && lastShows.artistId === artistId) return renderShows(artistId, lastShows.data);
  setLoading("Loading shows…");
  let res;
  try {
    res = await api("/api/artists/" + encodeURIComponent(artistId) + "/shows", { signal: req.signal });
  } catch (e) {
    return;
  }
  if (!req.current()) return;
  setLoading(null);
  if (!res.ok) {
    showError(res.error.message, {
      retry: retryable(res) ? { label: "Retry", run: () => loadShows(artistId) } : null,
    });
    return;
  }
  lastShows = { artistId, data: res.data };
  renderShows(artistId, res.data);
}

function renderShows(artistId, data) {
  $("shows-title").textContent = data.artist.name;
  if (data.shows.length === 0) {
    $("shows-empty").textContent = "No setlists found for " + data.artist.name + " on setlist.fm.";
    $("shows-empty").hidden = false;
    return;
  }
  const list = $("shows-list");
  for (const s of data.shows) {
    const b = el("button", "choice");
    b.type = "button";
    b.append(el("span", "name", s.date + " · " + s.city));
    b.append(el("span", "secondary", s.venue));
    if (s.tour) b.append(el("span", "secondary", s.tour));
    b.append(el("span", "secondary", s.songCount === 1 ? "1 song" : s.songCount + " songs"));
    b.onclick = () => setHash("#/artist/" + encodeURIComponent(artistId) + "/show/" + encodeURIComponent(s.id));
    const li = el("li");
    li.append(b);
    list.append(li);
  }
}

// ---- Preview (PREV-1 to PREV-7) ----

async function loadPreview(artistId, showId) {
  const req = newRequest();
  show("preview");
  preview = null;
  for (const id of ["preview-band", "preview-where", "preview-tour", "preview-summary"]) $(id).textContent = "";
  $("view-preview").hidden = true; // only the message while matching (PREV-1)
  $("preview-sets").replaceChildren();
  setLoading("Matching songs…");
  let res;
  try {
    res = await api("/api/shows/" + encodeURIComponent(showId) + "/preview", { signal: req.signal });
  } catch (e) {
    return;
  }
  if (!req.current()) return;
  setLoading(null);
  const backToShows = { label: "Back to shows", run: () => setHash("#/artist/" + encodeURIComponent(artistId)) };
  if (!res.ok) {
    showError(res.error.message, {
      retry: retryable(res) ? { label: "Retry", run: () => loadPreview(artistId, showId) } : null,
      nav: backToShows,
    });
    return;
  }
  preview = { artistId, data: res.data };
  renderPreview(res.data);
}

function renderPreview(data) {
  const s = data.show;
  $("view-preview").hidden = false;
  $("preview-band").textContent = s.band;
  $("preview-where").textContent = s.date + " · " + s.venue + ", " + s.city;
  $("preview-tour").textContent = s.tour || "";
  $("preview-tour").hidden = !s.tour;
  $("preview-setlist-link").href = s.setlistUrl;
  $("preview-summary").textContent = data.summary.found + " of " + data.summary.matchable + " songs found";
  $("preview-none").hidden = data.canCreate;
  $("create").disabled = !data.canCreate; // PREV-7
  $("create-status").hidden = true;

  const sets = $("preview-sets");
  for (const set of data.sets) {
    if (set.heading) sets.append(el("h3", null, set.heading));
    const ol = el("ol", "entries");
    for (const e of set.entries) ol.append(entryRow(e));
    sets.append(ol);
  }
}

function label(text) {
  return el("span", "label", text);
}

function entryRow(e) {
  const li = el("li");
  li.value = e.position;
  li.append(el("span", "name", e.name));
  if (e.note) li.append(el("span", "secondary", e.note));

  const labels = el("span", "labels");
  switch (e.status) {
    case "found":
      labels.append(label("Found"));
      break;
    case "live":
      labels.append(label("Live version"));
      break;
    case "cover_fallback":
      labels.append(label("Cover → " + e.originalArtist));
      if (e.live) labels.append(label("Live version"));
      break;
    case "not_found":
      labels.append(label("Not found"));
      break;
    case "not_found_medley":
      labels.append(label("Not found (medley)"));
      break;
  }
  if (e.tape) labels.append(label("Tape"));
  li.append(labels);

  if (e.track) {
    li.append(el("span", "track", e.track.title + " — " + e.track.artists.join(", ") + " · " + e.track.album));
  }
  return li;
}

// ---- Create and confirmation (PL-6 to PL-9, ERR-3, ERR-4) ----

async function createPlaylist() {
  if (!preview || $("create").disabled) return;
  const { artistId, data } = preview;
  const mine = generation;
  $("create").disabled = true; // PL-6
  hideError();
  $("create-status").hidden = false;
  let res;
  try {
    // Not aborted on navigation: once Spotify is called the server finishes anyway.
    res = await api("/api/playlists", { method: "POST", body: { showId: data.show.id, trackIds: data.trackIds } });
  } catch (e) {
    return;
  }
  if (mine !== generation) return; // the user went elsewhere meanwhile
  $("create-status").hidden = true;
  if (res.ok) {
    showDone(res.data.playlist);
    return;
  }
  $("create").disabled = false; // only after a failure (PL-6)
  const backToShows = { label: "Back to shows", run: () => setHash("#/artist/" + encodeURIComponent(artistId)) };
  if (res.error.code === "partial_create" && res.data.playlist) {
    // ERR-3: it exists; a retry makes a second one.
    showError(res.error.message, {
      link: { text: "Open the playlist in Spotify", href: res.data.playlist.url },
      retry: { label: "Try again (creates a new playlist)", run: createPlaylist },
      nav: backToShows,
    });
    return;
  }
  showError(res.error.message, {
    retry: retryable(res) ? { label: "Retry", run: createPlaylist } : null,
    nav: backToShows,
  });
}

function showDone(playlist) {
  newRequest();
  preview = null;
  show("done");
  const n = playlist.trackCount;
  $("done-message").textContent = "Playlist created: " + playlist.name + ", " + n + (n === 1 ? " track" : " tracks");
  $("done-link").href = playlist.url;
}

// ---- Start ----

async function start() {
  const login = new URLSearchParams(location.search).get("login");
  if (login !== null) history.replaceState(null, "", location.pathname + location.hash);
  const loginMessage = login === null ? null : LOGIN_MESSAGES[login] || LOGIN_MESSAGES.failed;
  await checkSession(loginMessage);
}

async function checkSession(loginMessage) {
  show(null);
  $("logout").hidden = true;
  setLoading("Loading…");
  let res;
  try {
    res = await api("/api/session");
  } catch (e) {
    return;
  }
  setLoading(null);
  if (!res.ok) {
    showError(res.error.message, { retry: { label: "Retry", run: () => checkSession(loginMessage) } });
    return;
  }
  loggedIn = res.data.loggedIn === true;
  if (!loggedIn) {
    clearHash();
    showLogin(loginMessage);
    return;
  }
  route(); // AUTH-3: no hash after a login, so the search view
}

$("search-form").addEventListener("submit", submitSearch);
$("search-input").addEventListener("input", updateSearchButton);
$("logout").addEventListener("click", logout);
$("shows-new-search").addEventListener("click", () => setHash("#/"));
$("back-to-shows").addEventListener("click", () => {
  if (preview) setHash("#/artist/" + encodeURIComponent(preview.artistId));
});
$("create").addEventListener("click", createPlaylist);
$("done-new-search").addEventListener("click", () => {
  lastSearch = null;
  setHash("#/");
});
window.addEventListener("hashchange", route);

start();
