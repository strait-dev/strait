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

// waitWithTimeout blocks until every tracked component finishes or its
// per-component timeout elapses. Components that exceed the deadline are
// logged at Error and counted on strait.scheduler.shutdown_timeouts_total.
// Returns the number of components that timed out.
func (t *componentTracker) waitWithTimeout(ctx context.Context, timeout time.Duration) int {
	t.mu.Lock()
	handles := make([]componentHandle, len(t.items))
	copy(handles, t.items)
	t.mu.Unlock()

	if timeout <= 0 {
		timeout = defaultComponentShutdownTimeout
	}

	var qm *queue.QueueMetrics
	if m, err := queue.Metrics(); err == nil {
		qm = m
	}

	timedOut := 0
	for _, h := range handles {
		timer := time.NewTimer(timeout)
		select {
		case <-h.done:
			timer.Stop()
		case <-timer.C:
			timedOut++
			slog.Error("scheduler component exceeded shutdown deadline",
				"component", h.name,
				"timeout", timeout,
			)
			if qm != nil && qm.SchedulerShutdownTimeouts != nil {
				qm.SchedulerShutdownTimeouts.Add(ctx, 1, metric.WithAttributes(
					attribute.String("component", h.name),
				))
			}
		}
	}
	return timedOut
}

const defaultComponentShutdownTimeout = 15 * time.Second
