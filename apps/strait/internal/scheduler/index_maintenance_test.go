package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		maintainer.
			runLocked(context.
				Background()))
	require.Equal(t, len(defaultReindexTargets), s.callCount())

}

func TestIndexMaintainer_ReindexErrorStillContinues(t *testing.T) {
	t.Parallel()

	s := &mockIndexMaintenanceStore{
		errs: map[string]error{
			"idx_runs_queue_covering": errors.New("reindex failed"),
		},
	}
	maintainer := NewIndexMaintainer(s, time.Second)
	require.NoError(t,
		maintainer.
			runLocked(context.
				Background()))
	require.Equal(t, len(defaultReindexTargets), s.callCount())

}

func TestIndexMaintainer_Run_RespectsInterval(t *testing.T) {
	t.Parallel()

	store := &mockIndexMaintenanceStore{}
	maintainer := NewIndexMaintainer(store, 20*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Millisecond)
	defer cancel()

	maintainer.Run(ctx)
	require.GreaterOrEqual(t, store.
		callCount(), len(defaultReindexTargets))

}

func TestIndexMaintainer_DefaultInterval(t *testing.T) {
	t.Parallel()

	maintainer := NewIndexMaintainer(&mockIndexMaintenanceStore{}, 0)
	require.Equal(t, defaultIndexMaintenanceInterval,

		maintainer.
			interval)

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
	require.GreaterOrEqual(t, store.
		callCount(), len(defaultReindexTargets))

	mu.Lock()
	finalTry := tryCalls
	finalRelease := releaseCalls
	mu.Unlock()
	require.NotEqual(t,
		0, finalTry,
	)
	require.Equal(t, finalTry,
		finalRelease,
	)

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
	require.EqualValues(t, 0,
		store.callCount(),
	)

	mu.Lock()
	finalRelease := releaseCalls
	mu.Unlock()
	require.EqualValues(t, 0,
		finalRelease,
	)

}
