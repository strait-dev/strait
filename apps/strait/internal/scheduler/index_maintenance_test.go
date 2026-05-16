package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/store"
)

type mockIndexMaintenanceStore struct {
	mu    sync.Mutex
	calls []string
	errs  map[string]error
}

func (m *mockIndexMaintenanceStore) ReindexIndexConcurrently(_ context.Context, indexName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, indexName)
	if err, ok := m.errs[indexName]; ok {
		return err
	}
	return nil
}

func (m *mockIndexMaintenanceStore) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func TestIndexMaintainer_MissingIndexDoesNotAbortCycle(t *testing.T) {
	t.Parallel()

	s := &mockIndexMaintenanceStore{
		errs: map[string]error{
			"idx_runs_queue_covering": store.ErrIndexNotFound,
		},
	}
	maintainer := NewIndexMaintainer(s, time.Second)

	if err := maintainer.runLocked(context.Background()); err != nil {
		t.Fatalf("runLocked() error = %v", err)
	}
	if s.callCount() != len(defaultReindexTargets) {
		t.Fatalf("reindex calls = %d, want %d", s.callCount(), len(defaultReindexTargets))
	}
}

func TestIndexMaintainer_ReindexErrorStillContinues(t *testing.T) {
	t.Parallel()

	s := &mockIndexMaintenanceStore{
		errs: map[string]error{
			"idx_runs_queue_covering": errors.New("reindex failed"),
		},
	}
	maintainer := NewIndexMaintainer(s, time.Second)

	if err := maintainer.runLocked(context.Background()); err != nil {
		t.Fatalf("runLocked() error = %v", err)
	}
	if s.callCount() != len(defaultReindexTargets) {
		t.Fatalf("reindex calls = %d, want %d", s.callCount(), len(defaultReindexTargets))
	}
}

func TestIndexMaintainer_Run_RespectsInterval(t *testing.T) {
	t.Parallel()

	store := &mockIndexMaintenanceStore{}
	maintainer := NewIndexMaintainer(store, 20*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Millisecond)
	defer cancel()

	maintainer.Run(ctx)

	if store.callCount() < len(defaultReindexTargets) {
		t.Fatalf("reindex calls = %d, want at least %d", store.callCount(), len(defaultReindexTargets))
	}
}

func TestIndexMaintainer_DefaultInterval(t *testing.T) {
	t.Parallel()

	maintainer := NewIndexMaintainer(&mockIndexMaintenanceStore{}, 0)
	if maintainer.interval != defaultIndexMaintenanceInterval {
		t.Fatalf("interval = %v, want %v", maintainer.interval, defaultIndexMaintenanceInterval)
	}
}

func TestIndexMaintainer_AdvisoryLock_Acquired(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		tryCalls     int
		releaseCalls int
	)
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			tryCalls++
			return true, nil
		},
		releaseFn: func(_ context.Context, _ int64) error {
			mu.Lock()
			defer mu.Unlock()
			releaseCalls++
			return nil
		},
	}

	store := &mockIndexMaintenanceStore{}
	maintainer := NewIndexMaintainer(store, 20*time.Millisecond).WithAdvisoryLocker(locker)

	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Millisecond)
	defer cancel()
	maintainer.Run(ctx)

	if store.callCount() < len(defaultReindexTargets) {
		t.Fatalf("reindex calls = %d, want at least %d", store.callCount(), len(defaultReindexTargets))
	}
	mu.Lock()
	finalTry := tryCalls
	finalRelease := releaseCalls
	mu.Unlock()
	if finalTry == 0 {
		t.Fatal("advisory lock was never attempted")
	}
	if finalRelease != finalTry {
		t.Fatalf("releaseCalls = %d, want %d", finalRelease, finalTry)
	}
}

func TestIndexMaintainer_AdvisoryLock_NotAcquired(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		releaseCalls int
	)
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
		releaseFn: func(_ context.Context, _ int64) error {
			mu.Lock()
			defer mu.Unlock()
			releaseCalls++
			return nil
		},
	}

	store := &mockIndexMaintenanceStore{}
	maintainer := NewIndexMaintainer(store, 20*time.Millisecond).WithAdvisoryLocker(locker)

	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Millisecond)
	defer cancel()
	maintainer.Run(ctx)

	if store.callCount() != 0 {
		t.Fatalf("reindex calls = %d, want 0 (lock not acquired)", store.callCount())
	}
	mu.Lock()
	finalRelease := releaseCalls
	mu.Unlock()
	if finalRelease != 0 {
		t.Fatalf("releaseCalls = %d, want 0 (lock never held)", finalRelease)
	}
}
