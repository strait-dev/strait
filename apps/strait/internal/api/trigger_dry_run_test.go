package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

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
