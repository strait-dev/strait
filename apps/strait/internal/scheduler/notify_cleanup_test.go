package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type mockNotifyCleanupStore struct {
	deleteExpiredFn func(ctx context.Context, limit int) (int64, error)
	deleteOldFn     func(ctx context.Context, before time.Time, limit int) (int64, error)
}

func (m *mockNotifyCleanupStore) DeleteExpiredNotifyProviderCallbackReceipts(ctx context.Context, limit int) (int64, error) {
	if m.deleteExpiredFn == nil {
		return 0, nil
	}
	return m.deleteExpiredFn(ctx, limit)
}

func (m *mockNotifyCleanupStore) DeleteOldNotifySuppressionEvents(ctx context.Context, before time.Time, limit int) (int64, error) {
	if m.deleteOldFn == nil {
		return 0, nil
	}
	return m.deleteOldFn(ctx, before, limit)
}

func TestNewNotifyCleanup_Defaults(t *testing.T) {
	t.Parallel()

	c := NewNotifyCleanup(&mockNotifyCleanupStore{}, 0, 0, 0)
	if c.interval != 30*time.Minute {
		t.Fatalf("interval = %v, want 30m", c.interval)
	}
	if c.suppressionRetention != 30*24*time.Hour {
		t.Fatalf("suppressionRetention = %v, want 720h", c.suppressionRetention)
	}
	if c.batchSize != 1000 {
		t.Fatalf("batchSize = %d, want 1000", c.batchSize)
	}
}

func TestNotifyCleanup_Run_StopsOnCancel(t *testing.T) {
	t.Parallel()

	c := NewNotifyCleanup(&mockNotifyCleanupStore{}, 10*time.Millisecond, 24*time.Hour, 100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestNotifyCleanup_Cleanup_BatchesUntilBelowLimit(t *testing.T) {
	t.Parallel()

	var receiptsCalls atomic.Int32
	var suppressionCalls atomic.Int32
	store := &mockNotifyCleanupStore{
		deleteExpiredFn: func(_ context.Context, limit int) (int64, error) {
			receiptsCalls.Add(1)
			if limit != 100 {
				t.Fatalf("limit = %d, want 100", limit)
			}
			if receiptsCalls.Load() == 1 {
				return 100, nil
			}
			return 30, nil
		},
		deleteOldFn: func(_ context.Context, _ time.Time, limit int) (int64, error) {
			suppressionCalls.Add(1)
			if limit != 100 {
				t.Fatalf("limit = %d, want 100", limit)
			}
			if suppressionCalls.Load() == 1 {
				return 100, nil
			}
			return 25, nil
		},
	}

	c := NewNotifyCleanup(store, time.Minute, 24*time.Hour, 100)
	c.cleanup(context.Background())

	if receiptsCalls.Load() != 2 {
		t.Fatalf("receipt delete calls = %d, want 2", receiptsCalls.Load())
	}
	if suppressionCalls.Load() != 2 {
		t.Fatalf("suppression delete calls = %d, want 2", suppressionCalls.Load())
	}
}

func TestNotifyCleanup_CleanupError_Continues(t *testing.T) {
	t.Parallel()

	var suppressionCalls atomic.Int32
	store := &mockNotifyCleanupStore{
		deleteExpiredFn: func(context.Context, int) (int64, error) {
			return 0, errors.New("boom")
		},
		deleteOldFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
			suppressionCalls.Add(1)
			return 0, nil
		},
	}

	c := NewNotifyCleanup(store, time.Minute, 24*time.Hour, 100)
	c.cleanup(context.Background())

	if suppressionCalls.Load() != 1 {
		t.Fatalf("suppression delete calls = %d, want 1", suppressionCalls.Load())
	}
}
