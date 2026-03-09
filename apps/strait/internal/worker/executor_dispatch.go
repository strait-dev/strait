package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

func (e *Executor) execute(ctx context.Context, run *domain.JobRun) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.Execute")
	defer span.End()

	executeStart := time.Now()

	job, err := e.store.GetJob(ctx, run.JobID)
	if err != nil || job == nil {
		e.logger.Error(
			"job lookup failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		e.handleSystemFailure(ctx, run, "job not found")
		return
	}

	policy := executionPolicy{
		maxAttempts:      job.MaxAttempts,
		timeoutSecs:      job.TimeoutSecs,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     3600,
	}
	resolved, policyErr := e.resolveExecutionPolicy(ctx, run, policy)
	if policyErr != nil {
		e.logger.Error("failed to resolve execution policy", "run_id", run.ID, "error", policyErr)
		e.handleSystemFailure(ctx, run, "resolve execution policy")
		return
	}
	policy = resolved

	// Environment endpoint override: if the job has an environment_id,
	// resolve its variables and check for ENDPOINT_URL override.
	if job.EnvironmentID != "" {
		envVars, envErr := e.store.GetResolvedEnvironmentVariables(ctx, job.EnvironmentID)
		if envErr != nil {
			e.logger.Warn("failed to resolve environment variables", "run_id", run.ID, "environment_id", job.EnvironmentID, "error", envErr)
		} else if override, ok := envVars["ENDPOINT_URL"]; ok && override != "" {
			if err := validateEndpointURL(override); err != nil {
				e.logger.Warn("environment ENDPOINT_URL failed SSRF validation",
					"run_id", run.ID,
					"environment_id", job.EnvironmentID,
					"error", err,
				)
			} else {
				e.logger.Info("overriding endpoint URL from environment",
					"run_id", run.ID,
					"environment_id", job.EnvironmentID,
				)
				job.EndpointURL = override
			}
		}
	}
	if e.circuitBreaker {
		allowed, retryAt, circuitErr := e.store.CanDispatchEndpoint(ctx, job.EndpointURL, time.Now().UTC())
		if circuitErr != nil {
			e.logger.Error(
				"circuit breaker check failed",
				"run_id", run.ID,
				"job_id", run.JobID,
				"endpoint", job.EndpointURL,
				"error", circuitErr,
			)
			e.handleSystemFailure(ctx, run, "circuit breaker unavailable")
			return
		}

		if !allowed {
			fields := map[string]any{
				"next_retry_at": retryAt,
				"error":         "endpoint circuit breaker open",
				"error_class":   "transient",
				"started_at":    nil,
				"finished_at":   nil,
			}
			if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, fields); err != nil {
				e.logger.Error(
					"failed to requeue run while circuit open",
					"run_id", run.ID,
					"job_id", run.JobID,
					"error", err,
				)
				return
			}

			e.recordRunTransition(ctx, domain.StatusDequeued, domain.StatusQueued)
			e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "queued", "reason": "circuit_open"})
			return
		}
	}

	acquired := e.tryAcquireBulkheadSlot(job.ID, job.MaxConcurrency)
	if !acquired {
		retryAt := NextRetryAt(run.Attempt)
		fields := map[string]any{
			"next_retry_at": retryAt,
			"error":         "job bulkhead at capacity",
			"error_class":   "transient",
			"started_at":    nil,
			"finished_at":   nil,
		}
		if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, fields); err != nil {
			e.logger.Error(
				"failed to requeue run while bulkhead at capacity",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
			return
		}

		e.recordRunTransition(ctx, domain.StatusDequeued, domain.StatusQueued)
		e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "queued", "reason": "bulkhead_capacity"})
		return
	}
	defer e.releaseBulkheadSlot(job.ID, job.MaxConcurrency)

	err = e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	})
	if err != nil {
		e.logger.Error(
			"failed to transition to executing",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	run.Status = domain.StatusExecuting
	e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "executing"})

	// Start heartbeat
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go e.heartbeat.Run(hbCtx, run.ID)

	timeout := time.Duration(policy.timeoutSecs) * time.Second
	if e.adaptiveTimeout && policy.timeoutSecs > 0 {
		stats, err := e.store.GetJobHealthStats(ctx, job.ID, time.Now().Add(-24*time.Hour))
		if err == nil && stats.P95DurationSecs > 0 {
			adaptiveTimeout := time.Duration(stats.P95DurationSecs * 1.5 * float64(time.Second))
			if adaptiveTimeout > timeout {
				timeout = adaptiveTimeout
				e.logger.Debug("using adaptive timeout", "job_id", job.ID, "p95_secs", stats.P95DurationSecs, "timeout", timeout)
			}
		}
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var result json.RawMessage
	var execTrace *domain.ExecutionTrace
	if job.ExecutionMode == domain.ExecutionModeSandbox {
		result, execTrace, err = e.dispatchSandbox(execCtx, job, run)
	} else {
		result, execTrace, err = e.tracedDispatch(execCtx, job, run)
	}
	if execTrace != nil {
		execTrace.TotalMs = durationMillisecondsAtLeastOne(time.Since(executeStart))
		queueWait := max(time.Duration(0), executeStart.Sub(run.CreatedAt))
		execTrace.QueueWaitMs = durationMillisecondsAtLeastOne(queueWait)
		if run.StartedAt != nil {
			dequeue := max(time.Duration(0), executeStart.Sub(*run.StartedAt))
			execTrace.DequeueMs = durationMillisecondsAtLeastOne(dequeue)
		}
	}
	if e.metrics != nil && execTrace != nil {
		e.metrics.ExecutionTraceDispatch.Record(ctx, float64(execTrace.DispatchMs))
		e.metrics.ExecutionTraceQueueWait.Record(ctx, float64(execTrace.QueueWaitMs))
	}
	if err != nil {
		// HTTP fallback only applies to HTTP-dispatched jobs. Sandbox jobs
		// execute code in Forge and have no meaningful HTTP fallback target.
		if job.ExecutionMode != domain.ExecutionModeSandbox && job.FallbackEndpointURL != "" {
			errClass := classifyError(err)
			if shouldUseFallbackForClass(errClass) {
				fallbackResult, fallbackErr := e.dispatchToEndpoint(execCtx, job.FallbackEndpointURL, run, nil)
				if fallbackErr == nil {
					e.handleSuccess(ctx, run, job, fallbackResult, execTrace)
					return
				}
				err = errors.Join(
					fmt.Errorf("primary dispatch failed: %w", err),
					fmt.Errorf("fallback dispatch failed: %w", fallbackErr),
				)
			}
		}

		if execCtx.Err() == context.DeadlineExceeded {
			e.handleTimeout(ctx, run, job, policy, execTrace)
		} else {
			e.handleFailure(ctx, run, job, policy, err, execTrace)
		}
		return
	}

	e.handleSuccess(ctx, run, job, result, execTrace)
}

