package cdc

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

func TestStatusReadModelHandler_PopulatesRedisAndRejectsOutOfOrder(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)
	require.NoError(t, handlers.
		JobRuns.Handle(t.Context(), Message{Action: ActionUpdate, Record: []byte(`{"id":"run-1","job_id":"job-1","project_id":"proj-1","status":"executing","cache_version":7}`), Metadata: Metadata{TableName: "job_runs"}}))
	require.NoError(t, handlers.
		JobRuns.Handle(t.Context(), Message{Action: ActionUpdate, Record: []byte(`{"id":"run-1","job_id":"job-1","project_id":"proj-1","status":"queued","cache_version":6}`), Metadata: Metadata{TableName: "job_runs"}}))

	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
	})
	got, err := model.Get(t.Context(), "run-1")
	require.NoError(t, err)
	require.False(t, got.
		Version !=
		7 || got.Value.Status != domain.
		StatusExecuting)

}

func TestStatusReadModelHandler_DeleteEvictsRedis(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)
	require.NoError(t, handlers.
		WorkflowRuns.Handle(t.Context(), Message{Action: ActionUpdate,

		Record: []byte(`{"id":"wfr-1","workflow_id":"wf-1","project_id":"proj-1","status":"running","cache_version":3}`), Metadata: Metadata{TableName: "workflow_runs"}}))
	require.NoError(t, handlers.
		WorkflowRuns.Handle(t.Context(), Message{Action: ActionDelete,

		Record: []byte(`{"id":"wfr-1","workflow_id":"wf-1","project_id":"proj-1","status":"running","cache_version":4}`), Metadata: Metadata{TableName: "workflow_runs"}}))

	model := straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
		Client:    rdb,
		Namespace: "status_workflow_run",
		TTL:       time.Minute,
	})
	if _, err := model.Get(t.Context(), "wfr-1"); err == nil {
		require.Fail(t,

			"Get() error = nil, want cache miss after delete")
	}
}

func TestStatusReadModelHandler_OutOfOrderDeleteDoesNotRemoveNewerRun(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)
	require.NoError(t, handlers.
		JobRuns.Handle(t.Context(), Message{Action: ActionUpdate, Record: []byte(`{"id":"run-newer","job_id":"job-1","project_id":"proj-1","status":"executing","cache_version":7}`), Metadata: Metadata{TableName: "job_runs"}}))
	require.NoError(t, handlers.
		JobRuns.Handle(t.Context(), Message{Action: ActionDelete, Record: []byte(`{"id":"run-newer","job_id":"job-1","project_id":"proj-1","status":"queued","cache_version":6}`), Metadata: Metadata{TableName: "job_runs"}}))

	model := straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
		Client:    rdb,
		Namespace: "status_job_run",
		TTL:       time.Minute,
	})
	got, err := model.Get(t.Context(), "run-newer")
	require.NoError(t, err)
	require.False(t, got.
		Version !=
		7 || got.Value.Status != domain.
		StatusExecuting)

}

func TestStatusReadModelHandler_DeleteBarrierSelfHealsOnEqualVersionUpdate(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)
	record := []byte(
		`{"id":"wfr-equal","workflow_id":"wf-1","project_id":"proj-1","status":"running","cache_version":8}`,
	)
	require.NoError(t, handlers.
		WorkflowRuns.Handle(t.Context(), Message{Action: ActionDelete,

		Record: record, Metadata: Metadata{TableName: "workflow_runs"}},
	))

	model := straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
		Client:    rdb,
		Namespace: "status_workflow_run",
		TTL:       time.Minute,
	})
	if _, err := model.Get(t.Context(), "wfr-equal"); err == nil {
		require.Fail(t,

			"Get() error = nil, want miss while delete barrier is present")
	}
	require.NoError(t, handlers.
		WorkflowRuns.Handle(t.Context(), Message{Action: ActionUpdate,

		Record: record, Metadata: Metadata{TableName: "workflow_runs"}},
	))

	got, err := model.Get(t.Context(), "wfr-equal")
	require.NoError(t, err)
	require.False(t, got.
		Version !=
		8 || got.Value.Status != domain.
		WfStatusRunning)

}

func TestStatusReadModelHandler_BadPayloadIsIgnored(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)
	require.NoError(t, handlers.
		JobRuns.Handle(t.Context(), Message{Action: ActionUpdate, Record: []byte(`{"id":`), Metadata: Metadata{TableName: "job_runs"}}))

}
