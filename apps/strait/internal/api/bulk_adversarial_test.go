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

	"strait/internal/domain"
	"strait/internal/store"
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	// Only the fresh item should have triggered the gate.
	if got := enforcer.calls.Load(); got != 1 {
		t.Fatalf("CheckMaxDispatchPriority calls = %d, want 1 (cached items skip the gate)", got)
	}
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

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 PaymentRequired, got %d: %s", w.Code, w.Body.String())
	}
	// Gate fired exactly 5 times: items 0..3 passed, item 4 failed and bailed.
	if got := enforcer.calls.Load(); got != 5 {
		t.Fatalf("gate calls = %d, want 5 (stop at first failure)", got)
	}
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

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %s", w.Code, w.Body.String())
	}
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "item 2") {
		t.Errorf("error body must reference 'item 2' (the offending index), got: %s", bodyStr)
	}
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
			if projectID != "proj-1" {
				t.Fatalf("projectID = %q, want proj-1", projectID)
			}
			if timezone != "UTC" {
				t.Fatalf("timezone = %q, want UTC", timezone)
			}
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

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued.Load() {
		t.Fatal("expected bulk trigger not to enqueue when daily cost budget is exhausted")
	}
	if got := budgetChecks.Load(); got != 1 {
		t.Fatalf("daily budget checks = %d, want 1", got)
	}
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
			t.Fatal("daily budget must not be checked when every item is an idempotency hit")
			return 0, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		t.Fatal("idempotency-only bulk trigger must not enqueue a new run")
		return nil
	}}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(
		http.MethodPost,
		"/v1/jobs/job-budget/trigger/bulk",
		`{"items":[{"idempotency_key":"cached-a"},{"idempotency_key":"cached-b"}]}`,
	))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp BulkTriggerResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Created != 0 {
		t.Fatalf("created = %d, want 0", resp.Created)
	}
	for idx, result := range resp.Results {
		if !result.IdempotencyHit {
			t.Fatalf("result %d idempotency_hit = false, want true", idx)
		}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := budgetChecks.Load(); got != 1 {
		t.Fatalf("daily budget checks = %d, want 1", got)
	}
	if got := enqueued.Load(); got != 3 {
		t.Fatalf("enqueued runs = %d, want 3", got)
	}
}
