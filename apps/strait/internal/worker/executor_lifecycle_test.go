package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

func TestExecutor_GracefulShutdown(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	jobStarted := make(chan struct{})
	jobCanProceed := make(chan struct{})

	var startedOnce sync.Once
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		startedOnce.Do(func() { close(jobStarted) })
		<-jobCanProceed
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer ts.Close()

	var transitionsMu sync.Mutex
	transitions := make([]string, 0, 2)

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		return &domain.Job{
			ID:          id,
			EndpointURL: ts.URL,
			MaxAttempts: 3,
			TimeoutSecs: 30,
		}, nil
	}
	store.updateRunStatusFn = func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
		transitionsMu.Lock()
		transitions = append(transitions, fmt.Sprintf("%s->%s", from, to))
		transitionsMu.Unlock()
		return nil
	}

	var dequeueCount atomic.Int32
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			if dequeueCount.Add(1) == 1 {
				return []domain.JobRun{{
					ID:      "run-shutdown-1",
					JobID:   "job-1",
					Status:  domain.StatusDequeued,
					Attempt: 1,
				}}, nil
			}
			return nil, nil
		},
	}

	pool := NewPool(5)
	ctx, cancel := context.WithCancel(context.Background())

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		HTTPClient:        ts.Client(),
		PollInterval:      5 * time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	runDone := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(runDone)
	})

	select {
	case <-jobStarted:
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for job to start")
	}

	cancel()

	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		require.Fail(t, "Run() did not exit after context cancellation")
	}

	close(jobCanProceed)

	shutdownDone := make(chan struct{})
	concWG.Go(func() {
		_ = pool.Shutdown(context.Background())
		close(shutdownDone)
	})

	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		require.Fail(t, "pool.Shutdown() did not return")
	}

	transitionsMu.Lock()
	defer transitionsMu.Unlock()
	require.GreaterOrEqual(t,
		len(transitions), 2,
	)
	assert.Equal(t,
		"dequeued->executing",

		transitions[0])

	last := transitions[len(transitions)-1]
	assert.Equal(t,
		"executing->completed",

		last)

}

func TestExecutor_Run_PollsOnWakeSignal(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	wake := make(chan struct{}, 1)
	polled := make(chan struct{}, 1)

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case polled <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Wake:              wake,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(done)
	})

	wake <- struct{}{}

	select {
	case <-polled:
	case <-time.After(time.Second):
		require.Fail(t, "expected poll to run after wake signal")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "executor did not stop after context cancel")
	}
}

func TestExecutor_Run_DegradedModeShortensPollInterval(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	wake := make(chan struct{}, 1)
	degradedCh := make(chan struct{})
	pollCount := make(chan struct{}, 100)

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case pollCount <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:                 pool,
		Queue:                q,
		Wake:                 wake,
		Degraded:             &mockDegradedNotifier{ch: degradedCh},
		DegradedPollInterval: 50 * time.Millisecond,
		Store:                &mockExecutorStore{},
		PollInterval:         time.Hour,
		HeartbeatInterval:    time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(done)
	})

	close(degradedCh)

	deadline := time.After(2 * time.Second)
	polls := 0
	for polls < 3 {
		select {
		case <-pollCount:
			polls++
		case <-deadline:
			require.Failf(t, "test failure", "expected at least 3 degraded polls, got %d", polls)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "executor did not stop after context cancel")
	}
}

type rearmDegradedNotifier struct {
	mu    sync.Mutex
	calls int
	chs   []<-chan struct{}
}

func (r *rearmDegradedNotifier) Degraded() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := r.calls
	r.calls++
	if idx < len(r.chs) {
		return r.chs[idx]
	}
	return make(chan struct{})
}

func TestExecutor_DegradedRecoveryDoesNotReenterOnStaleChannel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	wake := make(chan struct{}, 1)
	pollCount := atomic.Int64{}

	closedCh := make(chan struct{})
	close(closedCh)
	openCh := make(chan struct{})

	notifier := &rearmDegradedNotifier{
		chs: []<-chan struct{}{closedCh, openCh},
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			pollCount.Add(1)
			return nil, nil
		},
	}

	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	exec := NewExecutor(ExecutorConfig{
		Pool:                 pool,
		Queue:                q,
		Wake:                 wake,
		Degraded:             notifier,
		DegradedPollInterval: 50 * time.Millisecond,
		Store:                &mockExecutorStore{},
		PollInterval:         time.Hour,
		HeartbeatInterval:    time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(done)
	})

	time.Sleep(200 * time.Millisecond)

	wake <- struct{}{}
	time.Sleep(100 * time.Millisecond)

	baseline := pollCount.Load()
	time.Sleep(300 * time.Millisecond)
	final := pollCount.Load()
	assert.LessOrEqual(t, final-
		baseline,
		int64(2))

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "executor did not stop after context cancel")
	}
}

