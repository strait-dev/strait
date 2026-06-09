package worker

import (
	"context"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

// recordRetryAttempt samples the attempt number each time a run is
// re-enqueued for retry. No-op if queue metrics were never initialised.
func recordRetryAttempt(ctx context.Context, attempt int) {
	qm, err := queue.Metrics()
	if err != nil {
		return
	}
	if qm == nil || qm.RetryAttempts == nil {
		return
	}
	qm.RetryAttempts.Record(ctx, float64(attempt))
}

type retryRequeueLogMessages struct {
	scheduleFailure string
	updateFailure   string
	success         string
}

func (e *Executor) requeueRunForRetry(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	retryAt time.Time,
	fields map[string]any,
	execTrace *domain.ExecutionTrace,
	logs retryRequeueLogMessages,
) bool {
	// Side-table schedule write keeps the indexed job_runs.next_retry_at
	// column untouched so the requeue UPDATE stays HOT-eligible.
	if scheduleErr := e.store.ScheduleRetry(ctx, run.ID, retryAt, run.Attempt+1); scheduleErr != nil {
		e.logger.Error(logs.scheduleFailure,
			"run_id", run.ID, "job_id", run.JobID, "error", scheduleErr)
		return false
	}
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields)
	if err != nil {
		e.logger.Error(
			logs.updateFailure,
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return false
	}
	if logs.success != "" {
		e.logger.Info(
			logs.success,
			"run_id", run.ID,
			"job_id", run.JobID,
			"attempt", run.Attempt+1,
			"next_retry_at", retryAt,
		)
	}
	recordRetryAttempt(ctx, run.Attempt+1)
	e.emit(ctx, newRetriedRunEvent(run, job, execTrace))
	return true
}
