package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hmarp/setlisted/internal/auth"
	"github.com/hmarp/setlisted/internal/jsonapi"
	"github.com/hmarp/setlisted/internal/setlist"
	"github.com/hmarp/setlisted/internal/setlistfm"
	"github.com/hmarp/setlisted/internal/spotify"
)

// createBudget bounds all of one create request's upstream calls together:
// the setlist.fm fetch, creating the playlist and adding its tracks. Like
// previewBudget, it leaves 10 s of the 60 s write timeout.
const createBudget = 50 * time.Second

// Request limits for POST /api/playlists.
const (
	// maxTracks is far above any real setlist (a long show has about 30
	// songs) and keeps a request to a few add-items batches.
	maxTracks = 200
	// maxCreateBody caps the request body: 200 IDs of 22 characters plus
	// JSON punctuation is under 6 KB.
	maxCreateBody = 32 << 10
)

// Playlist text limits, in characters. Spotify's Web API reference doesn't
// state a name limit; these are the lengths Spotify's own apps allow (see
// docs/plan/11-create-playlist-api/handoff.md). A longer name or
// description is cut, never refused.
const (
	maxNameLength        = 100
	maxDescriptionLength = 300
)

type createPlaylistRequestJSON struct {
	ShowID   string   `json:"showId"`
	TrackIDs []string `json:"trackIds"`
}

// createdPlaylistJSON is the playlist in the 201 response and, without a
// track count, in the 502 partial_create response.
type createdPlaylistJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TrackCount *int   `json:"trackCount,omitempty"`
	URL        string `json:"url"`
}

type createPlaylistResponseJSON struct {
	Playlist createdPlaylistJSON `json:"playlist"`
}

// partialCreateJSON is the 502 partial_create body: the standard error plus
// the playlist that exists, so the UI can link to it (ERR-3).
type partialCreateJSON struct {
	Error    *jsonapi.Error      `json:"error"`
	Playlist createdPlaylistJSON `json:"playlist"`
}

// playlistHandler serves POST /api/playlists.
type playlistHandler struct {
	logger    *slog.Logger
	setlistFM *setlistfm.Client
	spotify   *spotify.Client
	budget    time.Duration // createBudget, shorter in tests
}

// create validates the request, fetches the show again for the name and
// description (PL-2, PL-3), creates a new private playlist (PL-1, PL-4) and
// adds the tracks in order, duplicates kept (PL-5). The track list is the
// preview's, taken as it is (ADR-005: create doesn't re-run matching).
//
// Once the Spotify calls start, the request no longer stops when the
// browser goes away: the playlist is created and every batch added (within
// the budget), so a closed tab leaves a complete playlist rather than an
// empty or half-filled one. Before that (validation, setlist.fm), a closed
// tab stops it with nothing created. Nothing is ever rolled back (ERR-3).
func (h *playlistHandler) create(w http.ResponseWriter, r *http.Request) {
	req, msg := decodeCreateRequest(w, r)
	if msg != "" {
		jsonapi.WriteError(w, jsonapi.InvalidRequest(msg))
		return
	}
	s, _ := auth.SessionFromContext(r.Context()) // always present behind RequireSession

	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()

	sl, err := h.setlistFM.Setlist(ctx, req.ShowID)
	switch {
	case errors.Is(err, setlistfm.ErrNotFound):
		jsonapi.WriteError(w, jsonapi.NotFound())
		return
	case err != nil:
		failUpstream(h.logger, w, r, ctx, "create_playlist", h.budget, jsonapi.ServiceSetlistFM, err)
		return
	}
	header := sl.Show().Header
	name, description := playlistName(header), playlistDescription(header)

	// From here on, the browser going away doesn't stop the request (see
	// above): a create call cut off mid-way could leave a playlist on
	// Spotify that nobody fills. The budget's deadline still applies. If
	// the browser has already gone, stop now: nothing exists yet.
	if r.Context().Err() != nil {
		failUpstream(h.logger, w, r, ctx, "create_playlist", h.budget, jsonapi.ServiceSpotify, r.Context().Err())
		return
	}
	deadline, _ := ctx.Deadline()
	spotifyCtx, cancelSpotify := context.WithDeadline(context.WithoutCancel(r.Context()), deadline)
	defer cancelSpotify()

	pl, err := h.spotify.CreatePlaylist(spotifyCtx, s.AccessToken, name, description)
	switch {
	case err == nil:
	case errors.Is(err, spotify.ErrUnauthorized):
		h.logger.Info("Spotify rejected the session; logging out", "op", "create_playlist")
		auth.SpotifyUnauthorized(w)
		return
	default:
		// ERR-4: nothing was created (as far as we can tell), so Retry.
		// The browser going away didn't cause this (see above), so it's a
		// real failure even if the tab has closed meanwhile.
		if errors.Is(spotifyCtx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("create_playlist budget of %v used up: %w", h.budget, err)
		}
		h.logger.Warn("Spotify call failed", "op", "create_playlist", "client_gone", r.Context().Err() != nil, "err", err.Error())
		jsonapi.WriteError(w, jsonapi.UpstreamUnavailable(jsonapi.ServiceSpotify))
		return
	}

	added, err := h.spotify.AddTracks(spotifyCtx, s.AccessToken, pl.ID, req.TrackIDs)
	if err != nil {
		h.partial(w, r, spotifyCtx, pl, name, added, len(req.TrackIDs), err)
		return
	}
	if r.Context().Err() != nil {
		h.logger.Info("create_playlist finished after the client went away", "tracks", added)
	}
	n := len(req.TrackIDs)
	jsonapi.WriteJSON(w, http.StatusCreated, createPlaylistResponseJSON{Playlist: createdPlaylistJSON{
		ID: pl.ID, Name: name, TrackCount: &n, URL: pl.URL,
	}})
}

