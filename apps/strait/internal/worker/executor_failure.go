package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
)

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
				"run_id", run.ID,
				"job_id", run.JobID,
				"project_id", run.ProjectID,
				"error", enforceErr,
			)
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
	e.emit(ctx, newTerminalRunEvent(eventType, run, job, targetStatus, execTrace))
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_failure job/workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, errMsg)
	}
	return true
}
