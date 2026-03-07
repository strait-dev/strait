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
	listStaleRunsFn     func(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	listExpiredRunsFn   func(ctx context.Context) ([]domain.JobRun, error)
	listStaleDequeuedFn func(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	deleteRetentionFn   func(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error)
	updateRunStatusFn   func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
}

type mockCronStore struct {
	listCronJobsFn func(ctx context.Context) ([]domain.Job, error)
}

func (m *mockCronStore) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	if m.listCronJobsFn != nil {
		return m.listCronJobsFn(ctx)
	}
	return nil, nil
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

func (m *mockReaperStore) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	if m.deleteRetentionFn != nil {
		return m.deleteRetentionFn(ctx, shortRetention, longRetention)
	}
	return 0, nil
}
