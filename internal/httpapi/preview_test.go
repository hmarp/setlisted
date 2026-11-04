package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hmarp/setlisted/internal/auth"
	"github.com/hmarp/setlisted/internal/pipeline"
	"github.com/hmarp/setlisted/internal/setlist"
	"github.com/hmarp/setlisted/internal/setlistfm"
	"github.com/hmarp/setlisted/internal/spotify"
)

const testSetlistID = "63de4613"

// previewSetlistJSON is a show with a main set, a set of only skipped
// entries and an encore. Expected outcomes, by position:
//
//	main set:  1 Intro (tape, found), – Drum Solo (skipped), 2 Stranded
//	           (note, found), 3 Silvera (live only), 4 a medley, 5 Unknown
//	           Song (not found), 6 Stranded again (found)
//	set 2:     only "Guitar Solo" (skipped): dropped
//	Encore 2:  7 Another Brick (tape, cover fallback to Pink Floyd),
//	           8 Hellion (cover fallback to Judas Priest, live),
//	           9 Territory (cover of Sepultura found under the band),
//	           10 Outro (tape, not found)
const previewSetlistJSON = `{
  "id": "63de4613", "eventDate": "14-03-2026", "url": "https://www.setlist.fm/setlist/gojira/2026/x-63de4613.html",
  "artist": {"mbid": "5a85c140-dcf8-4a4f-b8d6-1e5a2a4e0f4b", "name": "Gojira"},
  "venue": {"name": "Haus Auensee", "city": {"name": "Leipzig"}},
  "tour": {"name": "Mea Culpa Tour"},
  "sets": {"set": [
    {"song": [
      {"name": "Intro", "tape": true},
      {"name": "Drum Solo"},
      {"name": "Stranded", "info": "extended outro"},
      {"name": "Silvera"},
      {"name": "Medley: Backbone / Remembrance"},
      {"name": "Unknown Song"},
      {"name": "Stranded"}
    ]},
    {"encore": 1, "song": [{"name": "Guitar Solo"}]},
    {"encore": 2, "song": [
      {"name": "Another Brick", "tape": true, "cover": {"name": "Pink Floyd"}},
      {"name": "Hellion", "cover": {"name": "Judas Priest"}},
      {"name": "Territory", "cover": {"name": "Sepultura"}},
      {"name": "Outro", "tape": true}
    ]}
  ]}
}`

func spotifyTrack(id, title, artist, album, albumType string) string {
	return fmt.Sprintf(`{"id":%q,"name":%q,"artists":[{"name":%q}],"album":{"name":%q,"album_type":%q,"release_date":"2016-06-17","release_date_precision":"day"}}`,
		id, title, artist, album, albumType)
}

// previewSearches are the fake Spotify's results, keyed by the q parameter.
// Any other query has no results.
var previewSearches = map[string][]string{
	`track:"Intro" artist:"Gojira"`:    {spotifyTrack("t-intro", "Intro", "Gojira", "Magma", "album")},
	`track:"Stranded" artist:"Gojira"`: {spotifyTrack("t-stranded", "Stranded", "Gojira", "Magma", "album")},
	`track:"Silvera" artist:"Gojira"`:  {spotifyTrack("t-silvera-live", "Silvera - Live", "Gojira", "Live at Wacken", "album")},
	`track:"Another Brick" artist:"Pink Floyd"`: {
		spotifyTrack("t-brick", "Another Brick", "Pink Floyd", "The Wall", "album")},
	`track:"Hellion" artist:"Judas Priest"`: {
		spotifyTrack("t-hellion-live", "Hellion - Live", "Judas Priest", "Unleashed in the East (Live)", "album")},
	`track:"Territory" artist:"Gojira"`: {spotifyTrack("t-territory", "Territory", "Gojira", "Covers", "single")},
}

// fakeSpotify serves /search from previewSearches, or with handler if set.
type fakeSpotify struct {
	srv     *httptest.Server
	calls   atomic.Int32
	badAuth atomic.Bool // a search without "Bearer token" or market=from_token
}

