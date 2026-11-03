// Package quality holds the matching-quality fixtures (step 09, ADR-006):
// the on-disk format of recorded setlist.fm and Spotify data, a
// pipeline.Searcher that replays recorded searches, and the expected
// outcomes the quality test compares against.
//
// Layout, one directory per setlist under testdata/quality/<setlist-id>/:
//
//	setlist.json   the setlist.fm setlist, trimmed to the fields the app decodes
//	searches.json  every Spotify search the pipeline can make for it, trimmed
//	expected.json  the owner-reviewed expected outcome of each shown entry
//
// The recorder (cmd/recordfixtures, build tag live) writes the first two;
// expected.json is written by hand and never overwritten by the recorder.
package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hmarp/setlisted/internal/matching"
	"github.com/hmarp/setlisted/internal/pipeline"
	"github.com/hmarp/setlisted/internal/setlistfm"
)

// File names inside a setlist's fixture directory.
const (
	SetlistFile  = "setlist.json"
	SearchesFile = "searches.json"
	ExpectedFile = "expected.json"
)

// Searches is the content of searches.json.
type Searches struct {
	Market   string   `json:"market"`
	Recorded string   `json:"recorded"` // date recorded, YYYY-MM-DD
	Searches []Search `json:"searches"`
}

// Search is one recorded search, keyed by the raw (track, artist) pair the
// pipeline passes to its Searcher.
type Search struct {
	Track   string  `json:"track"`
	Artist  string  `json:"artist"`
	Query   string  `json:"query,omitempty"` // the q= the live client sent, for reference
	Results []Track `json:"results"`         // in Spotify's order; empty for no results
}

// Track is a search result trimmed to the fields the matcher reads.
type Track struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Artists              []string `json:"artists"`
	Album                string   `json:"album"`
	AlbumType            string   `json:"albumType"`
	ReleaseDate          string   `json:"releaseDate"`
	ReleaseDatePrecision string   `json:"releaseDatePrecision"`
}

// FromCandidate trims a live search result for recording.
func FromCandidate(c matching.Candidate) Track {
	return Track{
		ID:                   c.ID,
		Title:                c.Title,
		Artists:              c.Artists,
		Album:                c.Album,
		AlbumType:            c.AlbumType,
		ReleaseDate:          c.ReleaseDate,
		ReleaseDatePrecision: c.ReleaseDatePrecision,
	}
}

// Candidate is the matcher's view of a recorded track.
func (t Track) Candidate() matching.Candidate {
	return matching.Candidate{
		ID:                   t.ID,
		Title:                t.Title,
		Artists:              append([]string(nil), t.Artists...),
		Album:                t.Album,
		AlbumType:            t.AlbumType,
		ReleaseDate:          t.ReleaseDate,
		ReleaseDatePrecision: t.ReleaseDatePrecision,
	}
}

// Expected is the content of expected.json.
type Expected struct {
	SetlistID string  `json:"setlistId"`
	Comment   string  `json:"comment,omitempty"`
	Entries   []Entry `json:"entries"`
}

// Entry is the expected outcome of one shown entry (skipped entries have
// none, as in the preview). Position and Name identify it; Status, Live,
// Tape and TrackID are what's checked.
type Entry struct {
	Position int             `json:"position"`
	Name     string          `json:"name"`
	Status   pipeline.Status `json:"status"`
	// Live is the "Live version" label: true for status live, and for a
	// cover fallback whose chosen track is live.
	Live bool `json:"live,omitempty"`
	// Tape is the extra "Tape" label (PREV-4).
	Tape bool `json:"tape,omitempty"`
	// TrackID is the expected Spotify track for the found statuses.
	TrackID string `json:"trackId,omitempty"`
	// Comment explains a judgement call, for the owner's review.
	Comment string `json:"comment,omitempty"`
}

// ErrNoRecording means the fixture searcher was asked for a search that
// wasn't recorded. It fails the match, so missing data can't pass as "no
// results".
var ErrNoRecording = errors.New("quality: no recorded search")

// Searcher replays recorded searches. It implements pipeline.Searcher and is
// safe for concurrent use (it's read-only after loading).
type Searcher struct {
	market  string
	results map[key][]matching.Candidate
}

type key struct{ track, artist string }

// NewSearcher indexes s. A (track, artist) pair recorded twice is an error.
func NewSearcher(s Searches) (*Searcher, error) {
	fs := &Searcher{market: s.Market, results: make(map[key][]matching.Candidate, len(s.Searches))}
	for _, r := range s.Searches {
		k := key{r.Track, r.Artist}
		if _, dup := fs.results[k]; dup {
			return nil, fmt.Errorf("quality: search for %q by %q recorded twice", r.Track, r.Artist)
		}
		cands := make([]matching.Candidate, 0, len(r.Results))
		for _, t := range r.Results {
			cands = append(cands, t.Candidate())
		}
		fs.results[k] = cands
	}
	return fs, nil
}

// Market is the market the searches were recorded in.
func (s *Searcher) Market() string { return s.market }

// SearchTracks returns the recorded results for (track, artist), or
// ErrNoRecording. A recorded empty result is (nil, nil). market must be the
// recorded one.
func (s *Searcher) SearchTracks(ctx context.Context, track, artist, market string) ([]matching.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if market != s.market {
		return nil, fmt.Errorf("quality: searches were recorded in market %q, not %q", s.market, market)
	}
	cands, ok := s.results[key{track, artist}]
	if !ok {
		return nil, fmt.Errorf("%w for %q by %q", ErrNoRecording, track, artist)
	}
	if len(cands) == 0 {
		return nil, nil
	}
	// A copy, so a caller can't change the recording for later calls.
	out := make([]matching.Candidate, len(cands))
	for i, c := range cands {
		c.Artists = append([]string(nil), c.Artists...)
		out[i] = c
	}
	return out, nil
}

// Fixture is one setlist's recorded data and expectations.
type Fixture struct {
	Dir      string
	Setlist  setlistfm.Setlist
	Searches Searches
	Expected *Expected // nil if expected.json doesn't exist yet
}

// Load reads the fixture in dir. A missing expected.json isn't an error.
func Load(dir string) (Fixture, error) {
	f := Fixture{Dir: dir}
	if err := readJSON(filepath.Join(dir, SetlistFile), &f.Setlist); err != nil {
		return f, err
	}
	if err := readJSON(filepath.Join(dir, SearchesFile), &f.Searches); err != nil {
		return f, err
	}
	var exp Expected
	switch err := readJSON(filepath.Join(dir, ExpectedFile), &exp); {
	case err == nil:
		f.Expected = &exp
	case !errors.Is(err, os.ErrNotExist):
		return f, err
	}
	return f, nil
}

// Dirs lists the fixture directories under root (those with a
// setlist.json), sorted.
func Dirs(root string) ([]string, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		d := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(d, SetlistFile)); err == nil {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// WriteJSON writes v to path as indented JSON with a trailing newline.
func WriteJSON(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
