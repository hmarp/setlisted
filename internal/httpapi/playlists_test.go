package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hmarp/setlisted/internal/auth"
	"github.com/hmarp/setlisted/internal/setlist"
	"github.com/hmarp/setlisted/internal/spotify"
)

const (
	testPlaylistID  = "37i9dQZF1DXcBWIGoYBM5M"
	testPlaylistURL = "https://open.spotify.com/playlist/" + testPlaylistID
)

// noTourSetlistJSON is previewSetlistJSON's show without a tour.
var noTourSetlistJSON = strings.Replace(previewSetlistJSON, `"tour": {"name": "Mea Culpa Tour"},`, "", 1)

// testTrackIDs returns n valid track IDs, "0000000000000000000000",
// "0000000000000000000001", …
func testTrackIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%022d", i)
	}
	return ids
}

// fakePlaylistSpotify is a fake Spotify for create: POST /me/playlists and
// POST /playlists/{id}/items. It records every request.
type fakePlaylistSpotify struct {
	srv *httptest.Server

	mu       sync.Mutex
	creates  []map[string]any // decoded create bodies
	batches  [][]string       // each add-items request's URIs
	badAuth  bool
	unknown  []string // requests to any other path
	createFn http.HandlerFunc
	addFn    func(batch int, w http.ResponseWriter, r *http.Request) bool // true: handled (a failure)
}

