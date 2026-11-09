// Package spotify is a client for the Spotify Web API calls Setlisted
// makes: field-filtered track search for the matching pipeline, and
// creating a private playlist and adding tracks to it (ADR-005). It uses
// the standard library only (ADR-001).
//
// A rate-limit response (429) is retried after Retry-After, a bounded
// number of times. A 401 is ErrUnauthorized, so handlers can apply step 04's
// helper (auth.SpotifyUnauthorized). Any other failure wraps ErrUnavailable.
// Error text never contains the token, the request URL or the response
// body, so it can be logged.
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hmarp/setlisted/internal/matching"
)

// Defaults.
const (
	DefaultBaseURL = "https://api.spotify.com/v1"

	// MarketFromToken asks Spotify to use the user's own country, so
	// candidates are playable for them. It only works with a user token; an
	// app (client-credentials) token needs an explicit country code such as
	// "GB".
	MarketFromToken = "from_token"

	// SearchLimit is the number of results asked for: the development-mode
	// maximum since February 2026 (ADR-005).
	SearchLimit = 10

	// DefaultRetries is how many times a 429 is retried before giving up.
	DefaultRetries = 3

	// DefaultBackoff is the wait before the first retry when a 429 has no
	// usable Retry-After. It doubles for each later retry, up to the
	// MaxRetryAfter limit. A Retry-After header, if given, is used as is.
	DefaultBackoff = time.Second

	// DefaultMaxRetryAfter is the longest Retry-After the client waits for.
	// A longer one means give up at once: the user would rather retry
	// (ERR-1) than watch "Matching songs…" for minutes.
	DefaultMaxRetryAfter = 10 * time.Second

	// requestTimeout bounds each HTTP call, including reading the body.
	requestTimeout = 10 * time.Second

	// maxBody caps the response size read. Ten tracks are well under 100 KB.
	maxBody = 4 << 20
)

// ErrUnauthorized means Spotify answered 401: the token is expired or
// invalid. Handlers call auth.SpotifyUnauthorized (AUTH-8).
var ErrUnauthorized = errors.New("spotify: unauthorised")

// ErrUnavailable means Spotify couldn't be reached, answered with an error
// other than 401, kept rate-limiting after the retries, or sent a response
// that couldn't be decoded. The UI shows ERR-1's message.
var ErrUnavailable = errors.New("spotify unavailable")

// Config configures a Client. Every field is optional.
type Config struct {
	// BaseURL replaces DefaultBaseURL (tests).
	BaseURL string
	// HTTPClient replaces the default client, which has a timeout and never
	// follows redirects.
	HTTPClient *http.Client
	// Retries is how many times a 429 is retried; 0 means DefaultRetries and
	// a negative value means no retries.
	Retries int
	// Backoff and MaxRetryAfter replace DefaultBackoff and
	// DefaultMaxRetryAfter when non-zero.
	Backoff       time.Duration
	MaxRetryAfter time.Duration
}

// Client calls the Spotify Web API. It's safe for concurrent use. It holds
// no token: each call is given one.
type Client struct {
	baseURL       string
	http          *http.Client
	retries       int
	backoff       time.Duration
	maxRetryAfter time.Duration
	sleep         func(context.Context, time.Duration) error // replaced in tests
}

