// Package httpapi holds the HTTP server: routing, middleware and handlers.
package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/hmarp/setlisted/internal/auth"
	"github.com/hmarp/setlisted/internal/jsonapi"
	"github.com/hmarp/setlisted/internal/setlistfm"
	"github.com/hmarp/setlisted/internal/spotify"
)

// Server timeouts. WriteTimeout is generous because later endpoints (the
// preview) make many upstream calls before responding.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
)

// Deps are the handler's dependencies, built once in main. Every field is
// required.
type Deps struct {
	Logger    *slog.Logger
	Static    fs.FS // the UI's files
	Auth      *auth.Auth
	SetlistFM *setlistfm.Client
	Spotify   *spotify.Client
}

// NewServer returns an *http.Server listening on addr and serving
// NewHandler(d).
func NewServer(addr string, d Deps) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           NewHandler(d),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(d.Logger.Handler(), slog.LevelWarn),
	}
}

// NewHandler builds the router and wraps it in the middleware chain:
// request logging and panic recovery, then the Origin check for every
// state-changing request.
func NewHandler(d Deps) http.Handler {
	logger, a := d.Logger, d.Auth
	mux := http.NewServeMux()

	// Public API routes: no session needed.
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/session", a.Status)
	// Other methods on the public paths: not_found, not the session check.
	mux.HandleFunc("/api/health", handleNotFound)
	mux.HandleFunc("/api/session", handleNotFound)

	// Every other /api/ route requires a session. Register protected routes
	// on api, with a method, e.g. api.HandleFunc("GET /api/...", h).
	api := http.NewServeMux()
	setlists := &setlistFMHandlers{logger: logger, client: d.SetlistFM, budget: setlistFMBudget}
	api.HandleFunc("GET /api/artists", setlists.searchArtists)
	api.HandleFunc("GET /api/artists/{id}/shows", setlists.artistShows)
	preview := &previewHandler{logger: logger, setlistFM: d.SetlistFM, spotify: d.Spotify, budget: previewBudget}
	api.HandleFunc("GET /api/shows/{id}/preview", preview.serve)
	// Any other /api/ path (or method) is a JSON not_found error.
	api.HandleFunc("/api/", handleNotFound)
	mux.Handle("/api/", a.RequireSession(api))

	// Spotify login.
	mux.HandleFunc("GET /auth/login", a.Login)
	mux.HandleFunc("GET "+auth.CallbackPath, a.Callback)
	mux.HandleFunc("POST /auth/logout", a.Logout)

	// Everything else is the static UI.
	mux.Handle("/", http.FileServerFS(d.Static))

	return logAndRecover(logger, a.RequireSameOrigin(mux))
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	jsonapi.WriteError(w, jsonapi.NotFound())
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
