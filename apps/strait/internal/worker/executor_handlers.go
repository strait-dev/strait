package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type executionTraceMode string

const (
	executionTraceOff    executionTraceMode = "off"
	executionTraceErrors executionTraceMode = "errors"
	executionTraceFull   executionTraceMode = "full"
)

func normalizeExecutionTraceMode(mode string) executionTraceMode {
	switch executionTraceMode(strings.ToLower(strings.TrimSpace(mode))) {
	case executionTraceErrors:
		return executionTraceErrors
	case executionTraceFull:
		return executionTraceFull
	default:
		return executionTraceOff
	}
}

func (e *Executor) shouldPersistExecutionTrace(status domain.RunStatus, execTrace *domain.ExecutionTrace) bool {
	if execTrace == nil {
		return false
	}
	switch e.executionTraceMode {
	case executionTraceFull:
		return true
	case executionTraceErrors:
		return status != domain.StatusCompleted
	default:
		return false
	}
}

func (e *Executor) addExecutionTraceField(fields map[string]any, status domain.RunStatus, execTrace *domain.ExecutionTrace) {
	if e.shouldPersistExecutionTrace(status, execTrace) {
		fields["execution_trace"] = execTrace
	}
}

// recordRetryAttempt samples the attempt number each time a run is
// re-enqueued for retry. No-op if queue metrics were never initialised.
func recordRetryAttempt(ctx context.Context, attempt int) {
	qm, err := queue.Metrics()
	if err != nil || qm == nil || qm.RetryAttempts == nil {
		return
	}
	qm.RetryAttempts.Record(ctx, float64(attempt))
}

func (e *Executor) handleSuccess(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage) bool {
	return e.handleSuccessWithStats(ctx, run, job, result, nil, nil)
}

func (e *Executor) handleSuccessWithStats(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	result json.RawMessage,
	execTrace *domain.ExecutionTrace,
	stats *store.JobHealthStats,
) bool {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleSuccess")
	defer span.End()
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run completed", run, job, map[string]any{
		"to_status": string(domain.StatusCompleted),
	})

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
	}
	run.FinishedAt = &now
	if len(result) > 0 {
		fields["result"] = result
	}
	e.addExecutionTraceField(fields, domain.StatusCompleted, execTrace)

	run.Status = domain.StatusCompleted
	err := e.completeRunWithWebhook(ctx, run, job, domain.StatusCompleted, fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run completed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return false
	}
	if e.txPool == nil && job.EndpointURL != "" {
		if err := e.store.RecordEndpointCircuitSuccess(ctx, endpointStateKey(job.ProjectID, job.EndpointURL)); err != nil {
			e.logger.Warn("failed to record circuit breaker success", "endpoint", httputil.RedactURLForLog(job.EndpointURL), "error", err)
		}
	}

	var execDur time.Duration
	if run.StartedAt != nil {
		execDur = now.Sub(*run.StartedAt)
	}

	// Record health score for successful dispatch.
	if _, hsErr := e.healthScorer.RecordResult(ctx, DispatchResult{
		EndpointURL:  endpointStateKey(job.ProjectID, job.EndpointURL),
		Success:      true,
		LatencyMs:    float64(execDur.Milliseconds()),
		JobTimeoutMs: float64(job.TimeoutSecs * 1000),
	}); hsErr != nil {
		e.logger.Warn("failed to record health score success", "endpoint", httputil.RedactURLForLog(job.EndpointURL), "error", hsErr)
	}

	e.logger.Info(
		"run completed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
	)
	e.emit(ctx, RunLifecycleEvent{
		Type: EventCompleted, Run: run, Job: job,
		FromStatus: domain.StatusExecuting, ToStatus: domain.StatusCompleted,
		ExecTrace: execTrace, ExecDur: execDur, Attempt: run.Attempt,
		QueueWait: queueWait(run),
	})
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_complete workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTrigger(ctx, run, job, result)
	}

	// Latency anomaly detection: compare duration to job's P95.
	if run.StartedAt != nil {
		duration := now.Sub(*run.StartedAt)
		if stats == nil {
			var statsErr error
			stats, statsErr = e.getJobHealthStats(ctx, job.ID, time.Now())
			if statsErr != nil {
				stats = nil
			}
		}
		if stats != nil && stats.P95DurationSecs > 0 {
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
	return true
}

func classifyError(err error) string {
	if err == nil {
		return domain.ErrorClassUnknown
	}

	// Budget errors take highest priority.
	if isBudgetError(err) {
		return domain.ErrorClassBudget
	}

	// Deadline / timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.ErrorClassTimeout
	}

	// OOM signals.
	if isOOMError(err) {
		return domain.ErrorClassOOM
	}

	// Endpoint HTTP status classification.
	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		switch {
		case endpointErr.StatusCode == http.StatusTooManyRequests:
			return domain.ErrorClassRateLimited
		case endpointErr.StatusCode == http.StatusUnauthorized || endpointErr.StatusCode == http.StatusForbidden:
			return domain.ErrorClassAuth
		case endpointErr.StatusCode >= http.StatusBadRequest && endpointErr.StatusCode < http.StatusInternalServerError:
			return domain.ErrorClassClient
		case endpointErr.StatusCode >= http.StatusInternalServerError:
			return domain.ErrorClassServer
		}
	}

	// Connection errors.
	if isConnectionError(err) {
		return domain.ErrorClassConnection
	}

	// Generic network errors.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return domain.ErrorClassTransient
	}

	// Context canceled (not deadline) is transient.
	if errors.Is(err, context.Canceled) {
		return domain.ErrorClassTransient
	}

	return domain.ErrorClassUnknown
}

