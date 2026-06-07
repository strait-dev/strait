package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NoError(
		t, e.Shutdown(shutCtx))

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
	require.NoError(
		t, e.Shutdown(shutCtx))

	calls := hbStore.calls()
	require.NotEmpty(t, calls)
}

func TestExecutor_Poll_NoAvailableSlots(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	started := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(context.Background(), func() {
		close(started)
		<-release
	})
	waitForSignal(t, started, "blocking task did not start")

	var called atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(context.Context, int) ([]domain.JobRun, error) {
			called.Add(1)
			return nil, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	exec.poll(context.Background())
	require.EqualValues(t, 0, called.
		Load(),
	)

	close(release)
}

func TestExecutor_Poll_EmptyQueue(t *testing.T) {
	t.Parallel()
	var dequeueCalls atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			dequeueCalls.Add(1)
			require.Positive(t,
				n)

			return []domain.JobRun{}, nil
		},
	}

	exec := newTestExecutor(t, &mockExecutorStore{}, q, time.Hour, nil)
	exec.poll(context.Background())
	require.EqualValues(t, 1, dequeueCalls.
		Load())
	require.Equal(t, 0, exec.pool.
		ActiveCount())
}

func TestExecutor_Poll_DequeueError(t *testing.T) {
	t.Parallel()
	var dequeueCalls atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(context.Context, int) ([]domain.JobRun, error) {
			dequeueCalls.Add(1)
			return nil, errors.New("queue down")
		},
	}

	exec := newTestExecutor(t, &mockExecutorStore{}, q, time.Hour, nil)
	exec.poll(context.Background())
	require.EqualValues(t, 1, dequeueCalls.
		Load())
}

func TestExecutor_Poll_UsesProjectPartitionDequeue(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}

	called := false
	q := &mockExecQueue{
		dequeueNByProjectFn: func(_ context.Context, n int, projectID string) ([]domain.JobRun, error) {
			called = true
			require.Positive(t,
				n)
			require.Equal(t,
				"proj-a",
				projectID,
			)

			return nil, nil
		},
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			require.Fail(t,

				"did not expect global DequeueN when partitions are configured")
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Second,
		Partitions:        []string{"proj-a"},
	})

	exec.poll(context.Background())
	require.True(t,
		called)
}

func BenchmarkExecutorPoll(b *testing.B) {
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob("http://example.invalid", 1, 1), nil
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			runs := make([]domain.JobRun, 0, n)
			for i := range n {
				runs = append(runs, domain.JobRun{
					ID:        fmt.Sprintf("run-%d", i),
					JobID:     "job-1",
					ProjectID: "proj-1",
					Status:    domain.StatusDequeued,
					Attempt:   1,
				})
			}
			return runs, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(16),
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("skip") })},
	})
	exec.logger = slog.New(slog.DiscardHandler)
	defer func() { _ = exec.pool.Shutdown(context.Background()) }()

	b.ResetTimer()
	for range b.N {
		exec.poll(context.Background())
	}
}

func TestAdaptiveDequeue_SkipsWhenPoolSaturated(t *testing.T) {
	t.Parallel()

	var dequeueCalled atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			dequeueCalled.Store(true)
			return nil, nil
		},
	}

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	started := make(chan struct{})
	done := make(chan struct{})
	pool.Submit(context.Background(), func() {
		close(started)
		<-done
	})
	<-started

	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        q,
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
	})

	exec.poll(context.Background())
	close(done)
	require.False(t,
		dequeueCalled.
			Load())
}

func TestAdaptiveDequeue_UsesIdleCount(t *testing.T) {
	t.Parallel()

	var requestedN atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			requestedN.Store(int32(n))
			return nil, nil
		},
	}

	pool := NewPool(10)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	started := make(chan struct{}, 5)
	done := make(chan struct{})
	for range 5 {
		pool.Submit(context.Background(), func() {
			started <- struct{}{}
			<-done
		})
	}
	for range 5 {
		<-started
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        q,
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
	})

	exec.poll(context.Background())
	close(done)

	got := requestedN.Load()
	require.EqualValues(t, 5, got)
}

func TestAdaptiveDequeue_CapsAtMaxBatch(t *testing.T) {
	t.Parallel()

	var requestedN atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			requestedN.Store(int32(n))
			return nil, nil
		},
	}

	pool := NewPool(100)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Store:               &mockExecutorStore{},
		PollInterval:        time.Hour,
		MaxDequeueBatchSize: 10,
	})

	exec.poll(context.Background())

	got := requestedN.Load()
	require.EqualValues(t, 10, got)
}

