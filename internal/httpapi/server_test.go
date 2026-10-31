package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hmarp/setlisted/internal/auth"
	"github.com/hmarp/setlisted/internal/setlistfm"
	"github.com/hmarp/setlisted/web"
)

const testBaseURL = "https://setlisted.example"

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func testAuth(t *testing.T) *auth.Auth {
	t.Helper()
	a, err := auth.New(auth.Config{
		ClientID:   "client-id",
		BaseURL:    testBaseURL,
		SessionKey: bytes.Repeat([]byte{7}, 32),
		// No Spotify server: these tests don't complete a login.
		AuthURL: "https://accounts.invalid/authorize",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// noSetlistFM returns a setlist.fm client for tests that don't call it.
func noSetlistFM() *setlistfm.Client {
	return setlistfm.New(setlistfm.Config{APIKey: "key", BaseURL: "http://setlistfm.invalid"})
}

// loggedIn returns a valid session cookie for a.
func loggedIn(t *testing.T, a *auth.Auth) *http.Cookie {
	t.Helper()
	c, err := a.NewSessionCookie(auth.Session{AccessToken: "token", Expiry: time.Now().Add(time.Hour), UserID: "user"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRoutes(t *testing.T) {
	a := testAuth(t)
	h := NewHandler(discardLogger(), web.Static(), a, noSetlistFM())
	session := loggedIn(t, a)
	notFound := `{"error":{"code":"not_found","message":"Not found."}}` + "\n"
	notLoggedIn := `{"error":{"code":"not_logged_in","message":"You're not logged in. Log in with Spotify."}}` + "\n"
	forbidden := `{"error":{"code":"forbidden_origin","message":"This request isn't allowed."}}` + "\n"
	tests := []struct {
		name         string
		method, path string
		origin       string
		session      bool
		wantStatus   int
		wantType     string
		wantBody     string // exact for JSON, substring for HTML
	}{
		{"health", "GET", "/api/health", "", false, 200, "application/json", `{"status":"ok"}` + "\n"},
		{"session logged out", "GET", "/api/session", "", false, 200, "application/json", `{"loggedIn":false}` + "\n"},
		{"session logged in", "GET", "/api/session", "", true, 200, "application/json", `{"loggedIn":true}` + "\n"},
		// Every other /api/ route is protected, including unknown ones.
		{"api route without session", "GET", "/api/x", "", false, 401, "application/json", notLoggedIn},
		{"artist search without session", "GET", "/api/artists?q=gojira", "", false, 401, "application/json", notLoggedIn},
		{"show list without session", "GET", "/api/artists/5a85c140-dcf8-4a4f-b8d6-1e5a2a4e0f4b/shows", "", false, 401, "application/json", notLoggedIn},
		{"wrong method on artist search", "POST", "/api/artists?q=gojira", testBaseURL, true, 404, "application/json", notFound},
		{"unknown api path with session", "GET", "/api/x", "", true, 404, "application/json", notFound},
		{"wrong method on api path", "POST", "/api/health", testBaseURL, true, 404, "application/json", notFound},
		{"wrong method on public path, logged out", "POST", "/api/session", testBaseURL, false, 404, "application/json", notFound},
		{"POST without Origin", "POST", "/api/x", "", true, 403, "application/json", forbidden},
		{"POST with foreign Origin", "POST", "/auth/logout", "https://evil.example", true, 403, "application/json", forbidden},
		{"placeholder page", "GET", "/", "", false, 200, "text/html", "Setlist data from"},
		{"login link on placeholder page", "GET", "/", "", false, 200, "text/html", `href="/auth/login"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.session {
				req.AddCookie(session)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tt.wantType) {
				t.Errorf("Content-Type = %q, want prefix %q", ct, tt.wantType)
			}
			body := rec.Body.String()
			if tt.wantType == "application/json" {
				if body != tt.wantBody {
					t.Errorf("body = %q, want %q", body, tt.wantBody)
				}
			} else if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body %q does not contain %q", body, tt.wantBody)
			}
		})
	}
}

func TestAuthRoutes(t *testing.T) {
	a := testAuth(t)
	h := NewHandler(discardLogger(), web.Static(), a, noSetlistFM())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/login", nil))
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "https://accounts.invalid/authorize?") {
		t.Errorf("login: %d %q, want a redirect to Spotify", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/callback?code=x&state=y", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?login=failed" {
		t.Errorf("callback without login cookie: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.Header.Set("Origin", testBaseURL)
	req.AddCookie(loggedIn(t, a))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Set-Cookie"), auth.SessionCookie+"=; Path=/; Max-Age=0") {
		t.Errorf("logout: %d, Set-Cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
}

func TestPlaceholderPageLinksToSetlistFM(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(discardLogger(), web.Static(), testAuth(t), noSetlistFM()).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), `href="https://www.setlist.fm"`) {
		t.Errorf("placeholder page has no setlist.fm link")
	}
}

func TestRecoverPanics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := logAndRecover(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	want := `{"error":{"code":"internal_error","message":"Something went wrong. Try again."}}` + "\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if !strings.Contains(logs.String(), `"status":500`) {
		t.Errorf("request log does not record status 500: %s", logs.String())
	}
}

func TestPanicAfterWriteAbortsResponse(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := logAndRecover(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if v := recover(); v != http.ErrAbortHandler {
				t.Errorf("recovered %v, want http.ErrAbortHandler", v)
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	}()

	if got := rec.Body.String(); got != "partial" {
		t.Errorf("body = %q, want only the partial write", got)
	}
	if !strings.Contains(logs.String(), `"status":200`) {
		t.Errorf("request log does not record status 200: %s", logs.String())
	}
}

func TestLoggedStatus(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"status then body", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("x"))
		}, `"status":201`},
		{"body only", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }, `"status":200`},
		{"WriteHeader twice logs the last", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.WriteHeader(http.StatusNotFound)
		}, `"status":404`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := logAndRecover(slog.New(slog.NewJSONHandler(&logs, nil)), tt.handler)
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
			if !strings.Contains(logs.String(), tt.want) {
				t.Errorf("log %s does not contain %s", logs.String(), tt.want)
			}
		})
	}
}

func TestLogRequestsOmitsQueryString(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := NewHandler(logger, fstest.MapFS{}, testAuth(t), noSetlistFM())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/health?q=secret+band", nil))

	out := logs.String()
	for _, want := range []string{`"method":"GET"`, `"path":"/api/health"`, `"status":200`, `"duration_ms":`} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q missing %s", out, want)
		}
	}
	if strings.Contains(out, "secret") {
		t.Errorf("log contains the query string: %s", out)
	}
}
