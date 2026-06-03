package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
)

type mockSchedulerStore struct {
	cron   *mockCronStore
	poller *mockPollerStore
	reaper *mockReaperStore
	index  *mockIndexMaintenanceStore
}

func (m *mockSchedulerStore) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	return m.cron.ListCronJobs(ctx)
}

func (m *mockSchedulerStore) ListCronWorkflows(ctx context.Context) ([]domain.Workflow, error) {
	return m.cron.ListCronWorkflows(ctx)
}

func (m *mockSchedulerStore) CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error) {
	return m.cron.CountRunningWorkflowRuns(ctx, workflowID)
}

func (m *mockSchedulerStore) CountActiveRunsForJob(ctx context.Context, jobID string) (int, error) {
	return m.cron.CountActiveRunsForJob(ctx, jobID)
}

func (m *mockSchedulerStore) CancelActiveRunsForJob(ctx context.Context, jobID string, reason string) ([]store.CanceledRun, error) {
	return m.cron.CancelActiveRunsForJob(ctx, jobID, reason)
}

func (m *mockSchedulerStore) CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error) {
	return m.cron.CancelChildRunsByParentIDs(ctx, parentIDs, finishedAt, reason)
}

func (m *mockSchedulerStore) TryAcquireCronFire(ctx context.Context, projectID string, key string, ttl time.Duration) (bool, error) {
	return m.cron.TryAcquireCronFire(ctx, projectID, key, ttl)
}

func (m *mockSchedulerStore) DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	return m.reaper.DeleteWorkflowRunsFinishedBefore(ctx, before, limit)
}

func (m *mockSchedulerStore) ActivateDueRuns(ctx context.Context, limit int) (int64, error) {
	return m.poller.ActivateDueRuns(ctx, limit)
}

func (m *mockSchedulerStore) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	return m.reaper.ListStaleRuns(ctx, threshold)
}

func (m *mockSchedulerStore) ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error) {
	return m.reaper.ListExpiredRuns(ctx)
}

func (m *mockSchedulerStore) ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	return m.reaper.ListStaleDequeued(ctx, threshold)
}

func (m *mockSchedulerStore) ListTimedOutWorkflowRuns(ctx context.Context) ([]domain.WorkflowRun, error) {
	return m.reaper.ListTimedOutWorkflowRuns(ctx)
}

func (m *mockSchedulerStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
	return m.reaper.ListStepRunsByWorkflowRun(ctx, workflowRunID, limit, cursor)
}

func (m *mockSchedulerStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	return m.reaper.UpdateWorkflowRunStatus(ctx, id, from, to, fields)
}

func (m *mockSchedulerStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	return m.reaper.UpdateStepRunStatus(ctx, id, status, fields)
}

func (m *mockSchedulerStore) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	return m.reaper.GetRun(ctx, id)
}

func (m *mockSchedulerStore) ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error) {
	return m.reaper.ListExpiredWorkflowStepApprovals(ctx)
}

func (m *mockSchedulerStore) GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
	return m.reaper.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
}

func (m *mockSchedulerStore) UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
	return m.reaper.UpdateWorkflowStepApproval(ctx, id, status, approvedBy, approvedAt, errMsg)
}

func (m *mockSchedulerStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	return m.reaper.UpdateRunStatus(ctx, id, from, to, fields)
}

func (m *mockSchedulerStore) ScheduleRetry(_ context.Context, _ string, _ time.Time, _ int) error {
	return nil
}

func (m *mockSchedulerStore) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	return m.reaper.DeleteTerminalRunsPastRetention(ctx, shortRetention, longRetention)
}

func (m *mockSchedulerStore) DeleteRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error) {
	return m.reaper.DeleteRunsByOrgOlderThan(ctx, orgID, retention)
}

func (m *mockSchedulerStore) DeleteWorkflowRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error) {
	return m.reaper.DeleteWorkflowRunsByOrgOlderThan(ctx, orgID, retention)
}

func (m *mockSchedulerStore) ListExpiredEventTriggers(ctx context.Context) ([]domain.EventTrigger, error) {
	return m.reaper.ListExpiredEventTriggers(ctx)
}

