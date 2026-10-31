package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hmarp/setlisted/internal/setlistfm"
)

const testMBID = "5a85c140-dcf8-4a4f-b8d6-1e5a2a4e0f4b"

func TestClassifyArtists(t *testing.T) {
	artist := func(name string) setlistfm.Artist {
		return setlistfm.Artist{MBID: "id-" + name, Name: name}
	}
	many := make([]setlistfm.Artist, 15)
	for i := range many {
		many[i] = artist(fmt.Sprintf("Ghost Band %d", i))
	}
	tests := []struct {
		name      string
		query     string
		artists   []setlistfm.Artist
		wantMatch string
		wantNames []string
	}{
		{"exactly one exact match", "gojira",
			[]setlistfm.Artist{artist("Gojira Tribute"), artist("Gojira")}, "clear", []string{"Gojira"}},
		{"exact match ignoring accents and case", " motorhead ",
			[]setlistfm.Artist{artist("Motörhead"), artist("Motorheadache")}, "clear", []string{"Motörhead"}},
		{"two exact matches", "Ghost",
			[]setlistfm.Artist{artist("Ghost"), artist("Ghost B.C."), artist("GHOST")}, "picker", []string{"Ghost", "Ghost B.C.", "GHOST"}},
		{"no exact match", "sword",
			[]setlistfm.Artist{artist("The Sword"), artist("Swords")}, "picker", []string{"The Sword", "Swords"}},
		{"more than 10 results", "ghost band", many, "picker", namesOf(many[:10])},
		{"no results", "zzz", []setlistfm.Artist{}, "none", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyArtists(tt.query, tt.artists)
			if got.Match != tt.wantMatch {
				t.Errorf("match = %q, want %q", got.Match, tt.wantMatch)
			}
			var names []string
			for _, a := range got.Artists {
				names = append(names, a.Name)
			}
			if strings.Join(names, "|") != strings.Join(tt.wantNames, "|") || got.Artists == nil {
				t.Errorf("artists = %q, want %q", names, tt.wantNames)
			}
		})
	}
}

func namesOf(as []setlistfm.Artist) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.Name)
	}
	return out
}

// fakeSetlistFM is a fake setlist.fm API. Handlers are keyed by path.
type fakeSetlistFM struct {
	t     *testing.T
	srv   *httptest.Server
	calls atomic.Int32
}

func newFakeSetlistFM(t *testing.T, handler http.HandlerFunc) *fakeSetlistFM {
	f := &fakeSetlistFM{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSetlistFM) client() *setlistfm.Client {
	return setlistfm.New(setlistfm.Config{
		APIKey: "key", BaseURL: f.srv.URL, Interval: time.Microsecond, Retries: 1, Backoff: time.Millisecond,
	})
}

// setlistsJSON is a page of setlists in setlist.fm's format. songCounts
// gives each setlist's number of entries, newest first; ids are "s<page>-<i>".
func setlistsJSON(page, total int, songCounts []int) string {
	var sls []string
	for i, n := range songCounts {
		var songs []string
		for j := range n {
			songs = append(songs, fmt.Sprintf(`{"name":"Song %d","tape":%v}`, j, j == 0))
		}
		sets := `{"set":[]}`
		if n > 0 {
			sets = `{"set":[{"song":[` + strings.Join(songs, ",") + `]}]}`
		}
		sls = append(sls, fmt.Sprintf(`{"id":"s%d-%d","eventDate":"14-03-2026","artist":{"mbid":%q,"name":"Gojira"},
			"venue":{"name":"Zénith","city":{"name":"Paris"}},"tour":{"name":"Tour"},"sets":%s}`, page, i, testMBID, sets))
	}
	return fmt.Sprintf(`{"type":"setlists","itemsPerPage":20,"page":%d,"total":%d,"setlist":[%s]}`, page, total, strings.Join(sls, ","))
}

func repeat(n, count int) []int {
	out := make([]int, count)
	for i := range out {
		out[i] = n
	}
	return out
}

// get calls the real router with a session and decodes the JSON body.
func get(t *testing.T, sfm *setlistfm.Client, logger *slog.Logger, path string) (int, map[string]any) {
	t.Helper()
	a := testAuth(t)
	h := NewHandler(logger, fstest.MapFS{}, a, sfm)
	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(loggedIn(t, a))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return rec.Code, body
}

func TestSearchArtistsEndpoint(t *testing.T) {
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/artists" || r.URL.Query().Get("artistName") != "Gojira" {
			t.Errorf("url = %q", r.URL)
		}
		w.Write([]byte(`{"artist":[{"mbid":"` + testMBID + `","name":"Gojira","disambiguation":"French metal band"},{"mbid":"x","name":"Gojira Tribute"}]}`))
	})
	status, body := get(t, f.client(), discardLogger(), "/api/artists?q=+Gojira+")
	if status != 200 {
		t.Fatalf("status = %d, body %v", status, body)
	}
	want := `{"artists":[{"disambiguation":"French metal band","id":"` + testMBID + `","name":"Gojira"}],"match":"clear"}`
	if got, _ := json.Marshal(body); string(got) != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestSearchArtistsNoResultsEndpoint(t *testing.T) {
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	status, body := get(t, f.client(), discardLogger(), "/api/artists?q=zzzz")
	if got, _ := json.Marshal(body); status != 200 || string(got) != `{"artists":[],"match":"none"}` {
		t.Errorf("got %d %s", status, got)
	}
}

