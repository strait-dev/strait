package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/sourcegraph/conc"
)

// Pool manages a fixed number of concurrent worker goroutines backed by
// github.com/alitto/pond/v2. It provides the same Submit/Shutdown interface
// as the previous semaphore-based pool plus observability methods for
// running workers, waiting tasks, and throughput counters.
type Pool struct {
	inner       pond.Pool
	concurrency int
}

// PoolOption configures optional pool behaviour.
type PoolOption func(*poolConfig)

type poolConfig struct {
	queueSize int
}

// WithQueueSize sets a bounded task queue for backpressure. When the queue
// is full, Submit blocks until a slot opens or the context is canceled.
// A value ≤ 0 means unbounded (the default).
func WithQueueSize(n int) PoolOption {
	return func(c *poolConfig) { c.queueSize = n }
}

// NewPool creates a pool with the given concurrency limit and options.
func NewPool(concurrency int, opts ...PoolOption) *Pool {
	if concurrency < 1 {
		concurrency = 1
	}

	cfg := poolConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	var pondOpts []pond.Option
	if cfg.queueSize > 0 {
		pondOpts = append(pondOpts, pond.WithQueueSize(cfg.queueSize))
	}

	return &Pool{
		inner:       pond.NewPool(concurrency, pondOpts...),
		concurrency: concurrency,
	}
}

// Submit schedules work on the pool. With the default unbounded queue,
// this returns immediately. With WithQueueSize, it blocks if the queue
// is full. The ctx check before Submit is a best-effort guard; once
// Submit is called, the task is queued regardless of later cancellation.
func (p *Pool) Submit(ctx context.Context, fn func()) {
	select {
	case <-ctx.Done():
		slog.Warn("pool: work dropped, context canceled before submit")
		return
	default:
	}
	p.inner.Submit(fn)
}

// ActiveCount returns the number of currently running workers.
func (p *Pool) ActiveCount() int {
	running, _ := p.observedSnapshot()
	return running
}

// Available returns the number of idle worker slots.
func (p *Pool) Available() int {
	_, idle := p.observedSnapshot()
	return idle
}

func (p *Pool) observedSnapshot() (running int, idle int) {
	running, idle = p.snapshot()
	recordWorkerPool(context.Background(), dispatchModeHTTP, int64(running), int64(idle))
	return running, idle
}

func (p *Pool) snapshot() (running int, idle int) {
	running = int(p.inner.RunningWorkers())
	if running >= p.concurrency {
		return running, 0
	}
	return running, p.concurrency - running
}

// RunningWorkers returns the current number of goroutines executing tasks.
func (p *Pool) RunningWorkers() int64 {
	return p.inner.RunningWorkers()
}

// WaitingTasks returns the number of tasks waiting in the queue.
func (p *Pool) WaitingTasks() uint64 {
	return p.inner.WaitingTasks()
}

// SubmittedTasks returns the total number of tasks submitted to the pool.
func (p *Pool) SubmittedTasks() uint64 {
	return p.inner.SubmittedTasks()
}

// CompletedTasks returns the total number of tasks that have finished
// (successfully or with failure).
func (p *Pool) CompletedTasks() uint64 {
	return p.inner.CompletedTasks()
}

// SuccessfulTasks returns the total number of tasks that completed without error.
func (p *Pool) SuccessfulTasks() uint64 {
	return p.inner.SuccessfulTasks()
}

// FailedTasks returns the total number of tasks that panicked or returned an error.
func (p *Pool) FailedTasks() uint64 {
	return p.inner.FailedTasks()
}

// DroppedTasks returns the total number of tasks that were dropped because
// the pool was stopped before they could be executed.
func (p *Pool) DroppedTasks() uint64 {
	return p.inner.DroppedTasks()
}

// Shutdown waits for all in-flight work to complete. If ctx is canceled
// before all workers finish, it returns ctx.Err(). This prevents graceful
// shutdown from blocking indefinitely on stuck HTTP dispatches.
func (p *Pool) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	var wg conc.WaitGroup
	wg.Go(func() {
		p.inner.StopAndWait()
		close(done)
	})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		p.inner.Stop()

		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			slog.Warn("pool: timed out waiting for StopAndWait goroutine after Stop")
		}

		return ctx.Err()
	}
}
