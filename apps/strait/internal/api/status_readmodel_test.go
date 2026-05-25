package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

func TestStatusReadModel_GetRunUsesRedisBeforeStore(t *testing.T) {
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
	if ok, err := model.CompareAndSet(context.Background(), "run-1", &domain.JobRun{ID: "run-1", ProjectID: "proj-1", Status: domain.StatusExecuting}, 3); err != nil || !ok {
		t.Fatalf("CompareAndSet() = %v, %v; want true, nil", ok, err)
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
	if err != nil {
		t.Fatalf("getRunWithStatusReadModel() error = %v", err)
	}
	if got.Status != domain.StatusExecuting {
		t.Fatalf("status = %s, want executing", got.Status)
	}
	if storeCalls.Load() != 0 {
		t.Fatalf("store calls = %d, want 0", storeCalls.Load())
	}
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
				return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusQueued}, nil
			},
		},
		runStatusReadModel: model,
	}

	for range 2 {
		got, err := srv.getRunWithStatusReadModel(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("getRunWithStatusReadModel() error = %v", err)
		}
		if got.Status != domain.StatusQueued {
			t.Fatalf("status = %s, want queued", got.Status)
		}
	}
	if storeCalls.Load() != 1 {
		t.Fatalf("store calls = %d, want 1", storeCalls.Load())
	}
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
	if err != nil {
		t.Fatalf("getRunWithStatusReadModel() error = %v", err)
	}
	if got.Status != domain.StatusExecuting {
		t.Fatalf("status = %s, want executing", got.Status)
	}
	ok, err := model.CompareAndSet(context.Background(), "run-1", &domain.JobRun{ID: "run-1", Status: domain.StatusQueued}, 7)
	if err != nil {
		t.Fatalf("CompareAndSet(v7) error = %v", err)
	}
	if ok {
		t.Fatal("CompareAndSet(v7) = true, want false")
	}
	cached, err := model.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cached.Version != 9 || cached.Value.Status != domain.StatusExecuting {
		t.Fatalf("cached = %+v, want version 9 executing", cached)
	}
	if storeCalls.Load() != 1 {
		t.Fatalf("store calls = %d, want 1", storeCalls.Load())
	}
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
	if err != nil {
		t.Fatalf("getWorkflowRunWithStatusReadModel() error = %v", err)
	}
	if got.Status != domain.WfStatusRunning {
		t.Fatalf("status = %s, want running", got.Status)
	}
	ok, err := model.CompareAndSet(context.Background(), "wfr-1", &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusPending}, 9)
	if err != nil {
		t.Fatalf("CompareAndSet(v9) error = %v", err)
	}
	if ok {
		t.Fatal("CompareAndSet(v9) = true, want false")
	}
	cached, err := model.Get(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cached.Version != 14 || cached.Value.Status != domain.WfStatusRunning {
		t.Fatalf("cached = %+v, want version 14 running", cached)
	}
	if storeCalls.Load() != 1 {
		t.Fatalf("store calls = %d, want 1", storeCalls.Load())
	}
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
