package worker

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
)

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
	// No webhook for system failures - job may not be available.
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
