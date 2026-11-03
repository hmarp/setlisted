package matching

// This file is the single place that lists the version markers the matching
// rules recognise (spec MATCH-4, MATCH-5, MATCH-6). Markers are compared as
// whole words (or whole runs of words) against a title suffix or album title
// after title normalisation (case-folded, accents removed, punctuation turned
// into spaces), so "Remastered 2021", "(LIVE at Wembley)" and "Live!" all
// contain a marker, but "Alive" and "Demolition" don't.

// versionMarkers mark a trailing title segment as a version suffix to strip
// before comparing titles (MATCH-4). "live" also makes a suffix live
// (MATCH-6); it's liveMarker, defined once below, so the stripping and the
// live check can't drift apart. A year followed by one of yearVersionWords
// (e.g. "2016 Version", "2015 Recording") is a marker too; see isYearVersion.
var versionMarkers = []string{
	"remaster",
	"remastered",
	liveMarker,
	"single version",
	"explicit",
	"mono",
	"stereo",
	"deluxe",
	"radio edit",
	"edit",
}

// yearVersionWords make "‹year› ‹word›" a version marker (the spec's
// "‹year› Version"). "recording" was added in step 09 for re-recordings such
// as Steven Wilson's "Lazarus (2015 Recording)".
var yearVersionWords = []string{"version", "recording"}

// excludedMarkers identify a version that must never be chosen (MATCH-5),
// whether they're in a title suffix or the album title. A title segment that
// contains one is also stripped as a version suffix, so the rest of the title
// can still be compared and the version recognised (and excluded). Plurals
// and "remixed" are included because an album called "The Demos" or
// "Remixed" identifies its tracks as demos or remixes just as well.
var excludedMarkers = []string{
	"demo", "demos",
	"remix", "remixes", "remixed",
	"instrumental", "instrumentals",
	"acoustic",
	"karaoke",
	"tribute",
}

// liveMarker makes a version live when it's in a stripped title suffix or is
// a whole word of the album title (MATCH-6).
const liveMarker = "live"
