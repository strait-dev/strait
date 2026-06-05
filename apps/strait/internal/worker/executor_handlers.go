package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
)

const executionTimedOutError = "execution timed out"

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

	transition := e.newSuccessfulRunTransition(run, result, execTrace, time.Now())
	run.FinishedAt = &transition.finished
	run.Status = transition.to
	err := e.completeRunWithWebhook(ctx, run, job, transition.to, transition.fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run completed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return false
	}
	e.recordSuccessfulDispatchSignals(ctx, job, transition)

	e.logger.Info(
		"run completed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
	)
	e.emit(ctx, newCompletedRunEvent(run, job, execTrace, transition))
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_complete workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTrigger(ctx, run, job, result)
	}

	e.recordSuccessfulLatencyAnomaly(ctx, run, job, transition, stats)
	return true
}

func (e *Executor) handleFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, err error, execTrace *domain.ExecutionTrace) bool {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleFailure")
	defer span.End()

	errMsg := err.Error()
	errClass := classifyError(err)
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run failed", run, job, map[string]any{
		"error_class":  errClass,
		"max_attempts": policy.maxAttempts,
	})
	e.recordFailedDispatchSignals(ctx, job, failedDispatchSignalFailure)

	e.logger.Warn(
		"run failed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"max_attempts", policy.maxAttempts,
		"error", errMsg,
		"error_class", errClass,
	)

	now := time.Now()
	transition := newFailureRunTransition(run, job, policy, err, errMsg, errClass, now)
	if transition.poisonPill != nil {
		e.logger.Warn("poison pill detected: consecutive same-error failures",
			"run_id", run.ID, "error_hash", transition.poisonPill.hash, "count", transition.poisonPill.count,
			"threshold", transition.poisonPill.threshold)
	}

	if transition.retry {
		addWorkerRunBreadcrumb(ctx, "worker.retry", "run retry scheduled", run, job, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": transition.retryAt.Format(time.RFC3339),
			"error_class":   errClass,
		})
		return e.requeueRunForRetry(ctx, run, job, transition.retryAt, transition.fields, execTrace, retryRequeueLogMessages{
			scheduleFailure: "failed to schedule retry",
			updateFailure:   "failed to re-enqueue run",
			success:         "run re-enqueued for retry",
		})
	}

	errMsg = transition.errMsg
	errClass = transition.errClass
	fields := transition.fields
	run.FinishedAt = &now
	targetStatus := domain.StatusDeadLetter
	e.addExecutionTraceField(fields, targetStatus, execTrace)
	run.Status = targetStatus

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, map[string]any{"error_class": errClass})
		scope.SetLevel(sentry.LevelWarning)
		scope.SetContext("failure", map[string]any{
			"error_message": errMsg,
			"error_class":   errClass,
			"max_attempts":  policy.maxAttempts,
			"final_status":  string(targetStatus),
		})
		scope.SetFingerprint([]string{"run_dead_lettered", run.JobID})
		sentry.CaptureMessage(fmt.Sprintf("run dead-lettered: %s", errMsg))
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
	e.emit(ctx, newTerminalRunEvent(EventDeadLettered, run, job, targetStatus, execTrace))
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
	e.recordFailedDispatchSignals(ctx, job, failedDispatchSignalTimeout)

	e.logger.Warn(
		"run timed out",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"timeout_secs", policy.timeoutSecs,
	)

	now := time.Now()
	transition := newTimeoutRunTransition(run, job, policy, now)
	if transition.retry {
		e.requeueRunForRetry(ctx, run, job, transition.retryAt, transition.fields, execTrace, retryRequeueLogMessages{
			scheduleFailure: "failed to schedule timeout retry",
			updateFailure:   "failed to re-enqueue timed out run",
		})
		return
	}

	fields := transition.fields
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
	e.emit(ctx, newTerminalRunEvent(EventTimedOut, run, job, domain.StatusTimedOut, execTrace))
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_failure job/workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, executionTimedOutError)
	}
}

func (e *Executor) handleSystemFailure(ctx context.Context, run *domain.JobRun, reason string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleSystemFailure")
	defer span.End()
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run system failure", run, nil, map[string]any{
		"error_class": domain.ErrorClassServer,
	})

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, map[string]any{"error_class": domain.ErrorClassServer})
		scope.SetLevel(sentry.LevelError)
		scope.SetFingerprint([]string{"system_failure", reason})
		sentry.CaptureMessage(fmt.Sprintf("system failure: %s", reason))
	})

	transition := newSystemFailureTransition(run, reason, time.Now())
	err := e.store.UpdateRunStatus(ctx, run.ID, transition.from, transition.to, transition.fields)
	run.FinishedAt = &transition.finished
	if err != nil {
		e.logger.Error(
			"failed to mark system failure",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	run.Status = transition.to
	e.emit(ctx, newSystemFailedRunEvent(run, transition))
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
