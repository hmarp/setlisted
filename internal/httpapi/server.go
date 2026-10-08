// Package httpapi holds the HTTP server: routing, middleware and handlers.
package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

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
// NewHandler(logger, static).
func NewServer(addr string, logger *slog.Logger, static fs.FS) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           NewHandler(logger, static),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}

// NewHandler builds the router and wraps it in the middleware chain:
// request logging, then panic recovery.
func NewHandler(logger *slog.Logger, static fs.FS) http.Handler {
	mux := http.NewServeMux()

	// API routes. Register each with a method, e.g. "GET /api/...".
	mux.HandleFunc("GET /api/health", handleHealth)

	// Any other /api/ path (or method) is a JSON not_found error.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		jsonapi.WriteError(w, jsonapi.NotFound())
	})

	// Everything else is the static UI.
	mux.Handle("/", http.FileServerFS(static))

	return logRequests(logger, recoverPanics(logger, mux))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
