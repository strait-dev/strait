package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBulkTrigger_PriorityCheckedAfterIdempotencyHit_NotInvokedForCachedRun
// confirms that an idempotency hit short-circuits the gate (the run already
// exists, no new dispatch happens). This locks in the ordering: idempotency
// → priority gate → enqueue.
func TestBulkTrigger_PriorityCheckedAfterIdempotencyHit_NotInvokedForCachedRun(t *testing.T) {
	t.Parallel()

	existingRun := &domain.JobRun{ID: "existing-run", Status: domain.StatusCompleted}
	enforcer := &priorityRecordingEnforcer{maxPriority: 1}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _ string, key string) (*domain.JobRun, error) {
			if key == "cached" {
				return existingRun, nil
			}
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newServerWithEnforcer(t, ms, mq, enforcer)

	// Item 0: cached idempotency, priority 99 — must NOT be checked because
	// the idempotency hit returns the cached run.
	// Item 1: fresh, priority 1 — must be checked and pass.
	body := `{"items":[{"priority":99,"idempotency_key":"cached"},{"priority":1,"idempotency_key":"fresh"}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.EqualValues(t, 1, enforcer.calls.
		Load())

	// Only the fresh item should have triggered the gate.

}

// flakyEnforcer trips the gate on a specific item index. Used to simulate the
// "smuggle one bad item" scenario where the Nth check fails.
type flakyEnforcer struct {
	mockBillingEnforcer
	failOnCall int64
	calls      atomic.Int64
}

func (f *flakyEnforcer) CheckMaxDispatchPriority(_ context.Context, _ string, _ int) error {
	c := f.calls.Add(1)
	if c == f.failOnCall {
		return fmt.Errorf("priority over cap at call %d", c)
	}
	return nil
}

// TestBulkTrigger_FailureAtItemN_StopsAtFirstFailure verifies that when the
// gate fails on item N of a batch, the loop bails immediately — no further
// items are processed. The enclosing transaction (real Postgres path) rolls
// back; the unit-mock here only proves the loop short-circuits on first
// failure rather than swallowing the error and continuing.
func TestBulkTrigger_FailureAtItemN_StopsAtFirstFailure(t *testing.T) {
	t.Parallel()

	enforcer := &flakyEnforcer{failOnCall: 5}

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newServerWithEnforcer(t, ms, mq, enforcer)

	items := make([]map[string]any, 0, 10)
	for i := range 10 {
		items = append(items, map[string]any{"priority": 1, "idempotency_key": fmt.Sprintf("k-%d", i)})
	}
	bodyBytes, _ := json.Marshal(map[string]any{"items": items})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", string(bodyBytes)))
	require.Equal(t, http.StatusPaymentRequired,

		w.Code,
	)
	require.EqualValues(t, 5, enforcer.calls.
		Load())

	// Gate fired exactly 5 times: items 0..3 passed, item 4 failed and bailed.

}

// TestBulkTrigger_PerItemErrorMessageReferencesItemIndex verifies that the
// error response identifies which item triggered the gate, so a tenant
// submitting a 500-item batch can find the offending entry.
func TestBulkTrigger_PerItemErrorMessageReferencesItemIndex(t *testing.T) {
	t.Parallel()

	enforcer := &priorityRecordingEnforcer{maxPriority: 5}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newServerWithEnforcer(t, ms, mq, enforcer)

	body := `{"items":[{"priority":1},{"priority":2},{"priority":99}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))
	require.Equal(t, http.StatusPaymentRequired,

		w.Code,
	)

	bodyStr := w.Body.String()
	assert.True(t,
		strings.Contains(bodyStr,
			"item 2",
		))

}

func TestBulkTrigger_RateLimitCountsPendingBatchItems(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Bool
	job := testEnabledJob("job-rate")
	job.RateLimitMax = 2
	job.RateLimitWindowSecs = 60
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, job.ID, id)

			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CountRunsForJobSinceFunc: func(_ context.Context, jobID string, _ time.Time) (int, error) {
			require.Equal(t, job.ID, jobID)

			return 1, nil
		},
	}
	mq := &mockQueue{enqueueBatchFn: func(_ context.Context, _ []*domain.JobRun) (int64, error) {
		enqueued.Store(true)
		return 0, nil
	}}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-rate/trigger/bulk", `{"items":[{},{}]}`))
	require.Equal(t, http.StatusTooManyRequests,

		w.Code,
	)
	require.False(t, enqueued.Load())

}

func TestBulkTrigger_DailyCostBudgetExceeded(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Bool
	var budgetChecks atomic.Int64
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxDailyCostMicrousd: 5000, Timezone: "UTC"}, nil
		},
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, projectID string, timezone string) (int64, error) {
			budgetChecks.Add(1)
			require.Equal(t, "proj-1", projectID)
			require.Equal(t, "UTC", timezone)

			return 5000, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued.Store(true)
		return nil
	}}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-budget/trigger/bulk", `{"items":[{},{}]}`))
	require.Equal(t, http.StatusTooManyRequests,

		w.Code,
	)
	require.False(t, enqueued.Load())
	require.EqualValues(t, 1, budgetChecks.
		Load())

}

func TestBulkTrigger_DailyCostBudgetAllIdempotencyHitsBypass(t *testing.T) {
	t.Parallel()

	existingRun := &domain.JobRun{ID: "existing-run", Status: domain.StatusCompleted}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxDailyCostMicrousd: 5000, Timezone: "UTC"}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _ string, _ string) (*domain.JobRun, error) {
			return existingRun, nil
		},
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, _ string, _ string) (int64, error) {
			require.Fail(t,

				"daily budget must not be checked when every item is an idempotency hit")
			return 0, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		require.Fail(t,

			"idempotency-only bulk trigger must not enqueue a new run")
		return nil
	}}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(
		http.MethodPost,
		"/v1/jobs/job-budget/trigger/bulk",
		`{"items":[{"idempotency_key":"cached-a"},{"idempotency_key":"cached-b"}]}`,
	))
	require.Equal(t, http.StatusCreated,

		w.Code)

	var resp BulkTriggerResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.EqualValues(t, 0, resp.Created)

	for _, result := range resp.Results {
		require.True(
			t, result.IdempotencyHit,
		)

	}
}

func TestBulkTrigger_DailyCostBudgetCheckedOnceForNewBatch(t *testing.T) {
	t.Parallel()

	var budgetChecks atomic.Int64
	var enqueued atomic.Int64
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxDailyCostMicrousd: 5000, Timezone: "UTC"}, nil
		},
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, _ string, _ string) (int64, error) {
			budgetChecks.Add(1)
			return 3000, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued.Add(1)
		return nil
	}}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-budget/trigger/bulk", `{"items":[{},{},{}]}`))
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.EqualValues(t, 1, budgetChecks.
		Load())
	require.EqualValues(t, 3, enqueued.Load())

}
