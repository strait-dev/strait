package logdrain

import (
	"context"
	"testing"
	"time"
)

func TestWorker_StartStop(t *testing.T) {
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// Let a few ticks pass.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Worker stopped via context cancellation.
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop within 2s after context cancel")
	}
}

func TestWorker_StopMethod(t *testing.T) {
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, 50*time.Millisecond)

	ctx := context.Background()
	go w.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	// Stop should return promptly.
	stopped := make(chan struct{})
	go func() {
		w.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2s")
	}
}
