package workflow

import (
	"context"
	"testing"
	"time"

	"strait/internal/queue"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestWorkflowRuntimeMetrics_RecordStepLifecycle(t *testing.T) {
	reader := setupWorkflowRuntimeMetrics(t, 100)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)

	recordWorkflowStepTransition(ctx, "pending", "running")
	recordWorkflowStepTransition(ctx, "running", "running")
	recordWorkflowStepDuration(ctx, "job", "success", &startedAt, finishedAt)
	recordWorkflowDurableWait(ctx, &startedAt, startedAt.Add(10*time.Second))
	RecordWorkflowCompensation(ctx, "success")
	recordWorkflowActiveRunDelta(ctx, "project-a", 1)
	recordWorkflowActiveRunDelta(ctx, "project-a", 1)
	recordWorkflowActiveRunDelta(ctx, "project-a", -1)

	transition := metricByName(t, reader, "strait_workflow_step_transitions_total")
	transitionData, ok := transition.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("transition data = %T, want Sum[int64]", transition.Data)
	}
	if got := sumInt64WithAttrs(transitionData, map[string]string{"from": "pending", "to": "running"}); got != 1 {
		t.Fatalf("transition count = %d, want 1", got)
	}

	stepDuration := metricByName(t, reader, "strait_workflow_step_duration_seconds")
	stepDurationData, ok := stepDuration.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("step duration data = %T, want Histogram[float64]", stepDuration.Data)
	}
	if got := histogramCountWithAttrs(stepDurationData, map[string]string{"step_kind": "job", "outcome": "success"}); got != 1 {
		t.Fatalf("step duration count = %d, want 1", got)
	}

	durableWait := metricByName(t, reader, "strait_workflow_durable_wait_duration_seconds")
	durableWaitData, ok := durableWait.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("durable wait data = %T, want Histogram[float64]", durableWait.Data)
	}
	if got := histogramCountWithAttrs(durableWaitData, nil); got != 1 {
		t.Fatalf("durable wait count = %d, want 1", got)
	}

	compensation := metricByName(t, reader, "strait_workflow_compensation_runs_total")
	compensationData, ok := compensation.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("compensation data = %T, want Sum[int64]", compensation.Data)
	}
	if got := sumInt64WithAttrs(compensationData, map[string]string{"outcome": "success"}); got != 1 {
		t.Fatalf("compensation count = %d, want 1", got)
	}

	activeRuns := metricByName(t, reader, "strait_workflow_active_runs")
	activeRunsData, ok := activeRuns.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("active runs data = %T, want Sum[int64]", activeRuns.Data)
	}
	if activeRunsData.IsMonotonic {
		t.Fatal("active runs must be non-monotonic")
	}
	if got := sumInt64WithAttrs(activeRunsData, map[string]string{"project": "project-a"}); got != 1 {
		t.Fatalf("active runs = %d, want 1", got)
	}
}

func TestWorkflowRuntimeMetrics_BoundsProjectLabels(t *testing.T) {
	reader := setupWorkflowRuntimeMetrics(t, 2)
	ctx := context.Background()

	recordWorkflowActiveRunDelta(ctx, "project-a", 1)
	recordWorkflowActiveRunDelta(ctx, "project-b", 1)

	activeRuns := metricByName(t, reader, "strait_workflow_active_runs")
	activeRunsData, ok := activeRuns.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("active runs data = %T, want Sum[int64]", activeRuns.Data)
	}
	if got := sumInt64WithAttrs(activeRunsData, map[string]string{"project": "project-a"}); got != 1 {
		t.Fatalf("allowlisted active runs = %d, want 1", got)
	}
	if got := sumInt64WithAttrs(activeRunsData, map[string]string{"project": "_other"}); got != 1 {
		t.Fatalf("fallback active runs = %d, want 1", got)
	}
}

func setupWorkflowRuntimeMetrics(t *testing.T, projectLabelLimit int) *sdkmetric.ManualReader {
	t.Helper()
	oldProvider := otel.GetMeterProvider()
	oldMetrics := workflowMetrics
	oldProjectLabels := workflowProjectLabels

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	workflowProjectLabels = queue.NewProjectLabelAllowlist(projectLabelLimit)
	workflowMetrics = newWorkflowMetrics()

	t.Cleanup(func() {
		workflowMetrics = oldMetrics
		workflowProjectLabels = oldProjectLabels
		otel.SetMeterProvider(oldProvider)
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown meter provider: %v", err)
		}
	})
	return reader
}

func metricByName(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			if metric.Name == name {
				return metric
			}
		}
	}
	t.Fatalf("metric %q not collected", name)
	return metricdata.Metrics{}
}

func sumInt64WithAttrs(data metricdata.Sum[int64], attrs map[string]string) int64 {
	var total int64
	for _, dp := range data.DataPoints {
		if dataPointMatches(dp.Attributes, attrs) {
			total += dp.Value
		}
	}
	return total
}

func histogramCountWithAttrs(data metricdata.Histogram[float64], attrs map[string]string) uint64 {
	var total uint64
	for _, dp := range data.DataPoints {
		if dataPointMatches(dp.Attributes, attrs) {
			total += dp.Count
		}
	}
	return total
}

func dataPointMatches(set attribute.Set, attrs map[string]string) bool {
	for key, want := range attrs {
		got, ok := set.Value(attribute.Key(key))
		if !ok || got.AsString() != want {
			return false
		}
	}
	return true
}
