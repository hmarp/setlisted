package setlistfm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "test-api-key"

// testClient returns a client for srv with no rate-limit spacing to speak of
// and a tiny backoff, so tests run fast.
func testClient(srv *httptest.Server, retries int) *Client {
	return New(Config{
		APIKey:   testKey,
		BaseURL:  srv.URL,
		Interval: time.Microsecond,
		Retries:  retries,
		Backoff:  time.Millisecond,
	})
}

func TestSearchArtistsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != testKey {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if r.URL.Path != "/search/artists" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("artistName") != "Blue Öyster Cult" || q.Get("sort") != "relevance" || q.Get("p") != "1" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		w.Write([]byte(`{"type":"artists","itemsPerPage":30,"page":1,"total":1,"artist":[
			{"mbid":"c3cceeed-3332-4cf0-8c4c-bbde425147b6","name":"Blue Öyster Cult","sortName":"Blue Öyster Cult","disambiguation":"US rock band"}]}`))
	}))
	defer srv.Close()

	got, err := testClient(srv, 0).SearchArtists(context.Background(), "Blue Öyster Cult")
	if err != nil {
		t.Fatal(err)
	}
	want := Artist{MBID: "c3cceeed-3332-4cf0-8c4c-bbde425147b6", Name: "Blue Öyster Cult", Disambiguation: "US rock band"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want [%+v]", got, want)
	}
}

func TestSearchArtistsNoResults(t *testing.T) {
	// setlist.fm answers 404 when a search has no results.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":404,"status":"Not Found","message":"not found"}`))
	}))
	defer srv.Close()

	got, err := testClient(srv, 0).SearchArtists(context.Background(), "zzzz")
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("got %v, %v; want an empty slice and no error", got, err)
	}
}

func TestRetriesRateLimitThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"artist":[{"mbid":"m","name":"Gojira"}]}`))
	}))
	defer srv.Close()

	got, err := testClient(srv, 2).SearchArtists(context.Background(), "gojira")
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("calls = %d, want 3", n)
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := testClient(srv, 2).SearchArtists(context.Background(), "gojira")
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("calls = %d, want 3 (1 + 2 retries)", n)
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	start := time.Now()
	if _, err := testClient(srv, 1).Setlist(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < time.Second {
		t.Errorf("retried after %v, want Retry-After's 1s", d)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"server error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`secret upstream details`))
		}, ErrUnavailable},
		{"forbidden (bad key)", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}, ErrUnavailable},
		{"bad JSON", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<html>`))
		}, ErrUnavailable},
		{"not found", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			_, err := testClient(srv, 0).Setlist(context.Background(), "abc")
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), "secret upstream") {
				t.Errorf("error text contains the response body: %v", err)
			}
		})
	}
}

func TestUnreachableErrorHidesURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listening any more

	_, err := testClient(srv, 0).SearchArtists(context.Background(), "private search text")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "artistName") {
		t.Errorf("error text contains the request URL: %v", err)
	}
}

func TestArtistSetlistsNotFoundIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artist/mbid-1/setlists" || r.URL.Query().Get("p") != "4" {
			t.Errorf("url = %q", r.URL)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p, err := testClient(srv, 0).ArtistSetlists(context.Background(), "mbid-1", 4)
	if err != nil || len(p.Setlists) != 0 || p.HasMore() {
		t.Errorf("got %+v, %v; want an empty last page", p, err)
	}
}

func TestSetlistDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/setlist/63de4613" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"id":"63de4613","versionId":"v1","eventDate":"14-03-2026",
			"artist":{"mbid":"m","name":"Gojira"},
			"venue":{"id":"v","name":"Zénith","city":{"id":"c","name":"Paris","country":{"code":"FR","name":"France"}}},
			"tour":{"name":"World Tour"},
			"sets":{"set":[
				{"song":[{"name":"Intro","tape":true},{"name":"Born for One Thing"},{"name":""}]},
				{"name":"Acoustic","song":[{"name":"Hold On","info":"first time live"}]},
				{"encore":1,"song":[{"name":"Paranoid","cover":{"mbid":"x","name":"Black Sabbath"}}]}]},
			"url":"https://www.setlist.fm/setlist/gojira/2026/x.html"}`))
	}))
	defer srv.Close()

	s, err := testClient(srv, 0).Setlist(context.Background(), "63de4613")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "63de4613" || s.Artist.Name != "Gojira" || s.Venue.Name != "Zénith" || s.Venue.City.Name != "Paris" ||
		s.Tour == nil || s.Tour.Name != "World Tour" || s.URL == "" {
		t.Errorf("header fields: %+v", s)
	}
	if got := s.DisplayDate(); got != "14 Mar 2026" {
		t.Errorf("DisplayDate = %q", got)
	}
	if got := s.SongCount(); got != 5 {
		t.Errorf("SongCount = %d, want 5 (tapes and nameless entries count)", got)
	}
	sets := s.Sets.Set
	if len(sets) != 3 || !sets[0].Song[0].Tape || sets[1].Name != "Acoustic" || sets[1].Song[0].Info != "first time live" ||
		sets[2].Encore != 1 || sets[2].Song[0].Cover == nil || sets[2].Song[0].Cover.Name != "Black Sabbath" {
		t.Errorf("sets: %+v", sets)
	}
	if sets[0].Song[1].Cover != nil {
		t.Errorf("non-cover has a Cover")
	}
}

func TestDisplayDate(t *testing.T) {
	tests := []struct{ in, want string }{
		{"14-03-2026", "14 Mar 2026"},
		{"02-01-2025", "02 Jan 2025"},
		{"31-12-1999", "31 Dec 1999"},
		{"not a date", "not a date"},
	}
	for _, tt := range tests {
		if got := (Setlist{EventDate: tt.in}).DisplayDate(); got != tt.want {
			t.Errorf("DisplayDate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLimiterSpacesCalls(t *testing.T) {
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		times = append(times, time.Now())
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	const interval = 50 * time.Millisecond
	c := New(Config{APIKey: testKey, BaseURL: srv.URL, Interval: interval})
	for range 4 {
		if _, err := c.Setlist(context.Background(), "x"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(times); i++ {
		// Allow a little slack for timer and scheduling jitter.
		if gap := times[i].Sub(times[i-1]); gap < interval-5*time.Millisecond {
			t.Errorf("gap %d = %v, want at least %v", i, gap, interval)
		}
	}
}

func TestLimiterHonoursContext(t *testing.T) {
	l := newLimiter(time.Hour)
	if err := l.wait(context.Background()); err != nil { // first slot is free
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := l.wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}
