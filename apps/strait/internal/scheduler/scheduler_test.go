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

func (m *mockSchedulerStore) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	return m.reaper.DeleteTerminalRunsPastRetention(ctx, shortRetention, longRetention)
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
func (m *mockSchedulerStore) GetJob(_ context.Context, _ string) (*domain.Job, error) {
	return nil, nil
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