// partial writes ERR-3's 502 partial_create with the playlist's link: it
// exists, but adding tracks failed. Nothing is deleted. A 401 here also
// ends the session, so the next request goes to the login screen (AUTH-8),
// but this response still reports the playlist.
func (h *playlistHandler) partial(w http.ResponseWriter, r *http.Request, ctx context.Context, pl spotify.Playlist, name string, added, total int, err error) {
	if errors.Is(err, spotify.ErrUnauthorized) {
		auth.EndSession(w)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("create_playlist budget of %v used up: %w", h.budget, err)
	}
	h.logger.Warn("Spotify call failed after the playlist was created", "op", "create_playlist",
		"added", added, "tracks", total, "client_gone", r.Context().Err() != nil, "err", err.Error())
	jsonapi.WriteJSON(w, jsonapi.PartialCreate().Status, partialCreateJSON{
		Error:    jsonapi.PartialCreate(),
		Playlist: createdPlaylistJSON{ID: pl.ID, Name: name, URL: pl.URL},
	})
}

// decodeCreateRequest reads and validates the body. msg is the
// user-facing reason when it's invalid.
func decodeCreateRequest(w http.ResponseWriter, r *http.Request) (req createPlaylistRequestJSON, msg string) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateBody))
	if err := dec.Decode(&req); err != nil {
		return req, "The request isn't valid."
	}
	if dec.More() {
		return req, "The request isn't valid."
	}
	switch {
	case !setlistIDPattern.MatchString(req.ShowID):
		// The same rule as the preview's {id}; here it's a body field, so
		// a malformed one is a bad request rather than an unknown show.
		return req, "The request isn't valid."
	case len(req.TrackIDs) == 0:
		// PREV-7: an empty playlist is never created.
		return req, "None of these songs were found on Spotify."
	case len(req.TrackIDs) > maxTracks:
		return req, "That's too many songs for one playlist."
	}
	for _, id := range req.TrackIDs {
		if !spotify.ValidTrackID(id) {
			return req, "The request isn't valid."
		}
	}
	return req, ""
}

// playlistName is PL-2's name, cut to maxNameLength:
//
//	‹Band› – ‹Tour Name› (Setlist)   with a tour
//	‹Band› – ‹City›, ‹DD Mon YYYY›   without
//
// The dash is an en dash. h.Date is already DD Mon YYYY (step 05).
func playlistName(h setlist.Header) string {
	var name string
	if tour := strings.TrimSpace(h.Tour); tour != "" {
		name = h.Band + " – " + tour + " (Setlist)"
	} else {
		name = h.Band + " – " + joinNonEmpty(", ", h.City, h.Date)
	}
	return truncate(name, maxNameLength)
}

// playlistDescription is PL-3's description, cut to maxDescriptionLength:
//
//	‹Venue›, ‹City›, ‹DD Mon YYYY›. Setlist: ‹setlist.fm URL›. Created by Setlisted.
func playlistDescription(h setlist.Header) string {
	d := joinNonEmpty(", ", h.Venue, h.City, h.Date) + ". Setlist: " + h.URL + ". Created by Setlisted."
	return truncate(d, maxDescriptionLength)
}

// joinNonEmpty joins the parts that aren't blank, so a missing city or
// venue doesn't leave ", ," in the text.
func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// truncate cuts s to at most n characters (runes, so never mid-character),
// ending in "…" when it was cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}
