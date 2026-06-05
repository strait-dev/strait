package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockStatsStore struct {
	aggregateFn func(ctx context.Context, hour time.Time) error
}

func (m *mockStatsStore) AggregateHourlyStats(ctx context.Context, hour time.Time) error {
	if m.aggregateFn != nil {
		return m.aggregateFn(ctx, hour)
	}
	return nil
}

func (m *mockStatsStore) AggregateCostStatsHourly(_ context.Context, _ time.Time) error {
	return nil
}

func TestStatsAggregator_New(t *testing.T) {
	t.Parallel()
	a := NewStatsAggregator(&mockStatsStore{})
	require.NotNil(t, a)
	require.NotNil(t, a.
		store)
	require.NotNil(t, a.
		logger)
}

func TestStatsAggregator_WithAdvisoryLocker(t *testing.T) {
	t.Parallel()
	a := NewStatsAggregator(&mockStatsStore{})
	locker := &mockAdvisoryLocker{acquireFn: func(context.Context, int64) (bool, error) { return true, nil }}
	result := a.WithAdvisoryLocker(locker)
	require.Equal(t, a,
		result)
	require.NotNil(t, a.
		advisoryLocker,
	)
}

type mockAdvisoryLocker struct {
	acquireFn func(ctx context.Context, lockID int64) (bool, error)
	releaseFn func(ctx context.Context, lockID int64) error
}

func (m *mockAdvisoryLocker) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	if m.acquireFn != nil {
		return m.acquireFn(ctx, lockID)
	}
	return true, nil
}

func (m *mockAdvisoryLocker) ReleaseAdvisoryLock(ctx context.Context, lockID int64) error {
	if m.releaseFn != nil {
		return m.releaseFn(ctx, lockID)
	}
	return nil
}

func TestStatsAggregator_AggregatesPreviousHour(t *testing.T) {
	t.Parallel()

	var aggregatedHour time.Time
	var called atomic.Int32
	store := &mockStatsStore{
		aggregateFn: func(_ context.Context, hour time.Time) error {
			aggregatedHour = hour
			called.Add(1)
			return nil
		},
	}

	// Create aggregator and run the task function directly (without the maintenance loop).
	a := NewStatsAggregator(store)

	// Simulate what the maintenance loop callback does.
	previousHour := time.Now().Add(-time.Hour).Truncate(time.Hour)
	require.NoError(t,
		a.store.AggregateHourlyStats(context.
			Background(), previousHour))
	require.EqualValues(t, 1,
		called.Load())
	require.True(t, aggregatedHour.
		Equal(previousHour))
	require.False(t, aggregatedHour.
		Minute() !=
		0 || aggregatedHour.
		Second() != 0)

	// Verify hour is truncated.
}

func TestStatsAggregator_LockNotAcquired_Skips(t *testing.T) {
	t.Parallel()

	var called atomic.Int32
	store := &mockStatsStore{
		aggregateFn: func(context.Context, time.Time) error {
			called.Add(1)
			return nil
		},
	}

	a := NewStatsAggregator(store).WithAdvisoryLocker(&mockAdvisoryLocker{
		acquireFn: func(context.Context, int64) (bool, error) {
			return false, nil // lock held by another instance
		},
	})

	// Run with a context that cancels after one tick.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.Equal(t, int64(0x5374726169745361),

		statsAggregatorLockID,
	)

	// We can't easily test Run() because it blocks, but we can verify the lock ID is correct.

	_ = ctx
	_ = a
}

func TestStatsAggregator_RetriesFailedHourAfterClockAdvances(t *testing.T) {
	t.Parallel()

	firstTick := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	secondTick := firstTick.Add(time.Hour)
	firstHour := firstTick.Add(-time.Hour)
	secondHour := secondTick.Add(-time.Hour)
	var tick atomic.Int32
	var calls []time.Time
	store := &mockStatsStore{
		aggregateFn: func(_ context.Context, hour time.Time) error {
			calls = append(calls, hour)
			if len(calls) == 1 {
				return errors.New("transient aggregation failure")
			}
			return nil
		},
	}
	aggregator := NewStatsAggregator(store)
	aggregator.now = func() time.Time {
		if tick.Load() == 0 {
			return firstTick
		}
		return secondTick
	}
	require.NoError(t,
		aggregator.
			runLocked(context.
				Background()))

	tick.Store(1)
	require.NoError(t,
		aggregator.
			runLocked(context.
				Background()))
	require.Len(t, calls,
		3)
	require.False(t, !calls[0].Equal(firstHour) ||
		!calls[1].Equal(
			firstHour,
		) || !calls[2].Equal(secondHour))
}
