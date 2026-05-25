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
	bus      *straitcache.Bus

	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	entriesUp metric.Int64UpDownCounter
}

const quotaCacheNamespace = "quota"

func newQuotaCache(
	ttl time.Duration,
	loadFn func(ctx context.Context, projectID string) (*store.ProjectQuota, error),
	deps ...apiCacheDeps,
) *quotaCache {
	var dep apiCacheDeps
	if len(deps) > 0 {
		dep = deps[0]
	}
	meter := otel.Meter("strait")
	hits, _ := meter.Int64Counter("strait_quota_cache_hits_total")
	misses, _ := meter.Int64Counter("strait_quota_cache_misses_total")
	evictions, _ := meter.Int64Counter("strait_quota_cache_evictions_total")
	entriesUp, _ := meter.Int64UpDownCounter("strait_quota_cache_entries")

	var l2 straitcache.L2[string, *store.ProjectQuota]
	if dep.Redis != nil {
		l2 = straitcache.NewRedisL2[string, *store.ProjectQuota](straitcache.RedisL2Config[string, *store.ProjectQuota]{
			Client:    dep.Redis,
			Namespace: quotaCacheNamespace,
		})
	}
	c := &quotaCache{
		loadFn:    loadFn,
		disabled:  ttl <= 0,
		bus:       dep.Bus,
		hits:      hits,
		misses:    misses,
		evictions: evictions,
		entriesUp: entriesUp,
	}
	if !c.disabled {
		c.inner = straitcache.NewTier[string, *store.ProjectQuota](straitcache.TierConfig[string, *store.ProjectQuota]{
			Name:           quotaCacheNamespace,
			L2:             l2,
			Consistency:    straitcache.Strong,
			MaximumSize:    10_000,
			TTL:            ttl,
			TTLJitter:      0.1,
			DisableL1:      l2 != nil,
			DisableL2:      l2 == nil,
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
		if dep.Registry != nil {
			dep.Registry.Register(quotaCacheNamespace, straitcache.UpdatingStringTierHandler[*store.ProjectQuota]{Tier: c.inner})
		}
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
	_ = c.inner.InvalidateThrough(metricsCtx, projectID, c.bus, quotaCacheNamespace, projectID, time.Now().UnixNano())
}
