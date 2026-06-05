package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

func TestFindRecentDeduplicatedRunSkipsDisabledWindow(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			t.Fatal("FindRecentRunByPayload must not run when deduplication is disabled")
			return nil, nil
		},
	}}

	run, err := srv.findRecentDeduplicatedRun(context.Background(), &domain.Job{ID: "job-1"}, json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatalf("findRecentDeduplicatedRun() error = %v", err)
	}
	if run != nil {
		t.Fatalf("findRecentDeduplicatedRun() = %+v, want nil", run)
	}
}

func TestFindRecentDeduplicatedRunUsesDedupWindow(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"ok":true}`)
	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(_ context.Context, jobID string, gotPayload json.RawMessage, since time.Time) (*domain.JobRun, error) {
			if jobID != "job-1" {
				t.Fatalf("jobID = %q, want job-1", jobID)
			}
			if string(gotPayload) != string(payload) {
				t.Fatalf("payload = %s, want %s", gotPayload, payload)
			}
			cutoff := time.Now().Add(-60 * time.Second)
			if since.Before(cutoff.Add(-2*time.Second)) || since.After(cutoff.Add(2*time.Second)) {
				t.Fatalf("since = %s, want near %s", since, cutoff)
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}}

	run, err := srv.findRecentDeduplicatedRun(context.Background(), &domain.Job{ID: "job-1", DedupWindowSecs: 60}, payload)
	if err != nil {
		t.Fatalf("findRecentDeduplicatedRun() error = %v", err)
	}
	if run == nil || run.ID != "run-existing" {
		t.Fatalf("findRecentDeduplicatedRun() = %+v, want run-existing", run)
	}
}

func TestTriggerDedupOutputReturnsExistingRunShape(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusExecuting}, nil
		},
	}}
	state := &triggerRequestState{
		job:         &domain.Job{ID: "job-1", DedupWindowSecs: 60},
		payload:     json.RawMessage(`{"ok":true}`),
		payloadHash: "payload-hash",
	}

	out, err := srv.triggerDedupOutput(context.Background(), state)
	if err != nil {
		t.Fatalf("triggerDedupOutput() error = %v", err)
	}
	if out == nil {
		t.Fatal("triggerDedupOutput() = nil, want existing run output")
		return
	}
	body, ok := out.Body.(map[string]any)
	if !ok {
		t.Fatalf("output body = %T, want map[string]any", out.Body)
	}
	if body["id"] != "run-existing" {
		t.Fatalf("id = %v, want run-existing", body["id"])
	}
	if body["status"] != domain.StatusExecuting {
		t.Fatalf("status = %v, want %s", body["status"], domain.StatusExecuting)
	}
	if body["payload_hash"] != "payload-hash" {
		t.Fatalf("payload_hash = %v, want payload-hash", body["payload_hash"])
	}
	if body["idempotency_hit"] != false {
		t.Fatalf("idempotency_hit = %v, want false", body["idempotency_hit"])
	}
}

func TestTriggerDedupOutputMapsLookupError(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			return nil, errors.New("database unavailable")
		},
	}}
	state := &triggerRequestState{
		job:     &domain.Job{ID: "job-1", DedupWindowSecs: 60},
		payload: json.RawMessage(`{"ok":true}`),
	}

	_, err := srv.triggerDedupOutput(context.Background(), state)
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("triggerDedupOutput() = %T, want huma.StatusError", err)
	}
	if statusErr.GetStatus() != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", statusErr.GetStatus())
	}
}
