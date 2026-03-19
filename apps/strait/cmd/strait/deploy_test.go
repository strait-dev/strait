package main

import (
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