func (m *mockSchedulerStore) UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error {
	return m.reaper.UpdateEventTriggerStatus(ctx, id, status, responsePayload, receivedAt, errMsg)
}

func (m *mockSchedulerStore) CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error) {
	return m.reaper.CancelEventTriggersByWorkflowRun(ctx, workflowRunID)
}

func (m *mockSchedulerStore) ListReceivedEventTriggersWithStaleSteps(ctx context.Context) ([]domain.EventTrigger, error) {
	return m.reaper.ListReceivedEventTriggersWithStaleSteps(ctx)
}

func (m *mockSchedulerStore) DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	return m.reaper.DeleteEventTriggersFinishedBefore(ctx, before, limit)
}

func (m *mockSchedulerStore) ReindexIndexConcurrently(ctx context.Context, indexName string) error {
	if m.index == nil {
		return nil
	}
	return m.index.ReindexIndexConcurrently(ctx, indexName)
}

func (m *mockSchedulerStore) CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	return m.reaper.CancelNonTerminalStepRuns(ctx, workflowRunID, finishedAt, reason)
}

func (m *mockSchedulerStore) CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	return m.reaper.CancelJobRunsByWorkflowRun(ctx, workflowRunID, finishedAt, reason)
}

// DebounceStore methods (no-op for tests).
func (m *mockSchedulerStore) ListDueDebouncePending(_ context.Context) ([]domain.DebouncePending, error) {
	return nil, nil
}
func (m *mockSchedulerStore) DeleteDebouncePending(_ context.Context, _ string) error { return nil }
func (m *mockSchedulerStore) ClaimDueDebouncePending(_ context.Context, _ string) (*domain.DebouncePending, bool, error) {
	return nil, false, nil
}
func (m *mockSchedulerStore) CompleteDebouncePending(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}
func (m *mockSchedulerStore) RescheduleDebouncePending(_ context.Context, _ string, _, _ time.Time) (bool, error) {
	return false, nil
}
func (m *mockSchedulerStore) InsertDebouncePendingIfAbsent(_ context.Context, _ *domain.DebouncePending) (bool, error) {
	return false, nil
}
func (m *mockSchedulerStore) GetJob(_ context.Context, _ string) (*domain.Job, error) {
	return nil, nil
}
func (m *mockSchedulerStore) GetProjectQuota(context.Context, string) (*store.ProjectQuota, error) {
	return nil, nil
}
func (m *mockSchedulerStore) CountProjectQueuedRuns(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockSchedulerStore) CountProjectActiveRuns(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockSchedulerStore) CountRunsForJobSince(context.Context, string, time.Time) (int, error) {
	return 0, nil
}
func (m *mockSchedulerStore) SumProjectDailyCostMicrousd(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (m *mockSchedulerStore) CreateRun(_ context.Context, _ *domain.JobRun) error { return nil }
func (m *mockSchedulerStore) TryAdvisoryLock(_ context.Context, _ int64) (bool, error) {
	return false, nil
}
func (m *mockSchedulerStore) ReleaseAdvisoryLock(_ context.Context, _ int64) error { return nil }

// BatchStore methods (no-op for tests).
func (m *mockSchedulerStore) ListFlushableBatches(_ context.Context) ([]store.FlushableBatch, error) {
	return nil, nil
}
func (m *mockSchedulerStore) DrainBatchBuffer(_ context.Context, _, _ string, _ int) ([]domain.BatchBufferItem, error) {
	return nil, nil
}
func (m *mockSchedulerStore) ListBatchBufferItems(_ context.Context, _, _ string, _ int) ([]domain.BatchBufferItem, error) {
	return nil, nil
}
func (m *mockSchedulerStore) DeleteBatchBufferItems(_ context.Context, _ []string) error {
	return nil
}

// StatsAggregatorStore methods (no-op for tests).
func (m *mockSchedulerStore) AggregateHourlyStats(_ context.Context, _ time.Time) error {
	return nil
}

func (m *mockSchedulerStore) AggregateCostStatsHourly(_ context.Context, _ time.Time) error {
	return nil
}

// CostEstimateRefresherStore methods (no-op for tests).
func (m *mockSchedulerStore) ListActiveJobIDs(_ context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockSchedulerStore) UpsertJobCostEstimate(_ context.Context, _ string) error { return nil }
func (m *mockSchedulerStore) DeleteExpiredJobMemory(_ context.Context) (int64, error) {
	return 0, nil
}
func (m *mockSchedulerStore) ListStalledWorkflowRuns(_ context.Context, _ time.Duration) ([]domain.WorkflowRun, error) {
	return nil, nil
}
func (m *mockSchedulerStore) DeleteAuditEventsBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockSchedulerStore) DeleteAuditEventsBeforeExcluding(_ context.Context, _ time.Time, _ []string) (int64, error) {
	return 0, nil
}
func (m *mockSchedulerStore) ListAuditRetentionOverrides(_ context.Context) ([]store.AuditRetentionOverride, error) {
	return nil, nil
}
func (m *mockSchedulerStore) ListAuditEventsDeadletter(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
	return nil, nil, nil
}
func (m *mockSchedulerStore) CreateAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	return nil
}
func (m *mockSchedulerStore) DeleteAuditEventDeadletter(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockSchedulerStore) ListAuditEventsDeadletterWithAttempts(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
	return nil, nil, nil, nil
}
func (m *mockSchedulerStore) IncrementAuditDeadletterAttempt(_ context.Context, _ string) error {
	return nil
}
func (m *mockSchedulerStore) MarkAuditDeadletterReclaimed(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockSchedulerStore) ReplayAuditEventDeadletter(_ context.Context, _, _, _ string) (*domain.AuditEvent, bool, error) {
	return nil, false, nil
}
func (m *mockSchedulerStore) DeleteAuditDeadletterOlderThan(_ context.Context, _ time.Time) (map[string]int64, error) {
	return nil, nil
}

