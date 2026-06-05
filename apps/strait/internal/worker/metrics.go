package worker

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type workerDispatchMode string
type workerDispatchOutcome string
type workerRetryReason string
type dispatchMode string
type dispatchOutcome string
type snoozeSkippedReason string
type responseStatusClass string

const (
	workerDispatchModeGRPC       workerDispatchMode    = "grpc"
	workerDispatchOutcomeSuccess workerDispatchOutcome = "success"
	workerDispatchOutcomeError   workerDispatchOutcome = "error"
	workerDispatchOutcomeTimeout workerDispatchOutcome = "timeout"

	workerRetryReasonDispatcherUnconfigured workerRetryReason = "dispatcher_unconfigured"
	workerRetryReasonTimeout                workerRetryReason = "timeout"
	workerRetryReasonCancelled              workerRetryReason = "cancelled"
	workerRetryReasonNoWorker               workerRetryReason = "no_worker"
	workerRetryReasonDispatchError          workerRetryReason = "dispatch_error"
	workerRetryReasonWorkerFailure          workerRetryReason = "worker_failure"

	dispatchModeHTTP       dispatchMode    = "http"
	dispatchOutcomeSuccess dispatchOutcome = "success"
	dispatchOutcomeError   dispatchOutcome = "error"

	snoozeSkippedReasonLocked   snoozeSkippedReason = "locked"
	snoozeSkippedReasonConflict snoozeSkippedReason = "conflict"

	responseStatusClassUnknown responseStatusClass = "unknown"
)

// recordSnoozeSkipped increments the counter that tracks snooze no-ops
// caused by row-lock contention (reason="locked") or by the run having
// already moved past the expected from-status (reason="conflict"). The
// from label is the status the snooze was attempting to transition out of.
func recordSnoozeSkipped(ctx context.Context, from string, reason snoozeSkippedReason) {
	workerMetrics.Load().snoozeSkipped.Add(ctx, 1, metric.WithAttributes(
		attribute.String("from", from),
		attribute.String("reason", string(reason)),
	))
}

func recordWorkerDispatch(ctx context.Context, mode workerDispatchMode, outcome workerDispatchOutcome, started time.Time) {
	if started.IsZero() {
		return
	}
	workerMetrics.Load().dispatchDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(
		attribute.String("mode", string(mode)),
		attribute.String("outcome", string(outcome)),
	))
}

func recordWorkerRetry(ctx context.Context, reason workerRetryReason) {
	workerMetrics.Load().retries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", string(workerDispatchModeGRPC)),
		attribute.String("reason", string(reason)),
	))
}

func recordWorkerPool(ctx context.Context, mode dispatchMode, active, idle int64) {
	attrs := metric.WithAttributes(attribute.String("mode", string(mode)))
	m := workerMetrics.Load()
	m.poolActive.Record(ctx, active, attrs)
	m.poolIdle.Record(ctx, idle, attrs)
}

func recordHeartbeatLag(ctx context.Context, lag time.Duration) {
	if lag <= 0 {
		return
	}
	workerMetrics.Load().heartbeatLag.Record(ctx, lag.Seconds())
}

func recordDispatchAttempt(ctx context.Context, mode dispatchMode, outcome dispatchOutcome) {
	workerMetrics.Load().dispatchAttempts.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", string(mode)),
		attribute.String("outcome", string(outcome)),
	))
}

func recordDispatchPayloadBytes(ctx context.Context, mode dispatchMode, size int) {
	workerMetrics.Load().payloadBytes.Record(ctx, int64(size), metric.WithAttributes(attribute.String("mode", string(mode))))
}

func recordDispatchResponseStatus(ctx context.Context, mode dispatchMode, statusCode int) {
	workerMetrics.Load().responseStatus.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", string(mode)),
		attribute.String("status_class", string(statusClass(statusCode))),
	))
}

func statusClass(statusCode int) responseStatusClass {
	if statusCode < 100 || statusCode > 599 {
		return responseStatusClassUnknown
	}
	return responseStatusClass(string(rune('0'+statusCode/100)) + "xx")
}
