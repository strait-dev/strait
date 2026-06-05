package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/store"

	"github.com/getsentry/sentry-go"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel"
)

type redactedHTTPDispatchError struct {
	message string
	err     error
}

// dispatchPrefetch groups the endpoint guards that must be evaluated after
// environment endpoint overrides, because every check keys off the final URL.
type dispatchPrefetch struct {
	circuitAllowed bool
	circuitRetryAt *time.Time
	circuitErr     error
	healthScore    *domain.EndpointHealthScore
	healthAllowed  bool
	healthErr      error
	adaptiveStats  *store.JobHealthStats
}

const (
	workflowStepVisibilityRetryDelay = 250 * time.Millisecond

	workerRequeueReasonDispatcherUnconfigured = "worker dispatcher not configured"
	workerRequeueReasonDispatchCancelled      = "worker dispatch cancelled"
	workerRequeueReasonNoWorker               = "no worker available"

	workerResultStatusSuccess = "success"
)

func (e *redactedHTTPDispatchError) Error() string {
	return e.message
}

func (e *redactedHTTPDispatchError) Unwrap() error {
	return e.err
}

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

func (e *Executor) endpointSigningSecret(job *domain.Job) (string, error) {
	secret, err := straitcrypto.DecryptField(e.secretDecryptor, job.EndpointSigningSecret)
	if err != nil {
		return "", fmt.Errorf("decrypt endpoint signing secret: %w", err)
	}
	return secret, nil
}

func dispatchSecretsCacheKey(job *domain.Job) string {
	return "secrets:" + job.ID + ":" + job.EnvironmentID
}

func (e *Executor) dispatchSecrets(ctx context.Context, job *domain.Job) ([]domain.JobSecret, error) {
	secretsCacheKey := dispatchSecretsCacheKey(job)
	if cached, ok := dispatchCacheGet[[]domain.JobSecret](ctx, secretsCacheKey); ok {
		return cached, nil
	}

	secrets, err := e.store.ListJobSecretsByJob(ctx, job.ID, job.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("load job %s secrets: %w", job.ID, err)
	}
	dispatchCacheSet(ctx, secretsCacheKey, secrets)
	return secrets, nil
}

type dispatchHeaderInputs struct {
	secrets    []domain.JobSecret
	checkpoint *domain.RunCheckpoint
}

func (e *Executor) dispatchHeaderInputs(ctx context.Context, job *domain.Job, run *domain.JobRun) (dispatchHeaderInputs, error) {
	secrets, err := e.dispatchSecrets(ctx, job)
	if err != nil {
		return dispatchHeaderInputs{}, err
	}
	return dispatchHeaderInputs{
		secrets:    secrets,
		checkpoint: e.dispatchCheckpoint(ctx, run),
	}, nil
}

func (e *Executor) dispatchHeaders(ctx context.Context, job *domain.Job, run *domain.JobRun) (map[string]string, error) {
	inputs, err := e.dispatchHeaderInputs(ctx, job, run)
	if err != nil {
		return nil, err
	}
	return e.buildDispatchHeaders(job, run, inputs.secrets, inputs.checkpoint)
}