func TestExecutorRun_DrainsBacklogAfterFullBatchCompletes(t *testing.T) {
	t.Parallel()

	const batchSize = 2
	var dequeueCalls atomic.Int32
	var badRequest atomic.Int32
	var completed atomic.Int32
	var remaining atomic.Int32
	remaining.Store(6)

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			if n <= 0 || n > batchSize {
				badRequest.Store(int32(n))
			}
			call := dequeueCalls.Add(1)
			left := int(remaining.Load())
			if left <= 0 {
				return nil, nil
			}
			claim := min(n, left)
			remaining.Add(int32(-claim))
			runs := make([]domain.JobRun, 0, claim)
			for i := range claim {
				runs = append(runs, domain.JobRun{
					ID:        fmt.Sprintf("run-%d-%d", call, i),
					JobID:     "job-drain",
					ProjectID: "project-drain",
				})
			}
			return runs, nil
		},
	}

	pool := NewPool(batchSize)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	wake := make(chan struct{}, 1)
	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Wake:                wake,
		Store:               &mockExecutorStore{},
		PollInterval:        time.Hour,
		HeartbeatInterval:   time.Hour,
		MaxDequeueBatchSize: batchSize,
	})
	exec.Use(func(_ ExecutionHandler) ExecutionHandler {
		return func(_ context.Context, _ *ExecutionContext) {
			completed.Add(1)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.Run(ctx)
	}()

	wake <- struct{}{}

	require.Eventually(t, func() bool {
		return completed.Load() == 6 && remaining.Load() == 0 && dequeueCalls.Load() >= 4
	}, 2*time.Second, 10*time.Millisecond)
	require.EqualValues(t, 0, badRequest.Load())

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "executor did not stop")
	}
}

func TestExecutorRun_DoesNotDrainAfterPartialBatchCompletes(t *testing.T) {
	t.Parallel()

	var dequeueCalls atomic.Int32
	var completed atomic.Int32

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			if dequeueCalls.Add(1) > 1 {
				return nil, nil
			}
			return []domain.JobRun{{
				ID:        "run-partial",
				JobID:     "job-partial",
				ProjectID: "project-partial",
			}}, nil
		},
	}

	pool := NewPool(2)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	wake := make(chan struct{}, 1)
	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Wake:                wake,
		Store:               &mockExecutorStore{},
		PollInterval:        time.Hour,
		HeartbeatInterval:   time.Hour,
		MaxDequeueBatchSize: 2,
	})
	exec.Use(func(_ ExecutionHandler) ExecutionHandler {
		return func(_ context.Context, _ *ExecutionContext) {
			completed.Add(1)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.Run(ctx)
	}()

	wake <- struct{}{}

	require.Eventually(t, func() bool {
		return completed.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	require.EqualValues(t, 1, dequeueCalls.Load())

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "executor did not stop")
	}
}

func TestExecutorRun_EmptyDrainClearsBacklogHint(t *testing.T) {
	t.Parallel()

	var dequeueCalls atomic.Int32
	var completed atomic.Int32

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			if dequeueCalls.Add(1) > 1 {
				return nil, nil
			}
			return []domain.JobRun{
				{
					ID:        "run-full-a",
					JobID:     "job-full",
					ProjectID: "project-full",
				},
				{
					ID:        "run-full-b",
					JobID:     "job-full",
					ProjectID: "project-full",
				},
			}, nil
		},
	}

	pool := NewPool(2)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	wake := make(chan struct{}, 1)
	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Wake:                wake,
		Store:               &mockExecutorStore{},
		PollInterval:        time.Hour,
		HeartbeatInterval:   time.Hour,
		MaxDequeueBatchSize: 2,
	})
	exec.Use(func(_ ExecutionHandler) ExecutionHandler {
		return func(_ context.Context, _ *ExecutionContext) {
			completed.Add(1)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.Run(ctx)
	}()

	wake <- struct{}{}

	require.Eventually(t, func() bool {
		return completed.Load() == 2 && dequeueCalls.Load() >= 2 && !exec.backlogHint.Load()
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "executor did not stop")
	}
}

func TestAdaptiveDequeue_SingleIdleWorker(t *testing.T) {
	t.Parallel()

	var requestedN atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, n int) ([]domain.JobRun, error) {
			requestedN.Store(int32(n))
			return nil, nil
		},
	}

	pool := NewPool(2)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	started := make(chan struct{})
	done := make(chan struct{})
	pool.Submit(context.Background(), func() {
		close(started)
		<-done
	})
	<-started

	exec := NewExecutor(ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Store:               &mockExecutorStore{},
		PollInterval:        time.Hour,
		MaxDequeueBatchSize: 10,
	})

	exec.poll(context.Background())
	close(done)

	got := requestedN.Load()
	require.EqualValues(t, 1, got)
}

func TestPoll_MemoryPressure_SkipsDequeue(t *testing.T) {
	t.Parallel()

	var dequeueCalled atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			dequeueCalled.Store(true)
			return nil, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:                       NewPool(10),
		Queue:                      q,
		Store:                      &mockExecutorStore{},
		PollInterval:               time.Hour,
		MemoryPressureThresholdPct: 1,
	})

	exec.poll(context.Background())
	require.False(t,
		dequeueCalled.
			Load())
}

func TestPoll_MemoryPressure_DisabledByDefault(t *testing.T) {
	t.Parallel()

	var dequeueCalled atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			dequeueCalled.Store(true)
			return nil, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:                       NewPool(10),
		Queue:                      q,
		Store:                      &mockExecutorStore{},
		PollInterval:               time.Hour,
		MemoryPressureThresholdPct: 0,
	})

	exec.poll(context.Background())
	require.True(t,
		dequeueCalled.
			Load())
}
