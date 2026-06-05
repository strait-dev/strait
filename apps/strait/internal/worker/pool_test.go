package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestNewPool_MinimumConcurrency(t *testing.T) {
	t.Parallel()
	p0 := NewPool(0)
	require.EqualValues(t, 1, p0.Available())

	_ = p0.Shutdown(context.Background())

	pNeg := NewPool(-1)
	require.EqualValues(t, 1, pNeg.Available())

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
		require.Fail(t, "work not executed within timeout")
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
			require.Fail(t, "expected first two tasks to start")
		}
	}
	require.EqualValues(t, 2, p.ActiveCount())

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
		require.Fail(t, "task did not start")
	}

	shutdownReturned := make(chan struct{})
	concWG.Go(func() {
		_ = p.Shutdown(context.Background())
		close(shutdownReturned)
	})

	select {
	case <-shutdownReturned:
		require.Fail(t, "Shutdown returned before in-flight task completed")
	case <-time.After(150 * time.Millisecond):
	}

	close(release)

	select {
	case <-shutdownReturned:
	case <-time.After(time.Second):
		require.Fail(t, "Shutdown did not return after task completion")
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
	require.False(t,
		ran.Load())

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
			require.Fail(t, "tasks did not start in time")
		}
	}
	require.EqualValues(t, 2, p.ActiveCount())

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
	require.Error(t,
		err)
	require.True(t,
		errors.Is(
			err, context.DeadlineExceeded,
		))

}

func TestPool_Shutdown_ReturnsNilOnSuccess(t *testing.T) {
	t.Parallel()
	p := NewPool(1)
	done := make(chan struct{})
	p.Submit(context.Background(), func() { close(done) })
	<-done

	err := p.Shutdown(context.Background())
	require.NoError(
		t, err)

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
	require.EqualValues(t, 2, p.RunningWorkers())
	require.EqualValues(t, 2, p.SubmittedTasks())

	close(release)
	_ = p.Shutdown(context.Background())
	require.EqualValues(t, 2, p.CompletedTasks())
	require.EqualValues(t, 2, p.SuccessfulTasks())

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
	require.EqualValues(t, 3, p.SubmittedTasks())

}
