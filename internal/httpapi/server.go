// Package httpapi holds the HTTP server: routing, middleware and handlers.
package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/hmarp/setlisted/internal/auth"
	"github.com/hmarp/setlisted/internal/jsonapi"
)

// Server timeouts. WriteTimeout is generous because later endpoints (the
// preview) make many upstream calls before responding.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
)

// NewServer returns an *http.Server listening on addr and serving
// NewHandler(logger, static, a).
func NewServer(addr string, logger *slog.Logger, static fs.FS, a *auth.Auth) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           NewHandler(logger, static, a),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}

// NewHandler builds the router and wraps it in the middleware chain:
// request logging and panic recovery, then the Origin check for every
// state-changing request.
func NewHandler(logger *slog.Logger, static fs.FS, a *auth.Auth) http.Handler {
	mux := http.NewServeMux()

	// Public API routes: no session needed.
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/session", a.Status)

	// Every other /api/ route requires a session. Register protected routes
	// on api, with a method, e.g. api.HandleFunc("GET /api/...", h).
	api := http.NewServeMux()
	// Any other /api/ path (or method) is a JSON not_found error.
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		jsonapi.WriteError(w, jsonapi.NotFound())
	})
	mux.Handle("/api/", a.RequireSession(api))

	// Spotify login.
	mux.HandleFunc("GET /auth/login", a.Login)
	mux.HandleFunc("GET "+auth.CallbackPath, a.Callback)
	mux.HandleFunc("POST /auth/logout", a.Logout)

	// Everything else is the static UI.
	mux.Handle("/", http.FileServerFS(static))

	return logAndRecover(logger, a.RequireSameOrigin(mux))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
