package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"My Job Name", "my-job-name"},
		{"  spaces  ", "spaces"},
		{"UPPER-CASE", "upper-case"},
		{"special!@#chars", "specialchars"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"trailing-", "trailing"},
		{"123-numbers", "123-numbers"},
		{"", ""},
		{"already-slug", "already-slug"},
	}

	for _, tc := range tests {
		t.Run("input="+tc.input, func(t *testing.T) {
			t.Parallel()
			got := generateSlug(tc.input)
			if got != tc.want {
				t.Fatalf("generateSlug(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCreateJob_JSONModeWithProjectInBody(t *testing.T) {
	t.Parallel()

	// JSON has project_id, no --project flag, no project in state
	state := &appState{opts: &rootOptions{}}
	cmd := newCreateJobCommand(state)
	cmd.SetArgs([]string{"--json"})

	input := `{"project_id":"proj-from-json","name":"test","slug":"test","endpoint_url":"http://example.com"}`
	cmd.SetIn(bytes.NewBufferString(input))

	err := cmd.Execute()
	// Should NOT fail with "project ID is required" since JSON body has it.
	// It will fail with a network error trying to reach the API, which is expected.
	if err != nil && strings.Contains(err.Error(), "project ID is required") {
		t.Fatalf("should not require --project when JSON body has project_id: %v", err)
	}
}

func TestCreateJob_JSONModeWithoutProject(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{}}
	cmd := newCreateJobCommand(state)
	cmd.SetArgs([]string{"--json"})

	input := `{"name":"test","slug":"test","endpoint_url":"http://example.com"}`
	cmd.SetIn(bytes.NewBufferString(input))

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Fatalf("expected 'project ID is required' error, got: %v", err)
	}
}

func TestCreateJob_JSONModeFallsBackToFlag(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{projectID: "proj-from-flag"}}
	cmd := newCreateJobCommand(state)
	cmd.SetArgs([]string{"--json"})

	input := `{"name":"test","slug":"test","endpoint_url":"http://example.com"}`
	cmd.SetIn(bytes.NewBufferString(input))

	err := cmd.Execute()
	// Should NOT fail with "project ID is required" since state has project.
	// It will fail with a network error, which is expected.
	if err != nil && strings.Contains(err.Error(), "project ID is required") {
		t.Fatalf("should use project from state when JSON body lacks it: %v", err)
	}
}

func TestCreateWorkflow_JSONModeWithProjectInBody(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{}}
	cmd := newCreateWorkflowCommand(state)
	cmd.SetArgs([]string{"--json"})

	input := `{"project_id":"proj-from-json","name":"test","slug":"test"}`
	cmd.SetIn(bytes.NewBufferString(input))

	err := cmd.Execute()
	if err != nil && strings.Contains(err.Error(), "project ID is required") {
		t.Fatalf("should not require --project when JSON body has project_id: %v", err)
	}
}

func TestCreateWorkflow_JSONModeWithoutProject(t *testing.T) {
	t.Parallel()

	state := &appState{opts: &rootOptions{}}
	cmd := newCreateWorkflowCommand(state)
	cmd.SetArgs([]string{"--json"})

	input := `{"name":"test","slug":"test"}`
	cmd.SetIn(bytes.NewBufferString(input))

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Fatalf("expected 'project ID is required' error, got: %v", err)
	}
}