func newFakePlaylistSpotify(t *testing.T) *fakePlaylistSpotify {
	f := &fakePlaylistSpotify{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		if r.Header.Get("Authorization") != "Bearer token" {
			f.badAuth = true
		}
		f.mu.Unlock()
		switch {
		case r.Method == "POST" && r.URL.Path == "/me/playlists":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.creates = append(f.creates, body)
			fn := f.createFn
			f.mu.Unlock()
			if fn != nil {
				fn(w, r)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"id":%q,"name":"x","public":false,"external_urls":{"spotify":%q}}`, testPlaylistID, testPlaylistURL)
		case r.Method == "POST" && r.URL.Path == "/playlists/"+testPlaylistID+"/items":
			var body struct {
				URIs []string `json:"uris"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.batches = append(f.batches, body.URIs)
			n, fn := len(f.batches), f.addFn
			f.mu.Unlock()
			if fn != nil && fn(n, w, r) {
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"snapshot_id":"snap"}`)
		default:
			f.mu.Lock()
			f.unknown = append(f.unknown, r.Method+" "+r.URL.Path)
			f.mu.Unlock()
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePlaylistSpotify) client() *spotify.Client {
	return spotify.New(spotify.Config{BaseURL: f.srv.URL, Retries: -1})
}

// sentURIs is every URI added, in request order.
func (f *fakePlaylistSpotify) sentURIs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []string
	for _, b := range f.batches {
		all = append(all, b...)
	}
	return all
}

// postPlaylist calls the real router with a session and the app's Origin.
func postPlaylist(t *testing.T, sfm *fakeSetlistFM, sp *fakePlaylistSpotify, logger *slog.Logger, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	a := testAuth(t)
	h := NewHandler(Deps{Logger: logger, Static: fstest.MapFS{}, Auth: a, SetlistFM: sfm.client(), Spotify: sp.client()})
	req := httptest.NewRequest("POST", "/api/playlists", strings.NewReader(body))
	req.Header.Set("Origin", testBaseURL)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(loggedIn(t, a))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return rec, got
}

func createBody(showID string, ids []string) string {
	b, _ := json.Marshal(createPlaylistRequestJSON{ShowID: showID, TrackIDs: ids})
	return string(b)
}

func TestCreatePlaylist(t *testing.T) {
	sfm := newFakeSetlistFM(t, serveSetlist(previewSetlistJSON))
	sp := newFakePlaylistSpotify(t)
	ids := testTrackIDs(18)
	ids[10] = ids[2] // a duplicate is sent as it is (PL-5)
	rec, _ := postPlaylist(t, sfm, sp, discardLogger(), createBody(testSetlistID, ids))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := `{"playlist":{"id":"` + testPlaylistID + `","name":"Gojira – Mea Culpa Tour (Setlist)","trackCount":18,"url":"` + testPlaylistURL + `"}}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body =\n%s\nwant\n%s", rec.Body, want)
	}
	if sp.badAuth {
		t.Error("a Spotify call didn't use the session's token")
	}
	if sfm.calls.Load() != 1 {
		t.Errorf("setlist.fm calls = %d, want 1 (the show is fetched again)", sfm.calls.Load())
	}
	// PL-1, PL-4: one new private playlist, with PL-2's name and PL-3's
	// description, and no other fields.
	wantCreate := []map[string]any{{
		"name":        "Gojira – Mea Culpa Tour (Setlist)",
		"description": "Haus Auensee, Leipzig, 14 Mar 2026. Setlist: https://www.setlist.fm/setlist/gojira/2026/x-63de4613.html. Created by Setlisted.",
		"public":      false,
	}}
	if !reflect.DeepEqual(sp.creates, wantCreate) {
		t.Errorf("create requests = %v, want %v", sp.creates, wantCreate)
	}
	// PL-5: every track, in order, duplicates kept.
	var wantURIs []string
	for _, id := range ids {
		wantURIs = append(wantURIs, "spotify:track:"+id)
	}
	if got := sp.sentURIs(); !reflect.DeepEqual(got, wantURIs) {
		t.Errorf("URIs = %q, want %q", got, wantURIs)
	}
	if len(sp.batches) != 1 || len(sp.unknown) != 0 {
		t.Errorf("batches = %d, other calls = %v", len(sp.batches), sp.unknown)
	}
}

func TestCreatePlaylist_PL2_NameWithoutTour(t *testing.T) {
	sp := newFakePlaylistSpotify(t)
	rec, body := postPlaylist(t, newFakeSetlistFM(t, serveSetlist(noTourSetlistJSON)), sp, discardLogger(), createBody(testSetlistID, testTrackIDs(1)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	const want = "Gojira – Leipzig, 14 Mar 2026"
	if got := body["playlist"].(map[string]any)["name"]; got != want {
		t.Errorf("name = %v, want %q", got, want)
	}
	if got := sp.creates[0]["name"]; got != want {
		t.Errorf("name sent = %v, want %q", got, want)
	}
}

func TestPlaylistNameAndDescription(t *testing.T) {
	base := setlist.Header{Band: "Gojira", City: "Leipzig", Date: "14 Mar 2026", Venue: "Haus Auensee", URL: "https://www.setlist.fm/s.html"}
	withTour := base
	withTour.Tour = "Mea Culpa Tour"
	blankTour := base
	blankTour.Tour = "  "
	tests := []struct {
		name string
		h    setlist.Header
		want string
	}{
		{"PL2 with tour", withTour, "Gojira – Mea Culpa Tour (Setlist)"},
		{"PL2 without tour", base, "Gojira – Leipzig, 14 Mar 2026"},
		{"PL2 blank tour counts as none", blankTour, "Gojira – Leipzig, 14 Mar 2026"},
	}
	for _, tt := range tests {
		if got := playlistName(tt.h); got != tt.want {
			t.Errorf("%s: name = %q, want %q", tt.name, got, tt.want)
		}
	}
	// PL-3, exactly.
	if got, want := playlistDescription(base), "Haus Auensee, Leipzig, 14 Mar 2026. Setlist: https://www.setlist.fm/s.html. Created by Setlisted."; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
	// The dash is an en dash, U+2013.
	if !strings.Contains(playlistName(base), "–") {
		t.Error("name has no en dash")
	}
}

func TestPlaylistTextLimits(t *testing.T) {
	long := setlist.Header{
		Band: strings.Repeat("Ö", 80), Tour: strings.Repeat("ü", 80),
		Venue: strings.Repeat("é", 400), City: "Leipzig", Date: "14 Mar 2026", URL: "https://www.setlist.fm/s.html",
	}
	name, desc := playlistName(long), playlistDescription(long)
	if n := len([]rune(name)); n != maxNameLength || !strings.HasSuffix(name, "…") {
		t.Errorf("name: %d characters %q, want %d ending in …", n, name, maxNameLength)
	}
	// The venue part is cut, so the setlist.fm link stays whole.
	tail := "…. Setlist: https://www.setlist.fm/s.html. Created by Setlisted."
	if n := len([]rune(desc)); n != maxDescriptionLength || !strings.HasSuffix(desc, tail) {
		t.Errorf("description: %d characters %q, want %d ending in %q", n, desc, maxDescriptionLength, tail)
	}
	// A short description is untouched, link and all.
	short := long
	short.Venue = "Haus Auensee"
	if got, want := playlistDescription(short), "Haus Auensee, Leipzig, 14 Mar 2026. Setlist: https://www.setlist.fm/s.html. Created by Setlisted."; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
	if !strings.HasPrefix(name, long.Band+" – ") {
		t.Error("name lost its start")
	}
	// Exactly at the limit: untouched.
	if got := truncate(strings.Repeat("a", 100), 100); got != strings.Repeat("a", 100) {
		t.Errorf("truncate at the limit changed the text: %q", got)
	}
}

func TestCreatePlaylistBatches(t *testing.T) {
	// 150 tracks → two add requests, 100 then 50, in order.
	sp := newFakePlaylistSpotify(t)
	ids := testTrackIDs(150)
	ids[120] = ids[5]
	rec, body := postPlaylist(t, newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)), sp, discardLogger(), createBody(testSetlistID, ids))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(sp.batches) != 2 || len(sp.batches[0]) != 100 || len(sp.batches[1]) != 50 {
		t.Fatalf("batches: %d", len(sp.batches))
	}
	var want []string
	for _, id := range ids {
		want = append(want, "spotify:track:"+id)
	}
	if !reflect.DeepEqual(sp.sentURIs(), want) {
		t.Error("tracks not added in order with duplicates")
	}
	if got := body["playlist"].(map[string]any)["trackCount"]; got != 150.0 {
		t.Errorf("trackCount = %v, want 150", got)
	}
}

func TestCreatePlaylistValidation(t *testing.T) {
	valid := testTrackIDs(3)
	tests := map[string]string{
		"missing showId":         `{"trackIds":["0000000000000000000000"]}`,
		"empty showId":           createBody("", valid),
		"malformed showId":       createBody("63de-4613", valid),
		"PREV7 empty trackIds":   createBody(testSetlistID, []string{}),
		"missing trackIds":       `{"showId":"63de4613"}`,
		"malformed track ID":     createBody(testSetlistID, []string{valid[0], "not-a-spotify-id"}),
		"URI instead of ID":      createBody(testSetlistID, []string{"spotify:track:" + valid[0]}),
		"too many tracks":        createBody(testSetlistID, testTrackIDs(maxTracks+1)),
		"not JSON":               `showId=63de4613`,
		"wrong types":            `{"showId":63,"trackIds":"x"}`,
		"two JSON values":        createBody(testSetlistID, valid) + createBody(testSetlistID, valid),
		"body over the size cap": `{"showId":"63de4613","trackIds":["` + strings.Repeat("a", maxCreateBody) + `"]}`,
		"empty body":             ``,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			sfm := newFakeSetlistFM(t, serveSetlist(previewSetlistJSON))
			sp := newFakePlaylistSpotify(t)
			rec, got := postPlaylist(t, sfm, sp, discardLogger(), body)
			if rec.Code != http.StatusBadRequest || got["error"].(map[string]any)["code"] != "invalid_request" {
				t.Errorf("got %d %s, want 400 invalid_request", rec.Code, rec.Body)
			}
			if sfm.calls.Load() != 0 || len(sp.creates) != 0 {
				t.Error("an upstream was called for an invalid request")
			}
		})
	}
	// At the limit is fine.
	sp := newFakePlaylistSpotify(t)
	if rec, _ := postPlaylist(t, newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)), sp, discardLogger(), createBody(testSetlistID, testTrackIDs(maxTracks))); rec.Code != http.StatusCreated {
		t.Errorf("%d tracks: status %d", maxTracks, rec.Code)
	}
	if len(sp.batches) != 2 {
		t.Errorf("%d tracks: %d batches, want 2", maxTracks, len(sp.batches))
	}
}

