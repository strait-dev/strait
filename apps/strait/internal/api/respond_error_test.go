package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// TestRespondError_AlwaysAPIError verifies respondError emits the canonical
// {error: APIError, request_id?} envelope for every supported input shape.
// The "error" field is never a bare string.
func TestRespondError_AlwaysAPIError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		status   int
		input    any
		wantCode string
		wantMsg  string
	}{
		{"string input", http.StatusBadRequest, "bad payload", ErrorCodeBadRequest, "bad payload"},
		{"empty string falls back to status text", http.StatusBadRequest, "", ErrorCodeBadRequest, http.StatusText(http.StatusBadRequest)},
		{"error input", http.StatusInternalServerError, errors.New("boom"), ErrorCodeInternalError, "boom"},
		{"nil error falls back to status text", http.StatusInternalServerError, error(nil), ErrorCodeInternalError, http.StatusText(http.StatusInternalServerError)},
		{"APIError value preserved", http.StatusConflict, APIError{Code: ErrorCodeConflict, Message: "dup"}, ErrorCodeConflict, "dup"},
		{"APIError pointer preserved", http.StatusForbidden, &APIError{Code: ErrorCodeForbidden, Message: "no"}, ErrorCodeForbidden, "no"},
		{"validation 422 default code", http.StatusUnprocessableEntity, "field required", ErrorCodeValidationFailed, "field required"},
		{"unauthorized 401 default code", http.StatusUnauthorized, "missing token", ErrorCodeAuthenticationRequired, "missing token"},
		{"service unavailable 503 default code", http.StatusServiceUnavailable, "down", ErrorCodeServiceUnavailable, "down"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			respondError(w, nil, tc.status, tc.input)

			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			var resp ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
			}
			if resp.Error == nil {
				t.Fatalf("error field is nil; body=%s", w.Body.String())
			}
			if resp.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", resp.Error.Code, tc.wantCode)
			}
			if resp.Error.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", resp.Error.Message, tc.wantMsg)
			}
		})
	}
}

// TestRespondError_NeverBareString guards against the legacy behavior where
// respondError(... "string") put the raw string into the "error" field.
func TestRespondError_NeverBareString(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	respondError(w, nil, http.StatusBadRequest, "raw text")

	var raw struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(raw.Error) == 0 || raw.Error[0] != '{' {
		t.Fatalf("error field must be a JSON object, got %s", string(raw.Error))
	}
}

// TestHumaNewError_OverrideShape verifies the Huma override produces the
// same canonical envelope for errors that flow through Huma's error
// pipeline (e.g. middleware-emitted errors, generated 4xx responses).
func TestHumaNewError_OverrideShape(t *testing.T) {
	t.Parallel()
	// Trigger override installation by constructing a server (sync.Once).
	_ = newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	// Call the overridden huma.NewError directly. After the server is
	// built, this returns a humaStatusError that JSON-marshals to the
	// canonical envelope.
	se := huma.NewError(http.StatusUnprocessableEntity, "validation failed", errors.New("field x is required"))
	if se.GetStatus() != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", se.GetStatus())
	}
	body, err := json.Marshal(se)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	if resp.Error == nil {
		t.Fatalf("error nil; body=%s", string(body))
	}
	if resp.Error.Code != ErrorCodeValidationFailed {
		t.Errorf("code = %q, want %q", resp.Error.Code, ErrorCodeValidationFailed)
	}
	if resp.Error.Message != "validation failed" {
		t.Errorf("message = %q, want %q", resp.Error.Message, "validation failed")
	}
	if len(resp.Error.Details) != 1 || resp.Error.Details[0] != "field x is required" {
		t.Errorf("details = %v, want [field x is required]", resp.Error.Details)
	}
}
