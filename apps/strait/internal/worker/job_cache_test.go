package worker

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	orcstore "strait/internal/store"
)

func newTestJobCache(t *testing.T, ttl time.Duration) executorJobCache {
	t.Helper()
	return newTierJobCache(ttl)
}

func TestJobCache_HitAvoidsDatabaseLookup(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(t, 5*time.Second)
	ctx := t.Context()

	job := &domain.Job{
		ID:      "job-1",
		Version: 1,
		Name:    "test-job",
	}
	require.NoError(
		t, jobCache.Set(ctx,
			"job-1", job,
		))

	cached, err := jobCache.Get(ctx, "job-1")
	require.NoError(
		t, err)
	require.False(t,
		cached.ID != "job-1" ||
			cached.
				Version !=
				1)
}

func TestJobCache_MissReturnsError(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(t, 5*time.Second)
	ctx := t.Context()

	_, err := jobCache.Get(ctx, "nonexistent")
	require.Error(t,
		err)
}

func TestJobCache_ExpiresAfterTTL(t *testing.T) {
	t.Parallel()

	// Otter uses a timer wheel with ~1s granularity, so TTL must be >= 1s.
	jobCache := newTestJobCache(t, 1*time.Second)
	ctx := t.Context()

	job := &domain.Job{ID: "job-ttl", Version: 1}
	require.NoError(
		t, jobCache.Set(ctx,
			"job-ttl",
			job))

	time.Sleep(3 * time.Second)

	_, err := jobCache.Get(ctx, "job-ttl")
	require.Error(t,
		err)
}

func TestJobCache_OverwriteUpdatesValue(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(t, 5*time.Second)
	ctx := t.Context()

	v1 := &domain.Job{ID: "job-ow", Version: 1, Name: "old-name"}
	v2 := &domain.Job{ID: "job-ow", Version: 2, Name: "new-name"}

	_ = jobCache.Set(ctx, "job-ow", v1)
	_ = jobCache.Set(ctx, "job-ow", v2)

	cached, err := jobCache.Get(ctx, "job-ow")
	require.NoError(
		t, err)
	require.False(t,
		cached.Version !=
			2 || cached.Name !=
			"new-name",
	)
}

func TestJobCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(t, 5*time.Second)
	ctx := t.Context()

	const goroutines = 50
	var wg conc.WaitGroup

	for i := range goroutines {
		wg.Go(func() {
			job := &domain.Job{ID: "job-conc", Version: i}
			_ = jobCache.Set(ctx, "job-conc", job)
		})
	}

	for range goroutines {
		wg.Go(func() {
			_, _ = jobCache.Get(ctx, "job-conc")
		})
	}

	wg.Wait()

	// Should not panic or race. Final value should be one of the written versions.
	cached, err := jobCache.Get(ctx, "job-conc")
	require.NoError(
		t, err)
	require.Equal(t,
		"job-conc", cached.
			ID)
}

func TestJobCache_Delete(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(t, 5*time.Second)
	ctx := t.Context()

	job := &domain.Job{ID: "job-del", Version: 1}
	_ = jobCache.Set(ctx, "job-del", job)
	require.NoError(
		t, jobCache.Delete(ctx, "job-del"))

	_, err := jobCache.Get(ctx, "job-del")
	require.Error(t,
		err)
}

func TestWorkerJobCache_RedisL2BackfillAndCachebusInvalidate(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cacheA := newTierJobCache(time.Minute, workerCacheDeps{Redis: rdb})
	require.NoError(
		t, cacheA.Set(t.Context(), "job-redis",
			&domain.
				Job{
				ID: "job-redis", Version: 3, Name: "cached",
			}))

	registryB := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-b"})
	cacheB := newTierJobCache(time.Minute, workerCacheDeps{Redis: rdb, Registry: registryB})
	got, err := cacheB.Get(t.Context(), "job-redis")
	require.NoError(
		t, err)
	require.False(t,
		got.Name != "cached" ||
			got.Version !=
				3)

	publishTestWorkerInvalidate(t, registryB, workerJobCacheNamespace, "job-redis")
	if _, err := cacheB.Get(t.Context(), "job-redis"); err == nil {
		require.Fail(t,

			"expected cache miss after cachebus invalidation")
	}
}

