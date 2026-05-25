package api

import (
	"context"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type quotaCache struct {
	inner    *straitcache.Tier[string, *store.ProjectQuota]
	loadFn   func(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	disabled bool

	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	entriesUp metric.Int64UpDownCounter
}

func newQuotaCache(
	ttl time.Duration,
	loadFn func(ctx context.Context, projectID string) (*store.ProjectQuota, error),
) *quotaCache {
	meter := otel.Meter("strait")
	hits, _ := meter.Int64Counter("strait_quota_cache_hits_total")
	misses, _ := meter.Int64Counter("strait_quota_cache_misses_total")
	evictions, _ := meter.Int64Counter("strait_quota_cache_evictions_total")
	entriesUp, _ := meter.Int64UpDownCounter("strait_quota_cache_entries")

	c := &quotaCache{
		loadFn:    loadFn,
		disabled:  ttl <= 0,
		hits:      hits,
		misses:    misses,
		evictions: evictions,
		entriesUp: entriesUp,
	}
	if !c.disabled {
		c.inner = straitcache.NewTier[string, *store.ProjectQuota](straitcache.TierConfig[string, *store.ProjectQuota]{
			Name:           "quota",
			Consistency:    straitcache.Strong,
			MaximumSize:    10_000,
			TTL:            ttl,
			TTLJitter:      0.1,
			DisableL2:      true,
			EnableNegative: true,
			Clone: func(quota *store.ProjectQuota) *store.ProjectQuota {
				if quota == nil {
					return nil
				}
				clone := *quota
				return &clone
			},
			OnDelete: func(string) {
				c.evictions.Add(metricsCtx, 1)
				c.entriesUp.Add(metricsCtx, -1)
			},
		})
	}
	return c
}

func (c *quotaCache) Get(ctx context.Context, projectID string) (*store.ProjectQuota, error) {
	if c == nil {
		return nil, nil
	}
	if c.disabled || c.inner == nil {
		c.misses.Add(metricsCtx, 1)
		return c.loadFn(ctx, projectID)
	}
	if _, ok := c.inner.GetIfPresent(projectID); ok {
		c.hits.Add(metricsCtx, 1)
		return c.inner.Get(ctx, projectID, nil)
	}
	c.misses.Add(metricsCtx, 1)
	got, err := c.inner.Get(ctx, projectID, c.loadFn)
	if err != nil {
		return nil, err
	}
	c.entriesUp.Add(metricsCtx, 1)
	return got, nil
}

func (c *quotaCache) Invalidate(projectID string) {
	if c == nil || c.disabled || c.inner == nil {
		return
	}
	c.inner.Invalidate(metricsCtx, projectID)
}
