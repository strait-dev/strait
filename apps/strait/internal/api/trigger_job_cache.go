package api

import (
	"context"
	"encoding/json"
	"maps"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"
)

const apiTriggerJobCacheNamespace = "api_trigger_job"

type triggerJobCache struct {
	tier *straitcache.Tier[string, *domain.Job]
	bus  *straitcache.Bus
}

func newTriggerJobCache(ttl time.Duration, deps ...apiCacheDeps) *triggerJobCache {
	if ttl <= 0 {
		return nil
	}
	var dep apiCacheDeps
	if len(deps) > 0 {
		dep = deps[0]
	}
	var l2 straitcache.L2[string, *domain.Job]
	if dep.Redis != nil {
		l2 = straitcache.NewRedisL2[string, *domain.Job](straitcache.RedisL2Config[string, *domain.Job]{
			Client:    dep.Redis,
			Namespace: apiTriggerJobCacheNamespace,
		})
	}
	c := &triggerJobCache{bus: dep.Bus}
	c.tier = straitcache.NewTier[string, *domain.Job](straitcache.TierConfig[string, *domain.Job]{
		Name:        apiTriggerJobCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Strong,
		MaximumSize: 10_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL1:   l2 != nil,
		DisableL2:   l2 == nil,
		Clone:       cloneTriggerJob,
		Sanitize:    cloneTriggerJob,
	})
	if dep.Registry != nil {
		dep.Registry.Register(apiTriggerJobCacheNamespace, straitcache.UpdatingStringTierHandler[*domain.Job]{Tier: c.tier})
	}
	return c
}

func (c *triggerJobCache) Stop() {
	if c == nil || c.tier == nil {
		return
	}
	c.tier.Stop()
}

func (c *triggerJobCache) Get(
	ctx context.Context,
	jobID string,
	loader func(context.Context, string) (*domain.Job, error),
) (*domain.Job, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, jobID)
	}
	versionedLoader := func(loadCtx context.Context, loadJobID string) (straitcache.Versioned[*domain.Job], error) {
		job, err := loader(loadCtx, loadJobID)
		if err != nil {
			return straitcache.Versioned[*domain.Job]{}, err
		}
		return straitcache.Versioned[*domain.Job]{
			Value:   job,
			Version: triggerJobCacheVersion(job),
		}, nil
	}
	got, err := c.tier.GetConsistentVersioned(ctx, jobID, 0, versionedLoader)
	if err != nil {
		return nil, err
	}
	if got.Value == nil {
		return nil, store.ErrJobNotFound
	}
	return got.Value, nil
}

func (c *triggerJobCache) Invalidate(ctx context.Context, jobID string, version int64) {
	if c == nil || c.tier == nil || jobID == "" {
		return
	}
	if version <= 0 {
		version = time.Now().UnixNano()
	}
	_ = c.tier.StrongInvalidate(
		ctx,
		strongCachePolicy(apiTriggerJobCacheNamespace),
		jobID,
		jobID,
		cacheVersionBarrier(version),
		c.bus,
	)
}

func triggerJobCacheVersion(job *domain.Job) int64 {
	if job == nil {
		return 0
	}
	return job.CacheVersion
}

func cloneTriggerJob(job *domain.Job) *domain.Job {
	if job == nil {
		return nil
	}
	cloned := *job
	if job.Tags != nil {
		cloned.Tags = maps.Clone(job.Tags)
	}
	if job.DefaultRunMetadata != nil {
		cloned.DefaultRunMetadata = maps.Clone(job.DefaultRunMetadata)
	}
	if job.RetryDelaysSecs != nil {
		cloned.RetryDelaysSecs = append([]int(nil), job.RetryDelaysSecs...)
	}
	if job.RateLimitKeys != nil {
		cloned.RateLimitKeys = append([]domain.RateLimitKey(nil), job.RateLimitKeys...)
	}
	if job.PreferredRegions != nil {
		cloned.PreferredRegions = append([]string(nil), job.PreferredRegions...)
	}
	if job.PayloadSchema != nil {
		cloned.PayloadSchema = append(json.RawMessage(nil), job.PayloadSchema...)
	}
	if job.ResultSchema != nil {
		cloned.ResultSchema = append(json.RawMessage(nil), job.ResultSchema...)
	}
	if job.OnCompletePayloadMapping != nil {
		cloned.OnCompletePayloadMapping = append(json.RawMessage(nil), job.OnCompletePayloadMapping...)
	}
	if job.OnFailurePayloadMapping != nil {
		cloned.OnFailurePayloadMapping = append(json.RawMessage(nil), job.OnFailurePayloadMapping...)
	}
	return &cloned
}