func TestWorkerJobCache_UsesUpdatedAtVersionForRedisCAS(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	updatedAt := time.Unix(1700000000, 123).UTC()
	cache := newTierJobCache(time.Minute, workerCacheDeps{Redis: rdb})
	require.NoError(
		t, cache.Set(t.Context(), "job-versioned",

			&domain.
				Job{ID: "job-versioned", Version: 3,
				Name: "cached", UpdatedAt: updatedAt,
			}))

	raw, err := rdb.Get(t.Context(), "strait:cache:"+workerJobCacheNamespace+":job-versioned").Bytes()
	require.NoError(
		t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(
		t, json.Unmarshal(raw, &envelope))
	require.Equal(t,
		updatedAt.UnixNano(), envelope.
			Version)
}

func TestWorkerJobCache_PrefersCacheVersionForRedisCAS(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	updatedAt := time.Unix(1700000000, 123).UTC()
	cache := newTierJobCache(time.Minute, workerCacheDeps{Redis: rdb})
	require.NoError(
		t, cache.Set(t.Context(), "job-cache-version",

			&domain.
				Job{ID: "job-cache-version", Version: 3, Name: "cached", UpdatedAt: updatedAt, CacheVersion: 42},
		),
	)

	raw, err := rdb.Get(t.Context(), "strait:cache:"+workerJobCacheNamespace+":job-cache-version").Bytes()
	require.NoError(
		t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(
		t, json.Unmarshal(raw, &envelope))
	require.EqualValues(t, 42, envelope.Version)
}

func TestWorkerJobCache_StrongBarrierRejectsStaleLoaderFill(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cache := newTierJobCache(time.Minute, workerCacheDeps{Redis: rdb})
	require.NoError(
		t, cache.Delete(t.
			Context(), "job-deleted",
		),
	)

	_, err := cache.Load(t.Context(), "job-deleted", func(context.Context, string) (*domain.Job, error) {
		return &domain.Job{ID: "job-deleted", Name: "stale", CacheVersion: 1}, nil
	})
	require.Error(t,
		err)
}

func TestWorkerJobCache_StrongBarrierAllowsEqualVersionReplacement(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cache := newTierJobCache(time.Minute, workerCacheDeps{Redis: rdb})
	require.NoError(
		t, cache.tier.StrongInvalidate(t.
			Context(),
			workerCachePolicy(workerJobCacheNamespace),
			"job-recreated", "job-recreated",
			workerCacheBarrier(7), nil))

	got, err := cache.Load(t.Context(), "job-recreated", func(context.Context, string) (*domain.Job, error) {
		return &domain.Job{ID: "job-recreated", Name: "fresh", CacheVersion: 7}, nil
	})
	require.NoError(
		t, err)
	require.False(t,
		got == nil || got.
			Name != "fresh",
	)
}

func TestWorkerJobCache_LoadPreservesUpdatedAtVersionInRedis(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	updatedAt := time.Unix(1700000100, 456).UTC()
	cache := newTierJobCache(time.Minute, workerCacheDeps{Redis: rdb})
	got, err := cache.Load(t.Context(), "job-loaded", func(context.Context, string) (*domain.Job, error) {
		return &domain.Job{ID: "job-loaded", Version: 2, Name: "loaded", UpdatedAt: updatedAt}, nil
	})
	require.NoError(
		t, err)
	require.False(t,
		got == nil || got.
			UpdatedAt !=
			updatedAt)

	raw, err := rdb.Get(t.Context(), "strait:cache:"+workerJobCacheNamespace+":job-loaded").Bytes()
	require.NoError(
		t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(
		t, json.Unmarshal(raw, &envelope))
	require.Equal(t,
		updatedAt.UnixNano(), envelope.
			Version)
}

func TestJobCacheVersion(t *testing.T) {
	t.Parallel()

	updatedAt := time.Unix(1700000200, 789).UTC()
	tests := []struct {
		name string
		job  *domain.Job
		want int64
	}{
		{name: "nil job", job: nil, want: 0},
		{
			name: "cache version wins",
			job:  &domain.Job{CacheVersion: 42, UpdatedAt: updatedAt, Version: 3},
			want: 42,
		},
		{
			name: "updated at wins over semantic version",
			job:  &domain.Job{UpdatedAt: updatedAt, Version: 3},
			want: updatedAt.UnixNano(),
		},
		{
			name: "semantic version fallback",
			job:  &domain.Job{Version: 3},
			want: 3,
		},
		{
			name: "minimum nonzero fallback",
			job:  &domain.Job{},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t,
				tt.want, jobCacheVersion(tt.job))
		})
	}
}

func TestWorkerJobVersionKeyString(t *testing.T) {
	t.Parallel()

	got := workerJobVersionKeyString(jobVersionKey{JobID: "job-1", Version: 7})
	require.Equal(t,
		"job-1\x007", got,
	)
}

func TestVersionedJobCache_NilCacheUsesLoader(t *testing.T) {
	t.Parallel()

	var cache *tierVersionedJobCache
	got, err := cache.Load(t.Context(), jobVersionKey{JobID: "job-1", Version: 4}, func(_ context.Context, key jobVersionKey) (*domain.Job, error) {
		return &domain.Job{ID: key.JobID, Version: key.Version}, nil
	})
	require.NoError(
		t, err)
	require.False(t,
		got == nil || got.
			ID != "job-1" ||
			got.Version !=
				4,
	)
}

func TestJobCache_MultipleKeys(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(t, 5*time.Second)
	ctx := t.Context()

	for i := range 100 {
		job := &domain.Job{ID: "job-multi", Version: i}
		_ = jobCache.Set(ctx, job.ID+string(rune('a'+i)), job)
	}

	for _, i := range []int{0, 50, 99} {
		key := "job-multi" + string(rune('a'+i))
		cached, err := jobCache.Get(ctx, key)
		require.NoError(
			t, err)
		require.Equal(t,
			i, cached.Version,
		)
	}
}

func publishTestWorkerInvalidate(t *testing.T, registry *straitcache.Registry, namespace, key string) {
	t.Helper()
	data, err := json.Marshal(straitcache.BusMessage{
		Action:    straitcache.BusActionInvalidate,
		Namespace: namespace,
		Key:       key,
		Version:   time.Now().UnixNano(),
		Origin:    "peer",
		SentAt:    time.Now().UTC(),
	})
	require.NoError(
		t, err)

	registry.Handle(t.Context(), data)
}

func TestJobCache_NilCacheDisablesLookup(t *testing.T) {
	t.Parallel()

	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: nil, // disabled
	}

	ctx := t.Context()
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	job, err := e.resolveJobForRun(ctx, run)
	require.NoError(
		t, err)
	require.Equal(t,
		"job-1", job.ID)
	require.EqualValues(t, 1, dbCalls.Load())

	// Second call should also hit DB since cache is nil.
	_, _ = e.resolveJobForRun(ctx, run)
	require.EqualValues(t, 2, dbCalls.Load())
}

