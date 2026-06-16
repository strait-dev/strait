package api

import (
	"context"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/config"
	"strait/internal/domain"
)

const apiJobCacheNamespace = "api_job"

type apiJobCache struct {
	tier   *straitcache.Tier[string, *domain.Job]
	loadFn func(context.Context, string) (*domain.Job, error)
	bus    *straitcache.Bus
}

func newAPIJobCache(
	ttl time.Duration,
	loadFn func(context.Context, string) (*domain.Job, error),
	deps ...apiCacheDeps,
) *apiJobCache {
	if ttl <= 0 || loadFn == nil {
		return nil
	}
	var dep apiCacheDeps
	if len(deps) > 0 {
		dep = deps[0]
	}
	l2 := newAPIJobCacheL2(dep)
	c := &apiJobCache{loadFn: loadFn, bus: dep.Bus}
	c.tier = straitcache.NewTier[string, *domain.Job](apiJobTierConfig(ttl, l2))
	if dep.Registry != nil {
		dep.Registry.Register(apiJobCacheNamespace, straitcache.UpdatingStringTierHandler[*domain.Job]{Tier: c.tier})
	}
	return c
}

func newAPIJobCacheL2(dep apiCacheDeps) straitcache.L2[string, *domain.Job] {
	if dep.Redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[string, *domain.Job](
		straitcache.RedisL2Config[string, *domain.Job]{
			Client:    dep.Redis,
			Namespace: apiJobCacheNamespace,
		},
	)
}

func apiJobTierConfig(ttl time.Duration, l2 straitcache.L2[string, *domain.Job]) straitcache.TierConfig[string, *domain.Job] {
	return straitcache.TierConfig[string, *domain.Job]{
		Name:        apiJobCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Strong,
		MaximumSize: 20_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL2:   l2 == nil,
		Clone:       cloneAPIJob,
	}
}

func (c *apiJobCache) Stop() {
	if c == nil || c.tier == nil {
		return
	}
	c.tier.Stop()
}

func (c *apiJobCache) Get(ctx context.Context, jobID string) (*domain.Job, error) {
	if !c.cacheEnabled() {
		return c.loadFn(ctx, jobID)
	}
	loaded, err := c.tier.GetConsistentVersioned(ctx, jobID, 0, func(loadCtx context.Context, key string) (straitcache.Versioned[*domain.Job], error) {
		job, loadErr := c.loadFn(loadCtx, key)
		if loadErr != nil {
			return straitcache.Versioned[*domain.Job]{}, loadErr
		}
		return straitcache.Versioned[*domain.Job]{Value: job, Version: apiJobCacheVersion(job)}, nil
	})
	if err != nil {
		return nil, err
	}
	return loaded.Value, nil
}

func (c *apiJobCache) Invalidate(ctx context.Context, jobID string, version int64) {
	if !c.cacheEnabled() || jobID == "" {
		return
	}
	if version <= 0 {
		version = time.Now().UnixNano()
	}
	if ctx == nil {
		ctx = cacheMetricsContext
	}
	_ = c.tier.StrongInvalidate(
		ctx,
		strongCachePolicy(apiJobCacheNamespace),
		jobID,
		jobID,
		cacheVersionBarrier(version),
		c.bus,
	)
}

func (c *apiJobCache) cacheEnabled() bool {
	return c != nil && c.tier != nil && c.loadFn != nil
}

func apiJobCacheTTL(cfg *config.Config) time.Duration {
	if cfg != nil {
		return cfg.JobCacheTTL
	}
	return 5 * time.Minute
}

func apiJobCacheVersion(job *domain.Job) int64 {
	if job == nil || job.CacheVersion <= 0 {
		return 1
	}
	return job.CacheVersion
}

func cloneAPIJob(job *domain.Job) *domain.Job {
	if job == nil {
		return nil
	}
	clone := *job
	if job.PayloadSchema != nil {
		clone.PayloadSchema = append([]byte(nil), job.PayloadSchema...)
	}
	if job.Tags != nil {
		clone.Tags = make(map[string]string, len(job.Tags))
		for k, v := range job.Tags {
			clone.Tags[k] = v
		}
	}
	if job.RateLimitKeys != nil {
		clone.RateLimitKeys = append([]domain.RateLimitKey(nil), job.RateLimitKeys...)
	}
	if job.RetryDelaysSecs != nil {
		clone.RetryDelaysSecs = append([]int(nil), job.RetryDelaysSecs...)
	}
	if job.DefaultRunMetadata != nil {
		clone.DefaultRunMetadata = make(map[string]string, len(job.DefaultRunMetadata))
		for k, v := range job.DefaultRunMetadata {
			clone.DefaultRunMetadata[k] = v
		}
	}
	if job.ResultSchema != nil {
		clone.ResultSchema = append([]byte(nil), job.ResultSchema...)
	}
	if job.PreferredRegions != nil {
		clone.PreferredRegions = append([]string(nil), job.PreferredRegions...)
	}
	if job.OnCompletePayloadMapping != nil {
		clone.OnCompletePayloadMapping = append([]byte(nil), job.OnCompletePayloadMapping...)
	}
	if job.OnFailurePayloadMapping != nil {
		clone.OnFailurePayloadMapping = append([]byte(nil), job.OnFailurePayloadMapping...)
	}
	return &clone
}
