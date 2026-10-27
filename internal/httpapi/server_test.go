package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hmarp/setlisted/web"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRoutes(t *testing.T) {
	h := NewHandler(discardLogger(), web.Static())
	tests := []struct {
		name         string
		method, path string
		wantStatus   int
		wantType     string
		wantBody     string // exact for JSON, substring for HTML
	}{
		{"health", "GET", "/api/health", 200, "application/json", `{"status":"ok"}` + "\n"},
		{"unknown api path", "GET", "/api/x", 404, "application/json",
			`{"error":{"code":"not_found","message":"Not found."}}` + "\n"},
		{"wrong method on api path", "POST", "/api/health", 404, "application/json",
			`{"error":{"code":"not_found","message":"Not found."}}` + "\n"},
		{"placeholder page", "GET", "/", 200, "text/html", "Setlist data from"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
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

func TestPlaceholderPageLinksToSetlistFM(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(discardLogger(), web.Static()).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
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
	h := NewHandler(logger, fstest.MapFS{})

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
