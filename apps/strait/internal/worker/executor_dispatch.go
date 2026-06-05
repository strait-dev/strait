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
	"maps"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"

	workergrpc "strait/internal/api/grpc"
	"strait/internal/billing"
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

const workflowStepVisibilityRetryDelay = 250 * time.Millisecond

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

func (e *Executor) enforceDispatchBilling(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
) (func(), bool) {
	if e.billingEnforcer == nil {
		if e.edition.RequiresHTTPModeGating() {
			e.logger.Warn("billing enforcer unavailable for gated dispatch", "run_id", run.ID, "project_id", job.ProjectID)
			e.handleSystemFailureWithJob(ctx, run, job, "billing enforcement unavailable")
			return nil, false
		}
		return nil, true
	}
	if err := e.billingEnforcer.CheckProjectSuspended(ctx, job.ProjectID); err != nil {
		e.logger.Warn("project suspended", "run_id", run.ID, "project_id", job.ProjectID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return nil, false
	}

	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
	if err != nil {
		e.logger.Warn("failed to resolve org for billing check", "run_id", run.ID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, "billing enforcement unavailable")
		return nil, false
	}
	if orgID == "" {
		return nil, true
	}
	if !e.checkDispatchBillingLimits(ctx, run, job, orgID) {
		return nil, false
	}
	releaseCtx := context.WithoutCancel(ctx)
	return func() {
		e.billingEnforcer.DecrConcurrentRunCount(releaseCtx, orgID)
	}, true
}

func (e *Executor) checkDispatchBillingLimits(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	orgID string,
) bool {
	if err := e.billingEnforcer.CheckSpendingLimit(ctx, orgID); err != nil {
		e.logger.Warn("org spending limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	if err := e.billingEnforcer.CheckProjectBudgetLimit(ctx, job.ProjectID); err != nil {
		e.logger.Warn("project budget limit exceeded", "run_id", run.ID, "project_id", job.ProjectID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	countedMonthlyRun := shouldCountMonthlyRun(run)
	if countedMonthlyRun {
		if err := e.billingEnforcer.CheckMonthlyRunLimitForRun(ctx, orgID, run.ID); err != nil {
			e.logger.Warn("org monthly run limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
			e.handleSystemFailureWithJob(ctx, run, job, err.Error())
			return false
		}
	}
	if err := e.billingEnforcer.CheckConcurrentRunLimit(ctx, orgID); err != nil {
		e.logger.Warn("org concurrent run limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
		if countedMonthlyRun {
			e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
		}
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	return e.checkDispatchHTTPModeAllowed(ctx, run, job, orgID, countedMonthlyRun)
}

func shouldCountMonthlyRun(run *domain.JobRun) bool {
	if run == nil {
		return false
	}
	// Manual replays create a fresh run with attempt 1 and remain billable.
	// Automatic retries reuse the same run ID with attempt >1, so they must not
	// consume another monthly run or create another Stripe overage marker.
	return run.Attempt <= 1
}

func (e *Executor) checkDispatchHTTPModeAllowed(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	orgID string,
	countedMonthlyRun bool,
) bool {
	if job.ExecutionMode != domain.ExecutionModeHTTP && job.ExecutionMode != "" {
		return true
	}
	limits, err := e.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		if countedMonthlyRun {
			e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
		}
		e.billingEnforcer.DecrConcurrentRunCount(ctx, orgID)
		e.handleSystemFailureWithJob(ctx, run, job, "billing enforcement unavailable")
		return false
	}
	if limits.AllowsHTTPMode {
		return true
	}
	billing.RecordHTTPModeGateRejected(ctx, string(limits.PlanTier), "dispatch")
	// CheckConcurrentRunLimit already INCR'd the per-org concurrent counter on
	// the under-limit path; this early return happens before enforceDispatchBilling
	// installs the deferred DecrConcurrentRunCount, so balance it here to avoid
	// leaking the counter on every HTTP-mode-gate rejection.
	e.billingEnforcer.DecrConcurrentRunCount(ctx, orgID)
	if countedMonthlyRun {
		e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
	}
	e.handleSystemFailureWithJob(ctx, run, job, "HTTP execution mode is unavailable for this organization. Contact support if this persists.")
	return false
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

func cloneJob(job *domain.Job) *domain.Job {
	if job == nil {
		return nil
	}
	cloned := *job
	if job.Tags != nil {
		cloned.Tags = maps.Clone(job.Tags)
	}
	if job.DefaultRunMetadata != nil {
		cloned.DefaultRunMetadata = maps.Clone(job.DefaultRunMetadata)
	}
	if job.RetryDelaysSecs != nil {
		cloned.RetryDelaysSecs = append([]int(nil), job.RetryDelaysSecs...)
	}
	if job.RateLimitKeys != nil {
		cloned.RateLimitKeys = append([]domain.RateLimitKey(nil), job.RateLimitKeys...)
	}
	if job.PreferredRegions != nil {
		cloned.PreferredRegions = append([]string(nil), job.PreferredRegions...)
	}
	if job.ResultSchema != nil {
		cloned.ResultSchema = append(json.RawMessage(nil), job.ResultSchema...)
	}
	if job.OnCompletePayloadMapping != nil {
		cloned.OnCompletePayloadMapping = append(json.RawMessage(nil), job.OnCompletePayloadMapping...)
	}
	if job.OnFailurePayloadMapping != nil {
		cloned.OnFailurePayloadMapping = append(json.RawMessage(nil), job.OnFailurePayloadMapping...)
	}
	return &cloned
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

	effectiveConcurrency := job.MaxConcurrency
	if prefetch.healthScore != nil {
		effectiveConcurrency = ThrottledConcurrency(prefetch.healthScore, job.MaxConcurrency)
	}
	if !e.tryAcquireBulkheadSlot(job.ID, effectiveConcurrency) {
		bulkheadRetryAt := NextRetryAt(run.Attempt)
		e.snoozeRun(ctx, run, "job bulkhead at capacity", &bulkheadRetryAt)
		return httpDispatchReadiness{}
	}
	return httpDispatchReadiness{
		prefetch: prefetch,
		releaseBulkhead: func() {
			e.releaseBulkheadSlot(job.ID, job.MaxConcurrency)
		},
		ok: true,
	}
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
	secrets, err := e.dispatchSecrets(ctx, job)
	if err != nil {
		return nil, errors.Join(primaryErr, err), false
	}
	fallbackHeaders, err := e.buildDispatchHeaders(job, run, secrets, e.dispatchCheckpoint(ctx, run))
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

func (e *Executor) recordHTTPRunCost(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	if job.ExecutionMode != domain.ExecutionModeHTTP && job.ExecutionMode != "" {
		return
	}
	billing.RecordHTTPModeRunCompleted(ctx)
	e.recordTerminalRunBilling(ctx, job, run)
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

func (e *Executor) tracedDispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	dispatchStart := time.Now()

	// httptrace callbacks fire from net/http transport goroutines that have
	// no formal happens-before with the post-dispatch reader below, so a
	// short-lived mutex guards the three timestamps to keep -race clean.
	var (
		traceMu      sync.Mutex
		connectStart time.Time
		connectDone  time.Time
		gotFirstByte time.Time
	)

	trace := &httptrace.ClientTrace{
		ConnectStart: func(string, string) {
			now := time.Now()
			traceMu.Lock()
			connectStart = now
			traceMu.Unlock()
		},
		ConnectDone: func(string, string, error) {
			now := time.Now()
			traceMu.Lock()
			connectDone = now
			traceMu.Unlock()
		},
		GotFirstResponseByte: func() {
			now := time.Now()
			traceMu.Lock()
			gotFirstByte = now
			traceMu.Unlock()
		},
	}

	tracedCtx := httptrace.WithClientTrace(ctx, trace)

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
	var (
		secrets    []domain.JobSecret
		secretsErr error
		cp         *domain.RunCheckpoint
	)

	var dispatchWG conc.WaitGroup
	if cached, ok := dispatchCacheGet[[]domain.JobSecret](ctx, dispatchSecretsCacheKey(job)); ok {
		secrets = cached
	} else {
		dispatchWG.Go(func() {
			secrets, secretsErr = e.dispatchSecrets(tracedCtx, job)
		})
	}
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
	if run.Attempt > 1 && cp != nil {
		dispatchCacheSet(ctx, "checkpoint:"+run.ID, cp)
	}

	if secretsErr != nil {
		return nil, nil, fmt.Errorf("load job %s secrets: %w", job.ID, secretsErr)
	}

	extraHeaders, err := e.buildDispatchHeaders(job, run, secrets, cp)
	if err != nil {
		return nil, nil, err
	}

	result, err := e.dispatchToEndpoint(tracedCtx, job.EndpointURL, run, extraHeaders)
	gotLastByte := time.Now()

	traceMu.Lock()
	cs, cd, gfb := connectStart, connectDone, gotFirstByte
	traceMu.Unlock()

	execTrace := &domain.ExecutionTrace{}
	if !cs.IsZero() && !cd.IsZero() {
		execTrace.ConnectMs = durationMillisecondsAtLeastOne(cd.Sub(cs))
	}
	if !gfb.IsZero() {
		base := dispatchStart
		if !cd.IsZero() {
			base = cd
		}
		execTrace.TtfbMs = durationMillisecondsAtLeastOne(gfb.Sub(base))
	}
	if !gfb.IsZero() {
		execTrace.TransferMs = durationMillisecondsAtLeastOne(gotLastByte.Sub(gfb))
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
	secrets, err := e.dispatchSecrets(ctx, job)
	if err != nil {
		return err
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
	signingSecret, err := e.endpointSigningSecret(job)
	if err != nil {
		return err
	}
	addHMACHeaders(extraHeaders, signingSecret, run.Payload)

	_, dispatchErr := e.dispatchToEndpoint(ctx, job.EndpointURL, run, extraHeaders)
	return dispatchErr
}

func (e *Executor) dispatchToEndpoint(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (json.RawMessage, error) {
	recordDispatchPayloadBytes(ctx, "http", len(run.Payload))
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

	// Inject W3C trace context headers from run metadata.
	if tp, ok := run.Metadata[domain.RunMetadataTraceParent]; ok && tp != "" {
		req.Header.Set("Traceparent", tp)
		if ts, ok := run.Metadata[domain.RunMetadataTraceState]; ok && ts != "" {
			req.Header.Set("Tracestate", ts)
		}
	}
	if traceparent, ok := run.Metadata[domain.RunMetadataSentryTrace]; ok && traceparent != "" {
		req.Header.Set(sentry.SentryTraceHeader, traceparent)
		if baggage, ok := run.Metadata[domain.RunMetadataSentryBaggage]; ok && baggage != "" {
			req.Header.Set(sentry.SentryBaggageHeader, baggage)
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
		recordDispatchAttempt(ctx, "http", "error")
		return nil, &redactedHTTPDispatchError{
			message: "http dispatch: " + httputil.SanitizeHTTPClientError(err),
			err:     err,
		}
	}
	defer resp.Body.Close()
	recordDispatchResponseStatus(ctx, "http", resp.StatusCode)

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)-2))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		recordDispatchAttempt(ctx, "http", "error")
		return nil, &domain.EndpointError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	recordDispatchAttempt(ctx, "http", "success")

	if len(respBody) > 0 {
		return normalizeDispatchResult(respBody), nil
	}

	return nil, nil
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
		"error":       reason,
		"error_class": "transient",
		"started_at":  nil,
		"finished_at": nil,
		"metadata":    map[string]string{"snooze_count": strconv.Itoa(snoozeCount)},
	}
	if retryAt != nil {
		if err := e.store.ScheduleRetry(ctx, run.ID, *retryAt, run.Attempt); err != nil {
			e.logger.Error("failed to schedule snooze retry", "run_id", run.ID, "job_id", run.JobID, "error", err)
			return
		}
	} else if err := e.store.ClearRetry(ctx, run.ID); err != nil {
		e.logger.Warn("failed to clear retry on snooze", "run_id", run.ID, "job_id", run.JobID, "error", err)
	}
	from := domain.StatusDequeued
	if run.Status == domain.StatusExecuting {
		from = domain.StatusExecuting
	}
	if err := e.store.SnoozeRunWithLock(ctx, run.ID, from, domain.StatusQueued, fields); err != nil {
		if errors.Is(err, store.ErrRunLocked) {
			recordSnoozeSkipped(ctx, string(from), "locked")
			e.logger.Warn("snooze skipped: run row locked by another transaction",
				"run_id", run.ID, "job_id", run.JobID, "from", from)
			return
		}
		if errors.Is(err, store.ErrRunConflict) {
			recordSnoozeSkipped(ctx, string(from), "conflict")
			e.logger.Warn("snooze skipped: run no longer in expected state",
				"run_id", run.ID, "job_id", run.JobID, "from", from)
			return
		}
		e.logger.Error("failed to snooze run", "run_id", run.ID, "job_id", run.JobID, "error", err)
		return
	}
	if retryAt == nil {
		run.Status = domain.StatusQueued
		e.enqueueExistingRunIfReady(ctx, run, "snooze")
	}

	e.emit(ctx, RunLifecycleEvent{
		Type: EventSnoozed, Run: run,
		FromStatus: from, ToStatus: domain.StatusQueued,
		Attempt: run.Attempt,
	})
}

func endpointStateKey(projectID, endpointURL string) string {
	if projectID == "" {
		return endpointURL
	}
	sum := sha256.Sum256([]byte(endpointURL))
	return "project:" + projectID + ":endpoint:" + hex.EncodeToString(sum[:])
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
		"error":       reason,
		"error_class": "transient",
		"started_at":  nil,
		"finished_at": nil,
		"metadata":    map[string]string{"snooze_count": strconv.Itoa(snoozeCount)},
	}
	if retryAt != nil {
		if err := e.store.ScheduleRetry(ctx, run.ID, *retryAt, run.Attempt); err != nil {
			e.logger.Error("failed to schedule snooze retry from executing", "run_id", run.ID, "error", err)
			return
		}
	} else if err := e.store.ClearRetry(ctx, run.ID); err != nil {
		e.logger.Warn("failed to clear retry on snooze from executing", "run_id", run.ID, "error", err)
	}
	if err := e.store.SnoozeRunWithLock(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
		if errors.Is(err, store.ErrRunLocked) {
			recordSnoozeSkipped(ctx, string(domain.StatusExecuting), "locked")
			e.logger.Warn("snooze-from-executing skipped: run row locked by another transaction",
				"run_id", run.ID, "from", domain.StatusExecuting)
			return
		}
		if errors.Is(err, store.ErrRunConflict) {
			recordSnoozeSkipped(ctx, string(domain.StatusExecuting), "conflict")
			e.logger.Warn("snooze-from-executing skipped: run no longer in expected state",
				"run_id", run.ID, "from", domain.StatusExecuting)
			return
		}
		e.logger.Error("failed to snooze run from executing", "run_id", run.ID, "error", err)
		return
	}
	if retryAt == nil {
		run.Status = domain.StatusQueued
		e.enqueueExistingRunIfReady(ctx, run, "snooze_from_executing")
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

// executeWorkerMode dispatches a run to a connected gRPC worker. It mirrors
// the HTTP dispatch flow for billing, status transitions, and retry logic.
//
// If no worker is currently available for the run's queue, the run is left
// in its current state so it can be re-claimed on the next poll tick.
//
// On a successful result, cost is recorded via RecordWorkerRunCost.
func (e *Executor) executeWorkerMode(ctx context.Context, run *domain.JobRun, job *domain.Job, policies ...executionPolicy) {
	dispatchStarted := time.Now()
	dispatchOutcome := "success"
	defer func() {
		recordWorkerDispatch(context.Background(), "grpc", dispatchOutcome, dispatchStarted)
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
		dispatchOutcome = "error"
		recordWorkerRetry(ctx, "dispatcher_unconfigured")
		e.requeueWorkerModeRun(ctx, run, "worker dispatcher not configured")
		return
	}

	// Transition to executing if not already (claim-table dequeue may have
	// set it already).
	if run.Status != domain.StatusExecuting {
		if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
			"started_at": time.Now(),
		}); err != nil {
			e.logger.Error("executeWorkerMode: transition to executing failed",
				"run_id", run.ID,
				"error", err,
			)
			dispatchOutcome = "error"
			return
		}
		run.Status = domain.StatusExecuting
	}
	e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "executing"})
	e.heartbeat.Register(run.ID)
	defer e.heartbeat.Deregister(run.ID)

	timeout := time.Duration(policy.timeoutSecs) * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := e.workerDispatcher.WorkerDispatch(execCtx, run, job)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Worker-mode dispatch uses the same execution timeout policy as HTTP mode.
			dispatchOutcome = "timeout"
			recordWorkerRetry(ctx, "timeout")
			e.handleTimeout(ctx, run, job, policy, nil)
			return
		}
		if errors.Is(err, context.Canceled) {
			dispatchOutcome = "error"
			recordWorkerRetry(ctx, "cancelled")
			e.requeueWorkerModeRun(ctx, run, "worker dispatch cancelled")
			return
		}
		// ErrNoWorkerAvailable: leave queued, next tick retries.
		// Any other error: treat as a dispatch failure.
		e.logger.Warn("worker dispatch failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		if errors.Is(err, workergrpc.ErrNoWorkerAvailable) {
			dispatchOutcome = "error"
			recordWorkerRetry(ctx, "no_worker")
			e.requeueWorkerModeRun(ctx, run, "no worker available")
			return
		}
		policy := executionPolicy{
			maxAttempts:      job.MaxAttempts,
			timeoutSecs:      job.TimeoutSecs,
			retryBackoff:     domain.RetryBackoffExponential,
			retryInitialSecs: 1,
			retryMaxSecs:     3600,
		}
		dispatchOutcome = "error"
		recordWorkerRetry(ctx, "dispatch_error")
		e.handleFailure(ctx, run, job, policy, err, nil)
		return
	}

	// Inspect the worker's reported terminal status. Only "success" routes
	// to the success handler; everything else (including "failed", "" from
	// a nil/malformed result, or any unexpected sentinel) is routed to
	// handleFailure with the worker-supplied error message so retry / DLQ
	// policies kick in. This avoids silently recording worker failures as
	// successes and bypassing the executor's retry path.
	status := e.workerDispatcher.ResultStatus(result)
	if status != "success" {
		errMsg := e.workerDispatcher.ResultError(result)
		if errMsg == "" {
			if status == "" {
				errMsg = "worker returned malformed or empty result"
			} else {
				errMsg = fmt.Sprintf("worker reported terminal status %q without error message", status)
			}
		}
		if e.handleFailure(ctx, run, job, policy, errors.New(errMsg), nil) {
			e.completeWorkerTask(ctx, result, domain.WorkerTaskStatusFailed)
		}
		dispatchOutcome = "error"
		recordWorkerRetry(ctx, "worker_failure")
		return
	}

	// Successful result — record cost and complete the run.
	e.recordWorkerModeCost(ctx, run, job)

	runResult := e.workerDispatcher.ResultOutput(result)
	if e.handleSuccess(ctx, run, job, runResult) {
		e.completeWorkerTask(ctx, result, domain.WorkerTaskStatusCompleted)
	}
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

	policy := executionPolicy{
		maxAttempts:      job.MaxAttempts,
		timeoutSecs:      job.TimeoutSecs,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     3600,
	}
	if status != "success" {
		if errorMessage == "" {
			if status == "" {
				errorMessage = "worker returned malformed or empty result"
			} else {
				errorMessage = fmt.Sprintf("worker reported terminal status %q without error message", status)
			}
		}
		if !e.handleFailure(ctx, run, job, policy, errors.New(errorMessage), nil) {
			return "", fmt.Errorf("worker failure finalization did not transition run")
		}
		return domain.WorkerTaskStatusFailed, nil
	}

	e.recordWorkerModeCost(ctx, run, job)
	if !e.handleSuccess(ctx, run, job, output) {
		return "", fmt.Errorf("worker success finalization did not transition run")
	}
	return domain.WorkerTaskStatusCompleted, nil
}

func (e *Executor) recordWorkerModeCost(ctx context.Context, run *domain.JobRun, job *domain.Job) {
	e.recordTerminalRunBilling(ctx, job, run)
}

func (e *Executor) recordTerminalRunBilling(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	if job == nil || run == nil {
		return
	}
	costMicroUSD := billing.HTTPCostPerRunMicrousd
	recordCost := func(costCtx context.Context, orgID string) error {
		if e.runCostRecorder == nil {
			return nil
		}
		return e.runCostRecorder.RecordHTTPRunCost(costCtx, orgID, job.ProjectID, run.ID)
	}
	if job.ExecutionMode == domain.ExecutionModeWorker {
		costMicroUSD = billing.WorkerCostPerRunMicrousd
		recordCost = func(costCtx context.Context, orgID string) error {
			if e.runCostRecorder == nil {
				return nil
			}
			return e.runCostRecorder.RecordWorkerRunCost(costCtx, orgID, job.ProjectID, run.ID)
		}
	}

	e.ingestStripeUsageEvent(ctx, job.ProjectID, run.ID, costMicroUSD)

	if e.runCostRecorder == nil || e.billingEnforcer == nil {
		return
	}
	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
	if err != nil || orgID == "" {
		return
	}
	costCtx := context.WithoutCancel(ctx)
	e.stripeUsageWG.Go(func() {
		if err := recordCost(costCtx, orgID); err != nil {
			e.logger.Warn("failed to record run cost",
				"run_id", run.ID,
				"org_id", orgID,
				"execution_mode", job.ExecutionMode,
				"error", err,
			)
		}
	})
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

// ingestStripeUsageEvent sends a usage event to Stripe for metered billing.
// Runs asynchronously to avoid blocking the run completion path.
// Silently skips if no Stripe usage reporter is configured (self-hosted / dev).
// costMicroUSD is the per-run cost in micro-USD; HTTP and worker modes pass
// distinct constants today, but they currently coincide at 20µ$.
//
//nolint:unparam // HTTP/worker pass distinct named constants that may diverge
func (e *Executor) ingestStripeUsageEvent(ctx context.Context, projectID, runID string, costMicroUSD int64) {
	if e.stripeUsageReporter == nil || e.billingEnforcer == nil || costMicroUSD <= 0 {
		return
	}
	if !e.billingEnforcer.IsRunOverage(ctx, runID) {
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
		if err := e.stripeUsageReporter.IngestRunOverage(ingestCtx, stripeCustomerID, runID); err != nil {
			e.logger.Warn("failed to ingest stripe usage event",
				"run_id", runID,
				"error", err,
			)
		}
	})
}
