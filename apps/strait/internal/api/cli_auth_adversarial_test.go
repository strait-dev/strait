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
)

// TestGenerateDeviceCode_Format verifies that generateDeviceCode returns a
// 64-character lowercase hex string.
func TestGenerateDeviceCode_Format(t *testing.T) {
	t.Parallel()

	code, err := generateDeviceCode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 64 {
		t.Fatalf("expected 64-char hex string, got %d chars: %q", len(code), code)
	}
	if _, err := hex.DecodeString(code); err != nil {
		t.Fatalf("code is not valid hex: %v", err)
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
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if _, exists := seen[code]; exists {
			t.Fatalf("duplicate device code at iteration %d: %q", i, code)
		}
		seen[code] = struct{}{}
	}
}

// TestGenerateUserCode_Format verifies that generateUserCode returns an
// 8-character string.
func TestGenerateUserCode_Format(t *testing.T) {
	t.Parallel()

	code, err := generateUserCode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 8 {
		t.Fatalf("expected 8-char user code, got %d chars: %q", len(code), code)
	}
}

// TestGenerateUserCode_Alphabet verifies that all characters in the generated
// user code belong to the allowed userCodeAlphabet.
func TestGenerateUserCode_Alphabet(t *testing.T) {
	t.Parallel()

	for i := range 1000 {
		code, err := generateUserCode()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		for j, ch := range code {
			if !strings.ContainsRune(userCodeAlphabet, ch) {
				t.Fatalf("iteration %d: char %d (%q) not in alphabet %q", i, j, string(ch), userCodeAlphabet)
			}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "expired_token" {
		t.Fatalf("error = %q, want %q", resp["error"], "expired_token")
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "token_already_exchanged" {
		t.Fatalf("error = %q, want %q", resp["error"], "token_already_exchanged")
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// FuzzGenerateUserCode_Alphabet fuzzes generateUserCode to verify that every
// generated character is within the allowed alphabet.
func FuzzGenerateUserCode_Alphabet(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(42))
	f.Add(uint64(999999))

	f.Fuzz(func(t *testing.T, _ uint64) {
		code, err := generateUserCode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 8 {
			t.Fatalf("expected 8-char code, got %d", len(code))
		}
		for i, ch := range code {
			if !strings.ContainsRune(userCodeAlphabet, ch) {
				t.Fatalf("char %d (%q) not in alphabet", i, string(ch))
			}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 64 {
			t.Fatalf("expected 64-char hex, got %d chars", len(code))
		}
		if _, err := hex.DecodeString(code); err != nil {
			t.Fatalf("code is not valid hex: %v", err)
		}
	})
}
