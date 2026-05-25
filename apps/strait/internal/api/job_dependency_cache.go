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
			return uint32(len(deps))
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

func (c *jobDependencyCache) List(ctx context.Context, key jobDepsCacheKey, loader func(context.Context, jobDepsCacheKey) ([]domain.JobDependency, error)) ([]domain.JobDependency, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}

func (c *jobDependencyCache) InvalidateJob(ctx context.Context, jobID string) {
	if c == nil || c.tier == nil || jobID == "" {
		return
	}
	key := jobDepsCacheKey{JobID: jobID, Limit: 1000}
	_ = c.tier.InvalidateThrough(ctx, key, c.bus, jobDependencyCacheNamespace, jobDepsCacheKeyString(key), time.Now().UnixNano())
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
