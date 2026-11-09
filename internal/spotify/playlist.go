package spotify

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
)

// MaxItemsPerRequest is the most track URIs one "add items to playlist"
// request may carry (Spotify's documented maximum, historically 100; see
// docs/plan/11-create-playlist-api/handoff.md). AddTracks sends larger lists
// in batches of this size.
const MaxItemsPerRequest = 100

// trackIDPattern is a Spotify track ID: 22 base62 characters.
var trackIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{22}$`)

// ValidTrackID reports whether id looks like a Spotify track ID (22 base62
// characters). It says nothing about whether the track exists.
func ValidTrackID(id string) bool { return trackIDPattern.MatchString(id) }

// Playlist is a playlist Spotify has just created.
type Playlist struct {
	ID string
	// URL is Spotify's external_urls.spotify: the playlist's
	// open.spotify.com page, which opens the Spotify app on a phone that has
	// it (PL-8).
	URL string
}

type createPlaylistRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Public      bool   `json:"public"`
}

type playlistJSON struct {
	ID           string `json:"id"`
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

// CreatePlaylist creates a new, private playlist in the token's user's
// library: POST /me/playlists with name, description and public: false
// (PL-1, PL-4). It never reuses an existing playlist.
//
// sensitive are extra strings that must never reach error text, such as
// the parts the name and description were built from (band, venue, …);
// the name and description themselves always are.
//
// A response without an ID or a link wraps ErrUnavailable, like any other
// failure. Note that a network failure or timeout can't tell whether
// Spotify created the playlist before the connection was lost.
func (c *Client) CreatePlaylist(ctx context.Context, token, name, description string, sensitive ...string) (Playlist, error) {
	var resp playlistJSON
	req := createPlaylistRequest{Name: name, Description: description, Public: false}
	sensitive = append([]string{name, description}, sensitive...)
	if err := c.post(ctx, token, "/me/playlists", req, &resp, sensitive...); err != nil {
		return Playlist{}, err
	}
	if resp.ID == "" || resp.ExternalURLs.Spotify == "" {
		return Playlist{}, fmt.Errorf("%w: created playlist has no ID or link", ErrUnavailable)
	}
	return Playlist{ID: resp.ID, URL: resp.ExternalURLs.Spotify}, nil
}

type addItemsRequest struct {
	URIs []string `json:"uris"`
}

// AddTracks appends the tracks to the playlist, in order and duplicates
// included, with POST /playlists/{id}/items in batches of at most
// MaxItemsPerRequest, one after the other so the order is kept. trackIDs
// are bare track IDs; they're sent as spotify:track:<id> URIs.
//
// added is the number of tracks in the batches Spotify accepted before an
// error (all of them on success). The first failed batch stops it; nothing
// is retried except a 429, and nothing is undone. Spotify's reason for a
// failure is kept in the error only if it quotes neither the playlist ID
// nor any of the batch's track IDs.
func (c *Client) AddTracks(ctx context.Context, token, playlistID string, trackIDs []string) (added int, err error) {
	path := "/playlists/" + url.PathEscape(playlistID) + "/items"
	for start := 0; start < len(trackIDs); start += MaxItemsPerRequest {
		end := min(start+MaxItemsPerRequest, len(trackIDs))
		batch := trackIDs[start:end]
		uris := make([]string, 0, len(batch))
		for _, id := range batch {
			uris = append(uris, "spotify:track:"+id)
		}
		// The bare IDs cover the URIs too.
		sensitive := append([]string{playlistID}, batch...)
		if err := c.post(ctx, token, path, addItemsRequest{URIs: uris}, nil, sensitive...); err != nil {
			return added, err
		}
		added = end
	}
	return added, nil
}
