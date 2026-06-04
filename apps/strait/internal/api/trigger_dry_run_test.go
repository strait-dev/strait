package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

func TestHandleTriggerDryRunReturnsValidationResult(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			if jobID != "job-1" {
				t.Fatalf("jobID = %q, want job-1", jobID)
			}
			return &domain.Job{
				ID:          jobID,
				ProjectID:   "project-1",
				Name:        "Export",
				Slug:        "export",
				Enabled:     true,
				TimeoutSecs: 60,
				MaxAttempts: 2,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			if projectID != "project-1" {
				t.Fatalf("projectID = %q, want project-1", projectID)
			}
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
	}, &mockQueue{}, nil)

	out, err := srv.handleTriggerDryRun(context.Background(), "job-1", TriggerRequest{
		Payload: json.RawMessage(`{"b":2,"a":1}`),
	})
	if out != nil {
		t.Fatalf("output = %+v, want nil raw-status output", out)
	}
	var rawErr *rawStatusError
	if !errors.As(err, &rawErr) {
		t.Fatalf("error = %T, want rawStatusError", err)
	}
	if rawErr.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", rawErr.status)
	}
	result, ok := rawErr.body.(*DryRunValidationResult)
	if !ok {
		t.Fatalf("body = %T, want *DryRunValidationResult", rawErr.body)
	}
	if result.Job == nil || result.Job.ID != "job-1" {
		t.Fatalf("result.Job = %+v, want job-1", result.Job)
	}
	if string(result.Payload) != `{"a":1,"b":2}` {
		t.Fatalf("payload = %s, want canonical JSON", result.Payload)
	}
	if result.PayloadHash == "" {
		t.Fatal("payload hash is empty")
	}
}

func TestHandleTriggerDryRunMapsValidationErrorToBadRequest(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			return &domain.Job{ID: jobID, ProjectID: "project-1", Enabled: false, TimeoutSecs: 60}, nil
		},
	}, &mockQueue{}, nil)

	_, err := srv.handleTriggerDryRun(context.Background(), "job-1", TriggerRequest{})
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %T, want huma.StatusError", err)
	}
	if statusErr.GetStatus() != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", statusErr.GetStatus())
	}
	if !strings.Contains(err.Error(), "job is disabled") {
		t.Fatalf("error = %q, want disabled job message", err.Error())
	}
}

func TestDryRunValidationWarningsReportsDedupRun(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"customer":"acme"}`)
	job := &domain.Job{ID: "job-1", DedupWindowSecs: 60}
	ms := &APIStoreMock{
		FindRecentRunByPayloadFunc: func(_ context.Context, jobID string, gotPayload json.RawMessage, since time.Time) (*domain.JobRun, error) {
			if jobID != job.ID {
				t.Fatalf("jobID = %q, want %q", jobID, job.ID)
			}
			if string(gotPayload) != string(payload) {
				t.Fatalf("payload = %s, want %s", gotPayload, payload)
			}
			if since.IsZero() {
				t.Fatal("since must be set for dedup lookup")
			}
			return &domain.JobRun{ID: "run-existing"}, nil
		},
	}
	srv := &Server{store: ms}

	warnings, err := srv.dryRunValidationWarnings(context.Background(), job, payload)
	if err != nil {
		t.Fatalf("dryRunValidationWarnings: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "run-existing") {
		t.Fatalf("warnings = %#v, want dedup warning with run ID", warnings)
	}
}

func TestDryRunValidationWarningsSkipsDedupWhenDisabled(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			t.Fatal("dedup lookup must not run when dedup window is disabled")
			return nil, nil
		},
	}}

	warnings, err := srv.dryRunValidationWarnings(context.Background(), &domain.Job{ID: "job-1"}, nil)
	if err != nil {
		t.Fatalf("dryRunValidationWarnings: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
}

func TestDryRunJobInfoNilSafe(t *testing.T) {
	t.Parallel()

	if got := dryRunJobInfo(nil); got != nil {
		t.Fatalf("dryRunJobInfo(nil) = %#v, want nil", got)
	}
}
