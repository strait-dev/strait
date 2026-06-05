package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

const (
	workerRequeueReasonDispatcherUnconfigured = "worker dispatcher not configured"
	workerRequeueReasonDispatchCancelled      = "worker dispatch cancelled"
	workerRequeueReasonNoWorker               = "no worker available"

	workerResultStatusSuccess = "success"
)

func (e *Executor) execute(ctx context.Context, run *domain.JobRun) {
	ctx = withDispatchCache(ctx)
	ec := &ExecutionContext{
		Run:   run,
		Start: time.Now(),
	}

	handler := e.executeInner
	if len(e.middlewares) > 0 {
		handler = Chain(e.middlewares...)(handler)
	}
	handler(ctx, ec)
}

func (e *Executor) executeInner(ctx context.Context, ec *ExecutionContext) {
	run := ec.Run
	executeStart := ec.Start

	job, policy, ok := e.resolveDispatchJobAndPolicy(ctx, run)
	if !ok {
		return
	}
	ec.Job = job

	releaseBilling, ok := e.enforceDispatchBilling(ctx, run, job)
	if !ok {
		return
	}
	if releaseBilling != nil {
		defer releaseBilling()
	}

	switch job.ExecutionMode {
	case domain.ExecutionModeHTTP, "":
		// HTTP dispatch continues below.
	case domain.ExecutionModeWorker:
		e.executeWorkerMode(ctx, run, job, policy)
		return
	default:
		e.logger.Error("unknown execution_mode", "run_id", run.ID, "job_id", run.JobID, "execution_mode", job.ExecutionMode)
		e.handleSystemFailureWithJob(ctx, run, job, fmt.Sprintf("unknown execution_mode: %s", job.ExecutionMode))
		return
	}

	readiness := e.prepareHTTPDispatch(ctx, run, job, policy)
	if !readiness.ok {
		return
	}
	defer readiness.releaseBulkhead()

	if !e.transitionRunToExecuting(ctx, run) {
		return
	}

	e.heartbeat.Register(run.ID)
	defer e.heartbeat.Deregister(run.ID)

	execCtx, cancel := context.WithTimeout(ctx, e.dispatchTimeout(job, policy, readiness.prefetch.adaptiveStats))
	defer cancel()

	result, execTrace, err := e.tracedDispatch(execCtx, job, run)
	populateExecutionTraceRunTimings(execTrace, run, executeStart, time.Now())
	if err != nil {
		fallbackResult, fallbackErr, fallbackOK := e.tryFallbackDispatch(execCtx, job, run, err)
		if fallbackOK {
			e.handleSuccessWithStats(ctx, run, job, fallbackResult, execTrace, readiness.prefetch.adaptiveStats)
			return
		}
		if fallbackErr != nil {
			err = fallbackErr
		}

		if execCtx.Err() == context.DeadlineExceeded {
			e.handleTimeout(ctx, run, job, policy, execTrace)
		} else {
			e.handleFailure(ctx, run, job, policy, err, execTrace)
		}
		return
	}

	e.recordHTTPRunCost(ctx, job, run)
	e.handleSuccessWithStats(ctx, run, job, result, execTrace, readiness.prefetch.adaptiveStats)
}

func (e *Executor) transitionRunToExecuting(ctx context.Context, run *domain.JobRun) bool {
	startFrom := run.Status
	if startFrom == "" {
		startFrom = domain.StatusDequeued
	}
	publishFrom := startFrom
	if run.Status != domain.StatusExecuting {
		if err := e.store.UpdateRunStatus(ctx, run.ID, startFrom, domain.StatusExecuting, map[string]any{
			"started_at": time.Now(),
		}); err != nil {
			e.logger.Error(
				"failed to transition to executing",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
			return false
		}
		run.Status = domain.StatusExecuting
	} else {
		publishFrom = domain.StatusDequeued
	}
	e.publishEvent(ctx, run, map[string]any{"from": string(publishFrom), "to": "executing"})
	return true
}

func (e *Executor) tryFallbackDispatch(
	ctx context.Context,
	job *domain.Job,
	run *domain.JobRun,
	primaryErr error,
) (json.RawMessage, error, bool) {
	if job.FallbackEndpointURL == "" || !shouldUseFallbackForClass(classifyError(primaryErr)) {
		return nil, nil, false
	}
	// Build the same auth and durable-resume headers the primary path sends so a
	// secret-dependent or SDK-based fallback endpoint can authenticate callbacks
	// and resume from the last checkpoint on failover. ctx is the per-execution
	// context, so secrets and the checkpoint are served from the dispatch cache
	// the primary attempt already warmed.
	fallbackHeaders, err := e.dispatchHeaders(ctx, job, run)
	if err != nil {
		return nil, errors.Join(primaryErr, err), false
	}
	result, err := e.dispatchToEndpoint(ctx, job.FallbackEndpointURL, run, fallbackHeaders)
	if err == nil {
		return result, nil, true
	}
	return nil, errors.Join(
		fmt.Errorf("primary dispatch failed: %w", primaryErr),
		fmt.Errorf("fallback dispatch failed: %w", err),
	), false
}

func (e *Executor) dispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.Dispatch")
	defer span.End()
	start := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.DispatchDuration.Record(ctx, time.Since(start).Seconds())
		}
	}()

	extraHeaders, err := e.dispatchHeaders(ctx, job, run)
	if err != nil {
		return err
	}

	_, dispatchErr := e.dispatchToEndpoint(ctx, job.EndpointURL, run, extraHeaders)
	return dispatchErr
}
