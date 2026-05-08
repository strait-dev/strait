package workflow

import (
	"context"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var workflowMetrics = newWorkflowMetrics()
var workflowProjectLabels = queue.NewProjectLabelAllowlist(100)

type workflowRuntimeMetrics struct {
	stepDuration     metric.Float64Histogram
	stepTransitions  metric.Int64Counter
	compensationRuns metric.Int64Counter
	durableWait      metric.Float64Histogram
	activeRuns       metric.Int64UpDownCounter
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
	activeRuns, _ := meter.Int64UpDownCounter(
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
	if from == "" || to == "" || from == to {
		return
	}
	workflowMetrics.stepTransitions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("from", from),
		attribute.String("to", to),
	))
}

func recordWorkflowStepDuration(ctx context.Context, stepKind, outcome string, startedAt *time.Time, finishedAt time.Time) {
	if startedAt == nil || startedAt.IsZero() {
		return
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	duration := finishedAt.Sub(*startedAt)
	if duration < 0 {
		duration = 0
	}
	workflowMetrics.stepDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("step_kind", normalizeWorkflowStepKind(stepKind)),
		attribute.String("outcome", outcome),
	))
}

func RecordWorkflowCompensation(ctx context.Context, outcome string) {
	workflowMetrics.compensationRuns.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

func recordWorkflowDurableWait(ctx context.Context, startedAt *time.Time, finishedAt time.Time) {
	if startedAt == nil || startedAt.IsZero() {
		return
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	duration := finishedAt.Sub(*startedAt)
	if duration < 0 {
		duration = 0
	}
	workflowMetrics.durableWait.Record(ctx, duration.Seconds())
}

func recordWorkflowActiveRunDelta(ctx context.Context, projectID string, delta int64) {
	if delta == 0 {
		return
	}
	workflowMetrics.activeRuns.Add(ctx, delta, metric.WithAttributes(attribute.String("project", projectLabel(projectID))))
}

func normalizeWorkflowStepKind(kind string) string {
	switch kind {
	case "job", "approval", "sub_workflow", "wait_for_event", "sleep":
		return kind
	case "":
		return "unknown"
	default:
		return "unknown"
	}
}

func projectLabel(projectID string) string {
	if projectID == "" {
		return "_other"
	}
	workflowProjectLabels.Add(projectID)
	return workflowProjectLabels.Label(projectID)
}

func workflowStepKind(wc *wfCtx, stepRun *domain.WorkflowStepRun) string {
	if wc == nil || stepRun == nil {
		return "unknown"
	}
	step, ok := wc.stepByRef[stepRun.StepRef]
	if !ok {
		return "unknown"
	}
	return string(step.StepType)
}

func workflowStepOutcome(status domain.StepRunStatus) string {
	switch status {
	case domain.StepCompleted:
		return "success"
	case domain.StepFailed:
		return "failure"
	case domain.StepSkipped:
		return "skipped"
	case domain.StepCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}
