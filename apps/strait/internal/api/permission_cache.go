package api

import (
	"context"
	"reflect"
	"sync"
	"time"

	straitcache "strait/internal/cache"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

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
				c.evictions.Add(cacheMetricsContext, 1)
				c.entriesUp.Add(cacheMetricsContext, -1)
			},
		})
		if dep.Registry != nil {
			dep.Registry.Register(permissionCacheNamespace, straitcache.UpdatingStringTierHandler[[]string]{Tier: c.inner})
			dep.Registry.Register(permissionProjectCacheNamespace, straitcache.NamespaceHandlerFuncs{
				Invalidate: func(ctx context.Context, projectID string, _ int64) {
					c.invalidateProjectLocal(ctx, projectID)
				},
			})
		}
	}
	return c
}

func (c *permissionCache) Stop() {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.Stop()
}

func (c *permissionCache) cacheEnabled() bool {
	return c != nil && !c.disabled && c.inner != nil
}

func (c *permissionCache) key(projectID, userID string) string {
	return projectID + "\x00" + userID
}

func (c *permissionCache) Get(projectID, userID string) ([]string, bool) {
	return c.GetContext(cacheMetricsContext, projectID, userID)
}

func (c *permissionCache) GetContext(ctx context.Context, projectID, userID string) ([]string, bool) {
	if ctx == nil {
		ctx = cacheMetricsContext
	}
	if !c.cacheEnabled() {
		if c != nil {
			c.misses.Add(ctx, 1)
		}
		return nil, false
	}
	perms, err := c.inner.Get(ctx, c.key(projectID, userID), nil)
	if err != nil {
		c.misses.Add(ctx, 1)
		return nil, false
	}
	c.hits.Add(ctx, 1)
	return perms, true
}

func (c *permissionCache) Set(projectID, userID string, permissions []string) {
	c.SetContext(cacheMetricsContext, projectID, userID, permissions)
}

func (c *permissionCache) SetContext(ctx context.Context, projectID, userID string, permissions []string) {
	c.SetWithVersionContext(ctx, projectID, userID, permissions, time.Now().UnixNano())
}

func (c *permissionCache) SetWithVersion(projectID, userID string, permissions []string, version int64) {
	c.SetWithVersionContext(cacheMetricsContext, projectID, userID, permissions, version)
}

func (c *permissionCache) SetWithVersionContext(
	ctx context.Context,
	projectID string,
	userID string,
	permissions []string,
	version int64,
) {
	if !c.cacheEnabled() {
		return
	}
	if ctx == nil {
		ctx = cacheMetricsContext
	}
	if version <= 0 {
		version = 1
	}
	key := c.key(projectID, userID)
	_, existed := c.inner.GetIfPresent(key)
	_, _ = c.inner.StrongWriteThrough(
		ctx,
		strongCachePolicy(permissionCacheNamespace),
		key,
		key,
		permissions,
		version,
		c.bus,
	)
	c.trackProjectKey(ctx, projectID, key)
	if !existed {
		c.entriesUp.Add(ctx, 1)
	}
}

func (c *permissionCache) Invalidate(projectID, userID string) {
	c.InvalidateContext(cacheMetricsContext, projectID, userID)
}

func (c *permissionCache) InvalidateContext(ctx context.Context, projectID, userID string) {
	c.InvalidateWithVersionContext(ctx, projectID, userID, time.Now().UnixNano())
}

func (c *permissionCache) InvalidateWithVersion(projectID, userID string, version int64) {
	c.InvalidateWithVersionContext(cacheMetricsContext, projectID, userID, version)
}

func (c *permissionCache) InvalidateWithVersionContext(ctx context.Context, projectID, userID string, version int64) {
	if !c.cacheEnabled() {
		return
	}
	if ctx == nil {
		ctx = cacheMetricsContext
	}
	key := c.key(projectID, userID)
	_ = c.inner.StrongInvalidate(
		ctx,
		strongCachePolicy(permissionCacheNamespace),
		key,
		key,
		cacheVersionBarrier(version),
		c.bus,
	)
	c.untrackProjectKey(ctx, projectID, key)
}

func (c *permissionCache) InvalidateProject(ctx context.Context, projectID string, version int64) {
	if !c.cacheEnabled() {
		return
	}
	if projectID == "" {
		return
	}
	if ctx == nil {
		ctx = cacheMetricsContext
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

func (c *permissionCache) trackProjectKey(ctx context.Context, projectID, key string) {
	if !c.canIndexProjectKey(projectID, key) {
		return
	}
	c.mu.Lock()
	keys := c.byProject[projectID]
	if keys == nil {
		keys = make(map[string]struct{})
		c.byProject[projectID] = keys
	}
	keys[key] = struct{}{}
	c.mu.Unlock()

	c.addRedisProjectKey(ctx, projectID, key)
}

func (c *permissionCache) untrackProjectKey(ctx context.Context, projectID, key string) {
	if !c.canIndexProjectKey(projectID, key) {
		return
	}
	c.mu.Lock()
	keys := c.byProject[projectID]
	if keys == nil {
		c.mu.Unlock()
		return
	}
	delete(keys, key)
	if len(keys) == 0 {
		delete(c.byProject, projectID)
	}
	c.mu.Unlock()

	c.removeRedisProjectKey(ctx, projectID, key)
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
	if !c.canIndexRedisProject(projectID) {
		return
	}
	indexKey := c.redisProjectIndexKey(projectID)
	members, err := c.redis.SMembers(ctx, indexKey).Result()
	if err == nil && len(members) > 0 {
		keys := make([]string, 0, len(members)+1)
		for _, key := range members {
			keys = append(keys, c.redisPermissionKey(key))
		}
		keys = append(keys, indexKey)
		_ = c.redis.Del(ctx, keys...).Err()
		return
	}
	if err == nil {
		_ = c.redis.Del(ctx, indexKey).Err()
	}
	c.scanDeleteRedisProjectKeys(ctx, projectID)
}

func (c *permissionCache) addRedisProjectKey(ctx context.Context, projectID, key string) {
	if !c.canIndexRedisProjectKey(projectID, key) {
		return
	}
	_ = c.redis.SAdd(ctx, c.redisProjectIndexKey(projectID), key).Err()
}

func (c *permissionCache) removeRedisProjectKey(ctx context.Context, projectID, key string) {
	if !c.canIndexRedisProjectKey(projectID, key) {
		return
	}
	_ = c.redis.SRem(ctx, c.redisProjectIndexKey(projectID), key).Err()
}

func (c *permissionCache) canIndexProjectKey(projectID, key string) bool {
	if c == nil {
		return false
	}
	if projectID == "" {
		return false
	}
	return key != ""
}

func (c *permissionCache) canIndexRedisProject(projectID string) bool {
	if c == nil {
		return false
	}
	if projectID == "" {
		return false
	}
	return redisCmdableReady(c.redis)
}

func (c *permissionCache) canIndexRedisProjectKey(projectID, key string) bool {
	if !c.canIndexRedisProject(projectID) {
		return false
	}
	return key != ""
}

func (c *permissionCache) redisProjectIndexKey(projectID string) string {
	return "strait:cache:index:" + permissionCacheNamespace + ":" + projectID
}

func (c *permissionCache) redisPermissionKey(key string) string {
	return "strait:cache:" + permissionCacheNamespace + ":" + key
}

func (c *permissionCache) scanDeleteRedisProjectKeys(ctx context.Context, projectID string) {
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

func redisCmdableReady(client redis.Cmdable) bool {
	if client == nil {
		return false
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}
