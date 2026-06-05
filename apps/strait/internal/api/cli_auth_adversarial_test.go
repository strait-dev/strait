package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestGenerateDeviceCode_Format verifies that generateDeviceCode returns a
// 64-character lowercase hex string.
func TestGenerateDeviceCode_Format(t *testing.T) {
	t.Parallel()

	code, err := generateDeviceCode()
	require.NoError(t, err)
	require.Len(t,
		code, 64)

	if _, err := hex.DecodeString(code); err != nil {
		require.Failf(t, "test failure",

			"code is not valid hex: %v", err)
	}
}

// TestGenerateDeviceCode_Uniqueness verifies that 10000 generated device codes
// are all unique.
func TestGenerateDeviceCode_Uniqueness(t *testing.T) {
	t.Parallel()

	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := range n {
		code, err := generateDeviceCode()
		require.NoError(t, err)

		if _, exists := seen[code]; exists {
			require.Failf(t, "test failure",

				"duplicate device code at iteration %d: %q", i, code)
		}
		seen[code] = struct{}{}
	}
}

// TestGenerateUserCode_Format verifies that generateUserCode returns an
// 8-character string.
func TestGenerateUserCode_Format(t *testing.T) {
	t.Parallel()

	code, err := generateUserCode()
	require.NoError(t, err)
	require.Len(t,
		code, 8)
}

// TestGenerateUserCode_Alphabet verifies that all characters in the generated
// user code belong to the allowed userCodeAlphabet.
func TestGenerateUserCode_Alphabet(t *testing.T) {
	t.Parallel()

	for range 1000 {
		code, err := generateUserCode()
		require.NoError(t, err)

		for _, ch := range code {
			require.True(
				t, strings.ContainsRune(userCodeAlphabet,
					ch),
			)
		}
	}
}

// TestDeviceToken_ExpiredCode verifies that an expired device code is rejected
// with an expired_token error.
func TestDeviceToken_ExpiredCode(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeviceCodeFunc: func(_ context.Context, _, _, _ string, _ []string, _ time.Time) error {
			return nil
		},
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				Status:    "pending",
				ExpiresAt: time.Now().Add(-time.Hour),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"abc123","grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "expired_token",
		resp["error"])
}

// TestDeviceToken_AlreadyUsedCode verifies that an already-used device code is
// rejected with a token_already_exchanged error.
func TestDeviceToken_AlreadyUsedCode(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeviceCodeFunc: func(_ context.Context, _, _, _ string, _ []string, _ time.Time) error {
			return nil
		},
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				Status:    "used",
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"abc123","grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "token_already_exchanged",
		resp["error"])
}

// TestDeviceToken_InvalidGrantType verifies that a wrong grant_type value is
// rejected with a 400 error.
func TestDeviceToken_InvalidGrantType(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeviceCodeFunc: func(_ context.Context, _, _, _ string, _ []string, _ time.Time) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"abc123","grant_type":"authorization_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

// TestDeviceToken_EmptyDeviceCode verifies that an empty device_code value is
// rejected with a 400 error.
func TestDeviceToken_EmptyDeviceCode(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeviceCodeFunc: func(_ context.Context, _, _, _ string, _ []string, _ time.Time) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"","grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

// FuzzGenerateUserCode_Alphabet fuzzes generateUserCode to verify that every
// generated character is within the allowed alphabet.
func FuzzGenerateUserCode_Alphabet(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(42))
	f.Add(uint64(999999))

	f.Fuzz(func(t *testing.T, _ uint64) {
		code, err := generateUserCode()
		require.NoError(t, err)
		require.Len(t,
			code, 8)

		for _, ch := range code {
			require.True(
				t, strings.ContainsRune(userCodeAlphabet,
					ch),
			)
		}
	})
}

// FuzzDeviceCodeFormat fuzzes generateDeviceCode to verify that every
// generated code is valid 64-character hex.
func FuzzDeviceCodeFormat(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Add(uint64(12345))

	f.Fuzz(func(t *testing.T, _ uint64) {
		code, err := generateDeviceCode()
		require.NoError(t, err)
		require.Len(t,
			code, 64)

		if _, err := hex.DecodeString(code); err != nil {
			require.Failf(t, "test failure",

				"code is not valid hex: %v", err)
		}
	})
}
