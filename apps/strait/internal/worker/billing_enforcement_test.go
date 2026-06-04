package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
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
	projectOrgErr      error
	sub                *billing.OrgSubscription
	subErr             error
	periodSpend        int64
	projectBudget      int64
	projectAction      string
	projectPeriodSpend int64
	usageRecords       atomic.Int64
}

func (m *mockBillingEnforcerStore) UpdateEntitlements(context.Context, string, billing.OrgPlanLimits) error {
	return nil
}
func (m *mockBillingEnforcerStore) EnsureOrgSubscription(_ context.Context, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) GetOrgSubscription(_ context.Context, _ string) (*billing.OrgSubscription, error) {
	if m.subErr != nil {
		return nil, m.subErr
	}
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
func (m *mockBillingEnforcerStore) UpdateOrgSubscriptionStatus(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) UpdateOrgSubscriptionFull(_ context.Context, _, _, _ string, _, _ *time.Time) error {
	return nil
}
func (m *mockBillingEnforcerStore) UpdateSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockBillingEnforcerStore) UpdateOverageDisabled(_ context.Context, _ string, _ bool) error {
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
	if m.projectOrgErr != nil {
		return "", m.projectOrgErr
	}
	return m.projectOrgID, nil
}
func (m *mockBillingEnforcerStore) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
	if m.projectOrgErr != nil {
		return "", m.projectOrgErr
	}
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
func (m *mockBillingEnforcerStore) SetProjectOrgID(_ context.Context, _, _ string) error { return nil }
func (m *mockBillingEnforcerStore) UpsertUsageRecord(_ context.Context, _ *billing.UsageRecord) error {
	m.usageRecords.Add(1)
	return nil
}
func (m *mockBillingEnforcerStore) RecordUsageCost(_ context.Context, _ *billing.UsageRecord, _, _ string) (bool, error) {
	m.usageRecords.Add(1)
	return true, nil
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
func (m *mockBillingEnforcerStore) PauseHTTPJobsByOrg(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (m *mockBillingEnforcerStore) TryMarkBillingCapEvent(context.Context, string, billing.BillingCapEvent) (bool, error) {
	return false, nil
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

func TestBillingEnforcement_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	execStore := &mockExecutorStore{}
	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Store:        execStore,
		PollInterval: time.Millisecond,
		Edition:      domain.EditionCloud,
	})
	run := &domain.JobRun{
		ID:        "run-cloud-nil-billing",
		JobID:     "job-cloud-nil-billing",
		ProjectID: "proj-cloud-nil-billing",
		Status:    domain.StatusDequeued,
		Attempt:   1,
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		ExecutionMode: domain.ExecutionModeWorker,
	}

	release, ok := exec.enforceDispatchBilling(context.Background(), run, job)
	if ok {
		t.Fatal("expected missing cloud billing enforcer to block dispatch")
	}
	if release != nil {
		t.Fatal("blocked dispatch must not return a billing release callback")
	}
	execStore.mu.Lock()
	defer execStore.mu.Unlock()
	if len(execStore.statusCalls) != 1 {
		t.Fatalf("status calls = %d, want 1", len(execStore.statusCalls))
	}
	if got := execStore.statusCalls[0].to; got != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", got, domain.StatusSystemFailed)
	}
}

func TestBillingEnforcement_CommunityNilEnforcerAllows(t *testing.T) {
	t.Parallel()

	execStore := &mockExecutorStore{}
	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Store:        execStore,
		PollInterval: time.Millisecond,
		Edition:      domain.EditionCommunity,
	})
	run := &domain.JobRun{
		ID:        "run-community-nil-billing",
		JobID:     "job-community-nil-billing",
		ProjectID: "proj-community-nil-billing",
		Status:    domain.StatusDequeued,
		Attempt:   1,
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		ExecutionMode: domain.ExecutionModeWorker,
	}

	release, ok := exec.enforceDispatchBilling(context.Background(), run, job)
	if !ok {
		t.Fatal("expected community nil billing enforcer to allow dispatch")
	}
	if release != nil {
		t.Fatal("community nil enforcer should not return a billing release callback")
	}
	execStore.mu.Lock()
	defer execStore.mu.Unlock()
	if len(execStore.statusCalls) != 0 {
		t.Fatalf("status calls = %d, want 0", len(execStore.statusCalls))
	}
}

