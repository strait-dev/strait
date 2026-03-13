package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestDelayedPoller_ActivatesDueRuns(t *testing.T) {
	t.Parallel()
	var totalActivated atomic.Int64
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, limit int) (int64, error) {
			if limit != 1000 {
				t.Errorf("expected limit=1000, got %d", limit)
			}
			totalActivated.Add(2)
			return 2, nil
		},
	}

	p := NewDelayedPoller(ms, slog.Default(), 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if totalActivated.Load() < 2 {
		t.Fatalf("expected at least 2 activated runs, got %d", totalActivated.Load())
	}
}

func TestDelayedPoller_NoDueRuns(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, _ int) (int64, error) {
			calls.Add(1)
			return 0, nil
		},
	}

	p := NewDelayedPoller(ms, slog.Default(), 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if calls.Load() == 0 {
		t.Fatal("expected at least one call to ActivateDueRuns")
	}
}

func TestDelayedPoller_ActivateError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, _ int) (int64, error) {
			calls.Add(1)
			return 0, errors.New("activate failed")
		},
	}

	p := NewDelayedPoller(ms, slog.Default(), 30*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if calls.Load() < 2 {
		t.Fatalf("expected poller to continue after errors, got %d calls", calls.Load())
	}
}
