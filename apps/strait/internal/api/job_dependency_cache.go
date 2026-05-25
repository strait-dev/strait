package api

import (
	"context"
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
}

func newJobDependencyCache(ttl time.Duration) *jobDependencyCache {
	if ttl <= 0 {
		return nil
	}
	return &jobDependencyCache{tier: straitcache.NewTier[jobDepsCacheKey, []domain.JobDependency](straitcache.TierConfig[jobDepsCacheKey, []domain.JobDependency]{
		Name:          "api_job_dependencies",
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
		DisableL2: true,
		Clone: func(deps []domain.JobDependency) []domain.JobDependency {
			return append([]domain.JobDependency(nil), deps...)
		},
	})}
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
	c.tier.Invalidate(ctx, jobDepsCacheKey{JobID: jobID, Limit: 1000})
}