func isOOMError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "out of memory") ||
		strings.Contains(msg, "OOM") ||
		strings.Contains(msg, "memory limit exceeded") ||
		strings.Contains(msg, "ENOMEM")
}

func isConnectionError(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

func isBudgetError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "budget exceeded") ||
		strings.Contains(msg, "cost limit")
}

// errorHash returns a 16-char hex digest of the first 200 runes of an error
// message. Used for poison pill detection to identify identical errors across
// retry attempts without storing the full error string in metadata. Truncates
// by rune so multi-byte UTF-8 sequences are never split mid-character.
func errorHash(errMsg string) string {
	runes := []rune(errMsg)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	h := sha256.Sum256([]byte(string(runes)))
	return hex.EncodeToString(h[:8])
}

func errorHashForError(err error) string {
	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		return errorHash(fmt.Sprintf("endpoint returned %d: %s", endpointErr.StatusCode, endpointErr.Body))
	}
	return errorHash(err.Error())
}

// boostPriority adds boost to current priority, capping at 10 and
// guarding against integer overflow.
func boostPriority(current, boost int) int {
	boosted := current + boost
	if boosted < current { // integer overflow
		return 10
	}
	return min(boosted, 10)
}

func shouldRetryForClass(errClass string) bool {
	switch errClass {
	case domain.ErrorClassClient, domain.ErrorClassAuth, domain.ErrorClassBudget, domain.ErrorClassOOM:
		return false
	default:
		return true
	}
}

func shouldUseFallbackForClass(errClass string) bool {
	switch errClass {
	case domain.ErrorClassTransient, domain.ErrorClassRateLimited, domain.ErrorClassConnection, domain.ErrorClassTimeout:
		return true
	default:
		return false
	}
}

