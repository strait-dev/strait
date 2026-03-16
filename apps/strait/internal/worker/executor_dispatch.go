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
	"strconv"
	"time"

	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
)

// resolveJobForRun loads the job configuration for a run, applying version
// policy rules. For "pin" (default), returns the enqueue-time version. For
// "latest", upgrades to the current version. For "minor", upgrades only if
// the current version is marked backwards_compatible.
func (e *Executor) resolveJobForRun(ctx context.Context, run *domain.JobRun) (*domain.Job, error) {
	// Check job cache first.
	cacheKey := run.JobID
	var current *domain.Job
	if entry, ok := e.jobCache.Load(cacheKey); ok {
		if cached := entry.(*cachedJob); time.Now().Before(cached.expiresAt) {
			current = cached.job
		}
	}

	if current == nil {
		var err error
		current, err = e.store.GetJob(ctx, run.JobID)
		if err != nil {
			return nil, fmt.Errorf("load current job: %w", err)
		}
		if e.jobCacheTTL > 0 {
			e.jobCache.Store(cacheKey, &cachedJob{job: current, expiresAt: time.Now().Add(e.jobCacheTTL)})
		}
	}

	// If the run is already at the current version, no policy check needed.
	if current.Version == run.JobVersion {
		return current, nil
	}

	switch current.VersionPolicy {
	case domain.VersionPolicyLatest:
		e.logger.Info("version policy upgrade",
			"run_id", run.ID,
			"policy", "latest",
			"from_version", run.JobVersion,
			"to_version", current.Version,
		)
		run.JobVersion = current.Version
		run.JobVersionID = current.VersionID
		return current, nil

	case domain.VersionPolicyMinor:
		if current.BackwardsCompatible {
			e.logger.Info("version policy upgrade",
				"run_id", run.ID,
				"policy", "minor",
				"from_version", run.JobVersion,
				"to_version", current.Version,
			)
			run.JobVersion = current.Version
			run.JobVersionID = current.VersionID
			return current, nil
		}
		e.logger.Info("version policy: minor upgrade skipped (not backwards compatible)",
			"run_id", run.ID,
			"from_version", run.JobVersion,
			"current_version", current.Version,
		)
		// Fall through to load the enqueue-time version.

	case domain.VersionPolicyPin, "":
		// Pin: use the enqueue-time version. Fall through.
	}

	// Load the versioned snapshot.
	return e.store.GetJobAtVersion(ctx, run.JobID, run.JobVersion)
}

