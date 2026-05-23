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

func TestDelayedPoller_DrainsBoundedPagesPerTick(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var totalActivated atomic.Int64
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, limit int) (int64, error) {
			if limit != 3 {
				t.Errorf("expected limit=3, got %d", limit)
			}
			call := calls.Add(1)
			if call <= 2 {
				totalActivated.Add(3)
				return 3, nil
			}
			return 0, nil
		},
	}

	p := NewDelayedPoller(ms, slog.Default(), time.Hour).
		WithBatchLimit(3).
		WithMaxBatchesPerTick(4)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p.poll(context.Background())
	p.Run(ctx)

	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
	if totalActivated.Load() != 6 {
		t.Fatalf("total activated = %d, want 6", totalActivated.Load())
	}
}

func TestDelayedPoller_StopsAfterMaxBatchesPerTick(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, _ int) (int64, error) {
			calls.Add(1)
			return 5, nil
		},
	}

	p := NewDelayedPoller(ms, slog.Default(), time.Hour).
		WithBatchLimit(5).
		WithMaxBatchesPerTick(3)

	p.poll(context.Background())

	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want bounded 3", calls.Load())
	}
}

func TestDelayedPoller_ClampsUnsafeDefaults(t *testing.T) {
	t.Parallel()
	p := NewDelayedPoller(&mockPollerStore{}, nil, 0)

	if p.interval <= 0 {
		t.Fatalf("interval = %v, want positive default", p.interval)
	}
	if p.logger == nil {
		t.Fatal("logger should default when nil")
	}
	if p.batchLimit != defaultDelayedPollerBatchLimit {
		t.Fatalf("batchLimit = %d, want %d", p.batchLimit, defaultDelayedPollerBatchLimit)
	}
	if p.maxBatchesPerTick != defaultDelayedPollerMaxBatchesPerTick {
		t.Fatalf("maxBatchesPerTick = %d, want %d", p.maxBatchesPerTick, defaultDelayedPollerMaxBatchesPerTick)
	}
}