func (m *mockSchedulerStore) ArchiveTerminalRunsPastRetention(_ context.Context, _, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}

func (m *mockSchedulerStore) DeleteHistoryRunsPastRetention(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *mockSchedulerStore) ArchiveConsumedOutboxBatch(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}

func (m *mockSchedulerStore) DeleteOutboxHistoryPastRetention(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *mockSchedulerStore) PurgeQuarantinedOutboxOlderThan(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *mockSchedulerStore) GetRunFromHistory(_ context.Context, _ string) (*domain.JobRun, error) {
	return nil, nil
}

func testSchedulerConfig() *config.Config {
	return &config.Config{
		PollerInterval:           100 * time.Millisecond,
		ReaperInterval:           100 * time.Millisecond,
		StaleThreshold:           30 * time.Second,
		IndexMaintenanceInterval: time.Hour,
	}
}

func TestScheduler_New(t *testing.T) {
	t.Parallel()
	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}

	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	if s == nil {
		t.Fatal("expected scheduler to be non-nil")
	}
}

func TestScheduler_Components_RegistersRequiredLoops(t *testing.T) {
	t.Parallel()

	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}
	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)

	components := s.components()
	names := schedulerComponentNames(components)
	required := []string{
		"cron_reloader",
		"poller",
		"reaper",
		"index_maintainer",
		"debounce_poller",
		"batch_flusher",
		"stats_aggregator",
		"budget_monitor",
		"memory_cleanup",
	}
	for _, name := range required {
		if !names[name] {
			t.Fatalf("expected component %q to be registered", name)
		}
	}
	for i, name := range required {
		if components[i].name != name {
			t.Fatalf("component %d = %q, want %q", i, components[i].name, name)
		}
	}
}

func TestScheduler_Components_SkipsUnsetOptionalLoops(t *testing.T) {
	t.Parallel()

	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}
	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)

	names := schedulerComponentNames(s.components())
	for _, name := range []string{
		"usage_flusher",
		"slo_evaluator",
		"concurrent_reconciler",
		"heartbeat_gc",
	} {
		if names[name] {
			t.Fatalf("component %q registered without being configured", name)
		}
	}
}

