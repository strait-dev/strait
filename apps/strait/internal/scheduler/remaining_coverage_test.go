package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// Section separator.
// Remaining coverage tests for components with thin coverage.
// Section separator.

func TestWebhookMessageCleanup_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &rcMockWebhookCleanupStore{}
	c := NewWebhookMessageCleanup(s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		c.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

func TestWebhookMessageCleanup_DefaultInterval(t *testing.T) {
	t.Parallel()

	c := NewWebhookMessageCleanup(&rcMockWebhookCleanupStore{}, nil)
	require.Equal(t, 6*
		time.Hour,
		c.interval,
	)
}

func TestMemoryCleanup_Run_StopsOnCancel_RC(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &rcMockMemoryCleanupStore{}
	mc := NewMemoryCleanup(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		mc.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

func TestMemoryCleanup_CleanupError_Continues(t *testing.T) {
	t.Parallel()

	s := &rcMockMemoryCleanupStore{
		deleteExpiredFn: func(context.Context) (int64, error) {
			return 0, errors.New("cleanup failed")
		},
	}

	mc := NewMemoryCleanup(s, time.Minute)
	// Should not panic on error.
	mc.cleanup(context.Background())
}

func TestIndexMaintainer_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockIndexMaintenanceStore{}
	m := NewIndexMaintainer(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		m.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

func TestDebouncePoller_Run_StopsOnCancel_RC(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockDebounceStore{
		tryAdvisoryLockFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	p := NewDebouncePoller(s, &mockQueue{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

func TestBatchFlusher_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	s := &mockBatchStore{
		tryAdvisoryLockFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	f := NewBatchFlusher(s, &mockQueue{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		f.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

// Section separator.
// Local mock types (prefixed with rc to avoid conflicts).
// Section separator.

type rcMockWebhookCleanupStore struct{}

func (m *rcMockWebhookCleanupStore) DeleteOldWebhookMessages(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

type rcMockMemoryCleanupStore struct {
	deleteExpiredFn func(ctx context.Context) (int64, error)
}

func (m *rcMockMemoryCleanupStore) DeleteExpiredJobMemory(ctx context.Context) (int64, error) {
	if m.deleteExpiredFn != nil {
		return m.deleteExpiredFn(ctx)
	}
	return 0, nil
}

// Ensure imports are used.
var (
	_ = domain.Job{}
	_ = store.FlushableBatch{}
)
