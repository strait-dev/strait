package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			require.Equal(t, tc.
				status, w.
				Code)

			var resp ErrorResponse
			require.NoError(t,
				json.Unmarshal(w.Body.
					Bytes(), &resp))
			require.NotNil(t, resp.
				Error)
			assert.Equal(t, tc.
				wantCode,
				resp.Error.Code,
			)
			assert.Equal(t, tc.
				wantMsg, resp.
				Error.Message,
			)
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
	require.NoError(t,
		json.Unmarshal(w.Body.
			Bytes(), &raw))
	require.False(t, len(raw.Error) == 0 || raw.
		Error[0] != '{')
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
	require.Equal(t, http.
		StatusUnprocessableEntity,

		se.GetStatus())

	body, err := json.Marshal(se)
	require.NoError(t,
		err)

	var resp ErrorResponse
	require.NoError(t,
		json.Unmarshal(body, &resp))
	require.NotNil(t, resp.
		Error)
	assert.Equal(t, ErrorCodeValidationFailed,

		resp.Error.Code)
	assert.Equal(t, "validation failed",

		resp.
			Error.Message)
	assert.False(t, len(resp.Error.
		Details) !=
		1 || resp.Error.Details[0] !=
		"field x is required",
	)
}
