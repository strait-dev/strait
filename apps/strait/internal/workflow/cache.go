package workflow

import (
	"context"
	"strconv"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"

	"github.com/redis/go-redis/v9"
)

const workflowStepsVersionCacheNamespace = "worker_workflow_steps_version"

type WorkflowDefinitionCacheConfig struct {
	Redis      redis.Cmdable
	VersionTTL time.Duration
}

type workflowStepsVersionKey struct {
	WorkflowID string
	Version    int
}

type workflowStepsVersionCache struct {
	tier *straitcache.Tier[workflowStepsVersionKey, []domain.WorkflowStep]
}

func newWorkflowStepsVersionCache(cfg WorkflowDefinitionCacheConfig) *workflowStepsVersionCache {
	if cfg.VersionTTL <= 0 {
		return nil
	}
	l2 := newWorkflowStepsVersionL2(cfg.Redis)
	tierConfig := workflowStepsVersionTierConfig(cfg.VersionTTL, l2)
	tier := straitcache.NewTier[workflowStepsVersionKey, []domain.WorkflowStep](tierConfig)
	return &workflowStepsVersionCache{tier: tier}
}

func workflowStepsVersionTierConfig(
	ttl time.Duration,
	l2 straitcache.L2[workflowStepsVersionKey, []domain.WorkflowStep],
) straitcache.TierConfig[workflowStepsVersionKey, []domain.WorkflowStep] {
	disableL2 := l2 == nil
	return straitcache.TierConfig[workflowStepsVersionKey, []domain.WorkflowStep]{
		Name:          workflowStepsVersionCacheNamespace,
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

func newWorkflowStepsVersionL2(redis redis.Cmdable) straitcache.L2[workflowStepsVersionKey, []domain.WorkflowStep] {
	if redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[workflowStepsVersionKey, []domain.WorkflowStep](
		straitcache.RedisL2Config[workflowStepsVersionKey, []domain.WorkflowStep]{
			Client:    redis,
			Namespace: workflowStepsVersionCacheNamespace,
			Key:       workflowStepsVersionKeyString,
		},
	)
}

func workflowStepsVersionKeyString(key workflowStepsVersionKey) string {
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

func (c *workflowStepsVersionCache) Load(
	ctx context.Context,
	key workflowStepsVersionKey,
	loader straitcache.LoadFunc[workflowStepsVersionKey, []domain.WorkflowStep],
) ([]domain.WorkflowStep, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}
