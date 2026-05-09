package worker

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// mockBillingEnforcerStore satisfies billing.Store for testing.
// Returns a free-tier subscription with low limits to trigger enforcement.
type mockBillingEnforcerStore struct {
	projectOrgID       string
	sub                *billing.OrgSubscription
	periodSpend        int64
	projectBudget      int64
	projectAction      string
	projectPeriodSpend int64
}

func (m *mockBillingEnforcerStore) UpdateEntitlements(context.Context, string, billing.OrgPlanLimits) error {
	return nil
}
func (m *mockBillingEnforcerStore) EnsureOrgSubscription(_ context.Context, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) GetOrgSubscription(_ context.Context, _ string) (*billing.OrgSubscription, error) {
	if m.sub != nil {
		return m.sub, nil
	}
	return nil, billing.ErrSubscriptionNotFound
}
func (m *mockBillingEnforcerStore) GetOrgSubscriptionByStripeCustomerID(context.Context, string) (*billing.OrgSubscription, error) {
	return nil, billing.ErrSubscriptionNotFound
}
func (m *mockBillingEnforcerStore) GetOrgSubscriptionByStripeSubscriptionID(context.Context, string) (*billing.OrgSubscription, error) {
	return nil, billing.ErrSubscriptionNotFound
}
func (m *mockBillingEnforcerStore) UpsertOrgSubscription(_ context.Context, _ *billing.OrgSubscription) error {
	return nil
}
func (m *mockBillingEnforcerStore) UpdateOrgSubscriptionPlan(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) UpdateOrgSubscriptionFull(_ context.Context, _, _, _ string, _, _ *time.Time) error {
	return nil
}
func (m *mockBillingEnforcerStore) UpdateSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) SetPendingPlanTier(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) SetPendingDowngrade(_ context.Context, _, _ string, _, _ *time.Time) error {
	return nil
}
func (m *mockBillingEnforcerStore) ClearPendingPlanTier(_ context.Context, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) ApplyPendingDowngrade(_ context.Context, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) ListOrgsWithPendingDowngrade(_ context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return m.projectOrgID, nil
}
func (m *mockBillingEnforcerStore) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
	return m.projectOrgID, nil
}
func (m *mockBillingEnforcerStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) CountProjectsByOrg(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (m *mockBillingEnforcerStore) CountMembersByOrg(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (m *mockBillingEnforcerStore) CountExecutingRunsByOrg(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) BulkCountExecutingRunsByOrg(_ context.Context, orgIDs []string) (map[string]int, error) {
	return make(map[string]int, len(orgIDs)), nil
}
func (m *mockBillingEnforcerStore) CountAIModelCallsByOrg(_ context.Context, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) SetProjectOrgID(_ context.Context, _, _ string) error { return nil }
func (m *mockBillingEnforcerStore) UpsertUsageRecord(_ context.Context, _ *billing.UsageRecord) error {
	return nil
}
func (m *mockBillingEnforcerStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) GetProjectUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) GetOrgDailyUsage(_ context.Context, _ string, _ time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) SumOrgPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return m.periodSpend, nil
}
func (m *mockBillingEnforcerStore) GetProjectBudget(_ context.Context, _ string) (int64, string, error) {
	if m.projectAction == "" && m.projectBudget == 0 {
		return -1, "notify", nil
	}
	return m.projectBudget, m.projectAction, nil
}
func (m *mockBillingEnforcerStore) SetProjectBudget(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) GetProjectPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return m.projectPeriodSpend, nil
}
func (m *mockBillingEnforcerStore) UpdateAnomalyThresholds(_ context.Context, _ string, _, _ float64) error {
	return nil
}
func (m *mockBillingEnforcerStore) CountOrgsByUser(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) UpdatePaymentStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}
func (m *mockBillingEnforcerStore) ListOrgsInGracePeriod(_ context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) ListAllSubscribedOrgIDs(_ context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) ListStaleSubscriptions(_ context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) IsProjectSuspended(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockBillingEnforcerStore) SuspendExcessProjects(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) ListOrgAdminEmails(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) HasSentUsageReport(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}
func (m *mockBillingEnforcerStore) RecordSentUsageReport(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mockBillingEnforcerStore) UpdateMonthlyUsageEmail(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockBillingEnforcerStore) ListActiveAddons(_ context.Context, _ string) ([]billing.Addon, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) CreateAddon(_ context.Context, _ *billing.Addon) error {
	return nil
}
func (m *mockBillingEnforcerStore) DeactivateAddon(_ context.Context, _ string) error { return nil }
func (m *mockBillingEnforcerStore) CountActiveAddonsByType(_ context.Context, _ string, _ billing.AddonType) (int, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) RecordProcessedWebhook(_ context.Context, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) IsWebhookProcessed(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockBillingEnforcerStore) DeleteOldWebhookMessages(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) GetEnterpriseContract(_ context.Context, _ string) (*billing.EnterpriseContract, error) {
	return nil, billing.ErrContractNotFound
}
func (m *mockBillingEnforcerStore) UpsertEnterpriseContract(_ context.Context, _ *billing.EnterpriseContract) error {
	return nil
}
func (m *mockBillingEnforcerStore) ListExpiringContracts(_ context.Context, _ int) ([]billing.EnterpriseContract, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) PauseHTTPJobsByOrg(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) UnpauseJobsByPauseReason(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (m *mockBillingEnforcerStore) CountHTTPJobsByOrg(context.Context, string) (int, error) {
	return 0, nil
}

func newWorkerTestEnforcer(t *testing.T, billingStore billing.Store) (*billing.Enforcer, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return billing.NewEnforcer(billingStore, rdb, slog.Default()), mr
}

func TestBillingEnforcement_ConcurrentLimitFails_RollbackDailyCount(t *testing.T) {
	t.Parallel()

	// Set up a free-tier subscription with 1 concurrent run limit.
	sub := &billing.OrgSubscription{
		OrgID:    "org-test",
		PlanTier: string(domain.PlanFree),
	}
	bStore := &mockBillingEnforcerStore{
		projectOrgID: "org-test",
		sub:          sub,
	}

	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	// Pre-fill the concurrent counter to simulate max concurrent runs reached.
	// The free tier allows 2 concurrent runs. Set the counter to 2 so the
	// next increment (to 3) exceeds the limit.
	concurrentKey := "strait:org_concurrent:org-test"
	mr.Set(concurrentKey, "2")
	mr.SetTTL(concurrentKey, 24*time.Hour)

	// Set up an HTTP server that would handle the job.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:          "job-1",
				ProjectID:   "proj-1",
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

	run := &domain.JobRun{
		ID:         "run-billing-test",
		JobID:      "job-1",
		JobVersion: 1,
		Status:     domain.StatusDequeued,
	}

	ec := &ExecutionContext{Run: run, Start: time.Now()}
	handler := exec.executeInner
	handler(context.Background(), ec)

	// The run should have been failed because concurrent limit was exceeded.
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Verify that the run was transitioned to a terminal state.
	var foundFailure bool
	for _, call := range ms.statusCalls {
		if call.to == domain.StatusSystemFailed {
			foundFailure = true
			break
		}
	}
	if !foundFailure {
		t.Error("expected run to be marked as system_failed when concurrent limit exceeded")
	}

	// Verify the daily run counter was rolled back (decremented).
	dailyKey := "strait:org_runs:org-test:" + time.Now().UTC().Format("2006-01-02")
	val, err := mr.Get(dailyKey)
	if err == nil {
		// If the key exists, it should have been decremented.
		// After CheckDailyRunLimit increments (to 1) and DecrDailyRunCount decrements (back to 0),
		// the counter should be 0.
		if val != "0" {
			t.Errorf("daily run counter should be rolled back to 0 after concurrent limit failure, got %s", val)
		}
	}
	// If key doesn't exist, that's fine -- the decrement script floors at 0 and the key
	// may not have been set if the daily limit check passed without incrementing.
}

// TestBillingEnforcement_ConcurrentLimitFails_RollbackMonthlyCount verifies
// that when the concurrent run limit is exceeded after CheckMonthlyRunLimit
// increments the monthly counter, DecrMonthlyRunCount rolls it back so the
// counter does not drift upward from runs that were never durably enqueued.
func TestBillingEnforcement_ConcurrentLimitFails_RollbackMonthlyCount(t *testing.T) {
	t.Parallel()

	sub := &billing.OrgSubscription{
		OrgID:    "org-monthly-concurrent",
		PlanTier: string(domain.PlanFree),
	}
	bStore := &mockBillingEnforcerStore{
		projectOrgID: "org-monthly-concurrent",
		sub:          sub,
	}

	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	// Pre-fill the concurrent counter so the next CheckConcurrentRunLimit fails.
	// Free tier allows 2 concurrent runs; set to 2 so the check rejects.
	concurrentKey := "strait:org_concurrent:org-monthly-concurrent"
	mr.Set(concurrentKey, "2")
	mr.SetTTL(concurrentKey, 24*time.Hour)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:          "job-monthly-concurrent",
				ProjectID:   "proj-monthly-concurrent",
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

	run := &domain.JobRun{
		ID:         "run-monthly-concurrent",
		JobID:      "job-monthly-concurrent",
		JobVersion: 1,
		Status:     domain.StatusDequeued,
	}

	ec := &ExecutionContext{Run: run, Start: time.Now()}
	exec.executeInner(context.Background(), ec)

	// Verify the run was failed due to concurrent limit.
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var foundFailure bool
	for _, call := range ms.statusCalls {
		if call.to == domain.StatusSystemFailed {
			foundFailure = true
			break
		}
	}
	if !foundFailure {
		t.Error("expected run to be marked as system_failed when concurrent limit exceeded")
	}

	// Verify the monthly run counter was rolled back (decremented after the
	// concurrent limit abort).
	monthlyKey := "strait:org_monthly_runs:org-monthly-concurrent:" + time.Now().UTC().Format("2006-01")
	val, err := mr.Get(monthlyKey)
	if err == nil && val != "0" {
		t.Errorf("monthly run counter should be 0 after concurrent limit abort, got %s", val)
	}
}

// TestBillingEnforcement_MonthlyLimitExceeded_RollbackDailyCount verifies
// that when the monthly run limit is exceeded, the daily counter that was
// already incremented by CheckDailyRunLimit is rolled back via DecrDailyRunCount.
func TestBillingEnforcement_MonthlyLimitExceeded_RollbackDailyCount(t *testing.T) {
	t.Parallel()

	// Free tier: MaxRunsPerDay=100, MaxRunsPerMonth=2000 (use a very low monthly limit).
	// We pre-fill the monthly counter at the free cap so CheckMonthlyRunLimit rejects.
	sub := &billing.OrgSubscription{
		OrgID:    "org-monthly-cap",
		PlanTier: string(domain.PlanFree),
	}
	bStore := &mockBillingEnforcerStore{
		projectOrgID: "org-monthly-cap",
		sub:          sub,
	}

	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	// Pre-fill the monthly counter above the free-tier cap (2000) so
	// CheckMonthlyRunLimit hard-rejects on the next call.
	monthlyKey := "strait:org_monthly_runs:org-monthly-cap:" + time.Now().UTC().Format("2006-01")
	mr.Set(monthlyKey, "2001")
	mr.SetTTL(monthlyKey, 62*24*time.Hour)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:          "job-monthly-cap",
				ProjectID:   "proj-monthly-cap",
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

	run := &domain.JobRun{
		ID:         "run-monthly-cap",
		JobID:      "job-monthly-cap",
		JobVersion: 1,
		Status:     domain.StatusDequeued,
	}

	ec := &ExecutionContext{Run: run, Start: time.Now()}
	exec.executeInner(context.Background(), ec)

	// Verify the run was failed due to monthly limit.
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var foundFailure bool
	for _, call := range ms.statusCalls {
		if call.to == domain.StatusSystemFailed {
			foundFailure = true
			break
		}
	}
	if !foundFailure {
		t.Error("expected run to be marked as system_failed when monthly limit exceeded")
	}

	// Verify the daily run counter was rolled back after the monthly-limit abort.
	dailyKey := "strait:org_runs:org-monthly-cap:" + time.Now().UTC().Format("2006-01-02")
	val, err := mr.Get(dailyKey)
	if err == nil && val != "0" {
		t.Errorf("daily run counter should be 0 after monthly limit abort, got %s", val)
	}
}
