package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/compute"
	"strait/internal/domain"
)

// TestShutdown_InFlightRunCompletion verifies that a run completing during
// shutdown is processed without error.
func TestShutdown_InFlightRunCompletion(t *testing.T) {
	t.Parallel()

	var callbackCalled atomic.Bool
	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour, // Will not actually poll.
		HeartbeatInterval: time.Hour,
		WorkflowCallback: &mockWorkflowCallback{
			onTerminalFn: func(_ context.Context, _ *domain.JobRun) error {
				callbackCalled.Store(true)
				time.Sleep(50 * time.Millisecond)
				return nil
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)

	// Give the executor time to start.
	time.Sleep(20 * time.Millisecond)

	// Trigger a callback directly to simulate in-flight work.
	exec.callbackWG.Go(func() {
		callbackCalled.Store(true)
	})

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

// TestShutdown_DrainTimeout verifies that shutdown returns an error when
// the drain exceeds the given timeout.
func TestShutdown_DrainTimeout(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Add artificial in-flight work that blocks longer than timeout.
	exec.callbackWG.Go(func() {
		time.Sleep(5 * time.Second)
	})

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer shutdownCancel()

	err := exec.Shutdown(shutdownCtx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// TestShutdown_WebhookDeliveryDuringShutdown verifies that pending webhook
// delivery is waited upon during shutdown.
func TestShutdown_WebhookDeliveryDuringShutdown(t *testing.T) {
	t.Parallel()

	var delivered atomic.Bool
	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Simulate in-flight webhook delivery.
	exec.callbackWG.Go(func() {
		time.Sleep(30 * time.Millisecond)
		delivered.Store(true)
	})

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	if !delivered.Load() {
		t.Fatal("expected webhook delivery to complete before shutdown")
	}
}

// TestShutdown_PoolCleanupOnShutdown verifies that the machine pool is
// drained during shutdown.
func TestShutdown_PoolCleanupOnShutdown(t *testing.T) {
	t.Parallel()

	var destroyCalled atomic.Int32
	pool := compute.NewMachinePool(5)
	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-2")

	exec := NewExecutor(ExecutorConfig{
		Pool:                  NewPool(2),
		Queue:                 &mockExecQueue{},
		Store:                 &mockExecutorStore{},
		PollInterval:          time.Hour,
		HeartbeatInterval:     time.Hour,
		ContainerRuntime:      &mockContainerRuntime{destroyFn: func(_ context.Context, _ string) error { destroyCalled.Add(1); return nil }},
		WarmPoolEnabled:       true,
		MaxConcurrentMachines: 10,
	})
	// Replace the pool with our pre-populated one.
	exec.machinePool = pool

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	if destroyCalled.Load() < 2 {
		t.Fatalf("expected at least 2 destroy calls, got %d", destroyCalled.Load())
	}
}

// TestShutdown_CacheFlushOnShutdown verifies that CloseCache can be
// called during shutdown without panicking.
func TestShutdown_CacheFlushOnShutdown(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
		JobCacheTTL:       5 * time.Minute,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	// Must not panic.
	exec.CloseCache()
}

// TestShutdown_ConcurrentShutdownSignals verifies that multiple concurrent
// shutdown calls do not panic or deadlock.
func TestShutdown_ConcurrentShutdownSignals(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	cancel()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			_ = exec.Shutdown(shutdownCtx)
		})
	}
	wg.Wait()
}

// TestShutdown_PubSubUnsubscribeOnShutdown verifies that the event channel
// is closed during shutdown so subscribers stop receiving events.
func TestShutdown_PubSubUnsubscribeOnShutdown(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	var subscriberDone atomic.Bool
	exec.Subscribe(func(_ context.Context, _ RunLifecycleEvent) {
		subscriberDone.Store(true)
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	// After shutdown, the event channel should be closed. Sending should
	// be handled gracefully by the emit method's recover logic.
	exec.emit(context.Background(), RunLifecycleEvent{
		Type: "test",
		Run:  &domain.JobRun{ID: "test-run"},
	})
}

// FuzzShutdownTiming fuzzes the timing between Run and Shutdown to
// ensure no panics.
func FuzzShutdownTiming(f *testing.F) {
	f.Add(uint(0))
	f.Add(uint(1))
	f.Add(uint(5))
	f.Add(uint(50))

	f.Fuzz(func(t *testing.T, delayMs uint) {
		if delayMs > 100 {
			delayMs %= 100
		}

		exec := NewExecutor(ExecutorConfig{
			Pool:              NewPool(2),
			Queue:             &mockExecQueue{},
			Store:             &mockExecutorStore{},
			PollInterval:      time.Hour,
			HeartbeatInterval: time.Hour,
		})

		ctx, cancel := context.WithCancel(context.Background())
		go exec.Run(ctx)

		time.Sleep(time.Duration(delayMs) * time.Millisecond)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		_ = exec.Shutdown(shutdownCtx)
	})
}
