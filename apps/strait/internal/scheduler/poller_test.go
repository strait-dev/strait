package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelayedPoller_ActivatesDueRuns(t *testing.T) {
	t.Parallel()
	var totalActivated atomic.Int64
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, limit int) (int64, error) {
			assert.Equal(t, 1000,
				limit)

			totalActivated.Add(2)
			return 2, nil
		},
	}

	p := NewDelayedPoller(ms, slog.Default(), 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p.Run(ctx)
	require.GreaterOrEqual(t, totalActivated.
		Load(), int64(2))
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
	require.NotEqual(t,
		0, calls.
			Load())
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
	require.GreaterOrEqual(t, calls.
		Load(), int32(2),
	)
}

func TestDelayedPoller_UsesPromoterWhenConfigured(t *testing.T) {
	t.Parallel()
	var storeCalls atomic.Int32
	var promoterCalls atomic.Int32
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, _ int) (int64, error) {
			storeCalls.Add(1)
			return 0, nil
		},
	}
	promoter := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, limit int) (int64, error) {
			assert.Equal(t, 4,
				limit)

			promoterCalls.Add(1)
			return 0, nil
		},
	}

	NewDelayedPoller(ms, slog.Default(), time.Hour).
		WithPromoter(promoter).
		WithBatchLimit(4).
		poll(context.Background())
	require.EqualValues(t, 1,
		promoterCalls.
			Load())
	require.EqualValues(t, 0,
		storeCalls.
			Load())
}

func TestDelayedPoller_DrainsBoundedPagesPerTick(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var totalActivated atomic.Int64
	ms := &mockPollerStore{
		activateDueRunsFn: func(_ context.Context, limit int) (int64, error) {
			assert.Equal(t, 3,
				limit)

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
	require.EqualValues(t, 3,
		calls.Load())
	require.EqualValues(t, 6,
		totalActivated.
			Load(),
	)
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
	require.EqualValues(t, 3,
		calls.Load())
}

func TestDelayedPoller_ClampsUnsafeDefaults(t *testing.T) {
	t.Parallel()
	p := NewDelayedPoller(&mockPollerStore{}, nil, 0)
	require.Positive(t, p.
		interval)
	require.NotNil(t, p.
		logger)
	require.Equal(t, defaultDelayedPollerBatchLimit,

		p.batchLimit,
	)
	require.Equal(t, defaultDelayedPollerMaxBatchesPerTick,

		p.maxBatchesPerTick,
	)
}
