package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testClientID     = "client-id"
	testClientSecret = "client-secret"
	testBaseURL      = "https://setlisted.example"
	testRefreshToken = "REFRESH-TOKEN-MUST-NOT-LEAK"
)

var (
	testKey  = bytes.Repeat([]byte{7}, 32)
	otherKey = bytes.Repeat([]byte{9}, 32)
	testNow  = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
)

// fakeSpotify is a fake Spotify accounts and Web API server. Codes it accepts:
// "good" (an allowlisted user), "not-allowlisted" (GET /me answers 403),
// "me-fails" (GET /me answers 500), "short" (token expires in 10 minutes).
type fakeSpotify struct {
	*httptest.Server
	mu        sync.Mutex
	challenge string // the code_challenge Login sent, checked at the token endpoint
	tokenErr  string // a reason the last token request was refused, for diagnosis
}

func newFakeSpotify(t *testing.T) *fakeSpotify {
	f := &fakeSpotify{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/token", f.token)
	mux.HandleFunc("GET /v1/me", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer access-good", "Bearer access-short":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"user-123","display_name":"Test"}`))
		case "Bearer access-not-allowlisted":
			http.Error(w, "Check settings on developer.spotify.com/dashboard, the user may not be registered.", http.StatusForbidden)
		default:
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
		}
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeSpotify) token(w http.ResponseWriter, r *http.Request) {
	refuse := func(reason string) {
		f.mu.Lock()
		f.tokenErr = reason
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}
	id, secret, ok := r.BasicAuth()
	if !ok || id != testClientID || secret != testClientSecret {
		refuse("bad client credentials")
		return
	}
	if err := r.ParseForm(); err != nil {
		refuse("bad form")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		refuse("bad grant_type")
		return
	}
	if r.PostForm.Get("redirect_uri") != testBaseURL+CallbackPath {
		refuse("bad redirect_uri")
		return
	}
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	f.mu.Lock()
	challenge := f.challenge
	f.mu.Unlock()
	if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		refuse("PKCE verifier doesn't match the challenge")
		return
	}
	code := r.PostForm.Get("code")
	expiresIn := 3600
	switch code {
	case "good", "not-allowlisted", "me-fails":
	case "short":
		expiresIn = 600
	case "form":
		// oauth2 fills only Token.Expiry (not ExpiresIn) for form responses.
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		w.Write([]byte("access_token=access-short&token_type=bearer&expires_in=600&refresh_token=" + testRefreshToken))
		return
	default:
		refuse("unknown code")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "access-" + code,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"refresh_token": testRefreshToken,
		"scope":         Scope,
	})
}

