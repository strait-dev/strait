package worker

import (
	"context"
	"fmt"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"

	"github.com/redis/go-redis/v9"
)

type jobVersionKey struct {
	JobID   string
	Version int
}

type executorJobCache interface {
	Get(context.Context, string) (*domain.Job, error)
	Load(context.Context, string, straitcache.LoadFunc[string, *domain.Job]) (*domain.Job, error)
	Set(context.Context, string, *domain.Job) error
	Delete(context.Context, string) error
}

type executorVersionedJobCache interface {
	Load(context.Context, jobVersionKey, straitcache.LoadFunc[jobVersionKey, *domain.Job]) (*domain.Job, error)
}

type tierJobCache struct {
	tier *straitcache.Tier[string, *domain.Job]
	bus  *straitcache.Bus
}

const (
	workerJobCacheNamespace        = "worker_job"
	workerJobVersionCacheNamespace = "worker_job_version"
)

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

func workerCachePolicy(namespace string) straitcache.StrongNamespacePolicy {
	return straitcache.StrongNamespacePolicy{Namespace: namespace}
}

func workerCacheBarrier(version int64) straitcache.VersionBarrier {
	return straitcache.VersionBarrier{Version: version}
}

func jobCacheVersion(job *domain.Job) int64 {
	if job == nil {
		return 0
	}
	if job.CacheVersion > 0 {
		return job.CacheVersion
	}
	if !job.UpdatedAt.IsZero() {
		return job.UpdatedAt.UnixNano()
	}
	if job.Version > 0 {
		return int64(job.Version)
	}
	return 1
}

type tierVersionedJobCache struct {
	tier *straitcache.Tier[jobVersionKey, *domain.Job]
}

func newTierVersionedJobCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierVersionedJobCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	l2 := newWorkerJobVersionL2(deps.Redis)
	tier := straitcache.NewTier[jobVersionKey, *domain.Job](straitcache.TierConfig[jobVersionKey, *domain.Job]{
		Name:        workerJobVersionCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Immutable,
		MaximumSize: 10_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL2:   l2 == nil,
		Clone:       cloneJob,
	})
	return &tierVersionedJobCache{tier: tier}
}

func newWorkerJobVersionL2(redis redis.Cmdable) straitcache.L2[jobVersionKey, *domain.Job] {
	if redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[jobVersionKey, *domain.Job](
		straitcache.RedisL2Config[jobVersionKey, *domain.Job]{
			Client:    redis,
			Namespace: workerJobVersionCacheNamespace,
			Key:       workerJobVersionKeyString,
		},
	)
}

func workerJobVersionKeyString(key jobVersionKey) string {
	return fmt.Sprintf("%s\x00%d", key.JobID, key.Version)
}

func (c *tierVersionedJobCache) Load(
	ctx context.Context,
	key jobVersionKey,
	loader straitcache.LoadFunc[jobVersionKey, *domain.Job],
) (*domain.Job, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}
