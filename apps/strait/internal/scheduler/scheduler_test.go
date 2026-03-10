package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
)

type mockSchedulerStore struct {
	cron   *mockCronStore
	poller *mockPollerStore
	reaper *mockReaperStore
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

func (m *mockSchedulerStore) DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	return m.reaper.DeleteWorkflowRunsFinishedBefore(ctx, before, limit)
}

func (m *mockSchedulerStore) ListDueRuns(ctx context.Context) ([]domain.JobRun, error) {
	return m.poller.ListDueRuns(ctx)
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
	return m.poller.UpdateRunStatus(ctx, id, from, to, fields)
}

func (m *mockSchedulerStore) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	return m.reaper.DeleteTerminalRunsPastRetention(ctx, shortRetention, longRetention)
}

func testSchedulerConfig() *config.Config {
	return &config.Config{
		PollerInterval: 100 * time.Millisecond,
		ReaperInterval: 100 * time.Millisecond,
		StaleThreshold: 30 * time.Second,
	}
}

func TestScheduler_New(t *testing.T) {
	t.Parallel()
	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil, nil)
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
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil, nil)
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
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil, nil)
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
	}

	s := New(testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	cancel()
	s.Stop()
}
