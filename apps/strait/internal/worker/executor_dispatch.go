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

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel"
)

// addHMACHeaders injects X-Strait-Signature and X-Strait-Timestamp into
// headers when the job has an endpoint_signing_secret configured. The
// signature covers "<unix_timestamp>.<body>" using HMAC-SHA256.
func addHMACHeaders(headers map[string]string, secret string, body []byte) {
	if secret == "" {
		return
	}
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	headers["X-Strait-Timestamp"] = ts
	headers["X-Strait-Signature"] = SignHTTPDispatch(secret, ts, body)
}

// resolveJobForRun loads the job configuration for a run, applying version
// policy rules. For "pin" (default), returns the enqueue-time version. For
// "latest", upgrades to the current version. For "minor", upgrades only if
// the current version is marked backwards_compatible.
func (e *Executor) resolveJobForRun(ctx context.Context, run *domain.JobRun) (*domain.Job, error) {
	var current *domain.Job
	if e.jobCache != nil {
		if cached, err := e.jobCache.Get(ctx, run.JobID); err == nil {
			current = cached
		}
	}

	if current == nil {
		var err error
		current, err = e.store.GetJob(ctx, run.JobID)
		if err != nil {
			return nil, fmt.Errorf("load current job: %w", err)
		}
		if e.jobCache != nil {
			_ = e.jobCache.Set(ctx, run.JobID, current)
		}
	}

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
	}

	return e.store.GetJobAtVersion(ctx, run.JobID, run.JobVersion)
}

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

