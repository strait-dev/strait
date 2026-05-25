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