func TestCreatePlaylistOriginAndSession(t *testing.T) {
	body := createBody(testSetlistID, testTrackIDs(1))
	for name, tt := range map[string]struct {
		origin     string
		session    bool
		wantStatus int
		wantCode   string
	}{
		"no Origin":      {"", true, 403, "forbidden_origin"},
		"foreign Origin": {"https://evil.example", true, 403, "forbidden_origin"},
		"no session":     {testBaseURL, false, 401, "not_logged_in"},
	} {
		t.Run(name, func(t *testing.T) {
			sfm := newFakeSetlistFM(t, serveSetlist(previewSetlistJSON))
			sp := newFakePlaylistSpotify(t)
			a := testAuth(t)
			h := NewHandler(Deps{Logger: discardLogger(), Static: fstest.MapFS{}, Auth: a, SetlistFM: sfm.client(), Spotify: sp.client()})
			req := httptest.NewRequest("POST", "/api/playlists", strings.NewReader(body))
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.session {
				req.AddCookie(loggedIn(t, a))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), `"`+tt.wantCode+`"`) {
				t.Errorf("got %d %s, want %d %s", rec.Code, rec.Body, tt.wantStatus, tt.wantCode)
			}
			if sfm.calls.Load() != 0 || len(sp.creates) != 0 {
				t.Error("an upstream was called")
			}
		})
	}
	// GET on the path isn't a route.
	a := testAuth(t)
	h := NewHandler(testDeps(discardLogger(), fstest.MapFS{}, a, noSetlistFM()))
	req := httptest.NewRequest("GET", "/api/playlists", nil)
	req.AddCookie(loggedIn(t, a))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/playlists = %d, want 404", rec.Code)
	}
}