// buildDispatchHeaders constructs the headers injected on an HTTP dispatch: the
// job's decrypted secrets (X-Secret-*), the run-token JWT (X-Run-Token) the
// endpoint SDK uses to call back to Strait, the HMAC body+timestamp signature,
// and on retries the durable-resume headers (X-Last-Checkpoint / X-Checkpoint-At
// / X-Previous-Error). It is shared by the primary and fallback dispatch paths so
// failover preserves authentication and durable-resume semantics rather than
// silently dropping them.
func (e *Executor) buildDispatchHeaders(job *domain.Job, run *domain.JobRun, secrets []domain.JobSecret, cp *domain.RunCheckpoint) (map[string]string, error) {
	headers := make(map[string]string)
	for _, secret := range secrets {
		headers[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
	}

	// Generate a JWT run token so the endpoint's SDK can call back to Strait.
	if e.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := struct {
			Attempt int `json:"attempt,omitempty"`
			jwt.RegisteredClaims
		}{
			Attempt: run.Attempt,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    domain.RunTokenIssuer,
				Subject:   run.ID,
				ExpiresAt: jwt.NewNumericDate(expiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		if signed, signErr := tok.SignedString([]byte(e.jwtSigningKey)); signErr == nil {
			headers["X-Run-Token"] = signed
		}
	}

	// Add HMAC body+timestamp signing so the endpoint can verify request authenticity.
	signingSecret, err := e.endpointSigningSecret(job)
	if err != nil {
		return nil, err
	}
	addHMACHeaders(headers, signingSecret, run.Payload)

	if run.Attempt > 1 {
		if cp != nil {
			data, _ := json.Marshal(cp.State)
			if len(data) <= 65536 {
				headers["X-Last-Checkpoint"] = string(data)
				headers["X-Checkpoint-At"] = cp.CreatedAt.Format(time.RFC3339)
			}
		}
		if run.Error != "" {
			headers["X-Previous-Error"] = run.Error
		}
	}
	return headers, nil
}

// dispatchCheckpoint loads the latest run checkpoint for a retry, preferring the
// per-execution dispatch cache populated by the primary path so the fallback path
// reuses it instead of re-querying. Returns nil on the first attempt.
func (e *Executor) dispatchCheckpoint(ctx context.Context, run *domain.JobRun) *domain.RunCheckpoint {
	if run.Attempt <= 1 {
		return nil
	}
	checkpointCacheKey := "checkpoint:" + run.ID
	if cached, ok := dispatchCacheGet[*domain.RunCheckpoint](ctx, checkpointCacheKey); ok {
		return cached
	}
	cp, _ := e.store.GetLatestCheckpoint(ctx, run.ID)
	if cp != nil {
		dispatchCacheSet(ctx, checkpointCacheKey, cp)
	}
	return cp
}

// applyEnvironmentEndpointOverride only swaps the URL when the override passes
// SSRF validation and the dispatch has no job secrets to redirect.
func (e *Executor) applyEnvironmentEndpointOverride(ctx context.Context, run *domain.JobRun, job *domain.Job) {
	if job.EnvironmentID == "" {
		return
	}
	envVars, err := e.store.GetResolvedEnvironmentVariables(ctx, job.EnvironmentID)
	if err != nil {
		e.logger.Warn("failed to resolve environment variables", "run_id", run.ID, "environment_id", job.EnvironmentID, "error", err)
		return
	}
	override := envVars["ENDPOINT_URL"]
	if override == "" {
		return
	}
	if err := validateEndpointURL(override); err != nil {
		e.logger.Warn("environment ENDPOINT_URL failed SSRF validation",
			"run_id", run.ID,
			"environment_id", job.EnvironmentID,
			"error", err,
		)
		return
	}
	secrets, err := e.dispatchSecrets(ctx, job)
	if err != nil {
		e.logger.Warn("environment ENDPOINT_URL ignored because dispatch secrets could not be checked",
			"run_id", run.ID,
			"environment_id", job.EnvironmentID,
			"error", err,
		)
		return
	}
	if len(secrets) > 0 {
		e.logger.Warn("environment ENDPOINT_URL ignored because job dispatch includes secrets",
			"run_id", run.ID,
			"environment_id", job.EnvironmentID,
		)
		return
	}
	e.logger.Info("overriding endpoint URL from environment",
		"run_id", run.ID,
		"environment_id", job.EnvironmentID,
	)
	job.EndpointURL = override
}

func (e *Executor) prefetchDispatchGuards(
	ctx context.Context,
	job *domain.Job,
	policy executionPolicy,
) dispatchPrefetch {
	endpointKey := endpointStateKey(job.ProjectID, job.EndpointURL)
	var result dispatchPrefetch
	var prefetchWG conc.WaitGroup
	prefetchWG.Go(func() {
		result.circuitAllowed, result.circuitRetryAt, result.circuitErr = e.store.CanDispatchEndpoint(
			ctx,
			endpointKey,
			time.Now().UTC(),
		)
	})
	prefetchWG.Go(func() {
		result.healthScore, result.healthAllowed, result.healthErr = e.healthScorer.CheckHealth(ctx, endpointKey)
	})
	if policy.timeoutSecs > 0 {
		prefetchWG.Go(func() {
			result.adaptiveStats, _ = e.getJobHealthStats(ctx, job.ID, time.Now())
		})
	}
	prefetchWG.Wait()
	return result
}

// resolveJobForRun loads the job configuration for a run, applying version
// policy rules. For "pin" (default), returns the enqueue-time version. For
// "latest", upgrades to the current version. For "minor", upgrades only if
// the current version is marked backwards_compatible.
func (e *Executor) resolveJobForRun(ctx context.Context, run *domain.JobRun) (*domain.Job, error) {
	var current *domain.Job
	bypassCache := false
	if e.jobCache != nil {
		if cached, err := e.jobCache.Get(ctx, run.JobID); err == nil {
			current = cloneJob(cached)
			if current.VersionPolicy == domain.VersionPolicyLatest || current.VersionPolicy == domain.VersionPolicyMinor {
				current = nil
				bypassCache = true
			}
		}
	}

	if current == nil {
		loadCurrent := func(loadCtx context.Context, jobID string) (*domain.Job, error) {
			job, gerr := e.store.GetJob(loadCtx, jobID)
			if gerr != nil {
				return nil, gerr
			}
			return cloneJob(job), nil
		}
		var err error
		if e.jobCache != nil && !bypassCache {
			current, err = e.jobCache.Load(ctx, run.JobID, loadCurrent)
		} else {
			current, err = loadCurrent(ctx, run.JobID)
			if err == nil && e.jobCache != nil {
				_ = e.jobCache.Set(ctx, run.JobID, current)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("load current job: %w", err)
		}
		current = cloneJob(current)
	}

	if current.Version == run.JobVersion {
		return cloneJob(current), nil
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
		return cloneJob(current), nil

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
			return cloneJob(current), nil
		}
		e.logger.Info("version policy: minor upgrade skipped (not backwards compatible)",
			"run_id", run.ID,
			"from_version", run.JobVersion,
			"current_version", current.Version,
		)
		// Fall through to load the enqueue-time version.

	case domain.VersionPolicyPin, "":
	}

	loadVersion := func(loadCtx context.Context, key jobVersionKey) (*domain.Job, error) {
		job, err := e.store.GetJobAtVersion(loadCtx, key.JobID, key.Version)
		if err != nil {
			return nil, err
		}
		return cloneJob(job), nil
	}
	if e.jobVersionCache != nil {
		return e.jobVersionCache.Load(ctx, jobVersionKey{JobID: run.JobID, Version: run.JobVersion}, loadVersion)
	}
	return loadVersion(ctx, jobVersionKey{JobID: run.JobID, Version: run.JobVersion})
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

func (e *Executor) resolveDispatchJobAndPolicy(ctx context.Context, run *domain.JobRun) (*domain.Job, executionPolicy, bool) {
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
		return nil, executionPolicy{}, false
	}

	policy, err := e.resolveExecutionPolicy(ctx, run, defaultExecutionPolicy(job))
	if err != nil {
		if errors.Is(err, store.ErrWorkflowStepRunNotFound) {
			retryAt := time.Now().Add(workflowStepVisibilityRetryDelay)
			e.logger.Warn("workflow step run not visible yet; requeueing run",
				"run_id", run.ID,
				"workflow_step_run_id", run.WorkflowStepRunID,
				"retry_at", retryAt,
			)
			e.snoozeRun(ctx, run, "workflow step run not visible yet", &retryAt)
			return nil, executionPolicy{}, false
		}
		e.logger.Error("failed to resolve execution policy", "run_id", run.ID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, "resolve execution policy")
		return nil, executionPolicy{}, false
	}
	return job, policy, true
}

type httpDispatchReadiness struct {
	prefetch        dispatchPrefetch
	releaseBulkhead func()
	ok              bool
}

func (e *Executor) prepareHTTPDispatch(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	policy executionPolicy,
) httpDispatchReadiness {
	e.applyEnvironmentEndpointOverride(ctx, run, job)
	prefetch := e.prefetchDispatchGuards(ctx, job, policy)
	if !e.checkEndpointGuards(ctx, run, job, prefetch) {
		return httpDispatchReadiness{}
	}

	releaseBulkhead, ok := e.acquireHTTPDispatchSlot(ctx, run, job, prefetch)
	if !ok {
		return httpDispatchReadiness{}
	}
	return httpDispatchReadiness{
		prefetch:        prefetch,
		releaseBulkhead: releaseBulkhead,
		ok:              true,
	}
}

func (e *Executor) acquireHTTPDispatchSlot(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	prefetch dispatchPrefetch,
) (func(), bool) {
	bulkheadLimit := httpDispatchConcurrencyLimit(job, prefetch)
	if !e.tryAcquireBulkheadSlot(job.ID, bulkheadLimit) {
		bulkheadRetryAt := NextRetryAt(run.Attempt)
		e.snoozeRun(ctx, run, "job bulkhead at capacity", &bulkheadRetryAt)
		return nil, false
	}
	return func() {
		e.releaseBulkheadSlot(job.ID, bulkheadLimit)
	}, true
}

func httpDispatchConcurrencyLimit(job *domain.Job, prefetch dispatchPrefetch) int {
	if prefetch.healthScore == nil {
		return job.MaxConcurrency
	}
	return ThrottledConcurrency(prefetch.healthScore, job.MaxConcurrency)
}

func (e *Executor) checkEndpointGuards(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	prefetch dispatchPrefetch,
) bool {
	if prefetch.circuitErr != nil {
		e.logger.Error(
			"circuit breaker check failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"endpoint", httputil.RedactURLForLog(job.EndpointURL),
			"error", prefetch.circuitErr,
		)
		e.handleSystemFailureWithJob(ctx, run, job, "circuit breaker unavailable")
		return false
	}
	if !prefetch.circuitAllowed {
		e.snoozeRun(ctx, run, "endpoint circuit breaker open", prefetch.circuitRetryAt)
		return false
	}
	if prefetch.healthErr != nil {
		e.logger.Warn(
			"health score check failed, proceeding with dispatch",
			"run_id", run.ID,
			"endpoint", httputil.RedactURLForLog(job.EndpointURL),
			"error", prefetch.healthErr,
		)
		return true
	}
	if prefetch.healthAllowed {
		return true
	}
	healthRetryAt := NextRetryAt(run.Attempt)
	e.logger.Info(
		"endpoint unhealthy, snoozing run",
		"run_id", run.ID,
		"endpoint", httputil.RedactURLForLog(job.EndpointURL),
		"health_score", prefetch.healthScore.HealthScore,
	)
	e.snoozeRun(ctx, run, "endpoint health score below threshold", &healthRetryAt)
	return false
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

func (e *Executor) dispatchTimeout(job *domain.Job, policy executionPolicy, stats *store.JobHealthStats) time.Duration {
	timeout := time.Duration(policy.timeoutSecs) * time.Second
	if stats == nil || stats.P95DurationSecs <= 0 {
		return timeout
	}
	adaptiveTimeout := time.Duration(stats.P95DurationSecs * 1.5 * float64(time.Second))
	if adaptiveTimeout <= timeout {
		return timeout
	}
	e.logger.Debug("using adaptive timeout", "job_id", job.ID, "p95_secs", stats.P95DurationSecs, "timeout", adaptiveTimeout)
	return adaptiveTimeout
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

func defaultExecutionPolicy(job *domain.Job) executionPolicy {
	return executionPolicy{
		maxAttempts:      job.MaxAttempts,
		timeoutSecs:      job.TimeoutSecs,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     3600,
	}
}

type httpDispatchTraceRecorder struct {
	mu            sync.Mutex
	dispatchStart time.Time
	connectStart  time.Time
	connectDone   time.Time
	gotFirstByte  time.Time
}

func newHTTPDispatchTraceRecorder(dispatchStart time.Time) *httpDispatchTraceRecorder {
	return &httpDispatchTraceRecorder{dispatchStart: dispatchStart}
}

func (r *httpDispatchTraceRecorder) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		ConnectStart: func(string, string) {
			r.recordConnectStart(time.Now())
		},
		ConnectDone: func(string, string, error) {
			r.recordConnectDone(time.Now())
		},
		GotFirstResponseByte: func() {
			r.recordFirstByte(time.Now())
		},
	}
}

func (r *httpDispatchTraceRecorder) recordConnectStart(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectStart = at
}

func (r *httpDispatchTraceRecorder) recordConnectDone(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectDone = at
}

func (r *httpDispatchTraceRecorder) recordFirstByte(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gotFirstByte = at
}

func (r *httpDispatchTraceRecorder) executionTrace(gotLastByte time.Time) *domain.ExecutionTrace {
	r.mu.Lock()
	connectStart := r.connectStart
	connectDone := r.connectDone
	gotFirstByte := r.gotFirstByte
	r.mu.Unlock()

	execTrace := &domain.ExecutionTrace{}
	if !connectStart.IsZero() && !connectDone.IsZero() {
		execTrace.ConnectMs = durationMillisecondsAtLeastOne(connectDone.Sub(connectStart))
	}
	if !gotFirstByte.IsZero() {
		base := r.dispatchStart
		if !connectDone.IsZero() {
			base = connectDone
		}
		execTrace.TtfbMs = durationMillisecondsAtLeastOne(gotFirstByte.Sub(base))
		execTrace.TransferMs = durationMillisecondsAtLeastOne(gotLastByte.Sub(gotFirstByte))
	}
	execTrace.DispatchMs = execTrace.ConnectMs + execTrace.TtfbMs + execTrace.TransferMs
	return execTrace
}

func (e *Executor) tracedDispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	traceRecorder := newHTTPDispatchTraceRecorder(time.Now())

	tracedCtx := httptrace.WithClientTrace(ctx, traceRecorder.clientTrace())

	// Fetch secrets and checkpoint (with dispatch cache).
	//
	// Secrets and checkpoint live in two independent cache entries and must
	// be resolved independently. The resume-header emission below
	// (X-Last-Checkpoint / X-Checkpoint-At) depends on `cp` being populated
	// on every retry attempt, not just retries that also miss the secrets
	// cache. A job with an ENDPOINT_URL environment override warms the
	// secrets cache with an empty slice on attempt 1; collapsing the
	// checkpoint load into the secrets cache-miss branch lets that cache
	// hit silently swallow the checkpoint on attempt 2 and break durable
	// resume.
	var inputs dispatchHeaderInputs
	var secretsErr error

	var dispatchWG conc.WaitGroup
	if cached, ok := dispatchCacheGet[[]domain.JobSecret](ctx, dispatchSecretsCacheKey(job)); ok {
		inputs.secrets = cached
	} else {
		dispatchWG.Go(func() {
			inputs.secrets, secretsErr = e.dispatchSecrets(tracedCtx, job)
		})
	}
	if run.Attempt > 1 {
		checkpointCacheKey := "checkpoint:" + run.ID
		if cached, ok := dispatchCacheGet[*domain.RunCheckpoint](ctx, checkpointCacheKey); ok {
			inputs.checkpoint = cached
		} else {
			dispatchWG.Go(func() {
				inputs.checkpoint, _ = e.store.GetLatestCheckpoint(tracedCtx, run.ID)
			})
		}
	}
	dispatchWG.Wait()
	if run.Attempt > 1 && inputs.checkpoint != nil {
		dispatchCacheSet(ctx, "checkpoint:"+run.ID, inputs.checkpoint)
	}

	if secretsErr != nil {
		return nil, nil, fmt.Errorf("load job %s secrets: %w", job.ID, secretsErr)
	}

	extraHeaders, err := e.buildDispatchHeaders(job, run, inputs.secrets, inputs.checkpoint)
	if err != nil {
		return nil, nil, err
	}

	result, err := e.dispatchToEndpoint(tracedCtx, job.EndpointURL, run, extraHeaders)
	gotLastByte := time.Now()

	return result, traceRecorder.executionTrace(gotLastByte), err
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

func (e *Executor) dispatchToEndpoint(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (json.RawMessage, error) {
	recordDispatchPayloadBytes(ctx, dispatchModeHTTP, len(run.Payload))
	req, err := newDispatchRequest(ctx, endpointURL, run, extraHeaders)
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		if e.metrics != nil {
			e.metrics.DispatchErrors.Add(ctx, 1)
		}
		recordDispatchAttempt(ctx, dispatchModeHTTP, dispatchOutcomeError)
		return nil, &redactedHTTPDispatchError{
			message: "http dispatch: " + httputil.SanitizeHTTPClientError(err),
			err:     err,
		}
	}
	defer resp.Body.Close()
	return readDispatchResponse(ctx, resp)
}

func readDispatchResponse(ctx context.Context, resp *http.Response) (json.RawMessage, error) {
	recordDispatchResponseStatus(ctx, dispatchModeHTTP, resp.StatusCode)

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)-2))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		recordDispatchAttempt(ctx, dispatchModeHTTP, dispatchOutcomeError)
		return nil, &domain.EndpointError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	recordDispatchAttempt(ctx, dispatchModeHTTP, dispatchOutcomeSuccess)

	if len(respBody) == 0 {
		return nil, nil
	}
	return normalizeDispatchResult(respBody), nil
}

