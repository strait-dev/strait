package workflow

import (
	"context"
	"fmt"
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
	var l2 straitcache.L2[workflowStepsVersionKey, []domain.WorkflowStep]
	if cfg.Redis != nil {
		l2 = straitcache.NewRedisL2[workflowStepsVersionKey, []domain.WorkflowStep](straitcache.RedisL2Config[workflowStepsVersionKey, []domain.WorkflowStep]{
			Client:    cfg.Redis,
			Namespace: workflowStepsVersionCacheNamespace,
			Key: func(key workflowStepsVersionKey) string {
				return fmt.Sprintf("%s\x00%d", key.WorkflowID, key.Version)
			},
		})
	}
	return &workflowStepsVersionCache{tier: straitcache.NewTier[workflowStepsVersionKey, []domain.WorkflowStep](straitcache.TierConfig[workflowStepsVersionKey, []domain.WorkflowStep]{
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
		TTL:       cfg.VersionTTL,
		TTLJitter: 0.1,
		DisableL2: l2 == nil,
		Clone:     cloneWorkflowSteps,
	})}
}

func (c *workflowStepsVersionCache) Load(ctx context.Context, key workflowStepsVersionKey, loader straitcache.LoadFunc[workflowStepsVersionKey, []domain.WorkflowStep]) ([]domain.WorkflowStep, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}

func cloneWorkflowSteps(steps []domain.WorkflowStep) []domain.WorkflowStep {
	if steps == nil {
		return nil
	}
	out := make([]domain.WorkflowStep, len(steps))
	for i := range steps {
		out[i] = steps[i]
		out[i].DependsOn = append([]string(nil), steps[i].DependsOn...)
		out[i].Condition = append([]byte(nil), steps[i].Condition...)
		out[i].Payload = append([]byte(nil), steps[i].Payload...)
		out[i].ApprovalApprovers = append([]string(nil), steps[i].ApprovalApprovers...)
		out[i].StageNotifications = append([]byte(nil), steps[i].StageNotifications...)
	}
	return out
}