func TestCreatePlaylistErrors(t *testing.T) {
	ok := serveSetlist(previewSetlistJSON)
	fail := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }
	}
	tests := []struct {
		name        string
		showID      string
		setlistFM   http.HandlerFunc
		create      http.HandlerFunc
		wantStatus  int
		wantError   string
		wantCleared bool
	}{
		{name: "unknown show", showID: "0000abcd", setlistFM: ok,
			wantStatus: 404, wantError: `{"code":"not_found","message":"Not found."}`},
		{name: "setlist.fm unavailable", showID: testSetlistID, setlistFM: fail(http.StatusBadGateway),
			wantStatus: 503, wantError: `{"code":"upstream_unavailable","message":"Couldn't reach setlist.fm. Try again.","service":"setlist.fm"}`},
		{name: "ERR4 create fails", showID: testSetlistID, setlistFM: ok, create: fail(http.StatusInternalServerError),
			wantStatus: 503, wantError: `{"code":"upstream_unavailable","message":"Couldn't reach Spotify. Try again.","service":"Spotify"}`},
		{name: "ERR4 create forbidden", showID: testSetlistID, setlistFM: ok, create: fail(http.StatusForbidden),
			wantStatus: 503, wantError: `{"code":"upstream_unavailable","message":"Couldn't reach Spotify. Try again.","service":"Spotify"}`},
		{name: "AUTH8 create 401", showID: testSetlistID, setlistFM: ok, create: fail(http.StatusUnauthorized),
			wantStatus: 401, wantError: `{"code":"not_logged_in","message":"You're not logged in. Log in with Spotify."}`, wantCleared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := newFakePlaylistSpotify(t)
			sp.createFn = tt.create
			rec, body := postPlaylist(t, newFakeSetlistFM(t, tt.setlistFM), sp, discardLogger(), createBody(tt.showID, testTrackIDs(3)))
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			gotErr, _ := json.Marshal(body["error"])
			if string(gotErr) != tt.wantError {
				t.Errorf("error = %s, want %s", gotErr, tt.wantError)
			}
			if _, has := body["playlist"]; has {
				t.Error("a playlist in the response, but none was created")
			}
			cleared := strings.Contains(rec.Header().Get("Set-Cookie"), auth.SessionCookie+"=;")
			if cleared != tt.wantCleared {
				t.Errorf("session cleared = %v, want %v", cleared, tt.wantCleared)
			}
			if len(sp.batches) != 0 {
				t.Errorf("%d add requests, want none", len(sp.batches))
			}
			if wantCreates := map[bool]int{true: 1, false: 0}[tt.create != nil]; len(sp.creates) != wantCreates {
				t.Errorf("%d create requests, want %d", len(sp.creates), wantCreates)
			}
		})
	}
}

