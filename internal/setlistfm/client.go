// Package setlistfm is a client for the parts of the setlist.fm API the app
// uses: artist search, an artist's setlists and a single setlist (ADR-005).
//
// Every call goes through one in-process rate limiter. A rate-limit response
// (429) is retried quietly a few times. Any other failure is returned as an
// error wrapping ErrUnavailable, except a 404 for a single artist or
// setlist, which is ErrNotFound. Error text never contains the API key, the
// request URL or the response body, so it can be logged.
package setlistfm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Defaults.
const (
	DefaultBaseURL = "https://api.setlist.fm/rest/1.0"

	// DefaultInterval spaces calls just below a standard key's limit of
	// 2 requests per second (see the step 05 handoff).
	DefaultInterval = 600 * time.Millisecond

	// DefaultRetries is how many times a 429 is retried before giving up.
	DefaultRetries = 2

	// DefaultBackoff is the wait before the first retry. It doubles for each
	// later retry. A Retry-After header, if longer, wins, up to maxBackoff.
	DefaultBackoff = time.Second

	// requestTimeout bounds each HTTP call, including reading the body.
	requestTimeout = 10 * time.Second

	maxBackoff = 5 * time.Second

	// maxBody caps the response size read. A page of 20 setlists is well
	// under 100 KB.
	maxBody = 4 << 20
)

// ErrUnavailable means setlist.fm couldn't be reached, answered with an
// error, kept rate-limiting after the retries, or sent a response that
// couldn't be decoded. The UI shows ERR-1's message.
var ErrUnavailable = errors.New("setlist.fm unavailable")

// ErrNotFound means setlist.fm has no such artist or setlist.
var ErrNotFound = errors.New("setlist.fm: not found")

// Config configures a Client. Only APIKey is required.
type Config struct {
	APIKey string

	// Overridable for tests. Zero values use the defaults above.
	BaseURL    string
	HTTPClient *http.Client
	Interval   time.Duration
	Retries    int // a negative value means no retries
	Backoff    time.Duration
}

// Client calls the setlist.fm API. It's safe for concurrent use; calls are
// spaced by a shared rate limiter.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
	limiter *limiter
	retries int
	backoff time.Duration
}

// New returns a Client.
func New(cfg Config) *Client {
	c := &Client{
		apiKey:  cfg.APIKey,
		baseURL: cfg.BaseURL,
		http:    cfg.HTTPClient,
		retries: cfg.Retries,
		backoff: cfg.Backoff,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: requestTimeout}
	}
	interval := cfg.Interval
	if interval == 0 {
		interval = DefaultInterval
	}
	c.limiter = newLimiter(interval)
	switch {
	case c.retries == 0:
		c.retries = DefaultRetries
	case c.retries < 0:
		c.retries = 0
	}
	if c.backoff == 0 {
		c.backoff = DefaultBackoff
	}
	return c
}

// SearchArtists returns setlist.fm's first page of artists for name, in
// relevance order (up to 30). No results is an empty slice, not an error:
// setlist.fm answers 404 for that.
func (c *Client) SearchArtists(ctx context.Context, name string) ([]Artist, error) {
	q := url.Values{"artistName": {name}, "sort": {"relevance"}, "p": {"1"}}
	var page artistsPage
	if err := c.get(ctx, "/search/artists?"+q.Encode(), &page); err != nil {
		if errors.Is(err, ErrNotFound) {
			return []Artist{}, nil
		}
		return nil, err
	}
	if page.Artists == nil {
		page.Artists = []Artist{}
	}
	return page.Artists, nil
}

// Artist returns the artist with the given MusicBrainz ID, or ErrNotFound.
func (c *Client) Artist(ctx context.Context, mbid string) (Artist, error) {
	var a Artist
	err := c.get(ctx, "/artist/"+url.PathEscape(mbid), &a)
	return a, err
}

// ArtistSetlists returns one page (1-based) of the artist's setlists, newest
// first, 20 per page. A page past the end, or an artist with no setlists, is
// an empty page: setlist.fm answers 404 for both.
func (c *Client) ArtistSetlists(ctx context.Context, mbid string, page int) (SetlistsPage, error) {
	var p SetlistsPage
	path := "/artist/" + url.PathEscape(mbid) + "/setlists?p=" + strconv.Itoa(page)
	if err := c.get(ctx, path, &p); err != nil {
		if errors.Is(err, ErrNotFound) {
			return SetlistsPage{Page: page}, nil
		}
		return SetlistsPage{}, err
	}
	return p, nil
}

// Setlist returns one setlist by its setlist.fm ID, or ErrNotFound.
func (c *Client) Setlist(ctx context.Context, id string) (Setlist, error) {
	var s Setlist
	err := c.get(ctx, "/setlist/"+url.PathEscape(id), &s)
	return s, err
}

// get calls path (relative to the base URL, with any query) and decodes the
// JSON response into v, retrying a 429 up to c.retries times.
func (c *Client) get(ctx context.Context, path string, v any) error {
	for attempt := 0; ; attempt++ {
		retryAfter, err := c.do(ctx, path, v)
		if !errors.Is(err, errRateLimited) {
			return err
		}
		if attempt >= c.retries {
			return fmt.Errorf("%w: still rate-limited after %d retries", ErrUnavailable, c.retries)
		}
		wait := c.backoff << attempt
		if retryAfter > wait {
			wait = retryAfter
		}
		wait = min(wait, maxBackoff)
		if err := sleep(ctx, wait); err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
}

var errRateLimited = errors.New("rate limited")

// do makes one call. On a 429 it returns errRateLimited and the Retry-After
// delay, if the header gave one in seconds.
func (c *Client) do(ctx context.Context, path string, v any) (time.Duration, error) {
	if err := c.limiter.wait(ctx); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: building request", ErrUnavailable)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error's text includes the URL, which can hold the search
		// text. Keep only the underlying cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("%w: request failed: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return retryAfter(resp.Header.Get("Retry-After")), errRateLimited
	case resp.StatusCode == http.StatusNotFound:
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return 0, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return 0, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(v); err != nil {
		return 0, fmt.Errorf("%w: decoding response", ErrUnavailable)
	}
	return 0, nil
}

// retryAfter parses a Retry-After header given in seconds. Anything else
// (absent, an HTTP date, garbage) is 0, meaning use the normal backoff.
func retryAfter(h string) time.Duration {
	n, err := strconv.Atoi(h)
	if err != nil || n < 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
