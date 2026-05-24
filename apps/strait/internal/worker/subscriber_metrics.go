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

			// Per-tenant job duration tagged with execution mode for cost attribution.
			// Guard on non-empty ProjectID — recording project_id="" creates a
			// cardinality-polluting catch-all series with no actionable owner.
			if run.ProjectID != "" {
				m.JobDuration.Record(ctx, dur, metric.WithAttributes(
					statusAttr,
					attribute.String("project_id", run.ProjectID),
					attribute.String("tier", executionModeTier(event)),
				))
			}
		}
	}

	// End-to-end latency: created_at to finished_at.
	if run.FinishedAt != nil && !run.CreatedAt.IsZero() {
		e2e := run.FinishedAt.Sub(run.CreatedAt).Seconds()
		if e2e > 0 {
			m.RunEndToEnd.Record(ctx, e2e, metric.WithAttributes(
				attribute.String("status", string(event.ToStatus)),
			))
		}
	}

	// Per-tenant queue lag: time the run waited before execution began.
	if event.QueueWait > 0 && run.ProjectID != "" {
		m.QueueLag.Record(ctx, event.QueueWait.Seconds(), metric.WithAttributes(
			attribute.String("project_id", run.ProjectID),
		))
	}
}

// executionModeTier returns the execution mode label for the event's job, or
// "unknown" if the job is not set.
func executionModeTier(event RunLifecycleEvent) string {
	if event.Job != nil && event.Job.ExecutionMode != "" {
		return string(event.Job.ExecutionMode)
	}
	return "unknown"
}

func isTerminalStatus(s domain.RunStatus) bool {
	return s.IsTerminal()
}
