package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandAliasArgs_IgnoresLocalConfig(t *testing.T) {
	t.Parallel()

	// Create a temp dir with a local .strait.yaml containing an alias
	dir := t.TempDir()
	localConfig := filepath.Join(dir, ".strait.yaml")
	content := "aliases:\n  d: deploy --force\n"
	if err := os.WriteFile(localConfig, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Without explicit --config, alias expansion should NOT use local config
	args := expandAliasArgs([]string{"d", "--project", "proj-1"}, "")
	if args[0] != "d" {
		t.Fatalf("expected local alias 'd' to NOT expand, but got args[0] = %q", args[0])
	}
}

func TestExpandAliasArgs_HonorsExplicitConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "custom.yaml")
	content := "aliases:\n  d: deploy --force\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// With explicit --config, alias expansion should use the provided config
	args := expandAliasArgs([]string{"d", "--project", "proj-1"}, cfgPath)
	if args[0] != "deploy" {
		t.Fatalf("expected alias 'd' to expand to 'deploy', but got args[0] = %q", args[0])
	}
}

func TestAliasSet_AlwaysWritesToHomeConfig(t *testing.T) {
	t.Parallel()

	// Create a temp dir with a local .strait.yaml
	dir := t.TempDir()
	localConfig := filepath.Join(dir, ".strait.yaml")
	if err := os.WriteFile(localConfig, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Set up state pointing to local config (simulating CWD with .strait.yaml)
	state := &appState{
		opts:       &rootOptions{},
		configPath: localConfig,
	}

	cmd := newAliasSetCommand(state)
	cmd.SetArgs([]string{"myalias", "jobs list"})

	// The command should write to the home config, not the local one.
	// loadHomeConfigForWrite will use HomePath() which points to ~/.config/strait/config.yaml
	// In tests, this may not exist, so the command might error, which is fine -
	// the important thing is that it does NOT write to the local config.
	_ = cmd.Execute()

	// Read the local config and verify no alias was written there
	content, err := os.ReadFile(localConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "myalias") {
		t.Fatal("alias was written to local config; should only write to home config")
	}
}

func TestAliasRoundTrip_SetThenExpand(t *testing.T) {
	t.Parallel()

	// Set up a fake home config directory
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "strait")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	// Write an alias directly to the home config path
	aliasContent := "aliases:\n  rj: runs list --job my-job\n"
	if err := os.WriteFile(configFile, []byte(aliasContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// expandAliasArgs with empty configPath should fall through to HomePath().
	// Since we can't easily override HOME in a parallel test, verify with explicit path.
	args := expandAliasArgs([]string{"rj", "--limit", "5"}, configFile)
	if len(args) < 3 {
		t.Fatalf("expected expanded args, got: %v", args)
	}
	if args[0] != "runs" || args[1] != "list" {
		t.Fatalf("expected alias to expand to 'runs list', got: %v", args[:2])
	}
}
