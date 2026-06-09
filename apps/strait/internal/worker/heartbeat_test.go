package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
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

type mockHeartbeatSideTableStore struct {
	mu          sync.Mutex
	legacyCall  int
	sideCalls   [][]string
	singleCalls []string
}

func (m *mockHeartbeatSideTableStore) UpdateHeartbeat(context.Context, string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.legacyCall++
	return nil
}

func (m *mockHeartbeatSideTableStore) UpsertHeartbeatSideTable(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.singleCalls = append(m.singleCalls, id)
	return nil
}

func (m *mockHeartbeatSideTableStore) BatchUpsertHeartbeatSideTable(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := make([]string, len(ids))
	copy(clone, ids)
	m.sideCalls = append(m.sideCalls, clone)
	return nil
}

func (m *mockHeartbeatSideTableStore) snapshot() (int, [][]string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	side := make([][]string, len(m.sideCalls))
	for i := range m.sideCalls {
		side[i] = append([]string(nil), m.sideCalls[i]...)
	}
	return m.legacyCall, side, append([]string(nil), m.singleCalls...)
}

func TestNewHeartbeatSender_PrefersSideTableStore(t *testing.T) {
	t.Parallel()

	store := &mockHeartbeatSideTableStore{}
	h := NewHeartbeatSender(store, time.Hour)
	h.Register("run-1")
	h.Register("run-2")

	h.flush(context.Background())

	legacyCalls, sideCalls, singleCalls := store.snapshot()
	require.Equal(t, 0, legacyCalls)
	require.Empty(t, singleCalls)
	require.Len(t, sideCalls,

		1)

	got := map[string]struct{}{}
	for _, id := range sideCalls[0] {
		got[id] = struct{}{}
	}
	for _, id := range []string{"run-1", "run-2"} {
		if _, ok := got[id]; !ok {
			require.Failf(t, "test failure",

				"side-table batch ids = %v, missing %s", sideCalls[0], id)
		}
	}
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
		{
			name: "duplicate deregister keeps active count at zero",
			run: func(h *HeartbeatManager) {
				h.Register("run-1")
				h.Register("run-1")
				h.Deregister("run-1")
				h.Deregister("run-1")
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHeartbeatManager(&mockHeartbeatStore{}, 5*time.Millisecond)
			tt.run(h)
			require.Equal(t,
				tt.want,
				h.ActiveCount())
		})
	}
}

func TestHeartbeatManager_Run_Batching(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
			var concWG conc.WaitGroup
			defer concWG.Wait()
			store := &mockHeartbeatStore{}
			h := NewHeartbeatManager(store, 5*time.Millisecond)
			tt.setup(h)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			concWG.Go(func() {
				h.Run(ctx)
				close(done)
			})

			if tt.waitForCalls > 0 {
				waitForHeartbeatCalls(t, store, tt.waitForCalls, 300*time.Millisecond)
			} else {
				waitForNoHeartbeatCalls(t, store, 50*time.Millisecond)
			}

			cancel()
			select {
			case <-done:
			case <-time.After(300 * time.Millisecond):
				require.Fail(t, "Run() did not stop after cancel")
			}

			calls := store.calls()
			require.GreaterOrEqual(
				t, len(calls), tt.wantMinCalls,
			)

			if len(tt.wantFirstCallID) == 0 {
				require.Empty(t, calls)

				return
			}
			require.NotEmpty(t, calls)

			firstSet := make(map[string]struct{}, len(calls[0]))
			for _, id := range calls[0] {
				firstSet[id] = struct{}{}
			}
			require.Len(t, firstSet,

				len(tt.
					wantFirstCallID,
				))

			for id := range tt.wantFirstCallID {
				if _, ok := firstSet[id]; !ok {
					require.Failf(t, "test failure",

						"first batch ids = %v, missing %s", calls[0], id)
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

			var wg conc.WaitGroup
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
			require.Equal(t, 0, h.ActiveCount())
		})
	}
}

func TestHeartbeatSender_Run(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	beats := make(chan struct{}, 10)
	store := &mockExecutorStore{}
	store.batchUpdateHeartbeatFn = func(_ context.Context, ids []string) error {
		require.False(t,
			len(ids) != 1 ||
				ids[0] !=
					"run-1",
		)

		beats <- struct{}{}
		return nil
	}

	hb := NewHeartbeatSender(store, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		hb.Run(ctx, "run-1")
		close(done)
	})

	for i := range 2 {
		select {
		case <-beats:
		case <-time.After(300 * time.Millisecond):
			require.Failf(t, "test failure", "heartbeat %d not received in time", i+1)
		}
	}

	cancel()
	waitForSignal(t, done, "heartbeat sender did not stop after cancel")
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
			require.Failf(t, "test failure", "timed out waiting for %d heartbeat calls", want)
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
				require.Fail(t,

					"expected no heartbeat calls, but got some")
			}
		case <-deadline:
			return
		}
	}
}
