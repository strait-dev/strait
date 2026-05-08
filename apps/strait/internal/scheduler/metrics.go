package scheduler

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var schedulerMetrics = newSchedulerMetrics()

type schedulerRuntimeMetrics struct {
	loopDuration metric.Float64Histogram
	skew         metric.Float64Gauge
	overrun      metric.Int64Counter
	swept        metric.Int64Counter
}

func newSchedulerMetrics() schedulerRuntimeMetrics {
	meter := otel.Meter("strait/scheduler")
	loopDuration, _ := meter.Float64Histogram(
		"strait_scheduler_loop_duration_seconds",
		metric.WithDescription("Scheduler loop execution duration by loop"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 15, 30, 60),
	)
	skew, _ := meter.Float64Gauge(
		"strait_scheduler_skew_seconds",
		metric.WithDescription("Scheduler tick skew at loop start"),
		metric.WithUnit("s"),
	)
	overrun, _ := meter.Int64Counter(
		"strait_scheduler_overrun_total",
		metric.WithDescription("Scheduler ticks that took longer than their interval"),
		metric.WithUnit("1"),
	)
	swept, _ := meter.Int64Counter(
		"strait_scheduler_swept_total",
		metric.WithDescription("Items swept by scheduler loops"),
		metric.WithUnit("1"),
	)
	return schedulerRuntimeMetrics{loopDuration: loopDuration, skew: skew, overrun: overrun, swept: swept}
}

func recordSchedulerLoop(ctx context.Context, loop string, scheduledAt, startedAt time.Time, interval time.Duration, sweptKind string, swept int64) {
	attrs := metric.WithAttributes(attribute.String("loop", loop))
	duration := time.Since(startedAt)
	schedulerMetrics.loopDuration.Record(ctx, duration.Seconds(), attrs)
	if !scheduledAt.IsZero() && startedAt.After(scheduledAt) {
		schedulerMetrics.skew.Record(ctx, startedAt.Sub(scheduledAt).Seconds(), attrs)
	}
	if interval > 0 && duration > interval {
		schedulerMetrics.overrun.Add(ctx, 1, attrs)
	}
	if swept > 0 {
		schedulerMetrics.swept.Add(ctx, swept, metric.WithAttributes(
			attribute.String("loop", loop),
			attribute.String("kind", sweptKind),
		))
	}
}
