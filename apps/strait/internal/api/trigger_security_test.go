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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitForAuditEvent(t *testing.T, ch <-chan *domain.AuditEvent) *domain.AuditEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for audit event")
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
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(), "scheduled_at must not be in the past"))

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
	require.Equal(t, http.StatusPaymentRequired,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "dispatch priority exceeds plan limit",
		))

}

func TestCheckTriggerDispatchPriority_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	srv := &Server{edition: domain.EditionCloud}

	err := srv.checkTriggerDispatchPriority(context.Background(), "proj-1", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"billing enforcement unavailable",
	)

}

func TestCheckTriggerDispatchPriority_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	srv := &Server{edition: domain.EditionCommunity}
	require.NoError(t, srv.checkTriggerDispatchPriority(
		context.Background(),
		"proj-1",
		10))

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
			require.Equal(t, job.ProjectID,
				projectID,
			)
			require.Equal(t, "UTC", timezone)

			return 5000, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-budget/trigger", `{"dry_run":true}`))
	require.Equal(t, http.StatusTooManyRequests,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "project daily cost budget exceeded",
		))

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
	require.NoError(t, err)
	require.NotNil(t, out)

	ev := waitForAuditEvent(t, auditCh)
	require.Equal(t, domain.AuditActionJobTriggered,

		ev.
			Action)

	var details map[string]any
	require.NoError(t, json.Unmarshal(ev.Details,
		&details,
	))
	require.Equal(t, true, details["debounced"])

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
	require.NoError(t, err)
	require.NotNil(t, out)

	ev := waitForAuditEvent(t, auditCh)
	var details map[string]any
	require.NoError(t, json.Unmarshal(ev.Details,
		&details,
	))
	require.Equal(t, true, details["buffered"])

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
	require.NoError(t, err)
	require.NotNil(t, out)

	ev := waitForAuditEvent(t, auditCh)
	var details map[string]any
	require.NoError(t, json.Unmarshal(ev.Details,
		&details,
	))
	require.Equal(t, true, details["waiting"])

}
