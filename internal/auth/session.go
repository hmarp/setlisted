package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/hmarp/setlisted/internal/jsonapi"
)

// MaxSessionAge caps a login at one hour (spec AUTH-6), whatever lifetime
// Spotify gives the access token.
const MaxSessionAge = time.Hour

// Session is everything the server knows about a logged-in user. It lives
// only in the encrypted session cookie (ADR-004). It holds secrets, so it
// must never be logged or sent to the browser in readable form.
type Session struct {
	AccessToken string    `json:"t"`
	Expiry      time.Time `json:"e"`
	UserID      string    `json:"u"` // Spotify user ID
}

type sessionKey struct{}

// ContextWithSession returns ctx carrying s. RequireSession does this for
// real requests; handler tests can use it directly.
func ContextWithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionFromContext returns the session RequireSession put in the request
// context. ok is false outside a protected route.
func SessionFromContext(ctx context.Context) (s Session, ok bool) {
	s, ok = ctx.Value(sessionKey{}).(Session)
	return s, ok
}

// NewSessionCookie returns the session cookie for s, as a login sets it. It has
// no Max-Age, so it ends with the browser session; the expiry inside it
// enforces the one hour (ADR-009). Tests use it to make logged-in requests.
func (a *Auth) NewSessionCookie(s Session) (*http.Cookie, error) {
	value, err := a.sealer.seal(SessionCookie, s)
	if err != nil {
		return nil, err
	}
	return newCookie(SessionCookie, "/", value, 0), nil
}

// readSession returns the request's session. present reports whether a
// session cookie was sent at all; ok whether it is valid and unexpired.
func (a *Auth) readSession(r *http.Request) (s Session, present, ok bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return Session{}, false, false
	}
	if err := a.sealer.open(SessionCookie, c.Value, &s); err != nil {
		return Session{}, true, false
	}
	if s.AccessToken == "" || s.UserID == "" || !a.now().Before(s.Expiry) {
		return Session{}, true, false
	}
	return s, true, true
}

// clearSession deletes the session cookie.
func clearSession(w http.ResponseWriter) { clearCookie(w, SessionCookie, "/") }

// RequireSession lets the request through only with a valid session, which
// handlers read with SessionFromContext. Otherwise it answers
// 401 not_logged_in and deletes any unusable cookie (AUTH-8).
func (a *Auth) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, present, ok := a.readSession(r)
		if !ok {
			if present {
				clearSession(w)
			}
			jsonapi.WriteError(w, jsonapi.NotLoggedIn())
			return
		}
		next.ServeHTTP(w, r.WithContext(ContextWithSession(r.Context(), s)))
	})
}

// SpotifyUnauthorized ends the session after Spotify answers 401 to the
// user's access token: it deletes the cookie and writes 401 not_logged_in,
// so the UI sends the user to the login screen (AUTH-8). Handlers call it
// instead of writing any other response.
func SpotifyUnauthorized(w http.ResponseWriter) {
	clearSession(w)
	jsonapi.WriteError(w, jsonapi.NotLoggedIn())
}

// RequireSameOrigin rejects state-changing requests (any method other than
// GET and HEAD) whose Origin header isn't the app's own origin, with
// 403 forbidden_origin. A missing Origin is rejected too (ADR-004).
func (a *Auth) RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != a.origin {
			jsonapi.WriteError(w, jsonapi.ForbiddenOrigin())
			return
		}
		next.ServeHTTP(w, r)
	})
}