func TestBillingEnforcement_ProjectOrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	bStore := &mockBillingEnforcerStore{
		projectOrgErr: errors.New("project org lookup unavailable"),
		sub: &billing.OrgSubscription{
			OrgID:    "org-lookup-error",
			PlanTier: string(domain.PlanFree),
		},
	}
	enforcer, _ := newWorkerTestEnforcer(t, bStore)

	execStore := &mockExecutorStore{}
	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:            pool,
		Store:           execStore,
		PollInterval:    time.Millisecond,
		BillingEnforcer: enforcer,
	})
	run := &domain.JobRun{
		ID:        "run-org-lookup-error",
		JobID:     "job-org-lookup-error",
		ProjectID: "proj-org-lookup-error",
		Status:    domain.StatusDequeued,
		Attempt:   1,
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		ExecutionMode: domain.ExecutionModeWorker,
	}

	release, ok := exec.enforceDispatchBilling(context.Background(), run, job)
	if ok {
		t.Fatal("expected billing org lookup error to block dispatch")
	}
	if release != nil {
		t.Fatal("blocked dispatch must not return a billing release callback")
	}
	execStore.mu.Lock()
	defer execStore.mu.Unlock()
	if len(execStore.statusCalls) != 1 {
		t.Fatalf("status calls = %d, want 1", len(execStore.statusCalls))
	}
	if got := execStore.statusCalls[0].to; got != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", got, domain.StatusSystemFailed)
	}
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
	// Free tier allows ConcurrentFree concurrent runs; set at the cap so the
	// next increment exceeds it and the check rejects.
	concurrentKey := "strait:org_concurrent:org-monthly-concurrent"
	if err := mr.Set(concurrentKey, strconv.Itoa(billing.ConcurrentFree)); err != nil {
		t.Fatalf("seed concurrent count: %v", err)
	}
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

// TestBillingEnforcement_MonthlyLimitExceeded_DoesNotIncrementMonthlyCount
// verifies that a free-tier dispatch rejected at the monthly cap does not
// consume an additional monthly run.
func TestBillingEnforcement_MonthlyLimitExceeded_DoesNotIncrementMonthlyCount(t *testing.T) {
	t.Parallel()

	// Pre-fill the monthly counter at the free cap so CheckMonthlyRunLimit
	// rejects without incrementing the stored value.
	sub := &billing.OrgSubscription{
		OrgID:           "org-monthly-cap",
		PlanTier:        string(domain.PlanFree),
		OverageDisabled: true,
	}
	bStore := &mockBillingEnforcerStore{
		projectOrgID: "org-monthly-cap",
		sub:          sub,
	}

	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	// Pre-fill the monthly counter at the free-tier cap so
	// CheckMonthlyRunLimit hard-rejects on the next call.
	monthlyKey := "strait:org_monthly_runs:org-monthly-cap:" + time.Now().UTC().Format("2006-01")
	if err := mr.Set(monthlyKey, strconv.Itoa(billing.MaxRunsPerMonthFree)); err != nil {
		t.Fatalf("seed monthly count: %v", err)
	}
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

	val, err := mr.Get(monthlyKey)
	if err != nil {
		t.Fatalf("expected monthly counter to remain present: %v", err)
	}
	if val != strconv.Itoa(billing.MaxRunsPerMonthFree) {
		t.Errorf("monthly run counter = %s, want %d after monthly limit abort", val, billing.MaxRunsPerMonthFree)
	}
}

