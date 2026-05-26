package cdc

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

func TestStatusReadModelHandler_PopulatesRedisAndRejectsOutOfOrder(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)

	if err := handlers.JobRuns.Handle(context.Background(), Message{
		Action:   ActionUpdate,
		Record:   []byte(`{"id":"run-1","job_id":"job-1","project_id":"proj-1","status":"executing","cache_version":7}`),
		Metadata: Metadata{TableName: "job_runs"},
	}); err != nil {
		t.Fatalf("Handle(v7) error = %v", err)
	}
	if err := handlers.JobRuns.Handle(context.Background(), Message{
		Action:   ActionUpdate,
		Record:   []byte(`{"id":"run-1","job_id":"job-1","project_id":"proj-1","status":"queued","cache_version":6}`),
		Metadata: Metadata{TableName: "job_runs"},
	}); err != nil {
		t.Fatalf("Handle(v6) error = %v", err)
	}

	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
	})
	got, err := model.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Version != 7 || got.Value.Status != domain.StatusExecuting {
		t.Fatalf("read model = version %d status %s, want version 7 running", got.Version, got.Value.Status)
	}
}

func TestStatusReadModelHandler_DeleteEvictsRedis(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)

	if err := handlers.WorkflowRuns.Handle(context.Background(), Message{
		Action:   ActionUpdate,
		Record:   []byte(`{"id":"wfr-1","workflow_id":"wf-1","project_id":"proj-1","status":"running","cache_version":3}`),
		Metadata: Metadata{TableName: "workflow_runs"},
	}); err != nil {
		t.Fatalf("Handle(update) error = %v", err)
	}
	if err := handlers.WorkflowRuns.Handle(context.Background(), Message{
		Action:   ActionDelete,
		Record:   []byte(`{"id":"wfr-1","workflow_id":"wf-1","project_id":"proj-1","status":"running","cache_version":4}`),
		Metadata: Metadata{TableName: "workflow_runs"},
	}); err != nil {
		t.Fatalf("Handle(delete) error = %v", err)
	}

	model := straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
		Client:    rdb,
		Namespace: "status_workflow_run",
		TTL:       time.Minute,
	})
	if _, err := model.Get(context.Background(), "wfr-1"); err == nil {
		t.Fatal("Get() error = nil, want cache miss after delete")
	}
}

func TestStatusReadModelHandler_OutOfOrderDeleteDoesNotRemoveNewerRun(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)

	if err := handlers.JobRuns.Handle(context.Background(), Message{
		Action:   ActionUpdate,
		Record:   []byte(`{"id":"run-newer","job_id":"job-1","project_id":"proj-1","status":"executing","cache_version":7}`),
		Metadata: Metadata{TableName: "job_runs"},
	}); err != nil {
		t.Fatalf("Handle(update v7) error = %v", err)
	}
	if err := handlers.JobRuns.Handle(context.Background(), Message{
		Action:   ActionDelete,
		Record:   []byte(`{"id":"run-newer","job_id":"job-1","project_id":"proj-1","status":"queued","cache_version":6}`),
		Metadata: Metadata{TableName: "job_runs"},
	}); err != nil {
		t.Fatalf("Handle(delete v6) error = %v", err)
	}

	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
	})
	got, err := model.Get(context.Background(), "run-newer")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Version != 7 || got.Value.Status != domain.StatusExecuting {
		t.Fatalf("read model = version %d status %s, want version 7 executing", got.Version, got.Value.Status)
	}
}

func TestStatusReadModelHandler_DeleteBarrierSelfHealsOnEqualVersionUpdate(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)

	if err := handlers.WorkflowRuns.Handle(context.Background(), Message{
		Action:   ActionDelete,
		Record:   []byte(`{"id":"wfr-equal","workflow_id":"wf-1","project_id":"proj-1","status":"running","cache_version":8}`),
		Metadata: Metadata{TableName: "workflow_runs"},
	}); err != nil {
		t.Fatalf("Handle(delete v8) error = %v", err)
	}

	model := straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
		Client:    rdb,
		Namespace: "status_workflow_run",
		TTL:       time.Minute,
	})
	if _, err := model.Get(context.Background(), "wfr-equal"); err == nil {
		t.Fatal("Get() error = nil, want miss while delete barrier is present")
	}

	if err := handlers.WorkflowRuns.Handle(context.Background(), Message{
		Action:   ActionUpdate,
		Record:   []byte(`{"id":"wfr-equal","workflow_id":"wf-1","project_id":"proj-1","status":"running","cache_version":8}`),
		Metadata: Metadata{TableName: "workflow_runs"},
	}); err != nil {
		t.Fatalf("Handle(update v8) error = %v", err)
	}
	got, err := model.Get(context.Background(), "wfr-equal")
	if err != nil {
		t.Fatalf("Get() after equal update error = %v", err)
	}
	if got.Version != 8 || got.Value.Status != domain.WfStatusRunning {
		t.Fatalf("read model = version %d status %s, want version 8 running", got.Version, got.Value.Status)
	}
}

func TestStatusReadModelHandler_BadPayloadIsIgnored(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)

	if err := handlers.JobRuns.Handle(context.Background(), Message{
		Action:   ActionUpdate,
		Record:   []byte(`{"id":`),
		Metadata: Metadata{TableName: "job_runs"},
	}); err != nil {
		t.Fatalf("Handle(malformed) error = %v, want nil", err)
	}
}
