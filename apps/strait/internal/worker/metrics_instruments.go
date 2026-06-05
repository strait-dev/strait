package worker

import (
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// workerMetrics holds the package-level metric instruments. It is an atomic
// pointer so tests can swap in a meter backed by a ManualReader without
// racing against in-flight Add/Record calls from concurrent tests.
var workerMetrics atomic.Pointer[workerRuntimeMetrics]

// init seeds the default runtime metric instruments before any worker code can
// record against them. Tests replace this atomic pointer with a ManualReader
// backed meter when they need deterministic collection.
func init() {
	m := newWorkerMetrics()
	workerMetrics.Store(&m)
}

type workerRuntimeMetrics struct {
	dispatchDuration metric.Float64Histogram
	retries          metric.Int64Counter
	poolActive       metric.Int64Gauge
	poolIdle         metric.Int64Gauge
	heartbeatLag     metric.Float64Histogram
	dispatchAttempts metric.Int64Counter
	payloadBytes     metric.Int64Histogram
	responseStatus   metric.Int64Counter
	snoozeSkipped    metric.Int64Counter
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
	snoozeSkipped, _ := meter.Int64Counter(
		"strait_worker_snooze_skipped_total",
		metric.WithDescription("Snooze attempts that no-op'd because the run row was already locked by another transaction or had moved past the expected status. Labeled by from-status and reason."),
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
		snoozeSkipped:    snoozeSkipped,
	}
}
