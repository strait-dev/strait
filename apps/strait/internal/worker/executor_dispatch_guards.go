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

func (e *Executor) prefetchDispatchGuards(
	ctx context.Context,
	job *domain.Job,
	policy executionPolicy,
) dispatchPrefetch {
	endpointKey := endpointStateKey(job.ProjectID, job.EndpointURL)
	now := time.Now().UTC()
	if cached, ok := e.endpointGuardCache.get(endpointKey, now); ok {
		if e.shouldLoadAdaptiveTimeoutStats(policy) {
			cached.adaptiveStats, _ = e.getJobHealthStats(ctx, job.ID, now)
		}
		return cached
	}

	var result dispatchPrefetch
	var prefetchWG conc.WaitGroup
	prefetchWG.Go(func() {
		result.circuitAllowed, result.circuitRetryAt, result.circuitErr = e.store.CanDispatchEndpoint(
			ctx,
			endpointKey,
			now,
		)
	})
	prefetchWG.Go(func() {
		result.healthScore, result.healthAllowed, result.healthErr = e.healthScorer.CheckHealth(ctx, endpointKey)
	})
	if e.shouldLoadAdaptiveTimeoutStats(policy) {
		prefetchWG.Go(func() {
			result.adaptiveStats, _ = e.getJobHealthStats(ctx, job.ID, now)
		})
	}
	prefetchWG.Wait()
	e.endpointGuardCache.setAllowed(endpointKey, now, result)
	return result
}

func (e *Executor) shouldLoadAdaptiveTimeoutStats(policy executionPolicy) bool {
	return e != nil && e.adaptiveTimeoutEnabled && policy.timeoutSecs > 0
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

func endpointStateKey(projectID, endpointURL string) string {
	if projectID == "" {
		return endpointURL
	}
	sum := sha256.Sum256([]byte(endpointURL))
	const prefix = "project:"
	const separator = ":endpoint:"
	out := make([]byte, len(prefix)+len(projectID)+len(separator)+sha256.Size*2)
	n := copy(out, prefix)
	n += copy(out[n:], projectID)
	n += copy(out[n:], separator)
	hex.Encode(out[n:], sum[:])
	return string(out)
}
