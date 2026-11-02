package pipeline

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hmarp/setlisted/internal/matching"
	"github.com/hmarp/setlisted/internal/setlist"
)

const band = "Metallica"

// studio returns a studio album candidate for title by artist.
func studio(id, title, artist string) matching.Candidate {
	return matching.Candidate{ID: id, Title: title, Artists: []string{artist}, Album: "Album " + id,
		AlbumType: "album", ReleaseDate: "1986-03-03", ReleaseDatePrecision: "day"}
}

func live(id, title, artist string) matching.Candidate {
	c := studio(id, title, artist)
	c.Title += " - Live"
	return c
}

type query struct{ track, artist string }

// fake is a Searcher with canned answers per (track, artist). It records
// every call in order and the peak number of searches in flight. hook, if
// set, runs inside each search (after it's counted as in flight) and can
// block or fail it.
type fake struct {
	answers map[query][]matching.Candidate
	hook    func(ctx context.Context, q query) error

	mu       sync.Mutex
	calls    []query
	markets  []string
	inflight atomic.Int32
	peak     atomic.Int32
}

func (f *fake) SearchTracks(ctx context.Context, track, artist, market string) ([]matching.Candidate, error) {
	q := query{track, artist}
	f.mu.Lock()
	f.calls = append(f.calls, q)
	f.markets = append(f.markets, market)
	f.mu.Unlock()

	n := f.inflight.Add(1)
	defer f.inflight.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if f.hook != nil {
		if err := f.hook(ctx, q); err != nil {
			return nil, err
		}
	}
	return f.answers[q], nil
}

func (f *fake) count(q query) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == q {
			n++
		}
	}
	return n
}

func show(sets ...setlist.SourceSet) setlist.Show {
	return setlist.Build(setlist.Header{ID: "s1", Band: band}, sets)
}

func songs(names ...string) []setlist.SourceEntry {
	var out []setlist.SourceEntry
	for _, n := range names {
		out = append(out, setlist.SourceEntry{Name: n})
	}
	return out
}

func TestMatchShowStatusesAndShape(t *testing.T) {
	sh := show(
		setlist.SourceSet{Entries: []setlist.SourceEntry{
			{Name: "Ecstasy of Gold", Tape: true, CoverArtist: "Ennio Morricone"},
			{Name: "Battery", Note: "with intro"},
			{Name: "Drum Solo"},
			{Name: "One"},
			{Name: "Medley: A / B"},
			{Name: "Unknown Song"},
		}},
		setlist.SourceSet{Encore: 1, Entries: []setlist.SourceEntry{
			{Name: "Am I Evil?", CoverArtist: "Diamond Head"},
			{Name: "Helpless", CoverArtist: "Diamond Head"},
		}},
	)
	f := &fake{answers: map[query][]matching.Candidate{
		{"Ecstasy of Gold", band}:              nil,
		{"Ecstasy of Gold", "Ennio Morricone"}: {studio("eog", "The Ecstasy of Gold", "Ennio Morricone"), studio("eog2", "Ecstasy of Gold", "Ennio Morricone")},
		{"Battery", band}:                      {studio("bat", "Battery", band)},
		{"One", band}:                          {live("one-live", "One", band)},
		{"Am I Evil?", band}:                   {studio("aie", "Am I Evil?", band)},
		{"Helpless", "Diamond Head"}:           {live("help-live", "Helpless", "Diamond Head")},
	}}

	got, err := MatchShow(context.Background(), sh, f, "GB")
	if err != nil {
		t.Fatal(err)
	}
	want := []EntryResult{
		{Position: 1, Name: "Ecstasy of Gold", IsTape: true, OriginalArtist: "Ennio Morricone", Status: StatusCoverFallback,
			Track: &Track{ID: "eog2", Title: "Ecstasy of Gold", Artists: []string{"Ennio Morricone"}, Album: "Album eog2"}},
		{Position: 2, Name: "Battery", Note: "with intro", Status: StatusFound,
			Track: &Track{ID: "bat", Title: "Battery", Artists: []string{band}, Album: "Album bat"}},
		{Position: 3, Name: "One", Status: StatusLive, Live: true,
			Track: &Track{ID: "one-live", Title: "One - Live", Artists: []string{band}, Album: "Album one-live"}},
		{Position: 4, Name: "Medley: A / B", Status: StatusNotFoundMedley},
		{Position: 5, Name: "Unknown Song", Status: StatusNotFound},
		{Position: 6, SetHeading: "Encore 1", Name: "Am I Evil?", OriginalArtist: "Diamond Head", Status: StatusFound,
			Track: &Track{ID: "aie", Title: "Am I Evil?", Artists: []string{band}, Album: "Album aie"}},
		{Position: 7, SetHeading: "Encore 1", Name: "Helpless", OriginalArtist: "Diamond Head", Status: StatusCoverFallback, Live: true,
			Track: &Track{ID: "help-live", Title: "Helpless - Live", Artists: []string{"Diamond Head"}, Album: "Album help-live"}},
	}
	if !reflect.DeepEqual(got, want) {
		for i := range max(len(got), len(want)) {
			var g, w any
			if i < len(got) {
				g = got[i]
			}
			if i < len(want) {
				w = want[i]
			}
			if !reflect.DeepEqual(g, w) {
				t.Errorf("row %d:\n got  %+v\n want %+v", i, g, w)
			}
		}
	}
	for _, m := range f.markets {
		if m != "GB" {
			t.Errorf("market = %q, want GB", m)
		}
	}
	if !StatusFound.Found() || !StatusCoverFallback.Found() || !StatusLive.Found() ||
		StatusNotFound.Found() || StatusNotFoundMedley.Found() {
		t.Error("Status.Found is wrong")
	}
}

