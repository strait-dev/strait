package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func (e *Executor) handleSuccess(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleSuccess")
	defer span.End()

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
	}
	if len(result) > 0 {
		fields["result"] = result
	}
	if execTrace != nil {
		fields["execution_trace"] = execTrace
	}

	run.Status = domain.StatusCompleted
	err := e.completeRunWithWebhook(ctx, run, job, domain.StatusExecuting, domain.StatusCompleted, fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run completed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusCompleted)
	if err := e.store.RecordEndpointCircuitSuccess(ctx, job.EndpointURL); err != nil {
		e.logger.Warn("failed to record circuit breaker success", "endpoint", job.EndpointURL, "error", err)
	}

	e.logger.Info(
		"run completed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
	)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "completed"})
	e.notifyWorkflowCallback(ctx, run)

	// Latency anomaly detection: compare duration to job's P95.
	if run.StartedAt != nil {
		duration := now.Sub(*run.StartedAt)
		stats, statsErr := e.store.GetJobHealthStats(ctx, job.ID, time.Now().Add(-24*time.Hour))
		if statsErr == nil && stats != nil && stats.P95DurationSecs > 0 {
			p95 := time.Duration(stats.P95DurationSecs * float64(time.Second))
			if duration > 2*p95 {
				e.logger.Warn("latency anomaly detected",
					"run_id", run.ID, "job_id", run.JobID,
					"duration_ms", duration.Milliseconds(), "p95_ms", p95.Milliseconds())
				if e.metrics != nil {
					e.metrics.LatencyAnomalies.Add(ctx, 1,
						metric.WithAttributes(attribute.String("job_id", run.JobID)))
				}
			}
		}
	}
}

func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}

	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		switch {
		case endpointErr.StatusCode == http.StatusTooManyRequests:
			return "rate_limited"
		case endpointErr.StatusCode == http.StatusUnauthorized || endpointErr.StatusCode == http.StatusForbidden:
			return "auth"
		case endpointErr.StatusCode >= http.StatusBadRequest && endpointErr.StatusCode < http.StatusInternalServerError:
			return "client"
		case endpointErr.StatusCode >= http.StatusInternalServerError:
			return "server"
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return "transient"
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "transient"
	}

	return "unknown"
}

func shouldRetryForClass(errClass string) bool {
	switch errClass {
	case "client", "auth":
		return false
	default:
		return true
	}
}

func shouldUseFallbackForClass(errClass string) bool {
	switch errClass {
	case "transient", "rate_limited":
		return true
	default:
		return false
	}
}

func (e *Executor) handleFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, err error, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleFailure")
	defer span.End()

	errMsg := err.Error()
	errClass := classifyError(err)
	if recordErr := e.store.RecordEndpointCircuitFailure(ctx, job.EndpointURL, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); recordErr != nil {
		e.logger.Warn("failed to record circuit breaker failure", "endpoint", job.EndpointURL, "error", recordErr)
	}

	e.logger.Warn(
		"run failed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"max_attempts", policy.maxAttempts,
		"error", errMsg,
		"error_class", errClass,
	)

	shouldRetry := run.Attempt < policy.maxAttempts
	if shouldRetry && !shouldRetryForClass(errClass) {
		shouldRetry = false
	}

	const poisonPillThreshold = 3
	if shouldRetry && run.Attempt >= poisonPillThreshold {
		prevClass, err := e.store.GetRunErrorClass(ctx, run.ID)
		if err == nil && prevClass == errClass {
			shouldRetry = false
			e.logger.Warn("poison pill detected: consecutive same-class errors",
				"run_id", run.ID, "error_class", errClass, "attempt", run.Attempt)
		}
	}

	if shouldRetry {
		retryAt := NextRetryAtWithPolicy(run.Attempt, policy.retryBackoff, policy.retryInitialSecs, policy.retryMaxSecs)
		fields := map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"error":         errMsg,
			"error_class":   errClass,
			"started_at":    nil,
			"finished_at":   nil,
		}
		if job.RetryPriorityBoost > 0 {
			fields["priority"] = min(run.Priority+job.RetryPriorityBoost, 10)
		}
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields)
		if err != nil {
			e.logger.Error(
				"failed to re-enqueue run",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
		} else {
			e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusQueued)
			e.logger.Info(
				"run re-enqueued for retry",
				"run_id", run.ID,
				"job_id", run.JobID,
				"attempt", run.Attempt+1,
				"next_retry_at", retryAt,
			)
			e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "queued", "attempt": run.Attempt + 1})
		}
		return
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
		"error":       errMsg,
		"error_class": errClass,
	}
	if execTrace != nil {
		fields["execution_trace"] = execTrace
	}
	targetStatus := domain.StatusDeadLetter
	run.Status = targetStatus

	updateErr := e.completeRunWithWebhook(ctx, run, job, domain.StatusExecuting, targetStatus, fields)
	if updateErr != nil {
		e.logger.Error(
			"failed to mark run terminal",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", updateErr,
		)
		return
	}
	e.recordRunTransition(ctx, domain.StatusExecuting, targetStatus)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": string(targetStatus), "error": errMsg})
	e.notifyWorkflowCallback(ctx, run)
}

