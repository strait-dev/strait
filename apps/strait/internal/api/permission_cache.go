package api

import (
	"context"
	"sync"
	"time"

	straitcache "strait/internal/cache"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var metricsCtx = context.Background()

const permissionCacheNamespace = "permission"
const permissionProjectCacheNamespace = "permission_project"

type permissionCache struct {
	inner     *straitcache.Tier[string, []string]
	disabled  bool
	bus       *straitcache.Bus
	redis     redis.Cmdable
	mu        sync.Mutex
	byProject map[string]map[string]struct{}

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
		redis:     dep.Redis,
		byProject: make(map[string]map[string]struct{}),
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
			dep.Registry.Register(permissionProjectCacheNamespace, straitcache.NamespaceHandlerFuncs{
				Invalidate: func(ctx context.Context, projectID string, version int64) {
					c.invalidateProjectLocal(ctx, projectID)
				},
			})
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
	c.trackProjectKey(projectID, key)
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
	c.untrackProjectKey(projectID, key)
}

func (c *permissionCache) InvalidateProject(ctx context.Context, projectID string, version int64) {
	if c == nil || c.disabled || c.inner == nil || projectID == "" {
		return
	}
	if ctx == nil {
		ctx = metricsCtx
	}
	c.invalidateProjectLocal(ctx, projectID)
	if c.bus != nil {
		_ = c.bus.PublishInvalidate(ctx, permissionProjectCacheNamespace, projectID, version)
	}
}

func (c *permissionCache) invalidateProjectLocal(ctx context.Context, projectID string) {
	keys := c.projectKeys(projectID)
	for _, key := range keys {
		c.inner.Invalidate(ctx, key)
	}
	c.deleteRedisProjectKeys(ctx, projectID)
	c.clearProjectKeys(projectID)
}

func (c *permissionCache) trackProjectKey(projectID, key string) {
	if c == nil || projectID == "" || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := c.byProject[projectID]
	if keys == nil {
		keys = make(map[string]struct{})
		c.byProject[projectID] = keys
	}
	keys[key] = struct{}{}
}

func (c *permissionCache) untrackProjectKey(projectID, key string) {
	if c == nil || projectID == "" || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := c.byProject[projectID]
	if keys == nil {
		return
	}
	delete(keys, key)
	if len(keys) == 0 {
		delete(c.byProject, projectID)
	}
}

func (c *permissionCache) clearProjectKeys(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byProject, projectID)
}

func (c *permissionCache) projectKeys(projectID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := c.byProject[projectID]
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	return out
}

func (c *permissionCache) deleteRedisProjectKeys(ctx context.Context, projectID string) {
	if c == nil || c.redis == nil || projectID == "" {
		return
	}
	pattern := "strait:cache:" + permissionCacheNamespace + ":" + projectID + "\x00*"
	var cursor uint64
	for {
		keys, next, err := c.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			_ = c.redis.Del(ctx, keys...).Err()
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}
