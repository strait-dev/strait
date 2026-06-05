package api

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestPrepareTriggerRequestBuildsState(t *testing.T) {
	t.Parallel()

	quota := &store.ProjectQuota{ProjectID: "project-1", Timezone: "UTC"}
	srv := newTestServer(t, &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			if jobID != "job-1" {
				t.Fatalf("jobID = %q, want job-1", jobID)
			}
			if key != "idem-1" {
				t.Fatalf("key = %q, want idem-1", key)
			}
			return nil, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			if projectID != "project-1" {
				t.Fatalf("projectID = %q, want project-1", projectID)
			}
			return quota, nil
		},
	}, &mockQueue{}, nil)
	job := &domain.Job{
		ID:        "job-1",
		ProjectID: "project-1",
		PayloadSchema: json.RawMessage(
			`{"type":"object","required":["a"],"properties":{"a":{"type":"number"},"b":{"type":"number"}}}`,
		),
	}
	req := TriggerRequest{
		Payload:  json.RawMessage(`{"b":2,"a":1}`),
		Priority: 4,
		Tags:     map[string]string{"source": "test"},
	}

	state, idempotencyHit, err := srv.prepareTriggerRequest(context.Background(), &TriggerJobInput{
		XIdempotencyKey: "idem-1",
	}, job, req)
	if err != nil {
		t.Fatalf("prepareTriggerRequest() error = %v", err)
	}
	if idempotencyHit != nil {
		t.Fatalf("idempotencyHit = %+v, want nil", idempotencyHit)
	}
	if state.job != job {
		t.Fatalf("state.job = %p, want %p", state.job, job)
	}
	if state.idempotencyKey != "idem-1" {
		t.Fatalf("idempotencyKey = %q, want idem-1", state.idempotencyKey)
	}
	if string(state.payload) != `{"a":1,"b":2}` {
		t.Fatalf("payload = %s, want canonical JSON", state.payload)
	}
	if state.payloadHash == "" {
		t.Fatal("payloadHash is empty")
	}
	if state.projectQuota == nil {
		t.Fatal("projectQuota is nil")
	}
	if state.projectQuota.ProjectID != quota.ProjectID || state.projectQuota.Timezone != quota.Timezone {
		t.Fatalf("projectQuota = %+v, want %+v", state.projectQuota, quota)
	}
}

func TestPrepareTriggerRequestReturnsIdempotencyHitBeforeQuota(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			if jobID != "job-1" {
				t.Fatalf("jobID = %q, want job-1", jobID)
			}
			if key != "idem-hit" {
				t.Fatalf("key = %q, want idem-hit", key)
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
		GetProjectQuotaFunc: func(context.Context, string) (*store.ProjectQuota, error) {
			t.Fatal("GetProjectQuota must not run when idempotency hits")
			return nil, nil
		},
	}, &mockQueue{}, nil)

	state, idempotencyHit, err := srv.prepareTriggerRequest(context.Background(), &TriggerJobInput{
		XIdempotencyKey: "idem-hit",
	}, &domain.Job{ID: "job-1", ProjectID: "project-1"}, TriggerRequest{Payload: json.RawMessage(`{"ok":true}`)})
	if err != nil {
		t.Fatalf("prepareTriggerRequest() error = %v", err)
	}
	if state != nil {
		t.Fatalf("state = %+v, want nil", state)
	}
	assertIdempotencyResponse(t, idempotencyHit, "run-existing", domain.StatusQueued)
}
