package worker

import (
	"context"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

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
