package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type mockHeartbeatStore struct {
	mu         sync.Mutex
	batchCalls [][]string
}

func (m *mockHeartbeatStore) UpdateHeartbeat(context.Context, string) error {
	return nil
}

func (m *mockHeartbeatStore) BatchUpdateHeartbeat(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := make([]string, len(ids))
	copy(clone, ids)
	m.batchCalls = append(m.batchCalls, clone)
	return nil
}

func (m *mockHeartbeatStore) calls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]string, len(m.batchCalls))
	for i := range m.batchCalls {
		out[i] = append([]string(nil), m.batchCalls[i]...)
	}
	return out
}

func TestHeartbeatManager_RegisterDeregister(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(h *HeartbeatManager)
		want int
	}{
		{
			name: "register increments active count",
			run: func(h *HeartbeatManager) {
				h.Register("run-1")
			},
			want: 1,
		},
		{
			name: "register duplicate keeps unique active count",
			run: func(h *HeartbeatManager) {
				h.Register("run-1")
				h.Register("run-1")
			},
			want: 1,
		},
		{
			name: "deregister removes run",
			run: func(h *HeartbeatManager) {
				h.Register("run-1")
				h.Deregister("run-1")
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHeartbeatManager(&mockHeartbeatStore{}, 5*time.Millisecond)
			tt.run(h)
			if got := h.ActiveCount(); got != tt.want {
				t.Fatalf("ActiveCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHeartbeatManager_Run_Batching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setup           func(h *HeartbeatManager)
		waitForCalls    int
		wantMinCalls    int
		wantFirstCallID map[string]struct{}
	}{
		{
			name: "issues batch update for active runs",
			setup: func(h *HeartbeatManager) {
				h.Register("run-1")
				h.Register("run-2")
			},
			waitForCalls: 1,
			wantMinCalls: 1,
			wantFirstCallID: map[string]struct{}{
				"run-1": {},
				"run-2": {},
			},
		},
		{
			name:         "skips empty batch",
			setup:        func(*HeartbeatManager) {},
			waitForCalls: 0,
			wantMinCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockHeartbeatStore{}
			h := NewHeartbeatManager(store, 5*time.Millisecond)
			tt.setup(h)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				h.Run(ctx)
				close(done)
			}()

			if tt.waitForCalls > 0 {
				waitForHeartbeatCalls(t, store, tt.waitForCalls, 300*time.Millisecond)
			} else {
				waitForNoHeartbeatCalls(t, store, 50*time.Millisecond)
			}

			cancel()
			select {
			case <-done:
			case <-time.After(300 * time.Millisecond):
				t.Fatal("Run() did not stop after cancel")
			}

			calls := store.calls()
			if len(calls) < tt.wantMinCalls {
				t.Fatalf("batch call count = %d, want at least %d", len(calls), tt.wantMinCalls)
			}

			if len(tt.wantFirstCallID) == 0 {
				if len(calls) != 0 {
					t.Fatalf("batch call count = %d, want 0", len(calls))
				}
				return
			}

			if len(calls) == 0 {
				t.Fatal("no batch calls recorded")
			}

			firstSet := make(map[string]struct{}, len(calls[0]))
			for _, id := range calls[0] {
				firstSet[id] = struct{}{}
			}
			if len(firstSet) != len(tt.wantFirstCallID) {
				t.Fatalf("first batch unique id count = %d, want %d", len(firstSet), len(tt.wantFirstCallID))
			}
			for id := range tt.wantFirstCallID {
				if _, ok := firstSet[id]; !ok {
					t.Fatalf("first batch ids = %v, missing %s", calls[0], id)
				}
			}
		})
	}
}

func TestHeartbeatManager_ConcurrentRegisterDeregister(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		workers    int
		iterations int
	}{
		{name: "low concurrency", workers: 8, iterations: 200},
		{name: "high concurrency", workers: 32, iterations: 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHeartbeatManager(&mockHeartbeatStore{}, time.Hour)

			var wg sync.WaitGroup
			for i := range tt.workers {
				id := fmt.Sprintf("run-%d", i)
				wg.Go(func() {
					for range tt.iterations {
						h.Register(id)
						h.Deregister(id)
						h.Register(id)
					}
					h.Deregister(id)
				})
			}

			wg.Wait()

			if got := h.ActiveCount(); got != 0 {
				t.Fatalf("ActiveCount() = %d, want 0", got)
			}
		})
	}
}

func waitForHeartbeatCalls(t *testing.T, store *mockHeartbeatStore, want int, timeout time.Duration) {
	t.Helper()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(timeout)
	for {
		select {
		case <-ticker.C:
			if len(store.calls()) >= want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %d heartbeat calls", want)
		}
	}
}

func waitForNoHeartbeatCalls(t *testing.T, store *mockHeartbeatStore, wait time.Duration) {
	t.Helper()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(wait)
	for {
		select {
		case <-ticker.C:
			if len(store.calls()) > 0 {
				t.Fatal("expected no heartbeat calls, but got some")
			}
		case <-deadline:
			return
		}
	}
}
