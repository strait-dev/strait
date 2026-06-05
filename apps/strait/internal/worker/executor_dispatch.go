package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel"
)

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

func endpointStateKey(projectID, endpointURL string) string {
	if projectID == "" {
		return endpointURL
	}
	sum := sha256.Sum256([]byte(endpointURL))
	return "project:" + projectID + ":endpoint:" + hex.EncodeToString(sum[:])
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
