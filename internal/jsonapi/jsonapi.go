// Package jsonapi writes JSON responses and the API's single error format
// (see docs/api.md). It has no dependencies on other internal packages, so
// any package that writes HTTP responses (handlers, middleware, auth) can use
// it.
package jsonapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Code is a stable, machine-readable error code. The UI may branch on it, so
// existing codes must not change meaning. New codes are added to docs/api.md.
type Code string

const (
	CodeInvalidRequest Code = "invalid_request"
	CodeNotFound       Code = "not_found"
	CodeInternalError  Code = "internal_error"
)

// Error is an API error. Message is user-facing text and must not contain
// technical details such as status codes, stack traces or raw upstream
// responses (spec ERR-5). Service names the external service involved, if
// any (e.g. "setlist.fm", "Spotify").
type Error struct {
	Status  int    `json:"-"`
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Service string `json:"service,omitempty"`
}

// Error implements the error interface, so an *Error can be returned and
// passed up the stack before being written.
func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// InvalidRequest returns a 400 invalid_request error with the given
// user-facing message.
func InvalidRequest(message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: CodeInvalidRequest, Message: message}
}

// NotFound returns a 404 not_found error.
func NotFound() *Error {
	return &Error{Status: http.StatusNotFound, Code: CodeNotFound, Message: "Not found."}
}

// InternalError returns a 500 internal_error error.
func InternalError() *Error {
	return &Error{Status: http.StatusInternalServerError, Code: CodeInternalError, Message: "Something went wrong. Try again."}
}

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		// A marshalling failure is a programming error. Don't leak it.
		slog.Error("encoding JSON response", "err", err)
		status = http.StatusInternalServerError
		body, _ = json.Marshal(errorBody{InternalError()})
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(append(body, '\n'))
}

type errorBody struct {
	Error *Error `json:"error"`
}

// WriteError writes e in the standard error format:
//
//	{"error": {"code": "...", "message": "...", "service": "..."}}
func WriteError(w http.ResponseWriter, e *Error) {
	WriteJSON(w, e.Status, errorBody{e})
}
