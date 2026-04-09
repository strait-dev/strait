package build

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// mockGCStore is a controllable GCStore for unit testing DeploymentGC.
type mockGCStore struct {
	deleteExpiredFn func(ctx context.Context, pendingBefore, failedBefore time.Time) (int64, error)
	callCount       atomic.Int64
}

func (m *mockGCStore) DeleteExpiredDeployments(ctx context.Context, pendingBefore, failedBefore time.Time) (int64, error) {
	m.callCount.Add(1)
	if m.deleteExpiredFn != nil {
		return m.deleteExpiredFn(ctx, pendingBefore, failedBefore)
	}
	return 0, nil
}

// TestDeploymentGC_Collect_CallsDeleteExpiredDeployments verifies that a single
// GC sweep calls DeleteExpiredDeployments exactly once with correctly computed
// time windows derived from pendingTTL and failedAge options.
func TestDeploymentGC_Collect_CallsDeleteExpiredDeployments(t *testing.T) {
	t.Parallel()

	const pendingTTL = 15 * time.Minute
	const failedAge = 7 * 24 * time.Hour

	var capturedPendingBefore, capturedFailedBefore time.Time
	store := &mockGCStore{
		deleteExpiredFn: func(_ context.Context, pb, fb time.Time) (int64, error) {
			capturedPendingBefore = pb
			capturedFailedBefore = fb
			return 3, nil
		},
	}

	gc := NewDeploymentGC(store,
		WithGCPendingTTL(pendingTTL),
		WithGCFailedAge(failedAge),
	)

	before := time.Now()
	gc.collect(context.Background())
	after := time.Now()

	if store.callCount.Load() != 1 {
		t.Errorf("DeleteExpiredDeployments called %d times, want 1", store.callCount.Load())
	}

	// pendingBefore should be roughly (now - pendingTTL).
	expectedPendingBefore := before.Add(-pendingTTL)
	if capturedPendingBefore.Before(expectedPendingBefore.Add(-time.Second)) ||
		capturedPendingBefore.After(after.Add(-pendingTTL).Add(time.Second)) {
		t.Errorf("pendingBefore = %v, expected roughly %v (±1s)", capturedPendingBefore, expectedPendingBefore)
	}

	// failedBefore should be roughly (now - failedAge).
	expectedFailedBefore := before.Add(-failedAge)
	if capturedFailedBefore.Before(expectedFailedBefore.Add(-time.Second)) ||
		capturedFailedBefore.After(after.Add(-failedAge).Add(time.Second)) {
		t.Errorf("failedBefore = %v, expected roughly %v (±1s)", capturedFailedBefore, expectedFailedBefore)
	}
}

// TestDeploymentGC_Collect_StoreError_NosPanic verifies that a store error
// during collection is handled gracefully — the GC logs the error and returns
// without panicking, so the ticker can fire again on the next interval.
func TestDeploymentGC_Collect_StoreError_NoPanic(t *testing.T) {
	t.Parallel()

	store := &mockGCStore{
		deleteExpiredFn: func(_ context.Context, _, _ time.Time) (int64, error) {
			return 0, errors.New("database connection lost")
		},
	}

	gc := NewDeploymentGC(store)

	// Must not panic.
	gc.collect(context.Background())

	if store.callCount.Load() != 1 {
		t.Errorf("DeleteExpiredDeployments called %d times, want 1", store.callCount.Load())
	}
}

// TestDeploymentGC_Collect_EmptyStore verifies that when DeleteExpiredDeployments
// returns 0 rows (nothing to clean) the GC does not error or log excessively.
// This is the common steady-state case.
func TestDeploymentGC_Collect_EmptyStore(t *testing.T) {
	t.Parallel()

	store := &mockGCStore{
		deleteExpiredFn: func(_ context.Context, _, _ time.Time) (int64, error) {
			return 0, nil // nothing to delete
		},
	}

	gc := NewDeploymentGC(store)
	gc.collect(context.Background())

	if store.callCount.Load() != 1 {
		t.Errorf("DeleteExpiredDeployments called %d times, want 1", store.callCount.Load())
	}
}

// TestDeploymentGC_Collect_DeletedCountLogged verifies that when rows are
// deleted the count is passed through from the store without error. This is a
// structural check — we can't easily assert on log output without a log sink,
// but we verify the store is called and the return value is accepted cleanly.
func TestDeploymentGC_Collect_DeletedCountReturned(t *testing.T) {
	t.Parallel()

	var returned int64
	store := &mockGCStore{
		deleteExpiredFn: func(_ context.Context, _, _ time.Time) (int64, error) {
			return 42, nil
		},
	}
	// Wrap collect to capture what it returns (collect doesn't return, but
	// the store call count and no-panic guarantee is sufficient).
	gc := NewDeploymentGC(store)
	gc.collect(context.Background())
	_ = returned

	if store.callCount.Load() != 1 {
		t.Errorf("DeleteExpiredDeployments called %d times, want 1", store.callCount.Load())
	}
}

// TestDeploymentGC_Collect_ContextCancellation verifies that a cancelled context
// is passed through to the store and the GC does not block or panic.
func TestDeploymentGC_Collect_ContextCancellation(t *testing.T) {
	t.Parallel()

	var ctxErrAtCall error
	store := &mockGCStore{
		deleteExpiredFn: func(ctx context.Context, _, _ time.Time) (int64, error) {
			ctxErrAtCall = ctx.Err()
			return 0, ctx.Err()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling collect

	gc := NewDeploymentGC(store)
	gc.collect(ctx) // must not panic even with cancelled context

	if store.callCount.Load() == 0 {
		t.Error("DeleteExpiredDeployments was not called")
	}
	if ctxErrAtCall != context.Canceled {
		t.Errorf("store received context.Err() = %v, want context.Canceled", ctxErrAtCall)
	}
}

// TestNewDeploymentGC_DefaultOptions verifies that default GC options are sane:
// interval > 0, pendingTTL > 0, failedAge > 0.
func TestNewDeploymentGC_DefaultOptions(t *testing.T) {
	t.Parallel()

	store := &mockGCStore{}
	gc := NewDeploymentGC(store)

	if gc.interval <= 0 {
		t.Errorf("default interval = %v, want > 0", gc.interval)
	}
	if gc.pendingTTL <= 0 {
		t.Errorf("default pendingTTL = %v, want > 0", gc.pendingTTL)
	}
	if gc.failedAge <= 0 {
		t.Errorf("default failedAge = %v, want > 0", gc.failedAge)
	}
}
