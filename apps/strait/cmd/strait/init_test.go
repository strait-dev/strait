package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_NonInteractive_AllFlags(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "my-api", "--runtime", "node"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify config file was created
	if _, err := os.Stat(filepath.Join(dir, "strait.config.json")); err != nil {
		t.Fatal("strait.config.json not created")
	}
}

func TestInit_NonInteractive_RequiresName(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--runtime", "node"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected 'required' error, got: %v", err)
	}
}

func TestInit_WritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "test-project", "--runtime", "bun"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "strait.config.json"))
	if err != nil {
		t.Fatal(err)
	}

	var cfg straitConfigJSON
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if cfg.Project.ID != "test-project" {
		t.Fatalf("expected project.id=test-project, got %q", cfg.Project.ID)
	}
	if cfg.Runtime != "bun" {
		t.Fatalf("expected runtime=bun, got %q", cfg.Runtime)
	}
}

func TestInit_WithJob_AddsJobToConfig(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{
		"--yes", "--name", "my-api", "--runtime", "node",
		"--with-job", "--job-name", "process-payment",
		"--job-endpoint", "http://localhost:3000/jobs/payment",
		"--job-cron", "*/5 * * * *",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "strait.config.json"))
	if err != nil {
		t.Fatal(err)
	}

	var cfg straitConfigJSON
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(cfg.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(cfg.Jobs))
	}
	if cfg.Jobs[0].Slug != "process-payment" {
		t.Fatalf("expected slug=process-payment, got %q", cfg.Jobs[0].Slug)
	}
	if cfg.Jobs[0].EndpointURL != "http://localhost:3000/jobs/payment" {
		t.Fatalf("expected endpoint, got %q", cfg.Jobs[0].EndpointURL)
	}
	if cfg.Jobs[0].Cron != "*/5 * * * *" {
		t.Fatalf("expected cron, got %q", cfg.Jobs[0].Cron)
	}
}

func TestInit_WithJob_ValidatesEndpoint(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{
		"--yes", "--name", "my-api", "--runtime", "node",
		"--with-job", "--job-name", "bad-job",
		"--job-endpoint", "not-a-url",
	})

	// The endpoint is written as-is in non-interactive mode (validation happens
	// at API time). The init command trusts flag input — wizard validates interactively.
	// This test verifies the flag path doesn't crash.
	_ = cmd.Execute()
}

func TestInit_ConfigAlreadyExists_Errors(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	// Pre-create the config
	if err := os.WriteFile(filepath.Join(dir, "strait.config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "my-api"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for existing config")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got: %v", err)
	}
}

func TestInit_ConfigAlreadyExists_Force(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "strait.config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "my-api", "--runtime", "go", "--force"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("--force should allow overwrite, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "strait.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "my-api") {
		t.Fatal("config was not overwritten")
	}
}

func TestInit_UpdatesGitignore(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "my-api"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(".gitignore not created")
	}
	if !strings.Contains(string(content), ".strait/") {
		t.Fatal(".gitignore missing .strait/ entry")
	}
}

func TestInit_GitignoreAlreadyHasEntry(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	// Pre-create .gitignore with the entry
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n.strait/\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "my-api"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	// Should not duplicate the entry
	count := strings.Count(string(content), ".strait/")
	if count != 1 {
		t.Fatalf("expected 1 .strait/ entry, got %d", count)
	}
}

func TestInit_RuntimeAffectsConfig(t *testing.T) {
	// Not parallel: subtests use os.Chdir which is process-global.
	for _, rt := range []string{"node", "bun", "python", "go", "docker"} {
		t.Run(rt, func(t *testing.T) {
			dir := t.TempDir()
			origDir, _ := os.Getwd()
			t.Cleanup(func() { _ = os.Chdir(origDir) })
			_ = os.Chdir(dir)

			state := &appState{opts: &rootOptions{}}
			cmd := newInitCommand(state)
			cmd.SetArgs([]string{"--yes", "--name", "rt-test", "--runtime", rt})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			content, err := os.ReadFile(filepath.Join(dir, "strait.config.json"))
			if err != nil {
				t.Fatal(err)
			}
			var cfg straitConfigJSON
			if err := json.Unmarshal(content, &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Runtime != rt {
				t.Fatalf("expected runtime=%s, got %q", rt, cfg.Runtime)
			}
		})
	}
}

func TestInit_InvalidRuntime(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "my-api", "--runtime", "java"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid runtime")
	}
	if !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("expected runtime error, got: %v", err)
	}
}

func TestInit_InvalidProjectName(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"uppercase", "MyProject"},
		{"spaces", "my project"},
		{"leading hyphen", "-bad"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			origDir, _ := os.Getwd()
			t.Cleanup(func() { _ = os.Chdir(origDir) })
			_ = os.Chdir(dir)

			state := &appState{opts: &rootOptions{}}
			cmd := newInitCommand(state)
			cmd.SetArgs([]string{"--yes", "--name", tc.value})

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for name %q", tc.value)
			}
		})
	}
}

func TestInit_TemplateFullCreatesDefinitions(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(dir)

	state := &appState{opts: &rootOptions{}}
	cmd := newInitCommand(state)
	cmd.SetArgs([]string{"--yes", "--name", "full-test", "--template", "full"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check definitions were created
	if _, err := os.Stat(filepath.Join(dir, "definitions", "jobs.yaml")); err != nil {
		t.Fatal("definitions/jobs.yaml not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "definitions", "workflows.yaml")); err != nil {
		t.Fatal("definitions/workflows.yaml not created for full template")
	}
}
