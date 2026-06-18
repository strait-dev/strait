package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sourcegraph/conc"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// exitFunc is called when a scheduler component panics. Defaults to
// os.Exit(1). Tests override this to verify the crash path without
// actually killing the process.
var exitFunc = func(code int) { os.Exit(code) }

var captureSchedulerCheckIn = sentry.CaptureCheckIn

type schedulerCheckInContextKey struct{}

type schedulerCheckInContext struct {
	meta      sentrySchedulerMetadata
	component string
}

// safeGo wraps a goroutine with panic recovery. If the function panics,
// the panic is logged with a stack trace, reported to Sentry, and the
// process is terminated. A silently dead scheduler component is worse
// than a restart, so we crash to let the process manager
// restart us.
func safeGo(wg *conc.WaitGroup, name string, fn func()) {
	safeGoWithContext(context.Background(), sentrySchedulerMetadata{}, wg, name, func(context.Context) {
		fn()
	})
}

type sentrySchedulerMetadata struct {
	mode                 string
	region               string
	version              string
	checkInsEnabled      bool
	checkInMonitorPrefix string
}

func safeGoWithContext(ctx context.Context, meta sentrySchedulerMetadata, wg *conc.WaitGroup, name string, fn func(context.Context)) {
	wg.Go(func() {
		ctx := context.WithValue(telemetry.EnsureSentryHub(ctx), schedulerCheckInContextKey{}, schedulerCheckInContext{
			meta:      meta,
			component: name,
		})
		checkInID := startSchedulerLifecycleCheckIn(meta, name)
		checkInStart := time.Now()
		checkInFinished := false
		defer func() {
			if !checkInFinished {
				finishSchedulerLifecycleCheckIn(meta, name, checkInID, sentry.CheckInStatusOK, time.Since(checkInStart))
			}
		}()
		telemetry.AddSentryBreadcrumb(ctx, "scheduler.component", "scheduler component started", map[string]any{
			"component": name,
		})
		defer func() {
			if r := recover(); r != nil {
				checkInFinished = true
				finishSchedulerLifecycleCheckIn(meta, name, checkInID, sentry.CheckInStatusError, time.Since(checkInStart))
				stack := string(debug.Stack())
				telemetry.AddSentryBreadcrumb(ctx, "scheduler.component", "scheduler component panic", map[string]any{
					"component": name,
					"panic":     fmt.Sprintf("%v", r),
				})
				slog.Error("scheduler component panicked, crashing process",
					"component", name,
					"panic", fmt.Sprintf("%v", r),
					"stack", stack,
				)

				if hub := sentry.GetHubFromContext(ctx); hub != nil {
					hub.WithScope(func(scope *sentry.Scope) {
						applySchedulerSentryScope(scope, meta, name, r)
						hub.Recover(r)
					})
					sentry.Flush(2 * time.Second)
				}

				exitFunc(1)
			}
		}()
		fn(ctx)
		checkInFinished = true
		finishSchedulerLifecycleCheckIn(meta, name, checkInID, sentry.CheckInStatusOK, time.Since(checkInStart))
	})
}

func startSchedulerLifecycleCheckIn(meta sentrySchedulerMetadata, component string) sentry.EventID {
	if !meta.checkInsEnabled {
		return ""
	}
	eventID := captureSchedulerCheckIn(&sentry.CheckIn{
		MonitorSlug: schedulerCheckInSlug(meta, component),
		Status:      sentry.CheckInStatusInProgress,
	}, nil)
	if eventID == nil {
		return ""
	}
	return *eventID
}

func finishSchedulerLifecycleCheckIn(
	meta sentrySchedulerMetadata,
	component string,
	checkInID sentry.EventID,
	status sentry.CheckInStatus,
	duration time.Duration,
) {
	if !meta.checkInsEnabled {
		return
	}
	captureSchedulerCheckIn(&sentry.CheckIn{
		ID:          checkInID,
		MonitorSlug: schedulerCheckInSlug(meta, component),
		Status:      status,
		Duration:    duration,
	}, nil)
}

func runSchedulerCycleCheckIn(ctx context.Context, interval time.Duration, fn func()) {
	_ = runSchedulerCycleCheckInWithError(ctx, interval, func() error {
		fn()
		return nil
	})
}

