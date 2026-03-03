package scheduler

import (
	"context"
	"time"

	"orchestrator/internal/domain"
)

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
	updateRunStatusFn   func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
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
