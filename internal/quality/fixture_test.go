package quality

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hmarp/setlisted/internal/pipeline"
	"github.com/hmarp/setlisted/internal/setlistfm"
)

func sampleFixture() Fixture {
	sl := setlistfm.Setlist{
		ID: "abc123", EventDate: "01-06-2026", Artist: setlistfm.Artist{Name: "Metallica"},
		Sets: setlistfm.Sets{Set: []setlistfm.Set{{Song: []setlistfm.Song{
			{Name: "Intro", Tape: true, Cover: &setlistfm.Artist{Name: "Ennio Morricone"}},
			{Name: "Battery"},
			{Name: "Drum Solo"},
			{Name: "Helpless", Cover: &setlistfm.Artist{Name: "Diamond Head"}},
			{Name: "Medley: A / B"},
			{Name: "Nothing"},
		}}}},
	}
	m, d := "Metallica", "Diamond Head"
	tr := func(id, title, artist, album string) Track {
		return Track{ID: id, Title: title, Artists: []string{artist}, Album: album, AlbumType: "album", ReleaseDate: "1986", ReleaseDatePrecision: "year"}
	}
	return Fixture{
		Setlist: sl,
		Searches: Searches{Market: "GB", Searches: []Search{
			{Track: "Intro", Artist: m},
			{Track: "Intro", Artist: "Ennio Morricone", Results: []Track{tr("e1", "Intro", "Ennio Morricone", "Film")}},
			{Track: "Battery", Artist: m, Results: []Track{tr("b1", "Battery", m, "Master of Puppets")}},
			{Track: "Helpless", Artist: m},
			{Track: "Helpless", Artist: d, Results: []Track{tr("h1", "Helpless", d, "Lightning")}},
			{Track: "Nothing", Artist: m},
		}},
		Expected: &Expected{SetlistID: "abc123", Entries: []Entry{
			{Position: 1, Name: "Intro", Status: pipeline.StatusCoverFallback, Tape: true, TrackID: "e1"},
			{Position: 2, Name: "Battery", Status: pipeline.StatusFound, TrackID: "b1"},
			{Position: 3, Name: "Helpless", Status: pipeline.StatusCoverFallback, TrackID: "h1"},
			{Position: 4, Name: "Medley: A / B", Status: pipeline.StatusNotFoundMedley},
			{Position: 5, Name: "Nothing", Status: pipeline.StatusNotFound},
		}},
	}
}

func TestEvaluate_AllAsExpected(t *testing.T) {
	r, err := Evaluate(context.Background(), sampleFixture())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Mismatches) > 0 || len(r.WrongArtist) > 0 {
		t.Fatalf("mismatches %v, wrong artist %v", r.Mismatches, r.WrongArtist)
	}
	if r.Found != 3 || r.Matchable != 4 {
		t.Errorf("found/matchable = %d/%d, want 3/4", r.Found, r.Matchable)
	}
}

func TestEvaluate_Mismatches(t *testing.T) {
	cases := map[string]struct {
		edit func(*Fixture)
		want string
	}{
		"status": {func(f *Fixture) {
			f.Expected.Entries[4].Status = pipeline.StatusLive
			f.Expected.Entries[4].Live = true
			f.Expected.Entries[4].TrackID = "x"
		}, "status live, got not_found"},
		"track":         {func(f *Fixture) { f.Expected.Entries[1].TrackID = "other" }, `track "other", got b1`},
		"tape label":    {func(f *Fixture) { f.Expected.Entries[0].Tape = false }, "tape false, got true"},
		"live label":    {func(f *Fixture) { f.Expected.Entries[2].Live = true }, "live true, got false"},
		"count":         {func(f *Fixture) { f.Expected.Entries = f.Expected.Entries[:4] }, "has 4 entries, the pipeline gave 5"},
		"name":          {func(f *Fixture) { f.Expected.Entries[1].Name = "Batery" }, `expected #2 "Batery"`},
		"id":            {func(f *Fixture) { f.Expected.SetlistID = "zzz" }, `setlistId "zzz"`},
		"track missing": {func(f *Fixture) { f.Expected.Entries[1].TrackID = "" }, "trackId is needed"},
		"no expected":   {func(f *Fixture) { f.Expected = nil }, "no expected.json"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := sampleFixture()
			tc.edit(&f)
			r, err := Evaluate(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(r.Mismatches, "\n"), tc.want) {
				t.Errorf("mismatches %q, want one containing %q", r.Mismatches, tc.want)
			}
		})
	}
}

// The wrong-artist check doesn't trust the matcher: a found track credited to
// someone else is reported even if the pipeline chose it.
func TestCredits_IndependentCheck(t *testing.T) {
	if !credits([]string{"Someone", "Motörhead"}, "  motorhead ") {
		t.Error("Motörhead should credit motorhead")
	}
	if credits([]string{"Ghost B.C."}, "Ghost") || credits([]string{"X"}, "") || credits(nil, "X") {
		t.Error("credited a wrong artist")
	}
}

func TestSearcher_MissingRecordingIsAnError(t *testing.T) {
	f := sampleFixture()
	f.Searches.Searches = f.Searches.Searches[1:] // drop Intro by Metallica
	_, err := Evaluate(context.Background(), f)
	if !errors.Is(err, ErrNoRecording) {
		t.Fatalf("err = %v, want ErrNoRecording", err)
	}
}

func TestSearcher(t *testing.T) {
	s, err := NewSearcher(sampleFixture().Searches)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if c, err := s.SearchTracks(ctx, "Intro", "Metallica", "GB"); c != nil || err != nil {
		t.Errorf("recorded empty: %v, %v; want nil, nil", c, err)
	}
	if _, err := s.SearchTracks(ctx, "Battery", "Metallica", "US"); err == nil {
		t.Error("another market should be an error")
	}
	if _, err := s.SearchTracks(ctx, "battery", "Metallica", "GB"); !errors.Is(err, ErrNoRecording) {
		t.Errorf("keys are exact: err = %v", err)
	}
	c, _ := s.SearchTracks(ctx, "Battery", "Metallica", "GB")
	c[0].Artists[0] = "changed"
	c2, _ := s.SearchTracks(ctx, "Battery", "Metallica", "GB")
	if c2[0].Artists[0] != "Metallica" {
		t.Error("a caller changed the recording")
	}

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if _, err := s.SearchTracks(ctx, "Battery", "Metallica", "GB"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	dup := sampleFixture().Searches
	dup.Searches = append(dup.Searches, dup.Searches[0])
	if _, err := NewSearcher(dup); err == nil {
		t.Error("a duplicate recording should be an error")
	}
}

func TestLoad_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "abc123")
	f := sampleFixture()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]any{SetlistFile: f.Setlist, SearchesFile: f.Searches, ExpectedFile: f.Expected} {
		if err := WriteJSON(filepath.Join(dir, name), v); err != nil {
			t.Fatal(err)
		}
	}
	dirs, err := Dirs(filepath.Dir(dir))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("Dirs = %v, %v", dirs, err)
	}
	got, err := Load(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	r, err := Evaluate(context.Background(), got)
	if err != nil || len(r.Mismatches) > 0 {
		t.Fatalf("after round trip: %v, %v", err, r.Mismatches)
	}
}
