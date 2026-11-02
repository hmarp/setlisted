// Package setlist holds the app's own model of a show's setlist and the
// entry classification rules of spec §5.1–5.3 (ENT-1 to ENT-7), plus the
// expected artists that matching uses (MATCH-1, MATCH-2).
//
// It's pure code with no I/O (ADR-005) and doesn't depend on setlist.fm's
// JSON: setlistfm.Setlist.Show converts a fetched setlist into a Show.
package setlist

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/hmarp/setlisted/internal/normalize"
)

// Kind is how an entry is handled (spec §5).
type Kind string

const (
	// KindSkipped entries (ENT-1) are left out of the preview and the
	// playlist (ENT-2).
	KindSkipped Kind = "skipped"
	// KindMedley entries (ENT-3) are shown as "Not found (medley)" and never
	// matched (ENT-4).
	KindMedley Kind = "medley"
	// KindSong entries are matched (ENT-5): songs, tapes and covers.
	KindSong Kind = "song"
)

// Header is a show's details, as the preview's header shows them (PREV-2).
type Header struct {
	// ID is setlist.fm's setlist ID.
	ID string
	// Band is the performing artist's name.
	Band string
	// Date is the show date as DD Mon YYYY (e.g. "14 Mar 2026").
	Date  string
	Venue string
	City  string
	// Tour is the tour name, or "" when the show has none.
	Tour string
	// URL is the show's page on setlist.fm.
	URL string
}

// Show is a show with its classified entries.
type Show struct {
	Header
	// Sets are in setlist order. Each holds all its entries, skipped ones
	// included, so a set can have no visible entries.
	Sets []Set
}

// Set is one set of a show.
type Set struct {
	// Heading is "" for the first (main) set. Later sets use setlist.fm's
	// set name, else "Encore N" from the encore number, else "Set N" from
	// the set's 1-based position in the show (PREV-3).
	Heading string
	Entries []Entry
}

// Entry is one classified setlist entry.
type Entry struct {
	// Position is the entry's number in the preview: 1…n across the whole
	// show, in setlist order, counting only entries that aren't skipped.
	// It's 0 for skipped entries, which aren't shown (ENT-2, PREV-3).
	Position int
	// Name is the entry's name as on setlist.fm.
	Name string
	// Note is setlist.fm's note on the entry. It's shown but never affects
	// matching (ENT-6).
	Note   string
	IsTape bool
	// OriginalArtist is the artist setlist.fm credits the song to when it's
	// a cover, else "".
	OriginalArtist string
	Kind           Kind
	// ExpectedArtists are the artists to match against, in order: [band]
	// for a song or an unattributed tape (MATCH-1), [band, original artist]
	// for a cover (MATCH-2). Empty for skipped entries and medleys.
	ExpectedArtists []string
}

// IsCover reports whether setlist.fm credits the entry to another artist.
func (e Entry) IsCover() bool { return e.OriginalArtist != "" }

// SourceSet is one set as setlist.fm records it, the input to Build.
type SourceSet struct {
	Name string
	// Encore is the encore number (1, 2, …), or 0 for a set that isn't an
	// encore.
	Encore  int
	Entries []SourceEntry
}

// SourceEntry is one entry as setlist.fm records it, the input to Build.
type SourceEntry struct {
	Name string
	Note string
	Tape bool
	// CoverArtist is the name of the original artist setlist.fm gives, or ""
	// when it gives none.
	CoverArtist string
}

// Build classifies every entry of a show and numbers the visible ones.
// Order and duplicates are kept (ENT-7).
func Build(h Header, sets []SourceSet) Show {
	show := Show{Header: h, Sets: make([]Set, 0, len(sets))}
	pos := 0
	for i, src := range sets {
		set := Set{Heading: heading(i, src), Entries: make([]Entry, 0, len(src.Entries))}
		for _, se := range src.Entries {
			e := classifyEntry(h.Band, se)
			if e.Kind != KindSkipped {
				pos++
				e.Position = pos
			}
			set.Entries = append(set.Entries, e)
		}
		show.Sets = append(show.Sets, set)
	}
	return show
}

// heading is the heading of the set at index i (0-based) of a show.
func heading(i int, s SourceSet) string {
	switch {
	case i == 0:
		return ""
	case strings.TrimSpace(s.Name) != "":
		return strings.TrimSpace(s.Name)
	case s.Encore > 0:
		return "Encore " + strconv.Itoa(s.Encore)
	default:
		return "Set " + strconv.Itoa(i+1)
	}
}

func classifyEntry(band string, se SourceEntry) Entry {
	e := Entry{
		Name:   se.Name,
		Note:   se.Note,
		IsTape: se.Tape,
		Kind:   Classify(se.Name),
	}
	original := strings.TrimSpace(se.CoverArtist)
	if original != "" && normalize.Name(original) != normalize.Name(band) {
		e.OriginalArtist = original
	}
	if e.Kind == KindSong {
		e.ExpectedArtists = []string{band}
		if e.IsCover() {
			e.ExpectedArtists = append(e.ExpectedArtists, e.OriginalArtist)
		}
	}
	return e
}

// Classify gives the kind of an entry from its name, applying spec §5.1,
// then §5.2, then §5.3. Notes, tapes and covers don't affect the kind
// (ENT-5, ENT-6).
func Classify(name string) Kind {
	n := normalize.Name(name)
	switch {
	case isSkipped(n):
		return KindSkipped
	case isMedley(n):
		return KindMedley
	default:
		return KindSong
	}
}

// isSkipped applies ENT-1 to a normalised name: no name, or a whole name
// that is "solo", "… solo", "jam" or "improvisation".
func isSkipped(n string) bool {
	switch n {
	case "", "solo", "jam", "improvisation":
		return true
	}
	return strings.HasSuffix(n, " solo")
}

// isMedley applies ENT-3 to a normalised name: it contains the word
// "medley", or " / " between song names.
func isMedley(n string) bool {
	if strings.Contains(n, " / ") {
		return true
	}
	words := strings.FieldsFunc(n, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		if w == "medley" {
			return true
		}
	}
	return false
}

// Entries returns every entry of the show in setlist order, skipped ones
// included.
func (s Show) Entries() []Entry {
	var all []Entry
	for _, set := range s.Sets {
		all = append(all, set.Entries...)
	}
	return all
}

// Matchable is the number of matchable entries: those that are neither
// skipped nor medleys (spec §1, ENT-2, ENT-4).
func (s Show) Matchable() int {
	n := 0
	for _, set := range s.Sets {
		for _, e := range set.Entries {
			if e.Kind == KindSong {
				n++
			}
		}
	}
	return n
}
