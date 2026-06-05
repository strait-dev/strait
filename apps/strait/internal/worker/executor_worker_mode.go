package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	workergrpc "strait/internal/api/grpc"
	"strait/internal/billing"
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

func (e *Executor) handleWorkerDispatchResult(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	policy executionPolicy,
	result any,
) workerDispatchOutcome {
	// Only "success" routes to the success handler; everything else (including
	// "failed", "" from a nil/malformed result, or any unexpected sentinel) is
	// routed to handleFailure so retry and DLQ policy stay in one path.
	runResult := e.workerRunResultFromDispatch(result)
	if !runResult.succeeded() {
		_, transitioned := e.applyWorkerRunResult(ctx, run, job, policy, runResult)
		if transitioned {
			e.completeWorkerTask(ctx, result, domain.WorkerTaskStatusFailed)
		}
		recordWorkerRetry(ctx, workerRetryReasonWorkerFailure)
		return workerDispatchOutcomeError
	}

	taskStatus, transitioned := e.applyWorkerRunResult(ctx, run, job, policy, runResult)
	if transitioned {
		e.completeWorkerTask(ctx, result, taskStatus)
	}
	return workerDispatchOutcomeSuccess
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

// FinalizeWorkerRunResult applies worker-mode completion semantics for a result
// received outside the normal WorkerDispatch wait path, such as a late fallback
// TaskResult or reconnect-reported in-flight task.
func (e *Executor) FinalizeWorkerRunResult(ctx context.Context, runID, status, errorMessage string, output json.RawMessage) (domain.WorkerTaskStatus, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return "", fmt.Errorf("load run for worker finalization: %w", err)
	}
	job, err := e.store.GetJob(ctx, run.JobID)
	if err != nil {
		return "", fmt.Errorf("load job for worker finalization: %w", err)
	}

	policy := defaultExecutionPolicy(job)
	runResult := workerRunResult{
		status:       status,
		errorMessage: errorMessage,
		output:       output,
	}
	taskStatus, transitioned := e.applyWorkerRunResult(ctx, run, job, policy, runResult)
	if !transitioned {
		if !runResult.succeeded() {
			return "", fmt.Errorf("worker failure finalization did not transition run")
		}
		return "", fmt.Errorf("worker success finalization did not transition run")
	}

	return taskStatus, nil
}

func (e *Executor) applyWorkerRunResult(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	policy executionPolicy,
	result workerRunResult,
) (domain.WorkerTaskStatus, bool) {
	if !result.succeeded() {
		errorMessage := result.failureMessage()
		return domain.WorkerTaskStatusFailed, e.handleFailure(ctx, run, job, policy, errors.New(errorMessage), nil)
	}

	e.recordWorkerModeCost(ctx, run, job)
	return domain.WorkerTaskStatusCompleted, e.handleSuccess(ctx, run, job, result.output)
}

type workerRunResult struct {
	status       string
	errorMessage string
	output       json.RawMessage
}

func (e *Executor) workerRunResultFromDispatch(result any) workerRunResult {
	return workerRunResult{
		status:       e.workerDispatcher.ResultStatus(result),
		errorMessage: e.workerDispatcher.ResultError(result),
		output:       e.workerDispatcher.ResultOutput(result),
	}
}

func (r workerRunResult) succeeded() bool {
	return r.status == workerResultStatusSuccess
}

func (r workerRunResult) failureMessage() string {
	if r.errorMessage != "" {
		return r.errorMessage
	}
	if r.status == "" {
		return "worker returned malformed or empty result"
	}
	return fmt.Sprintf("worker reported terminal status %q without error message", r.status)
}

func (e *Executor) recordWorkerModeCost(ctx context.Context, run *domain.JobRun, job *domain.Job) {
	e.recordRunCostRow(ctx, job.ProjectID, run.ID, "failed to record worker run cost", func(costCtx context.Context, orgID, projectID, runID string) error {
		return e.runCostRecorder.RecordWorkerRunCost(costCtx, orgID, projectID, runID)
	})
	e.ingestStripeUsageEvent(ctx, job.ProjectID, run.ID, billing.WorkerCostPerRunMicrousd)
}

func (e *Executor) completeWorkerTask(ctx context.Context, result any, status domain.WorkerTaskStatus) {
	completer, ok := e.workerDispatcher.(workerTaskCompletionDispatcher)
	if !ok {
		return
	}
	if err := completer.CompleteWorkerTask(ctx, result, status); err != nil {
		e.logger.Warn("executeWorkerMode: complete worker task failed",
			"status", status,
			"error", err,
		)
	}
}

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
