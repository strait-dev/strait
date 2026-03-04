package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewPool_MinimumConcurrency(t *testing.T) {
	p0 := NewPool(0)
	if got := cap(p0.sem); got != 1 {
		t.Fatalf("NewPool(0) semaphore capacity = %d, want %d", got, 1)
	}

	pNeg := NewPool(-1)
	if got := cap(pNeg.sem); got != 1 {
		t.Fatalf("NewPool(-1) semaphore capacity = %d, want %d", got, 1)
	}
}

func TestPool_Submit_ExecutesWork(t *testing.T) {
	p := NewPool(1)
	done := make(chan struct{})

	p.Submit(context.Background(), func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("work not executed within timeout")
	}

	p.Shutdown()
}

func TestPool_ConcurrencyLimit(t *testing.T) {
	p := NewPool(2)
	block := make(chan struct{})
	started := make(chan struct{}, 3)

	task := func() {
		started <- struct{}{}
		<-block
	}

	p.Submit(context.Background(), task)
	p.Submit(context.Background(), task)

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("expected first two tasks to start")
		}
	}

	if got := p.ActiveCount(); got != 2 {
		t.Fatalf("ActiveCount() = %d, want %d", got, 2)
	}

	thirdSubmitReturned := make(chan struct{})
	go func() {
		p.Submit(context.Background(), task)
		close(thirdSubmitReturned)
	}()

	select {
	case <-thirdSubmitReturned:
		t.Fatal("third submit returned before a worker slot was free")
	case <-time.After(150 * time.Millisecond):
	}

	close(block)

	select {
	case <-thirdSubmitReturned:
	case <-time.After(time.Second):
		t.Fatal("third submit did not proceed after slot became free")
	}

	p.Shutdown()
}

func TestPool_Shutdown_WaitsForInFlight(t *testing.T) {
	p := NewPool(1)
	started := make(chan struct{})
	release := make(chan struct{})

	p.Submit(context.Background(), func() {
		close(started)
		<-release
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("task did not start")
	}

	shutdownReturned := make(chan struct{})
	go func() {
		p.Shutdown()
		close(shutdownReturned)
	}()

	select {
	case <-shutdownReturned:
		t.Fatal("Shutdown returned before in-flight task completed")
	case <-time.After(150 * time.Millisecond):
	}

	close(release)

	select {
	case <-shutdownReturned:
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not return after task completion")
	}
}

func TestPool_Submit_CanceledContext(t *testing.T) {
	p := NewPool(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var ran atomic.Bool
	p.Submit(ctx, func() {
		ran.Store(true)
	})

	if ran.Load() {
		t.Fatal("work executed despite canceled context")
	}

	p.Shutdown()
}

func TestPool_ActiveCount(t *testing.T) {
	p := NewPool(5)
	release := make(chan struct{})
	started := make(chan struct{}, 2)

	task := func() {
		started <- struct{}{}
		<-release
	}

	p.Submit(context.Background(), task)
	p.Submit(context.Background(), task)

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("tasks did not start in time")
		}
	}

	if got := p.ActiveCount(); got != 2 {
		t.Fatalf("ActiveCount() = %d, want %d", got, 2)
	}

	close(release)
	p.Shutdown()
}
