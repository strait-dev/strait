package worker

import (
	"context"
	"log/slog"
	"sync"
)

// Pool manages a fixed number of concurrent worker goroutines using a
// semaphore pattern (buffered channel).
type Pool struct {
	sem chan struct{}
	wg  sync.WaitGroup
}

// NewPool creates a pool with the given concurrency limit.
func NewPool(concurrency int) *Pool {
	if concurrency < 1 {
		concurrency = 1
	}

	return &Pool{
		sem: make(chan struct{}, concurrency),
	}
}

// Submit schedules work on the pool. It blocks if all slots are occupied.
// If ctx is canceled while waiting for a slot, the work is dropped.
func (p *Pool) Submit(ctx context.Context, fn func()) {
	select {
	case p.sem <- struct{}{}:
		p.wg.Add(1)
		go func() {
			defer func() {
				<-p.sem
				p.wg.Done()
			}()
			fn()
		}()
	case <-ctx.Done():
		slog.Warn("pool: work dropped, context canceled")
	}
}

// ActiveCount returns the number of currently running workers.
func (p *Pool) ActiveCount() int {
	return len(p.sem)
}

func (p *Pool) Available() int {
	return cap(p.sem) - len(p.sem)
}

// Shutdown waits for all in-flight work to complete.
func (p *Pool) Shutdown() {
	p.wg.Wait()
}
