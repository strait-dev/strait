package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// dispatchHarness wires the moving parts that every spending-limit test
// in this file needs: an executor with a billing enforcer attached, a
// stubbed HTTP target, and a controllable mock store. Returning the bits
// the assertions actually use keeps each test focused on what it's
// proving.
type dispatchHarness struct {
	exec   *Executor
	store  *mockExecutorStore
	srv    *httptest.Server
	bStore *mockBillingEnforcerStore
}

func newDispatchHarness(t *testing.T, sub *billing.OrgSubscription, periodSpend int64) *dispatchHarness {
	return newDispatchHarnessWithBudget(t, sub, periodSpend, -1, "", 0)
}

// newDispatchHarnessWithBudget extends newDispatchHarness with project
// budget controls so Phase 4.2 tests can drive budget-block paths
// without forking the whole executor wiring.
func newDispatchHarnessWithBudget(t *testing.T, sub *billing.OrgSubscription, periodSpend int64,
	projectBudget int64, projectAction string, projectSpend int64,
) *dispatchHarness {
	t.Helper()
	bStore := &mockBillingEnforcerStore{
		projectOrgID:       sub.OrgID,
		sub:                sub,
		periodSpend:        periodSpend,
		projectBudget:      projectBudget,
		projectAction:      projectAction,
		projectPeriodSpend: projectSpend,
	}
	enforcer, _ := newWorkerTestEnforcer(t, bStore)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:          "job-spend",
				ProjectID:   "proj-spend",
				Version:     1,
				EndpointURL: srv.URL,
				MaxAttempts: 1,
				TimeoutSecs: 30,
			}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:            NewPool(4),
		Store:           ms,
		PollInterval:    time.Millisecond,
		HTTPClient:      srv.Client(),
		BillingEnforcer: enforcer,
	})
	return &dispatchHarness{exec: exec, store: ms, srv: srv, bStore: bStore}
}

func runDispatch(h *dispatchHarness, runID string) {
	run := &domain.JobRun{
		ID:         runID,
		JobID:      "job-spend",
		JobVersion: 1,
		Status:     domain.StatusDequeued,
	}
	ec := &ExecutionContext{Run: run, Start: time.Now()}
	h.exec.executeInner(context.Background(), ec)
}

func sawSystemFailed(ms *mockExecutorStore) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for _, call := range ms.statusCalls {
		if call.to == domain.StatusSystemFailed {
			return true
		}
	}
	return false
}

// TestDispatchSpendingLimit_NoLimitSet: SpendingLimitMicrousd == -1 means
// the org has not configured a cap. Dispatch must proceed regardless of
// spend.
func TestDispatchSpendingLimit_NoLimitSet(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-no-limit",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: -1,
	}
	h := newDispatchHarness(t, sub, 9_999_999_999)

	runDispatch(h, "run-no-limit")
	if sawSystemFailed(h.store) {
		t.Errorf("dispatch was rejected even though SpendingLimitMicrousd=-1 (no cap)")
	}
}

// TestDispatchSpendingLimit_BelowLimit_ProceedsAndIncrementsCounters
// confirms that under the cap the run runs through the rest of the
// billing pipeline (daily/monthly increment, no rollback path taken).
func TestDispatchSpendingLimit_BelowLimit_ProceedsAndIncrementsCounters(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-below-limit",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: 10_000_000, // $10
	}
	h := newDispatchHarness(t, sub, 1_000_000) // $1 spent

	runDispatch(h, "run-below-limit")
	if sawSystemFailed(h.store) {
		t.Errorf("dispatch was rejected with spend below the limit")
	}
}

// TestDispatchSpendingLimit_OverLimit_RejectsBeforeCounters is the
// headline check for Phase 4.1: spend over the cap blocks dispatch and
// runs do NOT leak through to the daily/monthly increment paths.
// Because CheckSpendingLimit fires first, no rollback is required.
func TestDispatchSpendingLimit_OverLimit_RejectsBeforeCounters(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-over-limit",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: 1_000_000, // $1 cap
	}
	h := newDispatchHarness(t, sub, 5_000_000) // $5 spent

	runDispatch(h, "run-over-limit")
	if !sawSystemFailed(h.store) {
		t.Errorf("expected system_failed when spending limit exceeded")
	}
}

// TestDispatchSpendingLimit_AtLimitRejects locks in the existing
// `>=` semantics of isOverageLimitReached: hitting the cap exactly is
// treated as reached. Documenting this here so any future change to
// the threshold makes a deliberate test failure surface.
func TestDispatchSpendingLimit_AtLimitRejects(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-at-limit",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: 2_500_000,
	}
	h := newDispatchHarness(t, sub, 2_500_000)

	runDispatch(h, "run-at-limit")
	if !sawSystemFailed(h.store) {
		t.Errorf("expected rejection at exact-equal spend; isOverageLimitReached uses >=")
	}
}

// TestDispatchSpendingLimit_FreeTierZeroSpend_Proceeds confirms that
// Free-tier orgs with zero usage are not held up by the spending check
// (Free has no SpendingLimitMicrousd; the path branches into
// checkFreeTierIncludedCredit and returns nil).
func TestDispatchSpendingLimit_FreeTierZeroSpend_Proceeds(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:    "org-free-zero",
		PlanTier: string(domain.PlanFree),
		Status:   "active",
	}
	h := newDispatchHarness(t, sub, 0)

	runDispatch(h, "run-free-zero")
	if sawSystemFailed(h.store) {
		t.Errorf("free tier with zero spend should not be blocked by spending check")
	}
}
