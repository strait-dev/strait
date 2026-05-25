package api

import (
	"context"
	"time"

	straitcache "strait/internal/cache"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var metricsCtx = context.Background()

const permissionCacheNamespace = "permission"

type permissionCache struct {
	inner    *straitcache.Tier[string, []string]
	disabled bool
	bus      *straitcache.Bus

	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	entriesUp metric.Int64UpDownCounter
}

func newPermissionCache(ttl time.Duration, deps ...apiCacheDeps) *permissionCache {
	var dep apiCacheDeps
	if len(deps) > 0 {
		dep = deps[0]
	}
	meter := otel.Meter("strait")
	hits, _ := meter.Int64Counter("strait_permission_cache_hits_total")
	misses, _ := meter.Int64Counter("strait_permission_cache_misses_total")
	evictions, _ := meter.Int64Counter("strait_permission_cache_evictions_total")
	entriesUp, _ := meter.Int64UpDownCounter("strait_permission_cache_entries")

	var l2 straitcache.L2[string, []string]
	if dep.Redis != nil {
		l2 = straitcache.NewRedisL2[string, []string](straitcache.RedisL2Config[string, []string]{
			Client:    dep.Redis,
			Namespace: permissionCacheNamespace,
		})
	}
	c := &permissionCache{
		disabled:  ttl <= 0,
		bus:       dep.Bus,
		hits:      hits,
		misses:    misses,
		evictions: evictions,
		entriesUp: entriesUp,
	}
	if !c.disabled {
		c.inner = straitcache.NewTier[string, []string](straitcache.TierConfig[string, []string]{
			Name:        permissionCacheNamespace,
			L2:          l2,
			Consistency: straitcache.Strong,
			MaximumSize: 10_000,
			TTL:         ttl,
			TTLJitter:   0.1,
			DisableL1:   l2 != nil,
			DisableL2:   l2 == nil,
			Clone: func(perms []string) []string {
				if perms == nil {
					return nil
				}
				clone := make([]string, len(perms))
				copy(clone, perms)
				return clone
			},
			OnDelete: func(string) {
				c.evictions.Add(metricsCtx, 1)
				c.entriesUp.Add(metricsCtx, -1)
			},
		})
		if dep.Registry != nil {
			dep.Registry.Register(permissionCacheNamespace, straitcache.UpdatingStringTierHandler[[]string]{Tier: c.inner})
		}
	}
	return c
}

func (c *permissionCache) Stop() {}

func (c *permissionCache) key(projectID, userID string) string {
	return projectID + "\x00" + userID
}

func (c *permissionCache) Get(projectID, userID string) ([]string, bool) {
	if c == nil || c.disabled || c.inner == nil {
		if c != nil {
			c.misses.Add(metricsCtx, 1)
		}
		return nil, false
	}
	perms, err := c.inner.Get(metricsCtx, c.key(projectID, userID), nil)
	if err != nil {
		c.misses.Add(metricsCtx, 1)
		return nil, false
	}
	c.hits.Add(metricsCtx, 1)
	return perms, true
}

func (c *permissionCache) Set(projectID, userID string, permissions []string) {
	if c == nil || c.disabled || c.inner == nil {
		return
	}
	key := c.key(projectID, userID)
	_, existed := c.inner.GetIfPresent(key)
	_, _ = c.inner.WriteThrough(metricsCtx, key, permissions, time.Now().UnixNano(), c.bus, permissionCacheNamespace, key)
	if !existed {
		c.entriesUp.Add(metricsCtx, 1)
	}
}

func (c *permissionCache) Invalidate(projectID, userID string) {
	if c == nil || c.disabled || c.inner == nil {
		return
	}
	key := c.key(projectID, userID)
	_ = c.inner.InvalidateThrough(metricsCtx, key, c.bus, permissionCacheNamespace, key, time.Now().UnixNano())
}