func (e *Executor) handleFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, err error, execTrace *domain.ExecutionTrace) bool {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleFailure")
	defer span.End()

	errMsg := err.Error()
	errClass := classifyError(err)
	stateKey := endpointStateKey(job.ProjectID, job.EndpointURL)
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run failed", run, job, map[string]any{
		"error_class":  errClass,
		"max_attempts": policy.maxAttempts,
	})
	if recordErr := e.store.RecordEndpointCircuitFailure(ctx, stateKey, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); recordErr != nil {
		e.logger.Warn("failed to record circuit breaker failure", "endpoint", httputil.RedactURLForLog(job.EndpointURL), "error", recordErr)
	}

	// Record health score for failed dispatch.
	if _, hsErr := e.healthScorer.RecordResult(ctx, DispatchResult{
		EndpointURL:  stateKey,
		Success:      false,
		LatencyMs:    0,
		JobTimeoutMs: float64(job.TimeoutSecs * 1000),
	}); hsErr != nil {
		e.logger.Warn("failed to record health score failure", "endpoint", httputil.RedactURLForLog(job.EndpointURL), "error", hsErr)
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

	// Poison pill detection: count consecutive same-error-hash failures.
	// When a run keeps hitting the same error, fast-track to DLQ instead of
	// wasting retries and risking circuit breaker trips.
	var metadataModified bool
	if shouldRetry && job.PoisonPillThreshold != nil && *job.PoisonPillThreshold > 0 {
		hash := errorHashForError(err)
		prevHash := run.Metadata["_error_hash"]
		count := 1
		if prevHash == hash {
			if raw, ok := run.Metadata["_error_hash_count"]; ok {
				if n, parseErr := strconv.Atoi(raw); parseErr == nil {
					count = n + 1
				}
			}
		}
		if run.Metadata == nil {
			run.Metadata = make(map[string]string)
		}
		run.Metadata["_error_hash"] = hash
		run.Metadata["_error_hash_count"] = strconv.Itoa(count)
		metadataModified = true

		if count >= *job.PoisonPillThreshold {
			shouldRetry = false
			errMsg = fmt.Sprintf("poison pill detected (same error %d times): %s", count, errMsg)
			e.logger.Warn("poison pill detected: consecutive same-error failures",
				"run_id", run.ID, "error_hash", hash, "count", count,
				"threshold", *job.PoisonPillThreshold)
		}
	}

	if shouldRetry {
		retryAt := NextRetryAtWithPolicy(run.Attempt, policy.retryBackoff, policy.retryInitialSecs, policy.retryMaxSecs)
		addWorkerRunBreadcrumb(ctx, "worker.retry", "run retry scheduled", run, job, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt.Format(time.RFC3339),
			"error_class":   errClass,
		})
		fields := map[string]any{
			"attempt":     run.Attempt + 1,
			"error":       errMsg,
			"error_class": errClass,
			"started_at":  nil,
			"finished_at": nil,
		}
		if metadataModified {
			fields["metadata"] = run.Metadata
		}
		if job.RetryPriorityBoost > 0 {
			fields["priority"] = boostPriority(run.Priority, job.RetryPriorityBoost)
		}
		// Side-table schedule write keeps the indexed job_runs.next_retry_at
		// column untouched so the requeue UPDATE stays HOT-eligible.
		if scheduleErr := e.store.ScheduleRetry(ctx, run.ID, retryAt, run.Attempt+1); scheduleErr != nil {
			e.logger.Error("failed to schedule retry",
				"run_id", run.ID, "job_id", run.JobID, "error", scheduleErr)
			return false
		}
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields)
		if err != nil {
			e.logger.Error(
				"failed to re-enqueue run",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
			return false
		}
		e.logger.Info(
			"run re-enqueued for retry",
			"run_id", run.ID,
			"job_id", run.JobID,
			"attempt", run.Attempt+1,
			"next_retry_at", retryAt,
		)
		recordRetryAttempt(ctx, run.Attempt+1)
		e.emit(ctx, RunLifecycleEvent{
			Type: EventRetried, Run: run, Job: job,
			FromStatus: domain.StatusExecuting, ToStatus: domain.StatusQueued,
			ExecTrace: execTrace, Attempt: run.Attempt + 1,
			QueueWait: queueWait(run),
		})
		return true
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
		"error":       errMsg,
		"error_class": errClass,
	}
	run.FinishedAt = &now
	if metadataModified {
		fields["metadata"] = run.Metadata
	}
	targetStatus := domain.StatusDeadLetter
	eventType := EventDeadLettered
	sentryErrorClass := errClass
	if e.dlqCapEnforcer != nil {
		proceed, enforceErr := e.dlqCapEnforcer.EnforceBeforeTransition(ctx, job.ProjectID, job.ID)
		switch {
		case enforceErr == nil && proceed:
		case errors.Is(enforceErr, ErrDLQOverflow):
			targetStatus = domain.StatusSystemFailed
			eventType = EventSystemFailed
			sentryErrorClass = "dlq_overflow"
			fields["error"] = fmt.Sprintf("dlq overflow: cap reached before dead-lettering run: %s", errMsg)
			fields["error_class"] = "dlq_overflow"
		default:
			e.logger.Warn("dlq cap check failed; allowing dead-letter transition",
				"run_id", run.ID, "job_id", run.JobID, "project_id", run.ProjectID, "error", enforceErr)
		}
	}
	e.addExecutionTraceField(fields, targetStatus, execTrace)
	run.Status = targetStatus

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, map[string]any{"error_class": sentryErrorClass})
		scope.SetLevel(sentry.LevelWarning)
		scope.SetContext("failure", map[string]any{
			"error_message": errMsg,
			"error_class":   errClass,
			"max_attempts":  policy.maxAttempts,
			"final_status":  string(targetStatus),
		})
		if targetStatus == domain.StatusDeadLetter {
			scope.SetFingerprint([]string{"run_dead_lettered", run.JobID})
			sentry.CaptureMessage(fmt.Sprintf("run dead-lettered: %s", errMsg))
		} else {
			scope.SetFingerprint([]string{"run_dlq_overflow", run.JobID})
			sentry.CaptureMessage(fmt.Sprintf("run failed before dead-lettering: %s", errMsg))
		}
	})

	updateErr := e.completeRunWithWebhook(ctx, run, job, targetStatus, fields)
	if updateErr != nil {
		e.logger.Error(
			"failed to mark run terminal",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", updateErr,
		)
		return false
	}
	e.recordTerminalRunBilling(ctx, job, run)
	e.emit(ctx, RunLifecycleEvent{
		Type: eventType, Run: run, Job: job,
		FromStatus: domain.StatusExecuting, ToStatus: targetStatus,
		ExecTrace: execTrace, Attempt: run.Attempt,
		QueueWait: queueWait(run),
	})
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_failure job/workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, errMsg)
	}
	return true
}