//nolint:gocyclo,cyclop,funlen,gocognit
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
		e.handleSystemFailureWithJob(ctx, run, job, "resolve execution policy")
		return
	}
	policy = resolved

	// Billing enforcement: daily and concurrent run limits.
	if e.billingEnforcer != nil { //nolint:nestif // billing enforcement is inherently nested with multiple sequential checks
		if err := e.billingEnforcer.CheckProjectSuspended(ctx, job.ProjectID); err != nil {
			e.logger.Warn("project suspended",
				"run_id", run.ID, "project_id", job.ProjectID, "error", err)
			e.handleSystemFailureWithJob(ctx, run, job, err.Error())
			return
		}

		orgID, orgErr := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
		if orgErr != nil {
			e.logger.Warn("failed to resolve org for billing check",
				"run_id", run.ID, "error", orgErr, "fail_open", true)
		}
		if orgID != "" {
			if err := e.billingEnforcer.CheckDailyRunLimit(ctx, orgID); err != nil {
				e.logger.Warn("org daily run limit exceeded",
					"run_id", run.ID, "org_id", orgID, "error", err)
				e.handleSystemFailureWithJob(ctx, run, job, err.Error())
				return
			}
			if err := e.billingEnforcer.CheckConcurrentRunLimit(ctx, orgID); err != nil {
				e.logger.Warn("org concurrent run limit exceeded",
					"run_id", run.ID, "org_id", orgID, "error", err)
				e.billingEnforcer.DecrDailyRunCount(ctx, orgID)
				e.handleSystemFailureWithJob(ctx, run, job, err.Error())
				return
			}

			// HTTP mode plan gating at dispatch time.
			// Catches jobs created on Pro that continue after downgrade to Starter/Free.
			if job.ExecutionMode == domain.ExecutionModeHTTP || job.ExecutionMode == "" {
				limits, limErr := e.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
				if limErr == nil && !limits.AllowsHTTPMode {
					e.billingEnforcer.DecrDailyRunCount(ctx, orgID)
					e.handleSystemFailureWithJob(ctx, run, job,
						"HTTP execution mode requires the Pro plan. Upgrade at /settings/billing")
					return
				}
			}

			decrCtx := context.WithoutCancel(ctx)
			defer e.billingEnforcer.DecrConcurrentRunCount(decrCtx, orgID)
		}
	}

	switch job.ExecutionMode {
	case domain.ExecutionModeHTTP, "":
	default:
		e.logger.Error("unknown execution_mode", "run_id", run.ID, "job_id", run.JobID, "execution_mode", job.ExecutionMode)
		e.handleSystemFailureWithJob(ctx, run, job, fmt.Sprintf("unknown execution_mode: %s", job.ExecutionMode))
		return
	}

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
	// Run circuit breaker, health check, and adaptive timeout queries in parallel.
	// All three depend on job.EndpointURL (which env var resolution may have overridden above).
	var (
		circuitAllowed bool
		circuitRetryAt *time.Time
		circuitErr     error
		healthScore    *domain.EndpointHealthScore
		healthAllowed  bool
		healthErr      error
		adaptiveStats  *store.JobHealthStats
	)

	var prefetchWG conc.WaitGroup
	prefetchWG.Go(func() {
		circuitAllowed, circuitRetryAt, circuitErr = e.store.CanDispatchEndpoint(ctx, job.EndpointURL, time.Now().UTC())
	})
	prefetchWG.Go(func() {
		healthScore, healthAllowed, healthErr = e.healthScorer.CheckHealth(ctx, job.EndpointURL)
	})
	if policy.timeoutSecs > 0 {
		prefetchWG.Go(func() {
			adaptiveStats, _ = e.store.GetJobHealthStats(ctx, job.ID, time.Now().Add(-24*time.Hour))
		})
	}
	prefetchWG.Wait()

	if circuitErr != nil {
		e.logger.Error(
			"circuit breaker check failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"endpoint", job.EndpointURL,
			"error", circuitErr,
		)
		e.handleSystemFailureWithJob(ctx, run, job, "circuit breaker unavailable")
		return
	}

	if !circuitAllowed {
		e.snoozeRun(ctx, run, "endpoint circuit breaker open", circuitRetryAt)
		return
	}

	// Health score check: block unhealthy endpoints, throttle degraded ones.
	if healthErr != nil {
		e.logger.Warn(
			"health score check failed, proceeding with dispatch",
			"run_id", run.ID,
			"endpoint", job.EndpointURL,
			"error", healthErr,
		)
	} else if !healthAllowed {
		healthRetryAt := NextRetryAt(run.Attempt)
		e.logger.Info(
			"endpoint unhealthy, snoozing run",
			"run_id", run.ID,
			"endpoint", job.EndpointURL,
			"health_score", healthScore.HealthScore,
		)
		e.snoozeRun(ctx, run, "endpoint health score below threshold", &healthRetryAt)
		return
	}

	// Apply health-based concurrency throttling for degraded endpoints.
	effectiveConcurrency := job.MaxConcurrency
	if healthScore != nil {
		effectiveConcurrency = ThrottledConcurrency(healthScore, job.MaxConcurrency)
	}

	acquired := e.tryAcquireBulkheadSlot(job.ID, effectiveConcurrency)
	if !acquired {
		bulkheadRetryAt := NextRetryAt(run.Attempt)
		e.snoozeRun(ctx, run, "job bulkhead at capacity", &bulkheadRetryAt)
		return
	}
	defer e.releaseBulkheadSlot(job.ID, job.MaxConcurrency)

	// Claim-table dequeue already set status=executing; skip redundant transition.
	if run.Status != domain.StatusExecuting {
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
	}
	e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "executing"})

	e.heartbeat.Register(run.ID)
	defer e.heartbeat.Deregister(run.ID)

	timeout := time.Duration(policy.timeoutSecs) * time.Second
	if adaptiveStats != nil && adaptiveStats.P95DurationSecs > 0 {
		adaptiveTimeout := time.Duration(adaptiveStats.P95DurationSecs * 1.5 * float64(time.Second))
		if adaptiveTimeout > timeout {
			timeout = adaptiveTimeout
			e.logger.Debug("using adaptive timeout", "job_id", job.ID, "p95_secs", adaptiveStats.P95DurationSecs, "timeout", timeout)
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
				fallbackHeaders := make(map[string]string)
				addHMACHeaders(fallbackHeaders, job.EndpointSigningSecret, run.Payload)
				fallbackResult, fallbackErr := e.dispatchToEndpoint(execCtx, job.FallbackEndpointURL, run, fallbackHeaders)
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

	// Record HTTP run cost for Stripe billing and usage records (cloud only).
	if job.ExecutionMode == domain.ExecutionModeHTTP || job.ExecutionMode == "" {
		if e.metrics != nil && e.metrics.HTTPModeRunsCompleted != nil {
			e.metrics.HTTPModeRunsCompleted.Add(ctx, 1)
		}
		e.ingestStripeUsageEvent(ctx, job.ProjectID, run.ID, billing.HTTPCostPerRunMicrousd)
		if e.runCostRecorder != nil && e.billingEnforcer != nil {
			orgID, orgErr := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
			if orgErr == nil && orgID != "" {
				// Fire-and-forget: cost recording is non-critical; log failures but don't fail the run.
				costCtx := context.WithoutCancel(ctx)
				go func() {
					if err := e.runCostRecorder.RecordHTTPRunCost(costCtx, orgID, job.ProjectID, run.ID); err != nil {
						e.logger.Warn("failed to record HTTP run cost",
							"run_id", run.ID,
							"org_id", orgID,
							"error", err,
						)
					}
				}()
			}
		}
		// TODO(phase-7.2): wire RecordWorkerRunCost in the Worker dispatch result path.
	}

	e.handleSuccess(ctx, run, job, result, execTrace)
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

	// Fetch secrets and checkpoint (with dispatch cache).
	var (
		secrets    []domain.JobSecret
		secretsErr error
		cp         *domain.RunCheckpoint
	)

	secretsCacheKey := "secrets:" + job.ID
	if cached, ok := dispatchCacheGet[[]domain.JobSecret](ctx, secretsCacheKey); ok {
		secrets = cached
	} else {
		var dispatchWG conc.WaitGroup
		dispatchWG.Go(func() {
			secrets, secretsErr = e.store.ListJobSecretsByJob(tracedCtx, job.ID, "production")
		})
		if run.Attempt > 1 {
			checkpointCacheKey := "checkpoint:" + run.ID
			if cached, ok := dispatchCacheGet[*domain.RunCheckpoint](ctx, checkpointCacheKey); ok {
				cp = cached
			} else {
				dispatchWG.Go(func() {
					cp, _ = e.store.GetLatestCheckpoint(tracedCtx, run.ID)
				})
			}
		}
		dispatchWG.Wait()
		if secretsErr == nil {
			dispatchCacheSet(ctx, secretsCacheKey, secrets)
		}
		if run.Attempt > 1 && cp != nil {
			dispatchCacheSet(ctx, "checkpoint:"+run.ID, cp)
		}
	}

	if secretsErr != nil {
		return nil, nil, fmt.Errorf("failed to load secrets for job %s: %w", job.ID, secretsErr)
	}

	extraHeaders := make(map[string]string)
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

	// Add HMAC body+timestamp signing so the endpoint can verify request authenticity.
	addHMACHeaders(extraHeaders, job.EndpointSigningSecret, run.Payload)

	if run.Attempt > 1 {
		if cp != nil {
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
	var secrets []domain.JobSecret
	secretsCacheKey := "secrets:" + job.ID
	if cached, ok := dispatchCacheGet[[]domain.JobSecret](ctx, secretsCacheKey); ok {
		secrets = cached
	} else {
		var err error
		secrets, err = e.store.ListJobSecretsByJob(ctx, job.ID, "production")
		if err != nil {
			return fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
		}
		dispatchCacheSet(ctx, secretsCacheKey, secrets)
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
		checkpointCacheKey := "checkpoint:" + run.ID
		var cp *domain.RunCheckpoint
		if cached, ok := dispatchCacheGet[*domain.RunCheckpoint](ctx, checkpointCacheKey); ok {
			cp = cached
		} else {
			cp, _ = e.store.GetLatestCheckpoint(ctx, run.ID)
			dispatchCacheSet(ctx, checkpointCacheKey, cp)
		}
		if cp != nil {
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

	// Add HMAC body+timestamp signing so the endpoint can verify request authenticity.
	addHMACHeaders(extraHeaders, job.EndpointSigningSecret, run.Payload)

	_, dispatchErr := e.dispatchToEndpoint(ctx, job.EndpointURL, run, extraHeaders)
	return dispatchErr
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

// snoozeRunFromExecuting re-queues a run that is currently in the Executing
// state. This differs from snoozeRun which expects StatusDequeued as the
// source state.
//
//nolint:unparam // retryAt is nil in current callers but retained for symmetry with snoozeRun.
func (e *Executor) snoozeRunFromExecuting(ctx context.Context, run *domain.JobRun, reason string, retryAt *time.Time) {
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
			"run_id", run.ID, "snooze_count", snoozeCount)
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
	if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
		e.logger.Error("failed to snooze run from executing", "run_id", run.ID, "error", err)
		return
	}

	e.emit(ctx, RunLifecycleEvent{
		Type: EventSnoozed, Run: run,
		FromStatus: domain.StatusExecuting, ToStatus: domain.StatusQueued,
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

// ingestStripeUsageEvent sends a usage event to Stripe for metered billing.
// Runs asynchronously to avoid blocking the run completion path.
// Silently skips if no Stripe usage reporter is configured (self-hosted / dev).
func (e *Executor) ingestStripeUsageEvent(ctx context.Context, projectID, runID string, costMicroUSD int64) {
	if e.stripeUsageReporter == nil || e.billingEnforcer == nil || costMicroUSD <= 0 {
		return
	}

	// Look up the org's Stripe customer ID via the billing enforcer's store.
	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return
	}

	stripeCustomerID, err := e.billingEnforcer.GetStripeCustomerID(ctx, orgID)
	if err != nil || stripeCustomerID == "" {
		return
	}

	// Fire-and-forget: don't block the run on Stripe API latency.
	// Uses Background() intentionally — the parent request context may be canceled
	// before the Stripe API call completes, and we still want to record the usage.
	// Tracked via stripeUsageWG for graceful shutdown.
	e.stripeUsageWG.Go(func() {
		ingestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.stripeUsageReporter.IngestComputeUsage(ingestCtx, stripeCustomerID, runID, costMicroUSD); err != nil {
			e.logger.Warn("failed to ingest stripe usage event",
				"run_id", runID,
				"cost_microusd", costMicroUSD,
				"error", err,
			)
		}
	})
}
