// Package pipeline matches a classified show (step 06) against Spotify: it
// runs the searches each entry needs, applies the matching rules (step 07)
// and returns one result per shown entry, in setlist order, ready for the
// preview (step 10).
package pipeline

import (
	"context"
	"errors"
	"sync"

	"github.com/hmarp/setlisted/internal/matching"
	"github.com/hmarp/setlisted/internal/setlist"
	"github.com/hmarp/setlisted/internal/spotify"
)

// DefaultConcurrency is the most searches run at once for one show
// (ADR-005).
const DefaultConcurrency = 4

// Searcher finds Spotify candidates for one (track, artist) pair in market,
// in Spotify's order. No results is (nil, nil); an error is a real failure.
//
// spotify.Client.WithToken(token) implements it for live calls; step 09's
// fixtures implement it from recorded responses.
type Searcher interface {
	SearchTracks(ctx context.Context, track, artist, market string) ([]matching.Candidate, error)
}

// SearcherFunc adapts a function to Searcher.
type SearcherFunc func(ctx context.Context, track, artist, market string) ([]matching.Candidate, error)

// SearchTracks calls f.
func (f SearcherFunc) SearchTracks(ctx context.Context, track, artist, market string) ([]matching.Candidate, error) {
	return f(ctx, track, artist, market)
}

// Status is an entry's outcome (PREV-4).
type Status string

const (
	StatusFound          Status = "found"
	StatusCoverFallback  Status = "cover_fallback" // found under the original artist (MATCH-2); Live may also be true
	StatusLive           Status = "live"           // band's own live version (MATCH-6)
	StatusNotFound       Status = "not_found"      // MATCH-9
	StatusNotFoundMedley Status = "not_found_medley"
)

// Found reports whether s is one of the three "Found" statuses.
func (s Status) Found() bool {
	return s == StatusFound || s == StatusCoverFallback || s == StatusLive
}

// Track is the matched Spotify track, as the preview shows it.
type Track struct {
	ID      string
	Title   string
	Artists []string // all credited artists, in Spotify's order
	Album   string
}

// EntryResult is one shown entry (skipped entries have none) and its
// outcome.
type EntryResult struct {
	// From the entry (step 06).
	Position       int    // the preview's row number
	SetHeading     string // "" for the main set
	Name           string // as on setlist.fm
	Note           string
	IsTape         bool
	OriginalArtist string // "" unless it's a cover

	Status Status
	// Live is true for any live match: Status == StatusLive, or a cover
	// fallback whose chosen track is a live version (Status ==
	// StatusCoverFallback && Live). The preview shows both labels then.
	Live  bool
	Track *Track // nil unless Status.Found()
}

// MatchShow is MatchShowLimit with DefaultConcurrency.
func MatchShow(ctx context.Context, show setlist.Show, s Searcher, market string) ([]EntryResult, error) {
	return MatchShowLimit(ctx, show, s, market, DefaultConcurrency)
}

// MatchShowLimit matches every entry of show and returns the results in
// setlist order. Skipped entries are dropped; medleys are not_found_medley
// without a search; songs go through matching.Resolve, which searches the
// original artist of a cover only if the band pass found nothing.
//
// At most limit searches run at once (limit < 1 means 1): each of limit
// workers resolves one entry at a time, and an entry's passes run in
// sequence. If any search fails, the outstanding ones are cancelled and an
// error is returned unchanged (errors.Is works), with no results: the user
// retries the whole preview (ERR-1). Which error:
//
//   - if ctx itself was cancelled or timed out, ctx's error (context.Cause),
//     even if the searches reported it wrapped in another error, so a
//     client that went away isn't reported as a Spotify failure;
//   - otherwise spotify.ErrUnauthorized if any search returned it, even after
//     another search failed first, so an expired token always leads back to
//     login (AUTH-8);
//   - otherwise the first search error.
func MatchShowLimit(ctx context.Context, show setlist.Show, s Searcher, market string, limit int) ([]EntryResult, error) {
	var results []EntryResult
	var songs []int // indexes into results of entries to resolve
	var entries []setlist.Entry
	for _, set := range show.Sets {
		for _, e := range set.Entries {
			r := EntryResult{
				Position:       e.Position,
				SetHeading:     set.Heading,
				Name:           e.Name,
				Note:           e.Note,
				IsTape:         e.IsTape,
				OriginalArtist: e.OriginalArtist,
			}
			switch e.Kind {
			case setlist.KindSkipped:
				continue
			case setlist.KindMedley:
				r.Status = StatusNotFoundMedley
			default:
				songs = append(songs, len(results))
				entries = append(entries, e)
			}
			results = append(results, r)
		}
	}
	if len(songs) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return results, nil
	}

	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		firstErr error
		next     int
	)
	// take hands out the next entry to resolve, or -1 when there's none
	// left or the match has failed.
	take := func() int {
		mu.Lock()
		defer mu.Unlock()
		if firstErr != nil || next >= len(songs) {
			return -1
		}
		next++
		return next - 1
	}
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case firstErr == nil:
			firstErr = err
			cancel()
		case errors.Is(err, spotify.ErrUnauthorized) && !errors.Is(firstErr, spotify.ErrUnauthorized):
			firstErr = err
		}
	}

	limit = max(1, min(limit, len(songs)))
	var wg sync.WaitGroup
	for range limit {
		wg.Go(func() {
			for i := take(); i >= 0; i = take() {
				if err := ctx.Err(); err != nil {
					fail(context.Cause(ctx))
					return
				}
				e := entries[i]
				res, err := matching.Resolve(e, func(artist string) ([]matching.Candidate, error) {
					return s.SearchTracks(ctx, e.Name, artist, market)
				})
				if err != nil {
					fail(err)
					return
				}
				apply(&results[songs[i]], res)
			}
		})
	}
	wg.Wait()

	if firstErr != nil {
		if parent.Err() != nil {
			return nil, context.Cause(parent)
		}
		return nil, firstErr
	}
	return results, nil
}

// apply sets r's status and track from step 07's result.
func apply(r *EntryResult, res matching.Result) {
	if !res.Found {
		r.Status = StatusNotFound
		return
	}
	switch {
	case res.CoverFallback:
		r.Status = StatusCoverFallback
	case res.Live:
		r.Status = StatusLive
	default:
		r.Status = StatusFound
	}
	r.Live = res.Live
	r.Track = &Track{
		ID:      res.Track.ID,
		Title:   res.Track.Title,
		Artists: res.Track.Artists,
		Album:   res.Track.Album,
	}
}
