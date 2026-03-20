package main

import (
	"os"
	"path/filepath"
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