func TestBillingEnforcement_AutomaticRetryDoesNotIncrementMonthlyRunCount(t *testing.T) {
	t.Parallel()

	orgID := "org-auto-retry"
	bStore := &mockBillingEnforcerStore{
		projectOrgID: orgID,
		sub: &billing.OrgSubscription{
			OrgID:    orgID,
			PlanTier: string(domain.PlanFree),
		},
	}
	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:            pool,
		Store:           &mockExecutorStore{},
		PollInterval:    time.Millisecond,
		BillingEnforcer: enforcer,
	})

	run := &domain.JobRun{
		ID:        "run-auto-retry",
		JobID:     "job-auto-retry",
		ProjectID: "proj-auto-retry",
		Status:    domain.StatusDequeued,
		Attempt:   2,
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		ExecutionMode: domain.ExecutionModeWorker,
	}

	if !exec.checkDispatchBillingLimits(context.Background(), run, job, orgID) {
		t.Fatal("retry dispatch should remain eligible for non-usage billing gates")
	}

	monthlyKey := "strait:org_monthly_runs:" + orgID + ":" + time.Now().UTC().Format("2006-01")
	if val, err := mr.Get(monthlyKey); err == nil && val != "0" {
		t.Fatalf("automatic retry must not increment monthly run counter, got %s", val)
	}
}

func TestBillingEnforcement_FirstAttemptIncrementsMonthlyRunCount(t *testing.T) {
	t.Parallel()

	orgID := "org-first-attempt"
	bStore := &mockBillingEnforcerStore{
		projectOrgID: orgID,
		sub: &billing.OrgSubscription{
			OrgID:    orgID,
			PlanTier: string(domain.PlanFree),
		},
	}
	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:            pool,
		Store:           &mockExecutorStore{},
		PollInterval:    time.Millisecond,
		BillingEnforcer: enforcer,
	})

	run := &domain.JobRun{
		ID:        "run-first-attempt",
		JobID:     "job-first-attempt",
		ProjectID: "proj-first-attempt",
		Status:    domain.StatusDequeued,
		Attempt:   1,
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		ExecutionMode: domain.ExecutionModeWorker,
	}

	if !exec.checkDispatchBillingLimits(context.Background(), run, job, orgID) {
		t.Fatal("first dispatch attempt should pass billing gates")
	}

	monthlyKey := "strait:org_monthly_runs:" + orgID + ":" + time.Now().UTC().Format("2006-01")
	val, err := mr.Get(monthlyKey)
	if err != nil {
		t.Fatalf("expected first dispatch attempt to create monthly counter: %v", err)
	}
	if val != "1" {
		t.Fatalf("first dispatch attempt monthly counter = %s, want 1", val)
	}
}

func TestBillingEnforcement_TerminalFailureRecordsBillableRunCost(t *testing.T) {
	t.Parallel()

	orgID := "org-terminal-failure"
	bStore := &mockBillingEnforcerStore{
		projectOrgID: orgID,
		sub: &billing.OrgSubscription{
			OrgID:    orgID,
			PlanTier: string(domain.PlanPro),
		},
	}
	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:            pool,
		Store:           &mockExecutorStore{},
		PollInterval:    time.Millisecond,
		BillingEnforcer: enforcer,
		RunCostRecorder: billing.NewRunCostRecorder(bStore, nil, nil, slog.Default()),
	})

	run := &domain.JobRun{
		ID:        "run-terminal-failure",
		JobID:     "job-terminal-failure",
		ProjectID: "proj-terminal-failure",
		Status:    domain.StatusExecuting,
		Attempt:   1,
		Metadata:  map[string]string{},
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		EndpointURL:   "https://example.test/worker",
		ExecutionMode: domain.ExecutionModeHTTP,
		MaxAttempts:   1,
		TimeoutSecs:   30,
	}
	if err := enforcer.CheckMonthlyRunLimitForRun(context.Background(), orgID, run.ID); err != nil {
		t.Fatalf("mark monthly run overage: %v", err)
	}
	monthlyKey := "strait:org_monthly_runs:" + orgID + ":" + time.Now().UTC().Format("2006-01")
	if err := mr.Set(monthlyKey, strconv.Itoa(billing.MaxRunsPerMonthPro+1)); err != nil {
		t.Fatalf("seed monthly run count: %v", err)
	}

	if !exec.handleFailure(context.Background(), run, job, executionPolicy{
		maxAttempts:      1,
		timeoutSecs:      30,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     60,
	}, context.DeadlineExceeded, nil) {
		t.Fatal("expected terminal failure transition to succeed")
	}
	exec.stripeUsageWG.Wait()

	if got := bStore.usageRecords.Load(); got != 1 {
		t.Fatalf("terminal failed run cost records = %d, want 1", got)
	}
}

