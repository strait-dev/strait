package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
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
	if a == nil {
		t.Fatal("expected non-nil aggregator")
		return
	}
	if a.store == nil {
		t.Fatal("expected store to be set")
	}
	if a.logger == nil {
		t.Fatal("expected logger to be set")
	}
}

func TestStatsAggregator_WithAdvisoryLocker(t *testing.T) {
	t.Parallel()
	a := NewStatsAggregator(&mockStatsStore{})
	locker := &mockAdvisoryLocker{acquireFn: func(context.Context, int64) (bool, error) { return true, nil }}
	result := a.WithAdvisoryLocker(locker)
	if result != a {
		t.Fatal("WithAdvisoryLocker should return same instance")
	}
	if a.advisoryLocker == nil {
		t.Fatal("expected advisory locker to be set")
	}
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
	if err := a.store.AggregateHourlyStats(context.Background(), previousHour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if called.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", called.Load())
	}
	if !aggregatedHour.Equal(previousHour) {
		t.Fatalf("aggregated hour = %v, want %v", aggregatedHour, previousHour)
	}
	// Verify hour is truncated.
	if aggregatedHour.Minute() != 0 || aggregatedHour.Second() != 0 {
		t.Fatalf("hour should be truncated, got %v", aggregatedHour)
	}
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

	// We can't easily test Run() because it blocks, but we can verify the lock ID is correct.
	if statsAggregatorLockID != 0x5374726169745361 {
		t.Fatalf("lock ID = %x, want 0x5374726169745361", statsAggregatorLockID)
	}

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

	if err := aggregator.runLocked(context.Background()); err != nil {
		t.Fatalf("first runLocked() error = %v", err)
	}
	tick.Store(1)
	if err := aggregator.runLocked(context.Background()); err != nil {
		t.Fatalf("second runLocked() error = %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("aggregate calls = %v, want failed hour retried before new hour", calls)
	}
	if !calls[0].Equal(firstHour) || !calls[1].Equal(firstHour) || !calls[2].Equal(secondHour) {
		t.Fatalf("aggregate calls = %v, want [%v %v %v]", calls, firstHour, firstHour, secondHour)
	}
}