func TestCreatePlaylist_ERR3_PartialCreate(t *testing.T) {
	for name, tt := range map[string]struct {
		status      int
		wantCleared bool
	}{
		"second batch fails":  {http.StatusInternalServerError, false},
		"second batch is 401": {http.StatusUnauthorized, true},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			sp := newFakePlaylistSpotify(t)
			sp.addFn = func(batch int, w http.ResponseWriter, r *http.Request) bool {
				if batch == 2 {
					w.WriteHeader(tt.status)
					return true
				}
				return false
			}
			rec, _ := postPlaylist(t, newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)), sp,
				slog.New(slog.NewJSONHandler(&logs, nil)), createBody(testSetlistID, testTrackIDs(maxTracks)))
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502", rec.Code)
			}
			want := `{"error":{"code":"partial_create","message":"Creating the playlist failed part-way: it was created on Spotify, but some songs may be missing.","service":"Spotify"},` +
				`"playlist":{"id":"` + testPlaylistID + `","name":"Gojira – Mea Culpa Tour (Setlist)","url":"` + testPlaylistURL + `"}}` + "\n"
			if rec.Body.String() != want {
				t.Errorf("body =\n%s\nwant\n%s", rec.Body, want)
			}
			// No rollback: one create, two batches, no other calls (no
			// delete or unfollow).
			if len(sp.creates) != 1 || len(sp.batches) != 2 || len(sp.unknown) != 0 {
				t.Errorf("creates %d, batches %d, other calls %v", len(sp.creates), len(sp.batches), sp.unknown)
			}
			cleared := strings.Contains(rec.Header().Get("Set-Cookie"), auth.SessionCookie+"=;")
			if cleared != tt.wantCleared {
				t.Errorf("session cleared = %v, want %v", cleared, tt.wantCleared)
			}
			if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), `"added":100`) {
				t.Errorf("partial create not logged: %s", logs.String())
			}
		})
	}
}

// createDirect calls the handler itself, with a session in the context,
// so the budget and the request's context can be controlled.
func createDirect(ctx context.Context, h *playlistHandler, body string) *httptest.ResponseRecorder {
	ctx = auth.ContextWithSession(ctx, auth.Session{AccessToken: "token", Expiry: time.Now().Add(time.Hour), UserID: "user"})
	req := httptest.NewRequestWithContext(ctx, "POST", "/api/playlists", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.create(rec, req)
	return rec
}

func TestCreatePlaylistBudget(t *testing.T) {
	// Spotify is slower than the budget while creating: ERR-4's 503,
	// logged as a budget overrun.
	var logs bytes.Buffer
	sp := newFakePlaylistSpotify(t)
	sp.createFn = slowSpotify
	h := &playlistHandler{
		logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		setlistFM: newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)).client(),
		spotify:   sp.client(),
		budget:    100 * time.Millisecond,
	}
	start := time.Now()
	rec := createDirect(context.Background(), h, createBody(testSetlistID, testTrackIDs(3)))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"service":"Spotify"`) {
		t.Errorf("got %d %s, want 503 Spotify", rec.Code, rec.Body)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v, want the budget to end it", d)
	}
	if !strings.Contains(logs.String(), "budget") {
		t.Errorf("budget overrun not logged as one: %s", logs.String())
	}
}

func TestCreatePlaylistClientGoneBeforeCreate(t *testing.T) {
	// The browser goes away while setlist.fm is slow: nothing is created,
	// 499 is logged, no outage warning.
	var logs bytes.Buffer
	sp := newFakePlaylistSpotify(t)
	h := &playlistHandler{
		logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		setlistFM: newFakeSetlistFM(t, slowSpotify).client(),
		spotify:   sp.client(),
		budget:    time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	rec := createDirect(ctx, h, createBody(testSetlistID, testTrackIDs(3)))
	if rec.Code != statusClientClosed {
		t.Errorf("status = %d, want %d", rec.Code, statusClientClosed)
	}
	if strings.Contains(logs.String(), `"level":"WARN"`) || len(sp.creates) != 0 {
		t.Errorf("creates %d, logs %s", len(sp.creates), logs.String())
	}
}

func TestCreatePlaylistClientGoneAfterSetlistFM(t *testing.T) {
	// The browser goes away as setlist.fm answers: no playlist is created.
	sp := newFakePlaylistSpotify(t)
	ctx, cancel := context.WithCancel(context.Background())
	sfm := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		serveSetlist(previewSetlistJSON)(w, r)
		cancel()
	})
	h := &playlistHandler{logger: discardLogger(), setlistFM: sfm.client(), spotify: sp.client(), budget: time.Minute}
	rec := createDirect(ctx, h, createBody(testSetlistID, testTrackIDs(3)))
	if rec.Code != statusClientClosed || len(sp.creates) != 0 {
		t.Errorf("status = %d, creates = %d; want %d and none", rec.Code, len(sp.creates), statusClientClosed)
	}
}

func TestCreatePlaylistClientGoneDuringSpotify(t *testing.T) {
	// The browser goes away while Spotify creates the playlist: it's still
	// created and all the tracks added, so it isn't left empty or
	// half-filled.
	sp := newFakePlaylistSpotify(t)
	ctx, cancel := context.WithCancel(context.Background())
	sp.createFn = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"id":%q,"external_urls":{"spotify":%q}}`, testPlaylistID, testPlaylistURL)
		cancel() // the tab closes as Spotify answers
	}
	h := &playlistHandler{
		logger:    discardLogger(),
		setlistFM: newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)).client(),
		spotify:   sp.client(),
		budget:    time.Minute,
	}
	rec := createDirect(ctx, h, createBody(testSetlistID, testTrackIDs(150)))
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := len(sp.sentURIs()); got != 150 {
		t.Errorf("%d tracks added, want all 150", got)
	}
}

