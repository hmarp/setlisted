package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hmarp/setlisted/internal/jsonapi"
	"github.com/hmarp/setlisted/internal/normalize"
	"github.com/hmarp/setlisted/internal/setlistfm"
)

const (
	// maxPickerArtists is the most artists the picker shows (SRCH-4).
	maxPickerArtists = 10
	// maxQueryLength is the longest band name accepted, in characters.
	maxQueryLength = 200

	// The show list holds up to maxShows non-empty shows, from at most
	// maxShowPages pages of setlists (SHOW-1, ADR-005).
	maxShows     = 20
	maxShowPages = 3

	// setlistFMBudget bounds all of one request's setlist.fm calls, so a
	// slow or rate-limiting setlist.fm ends in ERR-1's 503 well before the
	// server's 60 s write timeout drops the connection.
	setlistFMBudget = 45 * time.Second
)

// Match outcomes of an artist search.
const (
	matchClear  = "clear"  // SRCH-3: go straight to the one artist's shows
	matchPicker = "picker" // SRCH-4: let the user choose
	matchNone   = "none"   // SRCH-5: no artists found
)

// mbidPattern is a MusicBrainz ID: a UUID.
var mbidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type artistJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Disambiguation string `json:"disambiguation"`
}

type artistSearchJSON struct {
	Match   string       `json:"match"`
	Artists []artistJSON `json:"artists"`
}

type showJSON struct {
	ID        string  `json:"id"`
	Date      string  `json:"date"`
	City      string  `json:"city"`
	Venue     string  `json:"venue"`
	Tour      *string `json:"tour"`
	SongCount int     `json:"songCount"`
}

type artistShowsJSON struct {
	Artist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
	Shows []showJSON `json:"shows"`
}

// setlistFMHandlers serves the endpoints backed by setlist.fm.
type setlistFMHandlers struct {
	logger *slog.Logger
	client *setlistfm.Client
	budget time.Duration // setlistFMBudget, shorter in tests
}

// searchArtists serves GET /api/artists?q=… (SRCH-1 to SRCH-5).
func (h *setlistFMHandlers) searchArtists(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		jsonapi.WriteError(w, jsonapi.InvalidRequest("Enter a band name."))
		return
	}
	if utf8.RuneCountInString(q) > maxQueryLength {
		jsonapi.WriteError(w, jsonapi.InvalidRequest("That band name is too long."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()
	artists, err := h.client.SearchArtists(ctx, q)
	if err != nil {
		h.upstreamError(w, "search_artists", err)
		return
	}
	jsonapi.WriteJSON(w, http.StatusOK, classifyArtists(q, artists))
}

// classifyArtists applies the clear-match rule (SRCH-3) to setlist.fm's
// results for query, in setlist.fm's order.
func classifyArtists(query string, artists []setlistfm.Artist) artistSearchJSON {
	if len(artists) == 0 {
		return artistSearchJSON{Match: matchNone, Artists: []artistJSON{}}
	}
	want := normalize.Name(query)
	exact := -1
	for i, a := range artists {
		if normalize.Name(a.Name) != want {
			continue
		}
		if exact >= 0 { // a second exact match: not clear
			exact = -1
			break
		}
		exact = i
	}
	if exact >= 0 {
		return artistSearchJSON{Match: matchClear, Artists: []artistJSON{toArtistJSON(artists[exact])}}
	}
	n := min(len(artists), maxPickerArtists)
	out := make([]artistJSON, n)
	for i := range n {
		out[i] = toArtistJSON(artists[i])
	}
	return artistSearchJSON{Match: matchPicker, Artists: out}
}

func toArtistJSON(a setlistfm.Artist) artistJSON {
	return artistJSON{ID: a.MBID, Name: a.Name, Disambiguation: a.Disambiguation}
}

// artistShows serves GET /api/artists/{id}/shows (SHOW-1 to SHOW-5): up to
// 20 of the artist's most recent shows with at least one entry, from at
// most 3 pages of setlists.
func (h *setlistFMHandlers) artistShows(w http.ResponseWriter, r *http.Request) {
	mbid := r.PathValue("id")
	if !mbidPattern.MatchString(mbid) {
		jsonapi.WriteError(w, jsonapi.NotFound())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()

	var resp artistShowsJSON
	resp.Artist.ID = mbid
	resp.Shows = []showJSON{}

	for page := 1; page <= maxShowPages && len(resp.Shows) < maxShows; page++ {
		p, err := h.client.ArtistSetlists(ctx, mbid, page)
		if err != nil {
			h.upstreamError(w, "artist_setlists", err)
			return
		}
		for _, s := range p.Setlists {
			if resp.Artist.Name == "" {
				resp.Artist.Name = s.Artist.Name
			}
			if len(resp.Shows) == maxShows {
				break
			}
			if s.SongCount() == 0 { // SHOW-2
				continue
			}
			resp.Shows = append(resp.Shows, toShowJSON(s))
		}
		if !p.HasMore() {
			break
		}
	}

	// No setlists at all, so no artist name from them: ask for the artist,
	// which also tells an unknown ID apart from a band with no setlists.
	if resp.Artist.Name == "" {
		a, err := h.client.Artist(ctx, mbid)
		if errors.Is(err, setlistfm.ErrNotFound) {
			jsonapi.WriteError(w, jsonapi.NotFound())
			return
		}
		if err != nil {
			h.upstreamError(w, "artist", err)
			return
		}
		resp.Artist.Name = a.Name
	}
	jsonapi.WriteJSON(w, http.StatusOK, resp)
}

func toShowJSON(s setlistfm.Setlist) showJSON {
	out := showJSON{
		ID:        s.ID,
		Date:      s.DisplayDate(),
		City:      s.Venue.City.Name,
		Venue:     s.Venue.Name,
		SongCount: s.SongCount(),
	}
	if s.Tour != nil && strings.TrimSpace(s.Tour.Name) != "" {
		name := s.Tour.Name
		out.Tour = &name
	}
	return out
}

// upstreamError logs a setlist.fm failure and writes ERR-1's error. The
// error text holds no search text, IDs, key or response body (see package
// setlistfm), so it's safe to log.
func (h *setlistFMHandlers) upstreamError(w http.ResponseWriter, op string, err error) {
	h.logger.Warn("setlist.fm call failed", "op", op, "err", err.Error())
	jsonapi.WriteError(w, jsonapi.UpstreamUnavailable(jsonapi.ServiceSetlistFM))
}
