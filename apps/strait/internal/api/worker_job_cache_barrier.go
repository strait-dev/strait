package api

import (
	"context"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/config"

	"github.com/redis/go-redis/v9"
)

const workerJobCacheNamespace = "worker_job"

func newWorkerJobBarrier(ttl time.Duration, rdb redis.Cmdable) *straitcache.Tier[string, struct{}] {
	if ttl <= 0 || rdb == nil {
		return nil
	}
	l2 := straitcache.NewRedisL2[string, struct{}](straitcache.RedisL2Config[string, struct{}]{
		Client:    rdb,
		Namespace: workerJobCacheNamespace,
	})
	if l2 == nil {
		return nil
	}
	return straitcache.NewTier[string, struct{}](straitcache.TierConfig[string, struct{}]{
		Name:        workerJobCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Strong,
		TTL:         ttl,
		DisableL1:   true,
	})
}

func workerJobBarrierTTL(cfg *config.Config) time.Duration {
	if cfg == nil {
		return 0
	}
	return cfg.JobCacheTTL
}

func (s *Server) invalidateWorkerJobCache(ctx context.Context, jobID string, version int64) {
	if s == nil || jobID == "" {
		return
	}
	if version <= 0 {
		version = time.Now().UnixNano()
	}
	if s.workerJobBarrier != nil {
		_ = s.workerJobBarrier.StrongInvalidate(
			ctx,
			strongCachePolicy(workerJobCacheNamespace),
			jobID,
			jobID,
			cacheVersionBarrier(version),
			s.cacheBus,
		)
		return
	}
	if s.cacheBus != nil {
		_ = s.cacheBus.PublishInvalidate(ctx, workerJobCacheNamespace, jobID, version)
	}
}
