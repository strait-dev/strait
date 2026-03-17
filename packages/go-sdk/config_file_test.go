package strait

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfigFile(t *testing.T, dir string, content any) string {
	t.Helper()
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigFromFile_FullConfig(t *testing.T) {
	dir := t.TempDir()
	timeout := 5000
	writeTestConfigFile(t, dir, map[string]any{
		"project": map[string]any{
			"id":   "proj_abc123",
			"name": "Test Project",
		},
		"sdk": map[string]any{
			"base_url":   "https://api.strait.dev/",
			"auth_type":  "bearer",
			"timeout_ms": timeout,
		},
	})

	t.Setenv("STRAIT_API_KEY", "sk_test_123")
	t.Setenv("STRAIT_BASE_URL", "")
	t.Setenv("STRAIT_AUTH_TYPE", "")
	t.Setenv("STRAIT_TIMEOUT_MS", "")

	cfg, err := ConfigFromFile(WithConfigDir(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://api.strait.dev" {
		t.Errorf("expected normalized base URL, got %q", cfg.BaseURL)
	}
	if cfg.Auth.Type != AuthTypeBearer {
		t.Errorf("expected bearer auth type, got %q", cfg.Auth.Type)
	}
	if cfg.Auth.Token != "sk_test_123" {
		t.Errorf("expected token from env, got %q", cfg.Auth.Token)
	}
	if cfg.TimeoutMs != 5000 {
		t.Errorf("expected timeout 5000, got %d", cfg.TimeoutMs)
	}
}

func TestConfigFromFile_EnvOverrides(t *testing.T) {
	dir := t.TempDir()
	timeout := 5000
	writeTestConfigFile(t, dir, map[string]any{
		"sdk": map[string]any{
			"base_url":   "https://file.example.com",
			"auth_type":  "apiKey",
			"timeout_ms": timeout,
		},
	})

	t.Setenv("STRAIT_BASE_URL", "https://env.example.com")
	t.Setenv("STRAIT_API_KEY", "env_key")
	t.Setenv("STRAIT_AUTH_TYPE", "bearer")
	t.Setenv("STRAIT_TIMEOUT_MS", "9000")

	cfg, err := ConfigFromFile(WithConfigDir(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://env.example.com" {
		t.Errorf("env var should override base URL, got %q", cfg.BaseURL)
	}
	if cfg.Auth.Type != AuthTypeBearer {
		t.Errorf("env var should override auth type, got %q", cfg.Auth.Type)
	}
	if cfg.Auth.Token != "env_key" {
		t.Errorf("expected env token, got %q", cfg.Auth.Token)
	}
	if cfg.TimeoutMs != 9000 {
		t.Errorf("env var should override timeout, got %d", cfg.TimeoutMs)
	}
}

func TestConfigFromFile_Defaults(t *testing.T) {
	dir := t.TempDir()
	writeTestConfigFile(t, dir, map[string]any{
		"project": map[string]any{"id": "proj_1"},
	})

	t.Setenv("STRAIT_BASE_URL", "")
	t.Setenv("STRAIT_API_KEY", "")
	t.Setenv("STRAIT_AUTH_TYPE", "")
	t.Setenv("STRAIT_TIMEOUT_MS", "")

	cfg, err := ConfigFromFile(WithConfigDir(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != "" {
		t.Errorf("expected empty base URL, got %q", cfg.BaseURL)
	}
	if cfg.Auth.Type != AuthTypeAPIKey {
		t.Errorf("expected default apiKey auth type, got %q", cfg.Auth.Type)
	}
	if cfg.TimeoutMs != 30000 {
		t.Errorf("expected default timeout 30000, got %d", cfg.TimeoutMs)
	}
}

func TestConfigFromFile_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ConfigFromFile(WithConfigDir(dir))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestConfigFromFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ConfigFromFile(WithConfigDir(dir))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConfigFromFile_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	timeout := 7000
	path := filepath.Join(dir, "custom-config.json")
	data, _ := json.Marshal(map[string]any{
		"sdk": map[string]any{
			"base_url":   "https://custom.example.com",
			"timeout_ms": timeout,
		},
	})
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("STRAIT_BASE_URL", "")
	t.Setenv("STRAIT_API_KEY", "key123")
	t.Setenv("STRAIT_AUTH_TYPE", "")
	t.Setenv("STRAIT_TIMEOUT_MS", "")

	cfg, err := ConfigFromFile(WithConfigPath(path))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://custom.example.com" {
		t.Errorf("expected custom base URL, got %q", cfg.BaseURL)
	}
	if cfg.TimeoutMs != 7000 {
		t.Errorf("expected timeout 7000, got %d", cfg.TimeoutMs)
	}
}

func TestConfigFromFile_InvalidTimeoutEnv(t *testing.T) {
	dir := t.TempDir()
	writeTestConfigFile(t, dir, map[string]any{
		"project": map[string]any{"id": "proj_1"},
	})

	t.Setenv("STRAIT_BASE_URL", "")
	t.Setenv("STRAIT_API_KEY", "")
	t.Setenv("STRAIT_TIMEOUT_MS", "not-a-number")

	_, err := ConfigFromFile(WithConfigDir(dir))
	if err == nil {
		t.Fatal("expected error for invalid timeout env var")
	}
}

func TestProjectIDFromFile(t *testing.T) {
	dir := t.TempDir()
	writeTestConfigFile(t, dir, map[string]any{
		"project": map[string]any{
			"id":   "proj_xyz",
			"name": "My Project",
		},
	})

	id, err := ProjectIDFromFile(WithConfigDir(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "proj_xyz" {
		t.Errorf("expected proj_xyz, got %q", id)
	}
}

func TestProjectIDFromFile_NoProject(t *testing.T) {
	dir := t.TempDir()
	writeTestConfigFile(t, dir, map[string]any{
		"sdk": map[string]any{"base_url": "https://api.example.com"},
	})

	id, err := ProjectIDFromFile(WithConfigDir(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestNewClientFromFile(t *testing.T) {
	dir := t.TempDir()
	writeTestConfigFile(t, dir, map[string]any{
		"sdk": map[string]any{
			"base_url":  "https://api.strait.dev/",
			"auth_type": "bearer",
		},
	})

	t.Setenv("STRAIT_API_KEY", "sk_test")
	t.Setenv("STRAIT_BASE_URL", "")
	t.Setenv("STRAIT_AUTH_TYPE", "")
	t.Setenv("STRAIT_TIMEOUT_MS", "")

	client, err := NewClientFromFile([]ConfigFileOption{WithConfigDir(dir)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.BaseURL() != "https://api.strait.dev" {
		t.Errorf("expected normalized base URL, got %q", client.BaseURL())
	}
}