func (e *Executor) handleTimeout(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleTimeout")
	defer span.End()
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run timed out", run, job, map[string]any{
		"timeout_secs": policy.timeoutSecs,
		"max_attempts": policy.maxAttempts,
	})
	stateKey := endpointStateKey(job.ProjectID, job.EndpointURL)

	if err := e.store.RecordEndpointCircuitFailure(ctx, stateKey, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); err != nil {
		e.logger.Warn("failed to record circuit breaker timeout", "endpoint", httputil.RedactURLForLog(job.EndpointURL), "error", err)
	}

	// Record health score for timeout (counts as failure with timeout flag).
	if _, hsErr := e.healthScorer.RecordResult(ctx, DispatchResult{
		EndpointURL:  stateKey,
		Success:      false,
		TimedOut:     true,
		LatencyMs:    float64(job.TimeoutSecs * 1000),
		JobTimeoutMs: float64(job.TimeoutSecs * 1000),
	}); hsErr != nil {
		e.logger.Warn("failed to record health score timeout", "endpoint", httputil.RedactURLForLog(job.EndpointURL), "error", hsErr)
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
		fields := map[string]any{
			"attempt":     run.Attempt + 1,
			"error":       "execution timed out",
			"error_class": "transient",
			"started_at":  nil,
			"finished_at": nil,
		}
		if job.RetryPriorityBoost > 0 {
			fields["priority"] = boostPriority(run.Priority, job.RetryPriorityBoost)
		}
		if scheduleErr := e.store.ScheduleRetry(ctx, run.ID, retryAt, run.Attempt+1); scheduleErr != nil {
			e.logger.Error("failed to schedule timeout retry",
				"run_id", run.ID, "job_id", run.JobID, "error", scheduleErr)
			return
		}
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields)
		if err != nil {
			e.logger.Error(
				"failed to re-enqueue timed out run",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
		} else {
			recordRetryAttempt(ctx, run.Attempt+1)
			e.emit(ctx, RunLifecycleEvent{
				Type: EventRetried, Run: run, Job: job,
				FromStatus: domain.StatusExecuting, ToStatus: domain.StatusQueued,
				ExecTrace: execTrace, Attempt: run.Attempt + 1,
				QueueWait: queueWait(run),
			})
		}
		return
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
		"error":       "execution timed out",
		"error_class": "transient",
	}
	run.FinishedAt = &now
	run.Status = domain.StatusTimedOut
	e.addExecutionTraceField(fields, domain.StatusTimedOut, execTrace)

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, nil)
		scope.SetLevel(sentry.LevelWarning)
		scope.SetContext("timeout", map[string]any{
			"timeout_secs": policy.timeoutSecs,
			"max_attempts": policy.maxAttempts,
		})
		scope.SetFingerprint([]string{"run_timed_out", run.JobID})
		sentry.CaptureMessage("run timed out after all retries")
	})

	err := e.completeRunWithWebhook(ctx, run, job, domain.StatusTimedOut, fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run timed_out",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.recordTerminalRunBilling(ctx, job, run)
	e.emit(ctx, RunLifecycleEvent{
		Type: EventTimedOut, Run: run, Job: job,
		FromStatus: domain.StatusExecuting, ToStatus: domain.StatusTimedOut,
		ExecTrace: execTrace, Attempt: run.Attempt,
		QueueWait: queueWait(run),
	})
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_failure job/workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, "execution timed out")
	}
}

