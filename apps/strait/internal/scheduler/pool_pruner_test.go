package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/compute"
)

// mockPrunerRuntime implements compute.ContainerRuntime for pool pruner tests.
type mockPrunerRuntime struct {
	destroyFn func(ctx context.Context, machineID string) error
}

func (m *mockPrunerRuntime) Run(_ context.Context, _ compute.RunRequest) (*compute.RunResult, error) {
	return &compute.RunResult{ExitCode: 0}, nil
}
func (m *mockPrunerRuntime) Create(_ context.Context, _ compute.RunRequest) (string, error) {
	return "mock-id", nil
}
func (m *mockPrunerRuntime) Wait(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
	return &compute.RunResult{MachineID: machineID, ExitCode: 0}, nil
}
func (m *mockPrunerRuntime) Start(_ context.Context, _ string, _ map[string]string) error {
	return compute.ErrMachineGone
}
func (m *mockPrunerRuntime) Stop(_ context.Context, _ string) error { return nil }
func (m *mockPrunerRuntime) Destroy(ctx context.Context, machineID string) error {
	if m.destroyFn != nil {
		return m.destroyFn(ctx, machineID)
	}
	return nil
}
func (m *mockPrunerRuntime) Status(_ context.Context, _ string) (compute.MachineStatus, error) {
	return compute.MachineStatusStopped, nil
}
func (m *mockPrunerRuntime) GetLogs(_ context.Context, _ string, _ int) (string, error) {
	return "", nil
}

func TestPoolPruner_Run_PrunesExpired(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	destroyed := make(map[string]bool)

	rt := &mockPrunerRuntime{
		destroyFn: func(_ context.Context, machineID string) error {
			mu.Lock()
			destroyed[machineID] = true
			mu.Unlock()
			return nil
		},
	}

	pool := compute.NewMachinePool(5)
	// Add entries to the pool.
	pool.Release("test-project", "img:latest", "iad", "m-old-1")
	pool.Release("test-project", "img:latest", "iad", "m-old-2")

	// Use nanosecond TTL so entries are considered expired on the first tick.
	pruner := NewPoolPruner(pool, rt, 50*time.Millisecond, time.Nanosecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	pruner.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	if !destroyed["m-old-1"] {
		t.Error("expected m-old-1 to be destroyed")
	}
	if !destroyed["m-old-2"] {
		t.Error("expected m-old-2 to be destroyed")
	}
	if pool.Size() != 0 {
		t.Errorf("expected pool size 0 after prune, got %d", pool.Size())
	}
}

func TestPoolPruner_Run_ContextCancel(t *testing.T) {
	t.Parallel()

	rt := &mockPrunerRuntime{}
	pool := compute.NewMachinePool(5)

	pruner := NewPoolPruner(pool, rt, 10*time.Second, 10*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		pruner.Run(ctx)
		close(done)
	}()

	// Cancel immediately — Run should exit cleanly.
	cancel()

	select {
	case <-done:
		// Success: Run exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("pruner.Run did not exit after context cancel")
	}
}

func TestPoolPruner_Run_NilPool(t *testing.T) {
	t.Parallel()

	rt := &mockPrunerRuntime{}

	// Passing nil pool should return immediately without panic.
	pruner := NewPoolPruner(nil, rt, 50*time.Millisecond, time.Minute)

	done := make(chan struct{})
	go func() {
		pruner.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Success: returned immediately.
	case <-time.After(2 * time.Second):
		t.Fatal("pruner.Run with nil pool did not return immediately")
	}
}

func TestPoolPruner_Run_DestroyError(t *testing.T) {
	t.Parallel()

	var destroyCalls atomic.Int32

	rt := &mockPrunerRuntime{
		destroyFn: func(_ context.Context, _ string) error {
			destroyCalls.Add(1)
			return errors.New("destroy failed")
		},
	}

	pool := compute.NewMachinePool(5)
	pool.Release("test-project", "img:latest", "iad", "m-err-1")
	pool.Release("test-project", "img:latest", "iad", "m-err-2")

	// Use nanosecond TTL so entries are considered expired on the first tick.
	pruner := NewPoolPruner(pool, rt, 50*time.Millisecond, time.Nanosecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Should not panic even when Destroy returns errors.
	pruner.Run(ctx)

	if destroyCalls.Load() < 2 {
		t.Errorf("expected at least 2 destroy calls, got %d", destroyCalls.Load())
	}
}