func TestBillingEnforcement_TerminalTimeoutRecordsBillableRunCost(t *testing.T) {
	t.Parallel()

	orgID := "org-terminal-timeout"
	bStore := &mockBillingEnforcerStore{
		projectOrgID: orgID,
		sub: &billing.OrgSubscription{
			OrgID:    orgID,
			PlanTier: string(domain.PlanPro),
		},
	}
	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:            pool,
		Store:           &mockExecutorStore{},
		PollInterval:    time.Millisecond,
		BillingEnforcer: enforcer,
		RunCostRecorder: billing.NewRunCostRecorder(bStore, nil, nil, slog.Default()),
	})

	run := &domain.JobRun{
		ID:        "run-terminal-timeout",
		JobID:     "job-terminal-timeout",
		ProjectID: "proj-terminal-timeout",
		Status:    domain.StatusExecuting,
		Attempt:   1,
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		EndpointURL:   "https://example.test/worker",
		ExecutionMode: domain.ExecutionModeWorker,
		MaxAttempts:   1,
		TimeoutSecs:   30,
	}
	if err := enforcer.CheckMonthlyRunLimitForRun(context.Background(), orgID, run.ID); err != nil {
		t.Fatalf("mark monthly run overage: %v", err)
	}
	monthlyKey := "strait:org_monthly_runs:" + orgID + ":" + time.Now().UTC().Format("2006-01")
	if err := mr.Set(monthlyKey, strconv.Itoa(billing.MaxRunsPerMonthPro+1)); err != nil {
		t.Fatalf("seed monthly run count: %v", err)
	}

	exec.handleTimeout(context.Background(), run, job, executionPolicy{
		maxAttempts:      1,
		timeoutSecs:      30,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     60,
	}, nil)
	exec.stripeUsageWG.Wait()

	if got := bStore.usageRecords.Load(); got != 1 {
		t.Fatalf("terminal timed-out run cost records = %d, want 1", got)
	}
}

