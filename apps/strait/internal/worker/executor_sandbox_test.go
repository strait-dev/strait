package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestDispatchSandbox_NoClient(t *testing.T) {
	t.Parallel()
	e := &Executor{
		sandboxClient: nil,
	}

	job := &domain.Job{
		ID:              "job-1",
		ExecutionMode:   domain.ExecutionModeSandbox,
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
	if !errors.Is(err, errSandboxNotConfigured) {
		t.Errorf("expected errSandboxNotConfigured, got: %v", err)
	}
}

func TestExecutionModeRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		job             domain.Job
		wantMode        domain.ExecutionMode
		wantLang        string
		wantEndpointURL string
	}{
		{
			name: "sandbox mode",
			job: domain.Job{
				ID:              "job-sandbox",
				ExecutionMode:   domain.ExecutionModeSandbox,
				SandboxLanguage: "python",
				SandboxCode:     "print('test')",
			},
			wantMode: domain.ExecutionModeSandbox,
			wantLang: "python",
		},
		{
			name: "http mode",
			job: domain.Job{
				ID:            "job-http",
				ExecutionMode: domain.ExecutionModeHTTP,
				EndpointURL:   "https://example.com/webhook",
			},
			wantMode:        domain.ExecutionModeHTTP,
			wantEndpointURL: "https://example.com/webhook",
		},
		{
			name: "empty execution mode defaults to empty string",
			job: domain.Job{
				ID:          "job-default",
				EndpointURL: "https://example.com/run",
			},
			wantMode:        "",
			wantEndpointURL: "https://example.com/run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.job.ExecutionMode != tt.wantMode {
				t.Errorf("execution mode: got %q, want %q", tt.job.ExecutionMode, tt.wantMode)
			}
			if tt.wantLang != "" && tt.job.SandboxLanguage != tt.wantLang {
				t.Errorf("sandbox language: got %q, want %q", tt.job.SandboxLanguage, tt.wantLang)
			}
			if tt.wantEndpointURL != "" && tt.job.EndpointURL != tt.wantEndpointURL {
				t.Errorf("endpoint URL: got %q, want %q", tt.job.EndpointURL, tt.wantEndpointURL)
			}
		})
	}
}

func TestJobJSON_SandboxFields_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		job  domain.Job
	}{
		{
			name: "sandbox job with all fields",
			job: domain.Job{
				ID:              "job-1",
				ProjectID:       "proj-1",
				Name:            "Sandbox Job",
				Slug:            "sandbox-job",
				ExecutionMode:   domain.ExecutionModeSandbox,
				SandboxLanguage: "python",
				SandboxCode:     "import os\nprint(os.environ.get('FORGE_PAYLOAD', ''))",
				MaxAttempts:     3,
				TimeoutSecs:     60,
				Enabled:         true,
				CreatedAt:       time.Now().Truncate(time.Second),
				UpdatedAt:       time.Now().Truncate(time.Second),
			},
		},
		{
			name: "http job omits sandbox fields",
			job: domain.Job{
				ID:            "job-2",
				ExecutionMode: domain.ExecutionModeHTTP,
				EndpointURL:   "https://example.com/run",
				CreatedAt:     time.Now().Truncate(time.Second),
				UpdatedAt:     time.Now().Truncate(time.Second),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.job)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var decoded domain.Job
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if decoded.ExecutionMode != tt.job.ExecutionMode {
				t.Errorf("ExecutionMode: got %q, want %q", decoded.ExecutionMode, tt.job.ExecutionMode)
			}
			if decoded.SandboxLanguage != tt.job.SandboxLanguage {
				t.Errorf("SandboxLanguage: got %q, want %q", decoded.SandboxLanguage, tt.job.SandboxLanguage)
			}
			if decoded.SandboxCode != tt.job.SandboxCode {
				t.Errorf("SandboxCode: got %q, want %q", decoded.SandboxCode, tt.job.SandboxCode)
			}
		})
	}
}
