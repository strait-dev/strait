package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

func TestStatusReadModel_GetRunTerminalCacheUsesRedisBeforeStore(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
		Clone:     cloneJobRunForStatusCache,
		Sanitize:  cloneJobRunForStatusCache,
	})
	if ok, err := model.CompareAndSet(context.Background(), "run-1", &domain.JobRun{ID: "run-1", ProjectID: "proj-1", Status: domain.StatusCompleted}, 3); err != nil || !ok {
		require.Failf(t, "test failure",

			"CompareAndSet() = %v, %v; want true, nil", ok, err)
	}
	var storeCalls atomic.Int64
	srv := &Server{
		store: &APIStoreMock{
			GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
				storeCalls.Add(1)
				return nil, nil
			},
		},
		runStatusReadModel: model,
	}

	got, err := srv.getRunWithStatusReadModel(context.Background(), "run-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, got.Status)
	require.EqualValues(t, 0, storeCalls.Load())
}

func TestStatusReadModel_GetRunRefreshesNonTerminalCacheFromStore(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
		Clone:     cloneJobRunForStatusCache,
		Sanitize:  cloneJobRunForStatusCache,
	})
	ok, err := model.CompareAndSet(context.Background(), "run-1", &domain.JobRun{
		ID:           "run-1",
		ProjectID:    "proj-1",
		Status:       domain.StatusQueued,
		CacheVersion: 2,
	}, 2)
	require.NoError(t, err)
	require.True(t, ok)

	var storeCalls atomic.Int64
	srv := &Server{
		store: &versionedStatusStore{
			APIStoreMock: &APIStoreMock{},
			getRunWithVersion: func(_ context.Context, id string) (*domain.JobRun, int64, error) {
				storeCalls.Add(1)
				return &domain.JobRun{
					ID:           id,
					ProjectID:    "proj-1",
					Status:       domain.StatusDeadLetter,
					CacheVersion: 3,
				}, 3, nil
			},
		},
		runStatusReadModel: model,
	}

	got, err := srv.getRunWithStatusReadModel(context.Background(), "run-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusDeadLetter, got.Status)
	require.EqualValues(t, 1, storeCalls.Load())

	cached, err := model.Get(context.Background(), "run-1")
	require.NoError(t, err)
	require.EqualValues(t, 3, cached.Version)
	require.Equal(t, domain.StatusDeadLetter, cached.Value.Status)
}

func TestStatusReadModel_GetRunColdFallbackFillsRedis(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
		Clone:     cloneJobRunForStatusCache,
		Sanitize:  cloneJobRunForStatusCache,
	})
	var storeCalls atomic.Int64
	srv := &Server{
		store: &APIStoreMock{
			GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
				storeCalls.Add(1)
				return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusCompleted}, nil
			},
		},
		runStatusReadModel: model,
	}

	for range 2 {
		got, err := srv.getRunWithStatusReadModel(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, domain.StatusCompleted, got.Status)
	}
	require.EqualValues(t, 1, storeCalls.Load())
}

func TestStatusReadModel_GetRunColdFallbackUsesStoreCacheVersion(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
		Clone:     cloneJobRunForStatusCache,
		Sanitize:  cloneJobRunForStatusCache,
	})
	var storeCalls atomic.Int64
	srv := &Server{
		store: &versionedStatusStore{
			APIStoreMock: &APIStoreMock{},
			getRunWithVersion: func(_ context.Context, id string) (*domain.JobRun, int64, error) {
				storeCalls.Add(1)
				return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting, CacheVersion: 9}, 9, nil
			},
		},
		runStatusReadModel: model,
	}

	got, err := srv.getRunWithStatusReadModel(context.Background(), "run-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusExecuting, got.Status)

	ok, err := model.CompareAndSet(context.Background(), "run-1", &domain.JobRun{ID: "run-1", Status: domain.StatusQueued}, 7)
	require.NoError(t, err)
	require.False(t, ok)

	cached, err := model.Get(context.Background(), "run-1")
	require.NoError(t, err)
	require.EqualValues(t, 9, cached.Version)
	require.Equal(t, domain.StatusExecuting, cached.Value.Status)
	require.EqualValues(t, 1, storeCalls.Load())
}

