package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/compute"
	"strait/internal/domain"
	"strait/internal/store"
)

// Section separator.
// Remaining coverage tests for components with thin coverage.
// Section separator.

func TestWebhookMessageCleanup_Run_StopsOnCancel(t *testing.T) {
	t.Parallel()

	s := &rcMockWebhookCleanupStore{}
	c := NewWebhookMessageCleanup(s, nil)

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

func TestWebhookMessageCleanup_DefaultInterval(t *testing.T) {
	t.Parallel()

	c := NewWebhookMessageCleanup(&rcMockWebhookCleanupStore{}, nil)
	if c.interval != 6*time.Hour {
		t.Fatalf("expected default interval 6h, got %v", c.interval)
	}
}

func TestCostEstimateRefresher_Run_StopsOnCancel(t *testing.T) {
	t.Parallel()

	s := &rcMockCostEstimateStore{}
	r := NewCostEstimateRefresher(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestCostEstimateRefresher_RefreshError_Continues(t *testing.T) {
	t.Parallel()

	var upsertCalls atomic.Int32
	s := &rcMockCostEstimateStore{
		listActiveJobIDsFn: func(context.Context) ([]string, error) {
			return []string{"job-fail", "job-ok"}, nil
		},
		upsertJobCostEstimateFn: func(_ context.Context, jobID string) error {
			upsertCalls.Add(1)
			if jobID == "job-fail" {
				return errors.New("estimate failed")
			}
			return nil
		},
	}

	r := NewCostEstimateRefresher(s, time.Minute)
	r.refresh(context.Background())

	// Both jobs should have been attempted.
	if upsertCalls.Load() != 2 {
		t.Fatalf("expected 2 upsert calls, got %d", upsertCalls.Load())
	}
}

func TestMemoryCleanup_Run_StopsOnCancel_RC(t *testing.T) {
	t.Parallel()

	s := &rcMockMemoryCleanupStore{}
	mc := NewMemoryCleanup(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		mc.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
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
	t.Parallel()

	s := &mockIndexMaintenanceStore{}
	m := NewIndexMaintainer(s, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestPoolPruner_Run_StopsOnCancel(t *testing.T) {
	t.Parallel()

	pool := compute.NewMachinePool(5)
	p := NewPoolPruner(pool, &mockPrunerRuntime{}, 10*time.Millisecond, 10*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestDebouncePoller_Run_StopsOnCancel_RC(t *testing.T) {
	t.Parallel()

	s := &mockDebounceStore{
		tryAdvisoryLockFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	p := NewDebouncePoller(s, &mockQueue{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

func TestBatchFlusher_Run_StopsOnCancel(t *testing.T) {
	t.Parallel()

	s := &mockBatchStore{
		tryAdvisoryLockFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	f := NewBatchFlusher(s, &mockQueue{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		f.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}

// Section separator.
// Local mock types (prefixed with rc to avoid conflicts).
// Section separator.

type rcMockWebhookCleanupStore struct{}

func (m *rcMockWebhookCleanupStore) DeleteOldWebhookMessages(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

type rcMockCostEstimateStore struct {
	listActiveJobIDsFn      func(ctx context.Context) ([]string, error)
	upsertJobCostEstimateFn func(ctx context.Context, jobID string) error
}

func (m *rcMockCostEstimateStore) ListActiveJobIDs(ctx context.Context) ([]string, error) {
	if m.listActiveJobIDsFn != nil {
		return m.listActiveJobIDsFn(ctx)
	}
	return nil, nil
}

func (m *rcMockCostEstimateStore) UpsertJobCostEstimate(ctx context.Context, jobID string) error {
	if m.upsertJobCostEstimateFn != nil {
		return m.upsertJobCostEstimateFn(ctx, jobID)
	}
	return nil
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