func TestWorkerCache_ConstructedFromExecutorConfig(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:                     NewPool(1),
		Queue:                    &mockExecQueue{},
		Store:                    &mockExecutorStore{},
		JobCacheTTL:              5 * time.Minute,
		VersionCacheTTL:          30 * time.Minute,
		RunVersionCacheTTL:       10 * time.Minute,
		JobHealthCacheTTL:        2 * time.Second,
		MaxDequeueBatchSize:      7,
		DefaultJobMaxConcurrency: 3,
	})
	require.NotNil(t,
		exec.jobCache)
	require.NotNil(t,
		exec.jobVersionCache,
	)
	require.NotNil(t,
		exec.runVersionCache,
	)
	require.NotNil(t,
		exec.stepsVersionCache,
	)
	require.NotNil(t,
		exec.jobHealthCache,
	)
	require.Equal(t, 7, exec.maxDequeueBatchSize)
	require.Equal(t, 3, exec.defaultJobMaxConcurrency)
}

func TestWorkerStrongCacheConstructorRegistersRuntimeNamespace(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
		mr.Close()
	})
	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "worker-test"})

	exec := NewExecutor(ExecutorConfig{
		Pool:          NewPool(1),
		Queue:         &mockExecQueue{},
		Store:         &mockExecutorStore{},
		JobCacheTTL:   time.Minute,
		RedisClient:   rdb,
		CacheRegistry: registry,
	})
	require.NotNil(t,
		exec.jobCache)

	assertWorkerRegisteredNamespaces(t, registry, []string{workerJobCacheNamespace})
}

