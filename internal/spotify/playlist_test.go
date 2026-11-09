package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func trackIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%022d", i) // 22 base62 characters
	}
	return ids
}

func TestValidTrackID(t *testing.T) {
	for id, want := range map[string]bool{
		"4uLU6hMCjMI75M1A2tKUQC":    true,
		"0000000000000000000000":    true,
		"4uLU6hMCjMI75M1A2tKUQ":     false, // 21
		"4uLU6hMCjMI75M1A2tKUQCx":   false, // 23
		"4uLU6hMCjMI75M1A2tKUQ-":    false,
		"spotify:track:4uLU6hMCjMI": false,
		"":                          false,
		"4uLU6hMCjMI75M1A2tKUQé":    false,
	} {
		if got := ValidTrackID(id); got != want {
			t.Errorf("ValidTrackID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestCreatePlaylist_PL1_PL4_PrivateRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/me/playlists" {
			t.Errorf("%s %s, want POST /me/playlists", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		b, _ := io.ReadAll(r.Body)
		// Exactly these fields, public explicitly false.
		if want := `{"name":"Gojira – Leipzig, 14 Mar 2026","description":"D","public":false}`; string(b) != want {
			t.Errorf("body = %s, want %s", b, want)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":"pl1","name":"x","public":false,"external_urls":{"spotify":"https://open.spotify.com/playlist/pl1"}}`)
	}))
	defer srv.Close()
	p, err := testClient(srv, 0, nil).CreatePlaylist(t.Context(), testToken, "Gojira – Leipzig, 14 Mar 2026", "D")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Playlist{ID: "pl1", URL: "https://open.spotify.com/playlist/pl1"}); p != want {
		t.Errorf("got %+v, want %+v", p, want)
	}
}

func TestCreatePlaylistErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"401":         {401, ``, ErrUnauthorized},
		"403":         {403, `{"error":{"status":403,"message":"Forbidden"}}`, ErrUnavailable},
		"500":         {500, ``, ErrUnavailable},
		"no ID":       {201, `{"external_urls":{"spotify":"https://open.spotify.com/playlist/x"}}`, ErrUnavailable},
		"no link":     {201, `{"id":"x"}`, ErrUnavailable},
		"bad JSON":    {201, `{`, ErrUnavailable},
		"200 is fine": {200, `{"id":"x","external_urls":{"spotify":"u"}}`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()
			_, err := testClient(srv, -1, nil).CreatePlaylist(t.Context(), testToken, "n", "d")
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCreatePlaylistRetriesRateLimit(t *testing.T) {
	// A 429 means Spotify didn't create anything, so the POST is retried.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"name":"n"`) {
			t.Errorf("retried body = %s", b)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":"x","external_urls":{"spotify":"u"}}`)
	}))
	defer srv.Close()
	var waits []time.Duration
	if _, err := testClient(srv, 0, &waits).CreatePlaylist(t.Context(), testToken, "n", "d"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || !reflect.DeepEqual(waits, []time.Duration{2 * time.Second}) {
		t.Errorf("calls = %d, waits = %v", calls.Load(), waits)
	}
}

func TestCreatePlaylistReasonOmitsNames(t *testing.T) {
	// Spotify's message is logged only if it doesn't echo the name or
	// description sent.
	for msg, kept := range map[string]bool{
		"Insufficient client scope":      true,
		"Bad name: Gojira – Mea Culpa":   false,
		"Description too long: Haus Aue": false,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":{"status":400,"message":%q}}`, msg)
		}))
		_, err := testClient(srv, -1, nil).CreatePlaylist(t.Context(), testToken, "Gojira – Mea Culpa", "Haus Aue")
		srv.Close()
		if got := strings.Contains(err.Error(), msg); got != kept {
			t.Errorf("message %q kept = %v, want %v (err %v)", msg, got, kept, err)
		}
	}
}

// fakeItems records each add-items request's URIs and fails batch failAt
// (1-based) with failStatus.
type fakeItems struct {
	batches    [][]string
	failAt     int
	failStatus int
}

func (f *fakeItems) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/playlists/pl1/items" {
			t.Errorf("%s %s, want POST /playlists/pl1/items", r.Method, r.URL.Path)
		}
		var body map[string][]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 {
			t.Errorf("body %v: %v, want only uris", body, err)
		}
		f.batches = append(f.batches, body["uris"])
		if len(f.batches) == f.failAt {
			w.WriteHeader(f.failStatus)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"snapshot_id":"s"}`)
	}
}

func TestAddTracks_PL5_BatchesInOrderWithDuplicates(t *testing.T) {
	ids := trackIDs(250)
	ids[120] = ids[3] // a duplicate across batches
	f := &fakeItems{}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	added, err := testClient(srv, 0, nil).AddTracks(t.Context(), testToken, "pl1", ids)
	if err != nil || added != 250 {
		t.Fatalf("added %d, err %v", added, err)
	}
	if len(f.batches) != 3 || len(f.batches[0]) != 100 || len(f.batches[1]) != 100 || len(f.batches[2]) != 50 {
		t.Fatalf("batch sizes wrong: %d batches", len(f.batches))
	}
	var sent []string
	for _, b := range f.batches {
		sent = append(sent, b...)
	}
	want := make([]string, len(ids))
	for i, id := range ids {
		want[i] = "spotify:track:" + id
	}
	if !reflect.DeepEqual(sent, want) {
		t.Error("URIs not sent in order, duplicates kept")
	}
}

func TestAddTracksExactlyOneBatch(t *testing.T) {
	f := &fakeItems{}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	if _, err := testClient(srv, 0, nil).AddTracks(t.Context(), testToken, "pl1", trackIDs(MaxItemsPerRequest)); err != nil {
		t.Fatal(err)
	}
	if len(f.batches) != 1 {
		t.Errorf("%d batches for %d tracks, want 1", len(f.batches), MaxItemsPerRequest)
	}
}

func TestAddTracksStopsAtFailedBatch(t *testing.T) {
	for status, want := range map[int]error{500: ErrUnavailable, 401: ErrUnauthorized} {
		f := &fakeItems{failAt: 2, failStatus: status}
		srv := httptest.NewServer(f.handler(t))
		added, err := testClient(srv, -1, nil).AddTracks(t.Context(), testToken, "pl1", trackIDs(250))
		srv.Close()
		if !errors.Is(err, want) || added != 100 {
			t.Errorf("status %d: added %d, err %v; want 100, %v", status, added, err, want)
		}
		if len(f.batches) != 2 {
			t.Errorf("status %d: %d batches sent, want to stop after the failed second", status, len(f.batches))
		}
	}
}
