package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"strait/internal/cli"
)

func TestConfigRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	t.Setenv("STRAIT_CONFIG", path)

	cfg := &cli.Config{
		ActiveProfile: "default",
		Profiles: map[string]*cli.Profile{
			"default": {
				APIURL:    "https://api.example.com",
				APIKey:    "sk_test_abc",
				ProjectID: "proj_123",
			},
		},
	}

	if err := cli.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := cli.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.ActiveProfile != "default" {
		t.Errorf("active_profile: got %q want %q", loaded.ActiveProfile, "default")
	}
	p := loaded.Profiles["default"]
	if p == nil {
		t.Fatal("profile 'default' missing after round-trip")
	}
	if p.APIKey != "sk_test_abc" {
		t.Errorf("api_key: got %q want %q", p.APIKey, "sk_test_abc")
	}
	if p.ProjectID != "proj_123" {
		t.Errorf("project_id: got %q want %q", p.ProjectID, "proj_123")
	}
}

func TestLoadConfig_MissingFileReturnsEmpty(t *testing.T) {
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "nonexistent.json"))

	cfg, err := cli.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("expected empty profiles, got %v", cfg.Profiles)
	}
}

func TestActiveProfileData_EnvOverrides(t *testing.T) {
	t.Setenv("STRAIT_API_KEY", "env_key")
	t.Setenv("STRAIT_API_URL", "https://env.example.com")
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	cfg, _ := cli.LoadConfig()
	p := cfg.ActiveProfileData("")
	if p.APIKey != "env_key" {
		t.Errorf("expected env key, got %q", p.APIKey)
	}
	if p.APIURL != "https://env.example.com" {
		t.Errorf("expected env url, got %q", p.APIURL)
	}
}

func TestActiveProfileData_DefaultURL(t *testing.T) {
	t.Setenv("STRAIT_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	// Ensure no override env is set
	_ = os.Unsetenv("STRAIT_API_URL")
	_ = os.Unsetenv("STRAIT_API_KEY")

	cfg, _ := cli.LoadConfig()
	p := cfg.ActiveProfileData("")
	if p.APIURL != cli.DefaultAPIURL {
		t.Errorf("expected default URL %q, got %q", cli.DefaultAPIURL, p.APIURL)
	}
}

func TestSetProfile_FirstProfileBecomesActive(t *testing.T) {
	cfg := &cli.Config{}
	cfg.SetProfile("staging", &cli.Profile{APIURL: "https://staging.example.com", APIKey: "key1"})
	if cfg.ActiveProfile != "staging" {
		t.Errorf("expected active_profile=staging, got %q", cfg.ActiveProfile)
	}
}

func TestRemoveProfile_ClearsActiveIfMatch(t *testing.T) {
	cfg := &cli.Config{
		ActiveProfile: "default",
		Profiles: map[string]*cli.Profile{
			"default": {APIKey: "k"},
		},
	}
	cfg.RemoveProfile("default")
	if cfg.ActiveProfile != "" {
		t.Errorf("expected active_profile cleared, got %q", cfg.ActiveProfile)
	}
}

func TestSaveConfig_CreatesParentDirs(t *testing.T) {
	deep := filepath.Join(t.TempDir(), "a", "b", "c", "config.json")
	t.Setenv("STRAIT_CONFIG", deep)

	cfg := &cli.Config{ActiveProfile: "default", Profiles: map[string]*cli.Profile{
		"default": {APIKey: "x"},
	}}
	if err := cli.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if _, err := os.Stat(deep); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}
