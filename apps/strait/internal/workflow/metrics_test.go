package workflow

import (
	"context"
	"testing"
	"time"

	"strait/internal/queue"

	"github.com/stretchr/testify/require"
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
	require.True(t, ok)
	require.EqualValues(t, 1,
		sumInt64WithAttrs(
			transitionData, map[string]string{"from": "pending", "to": "running"}))

	stepDuration := metricByName(t, reader, "strait_workflow_step_duration_seconds")
	stepDurationData, ok := stepDuration.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.EqualValues(t, 1,
		histogramCountWithAttrs(stepDurationData, map[string]string{"step_kind": "job", "outcome": "success"}))

	durableWait := metricByName(t, reader, "strait_workflow_durable_wait_duration_seconds")
	durableWaitData, ok := durableWait.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.EqualValues(t, 1,
		histogramCountWithAttrs(durableWaitData, nil))

	compensation := metricByName(t, reader, "strait_workflow_compensation_runs_total")
	compensationData, ok := compensation.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.EqualValues(t, 1,
		sumInt64WithAttrs(
			compensationData, map[string]string{"outcome": "success"}))

	activeRuns := metricByName(t, reader, "strait_workflow_active_runs")
	activeRunsData, ok := activeRuns.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.False(t, activeRunsData.
		IsMonotonic,
	)
	require.EqualValues(t, 1,
		sumInt64WithAttrs(
			activeRunsData, map[string]string{"project": "project-a"}))
}

func TestWorkflowRuntimeMetrics_BoundsProjectLabels(t *testing.T) {
	reader := setupWorkflowRuntimeMetrics(t, 2)
	ctx := context.Background()

	recordWorkflowActiveRunDelta(ctx, "project-a", 1)
	recordWorkflowActiveRunDelta(ctx, "project-b", 1)

	activeRuns := metricByName(t, reader, "strait_workflow_active_runs")
	activeRunsData, ok := activeRuns.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.EqualValues(t, 1,
		sumInt64WithAttrs(
			activeRunsData, map[string]string{"project": "project-a"}))
	require.EqualValues(t, 1,
		sumInt64WithAttrs(
			activeRunsData, map[string]string{"project": "_other"},
		))
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
		require.NoError(t,
			provider.Shutdown(
				context.
					Background()))
	})
	return reader
}

func metricByName(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(context.
			Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			if metric.Name == name {
				return metric
			}
		}
	}
	require.Failf(t, "test failure",

		"metric %q not collected", name)
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
