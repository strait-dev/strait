package workflow

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestWorkflowRuntimeMetrics_RecordContinuation verifies the continue-as-new
// telemetry: the counter increments per continuation and the depth gauge holds
// the most recent successor's lineage depth, both labeled by bounded project.
func TestWorkflowRuntimeMetrics_RecordContinuation(t *testing.T) {
	reader := setupWorkflowRuntimeMetrics(t, 100)
	ctx := context.Background()

	recordWorkflowContinuation(ctx, "project-a", 1)
	recordWorkflowContinuation(ctx, "project-a", 2)

	continuations := metricByName(t, reader, "strait_workflow_continuations_total")
	contData, ok := continuations.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("continuations data = %T, want Sum[int64]", continuations.Data)
	}
	if got := sumInt64WithAttrs(contData, map[string]string{"project": "project-a"}); got != 2 {
		t.Fatalf("continuations count = %d, want 2", got)
	}

	depth := metricByName(t, reader, "strait_workflow_continuation_depth")
	depthData, ok := depth.Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("continuation depth data = %T, want Gauge[int64]", depth.Data)
	}
	if got := gaugeInt64WithAttrs(depthData, map[string]string{"project": "project-a"}); got != 2 {
		t.Fatalf("continuation depth = %d, want 2 (latest successor depth)", got)
	}
}

// TestWorkflowRuntimeMetrics_ContinuationBoundsProjectLabels verifies the depth
// gauge folds unknown projects into the bounded fallback label.
func TestWorkflowRuntimeMetrics_ContinuationBoundsProjectLabels(t *testing.T) {
	// Limit 2 leaves room for one real project plus the _other fallback.
	reader := setupWorkflowRuntimeMetrics(t, 2)
	ctx := context.Background()

	recordWorkflowContinuation(ctx, "project-a", 1)
	recordWorkflowContinuation(ctx, "project-b", 3)

	continuations := metricByName(t, reader, "strait_workflow_continuations_total")
	contData, ok := continuations.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("continuations data = %T, want Sum[int64]", continuations.Data)
	}
	if got := sumInt64WithAttrs(contData, map[string]string{"project": "project-a"}); got != 1 {
		t.Fatalf("allowlisted continuations = %d, want 1", got)
	}
	if got := sumInt64WithAttrs(contData, map[string]string{"project": "_other"}); got != 1 {
		t.Fatalf("fallback continuations = %d, want 1", got)
	}
}

func gaugeInt64WithAttrs(data metricdata.Gauge[int64], attrs map[string]string) int64 {
	var v int64
	for _, dp := range data.DataPoints {
		if dataPointMatches(dp.Attributes, attrs) {
			v = dp.Value
		}
	}
	return v
}
