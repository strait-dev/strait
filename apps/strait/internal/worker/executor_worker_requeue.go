package worker

import (
	"context"
	"time"

	"strait/internal/domain"
)

func (e *Executor) requeueWorkerModeRun(ctx context.Context, run *domain.JobRun, reason string) {
	from := run.Status
	if from == "" {
		from = domain.StatusExecuting
	}
	requeueCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		requeueCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
	}
	if err := e.store.UpdateRunStatus(requeueCtx, run.ID, from, domain.StatusQueued, queuedRunResetFields()); err != nil {
		e.logger.Warn("executeWorkerMode: requeue failed",
			"run_id", run.ID,
			"from", from,
			"reason", reason,
			"error", err,
		)
		return
	}
	run.Status = domain.StatusQueued
	e.enqueueExistingRunIfReady(requeueCtx, run, reason)
}

func queuedRunResetFields() map[string]any {
	return map[string]any{
		"error":         nil,
		"error_class":   nil,
		"finished_at":   nil,
		"heartbeat_at":  nil,
		"next_retry_at": nil,
		"started_at":    nil,
	}
}

func (e *Executor) enqueueExistingRunIfReady(ctx context.Context, run *domain.JobRun, reason string) {
	if e == nil || e.queue == nil || run == nil {
		return
	}
	enqueuer, ok := e.queue.(existingRunEnqueuer)
	if !ok {
		return
	}
	readyRun := *run
	readyRun.Status = domain.StatusQueued
	if err := enqueuer.EnqueueExisting(ctx, &readyRun); err != nil {
		e.logger.Warn("failed to emit ready event for requeued run",
			"run_id", run.ID,
			"reason", reason,
			"error", err,
		)
	}
}
