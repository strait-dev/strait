package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestKeyName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty returns default", input: "", expected: "default"},
		{name: "whitespace only returns default", input: "   ", expected: "default"},
		{name: "simple name", input: "production", expected: "production"},
		{name: "trimmed name", input: "  staging  ", expected: "staging"},
		{name: "name with special chars", input: "my-context", expected: "my-context"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := KeyName(tt.input)
			if got != tt.expected {
				t.Fatalf("KeyName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSaveAPIKey_EmptyKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		key  string
	}{
		{name: "empty string", key: ""},
		{name: "whitespace only", key: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := SaveAPIKey("test", tt.key)
			if err == nil {
				t.Fatal("expected error for empty API key")
			}
		})
	}
}

func TestValidateAPIKey_EmptyInputs(t *testing.T) {
	t.Parallel()

	t.Run("empty server URL", func(t *testing.T) {
		t.Parallel()
		err := ValidateAPIKey(t.Context(), "", "some-key", 0)
		if err == nil {
			t.Fatal("expected error for empty server URL")
		}
	})

	t.Run("invalid server URL", func(t *testing.T) {
		t.Parallel()
		err := ValidateAPIKey(t.Context(), "not-a-url", "some-key", 0)
		if err == nil {
			t.Fatal("expected error for invalid URL scheme")
		}
	})

	t.Run("empty API key", func(t *testing.T) {
		t.Parallel()
		err := ValidateAPIKey(t.Context(), "https://example.com", "", 0)
		if err == nil {
			t.Fatal("expected error for empty API key")
		}
	})

	t.Run("whitespace API key", func(t *testing.T) {
		t.Parallel()
		err := ValidateAPIKey(t.Context(), "https://example.com", "   ", 0)
		if err == nil {
			t.Fatal("expected error for whitespace API key")
		}
	})
}

func TestValidateAPIKey_HTTPResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "valid api key", statusCode: http.StatusOK, wantErr: false},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantErr: true},
		{name: "server error", statusCode: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatalf("expected method GET, got %s", r.Method)
				}
				if r.URL.Path != "/v1/stats" {
					t.Fatalf("expected path /v1/stats, got %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Fatalf("expected auth header with api key")
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			err := ValidateAPIKey(t.Context(), strings.TrimRight(srv.URL, "/")+"/", "test-key", time.Second)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSaveLoadDeleteAPIKey(t *testing.T) {
	keyring.MockInit()

	if err := SaveAPIKey("staging", "sk_test_123"); err != nil {
		t.Fatalf("SaveAPIKey: %v", err)
	}

	got, err := LoadAPIKey("staging")
	if err != nil {
		t.Fatalf("LoadAPIKey: %v", err)
	}
	if got != "sk_test_123" {
		t.Fatalf("expected sk_test_123, got %q", got)
	}

	if err := DeleteAPIKey("staging"); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	_, err = LoadAPIKey("staging")
	if err == nil {
		t.Fatal("expected error for deleted key")
	}
}

func TestDeleteAPIKey_NotFound(t *testing.T) {
	keyring.MockInit()

	if err := DeleteAPIKey("missing"); err != nil {
		t.Fatalf("DeleteAPIKey should ignore not found: %v", err)
	}
}
