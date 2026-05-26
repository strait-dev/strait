package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

type jobDepsCacheKey struct {
	JobID  string
	Limit  int
	Cursor string
}

type jobDependencyCache struct {
	tier *straitcache.Tier[jobDepsCacheKey, []domain.JobDependency]
	bus  *straitcache.Bus
}

const jobDependencyCacheNamespace = "api_job_dependencies"

var jobDependencyCachedPageLimits = [...]int{defaultPageLimit + 1, 1000}

func newJobDependencyCache(ttl time.Duration, deps ...apiCacheDeps) *jobDependencyCache {
	if ttl <= 0 {
		return nil
	}
	var dep apiCacheDeps
	if len(deps) > 0 {
		dep = deps[0]
	}
	var l2 straitcache.L2[jobDepsCacheKey, []domain.JobDependency]
	if dep.Redis != nil {
		l2 = straitcache.NewRedisL2[jobDepsCacheKey, []domain.JobDependency](straitcache.RedisL2Config[jobDepsCacheKey, []domain.JobDependency]{
			Client:    dep.Redis,
			Namespace: jobDependencyCacheNamespace,
			Key:       jobDepsCacheKeyString,
		})
	}
	c := &jobDependencyCache{bus: dep.Bus}
	c.tier = straitcache.NewTier[jobDepsCacheKey, []domain.JobDependency](straitcache.TierConfig[jobDepsCacheKey, []domain.JobDependency]{
		Name:          jobDependencyCacheNamespace,
		L2:            l2,
		Consistency:   straitcache.BoundedStaleness,
		MaximumWeight: 100_000,
		Weigher: func(_ jobDepsCacheKey, deps []domain.JobDependency) uint32 {
			if len(deps) == 0 {
				return 1
			}
			if len(deps) > 100_000 {
				return 100_000
			}
			return uint32(len(deps)) // #nosec G115 -- bounded above before conversion.
		},
		TTL:       ttl,
		TTLJitter: 0.1,
		DisableL2: l2 == nil,
		Clone: func(deps []domain.JobDependency) []domain.JobDependency {
			return append([]domain.JobDependency(nil), deps...)
		},
	})
	if dep.Registry != nil {
		dep.Registry.Register(jobDependencyCacheNamespace, straitcache.TierHandler[jobDepsCacheKey, []domain.JobDependency]{
			Tier:  c.tier,
			Parse: parseJobDepsCacheKey,
		})
	}
	return c
}

func (c *jobDependencyCache) List(ctx context.Context, key jobDepsCacheKey, loader func(context.Context, jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error)) ([]domain.JobDependency, error) {
	if c == nil || c.tier == nil {
		loaded, err := loader(ctx, key)
		return loaded.Value, err
	}
	loaded, err := c.tier.GetConsistentVersioned(ctx, key, 0, loader)
	if err != nil {
		return nil, err
	}
	return loaded.Value, nil
}

func (c *jobDependencyCache) InvalidateJob(ctx context.Context, jobID string) {
	if c == nil || c.tier == nil || jobID == "" {
		return
	}
	for _, limit := range jobDependencyCachedPageLimits {
		key := jobDepsCacheKey{JobID: jobID, Limit: limit}
		_ = c.tier.InvalidateThrough(ctx, key, c.bus, jobDependencyCacheNamespace, jobDepsCacheKeyString(key), time.Now().UnixNano())
	}
}

func (c *jobDependencyCache) RefreshJob(ctx context.Context, jobID string, loader func(context.Context, jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error)) {
	if c == nil || c.tier == nil || jobID == "" {
		return
	}
	for _, limit := range jobDependencyCachedPageLimits {
		key := jobDepsCacheKey{JobID: jobID, Limit: limit}
		loaded, err := loader(ctx, key)
		if err != nil {
			c.InvalidateJob(ctx, jobID)
			return
		}
		version := loaded.Version
		if version <= 0 {
			version = time.Now().UnixNano()
		}
		_, _ = c.tier.WriteThrough(ctx, key, loaded.Value, version, c.bus, jobDependencyCacheNamespace, jobDepsCacheKeyString(key))
	}
}

func jobDependencyCacheableLimit(limit int) bool {
	for _, cached := range jobDependencyCachedPageLimits {
		if limit == cached {
			return true
		}
	}
	return false
}

func jobDepsCacheKeyString(key jobDepsCacheKey) string {
	return fmt.Sprintf("%s\x00%d\x00%s", key.JobID, key.Limit, key.Cursor)
}

func parseJobDepsCacheKey(raw string) (jobDepsCacheKey, bool) {
	parts := strings.Split(raw, "\x00")
	if len(parts) != 3 {
		return jobDepsCacheKey{}, false
	}
	limit, err := strconv.Atoi(parts[1])
	if err != nil {
		return jobDepsCacheKey{}, false
	}
	return jobDepsCacheKey{JobID: parts[0], Limit: limit, Cursor: parts[2]}, true
}

func jobDependenciesCacheVersion(deps []domain.JobDependency) int64 {
	var version int64
	for _, dep := range deps {
		if dep.CacheVersion > version {
			version = dep.CacheVersion
		}
	}
	return version
}
