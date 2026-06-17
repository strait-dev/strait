package worker

import (
	"context"
	"strconv"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"

	"github.com/redis/go-redis/v9"
)

type workflowStepsVersionKey struct {
	WorkflowID string
	Version    int
}

type workflowRunVersion struct {
	WorkflowID string
	Version    int
}

const (
	workerWorkflowRunVersionCacheNamespace = "worker_workflow_run_version"
	workerWorkflowStepsCacheNamespace      = "worker_workflow_steps_version"
)

type tierWorkflowRunVersionCache struct {
	tier *straitcache.Tier[string, workflowRunVersion]
}

func newTierWorkflowRunVersionCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierWorkflowRunVersionCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	var l2 straitcache.L2[string, workflowRunVersion]
	if deps.Redis != nil {
		l2 = straitcache.NewRedisL2[string, workflowRunVersion](straitcache.RedisL2Config[string, workflowRunVersion]{
			Client:    deps.Redis,
			Namespace: workerWorkflowRunVersionCacheNamespace,
		})
	}
	tier := straitcache.NewTier[string, workflowRunVersion](straitcache.TierConfig[string, workflowRunVersion]{
		Name:        workerWorkflowRunVersionCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Immutable,
		MaximumSize: 100_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL2:   l2 == nil,
	})
	return &tierWorkflowRunVersionCache{tier: tier}
}

func (c *tierWorkflowRunVersionCache) Load(
	ctx context.Context,
	key string,
	loader straitcache.LoadFunc[string, workflowRunVersion],
) (workflowRunVersion, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}

type tierWorkflowStepsVersionCache struct {
	tier *straitcache.Tier[workflowStepsVersionKey, []domain.WorkflowStep]
}

func newTierWorkflowStepsVersionCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierWorkflowStepsVersionCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	l2 := newWorkerWorkflowStepsL2(deps.Redis)
	tierConfig := workerWorkflowStepsTierConfig(ttl, l2)
	tier := straitcache.NewTier[workflowStepsVersionKey, []domain.WorkflowStep](tierConfig)
	return &tierWorkflowStepsVersionCache{tier: tier}
}

func workerWorkflowStepsTierConfig(
	ttl time.Duration,
	l2 straitcache.L2[workflowStepsVersionKey, []domain.WorkflowStep],
) straitcache.TierConfig[workflowStepsVersionKey, []domain.WorkflowStep] {
	disableL2 := l2 == nil
	return straitcache.TierConfig[workflowStepsVersionKey, []domain.WorkflowStep]{
		Name:          workerWorkflowStepsCacheNamespace,
		L2:            l2,
		Consistency:   straitcache.Immutable,
		MaximumWeight: 100_000,
		Weigher: func(_ workflowStepsVersionKey, steps []domain.WorkflowStep) uint32 {
			if len(steps) == 0 {
				return 1
			}
			if len(steps) > 100_000 {
				return 100_000
			}
			return uint32(len(steps)) // #nosec G115 -- bounded above before conversion.
		},
		TTL:       ttl,
		TTLJitter: 0.1,
		DisableL2: disableL2,
		Clone:     domain.CloneWorkflowSteps,
	}
}

func newWorkerWorkflowStepsL2(redis redis.Cmdable) straitcache.L2[workflowStepsVersionKey, []domain.WorkflowStep] {
	if redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[workflowStepsVersionKey, []domain.WorkflowStep](
		straitcache.RedisL2Config[workflowStepsVersionKey, []domain.WorkflowStep]{
			Client:    redis,
			Namespace: workerWorkflowStepsCacheNamespace,
			Key:       workerWorkflowStepsKeyString,
		},
	)
}

func workerWorkflowStepsKeyString(key workflowStepsVersionKey) string {
	const maxIntDigits = 20
	const sepLen = 1
	size := len(key.WorkflowID) + sepLen + maxIntDigits
	if size <= 64 {
		var buf [64]byte
		out := append(buf[:0], key.WorkflowID...)
		out = append(out, 0)
		out = strconv.AppendInt(out, int64(key.Version), 10)
		return string(out)
	}
	out := make([]byte, 0, size)
	out = append(out, key.WorkflowID...)
	out = append(out, 0)
	out = strconv.AppendInt(out, int64(key.Version), 10)
	return string(out)
}

func (c *tierWorkflowStepsVersionCache) Load(
	ctx context.Context,
	key workflowStepsVersionKey,
	loader straitcache.LoadFunc[workflowStepsVersionKey, []domain.WorkflowStep],
) ([]domain.WorkflowStep, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}
