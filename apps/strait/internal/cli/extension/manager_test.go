package extension

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtensionsDir_ContainsExtensions(t *testing.T) {
	t.Parallel()

	dir := ExtensionsDir()
	if !strings.Contains(dir, "extensions") {
		t.Errorf("expected extensions dir to contain 'extensions', got %q", dir)
	}
	if !strings.Contains(dir, "strait") {
		t.Errorf("expected extensions dir to contain 'strait', got %q", dir)
	}
}

func TestListInstalled_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	plugins, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestListInstalled_WithPlugins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create two plugin directories with manifests.
	for _, name := range []string{"alpha", "beta"} {
		pluginDir := filepath.Join(dir, name)
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := PluginManifest{
			Name:     name,
			Version:  "1.0.0",
			Commands: []string{name + "-cmd"},
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(pluginDir, "strait-plugin.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	plugins, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}

	names := map[string]bool{}
	for _, p := range plugins {
		names[p.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta plugins, got %v", plugins)
	}
}

func TestRemove_ExistingPlugin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "my-ext")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Remove(dir, "my-ext")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Error("expected plugin directory to be removed")
	}
}

func TestRemove_NotInstalled_Errors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := Remove(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-installed extension, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected 'not installed' in error, got: %v", err)
	}
}

func TestInstall_InvalidSource_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{name: "empty string", source: ""},
		{name: "random string", source: "foobar"},
		{name: "http non-github", source: "https://example.com/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Install(context.Background(), tt.source)
			if err == nil {
				t.Fatalf("expected error for source %q, got nil", tt.source)
			}
		})
	}
}

func TestInstall_ValidSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{name: "github URL", source: "https://github.com/user/repo"},
		{name: "github without https", source: "github.com/user/repo"},
		{name: "absolute path", source: "/tmp/my-plugin"},
		{name: "relative dot path", source: "./my-plugin"},
		{name: "relative parent path", source: "../my-plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Install(context.Background(), tt.source)
			if err != nil {
				t.Fatalf("unexpected error for source %q: %v", tt.source, err)
			}
		})
	}
}