func TestSearchArtistsInvalid(t *testing.T) {
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {})
	for _, q := range []string{"", "q=", "q=%20%20%09", "q=" + strings.Repeat("a", maxQueryLength+1)} {
		status, body := get(t, f.client(), discardLogger(), "/api/artists?"+q)
		if status != 400 || body["error"].(map[string]any)["code"] != "invalid_request" {
			t.Errorf("%q: got %d %v, want 400 invalid_request", q, status, body)
		}
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("setlist.fm called %d times for invalid queries", n)
	}
}

func TestUpstreamUnavailable(t *testing.T) {
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests) // still rate-limited after the retry
	})
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	want := `{"error":{"code":"upstream_unavailable","message":"Couldn't reach setlist.fm. Try again.","service":"setlist.fm"}}`

	for _, path := range []string{"/api/artists?q=Secret+Band", "/api/artists/" + testMBID + "/shows"} {
		status, body := get(t, f.client(), logger, path)
		if got, _ := json.Marshal(body); status != 503 || string(got) != want {
			t.Errorf("%s: got %d %s", path, status, got)
		}
	}
	// The search text is never logged. (The request line logs the path,
	// which holds the artist's MBID: a public ID, not a name.)
	if s := logs.String(); strings.Contains(s, "Secret") || strings.Contains(s, "key") {
		t.Errorf("logs contain the search text or key: %s", s)
	}
	if !strings.Contains(logs.String(), "setlist.fm call failed") {
		t.Errorf("failure not logged: %s", logs.String())
	}
}

func TestArtistShows(t *testing.T) {
	tests := []struct {
		name      string
		pages     map[int]string // page → JSON; a missing page is a 404
		wantIDs   []string
		wantCalls int
	}{
		{
			name:      "zero-entry shows dropped",
			pages:     map[int]string{1: setlistsJSON(1, 4, []int{0, 3, 0, 2})},
			wantIDs:   []string{"s1-1", "s1-3"},
			wantCalls: 1,
		},
		{
			name: "stops at 20",
			pages: map[int]string{
				1: setlistsJSON(1, 100, repeat(5, 15)),
				2: setlistsJSON(2, 100, repeat(5, 20)),
			},
			wantIDs:   append(ids(1, 15), ids(2, 5)...),
			wantCalls: 2,
		},
		{
			name: "follows up to 3 pages and no more",
			pages: map[int]string{
				1: setlistsJSON(1, 100, repeat(0, 20)),
				2: setlistsJSON(2, 100, append(repeat(0, 19), 4)),
				3: setlistsJSON(3, 100, append(repeat(0, 18), 1, 2)),
				4: setlistsJSON(4, 100, repeat(5, 20)),
			},
			wantIDs:   []string{"s2-19", "s3-18", "s3-19"},
			wantCalls: 3,
		},
		{
			name:      "fewer than 20 available",
			pages:     map[int]string{1: setlistsJSON(1, 3, []int{1, 2, 3})},
			wantIDs:   []string{"s1-0", "s1-1", "s1-2"},
			wantCalls: 1,
		},
		{
			name: "last page answers 404",
			pages: map[int]string{
				1: setlistsJSON(1, 45, repeat(0, 20)), // total says more pages exist
			},
			wantIDs:   []string{},
			wantCalls: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/artist/"+testMBID+"/setlists" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				p, _ := strconv.Atoi(r.URL.Query().Get("p"))
				body, ok := tt.pages[p]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Write([]byte(body))
			})
			status, body := get(t, f.client(), discardLogger(), "/api/artists/"+testMBID+"/shows")
			if status != 200 {
				t.Fatalf("status = %d, body %v", status, body)
			}
			var got []string
			for _, s := range body["shows"].([]any) {
				got = append(got, s.(map[string]any)["id"].(string))
			}
			if strings.Join(got, ",") != strings.Join(tt.wantIDs, ",") {
				t.Errorf("shows = %v, want %v", got, tt.wantIDs)
			}
			if n := int(f.calls.Load()); n != tt.wantCalls {
				t.Errorf("setlist.fm calls = %d, want %d", n, tt.wantCalls)
			}
		})
	}
}