func (e *Executor) execute(ctx context.Context, run *domain.JobRun) {
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

	job, err := e.resolveJobForRun(ctx, run)
	if err != nil || job == nil {
		e.logger.Error(
			"job lookup failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"job_version", run.JobVersion,
			"error", err,
		)
		e.handleSystemFailure(ctx, run, "job not found")
		return
	}
	ec.Job = job

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

	// Route based on execution mode.
	if job.ExecutionMode == domain.ExecutionModeManaged {
		e.managedDispatch(ctx, run, job)
		return
	}

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
		e.snoozeRun(ctx, run, "endpoint circuit breaker open", retryAt)
		return
	}

	acquired := e.tryAcquireBulkheadSlot(job.ID, job.MaxConcurrency)
	if !acquired {
		bulkheadRetryAt := NextRetryAt(run.Attempt)
		e.snoozeRun(ctx, run, "job bulkhead at capacity", &bulkheadRetryAt)
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

	e.heartbeat.Register(run.ID)
	defer e.heartbeat.Deregister(run.ID)

	timeout := time.Duration(policy.timeoutSecs) * time.Second
	if policy.timeoutSecs > 0 {
		stats, err := e.store.GetJobHealthStats(ctx, job.ID, time.Now().Add(-24*time.Hour))
		if err == nil && stats != nil && stats.P95DurationSecs > 0 {
			adaptiveTimeout := time.Duration(stats.P95DurationSecs * 1.5 * float64(time.Second))
			if adaptiveTimeout > timeout {
				timeout = adaptiveTimeout
				e.logger.Debug("using adaptive timeout", "job_id", job.ID, "p95_secs", stats.P95DurationSecs, "timeout", timeout)
			}
		}
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, execTrace, err := e.tracedDispatch(execCtx, job, run)
	if execTrace != nil {
		execTrace.TotalMs = durationMillisecondsAtLeastOne(time.Since(executeStart))
		queueWait := max(time.Duration(0), executeStart.Sub(run.CreatedAt))
		execTrace.QueueWaitMs = durationMillisecondsAtLeastOne(queueWait)
		if run.StartedAt != nil {
			dequeue := max(time.Duration(0), executeStart.Sub(*run.StartedAt))
			execTrace.DequeueMs = durationMillisecondsAtLeastOne(dequeue)
		}
	}
	if err != nil {
		if job.FallbackEndpointURL != "" {
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

// managedDispatch dispatches a job run to a container runtime (Fly Machines, Docker).
// This is a stub — the full implementation is in Phase 4 (STR-47).
func (e *Executor) managedDispatch(ctx context.Context, run *domain.JobRun, job *domain.Job) {
	_ = ctx
	_ = job
	e.logger.Error("managed execution not available: COMPUTE_RUNTIME not configured",
		"run_id", run.ID,
		"job_id", run.JobID,
		"execution_mode", job.ExecutionMode,
	)
	e.handleSystemFailure(ctx, run, "managed execution not available: COMPUTE_RUNTIME not configured")
}

func (e *Executor) tracedDispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
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

	extraHeaders := make(map[string]string)
	secrets, err := e.store.ListJobSecretsByJob(tracedCtx, job.ID, "production")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
	}
	for _, secret := range secrets {
		extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
	}

	if run.Attempt > 1 {
		cp, cpErr := e.store.GetLatestCheckpoint(tracedCtx, run.ID)
		if cpErr == nil && cp != nil {
			data, _ := json.Marshal(cp.State)
			if len(data) <= 65536 {
				extraHeaders["X-Last-Checkpoint"] = string(data)
				extraHeaders["X-Checkpoint-At"] = cp.CreatedAt.Format(time.RFC3339)
			}
		}
		if run.Error != "" {
			extraHeaders["X-Previous-Error"] = run.Error
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

func (e *Executor) dispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.Dispatch")
	defer span.End()
	start := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.DispatchDuration.Record(ctx, time.Since(start).Seconds())
		}
	}()

	extraHeaders := make(map[string]string)
	secrets, err := e.store.ListJobSecretsByJob(ctx, job.ID, "production")
	if err != nil {
		return fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
	}
	for _, secret := range secrets {
		extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
	}

	// Generate a JWT run token so the endpoint's SDK can call back to Strait.
	if e.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := jwt.RegisteredClaims{
			Subject:   run.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		if signed, signErr := tok.SignedString([]byte(e.jwtSigningKey)); signErr == nil {
			extraHeaders["X-Run-Token"] = signed
		}
	}

	if run.Attempt > 1 {
		cp, cpErr := e.store.GetLatestCheckpoint(ctx, run.ID)
		if cpErr == nil && cp != nil {
			data, _ := json.Marshal(cp.State)
			if len(data) <= 65536 {
				extraHeaders["X-Last-Checkpoint"] = string(data)
				extraHeaders["X-Checkpoint-At"] = cp.CreatedAt.Format(time.RFC3339)
			}
		}
		if run.Error != "" {
			extraHeaders["X-Previous-Error"] = run.Error
		}
	}

	_, err = e.dispatchToEndpoint(ctx, job.EndpointURL, run, extraHeaders)
	return err
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

	// Inject W3C trace context headers from run metadata.
	if tp, ok := run.Metadata["_trace_parent"]; ok && tp != "" {
		req.Header.Set("Traceparent", tp)
		if ts, ok := run.Metadata["_trace_state"]; ok && ts != "" {
			req.Header.Set("Tracestate", ts)
		}
	}

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

func (e *Executor) snoozeRun(ctx context.Context, run *domain.JobRun, reason string, retryAt *time.Time) {
	snoozeCount := 0
	if run.Metadata != nil {
		if raw, ok := run.Metadata["snooze_count"]; ok {
			if parsed, err := strconv.Atoi(raw); err == nil {
				snoozeCount = parsed
			}
		}
	}
	snoozeCount++

	if e.maxSnoozeCount > 0 && snoozeCount > e.maxSnoozeCount {
		e.logger.Warn("max snooze count exceeded, marking system_failed",
			"run_id", run.ID, "job_id", run.JobID, "snooze_count", snoozeCount)
		e.handleSystemFailure(ctx, run, fmt.Sprintf("max snooze count (%d) exceeded: %s", e.maxSnoozeCount, reason))
		return
	}

	fields := map[string]any{
		"error":         reason,
		"error_class":   "transient",
		"started_at":    nil,
		"finished_at":   nil,
		"next_retry_at": retryAt,
		"metadata":      map[string]string{"snooze_count": strconv.Itoa(snoozeCount)},
	}
	if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, fields); err != nil {
		e.logger.Error("failed to snooze run", "run_id", run.ID, "job_id", run.JobID, "error", err)
		return
	}

	e.emit(ctx, RunLifecycleEvent{
		Type: EventSnoozed, Run: run,
		FromStatus: domain.StatusDequeued, ToStatus: domain.StatusQueued,
		Attempt: run.Attempt,
	})
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
