package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"strait/internal/domain"
)

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
