package scheduler

import (
	"context"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/queue"
)

var _ queue.Queue = (*mockQueue)(nil)

// mockPollerStore implements PollerStore for testing.
type mockPollerStore struct {
	listDueRunsFn     func(ctx context.Context) ([]domain.JobRun, error)
	updateRunStatusFn func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
}

func (m *mockPollerStore) ListDueRuns(ctx context.Context) ([]domain.JobRun, error) {
	if m.listDueRunsFn != nil {
		return m.listDueRunsFn(ctx)
	}
	return nil, nil
}

func (m *mockPollerStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

// mockReaperStore implements ReaperStore for testing.
type mockReaperStore struct {
	listStaleRunsFn           func(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	listExpiredRunsFn         func(ctx context.Context) ([]domain.JobRun, error)
	listStaleDequeuedFn       func(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	listTimedOutWfRunsFn      func(ctx context.Context) ([]domain.WorkflowRun, error)
	listStepRunsByWfRunFn     func(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	updateWorkflowRunStatusFn func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	updateStepRunStatusFn     func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	getRunFn                  func(ctx context.Context, id string) (*domain.JobRun, error)
	listExpiredApprovalsFn    func(ctx context.Context) ([]domain.WorkflowStepApproval, error)
	getStepRunByRunAndRefFn   func(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	updateWorkflowApprovalFn  func(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	updateRunStatusFn         func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	deleteOldWorkflowRunsFn   func(ctx context.Context, before time.Time, limit int) (int64, error)
	deleteRetentionFn         func(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error)
}

type mockCronStore struct {
	listCronJobsFn       func(ctx context.Context) ([]domain.Job, error)
	listCronWorkflowsFn  func(ctx context.Context) ([]domain.Workflow, error)
	countRunningWfRunsFn func(ctx context.Context, workflowID string) (int, error)
}

func (m *mockCronStore) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	if m.listCronJobsFn != nil {
		return m.listCronJobsFn(ctx)
	}
	return nil, nil
}

func (m *mockCronStore) ListCronWorkflows(ctx context.Context) ([]domain.Workflow, error) {
	if m.listCronWorkflowsFn != nil {
		return m.listCronWorkflowsFn(ctx)
	}
	return nil, nil
}

func (m *mockCronStore) CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error) {
	if m.countRunningWfRunsFn != nil {
		return m.countRunningWfRunsFn(ctx, workflowID)
	}
	return 0, nil
}

type mockQueue struct {
	enqueueFn           func(ctx context.Context, run *domain.JobRun) error
	dequeueFn           func(ctx context.Context) (*domain.JobRun, error)
	dequeueNFn          func(ctx context.Context, n int) ([]domain.JobRun, error)
	dequeueNByProjectFn func(ctx context.Context, n int, projectID string) ([]domain.JobRun, error)
}

func (m *mockQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, run)
	}
	return nil
}

func (m *mockQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	if m.dequeueFn != nil {
		return m.dequeueFn(ctx)
	}
	return nil, nil
}

func (m *mockQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	if m.dequeueNFn != nil {
		return m.dequeueNFn(ctx, n)
	}
	return nil, nil
}

func (m *mockQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	if m.dequeueNByProjectFn != nil {
		return m.dequeueNByProjectFn(ctx, n, projectID)
	}
	return nil, nil
}

func (m *mockReaperStore) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	if m.listStaleRunsFn != nil {
		return m.listStaleRunsFn(ctx, threshold)
	}
	return nil, nil
}

func (m *mockReaperStore) ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error) {
	if m.listExpiredRunsFn != nil {
		return m.listExpiredRunsFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	if m.listStaleDequeuedFn != nil {
		return m.listStaleDequeuedFn(ctx, threshold)
	}
	return nil, nil
}

func (m *mockReaperStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockReaperStore) ListTimedOutWorkflowRuns(ctx context.Context) ([]domain.WorkflowRun, error) {
	if m.listTimedOutWfRunsFn != nil {
		return m.listTimedOutWfRunsFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByWfRunFn != nil {
		return m.listStepRunsByWfRunFn(ctx, workflowRunID)
	}
	return nil, nil
}

func (m *mockReaperStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	if m.updateWorkflowRunStatusFn != nil {
		return m.updateWorkflowRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockReaperStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, status, fields)
	}
	return nil
}

func (m *mockReaperStore) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	if m.getRunFn != nil {
		return m.getRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockReaperStore) ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error) {
	if m.listExpiredApprovalsFn != nil {
		return m.listExpiredApprovalsFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
	if m.getStepRunByRunAndRefFn != nil {
		return m.getStepRunByRunAndRefFn(ctx, workflowRunID, stepRef)
	}
	return nil, nil
}

func (m *mockReaperStore) UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
	if m.updateWorkflowApprovalFn != nil {
		return m.updateWorkflowApprovalFn(ctx, id, status, approvedBy, approvedAt, errMsg)
	}
	return nil
}

func (m *mockReaperStore) DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if m.deleteOldWorkflowRunsFn != nil {
		return m.deleteOldWorkflowRunsFn(ctx, before, limit)
	}
	return 0, nil
}

func (m *mockReaperStore) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	if m.deleteRetentionFn != nil {
		return m.deleteRetentionFn(ctx, shortRetention, longRetention)
	}
	return 0, nil
}
