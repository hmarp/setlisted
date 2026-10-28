// Package auth implements Spotify login (Authorization Code flow with PKCE)
// and the stateless, encrypted-cookie session (ADR-004, ADR-009).
//
// Nothing here may log tokens, authorisation codes, cookie values or the
// Spotify user ID.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/hmarp/setlisted/internal/config"
	"github.com/hmarp/setlisted/internal/jsonapi"
)

// Spotify endpoints used by default.
const (
	SpotifyAuthURL    = "https://accounts.spotify.com/authorize"
	SpotifyTokenURL   = "https://accounts.spotify.com/api/token"
	SpotifyAPIBaseURL = "https://api.spotify.com/v1"
)

// Scope is the only Spotify permission requested (AUTH-2): creating a
// private playlist. Reading the user's ID and searching need no scope.
const Scope = "playlist-modify-private"

// CallbackPath is the redirect URI's path, registered in the Spotify app.
const CallbackPath = "/auth/callback"

// loginCookieMaxAge bounds the round trip to Spotify's login screen.
const loginCookieMaxAge = 10 * 60 // seconds

// spotifyTimeout bounds each of the callback's two calls to Spotify (the
// token exchange and GET /me) separately.
const spotifyTimeout = 10 * time.Second

// Values of the login query parameter on "/" after a failed login. The UI
// shows the matching message.
const (
	LoginCancelled  = "cancelled"   // AUTH-5
	LoginInviteOnly = "invite_only" // AUTH-4
	LoginFailed     = "failed"
)

// Config configures Auth. The Spotify URLs and HTTPClient default to the
// real Spotify and http.DefaultClient's transport; tests point them at a
// fake server.
type Config struct {
	ClientID     string
	ClientSecret string
	BaseURL      string // the app's base URL, e.g. https://example.run.app (see config.NormalizeBaseURL)
	SessionKey   []byte // 32 bytes (AES-256)
	Logger       *slog.Logger

	AuthURL    string
	TokenURL   string
	APIBaseURL string
	HTTPClient *http.Client
	Now        func() time.Time
}

// Auth serves the login endpoints and the session middleware.
type Auth struct {
	oauth  oauth2.Config
	sealer *sealer
	origin string // the normalised BaseURL, as browsers send it in Origin
	meURL  string
	client *http.Client
	logger *slog.Logger
	now    func() time.Time
}

// New validates cfg and returns an Auth.
func New(cfg Config) (*Auth, error) {
	base, err := config.NormalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("auth: BaseURL %w", err)
	}
	s, err := newSealer(cfg.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("auth: session key: %w", err)
	}
	a := &Auth{
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:   orDefault(cfg.AuthURL, SpotifyAuthURL),
				TokenURL:  orDefault(cfg.TokenURL, SpotifyTokenURL),
				AuthStyle: oauth2.AuthStyleInHeader,
			},
			RedirectURL: base + CallbackPath,
			Scopes:      []string{Scope},
		},
		sealer: s,
		origin: base,
		meURL:  orDefault(cfg.APIBaseURL, SpotifyAPIBaseURL) + "/me",
		client: cfg.HTTPClient,
		logger: cfg.Logger,
		now:    cfg.Now,
	}
	if a.client == nil {
		a.client = &http.Client{}
	}
	if a.logger == nil {
		a.logger = slog.New(slog.DiscardHandler)
	}
	if a.now == nil {
		a.now = time.Now
	}
	return a, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// loginState is the content of the login cookie.
type loginState struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
}

// Login handles GET /auth/login: it stores a fresh state and PKCE verifier
// in the login cookie and redirects to Spotify's authorise page.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	if refuseHead(w, r) {
		return
	}
	ls := loginState{State: rand.Text(), Verifier: oauth2.GenerateVerifier()}
	value, err := a.sealer.seal(loginCookie, ls)
	if err != nil {
		jsonapi.WriteError(w, jsonapi.InternalError())
		return
	}
	setCookie(w, loginCookie, "/auth", value, loginCookieMaxAge)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, a.oauth.AuthCodeURL(ls.State, oauth2.S256ChallengeOption(ls.Verifier)), http.StatusFound)
}

