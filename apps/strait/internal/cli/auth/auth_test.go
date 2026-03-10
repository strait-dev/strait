package auth

import (
	"testing"
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