func TestExecutor_Shutdown_NoInFlight(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	exec := newTestExecutor(t, &mockExecutorStore{}, &mockExecQueue{}, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	runDone := make(chan struct{})
	concWG.Go(func() {
		exec.Run(ctx)
		close(runDone)
	})

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	require.NoError(
		t, exec.Shutdown(
			shutdownCtx),
	)

	select {
	case <-runDone:
	case <-time.After(time.Second):
		require.Fail(t, "executor Run did not stop after shutdown")
	}
}

func TestExecutor_Shutdown_WaitsForInFlight(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	pollStarted := make(chan struct{})
	allowPollExit := make(chan struct{})
	wake := make(chan struct{}, 1)

	q := &mockExecQueue{
		dequeueNFn: func(ctx context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case <-pollStarted:
			default:
				close(pollStarted)
			}
			select {
			case <-allowPollExit:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Wake:              wake,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	runDone := make(chan struct{})
	concWG.Go(func() {
		exec.Run(runCtx)
		close(runDone)
	})

	wake <- struct{}{}
	waitForSignal(t, pollStarted, "poll did not start")

	shutdownDone := make(chan error, 1)
	concWG.Go(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		shutdownDone <- exec.Shutdown(shutdownCtx)
	})

	select {
	case err := <-shutdownDone:
		require.Failf(t, "test failure", "Shutdown returned early with err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowPollExit)

	select {
	case err := <-shutdownDone:
		if err != nil {
			require.Failf(t, "test failure",

				"Shutdown() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		require.Fail(t, "Shutdown did not return after poll completed")
	}

	select {
	case <-runDone:
	case <-time.After(time.Second):
		require.Fail(t, "executor Run did not stop after shutdown")
	}
}

func TestExecutor_Shutdown_Timeout(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	pollStarted := make(chan struct{})
	allowPollExit := make(chan struct{})
	wake := make(chan struct{}, 1)

	q := &mockExecQueue{
		dequeueNFn: func(ctx context.Context, _ int) ([]domain.JobRun, error) {
			select {
			case <-pollStarted:
			default:
				close(pollStarted)
			}
			select {
			case <-allowPollExit:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Wake:              wake,
		Store:             &mockExecutorStore{},
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	concWG.Go(func() {
		exec.Run(runCtx)
		close(runDone)
	})

	wake <- struct{}{}
	waitForSignal(t, pollStarted, "poll did not start")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer shutdownCancel()
	err := exec.Shutdown(shutdownCtx)
	require.True(t,
		errors.Is(
			err, context.
				DeadlineExceeded,
		))

	runCancel()
	close(allowPollExit)
	select {
	case <-runDone:
	case <-time.After(time.Second):
		require.Fail(t, "executor Run did not stop after cancel")
	}
}

func TestShutdown_WaitsForCallbacks(t *testing.T) {
	t.Parallel()

	var callbackCalled atomic.Bool
	callback := &mockWorkflowCallback{
		onTerminalFn: func(_ context.Context, _ *domain.JobRun) error {
			time.Sleep(100 * time.Millisecond)
			callbackCalled.Store(true)
			return nil
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	var dequeued atomic.Bool
	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			if dequeued.CompareAndSwap(false, true) {
				return []domain.JobRun{*testRun(1)}, nil
			}
			return nil, nil
		},
	}

	pool := NewPool(4)
	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      50 * time.Millisecond,
		HeartbeatInterval: time.Hour,
		WorkflowCallback:  callback,
		HTTPClient:        server.Client(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if callbackCalled.Load() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	require.NoError(
		t, exec.Shutdown(
			shutdownCtx),
	)

	_ = pool.Shutdown(context.Background())
	require.True(t,
		callbackCalled.
			Load())

}

func TestShutdown_NoCallbacksNoDelay(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	q := &mockExecQueue{}

	pool := NewPool(4)
	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Hour,
		HeartbeatInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go exec.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()

	start := time.Now()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	require.NoError(
		t, exec.Shutdown(
			shutdownCtx),
	)

	_ = pool.Shutdown(context.Background())

	elapsed := time.Since(start)
	require.LessOrEqual(t, elapsed,
		2*
			time.Second,
	)

}
