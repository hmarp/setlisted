package quality

import (
	"context"
	"fmt"
	"slices"

	"github.com/hmarp/setlisted/internal/normalize"
	"github.com/hmarp/setlisted/internal/pipeline"
)

// Report is one setlist's quality result.
type Report struct {
	SetlistID string
	Band      string
	Date      string
	Found     int // shown entries with a found status
	Matchable int // spec §1: not skipped and not a medley
	// WrongArtist lists found tracks none of whose credited artists is the
	// expected artist (MATCH-3), checked here independently of the matcher.
	WrongArtist []string
	// Mismatches lists entries whose status, labels or track differ from
	// expected.json, and problems with expected.json itself.
	Mismatches []string
	Results    []pipeline.EntryResult
}

// Ratio is Found / Matchable (1 when nothing is matchable).
func (r Report) Ratio() float64 {
	if r.Matchable == 0 {
		return 1
	}
	return float64(r.Found) / float64(r.Matchable)
}

// Evaluate runs the pipeline offline on f and compares the results with
// f.Expected. An error means the fixture couldn't be run at all (e.g. a
// missing recording); quality problems are in the report.
func Evaluate(ctx context.Context, f Fixture) (Report, error) {
	show := f.Setlist.Show()
	r := Report{SetlistID: show.ID, Band: show.Band, Date: show.Date, Matchable: show.Matchable()}

	s, err := NewSearcher(f.Searches)
	if err != nil {
		return r, err
	}
	if s.Market() == "" {
		return r, fmt.Errorf("quality: %s has no market", SearchesFile)
	}
	results, err := pipeline.MatchShow(ctx, show, s, s.Market())
	if err != nil {
		return r, err
	}
	r.Results = results

	for _, res := range results {
		if !res.Status.Found() {
			continue
		}
		r.Found++
		if res.Track == nil {
			r.WrongArtist = append(r.WrongArtist, fmt.Sprintf("#%d %q: found with no track", res.Position, res.Name))
			continue
		}
		want := show.Band
		if res.Status == pipeline.StatusCoverFallback {
			want = res.OriginalArtist
		}
		if !credits(res.Track.Artists, want) {
			r.WrongArtist = append(r.WrongArtist, fmt.Sprintf("#%d %q: track %s %q is by %v, expected %q",
				res.Position, res.Name, res.Track.ID, res.Track.Title, res.Track.Artists, want))
		}
	}

	if f.Expected == nil {
		r.Mismatches = append(r.Mismatches, "no "+ExpectedFile)
		return r, nil
	}
	r.Mismatches = append(r.Mismatches, compare(*f.Expected, show.ID, results)...)
	return r, nil
}

// credits reports whether one of artists has the same normalised name as
// want (MATCH-3), using normalize.Name directly rather than the matcher.
func credits(artists []string, want string) bool {
	w := normalize.Name(want)
	if w == "" {
		return false
	}
	return slices.ContainsFunc(artists, func(a string) bool { return normalize.Name(a) == w })
}

func compare(exp Expected, setlistID string, results []pipeline.EntryResult) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	if exp.SetlistID != setlistID {
		add("%s: setlistId %q, but the setlist is %q", ExpectedFile, exp.SetlistID, setlistID)
	}
	for _, e := range exp.Entries {
		if e.Status.Found() != (e.TrackID != "") {
			add("%s #%d %q: a trackId is needed exactly for the found statuses", ExpectedFile, e.Position, e.Name)
		}
		if e.Status == pipeline.StatusLive && !e.Live {
			add("%s #%d %q: status live needs live: true", ExpectedFile, e.Position, e.Name)
		}
	}
	if len(exp.Entries) != len(results) {
		add("%s has %d entries, the pipeline gave %d", ExpectedFile, len(exp.Entries), len(results))
	}
	for i := range min(len(exp.Entries), len(results)) {
		e, r := exp.Entries[i], results[i]
		if e.Position != r.Position || e.Name != r.Name {
			add("entry %d: expected #%d %q, got #%d %q", i+1, e.Position, e.Name, r.Position, r.Name)
			continue
		}
		id := ""
		if r.Track != nil {
			id = r.Track.ID
		}
		var diffs []string
		if e.Status != r.Status {
			diffs = append(diffs, fmt.Sprintf("status %s, got %s", e.Status, r.Status))
		}
		if e.Live != r.Live {
			diffs = append(diffs, fmt.Sprintf("live %t, got %t", e.Live, r.Live))
		}
		if e.Tape != r.IsTape {
			diffs = append(diffs, fmt.Sprintf("tape %t, got %t", e.Tape, r.IsTape))
		}
		if e.TrackID != id {
			got := "none"
			if r.Track != nil {
				got = fmt.Sprintf("%s (%q on %q)", id, r.Track.Title, r.Track.Album)
			}
			diffs = append(diffs, fmt.Sprintf("track %q, got %s", e.TrackID, got))
		}
		if len(diffs) > 0 {
			add("#%d %q: %v", e.Position, e.Name, diffs)
		}
	}
	return out
}
