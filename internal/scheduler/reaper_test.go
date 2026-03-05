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

func TestReaper_ReapOldWorkflowRuns(t *testing.T) {
	var deleted atomic.Int64
	ms := &mockReaperStore{
		deleteOldWorkflowRunsFn: func(_ context.Context, before time.Time, limit int) (int64, error) {
			if limit <= 0 {
				t.Fatalf("expected positive limit, got %d", limit)
			}
			if before.IsZero() {
				t.Fatal("expected non-zero before timestamp")
			}
			deleted.Store(3)
			return 3, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapOldWorkflowRuns(context.Background())

	if deleted.Load() != 3 {
		t.Fatalf("expected deleted count 3, got %d", deleted.Load())
	}
}

func TestReaper_ReapTimedOutWorkflows(t *testing.T) {
	var wfUpdates atomic.Int32
	var stepUpdates atomic.Int32
	var runUpdates atomic.Int32

	ms := &mockReaperStore{
		listTimedOutWfRunsFn: func(_ context.Context) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
			if id != "wr-1" {
				t.Fatalf("unexpected workflow run id %q", id)
			}
			if from != domain.WfStatusRunning || to != domain.WfStatusTimedOut {
				t.Fatalf("unexpected workflow transition %s -> %s", from, to)
			}
			wfUpdates.Add(1)
			return nil
		},
		listStepRunsByWfRunFn: func(_ context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
			if workflowRunID != "wr-1" {
				t.Fatalf("unexpected workflowRunID %q", workflowRunID)
			}
			return []domain.WorkflowStepRun{{ID: "sr-1", Status: domain.StepRunning, JobRunID: "run-1"}}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id != "sr-1" || status != domain.StepCanceled {
				t.Fatalf("unexpected step update %s -> %s", id, status)
			}
			stepUpdates.Add(1)
			return nil
		},
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id != "run-1" {
				t.Fatalf("unexpected run id %q", id)
			}
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			if id != "run-1" || from != domain.StatusExecuting || to != domain.StatusCanceled {
				t.Fatalf("unexpected run update %s %s -> %s", id, from, to)
			}
			runUpdates.Add(1)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, nil)
	r.reapTimedOutWorkflows(context.Background())

	if wfUpdates.Load() != 1 {
		t.Fatalf("expected 1 workflow update, got %d", wfUpdates.Load())
	}
	if stepUpdates.Load() != 1 {
		t.Fatalf("expected 1 step update, got %d", stepUpdates.Load())
	}
	if runUpdates.Load() != 1 {
		t.Fatalf("expected 1 job run update, got %d", runUpdates.Load())
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
