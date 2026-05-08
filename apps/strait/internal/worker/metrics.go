package worker

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var workerMetrics = newWorkerMetrics()

type workerRuntimeMetrics struct {
	dispatchDuration metric.Float64Histogram
	retries          metric.Int64Counter
	poolActive       metric.Int64Gauge
	poolIdle         metric.Int64Gauge
	heartbeatLag     metric.Float64Histogram
	dispatchAttempts metric.Int64Counter
	payloadBytes     metric.Int64Histogram
	responseStatus   metric.Int64Counter
}

func newWorkerMetrics() workerRuntimeMetrics {
	meter := otel.Meter("strait/worker_runtime")
	dispatchDuration, _ := meter.Float64Histogram(
		"strait_worker_dispatch_duration_seconds",
		metric.WithDescription("Worker dispatch duration by execution mode and outcome"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60),
	)
	retries, _ := meter.Int64Counter(
		"strait_worker_retry_total",
		metric.WithDescription("Worker retry decisions by execution mode and reason"),
		metric.WithUnit("1"),
	)
	poolActive, _ := meter.Int64Gauge(
		"strait_worker_pool_active",
		metric.WithDescription("Active worker pool slots by execution mode"),
		metric.WithUnit("1"),
	)
	poolIdle, _ := meter.Int64Gauge(
		"strait_worker_pool_idle",
		metric.WithDescription("Idle worker pool slots by execution mode"),
		metric.WithUnit("1"),
	)
	heartbeatLag, _ := meter.Float64Histogram(
		"strait_worker_heartbeat_lag_seconds",
		metric.WithDescription("Age of the oldest active heartbeat at flush time"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.5, 1, 2.5, 5, 10, 30, 60, 120, 300),
	)
	dispatchAttempts, _ := meter.Int64Counter(
		"strait_dispatch_attempts_total",
		metric.WithDescription("Dispatch attempts by execution mode and outcome"),
		metric.WithUnit("1"),
	)
	payloadBytes, _ := meter.Int64Histogram(
		"strait_dispatch_payload_bytes",
		metric.WithDescription("Dispatch payload size by execution mode"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(0, 512, 1024, 4096, 16384, 65536, 262144, 1048576),
	)
	responseStatus, _ := meter.Int64Counter(
		"strait_dispatch_response_status_total",
		metric.WithDescription("HTTP dispatch responses by status class"),
		metric.WithUnit("1"),
	)
	return workerRuntimeMetrics{
		dispatchDuration: dispatchDuration,
		retries:          retries,
		poolActive:       poolActive,
		poolIdle:         poolIdle,
		heartbeatLag:     heartbeatLag,
		dispatchAttempts: dispatchAttempts,
		payloadBytes:     payloadBytes,
		responseStatus:   responseStatus,
	}
}

func recordWorkerDispatch(ctx context.Context, mode, outcome string, started time.Time) {
	if started.IsZero() {
		return
	}
	workerMetrics.dispatchDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(
		attribute.String("mode", mode),
		attribute.String("outcome", outcome),
	))
}

func recordWorkerRetry(ctx context.Context, reason string) {
	workerMetrics.retries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", "grpc"),
		attribute.String("reason", reason),
	))
}

func recordWorkerPool(ctx context.Context, mode string, active, idle int64) {
	attrs := metric.WithAttributes(attribute.String("mode", mode))
	workerMetrics.poolActive.Record(ctx, active, attrs)
	workerMetrics.poolIdle.Record(ctx, idle, attrs)
}

func recordHeartbeatLag(ctx context.Context, lag time.Duration) {
	if lag <= 0 {
		return
	}
	workerMetrics.heartbeatLag.Record(ctx, lag.Seconds())
}

func recordDispatchAttempt(ctx context.Context, mode, outcome string) {
	workerMetrics.dispatchAttempts.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", mode),
		attribute.String("outcome", outcome),
	))
}

func recordDispatchPayloadBytes(ctx context.Context, mode string, size int) {
	workerMetrics.payloadBytes.Record(ctx, int64(size), metric.WithAttributes(attribute.String("mode", mode)))
}

func recordDispatchResponseStatus(ctx context.Context, mode string, statusCode int) {
	workerMetrics.responseStatus.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", mode),
		attribute.String("status_class", statusClass(statusCode)),
	))
}

func statusClass(statusCode int) string {
	if statusCode < 100 || statusCode > 599 {
		return "unknown"
	}
	return string(rune('0'+statusCode/100)) + "xx"
}
