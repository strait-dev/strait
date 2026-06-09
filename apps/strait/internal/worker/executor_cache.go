package worker

import (
	"context"
	"strconv"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/store"

	"github.com/redis/go-redis/v9"
)

type jobHealthKey struct {
	JobID  string
	Bucket int64
}

type workerCacheDeps struct {
	Redis    redis.Cmdable
	Bus      *straitcache.Bus
	Registry *straitcache.Registry
}

const workerJobHealthCacheNamespace = "worker_job_health"

type tierJobHealthCache struct {
	tier *straitcache.Tier[jobHealthKey, *store.JobHealthStats]
	ttl  time.Duration
}

func newTierJobHealthCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierJobHealthCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	l2 := newWorkerJobHealthL2(deps.Redis)
	tierConfig := workerJobHealthTierConfig(ttl, l2)
	return &tierJobHealthCache{
		ttl:  ttl,
		tier: straitcache.NewTier[jobHealthKey, *store.JobHealthStats](tierConfig),
	}
}

func workerJobHealthTierConfig(
	ttl time.Duration,
	l2 straitcache.L2[jobHealthKey, *store.JobHealthStats],
) straitcache.TierConfig[jobHealthKey, *store.JobHealthStats] {
	return straitcache.TierConfig[jobHealthKey, *store.JobHealthStats]{
		Name:        workerJobHealthCacheNamespace,
		L2:          l2,
		Consistency: straitcache.BoundedStaleness,
		MaximumSize: 20_000,
		TTL:         ttl,
		TTLJitter:   0.05,
		DisableL2:   l2 == nil,
		Clone:       cloneJobHealthStats,
	}
}

func cloneJobHealthStats(v *store.JobHealthStats) *store.JobHealthStats {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func newWorkerJobHealthL2(redis redis.Cmdable) straitcache.L2[jobHealthKey, *store.JobHealthStats] {
	if redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[jobHealthKey, *store.JobHealthStats](
		straitcache.RedisL2Config[jobHealthKey, *store.JobHealthStats]{
			Client:    redis,
			Namespace: workerJobHealthCacheNamespace,
			Key:       workerJobHealthKeyString,
		},
	)
}

func workerJobHealthKeyString(key jobHealthKey) string {
	const maxInt64Digits = 20
	const sepLen = 1
	size := len(key.JobID) + sepLen + maxInt64Digits
	if size <= 64 {
		var buf [64]byte
		out := append(buf[:0], key.JobID...)
		out = append(out, 0)
		out = strconv.AppendInt(out, key.Bucket, 10)
		return string(out)
	}
	out := make([]byte, 0, size)
	out = append(out, key.JobID...)
	out = append(out, 0)
	out = strconv.AppendInt(out, key.Bucket, 10)
	return string(out)
}

func (c *tierJobHealthCache) Key(jobID string, now time.Time) jobHealthKey {
	bucketSecs := int64(c.ttl.Seconds())
	if bucketSecs <= 0 {
		bucketSecs = 1
	}
	return jobHealthKey{JobID: jobID, Bucket: now.Unix() / bucketSecs}
}

func (c *tierJobHealthCache) Load(
	ctx context.Context,
	key jobHealthKey,
	loader straitcache.LoadFunc[jobHealthKey, *store.JobHealthStats],
) (*store.JobHealthStats, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}
