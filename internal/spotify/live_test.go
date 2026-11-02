//go:build spotifylive

// The manual check against live Spotify (step 08). Never run in CI: it needs
// a real access token in the environment and is only built with
//
//	go test -tags spotifylive -run Live -v ./internal/spotify/
//
// SPOTIFY_TEST_TOKEN is a user or app (client-credentials) access token.
// SPOTIFY_TEST_MARKET defaults to GB; use from_token only with a user token.
// The token is never printed.
package spotify_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/hmarp/setlisted/internal/pipeline"
	"github.com/hmarp/setlisted/internal/setlist"
	"github.com/hmarp/setlisted/internal/spotify"
)

func TestLiveSearch(t *testing.T) {
	token := os.Getenv("SPOTIFY_TEST_TOKEN")
	if token == "" {
		t.Skip("SPOTIFY_TEST_TOKEN not set")
	}
	market := os.Getenv("SPOTIFY_TEST_MARKET")
	if market == "" {
		market = "GB"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := spotify.New(spotify.Config{})

	cands, err := c.SearchTracks(ctx, token, "Battery", "Metallica", market)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Logf("Battery / Metallica, market %s: %d candidates", market, len(cands))
	for i, cd := range cands {
		t.Logf("  %2d %s | %q | %v | %q (%s, %s/%s)", i+1, cd.ID, cd.Title, cd.Artists, cd.Album,
			cd.AlbumType, cd.ReleaseDate, cd.ReleaseDatePrecision)
	}
	if len(cands) == 0 {
		t.Fatal("no candidates for a well-known song")
	}

	// Through the pipeline: a song, a cover found under the band, a cover
	// that needs the original artist, a title with double quotes, a skipped
	// entry and a medley.
	show := setlist.Build(setlist.Header{Band: "Metallica"}, []setlist.SourceSet{
		{Entries: []setlist.SourceEntry{
			{Name: "Battery"},
			{Name: "Am I Evil?", CoverArtist: "Diamond Head"},
			{Name: "Helpless", CoverArtist: "Diamond Head"},
			{Name: "Drum Solo"},
			{Name: `"Heroes"`, CoverArtist: "David Bowie"},
			{Name: "Medley: A / B"},
		}},
	})
	start := time.Now()
	rows, err := pipeline.MatchShow(ctx, show, c.WithToken(token), market)
	if err != nil {
		t.Fatalf("MatchShow: %v", err)
	}
	t.Logf("MatchShow took %v", time.Since(start).Round(time.Millisecond))
	for _, r := range rows {
		if r.Track == nil {
			t.Logf("  %d %q → %s", r.Position, r.Name, r.Status)
			continue
		}
		t.Logf("  %d %q → %s (live %v): %s %q by %v on %q", r.Position, r.Name, r.Status, r.Live,
			r.Track.ID, r.Track.Title, r.Track.Artists, r.Track.Album)
	}
	if rows[0].Track == nil || !slices.Contains(rows[0].Track.Artists, "Metallica") {
		t.Errorf("Battery not matched to Metallica: %+v", rows[0])
	}
}