// completeRunWithWebhook atomically updates run status and enqueues a webhook
// delivery within a single database transaction. If the job has no webhook URL
// or no txPool is configured, it falls back to a plain status update.
// The run must be in StatusExecuting when this is called.
func (e *Executor) completeRunWithWebhook(ctx context.Context, run *domain.JobRun, job *domain.Job, to domain.RunStatus, fields map[string]any) error {
	from := domain.StatusExecuting
	webhookRun := runForTerminalWebhook(run, to, fields)
	if e.txPool != nil {
		return store.WithTx(ctx, e.txPool, func(q *store.Queries) error {
			if err := q.UpdateRunStatus(ctx, run.ID, from, to, fields); err != nil {
				return err
			}
			if to == domain.StatusCompleted && job.EndpointURL != "" {
				if err := q.RecordEndpointCircuitSuccess(ctx, endpointStateKey(job.ProjectID, job.EndpointURL)); err != nil {
					return err
				}
			}
			if job.WebhookURL == "" {
				return nil
			}
			_, err := q.EnqueueRunWebhook(ctx, job, webhookRun, e.webhookMaxRetry)
			return err
		})
	}
	if e.txPool == nil && job.WebhookURL != "" {
		e.logger.Warn("txPool not configured, webhook delivery skipped for completed run",
			"run_id", run.ID, "job_id", job.ID, "webhook_url", httputil.RedactURLForLog(job.WebhookURL))
	}
	return e.store.UpdateRunStatus(ctx, run.ID, from, to, fields)
}

func runForTerminalWebhook(run *domain.JobRun, status domain.RunStatus, fields map[string]any) *domain.JobRun {
	webhookRun := *run
	webhookRun.Status = status
	if result, ok := fields["result"].(json.RawMessage); ok {
		webhookRun.Result = result
	}
	if errMsg, ok := fields["error"].(string); ok {
		webhookRun.Error = errMsg
	}
	if finishedAt, ok := fields["finished_at"].(time.Time); ok {
		webhookRun.FinishedAt = &finishedAt
	}
	return &webhookRun
}

func (e *Executor) handleSystemFailure(ctx context.Context, run *domain.JobRun, reason string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleSystemFailure")
	defer span.End()
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run system failure", run, nil, map[string]any{
		"error_class": "server",
	})

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, map[string]any{"error_class": "server"})
		scope.SetLevel(sentry.LevelError)
		scope.SetFingerprint([]string{"system_failure", reason})
		sentry.CaptureMessage(fmt.Sprintf("system failure: %s", reason))
	})

	fromStatus := run.Status
	now := time.Now()
	err := e.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusSystemFailed, map[string]any{
		"finished_at": now,
		"error":       reason,
		"error_class": "server",
	})
	run.FinishedAt = &now
	if err != nil {
		e.logger.Error(
			"failed to mark system failure",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	run.Status = domain.StatusSystemFailed
	e.emit(ctx, RunLifecycleEvent{
		Type: EventSystemFailed, Run: run,
		FromStatus: fromStatus, ToStatus: domain.StatusSystemFailed,
		Attempt:   run.Attempt,
		QueueWait: queueWait(run),
	})
	e.notifyWorkflowCallback(ctx, run)
	// No webhook for system failures — job may not be available
}

// handleSystemFailureWithJob wraps handleSystemFailure and additionally fires
// on_failure triggers when the job is available. Some system failure paths
// (panic recovery, job-not-found) don't have the job object, so the base
// handleSystemFailure cannot require it.
func (e *Executor) handleSystemFailureWithJob(ctx context.Context, run *domain.JobRun, job *domain.Job, reason string) {
	e.handleSystemFailure(ctx, run, reason)
	if job != nil && e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, reason)
	}
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