func ids(page, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("s%d-%d", page, i)
	}
	return out
}

func TestArtistShowsFields(t *testing.T) {
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"itemsPerPage":20,"page":1,"total":2,"setlist":[
			{"id":"a","eventDate":"02-01-2026","artist":{"name":"Gojira"},"venue":{"name":"O2 Academy","city":{"name":"Glasgow"}},
			 "tour":{"name":"Europe 2026"},
			 "sets":{"set":[{"song":[{"name":"Intro","tape":true},{"name":"Stranded"}]},{"encore":1,"song":[{"name":"Vacuity"}]}]}},
			{"id":"b","eventDate":"14-03-2025","artist":{"name":"Gojira"},"venue":{"name":"Zénith","city":{"name":"Paris"}},
			 "sets":{"set":[{"song":[{"name":""}]}]}}]}`))
	})
	status, body := get(t, f.client(), discardLogger(), "/api/artists/"+testMBID+"/shows")
	want := `{"artist":{"id":"` + testMBID + `","name":"Gojira"},"shows":[` +
		`{"city":"Glasgow","date":"02 Jan 2026","id":"a","songCount":3,"tour":"Europe 2026","venue":"O2 Academy"},` +
		`{"city":"Paris","date":"14 Mar 2025","id":"b","songCount":1,"tour":null,"venue":"Zénith"}]}`
	if got, _ := json.Marshal(body); status != 200 || string(got) != want {
		t.Errorf("got %d %s\nwant %s", status, got, want)
	}
}

func TestArtistShowsNoSetlists(t *testing.T) {
	// No setlists at all: the name comes from the artist lookup (SHOW-5).
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artist/" + testMBID:
			w.Write([]byte(`{"mbid":"` + testMBID + `","name":"Brand New Band"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	status, body := get(t, f.client(), discardLogger(), "/api/artists/"+testMBID+"/shows")
	want := `{"artist":{"id":"` + testMBID + `","name":"Brand New Band"},"shows":[]}`
	if got, _ := json.Marshal(body); status != 200 || string(got) != want {
		t.Errorf("got %d %s", status, got)
	}
}

func TestArtistShowsOnlyEmptyShows(t *testing.T) {
	// Setlists exist but none has entries: the name comes from them, and
	// there's no artist lookup.
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artist/"+testMBID+"/setlists" {
			t.Errorf("unexpected call %q", r.URL.Path)
		}
		w.Write([]byte(setlistsJSON(1, 2, []int{0, 0})))
	})
	status, body := get(t, f.client(), discardLogger(), "/api/artists/"+testMBID+"/shows")
	want := `{"artist":{"id":"` + testMBID + `","name":"Gojira"},"shows":[]}`
	if got, _ := json.Marshal(body); status != 200 || string(got) != want {
		t.Errorf("got %d %s", status, got)
	}
}

func TestArtistShowsUnknownArtist(t *testing.T) {
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	for _, id := range []string{testMBID, "not-an-mbid", "..%2F..%2Fsearch"} {
		status, body := get(t, f.client(), discardLogger(), "/api/artists/"+id+"/shows")
		if status != 404 || body["error"].(map[string]any)["code"] != "not_found" {
			t.Errorf("%s: got %d %v, want 404 not_found", id, status, body)
		}
	}
	if n := f.calls.Load(); n != 2 { // setlists + artist, for the valid ID only
		t.Errorf("setlist.fm calls = %d, want 2", n)
	}
}

func TestSetlistFMBudget(t *testing.T) {
	// setlist.fm is slower than the request's budget: the user gets ERR-1's
	// 503 rather than a dropped connection.
	f := newFakeSetlistFM(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})
	h := &setlistFMHandlers{logger: discardLogger(), client: f.client(), budget: 50 * time.Millisecond}
	for name, serve := range map[string]func(http.ResponseWriter, *http.Request){
		"search": h.searchArtists,
		"shows":  h.artistShows,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/artists/"+testMBID+"/shows?q=gojira", nil)
			req.SetPathValue("id", testMBID)
			rec := httptest.NewRecorder()
			start := time.Now()
			serve(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", rec.Code)
			}
			if d := time.Since(start); d > time.Second {
				t.Errorf("took %v, want the budget to end it", d)
			}
		})
	}
}