// New returns a Client.
func New(cfg Config) *Client {
	c := &Client{
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		http:          cfg.HTTPClient,
		retries:       cfg.Retries,
		backoff:       cfg.Backoff,
		maxRetryAfter: cfg.MaxRetryAfter,
		sleep:         sleep,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.http == nil {
		c.http = &http.Client{
			Timeout: requestTimeout,
			// The API doesn't redirect for these calls. Never follow one, so the
			// bearer token is only ever sent to the configured host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	switch {
	case c.retries == 0:
		c.retries = DefaultRetries
	case c.retries < 0:
		c.retries = 0
	}
	if c.backoff == 0 {
		c.backoff = DefaultBackoff
	}
	if c.maxRetryAfter == 0 {
		c.maxRetryAfter = DefaultMaxRetryAfter
	}
	return c
}

// SearchTracks runs one field-filtered search,
// q=track:"<track>" artist:"<artist>", type=track, limit=10, in market, and
// returns the results as matching candidates in Spotify's order. No results
// is (nil, nil).
//
// market is MarketFromToken for a user token, or an ISO 3166-1 alpha-2
// country code (e.g. "GB") for an app token. It's required.
//
// Double quotes inside track and artist are replaced by spaces (see
// Query); a name that is empty after that is not searched and gives no
// results.
func (c *Client) SearchTracks(ctx context.Context, token, track, artist, market string) ([]matching.Candidate, error) {
	if market == "" {
		return nil, errors.New("spotify: market is required")
	}
	q, ok := Query(track, artist)
	if !ok {
		return nil, nil
	}
	v := url.Values{
		"q":      {q},
		"type":   {"track"},
		"limit":  {strconv.Itoa(SearchLimit)},
		"market": {market},
	}
	var resp searchResponse
	// The names as searched (after cleaning) must not be logged.
	if err := c.get(ctx, token, "/search?"+v.Encode(), &resp, clean(track), clean(artist)); err != nil {
		return nil, err
	}
	var out []matching.Candidate
	for _, t := range resp.Tracks.Items {
		if t == nil || t.ID == "" {
			// Spotify sometimes returns null items in search results.
			continue
		}
		out = append(out, t.candidate())
	}
	return out, nil
}

// WithToken returns a searcher bound to one token, with the method set the
// matching pipeline's Searcher interface expects.
func (c *Client) WithToken(token string) TokenSearcher {
	return TokenSearcher{c: c, token: token}
}

// TokenSearcher is a Client bound to one access token.
type TokenSearcher struct {
	c     *Client
	token string
}

// SearchTracks is Client.SearchTracks with the bound token.
func (s TokenSearcher) SearchTracks(ctx context.Context, track, artist, market string) ([]matching.Candidate, error) {
	return s.c.SearchTracks(ctx, s.token, track, artist, market)
}

// Query builds the search query track:"<track>" artist:"<artist>".
//
// Spotify's search syntax has no escape for a double quote inside a quoted
// field value, so each " in a name is replaced by a space, then runs of
// whitespace are collapsed and the ends trimmed. (Spotify's search ignores
// punctuation and works on words, so the space keeps the words apart without
// changing the result.) Other quote characters (', ’, “, ”) are left as they
// are. The matcher compares against the original entry name, not this
// query, so the change can't make a wrong title match.
//
// ok is false if either name is empty after cleaning; there's nothing to
// search for.
func Query(track, artist string) (q string, ok bool) {
	t, a := clean(track), clean(artist)
	if t == "" || a == "" {
		return "", false
	}
	return `track:"` + t + `" artist:"` + a + `"`, true
}

func clean(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, `"`, " ")), " ")
}

type searchResponse struct {
	Tracks struct {
		Items []*trackJSON `json:"items"`
	} `json:"tracks"`
}

type trackJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name                 string `json:"name"`
		AlbumType            string `json:"album_type"`
		ReleaseDate          string `json:"release_date"`
		ReleaseDatePrecision string `json:"release_date_precision"`
	} `json:"album"`
}

func (t *trackJSON) candidate() matching.Candidate {
	artists := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		artists = append(artists, a.Name)
	}
	return matching.Candidate{
		ID:                   t.ID,
		Title:                t.Name,
		Artists:              artists,
		Album:                t.Album.Name,
		AlbumType:            t.Album.AlbumType,
		ReleaseDate:          t.Album.ReleaseDate,
		ReleaseDatePrecision: t.Album.ReleaseDatePrecision,
	}
}

// get calls path (relative to the base URL, with any query) with the bearer
// token and decodes the JSON response into v. See call.
func (c *Client) get(ctx context.Context, token, path string, v any, sensitive ...string) error {
	return c.call(ctx, token, http.MethodGet, path, nil, v, sensitive...)
}

// post sends body as JSON to path and decodes the JSON response into v
// (nil to discard it). See call.
func (c *Client) post(ctx context.Context, token, path string, body, v any, sensitive ...string) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: encoding request", ErrUnavailable)
	}
	return c.call(ctx, token, http.MethodPost, path, b, v, sensitive...)
}