func (e *Executor) handleTimeout(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleTimeout")
	defer span.End()

	if err := e.store.RecordEndpointCircuitFailure(ctx, job.EndpointURL, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); err != nil {
		e.logger.Warn("failed to record circuit breaker timeout", "endpoint", job.EndpointURL, "error", err)
	}

	e.logger.Warn(
		"run timed out",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"timeout_secs", policy.timeoutSecs,
	)

	if run.Attempt < policy.maxAttempts {
		retryAt := NextRetryAtWithPolicy(run.Attempt, policy.retryBackoff, policy.retryInitialSecs, policy.retryMaxSecs)
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"error":         "execution timed out",
			"error_class":   "transient",
			"started_at":    nil,
			"finished_at":   nil,
		})
		if err != nil {
			e.logger.Error(
				"failed to re-enqueue timed out run",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
		} else {
			e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusQueued)
			e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "queued", "attempt": run.Attempt + 1})
		}
		return
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
		"error":       "execution timed out",
		"error_class": "transient",
	}
	if execTrace != nil {
		fields["execution_trace"] = execTrace
	}
	run.Status = domain.StatusTimedOut
	err := e.completeRunWithWebhook(ctx, run, job, domain.StatusExecuting, domain.StatusTimedOut, fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run timed_out",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusTimedOut)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "timed_out"})
	e.notifyWorkflowCallback(ctx, run)
}

// completeRunWithWebhook atomically updates run status and enqueues a webhook
// delivery within a single database transaction. If the job has no webhook URL
// or no txPool is configured, it falls back to a plain status update.
func (e *Executor) completeRunWithWebhook(ctx context.Context, run *domain.JobRun, job *domain.Job, from, to domain.RunStatus, fields map[string]any) error {
	if e.txPool != nil && job.WebhookURL != "" {
		return store.WithTx(ctx, e.txPool, func(q *store.Queries) error {
			if err := q.UpdateRunStatus(ctx, run.ID, from, to, fields); err != nil {
				return err
			}
			_, err := q.EnqueueRunWebhook(ctx, job, run, e.webhookMaxRetry)
			return err
		})
	}
	return e.store.UpdateRunStatus(ctx, run.ID, from, to, fields)
}

func (e *Executor) handleSystemFailure(ctx context.Context, run *domain.JobRun, reason string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleSystemFailure")
	defer span.End()

	now := time.Now()
	err := e.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusSystemFailed, map[string]any{
		"finished_at": now,
		"error":       reason,
		"error_class": "server",
	})
	if err != nil {
		e.logger.Error(
			"failed to mark system failure",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.recordRunTransition(ctx, run.Status, domain.StatusSystemFailed)
	e.publishEvent(ctx, run, map[string]any{"from": string(run.Status), "to": "system_failed", "error": reason})
	run.Status = domain.StatusSystemFailed
	e.notifyWorkflowCallback(ctx, run)
	// No webhook for system failures — job may not be available
}

func (e *Executor) recordRunTransition(ctx context.Context, fromStatus, toStatus domain.RunStatus) {
	if e.metrics == nil {
		return
	}

	e.metrics.RunTransitions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("from", string(fromStatus)),
		attribute.String("to", string(toStatus)),
	))
}

func durationMillisecondsAtLeastOne(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms == 0 {
		return 1
	}
	return ms
}
