package worker

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	heartbeatFlushDuration metric.Float64Histogram
	heartbeatFlushErrors   metric.Int64Counter
)

// init registers package-level OTel instruments. These instruments are
// process-global by design; construction has no external side effects beyond
// registering names with the active meter provider.
func init() {
	meter := otel.Meter("strait/worker")
	heartbeatFlushDuration, _ = meter.Float64Histogram(
		"strait_worker_heartbeat_flush_duration_seconds",
		metric.WithDescription("Duration of heartbeat batch flush to database"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	heartbeatFlushErrors, _ = meter.Int64Counter(
		"strait_worker_heartbeat_flush_errors_total",
		metric.WithDescription("Heartbeat batch flush failures"),
	)
}

func (h *HeartbeatManager) recordOldestLag(ctx context.Context, ids []string) {
	var oldest time.Time
	for _, id := range ids {
		value, ok := h.registeredAt.Load(id)
		if !ok {
			continue
		}
		registeredAt, ok := value.(time.Time)
		if !ok || registeredAt.IsZero() {
			continue
		}
		if oldest.IsZero() || registeredAt.Before(oldest) {
			oldest = registeredAt
		}
	}
	if oldest.IsZero() {
		return
	}
	recordHeartbeatLag(ctx, h.now().Sub(oldest))
}
