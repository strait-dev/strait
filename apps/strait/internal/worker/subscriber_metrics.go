package worker

import (
	"context"

	"strait/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MetricsSubscriber records run lifecycle metrics from events.
func MetricsSubscriber(m *telemetry.Metrics) RunEventSubscriber {
	return func(ctx context.Context, event RunLifecycleEvent) {
		m.RunTransitions.Add(ctx, 1, metric.WithAttributes(
			attribute.String("from", string(event.FromStatus)),
			attribute.String("to", string(event.ToStatus)),
		))

		if event.ExecTrace != nil {
			m.ExecutionTraceDispatch.Record(ctx, float64(event.ExecTrace.DispatchMs))
			m.ExecutionTraceQueueWait.Record(ctx, float64(event.ExecTrace.QueueWaitMs))
		}

		if event.Type == EventSnoozed {
			m.SnoozeTotal.Add(ctx, 1)
		}

		// Latency anomaly detection stays in handleSuccess — it requires a
		// DB call (GetJobHealthStats) that events intentionally don't carry.
	}
}