func assertWorkerRegisteredNamespaces(t *testing.T, registry *straitcache.Registry, expected []string) {
	t.Helper()

	registered := make(map[string]struct{}, len(registry.RegisteredNamespaces()))
	for _, namespace := range registry.RegisteredNamespaces() {
		registered[namespace] = struct{}{}
	}
	for _, namespace := range expected {
		if _, ok := registered[namespace]; !ok {
			require.Failf(t, "test failure",

				"cache namespace %s was not registered; registered namespaces: %v", namespace, registry.RegisteredNamespaces())
		}
	}
}

func TestResolveJobForRun_CachesPinnedVersion(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int64
	var getVersionCalls atomic.Int64
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{ID: id, Version: 2, VersionPolicy: domain.VersionPolicyPin}, nil
		},
		getJobAtVersionFn: func(_ context.Context, jobID string, version int) (*domain.Job, error) {
			getVersionCalls.Add(1)
			return &domain.Job{ID: jobID, Version: version, VersionPolicy: domain.VersionPolicyPin}, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:            NewPool(1),
		Queue:           &mockExecQueue{},
		Store:           store,
		JobCacheTTL:     5 * time.Minute,
		VersionCacheTTL: 30 * time.Minute,
	})
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	for range 2 {
		job, err := exec.resolveJobForRun(t.Context(), run)
		require.NoError(
			t, err)
		require.Equal(t, 1, job.Version)
	}
	require.EqualValues(t, 1, getJobCalls.Load())
	require.EqualValues(t, 1, getVersionCalls.
		Load())
}

func TestResolveExecutionPolicy_WarmPathUsesCachedRunVersionAndSteps(t *testing.T) {
	t.Parallel()

	var stepRunCalls atomic.Int64
	var workflowRunCalls atomic.Int64
	var listStepsCalls atomic.Int64
	store := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			stepRunCalls.Add(1)
			return &domain.WorkflowStepRun{ID: id, WorkflowRunID: "wfr-1", StepRef: "step-a"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			workflowRunCalls.Add(1)
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", WorkflowVersion: 4}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			listStepsCalls.Add(1)
			return []domain.WorkflowStep{{
				WorkflowID:            workflowID,
				StepRef:               "step-a",
				RetryMaxAttempts:      8,
				TimeoutSecsOverride:   42,
				RetryInitialDelaySecs: version,
			}}, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:               NewPool(1),
		Queue:              &mockExecQueue{},
		Store:              store,
		VersionCacheTTL:    30 * time.Minute,
		RunVersionCacheTTL: 10 * time.Minute,
	})
	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	for range 2 {
		got, err := exec.resolveExecutionPolicy(t.Context(), run, fallback)
		require.NoError(
			t, err)
		require.Equal(t, 8, got.maxAttempts)
		require.Equal(t, 42, got.timeoutSecs)
		require.Equal(t, 4, got.retryInitialSecs)
	}
	require.EqualValues(t, 2, stepRunCalls.
		Load())
	require.EqualValues(t, 1, workflowRunCalls.
		Load())
	require.EqualValues(t, 1, listStepsCalls.
		Load())
}

