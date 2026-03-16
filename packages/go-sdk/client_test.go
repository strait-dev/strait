package strait

import (
	"errors"
	"net/http"
	"testing"
)

func TestNewClient_WithBaseURL(t *testing.T) {
	client := NewClient(WithBaseURL("https://api.strait.dev/"))
	if client.BaseURL() != "https://api.strait.dev" {
		t.Errorf("expected normalized URL, got %q", client.BaseURL())
	}
}

func TestNewClient_WithBearerToken(t *testing.T) {
	client := NewClient(WithBearerToken("tok_123"))
	if client.config.Auth.Type != AuthTypeBearer {
		t.Errorf("expected bearer auth type, got %q", client.config.Auth.Type)
	}
	if client.config.Auth.Token != "tok_123" {
		t.Errorf("expected token 'tok_123', got %q", client.config.Auth.Token)
	}
}

func TestNewClient_WithAPIKey(t *testing.T) {
	client := NewClient(WithAPIKey("sk_live_abc"))
	if client.config.Auth.Type != AuthTypeAPIKey {
		t.Errorf("expected apiKey auth type, got %q", client.config.Auth.Type)
	}
}

func TestNewClient_WithRunToken(t *testing.T) {
	client := NewClient(WithRunToken("rt_xyz"))
	if client.config.Auth.Type != AuthTypeRunToken {
		t.Errorf("expected runToken auth type, got %q", client.config.Auth.Type)
	}
}

func TestNewClient_WithAuth(t *testing.T) {
	auth := AuthMode{Type: AuthTypeBearer, Token: "custom"}
	client := NewClient(WithAuth(auth))
	if client.config.Auth != auth {
		t.Error("expected auth mode to be set")
	}
}

func TestNewClient_WithDefaultHeaders(t *testing.T) {
	headers := map[string]string{"X-Org": "test"}
	client := NewClient(WithDefaultHeaders(headers))
	if client.config.DefaultHeaders["X-Org"] != "test" {
		t.Error("expected default headers to be set")
	}
}

func TestNewClient_WithTimeout(t *testing.T) {
	client := NewClient(WithTimeout(5000))
	if client.config.TimeoutMs != 5000 {
		t.Errorf("expected timeout 5000, got %d", client.config.TimeoutMs)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	client := NewClient()
	if client.config.TimeoutMs != 30000 {
		t.Errorf("expected default timeout 30000, got %d", client.config.TimeoutMs)
	}
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	doer := &mockDoer{}
	client := NewClient(WithHTTPClient(doer))
	if client.httpClient != doer {
		t.Error("expected custom HTTP client to be set")
	}
}

type mockDoer struct{}

func (m *mockDoer) Do(_ *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestNewClientFromEnv_Success(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "https://api.strait.dev")
	t.Setenv("STRAIT_API_KEY", "sk_live_test")
	t.Setenv("STRAIT_AUTH_TYPE", "")
	t.Setenv("STRAIT_TIMEOUT_MS", "")

	client, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.BaseURL() != "https://api.strait.dev" {
		t.Errorf("expected base URL, got %q", client.BaseURL())
	}
	if client.config.Auth.Token != "sk_live_test" {
		t.Errorf("expected token, got %q", client.config.Auth.Token)
	}
}

func TestNewClientFromEnv_WithOverrides(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "https://api.strait.dev")
	t.Setenv("STRAIT_API_KEY", "sk_live_test")

	client, err := NewClientFromEnv(WithTimeout(1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.config.TimeoutMs != 1000 {
		t.Errorf("expected timeout override 1000, got %d", client.config.TimeoutMs)
	}
}

func TestNewClientFromEnv_Error(t *testing.T) {
	t.Setenv("STRAIT_BASE_URL", "")
	t.Setenv("STRAIT_API_KEY", "")

	_, err := NewClientFromEnv()
	if err == nil {
		t.Fatal("expected error for missing env vars")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Error("expected ValidationError")
	}
}
