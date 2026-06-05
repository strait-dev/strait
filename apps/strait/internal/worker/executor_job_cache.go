package worker

import (
	"context"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

type executorJobCache interface {
	Get(context.Context, string) (*domain.Job, error)
	Load(context.Context, string, straitcache.LoadFunc[string, *domain.Job]) (*domain.Job, error)
	Set(context.Context, string, *domain.Job) error
	Delete(context.Context, string) error
}

type tierJobCache struct {
	tier *straitcache.Tier[string, *domain.Job]
	bus  *straitcache.Bus
}

const workerJobCacheNamespace = "worker_job"

func newTierJobCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierJobCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	var l2 straitcache.L2[string, *domain.Job]
	if deps.Redis != nil {
		l2 = straitcache.NewRedisL2[string, *domain.Job](straitcache.RedisL2Config[string, *domain.Job]{
			Client:    deps.Redis,
			Namespace: workerJobCacheNamespace,
		})
	}
	c := &tierJobCache{bus: deps.Bus}
	c.tier = straitcache.NewTier[string, *domain.Job](straitcache.TierConfig[string, *domain.Job]{
		Name:        workerJobCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Strong,
		MaximumSize: 10_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL1:   l2 != nil,
		DisableL2:   l2 == nil,
		Clone:       cloneJob,
	})
	if deps.Registry != nil {
		deps.Registry.Register(workerJobCacheNamespace, straitcache.UpdatingStringTierHandler[*domain.Job]{Tier: c.tier})
	}
	return c
}

func (c *tierJobCache) Get(ctx context.Context, key string) (*domain.Job, error) {
	if c == nil || c.tier == nil {
		return nil, straitcache.ErrCacheMiss
	}
	return c.tier.Get(ctx, key, nil)
}

func (c *tierJobCache) Load(
	ctx context.Context,
	key string,
	loader straitcache.LoadFunc[string, *domain.Job],
) (*domain.Job, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	versionedLoader := func(loadCtx context.Context, loadKey string) (straitcache.Versioned[*domain.Job], error) {
		job, err := loader(loadCtx, loadKey)
		if err != nil {
			return straitcache.Versioned[*domain.Job]{}, err
		}
		return straitcache.Versioned[*domain.Job]{Value: job, Version: jobCacheVersion(job)}, nil
	}
	got, err := c.tier.GetConsistentVersioned(ctx, key, 0, versionedLoader)
	if err != nil {
		return nil, err
	}
	return got.Value, nil
}

func (c *tierJobCache) Set(ctx context.Context, key string, job *domain.Job) error {
	if c == nil || c.tier == nil {
		return nil
	}
	_, err := c.tier.StrongWriteThrough(
		ctx,
		workerCachePolicy(workerJobCacheNamespace),
		key,
		key,
		job,
		jobCacheVersion(job),
		c.bus,
	)
	return err
}

func (c *tierJobCache) Delete(ctx context.Context, key string) error {
	if c == nil || c.tier == nil {
		return nil
	}
	return c.tier.StrongInvalidate(
		ctx,
		workerCachePolicy(workerJobCacheNamespace),
		key,
		key,
		workerCacheBarrier(time.Now().UnixNano()),
		c.bus,
	)
}