func TestWorkflowStepsVersionCache_ReturnsClones(t *testing.T) {
	t.Parallel()

	var listStepsCalls atomic.Int64
	store := &mockExecutorStore{
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			listStepsCalls.Add(1)
			return []domain.WorkflowStep{{
				WorkflowID:         workflowID,
				StepRef:            "step-a",
				DependsOn:          []string{"root"},
				Condition:          json.RawMessage(`{"ok":true}`),
				ApprovalApprovers:  []string{"ops"},
				StageNotifications: json.RawMessage(`{"start":true}`),
				RetryMaxAttempts:   version,
			}}, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:            NewPool(1),
		Queue:           &mockExecQueue{},
		Store:           store,
		VersionCacheTTL: 30 * time.Minute,
	})

	first, err := exec.getWorkflowStepsForVersion(t.Context(), "wf-1", 3)
	require.NoError(
		t, err)

	first[0].StepRef = "mutated"
	first[0].DependsOn[0] = "mutated"
	first[0].Condition[0] = '{'
	first[0].ApprovalApprovers[0] = "mutated"
	first[0].StageNotifications[0] = '{'

	second, err := exec.getWorkflowStepsForVersion(t.Context(), "wf-1", 3)
	require.NoError(
		t, err)
	require.EqualValues(t, 1, listStepsCalls.
		Load())

	stringFieldsWereCloned := second[0].StepRef == "step-a" &&
		second[0].DependsOn[0] == "root" &&
		second[0].ApprovalApprovers[0] == "ops"
	require.True(t,
		stringFieldsWereCloned,
	)

	rawFieldsWereCloned := string(second[0].Condition) == `{"ok":true}` &&
		string(second[0].StageNotifications) == `{"start":true}`
	require.True(t,
		rawFieldsWereCloned,
	)
}

func TestJobHealthCache_BucketHitAvoidsStore(t *testing.T) {
	t.Parallel()

	var healthCalls atomic.Int64
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, jobID string, _ time.Time) (*orcstore.JobHealthStats, error) {
			healthCalls.Add(1)
			return &orcstore.JobHealthStats{TotalRuns: 10, HealthScore: 99}, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(1),
		Queue:             &mockExecQueue{},
		Store:             store,
		JobHealthCacheTTL: time.Minute,
	})
	now := time.Unix(1_700_000_000, 0)

	first, err := exec.getJobHealthStats(t.Context(), "job-1", now)
	require.NoError(
		t, err)

	first.TotalRuns = 999
	second, err := exec.getJobHealthStats(t.Context(), "job-1", now.Add(10*time.Second))
	require.NoError(
		t, err)
	require.EqualValues(t, 1, healthCalls.Load())
	require.Equal(t, 10, second.TotalRuns)
}

func TestJobCache_ResolveJobForRun_CacheHit(t *testing.T) {
	t.Parallel()

	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: newTestJobCache(t, 5*time.Second),
	}

	ctx := t.Context()
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	// First call: cache miss, hits DB.
	job, err := e.resolveJobForRun(ctx, run)
	require.NoError(
		t, err)
	require.Equal(t,
		"job-1", job.ID)
	require.EqualValues(t, 1, dbCalls.Load())

	// Second call: cache hit, no DB call.
	_, err = e.resolveJobForRun(ctx, run)
	require.NoError(
		t, err)
	require.EqualValues(t, 1, dbCalls.Load())
}