// ENT-1, ENT-3/ENT-4: skipped entries have no row and medleys aren't
// searched.
func TestMatchShow_ENT1_ENT4_SkippedAndMedleysNotSearched(t *testing.T) {
	sh := show(setlist.SourceSet{Entries: songs("Guitar Solo", "Jam", "Medley", "A / B", "Battery")})
	f := &fake{}
	got, err := MatchShow(context.Background(), sh, f, "GB")
	if err != nil {
		t.Fatal(err)
	}
	if want := []query{{"Battery", band}}; !reflect.DeepEqual(f.calls, want) {
		t.Errorf("searches = %v, want %v", f.calls, want)
	}
	if len(got) != 3 || got[0].Status != StatusNotFoundMedley || got[1].Status != StatusNotFoundMedley ||
		got[2].Status != StatusNotFound || got[2].Position != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestMatchShowNoSongs(t *testing.T) {
	got, err := MatchShow(context.Background(), show(setlist.SourceSet{Entries: songs("Drum Solo")}), &fake{}, "GB")
	if err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
	got, err = MatchShow(context.Background(), setlist.Show{}, &fake{}, "GB")
	if err != nil || len(got) != 0 {
		t.Errorf("empty show: got %+v, %v", got, err)
	}
}

// Results stay in setlist order however the searches complete: earlier
// entries are made to finish last.
func TestMatchShowSetlistOrder(t *testing.T) {
	var names []string
	answers := map[query][]matching.Candidate{}
	for i := range 12 {
		n := fmt.Sprintf("Song %02d", i+1)
		names = append(names, n)
		answers[query{n, band}] = []matching.Candidate{studio(n, n, band)}
	}
	f := &fake{answers: answers, hook: func(ctx context.Context, q query) error {
		var i int
		fmt.Sscanf(q.track, "Song %d", &i)
		time.Sleep(time.Duration(13-i) * 2 * time.Millisecond)
		return nil
	}}
	got, err := MatchShow(context.Background(), show(setlist.SourceSet{Entries: songs(names...)}), f, "GB")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(names) {
		t.Fatalf("got %d rows", len(got))
	}
	for i, r := range got {
		if r.Position != i+1 || r.Name != names[i] || r.Track == nil || r.Track.ID != names[i] {
			t.Errorf("row %d = %+v", i, r)
		}
	}
}

// ADR-005: never more than 4 searches at once, and 4 are used. The first
// searches are held until 4 are in flight; if the pipeline started a fifth
// before any finished, the peak would show it.
func TestMatchShowConcurrencyBound(t *testing.T) {
	// Each entry is a cover with nothing under the band, so each makes two
	// searches in a row.
	var names []string
	for i := range 20 {
		names = append(names, fmt.Sprintf("Song %d", i))
	}
	release := make(chan struct{})
	f := &fake{hook: func(ctx context.Context, q query) error {
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		return nil
	}}
	var entries []setlist.SourceEntry
	for _, n := range names {
		entries = append(entries, setlist.SourceEntry{Name: n, CoverArtist: "Other"})
	}
	done := make(chan error, 1)
	go func() {
		_, err := MatchShow(context.Background(), show(setlist.SourceSet{Entries: entries}), f, "GB")
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.inflight.Load() < DefaultConcurrency && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // give a fifth search the chance to start
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if p := f.peak.Load(); p != DefaultConcurrency {
		t.Errorf("peak searches in flight = %d, want %d", p, DefaultConcurrency)
	}
	if n := len(f.calls); n != 40 {
		t.Errorf("searches = %d, want 40 (two per cover)", n)
	}
}

func TestMatchShowLimitOne(t *testing.T) {
	f := &fake{hook: func(context.Context, query) error { time.Sleep(time.Millisecond); return nil }}
	_, err := MatchShowLimit(context.Background(), show(setlist.SourceSet{Entries: songs("A", "B", "C")}), f, "GB", 0)
	if err != nil || f.peak.Load() != 1 {
		t.Errorf("peak = %d, err = %v; want 1, nil", f.peak.Load(), err)
	}
}

// MATCH-2: a cover found under the band costs one search; one not found
// under the band is searched again under the original artist.
func TestMatchShow_MATCH2_CoverSearches(t *testing.T) {
	sh := show(setlist.SourceSet{Entries: []setlist.SourceEntry{
		{Name: "Am I Evil?", CoverArtist: "Diamond Head"},
		{Name: "Helpless", CoverArtist: "Diamond Head"},
	}})
	f := &fake{answers: map[query][]matching.Candidate{
		{"Am I Evil?", band}:           {studio("aie", "Am I Evil?", band)},
		{"Am I Evil?", "Diamond Head"}: {studio("aie-dh", "Am I Evil?", "Diamond Head")},
		{"Helpless", "Diamond Head"}:   {studio("help-dh", "Helpless", "Diamond Head")},
	}}
	got, err := MatchShow(context.Background(), sh, f, "GB")
	if err != nil {
		t.Fatal(err)
	}
	if f.count(query{"Am I Evil?", band}) != 1 || f.count(query{"Am I Evil?", "Diamond Head"}) != 0 {
		t.Errorf("cover found under the band: searches %v, want one", f.calls)
	}
	if f.count(query{"Helpless", band}) != 1 || f.count(query{"Helpless", "Diamond Head"}) != 1 {
		t.Errorf("cover not found under the band: searches %v, want band then original", f.calls)
	}
	if got[0].Status != StatusFound || got[0].Track.ID != "aie" {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[1].Status != StatusCoverFallback || got[1].Live || got[1].Track.ID != "help-dh" {
		t.Errorf("row 1 = %+v", got[1])
	}
}

// MATCH-9: no candidate under either artist → not found, no other search.
func TestMatchShow_MATCH9_NotFound(t *testing.T) {
	sh := show(setlist.SourceSet{Entries: []setlist.SourceEntry{{Name: "Rare", CoverArtist: "Someone"}}})
	f := &fake{answers: map[query][]matching.Candidate{
		{"Rare", band}: {studio("x", "Rare", "Other Band")},
	}}
	got, err := MatchShow(context.Background(), sh, f, "GB")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || got[0].Status != StatusNotFound || got[0].Track != nil || got[0].Live {
		t.Errorf("calls %v, got %+v", f.calls, got)
	}
}

// ENT-7: duplicates are each searched and kept in place.
func TestMatchShow_ENT7_Duplicates(t *testing.T) {
	sh := show(
		setlist.SourceSet{Entries: songs("Battery", "One", "Battery")},
		setlist.SourceSet{Encore: 1, Entries: songs("Battery")},
	)
	f := &fake{answers: map[query][]matching.Candidate{
		{"Battery", band}: {studio("bat", "Battery", band)},
		{"One", band}:     {studio("one", "One", band)},
	}}
	got, err := MatchShow(context.Background(), sh, f, "GB")
	if err != nil {
		t.Fatal(err)
	}
	if n := f.count(query{"Battery", band}); n != 3 {
		t.Errorf("Battery searched %d times, want 3", n)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.Track.ID)
	}
	if want := []string{"bat", "one", "bat", "bat"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if got[3].Position != 4 || got[3].SetHeading != "Encore 1" {
		t.Errorf("last row = %+v", got[3])
	}
}

// ERR-1: one failed search fails the whole match, cancels the searches in
// flight and starts no more.
func TestMatchShow_ERR1_SearchErrorCancels(t *testing.T) {
	boom := errors.New("spotify down")
	var names []string
	for i := range 20 {
		names = append(names, fmt.Sprintf("Song %d", i))
	}
	var cancelled atomic.Int32
	f := &fake{}
	f.hook = func(ctx context.Context, q query) error {
		if q.track == "Song 3" {
			// Fail once the other workers are blocked in their searches.
			deadline := time.Now().Add(time.Second)
			for f.inflight.Load() < DefaultConcurrency && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			return boom
		}
		select {
		case <-ctx.Done():
			cancelled.Add(1)
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("not cancelled")
		}
	}
	got, err := MatchShow(context.Background(), show(setlist.SourceSet{Entries: songs(names...)}), f, "GB")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the search error", err)
	}
	if got != nil {
		t.Errorf("got %d rows, want none", len(got))
	}
	if c := cancelled.Load(); c != DefaultConcurrency-1 {
		t.Errorf("cancelled searches = %d, want %d", c, DefaultConcurrency-1)
	}
	if n := len(f.calls); n != DefaultConcurrency {
		t.Errorf("searches started = %d, want %d (none after the failure)", n, DefaultConcurrency)
	}
}

func TestMatchShowParentCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fake{}
	_, err := MatchShow(ctx, show(setlist.SourceSet{Entries: songs("A", "B")}), f, "GB")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("searched %v after cancellation", f.calls)
	}
	_, err = MatchShow(ctx, show(setlist.SourceSet{Entries: songs("Medley")}), f, "GB")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("medley-only: err = %v, want context.Canceled", err)
	}
}

func TestSearcherFunc(t *testing.T) {
	var s Searcher = SearcherFunc(func(_ context.Context, track, artist, market string) ([]matching.Candidate, error) {
		return []matching.Candidate{studio("x", track, artist)}, nil
	})
	got, err := MatchShow(context.Background(), show(setlist.SourceSet{Entries: songs("Battery")}), s, "GB")
	if err != nil || got[0].Status != StatusFound {
		t.Errorf("got %+v, %v", got, err)
	}
}
