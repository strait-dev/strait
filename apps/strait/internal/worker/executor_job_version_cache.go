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

type executorVersionedJobCache interface {
	Load(context.Context, jobVersionKey, straitcache.LoadFunc[jobVersionKey, *domain.Job]) (*domain.Job, error)
}

const workerJobVersionCacheNamespace = "worker_job_version"

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
