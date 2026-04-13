package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sourcegraph/conc"

	"strait/internal/queue"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// exitFunc is called when a scheduler component panics. Defaults to
// os.Exit(1). Tests override this to verify the crash path without
// actually killing the process.
var exitFunc = func(code int) { os.Exit(code) }

// safeGo wraps a goroutine with panic recovery. If the function panics,
// the panic is logged with a stack trace, reported to Sentry, and the
// process is terminated. A silently dead scheduler component is worse
// than a restart, so we crash to let the orchestrator (systemd/k8s)
// restart us.
func safeGo(wg *conc.WaitGroup, name string, fn func()) {
	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				slog.Error("scheduler component panicked, crashing process",
					"component", name,
					"panic", fmt.Sprintf("%v", r),
					"stack", stack,
				)

				if hub := sentry.CurrentHub(); hub != nil {
					hub.Recover(r)
					sentry.Flush(2 * time.Second)
				}

				exitFunc(1)
			}
		}()
		fn()
	})
}

// componentTracker records per-component done channels so Stop can wait on
// each one with an individual deadline.
type componentTracker struct {
	mu    sync.Mutex
	items []componentHandle
}

type componentHandle struct {
	name string
	done chan struct{}
}

// track launches fn with panic recovery (via safeGo) and registers a
// per-component done channel so Stop can enforce a shutdown deadline.
func (t *componentTracker) track(wg *conc.WaitGroup, name string, fn func()) {
	done := make(chan struct{})
	t.mu.Lock()
	t.items = append(t.items, componentHandle{name: name, done: done})
	t.mu.Unlock()
	safeGo(wg, name, func() {
		defer close(done)
		fn()
	})
}

// waitWithTimeout blocks until every tracked component finishes or the
// single shared deadline elapses. Components still running past the
// deadline are logged at Error and counted on
// strait.scheduler.shutdown_timeouts_total. Returns the number of
// components that timed out.
//
// Waits run concurrently under one deadline: if N components each can
// take up to `timeout` to drain, total wall-clock is ~timeout, not
// N*timeout. This keeps scheduler shutdown within the k8s
// terminationGracePeriodSeconds envelope even when several components
// are slow to unwind.
//
// Uses context.WithTimeout rather than a shared time.After channel
// because a time.After value can only be consumed once: if any watcher
// goroutine drains it first, the outer select can no longer observe
// the deadline and blocks forever on finished. A Done() channel,
// being close-based, is safely broadcast to every receiver.
func (t *componentTracker) waitWithTimeout(parent context.Context, timeout time.Duration) int {
	t.mu.Lock()
	handles := make([]componentHandle, len(t.items))
	copy(handles, t.items)
	t.mu.Unlock()

	if timeout <= 0 {
		timeout = defaultComponentShutdownTimeout
	}
	if len(handles) == 0 {
		return 0
	}

	var qm *queue.QueueMetrics
	if m, err := queue.Metrics(); err == nil {
		qm = m
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	finished := make(chan int, len(handles))
	for i, h := range handles {
		go func(idx int, h componentHandle) {
			select {
			case <-h.done:
				finished <- idx
			case <-ctx.Done():
				return
			}
		}(i, h)
	}

	done := make(map[int]struct{}, len(handles))
	for len(done) < len(handles) {
		select {
		case idx := <-finished:
			done[idx] = struct{}{}
		case <-ctx.Done():
			// Deadline hit: log everything still unfinished and count.
			timedOut := 0
			for i, h := range handles {
				if _, ok := done[i]; ok {
					continue
				}
				timedOut++
				slog.Error("scheduler component exceeded shutdown deadline",
					"component", h.name,
					"timeout", timeout,
				)
				if qm != nil && qm.SchedulerShutdownTimeouts != nil {
					// Use parent rather than ctx because ctx is already
					// cancelled by the timeout; the metric SDK may still
					// propagate the attributes but downstream exporters
					// often honour ctx.Err() and drop cancelled records.
					qm.SchedulerShutdownTimeouts.Add(parent, 1, metric.WithAttributes(
						attribute.String("component", h.name),
					))
				}
			}
			return timedOut
		}
	}
	return 0
}

const defaultComponentShutdownTimeout = 15 * time.Second
