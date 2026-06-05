//go:build integration

package scheduler_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/scheduler"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock stores for reaper optional-interface tests.
// These embed a base mock that satisfies scheduler.ReaperStore, plus
// optional interfaces that the reaper discovers via type assertions.

// baseReaperStore provides stub implementations for all ReaperStore methods.
type baseReaperStore struct{}

func (baseReaperStore) ListStaleRuns(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
	return nil, nil
}
func (baseReaperStore) ListExpiredRuns(_ context.Context) ([]domain.JobRun, error) {
	return nil, nil
}
func (baseReaperStore) ListStaleDequeued(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
	return nil, nil
}
func (baseReaperStore) ListTimedOutWorkflowRuns(_ context.Context) ([]domain.WorkflowRun, error) {
	return nil, nil
}
func (baseReaperStore) ListStepRunsByWorkflowRun(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
	return nil, nil
}
func (baseReaperStore) UpdateWorkflowRunStatus(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
	return nil
}
func (baseReaperStore) UpdateStepRunStatus(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
	return nil
}
func (baseReaperStore) GetRun(_ context.Context, _ string) (*domain.JobRun, error) {
	return nil, nil
}
func (baseReaperStore) ListExpiredWorkflowStepApprovals(_ context.Context) ([]domain.WorkflowStepApproval, error) {
	return nil, nil
}
func (baseReaperStore) GetStepRunByWorkflowRunAndRef(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
	return nil, nil
}
func (baseReaperStore) UpdateWorkflowStepApproval(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
	return nil
}
func (baseReaperStore) DeleteTerminalRunsPastRetention(_ context.Context, _, _ time.Duration) (int64, error) {
	return 0, nil
}
func (baseReaperStore) DeleteRunsByOrgOlderThan(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 0, nil
}
func (baseReaperStore) DeleteWorkflowRunsByOrgOlderThan(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 0, nil
}
func (baseReaperStore) UpdateRunStatus(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
	return nil
}
func (baseReaperStore) DeleteWorkflowRunsFinishedBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (baseReaperStore) ListExpiredEventTriggers(_ context.Context) ([]domain.EventTrigger, error) {
	return nil, nil
}
func (baseReaperStore) UpdateEventTriggerStatus(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
	return nil
}
func (baseReaperStore) CancelEventTriggersByWorkflowRun(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (baseReaperStore) CancelNonTerminalStepRuns(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
	return 0, nil
}
func (baseReaperStore) CancelJobRunsByWorkflowRun(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
	return 0, nil
}
func (baseReaperStore) ListReceivedEventTriggersWithStaleSteps(_ context.Context) ([]domain.EventTrigger, error) {
	return nil, nil
}
func (baseReaperStore) DeleteEventTriggersFinishedBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (baseReaperStore) DeleteAuditEventsBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (baseReaperStore) DeleteAuditEventsBeforeExcluding(_ context.Context, _ time.Time, _ []string) (int64, error) {
	return 0, nil
}
func (baseReaperStore) ListAuditRetentionOverrides(_ context.Context) ([]store.AuditRetentionOverride, error) {
	return nil, nil
}
func (baseReaperStore) ListAuditEventsDeadletter(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
	return nil, nil, nil
}
func (baseReaperStore) ListAuditEventsDeadletterWithAttempts(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
	return nil, nil, nil, nil
}
func (baseReaperStore) IncrementAuditDeadletterAttempt(_ context.Context, _ string) error {
	return nil
}
func (baseReaperStore) MarkAuditDeadletterReclaimed(_ context.Context, _, _ string) error {
	return nil
}
func (baseReaperStore) ReplayAuditEventDeadletter(_ context.Context, _, _, _ string) (*domain.AuditEvent, bool, error) {
	return nil, false, nil
}
func (baseReaperStore) DeleteAuditDeadletterOlderThan(_ context.Context, _ time.Time) (map[string]int64, error) {
	return nil, nil
}
func (baseReaperStore) CreateAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	return nil
}
func (baseReaperStore) DeleteAuditEventDeadletter(_ context.Context, _, _ string) error {
	return nil
}
func (baseReaperStore) ArchiveTerminalRunsPastRetention(_ context.Context, _, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}
func (baseReaperStore) DeleteHistoryRunsPastRetention(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (baseReaperStore) ArchiveConsumedOutboxBatch(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}
func (baseReaperStore) DeleteOutboxHistoryPastRetention(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (baseReaperStore) PurgeQuarantinedOutboxOlderThan(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (baseReaperStore) GetRunFromHistory(_ context.Context, _ string) (*domain.JobRun, error) {
	return nil, nil
}

// 1. monitorQueueDepth

// qdStore satisfies ReaperStore + QueueDepthMonitorStore.
type qdStore struct {
	baseReaperStore
	listFn func(ctx context.Context) ([]store.QueueJobDepth, error)
}

func (s *qdStore) ListQueueDepthByJob(ctx context.Context) ([]store.QueueJobDepth, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func TestIntegration_MonitorQueueDepth_WithItems(t *testing.T) {
	ctx := context.Background()
	called := atomic.Bool{}
	ms := &qdStore{
		listFn: func(_ context.Context) ([]store.QueueJobDepth, error) {
			called.Store(true)
			return []store.QueueJobDepth{
				{JobID: "job-1", QueuedCount: 42, QueueDepthAlertThreshold: 10},
			}, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(ctx)
	require.True(t, called.Load())

}

func TestIntegration_MonitorQueueDepth_Empty(t *testing.T) {
	ctx := context.Background()
	ms := &qdStore{
		listFn: func(_ context.Context) ([]store.QueueJobDepth, error) {
			return nil, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	// Should not panic with empty results.
	r.ReapOnce(ctx)
}

func TestIntegration_MonitorQueueDepth_StoreError(t *testing.T) {
	ctx := context.Background()
	ms := &qdStore{
		listFn: func(_ context.Context) ([]store.QueueJobDepth, error) {
			return nil, errSimulated
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	// Should not crash when the store returns an error.
	r.ReapOnce(ctx)
}

// 2. monitorDLQDepth

// dlqStore satisfies ReaperStore + DLQMonitorStore.
type dlqStore struct {
	baseReaperStore
	listFn func(ctx context.Context) ([]scheduler.DLQJobDepth, error)
}

func (s *dlqStore) ListDLQDepthByJob(ctx context.Context) ([]scheduler.DLQJobDepth, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func TestIntegration_MonitorDLQDepth_WithFailedRuns(t *testing.T) {
	ctx := context.Background()
	called := atomic.Bool{}
	ms := &dlqStore{
		listFn: func(_ context.Context) ([]scheduler.DLQJobDepth, error) {
			called.Store(true)
			return []scheduler.DLQJobDepth{
				{JobID: "job-dlq-1", WebhookURL: "https://example.com", DLQCount: 5, DLQAlertThreshold: 3},
			}, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(ctx)
	require.True(t, called.Load())

}

func TestIntegration_MonitorDLQDepth_Empty(t *testing.T) {
	ctx := context.Background()
	ms := &dlqStore{
		listFn: func(_ context.Context) ([]scheduler.DLQJobDepth, error) {
			return nil, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(ctx)
}

func TestIntegration_MonitorDLQDepth_StoreError(t *testing.T) {
	ctx := context.Background()
	ms := &dlqStore{
		listFn: func(_ context.Context) ([]scheduler.DLQJobDepth, error) {
			return nil, errSimulated
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(ctx)
}

// 3. reapOrphanedStepRuns

// reconciliationStore satisfies ReaperStore + ReconciliationStore.
type reconciliationStore struct {
	baseReaperStore
	listOrphansFn   func(ctx context.Context) ([]store.OrphanedStepRun, error)
	resetWebhooksFn func(ctx context.Context) (int64, error)
}

func (s *reconciliationStore) ListOrphanedStepRuns(ctx context.Context) ([]store.OrphanedStepRun, error) {
	if s.listOrphansFn != nil {
		return s.listOrphansFn(ctx)
	}
	return nil, nil
}

func (s *reconciliationStore) ResetStuckWebhookDeliveries(ctx context.Context) (int64, error) {
	if s.resetWebhooksFn != nil {
		return s.resetWebhooksFn(ctx)
	}
	return 0, nil
}

func TestIntegration_ReapOrphanedStepRuns_CompletedParent(t *testing.T) {
	ctx := context.Background()
	var completedCalls atomic.Int32
	wfCallback := &intMockWorkflowCallback{
		onStepCompletedFn: func(_ context.Context, wfRunID, stepRunID string) {
			assert.False(t, wfRunID !=
				"wfr-1" ||
				stepRunID !=
					"sr-1")

			completedCalls.Add(1)
		},
	}

	ms := &reconciliationStore{
		listOrphansFn: func(_ context.Context) ([]store.OrphanedStepRun, error) {
			return []store.OrphanedStepRun{
				{
					StepRunID:     "sr-1",
					StepRef:       "step-a",
					WorkflowRunID: "wfr-1",
					JobRunID:      "jr-1",
					JobStatus:     domain.StatusCompleted,
				},
			}, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, wfCallback)
	r.ReapOnce(ctx)
	require.EqualValues(t, 1, completedCalls.
		Load())

}

func TestIntegration_ReapOrphanedStepRuns_FailedParent(t *testing.T) {
	ctx := context.Background()
	var failedCalls atomic.Int32
	wfCallback := &intMockWorkflowCallback{
		onStepFailedFn: func(_ context.Context, wfRunID, stepRunID string) {
			assert.False(t, wfRunID !=
				"wfr-2" ||
				stepRunID !=
					"sr-2")

			failedCalls.Add(1)
		},
	}

	ms := &reconciliationStore{
		listOrphansFn: func(_ context.Context) ([]store.OrphanedStepRun, error) {
			return []store.OrphanedStepRun{
				{
					StepRunID:     "sr-2",
					StepRef:       "step-b",
					WorkflowRunID: "wfr-2",
					JobRunID:      "jr-2",
					JobStatus:     domain.StatusFailed,
				},
			}, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, wfCallback)
	r.ReapOnce(ctx)
	require.EqualValues(t, 1, failedCalls.
		Load())

}

func TestIntegration_ReapOrphanedStepRuns_NoOrphans(t *testing.T) {
	ctx := context.Background()
	ms := &reconciliationStore{
		listOrphansFn: func(_ context.Context) ([]store.OrphanedStepRun, error) {
			return nil, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	// No orphans means no callback invocations and no panic.
	r.ReapOnce(ctx)
}

// 4. reapStuckWebhookDeliveries

func TestIntegration_ReapStuckWebhookDeliveries_ResetsStuck(t *testing.T) {
	ctx := context.Background()
	var resetCount atomic.Int64
	ms := &reconciliationStore{
		resetWebhooksFn: func(_ context.Context) (int64, error) {
			resetCount.Add(3)
			return 3, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(ctx)
	require.EqualValues(t, 3, resetCount.
		Load(),
	)

}

func TestIntegration_ReapStuckWebhookDeliveries_NoneStuck(t *testing.T) {
	ctx := context.Background()
	ms := &reconciliationStore{
		resetWebhooksFn: func(_ context.Context) (int64, error) {
			return 0, nil
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(ctx)
}

func TestIntegration_ReapStuckWebhookDeliveries_StoreError(t *testing.T) {
	ctx := context.Background()
	ms := &reconciliationStore{
		resetWebhooksFn: func(_ context.Context) (int64, error) {
			return 0, errSimulated
		},
	}

	r := scheduler.NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	// Should log the error, not crash.
	r.ReapOnce(ctx)
}

// 5. StatsAggregator.Run

// intMockStatsStore implements scheduler.StatsAggregatorStore.
type intMockStatsStore struct {
	aggregateCalls     atomic.Int32
	aggregateCostCalls atomic.Int32
}

func (s *intMockStatsStore) AggregateHourlyStats(_ context.Context, _ time.Time) error {
	s.aggregateCalls.Add(1)
	return nil
}

func (s *intMockStatsStore) AggregateCostStatsHourly(_ context.Context, _ time.Time) error {
	s.aggregateCostCalls.Add(1)
	return nil
}

func TestIntegration_StatsAggregator_ContextCancellation(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ms := &intMockStatsStore{}
	agg := scheduler.NewStatsAggregator(ms)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		agg.Run(ctx)
		close(done)
	})

	// Cancel immediately -- the aggregator should exit without crashing.
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		require.Fail(t, "StatsAggregator.Run did not exit after context cancellation")
	}
}

// 6. recordCronDrift

func TestIntegration_RecordCronDrift_NilMetrics(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	q := intTestQueue(t)

	// CronScheduler with nil metrics should not panic when recording drift.
	cs := scheduler.NewCronScheduler(ctx, st, q, nil)
	// Create a cron job and trigger it; recordCronDrift is called inside triggerJob,
	// which runs when the cron fires. For a no-panic test, just load + start + stop.
	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

func TestIntegration_RecordCronDrift_ValidCronExpr(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	q := intTestQueue(t)

	// Create a cron job so LoadJobs picks it up.
	_ = intCreateJob(t, ctx, st, "proj-drift", func(j *domain.Job) {
		j.Cron = "* * * * *"
	})

	cs := scheduler.NewCronScheduler(ctx, st, q, nil)
	require.NoError(t, cs.LoadJobs(ctx))

	// Start and stop without waiting for a full minute --
	// the point is that LoadJobs + Start/Stop with a valid cron expr does not panic
	// even when metrics are nil.
	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

// 7. checkRunLimitWarnings

// intMockRunLimitStore implements scheduler.RunLimitStore.
type intMockRunLimitStore struct {
	orgIDs              []string
	projectsByOrg       map[string][]string
	channelsByProjectID map[string][]domain.NotificationChannel
	deliveries          []domain.NotificationDelivery
	deliveryCalls       atomic.Int32
}

func (s *intMockRunLimitStore) ListAllSubscribedOrgIDs(_ context.Context) ([]string, error) {
	return s.orgIDs, nil
}

func (s *intMockRunLimitStore) ListProjectsByOrg(_ context.Context, orgID string) ([]string, error) {
	return s.projectsByOrg[orgID], nil
}

func (s *intMockRunLimitStore) ListEnabledNotificationChannels(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
	return s.channelsByProjectID[projectID], nil
}

func (s *intMockRunLimitStore) ListEnabledNotificationChannelsByProjectIDs(_ context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
	result := make(map[string][]domain.NotificationChannel)
	for _, pid := range projectIDs {
		if chs, ok := s.channelsByProjectID[pid]; ok {
			result[pid] = chs
		}
	}
	return result, nil
}

func (s *intMockRunLimitStore) CreateNotificationDelivery(_ context.Context, d *domain.NotificationDelivery) error {
	s.deliveryCalls.Add(1)
	s.deliveries = append(s.deliveries, *d)
	return nil
}

// intMockEnforcer wraps the Check80PercentMonthlyWarning behavior
// without requiring a real billing.Enforcer (which needs Redis).
// Since checkRunLimitWarnings calls enforcer.Check80PercentMonthlyWarning
// and the BudgetMonitor expects a *billing.Enforcer, we test indirectly
// by running the BudgetMonitor with a short interval and verifying that
// the run-limit check path is exercised when the enforcer is nil (no crash).

func TestIntegration_CheckRunLimitWarnings_NilEnforcer(t *testing.T) {
	// When runLimitStore is set but enforcer is nil, checkRunLimitWarnings
	// should not be called (the guard: bm.runLimitStore != nil && bm.enforcer != nil).
	rlStore := &intMockRunLimitStore{
		orgIDs: []string{"org-1"},
		projectsByOrg: map[string][]string{
			"org-1": {"proj-1"},
		},
		channelsByProjectID: map[string][]domain.NotificationChannel{
			"proj-1": {{ID: "ch-1", ProjectID: "proj-1", ChannelType: "webhook", Enabled: true}},
		},
	}
	bm := scheduler.NewBudgetMonitor(struct{}{}, nil, 50*time.Millisecond).
		WithRunLimitNotifications(rlStore, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	bm.Run(ctx)
	require.EqualValues(t, 0, rlStore.
		deliveryCalls.
		Load(),
	)

}

// Shared helpers and mocks

var errSimulated = errSentinel("simulated store error")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// intMockWorkflowCallback implements scheduler.WorkflowCallback for tests.
type intMockWorkflowCallback struct {
	onJobRunTerminalFn func(ctx context.Context, run *domain.JobRun) error
	onEventReceivedFn  func(ctx context.Context, trigger *domain.EventTrigger) error
	onStepCompletedFn  func(ctx context.Context, workflowRunID string, stepRunID string)
	onStepFailedFn     func(ctx context.Context, workflowRunID string, stepRunID string)
	resumeWorkflowFn   func(ctx context.Context, workflowRunID string) error
	approveStepFn      func(ctx context.Context, workflowRunID, stepRef, approver string) error
}

func (m *intMockWorkflowCallback) OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error {
	if m.onJobRunTerminalFn != nil {
		return m.onJobRunTerminalFn(ctx, run)
	}
	return nil
}

func (m *intMockWorkflowCallback) OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error {
	if m.onEventReceivedFn != nil {
		return m.onEventReceivedFn(ctx, trigger)
	}
	return nil
}

func (m *intMockWorkflowCallback) OnStepCompleted(ctx context.Context, workflowRunID string, stepRunID string) {
	if m.onStepCompletedFn != nil {
		m.onStepCompletedFn(ctx, workflowRunID, stepRunID)
	}
}

func (m *intMockWorkflowCallback) OnStepFailed(ctx context.Context, workflowRunID string, stepRunID string) {
	if m.onStepFailedFn != nil {
		m.onStepFailedFn(ctx, workflowRunID, stepRunID)
	}
}

func (m *intMockWorkflowCallback) ResumeWorkflowRun(ctx context.Context, workflowRunID string) error {
	if m.resumeWorkflowFn != nil {
		return m.resumeWorkflowFn(ctx, workflowRunID)
	}
	return nil
}

func (m *intMockWorkflowCallback) ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error {
	if m.approveStepFn != nil {
		return m.approveStepFn(ctx, workflowRunID, stepRef, approver)
	}
	return nil
}
