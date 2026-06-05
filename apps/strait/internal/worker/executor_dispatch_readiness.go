package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
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

type httpDispatchReadiness struct {
	prefetch        dispatchPrefetch
	releaseBulkhead func()
	ok              bool
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
