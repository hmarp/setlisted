package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/hmarp/setlisted/internal/jsonapi"
)

// statusRecorder captures the status code written by the next handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logRequests logs one line per request: method, path, status and duration.
// Duration is in milliseconds. It deliberately omits the query string, headers and bodies, because they
// may contain user data such as search terms (spec GEN-5, ADR-003).
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.LogAttrs(r.Context(), slog.LevelInfo, "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
		)
	})
}

// recoverPanics turns a panic in a handler into a logged internal_error
// response, so a bug in one request doesn't crash the server.
func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v) // the server handles this sentinel itself
			}
			logger.LogAttrs(r.Context(), slog.LevelError, "panic serving request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Any("panic", v),
				slog.String("stack", string(debug.Stack())),
			)
			if rec.status == 0 {
				jsonapi.WriteError(w, jsonapi.InternalError())
			}
		}()
		next.ServeHTTP(rec, r)
	})
}
