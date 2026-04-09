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

		if isTerminalStatus(event.ToStatus) && event.Run != nil {
			recordTerminalMetrics(ctx, m, event)
		}

		// Latency anomaly detection stays in handleSuccess — it requires a
		// DB call (GetJobHealthStats) that events intentionally don't carry.
	}
}

// recordTerminalMetrics emits duration and queue-lag metrics for a run that has
// reached a terminal state. Called only when event.Run is non-nil.
func recordTerminalMetrics(ctx context.Context, m *telemetry.Metrics, event RunLifecycleEvent) {
	run := event.Run

	if run.StartedAt != nil && run.FinishedAt != nil {
		dur := run.FinishedAt.Sub(*run.StartedAt).Seconds()
		if dur > 0 {
			statusAttr := attribute.String("status", string(event.ToStatus))
			m.RunDuration.Record(ctx, dur, metric.WithAttributes(
				statusAttr,
				attribute.String("project_id", run.ProjectID),
			))

			// Per-tenant job duration with machine tier for cost attribution.
			// Guard on non-empty ProjectID — recording project_id="" creates a
			// cardinality-polluting catch-all series with no actionable owner.
			if run.ProjectID != "" {
				m.JobDuration.Record(ctx, dur, metric.WithAttributes(
					statusAttr,
					attribute.String("project_id", run.ProjectID),
					attribute.String("tier", machineTier(event)),
				))
			}
		}
	}

	// Per-tenant queue lag: time the run waited before execution began.
	if event.QueueWait > 0 && run.ProjectID != "" {
		m.QueueLag.Record(ctx, event.QueueWait.Seconds(), metric.WithAttributes(
			attribute.String("project_id", run.ProjectID),
		))
	}
}

// machineTier returns the machine preset label for the event's job, or "unknown"
// if the job or preset is not set.
func machineTier(event RunLifecycleEvent) string {
	if event.Job != nil && event.Job.MachinePreset != "" {
		return string(event.Job.MachinePreset)
	}
	return "unknown"
}

func isTerminalStatus(s domain.RunStatus) bool {
	return s.IsTerminal()
}
