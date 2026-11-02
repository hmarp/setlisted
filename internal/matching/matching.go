// Package matching holds the matching rules of spec §5.4 (MATCH-1 to
// MATCH-9): given an entry name, an expected artist and Spotify candidate
// tracks, it chooses the right track or decides there isn't one. A wrong
// match is worse than a missing one, so there are no guesses (MATCH-9).
//
// It's pure code with no I/O (ADR-005): candidates come in as plain values,
// and the Spotify client (step 08) is separate. The version-marker lists are
// in markers.go.
package matching

import (
	"sort"
	"strings"
	"time"

	"github.com/hmarp/setlisted/internal/normalize"
	"github.com/hmarp/setlisted/internal/setlist"
)

// Album types, as Spotify's album.album_type gives them.
const (
	AlbumTypeAlbum       = "album"
	AlbumTypeSingle      = "single" // Spotify doesn't distinguish EPs (MATCH-7)
	AlbumTypeCompilation = "compilation"
)

// Release date precisions, as Spotify's album.release_date_precision gives
// them.
const (
	PrecisionYear  = "year"
	PrecisionMonth = "month"
	PrecisionDay   = "day"
)

// Candidate is a Spotify track found by a search, with only the fields the
// rules read.
type Candidate struct {
	ID      string
	Title   string   // track name
	Artists []string // names of all credited artists
	Album   string   // album name
	// AlbumType is "album", "single" or "compilation".
	AlbumType string
	// ReleaseDate is the album's release date: "YYYY", "YYYY-MM" or
	// "YYYY-MM-DD". MATCH-8 reads it by its shape, as the start of that
	// period.
	ReleaseDate string
	// ReleaseDatePrecision is Spotify's "year", "month" or "day". It's
	// informational only: the rules never read it, because ReleaseDate's
	// shape already gives the precision, so a missing or conflicting value
	// changes nothing.
	ReleaseDatePrecision string
}

// Version is how a candidate's version markers classify it.
type Version string

const (
	// VersionStudio is any version that isn't live or excluded.
	VersionStudio Version = "studio"
	// VersionLive is chosen only when there's no studio candidate (MATCH-6).
	VersionLive Version = "live"
	// VersionExcluded (demo, remix, instrumental, acoustic, karaoke,
	// tribute) is never chosen (MATCH-5).
	VersionExcluded Version = "excluded"
)

// Classify gives a candidate's version from its stripped title suffixes and
// its album title, never from the core of its title, so "Live Wire" is a
// studio song.
//
//   - excluded if a suffix or the album title contains an exclusion marker
//     (MATCH-5);
//   - otherwise live if a suffix or the album title contains the word "live"
//     (MATCH-6);
//   - otherwise studio.
func Classify(c Candidate) Version {
	_, suffixes := splitTitle(c.Title)
	sources := make([][]string, 0, len(suffixes)+1)
	for _, s := range suffixes {
		sources = append(sources, strings.Fields(words(s)))
	}
	sources = append(sources, strings.Fields(words(c.Album)))

	for _, ws := range sources {
		if containsAny(ws, excludedMarkers) {
			return VersionExcluded
		}
	}
	for _, ws := range sources {
		if containsPhrase(ws, []string{liveMarker}) {
			return VersionLive
		}
	}
	return VersionStudio
}

// IsCandidate reports whether c is a candidate for an entry (MATCH-3 and
// MATCH-4): one of its credited artists has the expected artist's normalised
// name, and its title equals the entry name once both are normalised with
// Title. Exclusions (MATCH-5) aren't applied here.
func IsCandidate(entryName, expectedArtist string, c Candidate) bool {
	return artistMatches(expectedArtist, c.Artists) && titleMatches(entryName, c.Title)
}

func artistMatches(expected string, artists []string) bool {
	want := normalize.Name(expected)
	if want == "" {
		return false
	}
	for _, a := range artists {
		if normalize.Name(a) == want {
			return true
		}
	}
	return false
}

func titleMatches(entryName, title string) bool {
	want := Title(entryName)
	return want != "" && Title(title) == want
}

