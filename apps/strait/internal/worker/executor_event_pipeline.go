package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"strait/internal/domain"
)

const (
	defaultEventChannelSize     = 1024
	minEventChannelSize         = 16
	eventChannelSaturationRatio = 0.8
	eventChannelWarnInterval    = 30 * time.Second
	eventChannelKindClosed      = eventChannelKind("closed")
)

type eventChannelKind string

// Subscribe registers a run lifecycle event subscriber. Must be called before Run().
func (e *Executor) Subscribe(sub RunEventSubscriber) {
	e.subscribers = append(e.subscribers, sub)
}

// emit sends a lifecycle event to all subscribers via the buffered channel.
// Non-blocking: drops the event with a warning if the channel is full or closed.
func (e *Executor) emit(ctx context.Context, event RunLifecycleEvent) {
	if len(e.subscribers) == 0 {
		return
	}

	// Recover from send-on-closed-channel if the executor is shutting down
	// and a pool goroutine emits after eventCh is closed.
	defer func() {
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			sentry.Flush(2 * time.Second)
			e.logger.Warn("event channel closed, dropping event",
				"type", event.Type,
				"run_id", event.Run.ID,
			)
			e.recordEventChannelDrop(ctx, eventChannelKindClosed)
		}
	}()

	select {
	case e.eventCh <- runEventEnvelope{ctx: ctx, event: event}:
		e.sampleEventChannelSaturation(ctx, eventChannelKind(event.Type))
	default:
		kind := eventChannelKind(event.Type)
		if e.shouldLogSaturation(kind) {
			e.logger.Warn("event channel full, dropping event",
				"type", event.Type,
				"run_id", event.Run.ID,
			)
		}
		e.recordEventChannelDrop(ctx, kind)
	}
}

// resolveEventChannelSize applies defaults and a lower bound to the configured
// event channel capacity.
func resolveEventChannelSize(configured int) int {
	if configured <= 0 {
		return defaultEventChannelSize
	}
	if configured < minEventChannelSize {
		return minEventChannelSize
	}
	return configured
}

// sampleEventChannelSaturation records the current channel fill ratio and emits
// a rate-limited warning log plus the saturation gauge whenever it exceeds the
// threshold. Per-kind throttling prevents log floods under sustained pressure.
func (e *Executor) sampleEventChannelSaturation(ctx context.Context, kind eventChannelKind) {
	if e.eventChannelSize <= 0 {
		return
	}
	ratio := float64(len(e.eventCh)) / float64(e.eventChannelSize)
	if qm := e.queueMetrics; qm != nil && qm.EventChannelSaturationRatio != nil {
		qm.EventChannelSaturationRatio.Record(ctx, ratio,
			metric.WithAttributes(attribute.String("instance", e.resolveInstanceID())))
	}
	if ratio > eventChannelSaturationRatio && e.shouldLogSaturation(kind) {
		e.logger.Warn("event channel saturated",
			"kind", string(kind),
			"ratio", ratio,
			"depth", len(e.eventCh),
			"capacity", e.eventChannelSize,
		)
	}
}

// resolveInstanceID returns a stable per-process identifier suitable
// for use as a metric attribute. It prefers the OS hostname, which commonly
// matches the container or instance identity, and falls back to a process-scoped
// UUID if Hostname errors or returns empty. Resolution happens at most once per
// Executor; subsequent calls return the cached value.
func (e *Executor) resolveInstanceID() string {
	e.instanceIDOnce.Do(func() {
		host, err := os.Hostname()
		if err == nil && host != "" {
			e.instanceID = host
			return
		}
		e.instanceID = uuid.NewString()
	})
	return e.instanceID
}

// shouldLogSaturation returns true at most once per eventChannelWarnInterval
// per event kind, so the warn log survives sustained backpressure without
// spamming.
func (e *Executor) shouldLogSaturation(kind eventChannelKind) bool {
	e.saturationWarnMu.Lock()
	defer e.saturationWarnMu.Unlock()
	if e.saturationLastWarn == nil {
		e.saturationLastWarn = make(map[eventChannelKind]time.Time)
	}
	now := time.Now()
	if last, ok := e.saturationLastWarn[kind]; ok && now.Sub(last) < eventChannelWarnInterval {
		return false
	}
	e.saturationLastWarn[kind] = now
	return true
}

// recordEventChannelDrop increments the drop counter labelled by event kind.
// No-op when queue metrics have not been initialised. Uses the cached
// Executor queueMetrics handle to avoid a sync.Once + error-check
// lookup on every drop in the lifecycle hot path.
func (e *Executor) recordEventChannelDrop(ctx context.Context, kind eventChannelKind) {
	qm := e.queueMetrics
	if qm == nil || qm.EventChannelDropped == nil {
		return
	}
	qm.EventChannelDropped.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", string(kind))))
}

// runEventLoop drains the event channel and fans out to all subscribers.
// Exits when eventCh is closed (during shutdown or when Run exits).
func (e *Executor) runEventLoop() {
	for env := range e.eventCh {
		for _, sub := range e.subscribers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						e.logger.Error("event subscriber panicked", "panic", r)
					}
				}()
				sub(env.ctx, env.event)
			}()
		}
	}
}

func (e *Executor) notifyWorkflowCallback(ctx context.Context, run *domain.JobRun) {
	if e.workflowCallback == nil {
		return
	}

	e.callbackWG.Go(func() {
		if err := e.workflowCallback.OnJobRunTerminal(ctx, run); err != nil {
			e.logger.Error("workflow callback failed", "run_id", run.ID, "error", err)
		}
	})
}

func (e *Executor) publishEvent(ctx context.Context, run *domain.JobRun, data map[string]any) {
	if e.publisher == nil {
		return
	}

	event := map[string]any{
		"type":       "status_change",
		"run_id":     run.ID,
		"job_id":     run.JobID,
		"project_id": run.ProjectID,
		"timestamp":  time.Now().UTC(),
	}
	maps.Copy(event, data)

	payload, err := json.Marshal(event)
	if err != nil {
		e.logger.Error("failed to marshal event", "error", err)
		return
	}

	channel := fmt.Sprintf("run:%s", run.ID)
	if err := e.publisher.Publish(ctx, channel, payload); err != nil {
		e.logger.Error("failed to publish event", "run_id", run.ID, "error", err)
	}
}