func newFakeSpotify(t *testing.T, handler http.HandlerFunc) *fakeSpotify {
	f := &fakeSpotify{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer token" || r.URL.Query().Get("market") != spotify.MarketFromToken {
			f.badAuth.Store(true)
		}
		if handler != nil {
			handler(w, r)
			return
		}
		items := previewSearches[r.URL.Query().Get("q")]
		fmt.Fprintf(w, `{"tracks":{"items":[%s]}}`, strings.Join(items, ","))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSpotify) client() *spotify.Client {
	return spotify.New(spotify.Config{BaseURL: f.srv.URL, Retries: -1})
}

func serveSetlist(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/setlist/"+testSetlistID {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}
}

// getPreview calls the real router with a session. It returns the
// response and the decoded body.
func getPreview(t *testing.T, sfm *fakeSetlistFM, sp *fakeSpotify, logger *slog.Logger, id string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	a := testAuth(t)
	h := NewHandler(Deps{Logger: logger, Static: fstest.MapFS{}, Auth: a, SetlistFM: sfm.client(), Spotify: sp.client()})
	req := httptest.NewRequest("GET", "/api/shows/"+id+"/preview", nil)
	req.AddCookie(loggedIn(t, a))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return rec, body
}

func TestPreview(t *testing.T) {
	sfm := newFakeSetlistFM(t, serveSetlist(previewSetlistJSON))
	sp := newFakeSpotify(t, nil)
	rec, _ := getPreview(t, sfm, sp, discardLogger(), testSetlistID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if sp.badAuth.Load() {
		t.Error("a search didn't use the session's token and market=from_token")
	}
	var got previewJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	tour := "Mea Culpa Tour"
	wantShow := previewShowJSON{
		ID: testSetlistID, Band: "Gojira", Date: "14 Mar 2026", Venue: "Haus Auensee", City: "Leipzig",
		Tour: &tour, SetlistURL: "https://www.setlist.fm/setlist/gojira/2026/x-63de4613.html",
	}
	if !reflect.DeepEqual(got.Show, wantShow) {
		t.Errorf("show = %+v, want %+v", got.Show, wantShow)
	}

	// PREV-2, spec §1: found counts the three Found statuses; matchable is
	// every shown entry except the medley.
	if got.Summary != (previewSummary{Found: 7, Matchable: 9}) {
		t.Errorf("summary = %+v, want 7 of 9", got.Summary)
	}
	if !got.CanCreate {
		t.Error("canCreate = false, want true")
	}

	// In setlist order, duplicates kept, no not-found or medley entries.
	wantIDs := []string{"t-intro", "t-stranded", "t-silvera-live", "t-stranded", "t-brick", "t-hellion-live", "t-territory"}
	if !reflect.DeepEqual(got.TrackIDs, wantIDs) {
		t.Errorf("trackIds = %q, want %q", got.TrackIDs, wantIDs)
	}

	// PREV-3: the main set has no heading, the all-skipped set is dropped,
	// and positions run across sets with skipped entries absent.
	var headings []string
	var positions []int
	for _, s := range got.Sets {
		headings = append(headings, s.Heading)
		for _, e := range s.Entries {
			positions = append(positions, e.Position)
		}
	}
	if !reflect.DeepEqual(headings, []string{"", "Encore 2"}) {
		t.Errorf("headings = %q, want main set and Encore 2", headings)
	}
	if !reflect.DeepEqual(positions, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) {
		t.Errorf("positions = %v", positions)
	}
	for _, s := range got.Sets {
		for _, e := range s.Entries {
			if strings.Contains(e.Name, "Solo") {
				t.Errorf("skipped entry %q is shown", e.Name)
			}
		}
	}
}

func TestPreview_PREV4_Statuses(t *testing.T) {
	sfm := newFakeSetlistFM(t, serveSetlist(previewSetlistJSON))
	rec, _ := getPreview(t, sfm, newFakeSpotify(t, nil), discardLogger(), testSetlistID)
	var got struct {
		Sets []struct {
			Entries []json.RawMessage `json:"entries"`
		} `json:"sets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, s := range got.Sets {
		for _, e := range s.Entries {
			var c bytes.Buffer
			json.Compact(&c, e)
			rows = append(rows, c.String())
		}
	}
	track := func(id, title, artist, album string) string {
		return fmt.Sprintf(`{"id":%q,"title":%q,"artists":[%q],"album":%q}`, id, title, artist, album)
	}
	want := []string{
		// Found, with the Tape label.
		`{"position":1,"name":"Intro","note":null,"tape":true,"status":"found","live":false,"originalArtist":null,"track":` +
			track("t-intro", "Intro", "Gojira", "Magma") + `}`,
		// Found, with a note.
		`{"position":2,"name":"Stranded","note":"extended outro","tape":false,"status":"found","live":false,"originalArtist":null,"track":` +
			track("t-stranded", "Stranded", "Gojira", "Magma") + `}`,
		// Live version.
		`{"position":3,"name":"Silvera","note":null,"tape":false,"status":"live","live":true,"originalArtist":null,"track":` +
			track("t-silvera-live", "Silvera - Live", "Gojira", "Live at Wacken") + `}`,
		// Not found (medley), never searched.
		`{"position":4,"name":"Medley: Backbone / Remembrance","note":null,"tape":false,"status":"not_found_medley","live":false,"originalArtist":null,"track":null}`,
		// Not found.
		`{"position":5,"name":"Unknown Song","note":null,"tape":false,"status":"not_found","live":false,"originalArtist":null,"track":null}`,
		// A duplicate (ENT-7): its own row.
		`{"position":6,"name":"Stranded","note":null,"tape":false,"status":"found","live":false,"originalArtist":null,"track":` +
			track("t-stranded", "Stranded", "Gojira", "Magma") + `}`,
		// Cover fallback, with the Tape label.
		`{"position":7,"name":"Another Brick","note":null,"tape":true,"status":"cover_fallback","live":false,"originalArtist":"Pink Floyd","track":` +
			track("t-brick", "Another Brick", "Pink Floyd", "The Wall") + `}`,
		// Cover fallback that is live: both labels.
		`{"position":8,"name":"Hellion","note":null,"tape":false,"status":"cover_fallback","live":true,"originalArtist":"Judas Priest","track":` +
			track("t-hellion-live", "Hellion - Live", "Judas Priest", "Unleashed in the East (Live)") + `}`,
		// A cover found under the band: plain Found, originalArtist still set.
		`{"position":9,"name":"Territory","note":null,"tape":false,"status":"found","live":false,"originalArtist":"Sepultura","track":` +
			track("t-territory", "Territory", "Gojira", "Covers") + `}`,
		// Not found, with the Tape label.
		`{"position":10,"name":"Outro","note":null,"tape":true,"status":"not_found","live":false,"originalArtist":null,"track":null}`,
	}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d:\n%s", len(rows), len(want), strings.Join(rows, "\n"))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d:\n got %s\nwant %s", i+1, rows[i], want[i])
		}
	}
}

func TestPreview_PREV7_NothingFound(t *testing.T) {
	const body = `{"id":"63de4613","eventDate":"14-03-2026","artist":{"name":"Gojira"},"venue":{"name":"V","city":{"name":"C"}},
	  "sets":{"set":[{"song":[{"name":"Unknown Song"},{"name":"A / B"},{"name":"Bass Solo"}]}]}}`
	sfm := newFakeSetlistFM(t, serveSetlist(body))
	rec, body2 := getPreview(t, sfm, newFakeSpotify(t, nil), discardLogger(), testSetlistID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body2["canCreate"] != false {
		t.Errorf("canCreate = %v, want false", body2["canCreate"])
	}
	if s := body2["summary"].(map[string]any); s["found"] != 0.0 || s["matchable"] != 1.0 {
		t.Errorf("summary = %v, want 0 of 1", s)
	}
	if ids, ok := body2["trackIds"].([]any); !ok || len(ids) != 0 {
		t.Errorf("trackIds = %v, want []", body2["trackIds"])
	}
	if body2["show"].(map[string]any)["tour"] != nil {
		t.Errorf("tour = %v, want null", body2["show"].(map[string]any)["tour"])
	}
}

func TestPreview_NoSets(t *testing.T) {
	const body = `{"id":"63de4613","eventDate":"14-03-2026","artist":{"name":"Gojira"},"venue":{"name":"V","city":{"name":"C"}},"sets":{"set":[]}}`
	sp := newFakeSpotify(t, nil)
	rec, got := getPreview(t, newFakeSetlistFM(t, serveSetlist(body)), sp, discardLogger(), testSetlistID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if sets, ok := got["sets"].([]any); !ok || len(sets) != 0 {
		t.Errorf("sets = %v, want []", got["sets"])
	}
	if got["canCreate"] != false || sp.calls.Load() != 0 {
		t.Errorf("canCreate = %v, Spotify calls = %d", got["canCreate"], sp.calls.Load())
	}
}

func TestPreviewErrors(t *testing.T) {
	ok := serveSetlist(previewSetlistJSON)
	tests := []struct {
		name        string
		id          string
		setlistFM   http.HandlerFunc
		spotify     http.HandlerFunc
		wantStatus  int
		wantError   string // the whole error body
		wantCleared bool   // session cookie cleared
	}{
		{name: "unknown show", id: "0000abcd", setlistFM: ok,
			wantStatus: 404, wantError: `{"code":"not_found","message":"Not found."}`},
		{name: "malformed ID", id: "63de-4613", setlistFM: ok,
			wantStatus: 404, wantError: `{"code":"not_found","message":"Not found."}`},
		{name: "setlist.fm unavailable", id: testSetlistID,
			setlistFM:  func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) },
			wantStatus: 503, wantError: `{"code":"upstream_unavailable","message":"Couldn't reach setlist.fm. Try again.","service":"setlist.fm"}`},
		{name: "Spotify unavailable", id: testSetlistID, setlistFM: ok,
			spotify:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantStatus: 503, wantError: `{"code":"upstream_unavailable","message":"Couldn't reach Spotify. Try again.","service":"Spotify"}`},
		{name: "Spotify 401", id: testSetlistID, setlistFM: ok,
			spotify:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			wantStatus: 401, wantError: `{"code":"not_logged_in","message":"You're not logged in. Log in with Spotify."}`, wantCleared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sfm := newFakeSetlistFM(t, tt.setlistFM)
			sp := newFakeSpotify(t, tt.spotify)
			rec, body := getPreview(t, sfm, sp, discardLogger(), tt.id)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			gotErr, _ := json.Marshal(body["error"])
			if string(gotErr) != tt.wantError {
				t.Errorf("error = %s, want %s", gotErr, tt.wantError)
			}
			cleared := strings.Contains(rec.Header().Get("Set-Cookie"), auth.SessionCookie+"=;")
			if cleared != tt.wantCleared {
				t.Errorf("session cleared = %v, want %v (Set-Cookie %q)", cleared, tt.wantCleared, rec.Header().Get("Set-Cookie"))
			}
			if tt.wantStatus == 404 && sp.calls.Load() != 0 {
				t.Errorf("Spotify called %d times for an unknown show", sp.calls.Load())
			}
			if tt.name == "malformed ID" && sfm.calls.Load() != 0 {
				t.Error("setlist.fm called for a malformed ID")
			}
		})
	}
}

func TestPreviewNoSession(t *testing.T) {
	sfm := newFakeSetlistFM(t, serveSetlist(previewSetlistJSON))
	h := NewHandler(Deps{Logger: discardLogger(), Static: fstest.MapFS{}, Auth: testAuth(t), SetlistFM: sfm.client(), Spotify: noSpotify()})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/shows/"+testSetlistID+"/preview", nil))
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"not_logged_in"`) {
		t.Errorf("got %d %s, want 401 not_logged_in", rec.Code, rec.Body)
	}
	if sfm.calls.Load() != 0 {
		t.Error("setlist.fm called without a session")
	}
}

// previewDirect calls the handler itself, with a session in the context,
// so the budget and the request's context can be controlled.
func previewDirect(ctx context.Context, h *previewHandler) *httptest.ResponseRecorder {
	ctx = auth.ContextWithSession(ctx, auth.Session{AccessToken: "token", Expiry: time.Now().Add(time.Hour), UserID: "user"})
	req := httptest.NewRequestWithContext(ctx, "GET", "/api/shows/"+testSetlistID+"/preview", nil)
	req.SetPathValue("id", testSetlistID)
	rec := httptest.NewRecorder()
	h.serve(rec, req)
	return rec
}

func slowSpotify(w http.ResponseWriter, r *http.Request) {
	select {
	case <-time.After(5 * time.Second):
	case <-r.Context().Done():
	}
}

func TestPreviewBudget(t *testing.T) {
	// Spotify is slower than the preview's budget: the user gets ERR-1's
	// 503 rather than a dropped connection, and it's logged as a failure.
	var logs bytes.Buffer
	h := &previewHandler{
		logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		setlistFM: newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)).client(),
		spotify:   newFakeSpotify(t, slowSpotify).client(),
		budget:    100 * time.Millisecond,
	}
	start := time.Now()
	rec := previewDirect(context.Background(), h)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"service":"Spotify"`) {
		t.Errorf("got %d %s, want 503 Spotify", rec.Code, rec.Body)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v, want the budget to end it", d)
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), "budget") {
		t.Errorf("budget overrun not logged as one: %s", logs.String())
	}
}

func TestPreviewClientGone(t *testing.T) {
	// The browser goes away while the searches run: no outage warning.
	for name, fake := range map[string]struct{ setlistFM, spotify http.HandlerFunc }{
		"during setlist.fm": {setlistFM: slowSpotify},
		"during Spotify":    {setlistFM: serveSetlist(previewSetlistJSON), spotify: slowSpotify},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			h := &previewHandler{
				logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
				setlistFM: newFakeSetlistFM(t, fake.setlistFM).client(),
				spotify:   newFakeSpotify(t, fake.spotify).client(),
				budget:    time.Minute,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			rec := previewDirect(ctx, h)
			if rec.Code != statusClientClosed {
				t.Errorf("status = %d, want %d", rec.Code, statusClientClosed)
			}
			if strings.Contains(logs.String(), `"level":"WARN"`) {
				t.Errorf("logged as an outage: %s", logs.String())
			}
		})
	}
}

func TestPreviewLogsNoUserData(t *testing.T) {
	// GEN-5: no names, notes, track details or tokens in the app's logs,
	// on success or failure.
	for name, spotifyHandler := range map[string]http.HandlerFunc{
		"success":     nil,
		"unavailable": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"401":         func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			getPreview(t, newFakeSetlistFM(t, serveSetlist(previewSetlistJSON)), newFakeSpotify(t, spotifyHandler), logger, testSetlistID)
			for _, secret := range []string{"Gojira", "Stranded", "Pink Floyd", "extended outro", "Leipzig", "Magma", "t-intro", "token", url.QueryEscape("Gojira")} {
				if strings.Contains(logs.String(), secret) {
					t.Errorf("logs contain %q: %s", secret, logs.String())
				}
			}
		})
	}
}

func TestBuildPreviewMisalignedRows(t *testing.T) {
	// The pipeline returns one row per non-skipped entry. If that ever
	// breaks, the preview fails rather than misplacing rows.
	show := setlistfmShow(t, previewSetlistJSON)
	results := make([]pipeline.EntryResult, 9) // the show has 10 shown entries
	if _, err := buildPreview(show, results); err == nil {
		t.Error("too few rows: want an error")
	}
	if _, err := buildPreview(show, make([]pipeline.EntryResult, 11)); err == nil {
		t.Error("too many rows: want an error")
	}
	if _, err := buildPreview(show, make([]pipeline.EntryResult, 10)); err != nil {
		t.Errorf("matching rows: %v", err)
	}
}

func setlistfmShow(t *testing.T, body string) setlist.Show {
	t.Helper()
	var sl setlistfm.Setlist
	if err := json.Unmarshal([]byte(body), &sl); err != nil {
		t.Fatal(err)
	}
	return sl.Show()
}
