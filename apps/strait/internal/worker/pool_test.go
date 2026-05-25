package worker

import (
	"context"
	"errors"
	"github.com/sourcegraph/conc"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewPool_MinimumConcurrency(t *testing.T) {
	t.Parallel()
	p0 := NewPool(0)
	if got := p0.Available(); got != 1 {
		t.Fatalf("NewPool(0) available = %d, want %d", got, 1)
	}
	_ = p0.Shutdown(context.Background())

	pNeg := NewPool(-1)
	if got := pNeg.Available(); got != 1 {
		t.Fatalf("NewPool(-1) available = %d, want %d", got, 1)
	}
	_ = pNeg.Shutdown(context.Background())
}

func TestPool_Submit_ExecutesWork(t *testing.T) {
	t.Parallel()
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

	_ = p.Shutdown(context.Background())
}

func TestPool_ConcurrencyLimit(t *testing.T) {
	t.Parallel()
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

	close(block)
	_ = p.Shutdown(context.Background())
}

func TestPool_Shutdown_WaitsForInFlight(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
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
	concWG.Go(func() {
		_ = p.Shutdown(context.Background())
		close(shutdownReturned)
	})

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
	t.Parallel()
	p := NewPool(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var ran atomic.Bool
	p.Submit(ctx, func() {
		ran.Store(true)
	})

	time.Sleep(50 * time.Millisecond)
	if ran.Load() {
		t.Fatal("work executed despite canceled context")
	}

	_ = p.Shutdown(context.Background())
}

func TestPool_ActiveCount(t *testing.T) {
	t.Parallel()
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
	_ = p.Shutdown(context.Background())
}

func TestPool_Shutdown_RespectsContext(t *testing.T) {
	t.Parallel()
	p := NewPool(1)
	started := make(chan struct{})

	p.Submit(context.Background(), func() {
		close(started)
		time.Sleep(5 * time.Second)
	})

	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := p.Shutdown(ctx)
	if err == nil {
		t.Fatal("expected timeout error from Shutdown, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestPool_Shutdown_ReturnsNilOnSuccess(t *testing.T) {
	t.Parallel()
	p := NewPool(1)
	done := make(chan struct{})
	p.Submit(context.Background(), func() { close(done) })
	<-done

	err := p.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("expected nil error from Shutdown, got %v", err)
	}
}

func TestPool_Metrics(t *testing.T) {
	t.Parallel()
	p := NewPool(2)
	release := make(chan struct{})
	started := make(chan struct{}, 2)

	p.Submit(context.Background(), func() {
		started <- struct{}{}
		<-release
	})
	p.Submit(context.Background(), func() {
		started <- struct{}{}
		<-release
	})

	<-started
	<-started

	if got := p.RunningWorkers(); got != 2 {
		t.Fatalf("RunningWorkers() = %d, want 2", got)
	}
	if got := p.SubmittedTasks(); got != 2 {
		t.Fatalf("SubmittedTasks() = %d, want 2", got)
	}

	close(release)
	_ = p.Shutdown(context.Background())

	if got := p.CompletedTasks(); got != 2 {
		t.Fatalf("CompletedTasks() = %d, want 2", got)
	}
	if got := p.SuccessfulTasks(); got != 2 {
		t.Fatalf("SuccessfulTasks() = %d, want 2", got)
	}
}

func TestPool_WithQueueSize(t *testing.T) {
	t.Parallel()
	p := NewPool(1, WithQueueSize(2))
	block := make(chan struct{})
	started := make(chan struct{})

	p.Submit(context.Background(), func() {
		close(started)
		<-block
	})
	<-started

	p.Submit(context.Background(), func() { <-block })
	p.Submit(context.Background(), func() { <-block })

	close(block)
	_ = p.Shutdown(context.Background())

	if got := p.SubmittedTasks(); got != 3 {
		t.Fatalf("SubmittedTasks() = %d, want 3", got)
	}
}