// call makes a request with the bearer token, retrying a 429 up to
// c.retries times. A 429 means Spotify didn't act on the request, so
// retrying a POST is safe. sensitive are strings that must never reach
// error text (names sent in the request); the token always is one.
func (c *Client) call(ctx context.Context, token, method, path string, body []byte, v any, sensitive ...string) error {
	for attempt := 0; ; attempt++ {
		retryAfter, err := c.do(ctx, token, method, path, body, v, sensitive)
		if !errors.Is(err, errRateLimited) {
			return err
		}
		if attempt >= c.retries {
			return fmt.Errorf("%w: still rate-limited after %d retries", ErrUnavailable, c.retries)
		}
		if retryAfter > c.maxRetryAfter {
			// Retrying sooner would only be refused again.
			return fmt.Errorf("%w: rate-limited, Retry-After %v", ErrUnavailable, retryAfter)
		}
		wait := retryAfter
		if wait <= 0 {
			wait = backoff(c.backoff, c.maxRetryAfter, attempt)
		}
		if err := c.sleep(ctx, wait); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
}

var errRateLimited = errors.New("rate limited")

// do makes one call. On a 429 it returns errRateLimited and the Retry-After
// delay, if the header gave one in seconds. Any 2xx is success: search
// answers 200, creating a playlist and adding items answer 201.
func (c *Client) do(ctx context.Context, token, method, path string, body []byte, v any, sensitive []string) (time.Duration, error) {
	var rb io.Reader
	if body != nil {
		rb = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rb)
	if err != nil {
		return 0, fmt.Errorf("%w: building request", ErrUnavailable)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error's text includes the URL, which holds the search text.
		// Keep only the underlying cause (which may be the context's error,
		// so errors.Is(err, context.Canceled) works).
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("%w: request failed: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return retryAfter(resp.Header.Get("Retry-After")), errRateLimited
	case resp.StatusCode == http.StatusUnauthorized:
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return 0, ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		reason := spotifyReason(resp.Body, token, sensitive)
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		if reason != "" {
			return 0, fmt.Errorf("%w: status %d, Spotify says %q", ErrUnavailable, resp.StatusCode, reason)
		}
		return 0, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	if v != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(v); err != nil {
			return 0, fmt.Errorf("%w: decoding response", ErrUnavailable)
		}
	}
	// Read to EOF so the connection can be reused for the next search.
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	return 0, nil
}

// maxReason caps the Spotify error message kept in error text.
const maxReason = 120

// spotifyReason returns the message of Spotify's error body,
// {"error": {"status": 403, "message": "…"}}, so a refusal can be diagnosed
// from the logs. It's kept only if it's short, plain text and contains
// neither the token nor any of sensitive (the names sent in the request);
// otherwise it's "".
// It goes into error text for logs only: handlers never show it to users
// (ERR-5).
func spotifyReason(body io.Reader, token string, sensitive []string) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(body, 16<<10)).Decode(&e) != nil {
		return ""
	}
	msg := strings.TrimSpace(e.Error.Message)
	if msg == "" || utf8.RuneCountInString(msg) > maxReason {
		return ""
	}
	for _, r := range msg {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	lower := strings.ToLower(msg)
	for _, s := range append([]string{token}, sensitive...) {
		if s != "" && strings.Contains(lower, strings.ToLower(s)) {
			return ""
		}
	}
	return msg
}

// backoff is base doubled attempt times, never more than limit (and so
// never overflowing, however many retries are configured).
func backoff(base, limit time.Duration, attempt int) time.Duration {
	d := base
	for range attempt {
		if d >= limit {
			break
		}
		d *= 2
	}
	return min(d, limit)
}

// retryAfter parses a Retry-After header given in seconds. Anything else
// (absent, an HTTP date, garbage) is 0, meaning use the normal backoff.
func retryAfter(h string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n < 0 {
		return 0
	}
	const day = 24 * 60 * 60
	if n > day {
		n = day // avoids overflow; still far past any cap
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