func runSchedulerCycleCheckInWithError(ctx context.Context, interval time.Duration, fn func() error) (err error) {
	checkInCtx, ok := ctx.Value(schedulerCheckInContextKey{}).(schedulerCheckInContext)
	if !ok || !checkInCtx.meta.checkInsEnabled {
		return fn()
	}
	component := checkInCtx.component + "_cycle"
	checkInID := startSchedulerCycleCheckIn(checkInCtx.meta, component, interval)
	started := time.Now()
	status := sentry.CheckInStatusOK
	defer func() {
		if r := recover(); r != nil {
			status = sentry.CheckInStatusError
			finishSchedulerCycleCheckIn(checkInCtx.meta, component, checkInID, status, time.Since(started), nil)
			panic(r)
		}
		if err != nil {
			status = sentry.CheckInStatusError
		}
		finishSchedulerCycleCheckIn(checkInCtx.meta, component, checkInID, status, time.Since(started), nil)
	}()
	return fn()
}

func startSchedulerCycleCheckIn(meta sentrySchedulerMetadata, component string, interval time.Duration) sentry.EventID {
	eventID := captureSchedulerCheckIn(&sentry.CheckIn{
		MonitorSlug: schedulerCheckInSlug(meta, component),
		Status:      sentry.CheckInStatusInProgress,
	}, schedulerMonitorConfig(interval))
	if eventID == nil {
		return ""
	}
	return *eventID
}

func finishSchedulerCycleCheckIn(
	meta sentrySchedulerMetadata,
	component string,
	checkInID sentry.EventID,
	status sentry.CheckInStatus,
	duration time.Duration,
	monitorConfig *sentry.MonitorConfig,
) {
	captureSchedulerCheckIn(&sentry.CheckIn{
		ID:          checkInID,
		MonitorSlug: schedulerCheckInSlug(meta, component),
		Status:      status,
		Duration:    duration,
	}, monitorConfig)
}

func schedulerMonitorConfig(interval time.Duration) *sentry.MonitorConfig {
	if interval <= 0 {
		return nil
	}
	minutes := int64(interval / time.Minute)
	if interval%time.Minute != 0 {
		minutes++
	}
	if minutes < 1 {
		minutes = 1
	}
	return &sentry.MonitorConfig{
		Schedule:      sentry.IntervalSchedule(minutes, sentry.MonitorScheduleUnitMinute),
		CheckInMargin: maxInt64(1, minutes),
		MaxRuntime:    maxInt64(1, minutes),
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func schedulerCheckInSlug(meta sentrySchedulerMetadata, component string) string {
	prefix := sanitizeSchedulerCheckInPart(meta.checkInMonitorPrefix)
	if prefix == "" {
		prefix = "strait-scheduler"
	}
	name := sanitizeSchedulerCheckInPart(component)
	if name == "" {
		name = "component"
	}
	return prefix + "-" + name
}

func sanitizeSchedulerCheckInPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func applySchedulerSentryScope(scope *sentry.Scope, meta sentrySchedulerMetadata, name string, panicValue any) {
	telemetry.ApplySentryRuntimeScope(scope, telemetry.SentryRuntime{
		Edition:   string(domain.BuildEdition()),
		Subsystem: telemetry.SubsystemScheduler,
		Mode:      meta.mode,
		Region:    meta.region,
		Version:   meta.version,
	})
	telemetry.SetSentryTag(scope, telemetry.TagOperation, name)
	scope.SetContext("scheduler.component", sentry.Context{
		"component": name,
		"panic":     fmt.Sprintf("%v", panicValue),
	})
}

// componentTracker records per-component done channels so Stop can wait on
// each one with an individual deadline.
type componentTracker struct {
	mu     sync.Mutex
	items  []componentHandle
	sentry sentrySchedulerMetadata
}

type componentHandle struct {
	name string
	done chan struct{}
}

// track launches fn with panic recovery (via safeGo) and registers a
// per-component done channel so Stop can enforce a shutdown deadline.
func (t *componentTracker) track(ctx context.Context, wg *conc.WaitGroup, name string, fn func(context.Context)) {
	done := make(chan struct{})
	t.mu.Lock()
	t.items = append(t.items, componentHandle{name: name, done: done})
	t.mu.Unlock()
	safeGoWithContext(ctx, t.sentry, wg, name, func(componentCtx context.Context) {
		defer close(done)
		fn(componentCtx)
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
// N*timeout. This keeps scheduler shutdown within the configured shutdown
// deadline even when several components are slow to unwind.
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
		timeout = defaultComponentShutdownTimeout()
	}
	if len(handles) == 0 {
		return 0
	}

	var qm *queue.QueueMetrics
	if m, err := queue.Metrics(); err == nil {
		qm = m
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	var waiterWG conc.WaitGroup
	defer waiterWG.Wait()
	defer cancel()

	finished := make(chan int, len(handles))
	for i, h := range handles {
		waiterWG.Go(func() {
			select {
			case <-h.done:
				finished <- i
			case <-ctx.Done():
				return
			}
		})
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

func defaultComponentShutdownTimeout() time.Duration {
	return 15 * time.Second
}
