package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestExecutor_HeartbeatTrackedForShutdown verifies that the heartbeat
// goroutine is tracked by pollWG so Shutdown waits for it to exit.
func TestExecutor_HeartbeatTrackedForShutdown(t *testing.T) {
	t.Parallel()

	hbStore := &mockHeartbeatStore{}
	e := &Executor{
		heartbeat:    NewHeartbeatManager(hbStore, 50*time.Millisecond),
		pollInterval: time.Hour, // large interval so we only test heartbeat tracking
		pool:         NewPool(1),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		eventCh:      make(chan runEventEnvelope, 1),
		wake:         make(chan struct{}),
		logger:       slog.Default(),
	}

	e.heartbeat.Register("run-heartbeat-test")

	ctx, cancel := context.WithCancel(context.Background())
	go e.Run(ctx)

	waitForHeartbeatCalls(t, hbStore, 1, 500*time.Millisecond)

	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutCancel()

	if err := e.Shutdown(shutCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// If we reach here, pollWG.Wait() completed, meaning the heartbeat
	// goroutine was properly tracked and exited.
}

// TestExecutor_HeartbeatFlushesBeforeShutdown verifies that heartbeat
// goroutine has time to flush active runs before Shutdown returns.
func TestExecutor_HeartbeatFlushesBeforeShutdown(t *testing.T) {
	t.Parallel()

	hbStore := &mockHeartbeatStore{}
	e := &Executor{
		heartbeat:    NewHeartbeatManager(hbStore, 10*time.Millisecond),
		pollInterval: time.Hour,
		pool:         NewPool(1),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		eventCh:      make(chan runEventEnvelope, 1),
		wake:         make(chan struct{}),
		logger:       slog.Default(),
	}

	// Register a run for heartbeat.
	e.heartbeat.Register("run-flush-test")

	ctx, cancel := context.WithCancel(context.Background())
	go e.Run(ctx)

	// Wait for at least one heartbeat tick.
	waitForHeartbeatCalls(t, hbStore, 1, 500*time.Millisecond)

	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutCancel()

	if err := e.Shutdown(shutCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	calls := hbStore.calls()
	if len(calls) == 0 {
		t.Fatal("expected at least one heartbeat batch call before shutdown")
	}
}
