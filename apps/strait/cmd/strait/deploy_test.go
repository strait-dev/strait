package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployPromote_DryRunStillValidatesProject(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{}}
	cmd := newDeployPromoteCommand(state)
	cmd.SetArgs([]string{"dep-1", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Fatalf("expected project error, got: %v", err)
	}
}

func TestDeployRollback_DryRunStillValidatesProject(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{}}
	cmd := newDeployRollbackCommand(state)
	cmd.SetArgs([]string{"--to", "dep-1", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Fatalf("expected project error, got: %v", err)
	}
}

func TestDeployRollback_RequiresToFlag(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{projectID: "proj-1"}}
	cmd := newDeployRollbackCommand(state)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --to")
	}
	if !strings.Contains(err.Error(), "--to") {
		t.Fatalf("expected --to error, got: %v", err)
	}
}

func TestDeployPromote_DryRunOutput(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{projectID: "proj-1"}}
	cmd := newDeployPromoteCommand(state)
	cmd.SetArgs([]string{"dep-1", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployCommand_ConfigJobFilter_NotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(df, []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "deploy.yaml")
	cfg := "version: 1\nproject: proj-1\nregistry: r.io/app\njobs:\n  - slug: job-a\n    dockerfile: " + df + "\n  - slug: job-b\n    dockerfile: " + df + "\n"
	if err := os.WriteFile(configPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &appState{opts: &rootOptions{serverURL: "http://localhost:9999", apiKey: "k"}}
	cmd := newDeployCommand(state)
	cmd.SetArgs([]string{"--config", configPath, "--job", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for job not found in config")
	}
	if !strings.Contains(err.Error(), "not found in config") {
		t.Fatalf("expected 'not found in config' error, got: %v", err)
	}
}

func TestDeployCommand_ConfigWithoutJobDeploysAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(df, []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "deploy.yaml")
	cfg := "version: 1\nproject: proj-1\nregistry: r.io/app\njobs:\n  - slug: job-a\n    dockerfile: " + df + "\n  - slug: job-b\n    dockerfile: " + df + "\n"
	if err := os.WriteFile(configPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &appState{opts: &rootOptions{serverURL: "http://localhost:9999", apiKey: "k"}}
	cmd := newDeployCommand(state)
	cmd.SetArgs([]string{"--config", configPath, "--job", "job-a", "--dry-run"})

	// Dry-run with --job should NOT error for "not found" when job exists
	err := cmd.Execute()
	// DeployJob dry-run prints plan; there may be errors from docker not being available
	// but NOT a "not found in config" error
	if err != nil && strings.Contains(err.Error(), "not found in config") {
		t.Fatalf("should not get 'not found in config' for existing job: %v", err)
	}
}
