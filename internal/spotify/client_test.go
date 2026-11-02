package spotify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hmarp/setlisted/internal/matching"
)

const testToken = "test-access-token"

// testClient returns a client for srv whose waits are recorded in *waits
// instead of slept.
func testClient(srv *httptest.Server, retries int, waits *[]time.Duration) *Client {
	c := New(Config{BaseURL: srv.URL, Retries: retries})
	c.sleep = func(ctx context.Context, d time.Duration) error {
		if waits != nil {
			*waits = append(*waits, d)
		}
		return ctx.Err()
	}
	return c
}

const twoTracks = `{"tracks":{"href":"…","limit":10,"total":2,"items":[
  {"id":"t1","name":"Battery","popularity":70,
   "artists":[{"id":"a1","name":"Metallica"},{"id":"a2","name":"Guest"}],
   "album":{"id":"al1","name":"Master of Puppets","album_type":"album",
            "release_date":"1986-03-03","release_date_precision":"day","images":[]}},
  null,
  {"id":"t2","name":"Battery - Live","artists":[{"name":"Metallica"}],
   "album":{"name":"Live Shit","album_type":"compilation","release_date":"1993","release_date_precision":"year"}}
]}}`

func TestSearchTracksRequestAndMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("Authorization = %q", got)
		}
		if r.Method != http.MethodGet || r.URL.Path != "/search" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		want := map[string]string{
			"q":      `track:"Battery" artist:"Metallica"`,
			"type":   "track",
			"limit":  "10",
			"market": "from_token",
		}
		for k, v := range want {
			if q.Get(k) != v {
				t.Errorf("%s = %q, want %q", k, q.Get(k), v)
			}
		}
		if len(q) != len(want) {
			t.Errorf("query has extra parameters: %q", r.URL.RawQuery)
		}
		w.Write([]byte(twoTracks))
	}))
	defer srv.Close()

	got, err := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, "Battery", "Metallica", MarketFromToken)
	if err != nil {
		t.Fatal(err)
	}
	want := []matching.Candidate{
		{ID: "t1", Title: "Battery", Artists: []string{"Metallica", "Guest"}, Album: "Master of Puppets",
			AlbumType: "album", ReleaseDate: "1986-03-03", ReleaseDatePrecision: "day"},
		{ID: "t2", Title: "Battery - Live", Artists: []string{"Metallica"}, Album: "Live Shit",
			AlbumType: "compilation", ReleaseDate: "1993", ReleaseDatePrecision: "year"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestSearchTracksExplicitMarket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m := r.URL.Query().Get("market"); m != "GB" {
			t.Errorf("market = %q", m)
		}
		w.Write([]byte(`{"tracks":{"items":[]}}`))
	}))
	defer srv.Close()

	got, err := testClient(srv, 0, nil).WithToken("app-token").SearchTracks(context.Background(), "One", "Metallica", "GB")
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil for no results", got, err)
	}
}

func TestSearchTracksMarketRequired(t *testing.T) {
	c := New(Config{BaseURL: "http://127.0.0.1:1"})
	if _, err := c.SearchTracks(context.Background(), testToken, "One", "Metallica", ""); err == nil {
		t.Error("want an error for an empty market")
	}
}

func TestQueryQuotes(t *testing.T) {
	tests := []struct {
		track, artist, want string
		ok                  bool
	}{
		{"Battery", "Metallica", `track:"Battery" artist:"Metallica"`, true},
		{`The "Hellion"`, "Judas Priest", `track:"The Hellion" artist:"Judas Priest"`, true},
		{`"Heroes"`, "David Bowie", `track:"Heroes" artist:"David Bowie"`, true},
		{`A"B`, `Band "X"`, `track:"A B" artist:"Band X"`, true},
		{"Don't Tread on Me", "Metallica", `track:"Don't Tread on Me" artist:"Metallica"`, true},
		{"“Curly”", "Band", `track:"“Curly”" artist:"Band"`, true},
		{"  Spaced   Out ", "Band", `track:"Spaced Out" artist:"Band"`, true},
		{`""`, "Band", "", false},
		{"Song", "", "", false},
	}
	for _, tt := range tests {
		got, ok := Query(tt.track, tt.artist)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Query(%q, %q) = %q, %v; want %q, %v", tt.track, tt.artist, got, ok, tt.want, tt.ok)
		}
	}
}

func TestSearchTracksQuotesInRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query().Get("q"); q != `track:"The Hellion" artist:"Judas Priest"` {
			t.Errorf("q = %q", q)
		}
		w.Write([]byte(`{"tracks":{"items":[]}}`))
	}))
	defer srv.Close()
	if _, err := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, `The "Hellion"`, "Judas Priest", "GB"); err != nil {
		t.Fatal(err)
	}
}

func TestSearchTracksEmptyNameNotSearched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("searched for an empty name")
	}))
	defer srv.Close()
	got, err := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, `"`, "Band", "GB")
	if got != nil || err != nil {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestSearchTracksRetryAfterThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusTooManyRequests) // no Retry-After: backoff
		default:
			w.Write([]byte(twoTracks))
		}
	}))
	defer srv.Close()

	var waits []time.Duration
	got, err := testClient(srv, 0, &waits).SearchTracks(context.Background(), testToken, "Battery", "Metallica", "GB")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %d candidates, %v", len(got), err)
	}
	want := []time.Duration{3 * time.Second, 2 * DefaultBackoff}
	if !reflect.DeepEqual(waits, want) {
		t.Errorf("waits = %v, want %v", waits, want)
	}
}

func TestSearchTracksRateLimitedAfterBound(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, "Battery", "Metallica", "GB")
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if n := calls.Load(); n != DefaultRetries+1 {
		t.Errorf("calls = %d, want %d", n, DefaultRetries+1)
	}
}

func TestSearchTracksRetryAfterTooLong(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	var waits []time.Duration
	_, err := testClient(srv, 0, &waits).SearchTracks(context.Background(), testToken, "Battery", "Metallica", "GB")
	if !errors.Is(err, ErrUnavailable) || calls.Load() != 1 || len(waits) != 0 {
		t.Errorf("err = %v, calls = %d, waits = %v; want ErrUnavailable at once", err, calls.Load(), waits)
	}
}

func TestSearchTracks_AUTH8_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"status":401,"message":"The access token expired"}}`))
	}))
	defer srv.Close()

	_, err := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, "Battery", "Metallica", "GB")
	if !errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnauthorized only", err)
	}
}

func TestSearchTracks_ERR1_Unavailable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"500", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}},
		{"403", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}},
		{"redirect not followed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com/", http.StatusFound)
		}},
		{"bad JSON", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"tracks":`))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			_, err := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, "Battery", "Metallica", "GB")
			if !errors.Is(err, ErrUnavailable) {
				t.Errorf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}

// ERR-5: error text carries no token, URL, query or response body.
func TestSearchTracks_ERR5_NoDetailsInErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("upstream body secret"))
	}))
	srvURL := srv.URL
	_, err1 := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, "Battery", "Metallica", "GB")
	srv.Close()
	_, err2 := testClient(srv, 0, nil).SearchTracks(context.Background(), testToken, "Battery", "Metallica", "GB")
	for _, err := range []error{err1, err2} {
		if err == nil {
			t.Fatal("want an error")
		}
		for _, s := range []string{testToken, srvURL, "Battery", "upstream body"} {
			if strings.Contains(err.Error(), s) {
				t.Errorf("error %q contains %q", err, s)
			}
		}
	}
}

func TestSearchTracksContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := testClient(srv, 0, nil).SearchTracks(ctx, testToken, "Battery", "Metallica", "GB")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
