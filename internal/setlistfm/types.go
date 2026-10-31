package setlistfm

import "time"

// Artist is a setlist.fm artist, identified by its MusicBrainz ID.
type Artist struct {
	MBID           string `json:"mbid"`
	Name           string `json:"name"`
	Disambiguation string `json:"disambiguation"`
}

// Setlist is one show: a setlist.fm event for an artist. Only the fields the
// app uses are decoded.
type Setlist struct {
	ID string `json:"id"`
	// EventDate is setlist.fm's dd-MM-yyyy date. Use Date to parse it.
	EventDate string `json:"eventDate"`
	Artist    Artist `json:"artist"`
	Venue     Venue  `json:"venue"`
	// Tour is nil when setlist.fm has no tour for the show.
	Tour *Tour  `json:"tour"`
	Sets Sets   `json:"sets"`
	URL  string `json:"url"`
}

// Venue is where a show took place.
type Venue struct {
	Name string `json:"name"`
	City City   `json:"city"`
}

// City is a venue's city.
type City struct {
	Name string `json:"name"`
}

// Tour is the tour a show belongs to.
type Tour struct {
	Name string `json:"name"`
}

// Sets wraps the list of sets, as setlist.fm nests it ("sets": {"set": []}).
type Sets struct {
	Set []Set `json:"set"`
}

// Set is one set of a show: the main set, a named set or an encore.
type Set struct {
	// Name is the set's name, if it has one (e.g. "Acoustic").
	Name string `json:"name"`
	// Encore is the encore number (1, 2, …), or 0 for a set that isn't an
	// encore.
	Encore int    `json:"encore"`
	Song   []Song `json:"song"`
}

// Song is one setlist entry. Despite the name it can be a tape, a medley or
// an entry with no name.
type Song struct {
	Name string `json:"name"`
	// Tape is true for a recording played rather than a song performed.
	Tape bool `json:"tape"`
	// Cover is the original artist when the entry is a cover, else nil.
	Cover *Artist `json:"cover"`
	// Info is setlist.fm's free-text note on the entry.
	Info string `json:"info"`
}

// SongCount is the number of entries across all sets, including tapes and
// entries with no name (spec SHOW-3).
func (s Setlist) SongCount() int {
	n := 0
	for _, set := range s.Sets.Set {
		n += len(set.Song)
	}
	return n
}

// eventDateLayout is setlist.fm's dd-MM-yyyy date format.
const eventDateLayout = "02-01-2006"

// Date parses EventDate. The result is a date at midnight UTC.
func (s Setlist) Date() (time.Time, error) {
	return time.Parse(eventDateLayout, s.EventDate)
}

// FormatDate formats a date as the spec's DD Mon YYYY (e.g. "14 Mar 2026"),
// with English month abbreviations.
func FormatDate(t time.Time) string { return t.Format("02 Jan 2006") }

// DisplayDate is EventDate as DD Mon YYYY, or EventDate unchanged if
// setlist.fm sent something that doesn't parse.
func (s Setlist) DisplayDate() string {
	t, err := s.Date()
	if err != nil {
		return s.EventDate
	}
	return FormatDate(t)
}

// SetlistsPage is one page of an artist's setlists, newest first.
type SetlistsPage struct {
	Setlists     []Setlist `json:"setlist"`
	Total        int       `json:"total"`
	Page         int       `json:"page"`
	ItemsPerPage int       `json:"itemsPerPage"`
}

// HasMore reports whether a later page has setlists. A page with no items
// is the last.
func (p SetlistsPage) HasMore() bool {
	return len(p.Setlists) > 0 && p.Page*p.ItemsPerPage < p.Total
}

type artistsPage struct {
	Artists []Artist `json:"artist"`
}