// Choose picks the track for an entry from the candidates of one search:
//
//  1. keep tracks credited to the expected artist (MATCH-3) whose title
//     matches the entry name (MATCH-4);
//  2. drop excluded versions (MATCH-5);
//  3. prefer studio over live (MATCH-6);
//  4. then album over single/EP over compilation (MATCH-7);
//  5. then the earliest release date (MATCH-8), with year and month
//     precisions taken as the start of that period. Remaining ties keep the
//     input order.
//
// ok is false when nothing is left (MATCH-9). isLive is true when the chosen
// track is a live version (labelled "Live version").
func Choose(entryName, expectedArtist string, candidates []Candidate) (c Candidate, isLive bool, ok bool) {
	type ranked struct {
		c    Candidate
		live bool
		typ  int
		date time.Time
	}
	var kept []ranked
	for _, cand := range candidates {
		if !IsCandidate(entryName, expectedArtist, cand) {
			continue
		}
		v := Classify(cand)
		if v == VersionExcluded {
			continue
		}
		kept = append(kept, ranked{c: cand, live: v == VersionLive, typ: albumTypeRank(cand.AlbumType), date: releaseDate(cand)})
	}
	if len(kept) == 0 {
		return Candidate{}, false, false
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if a.live != b.live {
			return !a.live
		}
		if a.typ != b.typ {
			return a.typ < b.typ
		}
		return a.date.Before(b.date)
	})
	return kept[0].c, kept[0].live, true
}

// albumTypeRank orders album types for MATCH-7. An unknown type comes after
// compilations.
func albumTypeRank(t string) int {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case AlbumTypeAlbum:
		return 0
	case AlbumTypeSingle:
		return 1
	case AlbumTypeCompilation:
		return 2
	default:
		return 3
	}
}

// unknownDate sorts after every real date, so a candidate whose release date
// can't be read loses ties to any that can.
var unknownDate = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

// releaseDate parses a candidate's release date as the start of its period:
// "1984" → 1 Jan 1984, "1984-07" → 1 Jul 1984. The layout follows the
// string's shape, so a precision that disagrees with the string (or is
// missing) doesn't matter; a date that doesn't parse, or has year 0
// (Spotify's "0000" placeholder), is unknownDate.
func releaseDate(c Candidate) time.Time {
	s := strings.TrimSpace(c.ReleaseDate)
	layout := ""
	switch len(s) {
	case 4:
		layout = "2006"
	case 7:
		layout = "2006-01"
	case 10:
		layout = "2006-01-02"
	}
	if layout == "" {
		return unknownDate
	}
	t, err := time.Parse(layout, s)
	if err != nil || t.Year() == 0 {
		// Year 0 is Spotify's placeholder ("0000") for an unknown date.
		return unknownDate
	}
	return t
}

// Result is the outcome of matching one entry.
type Result struct {
	// Found is false when the entry is Not found (MATCH-9); the other fields
	// are then zero.
	Found bool
	Track Candidate
	// Artist is the expected artist of the pass that found Track.
	Artist string
	// CoverFallback is true when Track came from the original artist's pass
	// of a cover (MATCH-2).
	CoverFallback bool
	// Live is true when Track is a live version (MATCH-6). It can be true
	// together with CoverFallback.
	Live bool
}

// CandidatesFunc returns the candidate tracks for an entry when matched
// against one expected artist. Step 08 implements it with a Spotify search;
// an error stops matching and is returned by Resolve as it is.
type CandidatesFunc func(expectedArtist string) ([]Candidate, error)

// Resolve matches a classified entry, trying its expected artists in order
// (MATCH-1, MATCH-2): the band first and, for a cover, the original artist
// only if the band pass chose nothing. candidatesFor is called once per pass
// tried, so a cover found under the band costs one search.
//
// Only entry.Name and entry.ExpectedArtists are read; the note never affects
// matching (ENT-6). Entries that aren't setlist.KindSong are never matched:
// Resolve returns a not-found Result without calling candidatesFor.
func Resolve(entry setlist.Entry, candidatesFor CandidatesFunc) (Result, error) {
	if entry.Kind != setlist.KindSong {
		return Result{}, nil
	}
	for i, artist := range entry.ExpectedArtists {
		cands, err := candidatesFor(artist)
		if err != nil {
			return Result{}, err
		}
		if c, live, ok := Choose(entry.Name, artist, cands); ok {
			return Result{Found: true, Track: c, Artist: artist, CoverFallback: i > 0, Live: live}, nil
		}
	}
	return Result{}, nil
}