func TestJobCache_ResolveJobForRun_CacheExpiry(t *testing.T) {
	t.Parallel()

	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	// Otter uses a timer wheel with ~1s granularity for expiration.
	e := &Executor{
		store:    mockStore,
		jobCache: newTestJobCache(t, 1*time.Second),
	}

	ctx := t.Context()
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	// First call: populates cache.
	_, _ = e.resolveJobForRun(ctx, run)
	require.EqualValues(t, 1, dbCalls.Load())

	time.Sleep(3 * time.Second)

	// After TTL: cache miss, hits DB again.
	_, _ = e.resolveJobForRun(ctx, run)
	require.EqualValues(t, 2, dbCalls.Load())
}

func TestResolveJob_CacheHit(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{ID: "job-1", Version: 1, EndpointURL: "http://example.com"}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  5 * time.Minute,
	})
	t.Cleanup(exec.CloseCache)

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	job1, err := exec.resolveJobForRun(context.Background(), run)
	require.NoError(
		t, err)
	require.NotNil(t,
		job1)

	job2, err := exec.resolveJobForRun(context.Background(), run)
	require.NoError(
		t, err)
	require.NotNil(t,
		job2)
	assert.EqualValues(t, 1, getJobCalls.Load())
}

func TestDeepSecResolveJob_ClonesCachedJobBeforeEnvironmentOverrideMutation(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        &mockExecutorStore{},
		PollInterval: time.Hour,
		JobCacheTTL:  5 * time.Minute,
	})
	t.Cleanup(exec.CloseCache)

	cached := &domain.Job{ID: "job-1", ProjectID: "proj-1", Version: 1, EndpointURL: "https://original.example/run"}
	require.NoError(
		t, exec.jobCache.
			Set(context.Background(),
				"job-1",

				cached))

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	resolved, err := exec.resolveJobForRun(context.Background(), run)
	require.NoError(
		t, err)

	resolved.EndpointURL = "https://override.example/run"

	again, err := exec.jobCache.Get(context.Background(), "job-1")
	require.NoError(
		t, err)
	require.Equal(t,
		"https://original.example/run",

		again.EndpointURL,
	)
}

func TestDeepSecResolveJob_RefreshesLatestPolicyCacheHit(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{
				ID:            "job-1",
				ProjectID:     "proj-1",
				Version:       2,
				VersionID:     "v2",
				VersionPolicy: domain.VersionPolicyLatest,
				EndpointURL:   "https://fresh.example/run",
			}, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  5 * time.Minute,
	})
	t.Cleanup(exec.CloseCache)
	require.NoError(
		t, exec.jobCache.
			Set(context.Background(),
				"job-1",

				&domain.Job{ID: "job-1", ProjectID: "proj-1", Version: 1, VersionPolicy: domain.VersionPolicyLatest,

					EndpointURL: "https://stale.example/run",
				}))

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}
	resolved, err := exec.resolveJobForRun(context.Background(), run)
	require.NoError(
		t, err)
	require.EqualValues(t, 1, getJobCalls.Load())
	require.False(t,
		resolved.Version !=
			2 || resolved.
			EndpointURL !=
			"https://fresh.example/run",
	)
}

func TestResolveJob_CacheExpiry(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{ID: "job-1", Version: 1, EndpointURL: "http://example.com"}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  1 * time.Second,
	})
	t.Cleanup(exec.CloseCache)

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	_, _ = exec.resolveJobForRun(context.Background(), run)
	time.Sleep(3 * time.Second)
	_, _ = exec.resolveJobForRun(context.Background(), run)
	assert.EqualValues(t, 2, getJobCalls.Load())
}

func TestResolveJob_CacheDisabledWhenTTLZero(t *testing.T) {
	t.Parallel()

	var getJobCalls atomic.Int32
	store := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			getJobCalls.Add(1)
			return &domain.Job{ID: "job-1", Version: 1, EndpointURL: "http://example.com"}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
		JobCacheTTL:  0,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	_, _ = exec.resolveJobForRun(context.Background(), run)
	_, _ = exec.resolveJobForRun(context.Background(), run)
	assert.EqualValues(t, 2, getJobCalls.Load())
}
