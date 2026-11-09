package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hmarp/setlisted/internal/auth"
	"github.com/hmarp/setlisted/internal/jsonapi"
	"github.com/hmarp/setlisted/internal/pipeline"
	"github.com/hmarp/setlisted/internal/setlist"
	"github.com/hmarp/setlisted/internal/setlistfm"
	"github.com/hmarp/setlisted/internal/spotify"
)

// previewBudget bounds all of one preview's upstream calls together: the
// setlist.fm fetch and every Spotify search. It leaves 10 s of the server's
// 60 s write timeout, so a slow upstream ends in ERR-1's 503 rather than a
// dropped connection.
const previewBudget = 50 * time.Second

// statusClientClosed is logged (never received) when the browser went away
// before the preview was ready: nginx's "client closed request". It keeps
// a closed tab or a Retry out of the outage warnings.
const statusClientClosed = 499

// setlistIDPattern accepts setlist.fm's setlist IDs (short hex strings, e.g.
// "63de4613") with room to spare. Anything else is an unknown show, without
// calling setlist.fm.
var setlistIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{1,32}$`)

// previewJSON is the GET /api/shows/{id}/preview response (docs/api.md).
type previewJSON struct {
	Show      previewShowJSON  `json:"show"`
	Summary   previewSummary   `json:"summary"`
	CanCreate bool             `json:"canCreate"`
	Sets      []previewSetJSON `json:"sets"`
	TrackIDs  []string         `json:"trackIds"`
}

type previewShowJSON struct {
	ID         string  `json:"id"`
	Band       string  `json:"band"`
	Date       string  `json:"date"`
	Venue      string  `json:"venue"`
	City       string  `json:"city"`
	Tour       *string `json:"tour"`
	SetlistURL string  `json:"setlistUrl"`
}

type previewSummary struct {
	Found     int `json:"found"`
	Matchable int `json:"matchable"`
}

type previewSetJSON struct {
	Heading string             `json:"heading"`
	Entries []previewEntryJSON `json:"entries"`
}

type previewEntryJSON struct {
	Position       int               `json:"position"`
	Name           string            `json:"name"`
	Note           *string           `json:"note"`
	Tape           bool              `json:"tape"`
	Status         pipeline.Status   `json:"status"`
	Live           bool              `json:"live"`
	OriginalArtist *string           `json:"originalArtist"`
	Track          *previewTrackJSON `json:"track"`
}

type previewTrackJSON struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Artists []string `json:"artists"`
	Album   string   `json:"album"`
}

// previewHandler serves GET /api/shows/{id}/preview.
type previewHandler struct {
	logger    *slog.Logger
	setlistFM *setlistfm.Client
	spotify   *spotify.Client
	budget    time.Duration // previewBudget, shorter in tests
}

// serve fetches the setlist, classifies it, matches every song on Spotify
// with the user's token and returns the preview (PREV-2 to PREV-4, PREV-7,
// ADR-005).
func (h *previewHandler) serve(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !setlistIDPattern.MatchString(id) {
		jsonapi.WriteError(w, jsonapi.NotFound())
		return
	}
	s, _ := auth.SessionFromContext(r.Context()) // always present behind RequireSession

	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()

	sl, err := h.setlistFM.Setlist(ctx, id)
	switch {
	case errors.Is(err, setlistfm.ErrNotFound):
		jsonapi.WriteError(w, jsonapi.NotFound())
		return
	case err != nil:
		h.fail(w, r, ctx, jsonapi.ServiceSetlistFM, err)
		return
	}
	show := sl.Show()

	results, err := pipeline.MatchShow(ctx, show, h.spotify.WithToken(s.AccessToken), spotify.MarketFromToken)
	switch {
	case err == nil:
	case errors.Is(err, spotify.ErrUnauthorized):
		h.logger.Info("Spotify rejected the session; logging out", "op", "preview")
		auth.SpotifyUnauthorized(w)
		return
	default:
		h.fail(w, r, ctx, jsonapi.ServiceSpotify, err)
		return
	}

	p, err := buildPreview(show, results)
	if err != nil {
		h.logger.Error("building the preview failed", "op", "preview", "err", err.Error())
		jsonapi.WriteError(w, jsonapi.InternalError())
		return
	}
	jsonapi.WriteJSON(w, http.StatusOK, p)
}

// fail writes ERR-1's 503 for service; see failUpstream.
func (h *previewHandler) fail(w http.ResponseWriter, r *http.Request, ctx context.Context, service string, err error) {
	failUpstream(h.logger, w, r, ctx, "preview", h.budget, service, err)
}

// failUpstream writes ERR-1's 503 for service, or, if the browser has gone
// away, only records 499 so the request isn't logged as an outage (the
// error is still logged at INFO, in case a real failure came first). ctx is
// the request's budget context: only its own deadline counts as the budget
// running out, not a client's per-call timeout. op names the endpoint in
// the logs. Error text from both clients holds no IDs, names, tokens or
// bodies, so it's safe to log.
func failUpstream(logger *slog.Logger, w http.ResponseWriter, r *http.Request, ctx context.Context, op string, budget time.Duration, service string, err error) {
	if r.Context().Err() != nil {
		logger.Info(op+" abandoned by the client", "service", service, "err", err.Error())
		w.WriteHeader(statusClientClosed)
		return
	}
	err = budgetUsedUp(ctx, op, budget, err)
	logger.Warn(service+" call failed", "op", op, "err", err.Error())
	jsonapi.WriteError(w, jsonapi.UpstreamUnavailable(service))
}

// budgetUsedUp says in err that op's budget ran out, if ctx (the budget's
// context) ended by its own deadline. A client's per-call timeout isn't
// reported as one.
func budgetUsedUp(ctx context.Context, op string, budget time.Duration, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s budget of %v used up: %w", op, budget, err)
	}
	return err
}

// buildPreview turns the show and its match results (in setlist order,
// skipped entries already dropped) into the response.
//
// Sets follow show.Sets: each set takes as many result rows as it has
// non-skipped entries, so a set with nothing to show is left out (no
// heading with nothing under it), and two adjacent sets that happen to
// share a heading stay apart. If the rows don't line up with the show's
// non-skipped entries (a bug), it returns an error rather than put rows
// under the wrong heading.
func buildPreview(show setlist.Show, results []pipeline.EntryResult) (previewJSON, error) {
	p := previewJSON{
		Show: previewShowJSON{
			ID:         show.ID,
			Band:       show.Band,
			Date:       show.Date,
			Venue:      show.Venue,
			City:       show.City,
			Tour:       optional(show.Tour),
			SetlistURL: show.URL,
		},
		Summary:  previewSummary{Matchable: show.Matchable()},
		Sets:     []previewSetJSON{},
		TrackIDs: []string{},
	}

	next := 0
	for _, set := range show.Sets {
		n := 0
		for _, e := range set.Entries {
			if e.Kind != setlist.KindSkipped {
				n++
			}
		}
		if n == 0 {
			continue
		}
		if next+n > len(results) {
			return previewJSON{}, fmt.Errorf("%d result rows for at least %d entries", len(results), next+n)
		}
		out := previewSetJSON{Heading: set.Heading, Entries: make([]previewEntryJSON, 0, n)}
		for _, res := range results[next : next+n] {
			e := toPreviewEntry(res)
			out.Entries = append(out.Entries, e)
			if e.Track != nil {
				p.Summary.Found++
				p.TrackIDs = append(p.TrackIDs, e.Track.ID)
			}
		}
		next += n
		p.Sets = append(p.Sets, out)
	}
	if next != len(results) {
		return previewJSON{}, fmt.Errorf("%d result rows for %d entries", len(results), next)
	}
	p.CanCreate = p.Summary.Found > 0 // PREV-7
	return p, nil
}

func toPreviewEntry(r pipeline.EntryResult) previewEntryJSON {
	e := previewEntryJSON{
		Position:       r.Position,
		Name:           r.Name,
		Note:           optional(r.Note),
		Tape:           r.IsTape,
		Status:         r.Status,
		Live:           r.Live,
		OriginalArtist: optional(r.OriginalArtist),
	}
	if r.Track != nil && r.Status.Found() {
		artists := r.Track.Artists
		if artists == nil {
			artists = []string{}
		}
		e.Track = &previewTrackJSON{ID: r.Track.ID, Title: r.Track.Title, Artists: artists, Album: r.Track.Album}
	}
	return e
}

// optional is nil for a blank string, so absent values are JSON null.
func optional(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
