package worker

import (
	"context"
	"encoding/json"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
)

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