// TestExecuteInner_HTTPModeGateRebalancesConcurrentCounter pins the fix for
// the per-org concurrent counter leak: when the HTTP-mode plan gate rejects
// a dispatch, the unconditional INCR inside CheckConcurrentRunLimit must
// still be balanced by DecrConcurrentRunCount. Before the fix, the
// `defer DecrConcurrentRunCount` was registered AFTER the gate's early
// return, so each rejected HTTP-mode dispatch leaked one counter slot.
// Once the leak accumulated past MaxConcurrentRuns within the 5-minute
// reconciler window, subsequent runs falsely reported
// org_concurrent_run_limit_exceeded and went terminally StatusSystemFailed.
func TestExecuteInner_HTTPModeGateRebalancesConcurrentCounter(t *testing.T) {
	t.Parallel()

	orgID := "org-http-gate"
	// The mock store returns this subscription, and entitlementsAuthoritative
	// defaults to true on NewEnforcer, so we can force AllowsHTTPMode=false
	// without modifying the catalog plans (which currently set it true for
	// every tier). This mirrors the live failure mode: a persisted snapshot
	// that no longer permits HTTP mode after a downgrade.
	limits := billing.GetPlanLimits(domain.PlanFree)
	limits.AllowsHTTPMode = false
	entitlements, err := json.Marshal(limits)
	if err != nil {
		t.Fatalf("marshal entitlements: %v", err)
	}

	sub := &billing.OrgSubscription{
		OrgID:        orgID,
		PlanTier:     string(domain.PlanFree),
		Entitlements: entitlements,
	}
	bStore := &mockBillingEnforcerStore{
		projectOrgID: orgID,
		sub:          sub,
	}

	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID:            "job-http-gate",
				ProjectID:     "proj-http-gate",
				Version:       1,
				EndpointURL:   srv.URL,
				MaxAttempts:   1,
				TimeoutSecs:   30,
				ExecutionMode: domain.ExecutionModeHTTP,
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
		ID:         "run-http-gate",
		JobID:      "job-http-gate",
		JobVersion: 1,
		Status:     domain.StatusDequeued,
	}

	ec := &ExecutionContext{Run: run, Start: time.Now()}
	exec.executeInner(context.Background(), ec)

	// The gate must have rejected the run.
	ms.mu.Lock()
	var foundFailure bool
	for _, call := range ms.statusCalls {
		if call.to == domain.StatusSystemFailed {
			foundFailure = true
			break
		}
	}
	ms.mu.Unlock()
	if !foundFailure {
		t.Fatal("expected run to be marked as system_failed when HTTP mode is not allowed")
	}

	// The concurrent counter must be balanced: CheckConcurrentRunLimit INCRed
	// it from 0 to 1 (script returns count, not -1) and the deferred
	// DecrConcurrentRunCount must DECR it back to 0 (floor-at-zero).
	concurrentKey := "strait:org_concurrent:" + orgID
	val, getErr := mr.Get(concurrentKey)
	if getErr != nil {
		// miniredis returns an error when the key is absent; that's also a
		// valid balanced state (DECR floors at zero, miniredis garbage-
		// collects zero values).
		return
	}
	if val != "0" {
		t.Errorf("concurrent counter must be balanced after HTTP-mode gate rejection, got %s (leak indicates the DecrConcurrentRunCount defer was not registered before the gate)", val)
	}
}

func TestBillingEnforcement_HTTPModePlanLookupErrorFailsClosedAndRollsBackCounters(t *testing.T) {
	t.Parallel()

	orgID := "org-http-plan-error"
	bStore := &mockBillingEnforcerStore{
		projectOrgID: orgID,
		subErr:       errors.New("subscription lookup unavailable"),
	}
	enforcer, mr := newWorkerTestEnforcer(t, bStore)

	execStore := &mockExecutorStore{}
	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:            pool,
		Store:           execStore,
		PollInterval:    time.Millisecond,
		BillingEnforcer: enforcer,
	})
	run := &domain.JobRun{
		ID:        "run-http-plan-error",
		JobID:     "job-http-plan-error",
		ProjectID: "proj-http-plan-error",
		Status:    domain.StatusDequeued,
		Attempt:   1,
	}
	job := &domain.Job{
		ID:            run.JobID,
		ProjectID:     run.ProjectID,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
	monthlyKey := "strait:org_monthly_runs:" + orgID + ":" + time.Now().UTC().Format("2006-01")
	concurrentKey := "strait:org_concurrent:" + orgID
	if err := mr.Set(monthlyKey, "1"); err != nil {
		t.Fatalf("seed monthly run count: %v", err)
	}
	if err := mr.Set(concurrentKey, "1"); err != nil {
		t.Fatalf("seed concurrent run count: %v", err)
	}

	if exec.checkDispatchHTTPModeAllowed(context.Background(), run, job, orgID, true) {
		t.Fatal("expected HTTP-mode plan lookup error to block dispatch")
	}

	execStore.mu.Lock()
	statusCalls := append([]statusUpdateCall(nil), execStore.statusCalls...)
	execStore.mu.Unlock()
	if len(statusCalls) != 1 {
		t.Fatalf("status calls = %d, want 1", len(statusCalls))
	}
	if got := statusCalls[0].to; got != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", got, domain.StatusSystemFailed)
	}
	if val, err := mr.Get(monthlyKey); err == nil && val != "0" {
		t.Fatalf("monthly counter after rollback = %s, want 0 or missing", val)
	}
	if val, err := mr.Get(concurrentKey); err == nil && val != "0" {
		t.Fatalf("concurrent counter after rollback = %s, want 0 or missing", val)
	}
}
