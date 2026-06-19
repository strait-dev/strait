package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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

func TestWebhookMessageCleanup_CleanupDeletesOlderThanThirtyDays(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	store := &rcMockWebhookCleanupStore{deleteCount: 3}
	c := NewWebhookMessageCleanup(
		store,
		slog.New(slog.NewTextHandler(&logs, nil)),
	)
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

	c.cleanup(context.Background(), now)

	require.Equal(t, now.Add(-30*24*time.Hour), store.cutoff)
	require.Contains(t, logs.String(), "cleaned up old webhook messages")
	require.Contains(t, logs.String(), "deleted=3")
}

func TestWebhookMessageCleanup_CleanupSkipsZeroDeleteLog(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	c := NewWebhookMessageCleanup(
		&rcMockWebhookCleanupStore{},
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	c.cleanup(context.Background(), time.Now())

	require.Empty(t, logs.String())
}

func TestWebhookMessageCleanup_CleanupLogsDeleteError(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	c := NewWebhookMessageCleanup(
		&rcMockWebhookCleanupStore{deleteErr: errors.New("delete failed")},
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	c.cleanup(context.Background(), time.Now())

	require.Contains(t, logs.String(), "failed to clean up old webhook messages")
	require.Contains(t, logs.String(), "delete failed")
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

type rcMockWebhookCleanupStore struct {
	deleteCount int64
	deleteErr   error
	cutoff      time.Time
}

func (m *rcMockWebhookCleanupStore) DeleteOldWebhookMessages(_ context.Context, cutoff time.Time) (int64, error) {
	m.cutoff = cutoff
	return m.deleteCount, m.deleteErr
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