func TestStatusReadModel_GetWorkflowRunColdFallbackUsesStoreCacheVersion(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
		Client:    rdb,
		Namespace: "status_workflow_run",
		TTL:       time.Minute,
		Clone:     cloneWorkflowRunForStatusCache,
		Sanitize:  cloneWorkflowRunForStatusCache,
	})
	var storeCalls atomic.Int64
	srv := &Server{
		store: &versionedStatusStore{
			APIStoreMock: &APIStoreMock{},
			getWorkflowRunWithVersion: func(_ context.Context, id string) (*domain.WorkflowRun, int64, error) {
				storeCalls.Add(1)
				return &domain.WorkflowRun{ID: id, ProjectID: "proj-1", Status: domain.WfStatusRunning, CacheVersion: 14}, 14, nil
			},
		},
		workflowRunStatusReadModel: model,
	}

	got, err := srv.getWorkflowRunWithStatusReadModel(context.Background(), "wfr-1")
	require.NoError(t, err)
	require.Equal(t, domain.WfStatusRunning, got.Status)

	ok, err := model.CompareAndSet(context.Background(), "wfr-1", &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusPending}, 9)
	require.NoError(t, err)
	require.False(t, ok)

	cached, err := model.Get(context.Background(), "wfr-1")
	require.NoError(t, err)
	require.EqualValues(t, 14, cached.Version)
	require.Equal(t, domain.WfStatusRunning, cached.Value.Status)
	require.EqualValues(t, 1, storeCalls.Load())
}

func TestStatusReadModel_GetWorkflowRunRefreshesNonTerminalCacheFromStore(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
		Client:    rdb,
		Namespace: "status_workflow_run",
		TTL:       time.Minute,
		Clone:     cloneWorkflowRunForStatusCache,
		Sanitize:  cloneWorkflowRunForStatusCache,
	})
	ok, err := model.CompareAndSet(context.Background(), "wfr-1", &domain.WorkflowRun{
		ID:           "wfr-1",
		ProjectID:    "proj-1",
		Status:       domain.WfStatusRunning,
		CacheVersion: 2,
	}, 2)
	require.NoError(t, err)
	require.True(t, ok)

	var storeCalls atomic.Int64
	srv := &Server{
		store: &versionedStatusStore{
			APIStoreMock: &APIStoreMock{},
			getWorkflowRunWithVersion: func(_ context.Context, id string) (*domain.WorkflowRun, int64, error) {
				storeCalls.Add(1)
				return &domain.WorkflowRun{
					ID:           id,
					ProjectID:    "proj-1",
					Status:       domain.WfStatusCompleted,
					CacheVersion: 3,
				}, 3, nil
			},
		},
		workflowRunStatusReadModel: model,
	}

	got, err := srv.getWorkflowRunWithStatusReadModel(context.Background(), "wfr-1")
	require.NoError(t, err)
	require.Equal(t, domain.WfStatusCompleted, got.Status)
	require.EqualValues(t, 1, storeCalls.Load())

	cached, err := model.Get(context.Background(), "wfr-1")
	require.NoError(t, err)
	require.EqualValues(t, 3, cached.Version)
	require.Equal(t, domain.WfStatusCompleted, cached.Value.Status)
}

type versionedStatusStore struct {
	*APIStoreMock
	getRunWithVersion         func(context.Context, string) (*domain.JobRun, int64, error)
	getWorkflowRunWithVersion func(context.Context, string) (*domain.WorkflowRun, int64, error)
}

func (s *versionedStatusStore) GetRunWithCacheVersion(ctx context.Context, id string) (*domain.JobRun, int64, error) {
	return s.getRunWithVersion(ctx, id)
}

func (s *versionedStatusStore) GetWorkflowRunWithCacheVersion(ctx context.Context, id string) (*domain.WorkflowRun, int64, error) {
	return s.getWorkflowRunWithVersion(ctx, id)
}