func TestScheduler_TrackComponents_SkipsInvalidComponents(t *testing.T) {
	t.Parallel()

	s := &Scheduler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ran := make(chan string, 1)
	s.trackComponents(ctx, []schedulerComponent{
		{},
		{name: "missing_run"},
		{run: func(context.Context) {}},
		{
			name: "valid",
			run: func(ctx context.Context) {
				ran <- "valid"
				<-ctx.Done()
			},
		},
	})

	select {
	case got := <-ran:
		if got != "valid" {
			t.Fatalf("unexpected component ran: %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("valid component did not run")
	}

	cancel()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tracked component did not stop")
	}
}

func schedulerComponentNames(components []schedulerComponent) map[string]bool {
	names := make(map[string]bool, len(components))
	for _, component := range components {
		names[component.name] = true
	}
	return names
}

func TestWithBudgetMonitoringStores_WiresSpendingStore(t *testing.T) {
	t.Parallel()

	spending := &mockSpendingLimitStore{}
	s := &Scheduler{budgetMonitor: NewBudgetMonitor(struct{}{}, nil, time.Minute)}

	WithBudgetMonitoringStores(spending, nil, nil)(s)

	if s.budgetMonitor.spendingStore != spending {
		t.Fatal("expected spending store to be wired into budget monitor")
	}
}

func TestWithSLOEvaluator_WiresSchedulerComponent(t *testing.T) {
	t.Parallel()

	evaluator := &SLOEvaluator{}
	s := &Scheduler{}

	WithSLOEvaluator(evaluator)(s)

	if s.sloEvaluator != evaluator {
		t.Fatal("expected SLO evaluator to be wired into scheduler")
	}
}

func TestWithHeartbeatGC_WiresSchedulerComponent(t *testing.T) {
	t.Parallel()

	gc := &HeartbeatGC{}
	s := &Scheduler{}

	WithHeartbeatGC(gc)(s)

	if s.heartbeatGC != gc {
		t.Fatal("expected heartbeat GC to be wired into scheduler")
	}
}

func TestWithGracePeriodEnforcer_WiresSchedulerComponent(t *testing.T) {
	t.Parallel()

	enforcer := &GracePeriodEnforcer{}
	s := &Scheduler{}

	WithGracePeriodEnforcer(enforcer)(s)

	if s.gracePeriodEnforcer != enforcer {
		t.Fatal("expected grace period enforcer to be wired into scheduler")
	}
}

func TestScheduler_Start_Success(t *testing.T) {
	t.Parallel()
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return []domain.Job{}, nil },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}

	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	cancel()
	s.Stop()
}

func TestScheduler_Start_LoadJobsError(t *testing.T) {
	t.Parallel()
	storeErr := errors.New("list failed")
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return nil, storeErr },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}

	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "load cron jobs") {
		t.Fatalf("expected load cron jobs error, got %v", err)
	}
}

func TestScheduler_Stop(t *testing.T) {
	t.Parallel()
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return []domain.Job{}, nil },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}

	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	cancel()
	s.Stop()
}

func TestScheduler_Stop_CompletesWithinTimeout(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return []domain.Job{}, nil },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}

	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	cancel()

	done := make(chan struct{})
	concWG.Go(func() {
		s.Stop()
		close(done)
	})

	select {
	case <-done:
		// Stop completed without deadlock.
	case <-time.After(10 * time.Second):
		t.Fatal("Scheduler.Stop() did not complete within 10s, possible deadlock")
	}
}

func TestScheduler_Stop_CalledTwice_NoPanic(t *testing.T) {
	t.Parallel()
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return []domain.Job{}, nil },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}

	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	cancel()
	s.Stop()
	// Second Stop should not panic.
	s.Stop()
}

func TestScheduler_Start_LoadJobsError_Wrapped(t *testing.T) {
	t.Parallel()
	storeErr := errors.New("connection refused")
	store := &mockSchedulerStore{
		cron: &mockCronStore{
			listCronJobsFn: func(context.Context) ([]domain.Job, error) { return nil, storeErr },
		},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}

	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "load cron jobs") {
		t.Errorf("error should wrap with 'load cron jobs', got: %v", err)
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}
