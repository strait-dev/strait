package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockIndexMaintenanceStore struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockIndexMaintenanceStore) ReindexIndexConcurrently(_ context.Context, indexName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, indexName)
	return nil
}

func (m *mockIndexMaintenanceStore) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
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
