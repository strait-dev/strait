package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestDispatchSandbox_NoClient(t *testing.T) {
	e := &Executor{
		sandboxClient: nil,
	}

	job := &domain.Job{
		ID:              "job-1",
		ExecutionMode:   "sandbox",
		SandboxLanguage: "python",
		SandboxCode:     "print('hello')",
		TimeoutSecs:     30,
	}
	run := &domain.JobRun{
		ID:    "run-1",
		JobID: "job-1",
	}

	_, _, err := e.dispatchSandbox(context.Background(), job, run)
	if err == nil {
		t.Fatal("expected error when sandbox client is nil")
	}
	if err.Error() != "sandbox client not configured" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecutionModeRouting(t *testing.T) {
	// Verify that the Job struct correctly holds the new fields
	job := domain.Job{
		ID:              "job-sandbox",
		ExecutionMode:   "sandbox",
		SandboxLanguage: "python",
		SandboxCode:     "print('test')",
		EndpointURL:     "",
	}

	if job.ExecutionMode != "sandbox" {
		t.Errorf("expected sandbox mode, got %s", job.ExecutionMode)
	}
	if job.SandboxLanguage != "python" {
		t.Errorf("expected python language, got %s", job.SandboxLanguage)
	}

	// HTTP mode job
	httpJob := domain.Job{
		ID:            "job-http",
		ExecutionMode: "http",
		EndpointURL:   "https://example.com/webhook",
	}

	if httpJob.ExecutionMode != "http" {
		t.Errorf("expected http mode, got %s", httpJob.ExecutionMode)
	}
}

func TestJobJSON_WithSandboxFields(t *testing.T) {
	job := domain.Job{
		ID:              "job-1",
		ProjectID:       "proj-1",
		Name:            "Sandbox Job",
		Slug:            "sandbox-job",
		ExecutionMode:   "sandbox",
		SandboxLanguage: "python",
		SandboxCode:     "import os\nprint(os.environ.get('FORGE_PAYLOAD', ''))",
		MaxAttempts:     3,
		TimeoutSecs:     60,
		Enabled:         true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded domain.Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ExecutionMode != "sandbox" {
		t.Errorf("expected sandbox, got %s", decoded.ExecutionMode)
	}
	if decoded.SandboxLanguage != "python" {
		t.Errorf("expected python, got %s", decoded.SandboxLanguage)
	}
	if decoded.SandboxCode != job.SandboxCode {
		t.Errorf("code mismatch")
	}
}

func TestJobJSON_HTTPMode_NoSandboxFields(t *testing.T) {
	job := domain.Job{
		ID:            "job-2",
		ExecutionMode: "http",
		EndpointURL:   "https://example.com/run",
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded domain.Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.SandboxCode != "" {
		t.Errorf("expected empty sandbox_code for http job, got %s", decoded.SandboxCode)
	}
	if decoded.SandboxLanguage != "" {
		t.Errorf("expected empty sandbox_language for http job, got %s", decoded.SandboxLanguage)
	}
}
