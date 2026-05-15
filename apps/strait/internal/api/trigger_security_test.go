package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

func waitForAuditEvent(t *testing.T, ch <-chan *domain.AuditEvent) *domain.AuditEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for audit event")
		return nil
	}
}

func TestTriggerJob_DryRunRejectsPastScheduledAt(t *testing.T) {
	t.Parallel()

	job := testEnabledJob("job-1")
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, job.ProjectID)
	past := time.Now().Add(-time.Minute)

	_, err := srv.handleTriggerJob(ctx, &TriggerJobInput{
		JobID: job.ID,
		Body: TriggerRequest{
			DryRun:      true,
			ScheduledAt: &past,
		},
	})
	if err == nil {
		t.Fatal("expected dry-run trigger with past scheduled_at to fail")
	}
	if !strings.Contains(err.Error(), "scheduled_at must not be in the past") {
		t.Fatalf("error = %q, want scheduled_at validation", err.Error())
	}
}

func TestTriggerJob_DebounceSuccessEmitsAuditEvent(t *testing.T) {
	auditCh := make(chan *domain.AuditEvent, 1)
	job := testEnabledJob("job-debounce")
	job.DebounceWindowSecs = 60

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		UpsertDebouncePendingFunc: func(context.Context, *domain.DebouncePending) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			auditCh <- ev
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, job.ProjectID)

	out, err := srv.handleTriggerJob(ctx, &TriggerJobInput{
		JobID: job.ID,
		Body:  TriggerRequest{DebounceKey: "customer-1"},
	})
	if err != nil {
		t.Fatalf("handleTriggerJob: %v", err)
	}
	if out == nil {
		t.Fatal("expected debounce output")
	}

	ev := waitForAuditEvent(t, auditCh)
	if ev.Action != domain.AuditActionJobTriggered {
		t.Fatalf("audit action = %q", ev.Action)
	}
	var details map[string]any
	if err := json.Unmarshal(ev.Details, &details); err != nil {
		t.Fatalf("audit details: %v", err)
	}
	if details["debounced"] != true {
		t.Fatalf("audit details debounced = %v, want true", details["debounced"])
	}
}

func TestTriggerJob_BatchBufferSuccessEmitsAuditEvent(t *testing.T) {
	auditCh := make(chan *domain.AuditEvent, 1)
	job := testEnabledJob("job-batch")
	job.BatchWindowSecs = 60

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		InsertBatchBufferItemFunc: func(context.Context, *domain.BatchBufferItem) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			auditCh <- ev
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, job.ProjectID)

	out, err := srv.handleTriggerJob(ctx, &TriggerJobInput{
		JobID: job.ID,
		Body:  TriggerRequest{BatchKey: "customer-1"},
	})
	if err != nil {
		t.Fatalf("handleTriggerJob: %v", err)
	}
	if out == nil {
		t.Fatal("expected batch-buffer output")
	}

	ev := waitForAuditEvent(t, auditCh)
	var details map[string]any
	if err := json.Unmarshal(ev.Details, &details); err != nil {
		t.Fatalf("audit details: %v", err)
	}
	if details["buffered"] != true {
		t.Fatalf("audit details buffered = %v, want true", details["buffered"])
	}
}

func TestTriggerJob_WaitingDependencySuccessEmitsAuditEvent(t *testing.T) {
	auditCh := make(chan *domain.AuditEvent, 1)
	job := testEnabledJob("job-waiting")

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		AreJobDependenciesSatisfiedFunc: func(context.Context, *domain.JobRun) (bool, error) {
			return false, nil
		},
		CreateRunFunc: func(context.Context, *domain.JobRun) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			auditCh <- ev
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, job.ProjectID)

	out, err := srv.handleTriggerJob(ctx, &TriggerJobInput{JobID: job.ID})
	if err != nil {
		t.Fatalf("handleTriggerJob: %v", err)
	}
	if out == nil {
		t.Fatal("expected waiting output")
	}

	ev := waitForAuditEvent(t, auditCh)
	var details map[string]any
	if err := json.Unmarshal(ev.Details, &details); err != nil {
		t.Fatalf("audit details: %v", err)
	}
	if details["waiting"] != true {
		t.Fatalf("audit details waiting = %v, want true", details["waiting"])
	}
}