func TestCreatePlaylistLogsNoUserData(t *testing.T) {
	// GEN-5: no names, IDs of tracks, the token or the playlist link in
	// the app's logs, on success or failure.
	for name, setup := range map[string]func(*fakePlaylistSpotify){
		"success": func(*fakePlaylistSpotify) {},
		"create fails": func(f *fakePlaylistSpotify) {
			f.createFn = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }
		},
		"create 401": func(f *fakePlaylistSpotify) {
			f.createFn = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }
		},
		"partial create": func(f *fakePlaylistSpotify) { f.addFn = failAll },
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			sp := newFakePlaylistSpotify(t)
			setup(sp)
			postPlaylist(t, newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)), sp, slog.New(slog.NewJSONHandler(&logs, nil)),
				createBody(testSetlistID, testTrackIDs(3)))
			for _, secret := range []string{"Gojira", "Mea Culpa", "Leipzig", "Haus Auensee", "0000000000000000000001", "token", testPlaylistURL} {
				if strings.Contains(logs.String(), secret) {
					t.Errorf("logs contain %q: %s", secret, logs.String())
				}
			}
		})
	}
}

func TestCreatePlaylistReasonOmitsShowDetails(t *testing.T) {
	// Spotify's reason is logged only if it quotes no part the name or
	// description was built from, nor any ID sent.
	for name, tt := range map[string]struct {
		createMsg, addMsg string
	}{
		"create quotes the band":     {createMsg: "Invalid name near Gojira"},
		"create quotes the venue":    {createMsg: "Bad description: Haus Auensee"},
		"add quotes a track URI":     {addMsg: "Invalid track uri: spotify:track:0000000000000000000001"},
		"add quotes the playlist ID": {addMsg: "Playlist " + testPlaylistID + " is not modifiable"},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			sp := newFakePlaylistSpotify(t)
			reply := func(msg string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					io.Copy(io.Discard, r.Body)
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, `{"error":{"status":400,"message":%q}}`, msg)
				}
			}
			msg := tt.createMsg
			if msg != "" {
				sp.createFn = reply(msg)
			} else {
				msg = tt.addMsg
				sp.addFn = func(_ int, w http.ResponseWriter, r *http.Request) bool { reply(msg)(w, r); return true }
			}
			postPlaylist(t, newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)), sp, slog.New(slog.NewJSONHandler(&logs, nil)),
				createBody(testSetlistID, testTrackIDs(3)))
			if !strings.Contains(logs.String(), `"level":"WARN"`) {
				t.Fatalf("no WARN: %s", logs.String())
			}
			if strings.Contains(logs.String(), msg) {
				t.Errorf("logs contain Spotify's message %q: %s", msg, logs.String())
			}
		})
	}
}

func failAll(_ int, w http.ResponseWriter, r *http.Request) bool {
	io.Copy(io.Discard, r.Body)
	w.WriteHeader(http.StatusInternalServerError)
	return true
}
