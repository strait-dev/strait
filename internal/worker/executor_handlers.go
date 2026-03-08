package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"orchestrator/internal/domain"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func (e *Executor) handleSuccess(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleSuccess")
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

	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, fields)
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
	if e.circuitBreaker {
		if err := e.store.RecordEndpointCircuitSuccess(ctx, job.EndpointURL); err != nil {
			e.logger.Warn("failed to record circuit breaker success", "endpoint", job.EndpointURL, "error", err)
		}
	}

	e.logger.Info(
		"run completed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
	)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "completed"})
	run.Status = domain.StatusCompleted
	e.notifyWorkflowCallback(ctx, run)
	e.submitWebhook(ctx, job, run)
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleFailure")
	defer span.End()

	errMsg := err.Error()
	errClass := classifyError(err)
	if e.circuitBreaker {
		if recordErr := e.store.RecordEndpointCircuitFailure(ctx, job.EndpointURL, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); recordErr != nil {
			e.logger.Warn("failed to record circuit breaker failure", "endpoint", job.EndpointURL, "error", recordErr)
		}
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
	if shouldRetry && e.smartRetry && !shouldRetryForClass(errClass) {
		shouldRetry = false
	}

	if shouldRetry {
		retryAt := NextRetryAtWithPolicy(run.Attempt, policy.retryBackoff, policy.retryInitialSecs, policy.retryMaxSecs)
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"error":         errMsg,
			"error_class":   errClass,
			"started_at":    nil,
			"finished_at":   nil,
		})
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
	targetStatus := domain.StatusFailed
	if e.dlqEnabled {
		targetStatus = domain.StatusDeadLetter
	}

	updateErr := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, targetStatus, fields)
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
	run.Status = targetStatus
	e.notifyWorkflowCallback(ctx, run)
	e.submitWebhook(ctx, job, run)
}

func (e *Executor) handleTimeout(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleTimeout")
	defer span.End()

	if e.circuitBreaker {
		if err := e.store.RecordEndpointCircuitFailure(ctx, job.EndpointURL, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); err != nil {
			e.logger.Warn("failed to record circuit breaker timeout", "endpoint", job.EndpointURL, "error", err)
		}
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
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusTimedOut, fields)
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
	run.Status = domain.StatusTimedOut
	e.notifyWorkflowCallback(ctx, run)
	e.submitWebhook(ctx, job, run)
}

func (e *Executor) submitWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	detached := context.WithoutCancel(ctx)
	e.pool.Submit(detached, func() {
		webhookCtx, wCancel := context.WithTimeout(detached, e.webhookDispatchTimeout)
		defer wCancel()
		SendWebhookWithClient(webhookCtx, e.webhookClient, job, run, e.webhookMaxRetry)
	})
}

func (e *Executor) handleSystemFailure(ctx context.Context, run *domain.JobRun, reason string) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleSystemFailure")
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
