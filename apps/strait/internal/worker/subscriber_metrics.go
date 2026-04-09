package worker

import (
	"context"

	"strait/internal/domain"
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

		// Record run duration on terminal events.
		if isTerminalStatus(event.ToStatus) && event.Run != nil {
			if event.Run.StartedAt != nil && event.Run.FinishedAt != nil {
				dur := event.Run.FinishedAt.Sub(*event.Run.StartedAt).Seconds()
				if dur > 0 {
					statusAttr := attribute.String("status", string(event.ToStatus))
					m.RunDuration.Record(ctx, dur, metric.WithAttributes(
						statusAttr,
						attribute.String("project_id", event.Run.ProjectID),
					))

					// Per-tenant job duration with machine tier for cost attribution.
					tier := "unknown"
					if event.Job != nil && event.Job.MachinePreset != "" {
						tier = string(event.Job.MachinePreset)
					}
					m.JobDuration.Record(ctx, dur, metric.WithAttributes(
						statusAttr,
						attribute.String("project_id", event.Run.ProjectID),
						attribute.String("tier", tier),
					))
				}
			}

			// Per-tenant queue lag: time the run waited before execution began.
			if event.QueueWait > 0 && event.Run.ProjectID != "" {
				m.QueueLag.Record(ctx, event.QueueWait.Seconds(), metric.WithAttributes(
					attribute.String("project_id", event.Run.ProjectID),
				))
			}
		}

		// Latency anomaly detection stays in handleSuccess — it requires a
		// DB call (GetJobHealthStats) that events intentionally don't carry.
	}
}

func isTerminalStatus(s domain.RunStatus) bool {
	return s.IsTerminal()
}
