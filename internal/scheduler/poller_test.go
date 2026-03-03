package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/internal/domain"
)

func TestDelayedPoller_TransitionsDueRuns(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockPollerStore{
		listDueRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-1", JobID: "job-1", Status: domain.StatusDelayed},
				{ID: "run-2", JobID: "job-2", Status: domain.StatusDelayed},
			}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from != domain.StatusDelayed {
				t.Errorf("expected from=delayed, got %s", from)
			}
			if to != domain.StatusQueued {
				t.Errorf("expected to=queued, got %s", to)
			}
			transitioned.Add(1)
			return nil
		},
	}

	p := NewDelayedPoller(ms, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if transitioned.Load() < 2 {
		t.Fatalf("expected at least 2 transitions, got %d", transitioned.Load())
	}
}

func TestDelayedPoller_NoDueRuns(t *testing.T) {
	var transitioned atomic.Int32
	ms := &mockPollerStore{
		listDueRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return nil, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			transitioned.Add(1)
			return nil
		},
	}

	p := NewDelayedPoller(ms, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if transitioned.Load() != 0 {
		t.Fatalf("expected 0 transitions, got %d", transitioned.Load())
	}
}

func TestDelayedPoller_ListDueRunsError(t *testing.T) {
	var listCalls atomic.Int32
	ms := &mockPollerStore{
		listDueRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			listCalls.Add(1)
			return nil, errors.New("list failed")
		},
	}

	p := NewDelayedPoller(ms, 30*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if listCalls.Load() < 2 {
		t.Fatalf("expected poller to continue after errors, got %d list calls", listCalls.Load())
	}
}

func TestDelayedPoller_UpdateRunStatusError(t *testing.T) {
	var updateCalls atomic.Int32
	ms := &mockPollerStore{
		listDueRunsFn: func(_ context.Context) ([]domain.JobRun, error) {
			return []domain.JobRun{{ID: "run-1", JobID: "job-1", Status: domain.StatusDelayed}}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			return errors.New("update failed")
		},
	}

	p := NewDelayedPoller(ms, 30*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if updateCalls.Load() < 2 {
		t.Fatalf("expected poller to continue after update errors, got %d update calls", updateCalls.Load())
	}
}
