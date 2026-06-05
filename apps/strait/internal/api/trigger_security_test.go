package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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

func TestTriggerJob_DryRunRejectsPriorityAboveBillingLimit(t *testing.T) {
	t.Parallel()

	job := testEnabledJob("job-priority")
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud
	srv.billingEnforcer = &triggerDryRunBillingEnforcer{priorityErr: errors.New("dispatch priority exceeds plan limit")}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-priority/trigger", `{"dry_run":true,"priority":10}`))

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "dispatch priority exceeds plan limit") {
		t.Fatalf("response body = %s, want billing priority error", w.Body.String())
	}
}

func TestCheckTriggerDispatchPriority_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	srv := &Server{edition: domain.EditionCloud}

	err := srv.checkTriggerDispatchPriority(context.Background(), "proj-1", 10)
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got %v", err)
	}
}

func TestCheckTriggerDispatchPriority_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	srv := &Server{edition: domain.EditionCommunity}

	if err := srv.checkTriggerDispatchPriority(context.Background(), "proj-1", 10); err != nil {
		t.Fatalf("community nil enforcer must fail open; got %v", err)
	}
}

func TestTriggerJob_DryRunRejectsDailyCostBudgetExceeded(t *testing.T) {
	t.Parallel()

	job := testEnabledJob("job-budget")
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxDailyCostMicrousd: 5000, Timezone: "UTC"}, nil
		},
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, projectID string, timezone string) (int64, error) {
			if projectID != job.ProjectID {
				t.Fatalf("projectID = %q, want %q", projectID, job.ProjectID)
			}
			if timezone != "UTC" {
				t.Fatalf("timezone = %q, want UTC", timezone)
			}
			return 5000, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-budget/trigger", `{"dry_run":true}`))

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "project daily cost budget exceeded") {
		t.Fatalf("response body = %s, want daily cost budget error", w.Body.String())
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

type triggerDryRunBillingEnforcer struct {
	mockBillingEnforcer
	priorityErr error
}

func (e *triggerDryRunBillingEnforcer) CheckMaxDispatchPriority(_ context.Context, _ string, _ int) error {
	return e.priorityErr
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
