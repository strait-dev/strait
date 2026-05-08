package workflow

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var workflowMetrics = newWorkflowMetrics()

type workflowRuntimeMetrics struct {
	stepDuration     metric.Float64Histogram
	stepTransitions  metric.Int64Counter
	compensationRuns metric.Int64Counter
	durableWait      metric.Float64Histogram
	activeRuns       metric.Int64Gauge
}

func newWorkflowMetrics() workflowRuntimeMetrics {
	meter := otel.Meter("strait/workflow_runtime")
	stepDuration, _ := meter.Float64Histogram(
		"strait_workflow_step_duration_seconds",
		metric.WithDescription("Workflow step duration by kind and outcome"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.5, 1, 5, 30, 60, 300, 900),
	)
	stepTransitions, _ := meter.Int64Counter(
		"strait_workflow_step_transitions_total",
		metric.WithDescription("Workflow step status transitions"),
		metric.WithUnit("1"),
	)
	compensationRuns, _ := meter.Int64Counter(
		"strait_workflow_compensation_runs_total",
		metric.WithDescription("Workflow compensation runs by outcome"),
		metric.WithUnit("1"),
	)
	durableWait, _ := meter.Float64Histogram(
		"strait_workflow_durable_wait_duration_seconds",
		metric.WithDescription("Durable workflow wait duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(1, 5, 30, 60, 300, 900, 3600, 21600, 86400),
	)
	activeRuns, _ := meter.Int64Gauge(
		"strait_workflow_active_runs",
		metric.WithDescription("Active workflow runs by bounded project label"),
		metric.WithUnit("1"),
	)
	return workflowRuntimeMetrics{
		stepDuration:     stepDuration,
		stepTransitions:  stepTransitions,
		compensationRuns: compensationRuns,
		durableWait:      durableWait,
		activeRuns:       activeRuns,
	}
}

func recordWorkflowStepTransition(ctx context.Context, from, to string) {
	workflowMetrics.stepTransitions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("from", from),
		attribute.String("to", to),
	))
}

func recordWorkflowStepDuration(ctx context.Context, stepKind, outcome string, startedAt time.Time) {
	if startedAt.IsZero() {
		return
	}
	workflowMetrics.stepDuration.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(
		attribute.String("step_kind", stepKind),
		attribute.String("outcome", outcome),
	))
}

func recordWorkflowCompensation(ctx context.Context, outcome string) {
	workflowMetrics.compensationRuns.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}
