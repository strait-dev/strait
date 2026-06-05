package worker

import (
	"context"
	"fmt"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/redis/go-redis/v9"
)

type jobVersionKey struct {
	JobID   string
	Version int
}

type workflowStepsVersionKey struct {
	WorkflowID string
	Version    int
}

type workflowRunVersion struct {
	WorkflowID string
	Version    int
}

type jobHealthKey struct {
	JobID  string
	Bucket int64
}

type workerCacheDeps struct {
	Redis    redis.Cmdable
	Bus      *straitcache.Bus
	Registry *straitcache.Registry
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
	workerJobCacheNamespace                = "worker_job"
	workerJobVersionCacheNamespace         = "worker_job_version"
	workerWorkflowRunVersionCacheNamespace = "worker_workflow_run_version"
	workerWorkflowStepsCacheNamespace      = "worker_workflow_steps_version"
	workerJobHealthCacheNamespace          = "worker_job_health"
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
		DisableL2: l2 == nil,
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
	return fmt.Sprintf("%s\x00%d", key.WorkflowID, key.Version)
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
	return fmt.Sprintf("%s\x00%d", key.JobID, key.Bucket)
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
