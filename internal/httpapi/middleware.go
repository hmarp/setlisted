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
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Flush lets handlers that assert http.Flusher stream through the recorder.
func (r *statusRecorder) Flush() {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	_ = http.NewResponseController(r.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logAndRecover logs one line per request (method, path, status and duration
// in milliseconds) and turns a panic in a handler into a logged internal_error
// response, so a bug in one request doesn't crash the server. The request log
// deliberately omits the query string, headers and bodies, because they may
// contain user data such as search terms (spec GEN-5, ADR-003).
func logAndRecover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		abort := false
		defer func() {
			if v := recover(); v != nil {
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
					jsonapi.WriteError(rec, jsonapi.InternalError())
				} else {
					abort = true // response already started: let the client see a failed request
				}
			}
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
			if abort {
				panic(http.ErrAbortHandler)
			}
		}()
		next.ServeHTTP(rec, r)
	})
}