func newDispatchRequest(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (*http.Request, error) {
	var body io.Reader
	if len(run.Payload) > 0 {
		body = bytes.NewReader(run.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: invalid endpoint URL")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", run.ID)
	req.Header.Set("X-Job-ID", run.JobID)
	req.Header.Set("X-Attempt", fmt.Sprintf("%d", run.Attempt))
	addRunTraceHeaders(req.Header, run.Metadata)

	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}
	return req, nil
}

func addRunTraceHeaders(headers http.Header, metadata map[string]string) {
	if tp, ok := metadata[domain.RunMetadataTraceParent]; ok && tp != "" {
		headers.Set("Traceparent", tp)
		if ts, ok := metadata[domain.RunMetadataTraceState]; ok && ts != "" {
			headers.Set("Tracestate", ts)
		}
	}
	if traceparent, ok := metadata[domain.RunMetadataSentryTrace]; ok && traceparent != "" {
		headers.Set(sentry.SentryTraceHeader, traceparent)
		if baggage, ok := metadata[domain.RunMetadataSentryBaggage]; ok && baggage != "" {
			headers.Set(sentry.SentryBaggageHeader, baggage)
		}
	}
}

func normalizeDispatchResult(body []byte) json.RawMessage {
	if json.Valid(body) {
		return json.RawMessage(body)
	}
	encoded, err := json.Marshal(string(body))
	if err != nil {
		return nil
	}
	return json.RawMessage(encoded)
}

func endpointStateKey(projectID, endpointURL string) string {
	if projectID == "" {
		return endpointURL
	}
	sum := sha256.Sum256([]byte(endpointURL))
	return "project:" + projectID + ":endpoint:" + hex.EncodeToString(sum[:])
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
		return fallback, fmt.Errorf("%w: %s", store.ErrWorkflowStepRunNotFound, run.WorkflowStepRunID)
	}

	runVersion, err := e.getWorkflowRunVersion(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fallback, err
	}
	if runVersion.WorkflowID == "" {
		return fallback, nil
	}

	steps, err := e.getWorkflowStepsForVersion(ctx, runVersion.WorkflowID, runVersion.Version)
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

func (e *Executor) getWorkflowRunVersion(ctx context.Context, workflowRunID string) (workflowRunVersion, error) {
	loader := func(loadCtx context.Context, key string) (workflowRunVersion, error) {
		wfRun, err := e.store.GetWorkflowRun(loadCtx, key)
		if err != nil || wfRun == nil {
			if err != nil {
				return workflowRunVersion{}, err
			}
			return workflowRunVersion{}, nil
		}
		return workflowRunVersion{WorkflowID: wfRun.WorkflowID, Version: wfRun.WorkflowVersion}, nil
	}
	if e.runVersionCache == nil {
		return loader(ctx, workflowRunID)
	}
	return e.runVersionCache.Load(ctx, workflowRunID, loader)
}

func (e *Executor) getWorkflowStepsForVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	key := workflowStepsVersionKey{WorkflowID: workflowID, Version: version}
	loader := func(loadCtx context.Context, loadKey workflowStepsVersionKey) ([]domain.WorkflowStep, error) {
		steps, err := e.store.ListStepsByWorkflowVersion(loadCtx, loadKey.WorkflowID, loadKey.Version)
		if err != nil {
			return nil, err
		}
		return domain.CloneWorkflowSteps(steps), nil
	}
	if e.stepsVersionCache == nil {
		return loader(ctx, key)
	}
	return e.stepsVersionCache.Load(ctx, key, loader)
}

func (e *Executor) getJobHealthStats(ctx context.Context, jobID string, now time.Time) (*store.JobHealthStats, error) {
	since := now.Add(-24 * time.Hour)
	if e.jobHealthCache == nil {
		return e.store.GetJobHealthStats(ctx, jobID, since)
	}
	key := e.jobHealthCache.Key(jobID, now)
	return e.jobHealthCache.Load(ctx, key, func(loadCtx context.Context, loadKey jobHealthKey) (*store.JobHealthStats, error) {
		return e.store.GetJobHealthStats(loadCtx, loadKey.JobID, since)
	})
}
