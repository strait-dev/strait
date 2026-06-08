package api

import (
	"context"
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
	l2 := newJobDependencyCacheL2(dep)
	c := &jobDependencyCache{bus: dep.Bus}
	c.tier = straitcache.NewTier[jobDepsCacheKey, []domain.JobDependency](jobDependencyTierConfig(ttl, l2))
	if dep.Registry != nil {
		dep.Registry.Register(jobDependencyCacheNamespace, straitcache.TierHandler[jobDepsCacheKey, []domain.JobDependency]{
			Tier:  c.tier,
			Parse: parseJobDepsCacheKey,
		})
	}
	return c
}

func (c *jobDependencyCache) Stop() {
	if c == nil || c.tier == nil {
		return
	}
	c.tier.Stop()
}

func newJobDependencyCacheL2(dep apiCacheDeps) straitcache.L2[jobDepsCacheKey, []domain.JobDependency] {
	if dep.Redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[jobDepsCacheKey, []domain.JobDependency](
		straitcache.RedisL2Config[jobDepsCacheKey, []domain.JobDependency]{
			Client:    dep.Redis,
			Namespace: jobDependencyCacheNamespace,
			Key:       jobDepsCacheKeyString,
		},
	)
}

func jobDependencyTierConfig(
	ttl time.Duration,
	l2 straitcache.L2[jobDepsCacheKey, []domain.JobDependency],
) straitcache.TierConfig[jobDepsCacheKey, []domain.JobDependency] {
	return straitcache.TierConfig[jobDepsCacheKey, []domain.JobDependency]{
		Name:          jobDependencyCacheNamespace,
		L2:            l2,
		Consistency:   straitcache.Strong,
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
		DisableL1: l2 != nil,
		DisableL2: l2 == nil,
		Clone: func(deps []domain.JobDependency) []domain.JobDependency {
			return append([]domain.JobDependency(nil), deps...)
		},
	}
}

func (c *jobDependencyCache) List(
	ctx context.Context,
	key jobDepsCacheKey,
	loader func(context.Context, jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error),
) ([]domain.JobDependency, error) {
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
	c.InvalidateJobWithVersion(ctx, jobID, time.Now().UnixNano())
}

func (c *jobDependencyCache) InvalidateJobWithVersion(ctx context.Context, jobID string, version int64) {
	if c == nil || c.tier == nil || jobID == "" {
		return
	}
	for _, limit := range jobDependencyCachedPageLimits {
		key := jobDepsCacheKey{JobID: jobID, Limit: limit}
		_ = c.tier.StrongInvalidate(
			ctx,
			strongCachePolicy(jobDependencyCacheNamespace),
			key,
			jobDepsCacheKeyString(key),
			cacheVersionBarrier(version),
			c.bus,
		)
	}
}

func (c *jobDependencyCache) RefreshJob(
	ctx context.Context,
	jobID string,
	loader func(context.Context, jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error),
) {
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
		_, _ = c.tier.StrongWriteThrough(
			ctx,
			strongCachePolicy(jobDependencyCacheNamespace),
			key,
			jobDepsCacheKeyString(key),
			loaded.Value,
			version,
			c.bus,
		)
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
	const maxIntDigits = 20
	const sepCount = 2
	size := len(key.JobID) + sepCount + maxIntDigits + len(key.Cursor)
	if size <= 96 {
		var buf [96]byte
		out := append(buf[:0], key.JobID...)
		out = append(out, 0)
		out = strconv.AppendInt(out, int64(key.Limit), 10)
		out = append(out, 0)
		out = append(out, key.Cursor...)
		return string(out)
	}
	out := make([]byte, 0, size)
	out = append(out, key.JobID...)
	out = append(out, 0)
	out = strconv.AppendInt(out, int64(key.Limit), 10)
	out = append(out, 0)
	out = append(out, key.Cursor...)
	return string(out)
}

func parseJobDepsCacheKey(raw string) (jobDepsCacheKey, bool) {
	jobID, rest, ok := strings.Cut(raw, "\x00")
	if !ok {
		return jobDepsCacheKey{}, false
	}
	limitRaw, cursor, ok := strings.Cut(rest, "\x00")
	if !ok || strings.Contains(cursor, "\x00") {
		return jobDepsCacheKey{}, false
	}
	limit, err := strconv.Atoi(limitRaw)
	if err != nil {
		return jobDepsCacheKey{}, false
	}
	return jobDepsCacheKey{JobID: jobID, Limit: limit, Cursor: cursor}, true
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
