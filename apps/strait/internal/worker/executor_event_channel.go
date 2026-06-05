package worker

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	defaultEventChannelSize     = 1024
	minEventChannelSize         = 16
	eventChannelSaturationRatio = 0.8
	eventChannelWarnInterval    = 30 * time.Second
	eventChannelKindClosed      = eventChannelKind("closed")
)

type eventChannelKind string

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
