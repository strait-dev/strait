package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_IsLocal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns path override
		isLocal bool
	}{
		{
			name: "home config is not local",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(p, []byte("server: https://home.example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			isLocal: false,
		},
		{
			name: "cwd config is local",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, ".strait.yaml")
				if err := os.WriteFile(p, []byte("server: https://local.example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				// Change to the directory so CWD-based detection kicks in
				origDir, _ := os.Getwd()
				t.Cleanup(func() { _ = os.Chdir(origDir) })
				if err := os.Chdir(dir); err != nil {
					t.Fatal(err)
				}
				return "" // no path override; uses CWD discovery
			},
			isLocal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			override := tt.setup(t)
			res, err := Load(override)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if res.IsLocal != tt.isLocal {
				t.Fatalf("IsLocal = %v, want %v", res.IsLocal, tt.isLocal)
			}
		})
	}
}

func TestHomePath(t *testing.T) {
	t.Parallel()

	p, err := HomePath()
	if err != nil {
		t.Fatalf("HomePath: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "strait", "config.yaml")
	if p != want {
		t.Fatalf("HomePath = %q, want %q", p, want)
	}
}

func TestHasSensitiveLocalFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cfg    *File
		expect []string
	}{
		{name: "nil config", cfg: nil, expect: nil},
		{name: "empty config", cfg: &File{}, expect: nil},
		{name: "server set", cfg: &File{ServerURL: "https://attacker.com"}, expect: []string{"server"}},
		{name: "aliases set", cfg: &File{Aliases: map[string]string{"d": "deploy --force"}}, expect: []string{"aliases"}},
		{
			name: "multiple fields",
			cfg: &File{
				ServerURL:     "https://evil.com",
				Token:         "stolen-key",
				ActiveContext: "production",
				Contexts:      map[string]Context{"prod": {Server: "x"}},
				Aliases:       map[string]string{"x": "y"},
			},
			expect: []string{"server", "api_key", "active_context", "contexts", "aliases"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HasSensitiveLocalFields(tt.cfg)
			if len(got) != len(tt.expect) {
				t.Fatalf("got %v, want %v", got, tt.expect)
			}
			for i := range got {
				if got[i] != tt.expect[i] {
					t.Fatalf("field[%d] = %q, want %q", i, got[i], tt.expect[i])
				}
			}
		})
	}
}

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
			"STRAIT_SERVER":  "https://env.example",
			"STRAIT_PROJECT": "proj-env",
			"STRAIT_FORMAT":  "table",
			"STRAIT_TIMEOUT": "60s",
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
