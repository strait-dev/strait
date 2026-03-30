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

			if w.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tc.wantCode)
			}

			body := w.Body.String()
			if strings.Contains(body, tc.leaked) {
				t.Errorf("response body leaks internal detail %q:\n%s", tc.leaked, body)
			}

			if !strings.Contains(body, "internal server error") {
				t.Errorf("response body should contain generic error message, got:\n%s", body)
			}
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

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Error.Message != "name is required" {
		t.Errorf("message = %q, want %q", resp.Error.Message, "name is required")
	}
}

func TestWriteTypedError_WrappedError_NoLeak(t *testing.T) {
	t.Parallel()

	inner := errors.New("FATAL: password authentication failed for user \"admin\"")
	wrapped := fmt.Errorf("database query failed: %w", inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	writeTypedError(w, r, wrapped)

	body := w.Body.String()
	if strings.Contains(body, "password") {
		t.Errorf("response leaks password-related error: %s", body)
	}
	if strings.Contains(body, "admin") {
		t.Errorf("response leaks username: %s", body)
	}
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

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}

		body := w.Body.String()
		if errMsg != "" && errMsg != "internal server error" && strings.Contains(body, errMsg) {
			t.Errorf("response leaks raw error message: %q", errMsg)
		}
	})
}
