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
				c.evictions.Add(cacheMetricsContext, 1)
				c.entriesUp.Add(cacheMetricsContext, -1)
			},
		})
		if dep.Registry != nil {
			dep.Registry.Register(quotaCacheNamespace, straitcache.UpdatingStringTierHandler[*store.ProjectQuota]{Tier: c.inner})
		}
	}
	return c
}

func (c *quotaCache) Stop() {
	if !c.cacheEnabled() {
		return
	}
	c.inner.Stop()
}

func (c *quotaCache) Get(ctx context.Context, projectID string) (*store.ProjectQuota, error) {
	if c == nil {
		return nil, nil
	}
	if !c.cacheEnabled() {
		c.misses.Add(cacheMetricsContext, 1)
		return c.loadFn(ctx, projectID)
	}
	if _, ok := c.inner.GetIfPresent(projectID); ok {
		c.hits.Add(cacheMetricsContext, 1)
		return c.inner.Get(ctx, projectID, nil)
	}
	c.misses.Add(cacheMetricsContext, 1)
	loader := func(loadCtx context.Context, key string) (straitcache.Versioned[*store.ProjectQuota], error) {
		quota, err := c.loadFn(loadCtx, key)
		if err != nil {
			return straitcache.Versioned[*store.ProjectQuota]{}, err
		}
		if quota == nil {
			return straitcache.Versioned[*store.ProjectQuota]{Value: nil, Version: 0}, nil
		}
		return straitcache.Versioned[*store.ProjectQuota]{Value: quota, Version: projectQuotaCacheVersion(quota)}, nil
	}
	got, err := c.inner.GetConsistentVersioned(ctx, projectID, 0, loader)
	if err != nil {
		return nil, err
	}
	c.entriesUp.Add(cacheMetricsContext, 1)
	return got.Value, nil
}

func (c *quotaCache) Invalidate(projectID string) {
	c.InvalidateContext(cacheMetricsContext, projectID)
}

func (c *quotaCache) InvalidateContext(ctx context.Context, projectID string) {
	c.InvalidateWithVersionContext(ctx, projectID, time.Now().UnixNano())
}

func (c *quotaCache) InvalidateWithVersion(projectID string, version int64) {
	c.InvalidateWithVersionContext(cacheMetricsContext, projectID, version)
}

func (c *quotaCache) InvalidateWithVersionContext(ctx context.Context, projectID string, version int64) {
	if !c.cacheEnabled() {
		return
	}
	if ctx == nil {
		ctx = cacheMetricsContext
	}
	_ = c.inner.StrongInvalidate(
		ctx,
		strongCachePolicy(quotaCacheNamespace),
		projectID,
		projectID,
		cacheVersionBarrier(version),
		c.bus,
	)
}

func (c *quotaCache) cacheEnabled() bool {
	return c != nil && !c.disabled && c.inner != nil
}

func projectQuotaCacheVersion(quota *store.ProjectQuota) int64 {
	if quota == nil || quota.CacheVersion <= 0 {
		return 1
	}
	return quota.CacheVersion
}