// Callback handles GET /auth/callback, where Spotify sends the user back.
// It always ends with a redirect to "/" (logged in) or "/?login=<reason>".
//
// Any outcome other than success also ends an existing session in this
// browser, so a refused login (say, a second person on a shared browser)
// never leaves the previous user logged in.
func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	if refuseHead(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	clearCookie(w, loginCookie, "/auth") // single use, whatever happens next

	fail := func(login, reason string, attrs ...slog.Attr) {
		clearSession(w)
		attrs = append(attrs, slog.String("reason", reason))
		a.logger.LogAttrs(r.Context(), slog.LevelWarn, "login failed", attrs...)
		http.Redirect(w, r, "/?login="+login, http.StatusFound)
	}

	q := r.URL.Query()
	// A cancel is reported even without a valid state (for example after the
	// login cookie expired): it creates no session, so there's nothing to
	// protect, and the user should see AUTH-5's message.
	if q.Get("error") == "access_denied" {
		clearSession(w)
		a.logger.LogAttrs(r.Context(), slog.LevelInfo, "login cancelled")
		http.Redirect(w, r, "/?login="+LoginCancelled, http.StatusFound)
		return
	}
	var ls loginState
	c, err := r.Cookie(loginCookie)
	if err != nil || a.sealer.open(loginCookie, c.Value, &ls) != nil {
		fail(LoginFailed, "missing or invalid login cookie")
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(ls.State)) != 1 {
		fail(LoginFailed, "state mismatch")
		return
	}
	if q.Get("error") != "" {
		fail(LoginFailed, "spotify returned an error")
		return
	}
	code := q.Get("code")
	if code == "" {
		fail(LoginFailed, "no code")
		return
	}

	// Read the clock before the exchange: Spotify's expires_in counts from
	// when it issues the token, so the session can't outlive it.
	now := a.now()
	exchangeCtx, cancel := context.WithTimeout(context.WithValue(r.Context(), oauth2.HTTPClient, a.client), spotifyTimeout)
	defer cancel()
	tok, err := a.oauth.Exchange(exchangeCtx, code, oauth2.VerifierOption(ls.Verifier))
	if err != nil {
		// Log only the category: the error text can include Spotify's body.
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.Response != nil {
			fail(LoginFailed, "token exchange refused", slog.Int("spotify_status", re.Response.StatusCode))
		} else {
			fail(LoginFailed, "token exchange failed")
		}
		return
	}
	// tok.RefreshToken is deliberately discarded here: it is never stored,
	// sent to the browser or logged (ADR-004).

	// The session ends when the token does, and at most an hour from now.
	// (Exchange has already refused a response without an access token.)
	// ExpiresIn is relative, so it works with the injected clock; oauth2 only
	// fills it for JSON responses, so fall back to the absolute Expiry.
	lifetime := time.Duration(tok.ExpiresIn) * time.Second
	if lifetime <= 0 && !tok.Expiry.IsZero() {
		if lifetime = time.Until(tok.Expiry); lifetime <= 0 {
			fail(LoginFailed, "token already expired")
			return
		}
	}
	expiry := now.Add(MaxSessionAge)
	if lifetime > 0 && lifetime < MaxSessionAge {
		expiry = now.Add(lifetime)
	}

	meCtx, cancelMe := context.WithTimeout(r.Context(), spotifyTimeout)
	defer cancelMe()
	userID, status, err := a.fetchUserID(meCtx, tok.AccessToken)
	switch {
	case status == http.StatusForbidden:
		// In development mode Spotify refuses API calls from users who
		// aren't on the app's allowlist (AUTH-4). No session is created.
		fail(LoginInviteOnly, "user not allowlisted")
		return
	case err != nil:
		fail(LoginFailed, "fetching user failed", slog.Int("spotify_status", status))
		return
	}

	sc, err := a.NewSessionCookie(Session{AccessToken: tok.AccessToken, Expiry: expiry, UserID: userID})
	if err != nil {
		fail(LoginFailed, "writing session failed")
		return
	}
	http.SetCookie(w, sc)
	a.logger.LogAttrs(r.Context(), slog.LevelInfo, "login succeeded")
	http.Redirect(w, r, "/", http.StatusFound)
}

// refuseHead answers HEAD with 405. Go's "GET" route patterns also match
// HEAD, and running the login flow for a HEAD (a link scanner or prefetch)
// would use up the single-use login cookie and code.
func refuseHead(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodHead {
		return false
	}
	w.Header().Set("Allow", http.MethodGet)
	w.WriteHeader(http.StatusMethodNotAllowed)
	return true
}

// fetchUserID calls Spotify's GET /me. status is Spotify's HTTP status, or 0
// if there was no response.
func (a *Auth) fetchUserID(ctx context.Context, accessToken string) (id string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.meURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", 0, errors.New("GET /me: no response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, errors.New("GET /me: unexpected status")
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil || me.ID == "" {
		return "", resp.StatusCode, errors.New("GET /me: no user ID")
	}
	return me.ID, resp.StatusCode, nil
}

// Logout handles POST /auth/logout: it deletes the session cookie (AUTH-7).
// The route must be behind RequireSameOrigin.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// Status handles GET /api/session. It needs no session and never returns
// tokens or the user ID.
func (a *Auth) Status(w http.ResponseWriter, r *http.Request) {
	_, present, ok := a.readSession(r)
	if present && !ok {
		clearSession(w)
	}
	jsonapi.WriteJSON(w, http.StatusOK, map[string]bool{"loggedIn": ok})
}