func newTestAuth(t *testing.T, f *fakeSpotify, logs *bytes.Buffer) *Auth {
	t.Helper()
	var handler slog.Handler = slog.DiscardHandler
	if logs != nil {
		handler = slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	cfg := Config{
		ClientID:     testClientID,
		ClientSecret: testClientSecret,
		BaseURL:      testBaseURL,
		SessionKey:   testKey,
		Logger:       slog.New(handler),
		Now:          func() time.Time { return testNow },
	}
	if f != nil {
		cfg.AuthURL = f.URL + "/authorize"
		cfg.TokenURL = f.URL + "/api/token"
		cfg.APIBaseURL = f.URL + "/v1"
		cfg.HTTPClient = f.Client()
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// login runs GET /auth/login and returns the authorise URL's query and the
// login cookie. It records the PKCE challenge on f.
func login(t *testing.T, a *Auth, f *fakeSpotify) (url.Values, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	a.Login(rec, httptest.NewRequest("GET", "/auth/login", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want 302", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("bad Location: %v", err)
	}
	q := loc.Query()
	if f != nil {
		f.mu.Lock()
		f.challenge = q.Get("code_challenge")
		f.mu.Unlock()
	}
	c := findCookie(rec.Result(), loginCookie)
	if c == nil {
		t.Fatal("login set no login cookie")
	}
	return q, c
}

// callback runs GET /auth/callback with the given query and cookie.
func callback(a *Auth, query string, c *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", CallbackPath+"?"+query, nil)
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	a.Callback(rec, req)
	return rec
}

func findCookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestLoginRedirect(t *testing.T) {
	f := newFakeSpotify(t)
	a := newTestAuth(t, f, nil)
	rec := httptest.NewRecorder()
	a.Login(rec, httptest.NewRequest("GET", "/auth/login", nil))

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if got := loc.Scheme + "://" + loc.Host + loc.Path; got != f.URL+"/authorize" {
		t.Errorf("redirects to %s, want the authorise URL", got)
	}
	q := loc.Query()
	want := map[string]string{
		"client_id":             testClientID,
		"redirect_uri":          testBaseURL + "/auth/callback",
		"response_type":         "code",
		"scope":                 "playlist-modify-private",
		"code_challenge_method": "S256",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if len(q.Get("state")) < 20 {
		t.Errorf("state %q is missing or too short", q.Get("state"))
	}
	if len(q.Get("code_challenge")) != 43 { // base64url of a SHA-256
		t.Errorf("code_challenge %q is not an S256 challenge", q.Get("code_challenge"))
	}

	c := findCookie(rec.Result(), loginCookie)
	if c == nil {
		t.Fatal("no login cookie")
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/auth" {
		t.Errorf("login cookie attributes = %+v", c)
	}
	if c.MaxAge <= 0 || c.MaxAge > 15*60 {
		t.Errorf("login cookie Max-Age = %d, want a few minutes", c.MaxAge)
	}
	var ls loginState
	if err := a.sealer.open(loginCookie, c.Value, &ls); err != nil {
		t.Fatalf("login cookie doesn't decrypt: %v", err)
	}
	if ls.State != q.Get("state") {
		t.Error("login cookie state differs from the redirect's state")
	}
	sum := sha256.Sum256([]byte(ls.Verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != q.Get("code_challenge") {
		t.Error("code_challenge isn't the S256 of the stored verifier")
	}

	// Each login gets fresh values.
	q2, _ := login(t, a, nil)
	if q2.Get("state") == q.Get("state") || q2.Get("code_challenge") == q.Get("code_challenge") {
		t.Error("state or challenge reused between logins")
	}
}

func TestCallbackSuccess(t *testing.T) {
	f := newFakeSpotify(t)
	var logs bytes.Buffer
	a := newTestAuth(t, f, &logs)
	q, lc := login(t, a, f)

	rec := callback(a, "code=good&state="+url.QueryEscape(q.Get("state")), lc)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		f.mu.Lock()
		t.Fatalf("status %d, Location %q, want 302 to / (token endpoint: %q)", rec.Code, rec.Header().Get("Location"), f.tokenErr)
	}
	resp := rec.Result()

	c := findCookie(resp, SessionCookie)
	if c == nil {
		t.Fatal("no session cookie")
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("session cookie attributes = %+v", c)
	}
	// A browser-session cookie: no Max-Age or Expires (ADR-009, AUTH-6).
	if c.MaxAge != 0 || !c.Expires.IsZero() || strings.Contains(c.Raw, "Max-Age") || strings.Contains(c.Raw, "Expires") {
		t.Errorf("session cookie must have no Max-Age or Expires: %q", c.Raw)
	}

	var s Session
	if err := a.sealer.open(SessionCookie, c.Value, &s); err != nil {
		t.Fatalf("session cookie doesn't decrypt: %v", err)
	}
	if s.AccessToken != "access-good" || s.UserID != "user-123" {
		t.Errorf("session = %+v", s)
	}
	if !s.Expiry.After(testNow) || s.Expiry.After(testNow.Add(time.Hour)) {
		t.Errorf("expiry %v not within an hour of %v", s.Expiry, testNow)
	}

	// The login cookie is deleted.
	if lc := findCookie(resp, loginCookie); lc == nil || lc.MaxAge >= 0 {
		t.Errorf("login cookie not deleted: %+v", lc)
	}

	// The refresh token, the access token and the user ID appear nowhere:
	// not in the decrypted session, the response, or the logs.
	plain, _ := json.Marshal(s)
	var headers bytes.Buffer
	resp.Header.Write(&headers)
	for where, text := range map[string]string{"session": string(plain), "headers": headers.String(), "logs": logs.String()} {
		if strings.Contains(text, testRefreshToken) {
			t.Errorf("refresh token found in %s", where)
		}
	}
	for _, secret := range []string{"access-good", "user-123", "code=good", q.Get("state")} {
		if strings.Contains(logs.String(), secret) || strings.Contains(headers.String(), secret) {
			t.Errorf("%q found in the logs or headers", secret)
		}
	}
}

func TestCallbackShortToken(t *testing.T) {
	// The session ends when the token does, if that's sooner than an hour,
	// whether Spotify answers in JSON or (hypothetically) form-encoded.
	for _, code := range []string{"short", "form"} {
		t.Run(code, func(t *testing.T) {
			f := newFakeSpotify(t)
			a := newTestAuth(t, f, nil)
			q, lc := login(t, a, f)
			rec := callback(a, "code="+code+"&state="+url.QueryEscape(q.Get("state")), lc)
			c := findCookie(rec.Result(), SessionCookie)
			if c == nil {
				t.Fatalf("no session cookie; Location %q", rec.Header().Get("Location"))
			}
			var s Session
			if err := a.sealer.open(SessionCookie, c.Value, &s); err != nil {
				t.Fatal(err)
			}
			if want := testNow.Add(10 * time.Minute); s.Expiry.Sub(want).Abs() > time.Minute {
				t.Errorf("expiry = %v, want about %v", s.Expiry, want)
			}
		})
	}
}

func TestCallbackFailures(t *testing.T) {
	f := newFakeSpotify(t)
	a := newTestAuth(t, f, nil)

	tests := []struct {
		name       string
		query      func(state string) string
		noCookie   bool
		otherState bool // use a cookie from a different login
		wantLoc    string
	}{
		{"missing state", func(string) string { return "code=good" }, false, false, "/?login=failed"},
		{"wrong state", func(string) string { return "code=good&state=forged" }, false, false, "/?login=failed"},
		{"state from another login", func(s string) string { return "code=good&state=" + s }, false, true, "/?login=failed"},
		{"no login cookie", func(s string) string { return "code=good&state=" + s }, true, false, "/?login=failed"},
		{"cancelled", func(s string) string { return "error=access_denied&state=" + s }, false, false, "/?login=cancelled"},
		{"cancelled after the login cookie expired", func(s string) string { return "error=access_denied&state=" + s }, true, false, "/?login=cancelled"},
		{"other Spotify error", func(s string) string { return "error=server_error&state=" + s }, false, false, "/?login=failed"},
		{"no code", func(s string) string { return "state=" + s }, false, false, "/?login=failed"},
		{"code refused", func(s string) string { return "code=bogus&state=" + s }, false, false, "/?login=failed"},
		{"not allowlisted", func(s string) string { return "code=not-allowlisted&state=" + s }, false, false, "/?login=invite_only"},
		{"GET /me fails", func(s string) string { return "code=me-fails&state=" + s }, false, false, "/?login=failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, lc := login(t, a, f)
			if tt.otherState {
				_, lc = login(t, a, nil)
			}
			if tt.noCookie {
				lc = nil
			}
			rec := callback(a, tt.query(url.QueryEscape(q.Get("state"))), lc)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != tt.wantLoc {
				t.Errorf("status %d, Location %q, want 302 to %s", rec.Code, rec.Header().Get("Location"), tt.wantLoc)
			}
			if c := findCookie(rec.Result(), SessionCookie); c != nil {
				t.Errorf("session cookie set: %+v", c)
			}
		})
	}
}

// sessionCookie returns a session cookie sealed with key.
func sessionCookie(t *testing.T, key []byte, s Session) *http.Cookie {
	t.Helper()
	sl, err := newSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	v, err := sl.seal(SessionCookie, s)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: SessionCookie, Value: v}
}

func validSession() Session {
	return Session{AccessToken: "access-good", Expiry: testNow.Add(30 * time.Minute), UserID: "user-123"}
}

// tamper flips one bit in the middle of the cookie's ciphertext.
func tamper(c *http.Cookie) *http.Cookie {
	b, _ := base64.RawURLEncoding.DecodeString(c.Value)
	b[len(b)/2] ^= 1
	return &http.Cookie{Name: c.Name, Value: base64.RawURLEncoding.EncodeToString(b)}
}

func TestRequireSession(t *testing.T) {
	a := newTestAuth(t, nil, nil)
	var got Session
	var called bool
	h := a.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, called = SessionFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	good := sessionCookie(t, testKey, validSession())
	login := func() *http.Cookie { // a login cookie presented as a session cookie
		v, _ := a.sealer.seal(loginCookie, loginState{State: "s", Verifier: "v"})
		return &http.Cookie{Name: SessionCookie, Value: v}
	}
	expired := validSession()
	expired.Expiry = testNow
	tests := []struct {
		name   string
		cookie *http.Cookie
	}{
		{"no cookie", nil},
		{"tampered", tamper(good)},
		{"other key", sessionCookie(t, otherKey, validSession())},
		{"expired", sessionCookie(t, testKey, expired)},
		{"not base64", &http.Cookie{Name: SessionCookie, Value: "!!!"}},
		{"empty", &http.Cookie{Name: SessionCookie, Value: ""}},
		{"login cookie value", login()},
		{"no token", sessionCookie(t, testKey, Session{Expiry: testNow.Add(time.Minute), UserID: "u"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest("GET", "/api/x", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if called {
				t.Error("handler called without a valid session")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			want := `{"error":{"code":"not_logged_in","message":"You're not logged in. Log in with Spotify."}}` + "\n"
			if rec.Body.String() != want {
				t.Errorf("body = %q", rec.Body.String())
			}
			c := findCookie(rec.Result(), SessionCookie)
			if tt.cookie != nil && (c == nil || c.MaxAge >= 0) {
				t.Errorf("unusable cookie not deleted: %+v", c)
			}
		})
	}

	t.Run("valid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/x", nil)
		req.AddCookie(good)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent || !called {
			t.Fatalf("status = %d, called = %v", rec.Code, called)
		}
		if got != validSession() {
			t.Errorf("context session = %+v", got)
		}
	})
}

func TestSessionFromContextOutsideProtectedRoute(t *testing.T) {
	if _, ok := SessionFromContext(httptest.NewRequest("GET", "/", nil).Context()); ok {
		t.Error("ok = true without a session")
	}
}

func TestRequireSameOrigin(t *testing.T) {
	a := newTestAuth(t, nil, nil)
	h := a.RequireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	tests := []struct {
		method, origin string
		want           int
	}{
		{"POST", "", 403},
		{"POST", "https://evil.example", 403},
		{"POST", "http://setlisted.example", 403}, // scheme differs
		{"POST", "https://setlisted.example:8443", 403},
		{"POST", "null", 403},
		{"POST", testBaseURL, 204},
		{"DELETE", "", 403},
		{"GET", "", 204},
		{"HEAD", "https://evil.example", 204},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.origin, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/auth/logout", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
			if tt.want == 403 && !strings.Contains(rec.Body.String(), `"code":"forbidden_origin"`) {
				t.Errorf("body = %q", rec.Body.String())
			}
		})
	}
}

func TestStatus(t *testing.T) {
	a := newTestAuth(t, nil, nil)
	for _, tt := range []struct {
		name   string
		cookie *http.Cookie
		want   string
	}{
		{"logged out", nil, `{"loggedIn":false}`},
		{"logged in", sessionCookie(t, testKey, validSession()), `{"loggedIn":true}`},
		{"tampered", tamper(sessionCookie(t, testKey, validSession())), `{"loggedIn":false}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/session", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()
			a.Status(rec, req)
			if rec.Code != 200 || rec.Body.String() != tt.want+"\n" {
				t.Errorf("got %d %q, want 200 %s", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestLogoutAndSpotifyUnauthorizedClearCookie(t *testing.T) {
	a := newTestAuth(t, nil, nil)
	rec := httptest.NewRecorder()
	a.Logout(rec, httptest.NewRequest("POST", "/auth/logout", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("logout status = %d, want 204", rec.Code)
	}
	if c := findCookie(rec.Result(), SessionCookie); c == nil || c.MaxAge >= 0 || c.Path != "/" {
		t.Errorf("logout didn't delete the session cookie: %+v", c)
	}

	rec = httptest.NewRecorder()
	SpotifyUnauthorized(rec)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"code":"not_logged_in"`) {
		t.Errorf("got %d %q, want 401 not_logged_in", rec.Code, rec.Body.String())
	}
	if c := findCookie(rec.Result(), SessionCookie); c == nil || c.MaxAge >= 0 {
		t.Errorf("SpotifyUnauthorized didn't delete the session cookie: %+v", c)
	}
}

func TestOriginIsNormalised(t *testing.T) {
	a, err := New(Config{BaseURL: "HTTPS://Setlisted.Example:443/", SessionKey: testKey})
	if err != nil {
		t.Fatal(err)
	}
	if a.origin != testBaseURL || a.oauth.RedirectURL != testBaseURL+CallbackPath {
		t.Errorf("origin %q, redirect URL %q", a.origin, a.oauth.RedirectURL)
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	for _, cfg := range []Config{
		{BaseURL: "setlisted.example", SessionKey: testKey},
		{BaseURL: testBaseURL + "/app", SessionKey: testKey},
		{BaseURL: testBaseURL, SessionKey: []byte("short")},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded", cfg)
		}
	}
}
