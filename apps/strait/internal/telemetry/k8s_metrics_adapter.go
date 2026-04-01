package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// K8sMetricsAdapter bridges telemetry.Metrics to the compute.K8sMetrics interface.
// This keeps the compute package decoupled from the telemetry package.
type K8sMetricsAdapter struct {
	m *Metrics
}

// NewK8sMetricsAdapter creates an adapter for the given Metrics.
func NewK8sMetricsAdapter(m *Metrics) *K8sMetricsAdapter {
	if m == nil {
		return nil
	}
	return &K8sMetricsAdapter{m: m}
}

// RecordJobCreate records a K8s job creation attempt.
func (a *K8sMetricsAdapter) RecordJobCreate(status, preset string, durationSecs float64) {
	ctx := context.Background()
	a.m.K8sJobCreateTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("preset", preset),
		))
	a.m.K8sJobCreateDuration.Record(ctx, durationSecs,
		metric.WithAttributes(
			attribute.String("preset", preset),
		))
}

// RecordJobWait records a K8s job wait completion.
func (a *K8sMetricsAdapter) RecordJobWait(exitStatus string, durationSecs float64) {
	a.m.K8sJobWaitDuration.Record(context.Background(), durationSecs,
		metric.WithAttributes(
			attribute.String("exit_status", exitStatus),
		))
}

// RecordPodScheduling records the time from job creation to pod entering Running state.
func (a *K8sMetricsAdapter) RecordPodScheduling(durationSecs float64) {
	a.m.K8sPodSchedulingDuration.Record(context.Background(), durationSecs)
}

// IncJobsActive increments or decrements the active jobs counter.
func (a *K8sMetricsAdapter) IncJobsActive(delta int64) {
	a.m.K8sJobsActive.Add(context.Background(), delta)
}