func (e *Executor) tracedDispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	if !e.executionTracing {
		result, err := e.dispatch(ctx, job, run)
		return result, nil, err
	}

	dispatchStart := time.Now()
	var connectStart time.Time
	var connectDone time.Time
	var gotFirstByte time.Time

	trace := &httptrace.ClientTrace{
		ConnectStart:         func(string, string) { connectStart = time.Now() },
		ConnectDone:          func(string, string, error) { connectDone = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	tracedCtx := httptrace.WithClientTrace(ctx, trace)

	var extraHeaders map[string]string
	if e.secretInjection {
		secrets, err := e.store.ListJobSecretsByJob(tracedCtx, job.ID, "production")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
		}
		if len(secrets) > 0 {
			extraHeaders = make(map[string]string, len(secrets))
			for _, secret := range secrets {
				extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
			}
		}
	}

	result, err := e.dispatchToEndpoint(tracedCtx, job.EndpointURL, run, extraHeaders)
	gotLastByte := time.Now()

	execTrace := &domain.ExecutionTrace{}
	if !connectStart.IsZero() && !connectDone.IsZero() {
		execTrace.ConnectMs = durationMillisecondsAtLeastOne(connectDone.Sub(connectStart))
	}
	if !gotFirstByte.IsZero() {
		base := dispatchStart
		if !connectDone.IsZero() {
			base = connectDone
		}
		execTrace.TtfbMs = durationMillisecondsAtLeastOne(gotFirstByte.Sub(base))
	}
	if !gotFirstByte.IsZero() {
		execTrace.TransferMs = durationMillisecondsAtLeastOne(gotLastByte.Sub(gotFirstByte))
	}
	execTrace.DispatchMs = execTrace.ConnectMs + execTrace.TtfbMs + execTrace.TransferMs

	return result, execTrace, err
}

func (e *Executor) dispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.Dispatch")
	defer span.End()
	start := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.DispatchDuration.Record(ctx, time.Since(start).Seconds())
		}
	}()

	var extraHeaders map[string]string
	if e.secretInjection {
		secrets, err := e.store.ListJobSecretsByJob(ctx, job.ID, "production")
		if err != nil {
			return nil, fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
		}
		if len(secrets) > 0 {
			extraHeaders = make(map[string]string, len(secrets))
			for _, secret := range secrets {
				extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
			}
		}
	}

	return e.dispatchToEndpoint(ctx, job.EndpointURL, run, extraHeaders)
}

func (e *Executor) dispatchToEndpoint(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (json.RawMessage, error) {

	var body io.Reader
	if len(run.Payload) > 0 {
		body = bytes.NewReader(run.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", run.ID)
	req.Header.Set("X-Job-ID", run.JobID)
	req.Header.Set("X-Attempt", fmt.Sprintf("%d", run.Attempt))
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		if e.metrics != nil {
			e.metrics.DispatchErrors.Add(ctx, 1)
		}
		return nil, fmt.Errorf("http dispatch: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &domain.EndpointError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if len(respBody) > 0 {
		return json.RawMessage(respBody), nil
	}

	return nil, nil
}

func (e *Executor) resolveExecutionPolicy(ctx context.Context, run *domain.JobRun, fallback executionPolicy) (executionPolicy, error) {
	if run.WorkflowStepRunID == "" {
		return fallback, nil
	}

	stepRun, err := e.store.GetWorkflowStepRun(ctx, run.WorkflowStepRunID)
	if err != nil || stepRun == nil {
		if err != nil {
			return fallback, err
		}
		return fallback, nil
	}

	wfRun, err := e.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil || wfRun == nil {
		if err != nil {
			return fallback, err
		}
		return fallback, nil
	}

	steps, err := e.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fallback, err
	}

	for _, step := range steps {
		if step.StepRef != stepRun.StepRef {
			continue
		}

		if step.RetryMaxAttempts > 0 {
			fallback.maxAttempts = step.RetryMaxAttempts
		}
		if step.RetryBackoff != "" {
			fallback.retryBackoff = step.RetryBackoff
		}
		if step.RetryInitialDelaySecs > 0 {
			fallback.retryInitialSecs = step.RetryInitialDelaySecs
		}
		if step.RetryMaxDelaySecs > 0 {
			fallback.retryMaxSecs = step.RetryMaxDelaySecs
		}
		if step.TimeoutSecsOverride > 0 {
			fallback.timeoutSecs = step.TimeoutSecsOverride
		}
		return fallback, nil
	}

	return fallback, nil
}
