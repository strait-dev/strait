package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPathOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "custom.yaml")
	content := []byte("server: https://custom.example\nactive_context: staging\n")
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	res, err := Load(p)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !res.Exists {
		t.Fatalf("expected config to exist")
	}
	if res.Data.ServerURL != "https://custom.example" {
		t.Fatalf("server = %q", res.Data.ServerURL)
	}
}

func TestResolvePrecedence(t *testing.T) {
	t.Parallel()

	resolved := Resolve(ResolveInput{
		Flags: map[string]string{
			"server":  "https://flag.example",
			"project": "proj-flag",
			"format":  "json",
		},
		BoolFlags: map[string]bool{
			"no-color": true,
		},
		DurationFlags: map[string]string{
			"timeout": "15s",
		},
		Changed: map[string]bool{
			"server":   true,
			"project":  true,
			"format":   true,
			"timeout":  true,
			"no-color": true,
		},
		Config: &File{
			ServerURL:      "https://cfg.example",
			DefaultProject: "proj-cfg",
			OutputFormat:   "yaml",
			ActiveContext:  "prod",
			Contexts: map[string]Context{
				"prod": {
					Server:  "https://ctx.example",
					Project: "proj-ctx",
					Format:  "csv",
				},
			},
		},
		Env: map[string]string{
			"ORCHESTRATOR_SERVER":  "https://env.example",
			"ORCHESTRATOR_PROJECT": "proj-env",
			"ORCHESTRATOR_FORMAT":  "table",
			"ORCHESTRATOR_TIMEOUT": "60s",
		},
	})

	if resolved.ServerURL != "https://flag.example" {
		t.Fatalf("server precedence mismatch: %q", resolved.ServerURL)
	}
	if resolved.ProjectID != "proj-flag" {
		t.Fatalf("project precedence mismatch: %q", resolved.ProjectID)
	}
	if resolved.Format != "json" {
		t.Fatalf("format precedence mismatch: %q", resolved.Format)
	}
	if resolved.Timeout != "15s" {
		t.Fatalf("timeout precedence mismatch: %q", resolved.Timeout)
	}
	if !resolved.NoColor {
		t.Fatalf("expected no-color true")
	}
}
