package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTypedError_SanitizesRawError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		leaked   string
		wantCode int
	}{
		{
			name:     "SQL error not leaked",
			err:      fmt.Errorf("pq: relation \"users\" does not exist"),
			leaked:   "pq: relation",
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "file path not leaked",
			err:      fmt.Errorf("open /var/data/secrets.json: no such file or directory"),
			leaked:   "/var/data",
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "connection string not leaked",
			err:      fmt.Errorf("dial tcp 10.0.0.5:5432: connection refused"),
			leaked:   "10.0.0.5",
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "stack trace not leaked",
			err:      fmt.Errorf("goroutine 42 [running]:\nmain.handler(0xc0001a2000)"),
			leaked:   "goroutine",
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/test", nil)

			writeTypedError(w, r, tc.err)
			assert.Equal(t, tc.wantCode,
				w.Code)

			body := w.Body.String()
			assert.False(t, strings.Contains(body,
				tc.leaked,
			))
			assert.True(t, strings.Contains(body,
				"internal server error",
			))

		})
	}
}

func TestWriteTypedError_PreservesKnownErrorTypes(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	apiErr := &typedAPIError{
		status: http.StatusBadRequest,
		apiError: APIError{
			Code:    "VALIDATION_ERROR",
			Message: "name is required",
		},
	}

	writeTypedError(w, r, apiErr)
	assert.Equal(t, http.StatusBadRequest,

		w.Code,
	)

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "name is required",
		resp.
			Error.
			Message)

}

func TestWriteTypedError_WrappedError_NoLeak(t *testing.T) {
	t.Parallel()

	inner := errors.New("FATAL: password authentication failed for user \"admin\"")
	wrapped := fmt.Errorf("database query failed: %w", inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	writeTypedError(w, r, wrapped)

	body := w.Body.String()
	assert.False(t, strings.Contains(body,
		"password",
	))
	assert.False(t, strings.Contains(body,
		"admin",
	))

}

func TestWriteTypedError_Huma5xxDoesNotLeakMessage(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	writeTypedError(w, r, huma.Error500InternalServerError("failed to retry workflow run: pq: relation workflow_runs missing"))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

	body := w.Body.String()
	require.False(t, strings.Contains(body,
		"workflow_runs",
	) ||
		strings.Contains(body,
			"pq:") ||
		strings.Contains(body, "failed to retry"))
	require.True(t, strings.Contains(body,
		"internal server error",
	))

}

func TestWriteTypedError_Huma4xxKeepsPublicMessage(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	writeTypedError(w, r, huma.Error404NotFound("job not found"))
	require.Equal(t, http.StatusNotFound,

		w.Code)
	require.True(t, strings.Contains(w.Body.
		String(), "job not found",
	))

}

func FuzzWriteTypedError(f *testing.F) {
	f.Add("simple error")
	f.Add("pq: relation \"users\" does not exist")
	f.Add("open /etc/passwd: permission denied")
	f.Add("dial tcp 192.168.1.1:5432: connection refused")
	f.Add("goroutine 1 [running]:\nmain.main()")
	f.Add("password=hunter2&token=abc123")
	f.Add("")
	f.Add(strings.Repeat("x", 10000))

	f.Fuzz(func(t *testing.T, errMsg string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/fuzz", bytes.NewReader(nil))

		writeTypedError(w, r, errors.New(errMsg))
		assert.Equal(t, http.StatusInternalServerError,

			w.Code)

		body := w.Body.String()
		assert.False(t, errMsg != "" &&
			errMsg !=
				"internal server error" &&
			strings.Contains(body,
				errMsg))

	})
}
