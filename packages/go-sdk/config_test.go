package strait

import (
	"errors"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.strait.dev", "https://api.strait.dev"},
		{"https://api.strait.dev/", "https://api.strait.dev"},
		{"https://api.strait.dev///", "https://api.strait.dev"},
		{"", ""},
	}

	for _, tt := range tests {
		result := NormalizeBaseURL(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetAuthorizationHeader(t *testing.T) {
	tests := []struct {
		auth     AuthMode
		expected string
	}{
		{AuthMode{Type: AuthTypeBearer, Token: "tok_123"}, "Bearer tok_123"},
		{AuthMode{Type: AuthTypeAPIKey, Token: "sk_live_abc"}, "Bearer sk_live_abc"},
		{AuthMode{Type: AuthTypeRunToken, Token: "rt_xyz"}, "Bearer rt_xyz"},
	}

	for _, tt := range tests {
		result := GetAuthorizationHeader(tt.auth)
		if result != tt.expected {
			t.Errorf("GetAuthorizationHeader(%v) = %q, want %q", tt.auth, result, tt.expected)
		}
	}
}

func TestConfigFromEnv_Success(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "https://api.strait.dev/")
	t.Setenv("STRAIT_API_KEY", "sk_live_test")
	t.Setenv("STRAIT_AUTH_TYPE", "bearer")
	t.Setenv("STRAIT_TIMEOUT_MS", "5000")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://api.strait.dev" {
		t.Errorf("expected normalized base URL, got %q", cfg.BaseURL)
	}
	if cfg.Auth.Type != AuthTypeBearer {
		t.Errorf("expected bearer auth type, got %q", cfg.Auth.Type)
	}
	if cfg.Auth.Token != "sk_live_test" {
		t.Errorf("expected token 'sk_live_test', got %q", cfg.Auth.Token)
	}
	if cfg.TimeoutMs != 5000 {
		t.Errorf("expected timeout 5000, got %d", cfg.TimeoutMs)
	}
}

func TestConfigFromEnv_DefaultAuthType(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "https://api.strait.dev")
	t.Setenv("STRAIT_API_KEY", "sk_live_test")
	t.Setenv("STRAIT_AUTH_TYPE", "")
	t.Setenv("STRAIT_TIMEOUT_MS", "")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Type != AuthTypeAPIKey {
		t.Errorf("expected default auth type apiKey, got %q", cfg.Auth.Type)
	}
	if cfg.TimeoutMs != 30000 {
		t.Errorf("expected default timeout 30000, got %d", cfg.TimeoutMs)
	}
}

func TestConfigFromEnv_MissingBaseURL(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "")
	t.Setenv("STRAIT_API_KEY", "sk_live_test")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Error("expected ValidationError")
	}
}

func TestConfigFromEnv_MissingAPIKey(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "https://api.strait.dev")
	t.Setenv("STRAIT_API_KEY", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Error("expected ValidationError")
	}
}

func TestConfigFromEnv_InvalidTimeout(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "https://api.strait.dev")
	t.Setenv("STRAIT_API_KEY", "sk_live_test")
	t.Setenv("STRAIT_TIMEOUT_MS", "not-a-number")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Error("expected ValidationError")
	}
}
