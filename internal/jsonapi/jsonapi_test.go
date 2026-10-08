package jsonapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"a": "b"})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got, want := rec.Body.String(), `{"a":"b"}`+"\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestWriteJSONUnencodable(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, func() {})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	want := `{"error":{"code":"internal_error","message":"Something went wrong. Try again."}}` + "\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		err        *Error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "invalid_request",
			err:        InvalidRequest("Enter a band name."),
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":{"code":"invalid_request","message":"Enter a band name."}}`,
		},
		{
			name:       "not_found",
			err:        NotFound(),
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":{"code":"not_found","message":"Not found."}}`,
		},
		{
			name:       "internal_error",
			err:        InternalError(),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":{"code":"internal_error","message":"Something went wrong. Try again."}}`,
		},
		{
			name: "with service",
			err: &Error{Status: http.StatusBadGateway, Code: "upstream_unavailable",
				Message: "Couldn't reach setlist.fm. Try again.", Service: "setlist.fm"},
			wantStatus: http.StatusBadGateway,
			wantBody:   `{"error":{"code":"upstream_unavailable","message":"Couldn't reach setlist.fm. Try again.","service":"setlist.fm"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, tt.err)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Body.String(); got != tt.wantBody+"\n" {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}
