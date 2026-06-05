package worker

import (
	"context"
	"errors"
	"time"

	workergrpc "strait/internal/api/grpc"
	"strait/internal/domain"
)

// executeWorkerMode dispatches a run to a connected gRPC worker. It mirrors
// the HTTP dispatch flow for billing, status transitions, and retry logic.
//
// If no worker is currently available for the run's queue, the run is left
// in its current state so it can be re-claimed on the next poll tick.
//
// On a successful result, cost is recorded via RecordWorkerRunCost.
func (e *Executor) executeWorkerMode(ctx context.Context, run *domain.JobRun, job *domain.Job, policies ...executionPolicy) {
	dispatchStarted := time.Now()
	dispatchOutcome := workerDispatchOutcomeSuccess
	defer func() {
		recordWorkerDispatch(context.Background(), workerDispatchModeGRPC, dispatchOutcome, dispatchStarted)
	}()

	policy := defaultExecutionPolicy(job)
	if len(policies) > 0 {
		policy = policies[0]
	}

	if e.workerDispatcher == nil {
		e.logger.Warn("worker dispatcher not configured; leaving run queued",
			"run_id", run.ID,
			"job_id", run.JobID,
		)
		dispatchOutcome = workerDispatchOutcomeError
		recordWorkerRetry(ctx, workerRetryReasonDispatcherUnconfigured)
		e.requeueWorkerModeRun(ctx, run, workerRequeueReasonDispatcherUnconfigured)
		return
	}

	if !e.transitionRunToExecuting(ctx, run) {
		dispatchOutcome = workerDispatchOutcomeError
		return
	}
	e.heartbeat.Register(run.ID)
	defer e.heartbeat.Deregister(run.ID)

	timeout := time.Duration(policy.timeoutSecs) * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := e.workerDispatcher.WorkerDispatch(execCtx, run, job)
	if err != nil {
		dispatchOutcome = e.handleWorkerDispatchError(ctx, run, job, policy, err)
		return
	}

	dispatchOutcome = e.handleWorkerDispatchResult(ctx, run, job, policy, result)
}

func (e *Executor) handleWorkerDispatchError(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	policy executionPolicy,
	err error,
) workerDispatchOutcome {
	if errors.Is(err, context.DeadlineExceeded) {
		// Worker-mode dispatch uses the same execution timeout policy as HTTP mode.
		recordWorkerRetry(ctx, workerRetryReasonTimeout)
		e.handleTimeout(ctx, run, job, policy, nil)
		return workerDispatchOutcomeTimeout
	}
	if errors.Is(err, context.Canceled) {
		recordWorkerRetry(ctx, workerRetryReasonCancelled)
		e.requeueWorkerModeRun(ctx, run, workerRequeueReasonDispatchCancelled)
		return workerDispatchOutcomeError
	}

	// ErrNoWorkerAvailable leaves the run queued for the next poll tick; any
	// other error is a dispatch failure that follows normal retry policy.
	e.logger.Warn("worker dispatch failed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"error", err,
	)
	if errors.Is(err, workergrpc.ErrNoWorkerAvailable) {
		recordWorkerRetry(ctx, workerRetryReasonNoWorker)
		e.requeueWorkerModeRun(ctx, run, workerRequeueReasonNoWorker)
		return workerDispatchOutcomeError
	}

	recordWorkerRetry(ctx, workerRetryReasonDispatchError)
	e.handleFailure(ctx, run, job, policy, err, nil)
	return workerDispatchOutcomeError
}
