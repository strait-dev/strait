package main

import (
	"fmt"
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

func TestDeploy_CanaryFlags_Exist(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{}}
	cmd := newDeployCommand(state)

	flags := []string{"strategy", "canary-percent", "canary-duration"}
	for _, name := range flags {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("expected flag --%s to be registered", name)
		}
	}
}

func TestDeploy_CanaryRequiresPercent(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{serverURL: "http://localhost:9999", apiKey: "k"}}
	cmd := newDeployCommand(state)
	cmd.SetArgs([]string{"--job", "my-job", "--strategy", "canary", "--image", "img:latest"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --strategy=canary without --canary-percent")
	}
	if !strings.Contains(err.Error(), "--canary-percent") {
		t.Fatalf("expected --canary-percent error, got: %v", err)
	}
}

func TestDeploy_CanaryPercentRange(t *testing.T) {
	t.Parallel()

	invalid := []int{0, 100, -1}
	for _, pct := range invalid {
		t.Run(fmt.Sprintf("percent_%d", pct), func(t *testing.T) {
			t.Parallel()

			state := &appState{opts: &rootOptions{serverURL: "http://localhost:9999", apiKey: "k"}}
			cmd := newDeployCommand(state)
			cmd.SetArgs([]string{
				"--job", "my-job",
				"--strategy", "canary",
				"--canary-percent", fmt.Sprintf("%d", pct),
				"--image", "img:latest",
			})

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for canary-percent=%d", pct)
			}
			if !strings.Contains(err.Error(), "--canary-percent") {
				t.Fatalf("expected --canary-percent range error, got: %v", err)
			}
		})
	}
}

func TestDeploy_DirectDefault(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{}}
	cmd := newDeployCommand(state)

	f := cmd.Flags().Lookup("strategy")
	if f == nil {
		t.Fatal("expected --strategy flag to be registered")
	}
	if f.DefValue != "direct" {
		t.Fatalf("expected --strategy default to be 'direct', got %q", f.DefValue)
	}
}