func addWorkerRunBreadcrumb(ctx context.Context, category, message string, run *domain.JobRun, job *domain.Job, data map[string]any) {
	if run == nil {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["run_id"] = run.ID
	data["job_id"] = run.JobID
	data["project_id"] = run.ProjectID
	data["attempt"] = run.Attempt
	data["status"] = string(run.Status)
	data["execution_mode"] = string(run.ExecutionMode)
	if job != nil {
		data["job_version"] = job.Version
		data["environment_id"] = job.EnvironmentID
	}
	telemetry.AddSentryBreadcrumb(ctx, category, message, data)
}

func (e *Executor) applyWorkerSentryScope(scope *sentry.Scope, run *domain.JobRun, data map[string]any) {
	telemetry.ApplySentryRuntimeScope(scope, telemetry.SentryRuntime{
		Edition:   string(domain.BuildEdition()),
		Subsystem: telemetry.SubsystemWorker,
		Mode:      e.mode,
		Region:    e.defaultRegion,
		Version:   e.version,
	})
	if run != nil {
		telemetry.SetSentryTag(scope, telemetry.TagRunID, run.ID)
		telemetry.SetSentryTag(scope, telemetry.TagJobID, run.JobID)
		telemetry.SetSentryTag(scope, telemetry.TagProjectID, run.ProjectID)
		telemetry.SetSentryTag(scope, telemetry.TagAttempt, fmt.Sprintf("%d", run.Attempt))
		if run.CreatedBy != "" {
			actorType := workerActorType(run)
			telemetry.SetSentryTag(scope, telemetry.TagActorID, run.CreatedBy)
			telemetry.SetSentryTag(scope, telemetry.TagActorType, actorType)
			scope.SetUser(sentry.User{
				ID: run.CreatedBy,
				Data: map[string]string{
					"actor_type": actorType,
					"project_id": run.ProjectID,
				},
			})
		}
		requestContext := sentry.Context{
			"created_by":   run.CreatedBy,
			"triggered_by": run.TriggeredBy,
		}
		if requestID := run.Metadata[domain.RunMetadataSentryRequestID]; requestID != "" {
			telemetry.SetSentryTag(scope, telemetry.TagRequestID, requestID)
			requestContext["request_id"] = requestID
		}
		route := run.Metadata[domain.RunMetadataSentryRoute]
		if route == "" {
			route = "worker.dispatch"
		}
		telemetry.SetSentryTag(scope, telemetry.TagRoute, route)
		requestContext["route"] = route
		if actorType := run.Metadata[domain.RunMetadataSentryActorType]; actorType != "" {
			requestContext["actor_type"] = actorType
		}
		scope.SetContext("dispatch.request", requestContext)
		scope.SetContext("run", sentry.Context{
			"run_id":         run.ID,
			"job_id":         run.JobID,
			"project_id":     run.ProjectID,
			"attempt":        run.Attempt,
			"priority":       run.Priority,
			"execution_mode": string(run.ExecutionMode),
			"status":         string(run.Status),
		})
	}
	for key, val := range data {
		if tag, ok := telemetry.SentryTagFromString(key); ok {
			telemetry.SetSentryTag(scope, tag, fmt.Sprintf("%v", val))
		}
	}
}

func workerActorType(run *domain.JobRun) string {
	if run == nil {
		return ""
	}
	if actorType := run.Metadata[domain.RunMetadataSentryActorType]; actorType != "" {
		return actorType
	}
	switch {
	case strings.HasPrefix(run.CreatedBy, "apikey:"):
		return "api_key"
	case strings.HasPrefix(run.CreatedBy, "run:"):
		return "run_token"
	case strings.HasPrefix(run.CreatedBy, "sse:"):
		return "sse_token"
	case run.CreatedBy != "":
		return "user"
	default:
		return ""
	}
}

// queueWait returns the duration a run spent queued (created_at to started_at).
func queueWait(run *domain.JobRun) time.Duration {
	if run == nil || run.CreatedAt.IsZero() {
		return 0
	}
	if run.StartedAt == nil {
		return 0
	}
	return run.StartedAt.Sub(run.CreatedAt)
}
