package worker

import (
	"context"
	"time"

	"strait/internal/domain"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
)

const executionTimedOutError = "execution timed out"

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
	e.recordTerminalRunBilling(ctx, job, run)
	e.emit(ctx, newTerminalRunEvent(EventTimedOut, run, job, domain.StatusTimedOut, execTrace))
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_failure job/workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, executionTimedOutError)
	}
}
