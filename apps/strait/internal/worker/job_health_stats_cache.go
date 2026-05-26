package worker

import (
	"context"
	"time"

	"strait/internal/store"
)

// getJobHealthStatsCached returns the JobHealthStats for a job, served from a
// short TTL cache when one is configured. The dispatch adaptive-timeout path
// (executor_dispatch.go) and the post-run latency anomaly check
// (executor_handlers.go) both want Avg/P95/P99 over a fixed 24h window for the
// same job, often dozens of times per second under load. The underlying query
// is a PERCENTILE_CONT ordered-set aggregate over job_runs which is several
// orders of magnitude more expensive than the count-only sibling — under load
// it dominates the dispatch span and contends for the same Postgres connection
// pool the rest of the worker shares. A small TTL (~30s by default; configured
// via ExecutorConfig.JobHealthStatsCacheTTL) is a fine tradeoff: p95 over 24h
// does not move meaningfully on that horizon, but coalescing 1000 dispatches
// into a single store call per TTL window is decisive.
//
// When the cache is disabled (TTL == 0, store nil) the helper falls back to
// a direct store call so callers can stay branch-free.
//
// since is honored on cache miss but does not contribute to the cache key:
// both production callers pass time.Now().Add(-24*time.Hour) and a sub-second
// drift across cache lifetimes is irrelevant to the stats they consume.
// Pass-through of since is preserved so the function remains a drop-in
// replacement for store.GetJobHealthStats from the caller's perspective.
func (e *Executor) getJobHealthStatsCached(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error) {
	if e.jobHealthStatsCache == nil {
		return e.store.GetJobHealthStats(ctx, jobID, since)
	}

	if cached, err := e.jobHealthStatsCache.Get(ctx, jobID); err == nil && cached != nil {
		return cached, nil
	}

	v, err, _ := e.jobHealthStatsGroup.Do(jobID, func() (any, error) {
		// Re-check inside singleflight: another caller may have populated
		// the cache while we were queued behind it.
		if cached, cErr := e.jobHealthStatsCache.Get(ctx, jobID); cErr == nil && cached != nil {
			return cached, nil
		}
		stats, sErr := e.store.GetJobHealthStats(ctx, jobID, since)
		if sErr != nil {
			return nil, sErr
		}
		if stats != nil {
			// Best-effort set; cache misses on Set are not fatal.
			_ = e.jobHealthStatsCache.Set(ctx, jobID, stats)
		}
		return stats, nil
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(*store.JobHealthStats), nil
}
