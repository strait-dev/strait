package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/internal/domain"
)

func TestReaper_ReapStale(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusExecuting},
			}, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from != domain.StatusExecuting {
				t.Errorf("expected from=executing, got %s", from)
			}
			if to != domain.StatusCrashed {
				t.Errorf("expected to=crashed, got %s", to)
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapStale(context.Background())

	if transitioned.Load() != 2 {
		t.Fatalf("expected 2 status transitions, got %d", transitioned.Load())
	}
}

func TestReaper_ReapExpired(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusQueued},
			}, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			if to != domain.StatusExpired {
				t.Errorf("expected to=expired, got %s", to)
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapExpired(context.Background())

	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 status transition, got %d", transitioned.Load())
	}
}

func TestReaper_ReapStaleDequeued(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusDequeued},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from != domain.StatusDequeued {
				t.Errorf("expected from=dequeued, got %s", from)
			}
			if to != domain.StatusQueued {
				t.Errorf("expected to=queued, got %s", to)
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapStaleDequeued(context.Background())

	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 status transition, got %d", transitioned.Load())
	}
}

func TestReaper_NoStaleRuns(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapStale(context.Background())
	r.reapExpired(context.Background())
	r.reapStaleDequeued(context.Background())

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
}

func TestReaper_RunLoop(t *testing.T) {
	var ticked atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			ticked.Add(1)
			return nil, nil
		},
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, nil
		},
	}

	r := NewReaper(ms, 50*time.Millisecond, 30*time.Second, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	r.Run(ctx)

	if ticked.Load() < 1 {
		t.Fatalf("expected at least 1 tick, got %d", ticked.Load())
	}
}

func TestReaper_ReapStale_ListError(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, errors.New("list stale failed")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapStale(context.Background())

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
}

func TestReaper_ReapStale_UpdateError(t *testing.T) {
	var transitioned atomic.Int32
	var updateCalls atomic.Int32
	ms := &mockReaperStore{
		listStaleRunsFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusExecuting},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			if id == "run-1" {
				return errors.New("update failed")
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapStale(context.Background())

	if updateCalls.Load() != 2 {
		t.Fatalf("expected 2 update attempts, got %d", updateCalls.Load())
	}
	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 successful transition, got %d", transitioned.Load())
	}
}

func TestReaper_ReapExpired_ListError(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, errors.New("list expired failed")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapExpired(context.Background())

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
}

func TestReaper_ReapExpired_UpdateError(t *testing.T) {
	var transitioned atomic.Int32
	var updateCalls atomic.Int32
	ms := &mockReaperStore{
		listExpiredRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusQueued},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusExecuting},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			if id == "run-1" {
				return errors.New("update failed")
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapExpired(context.Background())

	if updateCalls.Load() != 2 {
		t.Fatalf("expected 2 update attempts, got %d", updateCalls.Load())
	}
	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 successful transition, got %d", transitioned.Load())
	}
}

func TestReaper_ReapStaleDequeued_ListError(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockReaperStore{
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return nil, errors.New("list stale dequeued failed")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapStaleDequeued(context.Background())

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 status transitions, got %d", transitioned.Load())
	}
}

func TestReaper_ReapStaleDequeued_UpdateError(t *testing.T) {
	var transitioned atomic.Int32
	var updateCalls atomic.Int32
	ms := &mockReaperStore{
		listStaleDequeuedFn: func(_ context.Context, _ time.Duration) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusDequeued},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusDequeued},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			if id == "run-1" {
				return errors.New("update failed")
			}
			transitioned.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapStaleDequeued(context.Background())

	if updateCalls.Load() != 2 {
		t.Fatalf("expected 2 update attempts, got %d", updateCalls.Load())
	}
	if transitioned.Load() != 1 {
		t.Fatalf("expected 1 successful transition, got %d", transitioned.Load())
	}
}

func TestReaper_ReapTerminalRetention(t *testing.T) {
	var called atomic.Int32
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
			if shortRetention != 30*24*time.Hour {
				t.Fatalf("short retention = %v, want %v", shortRetention, 30*24*time.Hour)
			}
			if longRetention != 90*24*time.Hour {
				t.Fatalf("long retention = %v, want %v", longRetention, 90*24*time.Hour)
			}
			called.Add(1)
			return 2, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapTerminalRetention(context.Background())

	if called.Load() != 1 {
		t.Fatalf("retention call count = %d, want 1", called.Load())
	}
}
