package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestConnectedWorkerRefs_IncludesAllConnectedWorkers(t *testing.T) {
	registry := NewConnectionRegistry()
	active := &ConnectedWorker{
		WorkerID:       "worker-active",
		ProjectID:      "project-1",
		APIKeyID:       "key-active",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}
	draining := &ConnectedWorker{
		WorkerID:       "worker-draining",
		ProjectID:      "project-1",
		APIKeyID:       "key-draining",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "draining",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t, registry.Register(active))
	require.NoError(t, registry.Register(draining))

	refs := connectedWorkerRefs(registry)
	require.Len(t,

		refs, 2)

	got := map[store.ActiveWorkerRef]bool{}
	for _, ref := range refs {
		got[ref] = true
	}
	require.False(
		t,
		!got[store.ActiveWorkerRef{WorkerID: "worker-active",
			ProjectID: "project-1",
		}] ||
			!got[store.ActiveWorkerRef{WorkerID: "worker-draining", ProjectID: "project-1"}])
}

func TestSweepOnce_RecoversRequeuesEvictsAndDeletes(t *testing.T) {
	t.Parallel()

	registry := NewConnectionRegistry()
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "worker-active",
		ProjectID:      "project-1",
		APIKeyID:       "key-active",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))

	queries := &fakeSweepQueries{
		recoverableRunIDs: []string{"run-queued", "run-executing"},
		recoveredCount:    2,
		evictedCount:      3,
		deletedCount:      4,
		runsByID: map[string]*domain.JobRun{
			"run-queued":    {ID: "run-queued", ProjectID: "project-1", Status: domain.StatusQueued},
			"run-executing": {ID: "run-executing", ProjectID: "project-1", Status: domain.StatusExecuting},
		},
	}
	readyQueue := &fakeReadyRunQueue{}

	sweepOnce(context.Background(), registry, queries, 5*time.Minute, nil, readyQueue)

	require.Len(t, queries.listActiveWorkers, 1)
	require.Equal(t, store.ActiveWorkerRef{WorkerID: "worker-active", ProjectID: "project-1"}, queries.listActiveWorkers[0])
	require.Equal(t, queries.listActiveWorkers, queries.recoverActiveWorkers)
	require.Equal(t, queries.listActiveWorkers, queries.evictActiveWorkers)
	require.Equal(t, "worker heartbeat expired before reporting result", queries.recoverReason)
	require.Equal(t, int64(3), queries.evictedCount)
	require.Equal(t, int64(4), queries.deletedCount)
	require.Equal(t, []string{"run-queued"}, readyQueue.enqueuedRunIDs)
	require.False(t, queries.recoverCutoff.IsZero())
	require.False(t, queries.deleteCutoff.IsZero())
}

func TestSweepOnce_RecoverErrorSkipsEvictAndDelete(t *testing.T) {
	t.Parallel()

	queries := &fakeSweepQueries{recoverErr: errors.New("recover failed")}

	sweepOnce(context.Background(), nil, queries, time.Minute, nil, &fakeReadyRunQueue{})

	require.Equal(t, 1, queries.recoverCalls)
	require.Zero(t, queries.evictCalls)
	require.Zero(t, queries.deleteCalls)
}

func TestSweepOnce_EvictErrorSkipsDelete(t *testing.T) {
	t.Parallel()

	queries := &fakeSweepQueries{evictErr: errors.New("evict failed")}

	sweepOnce(context.Background(), nil, queries, time.Minute, nil, &fakeReadyRunQueue{})

	require.Equal(t, 1, queries.recoverCalls)
	require.Equal(t, 1, queries.evictCalls)
	require.Zero(t, queries.deleteCalls)
}

func TestRecoverDurableResultHandoffs_HandlesNilFinalizer(t *testing.T) {
	t.Parallel()

	queries := &fakeSweepQueries{claimedTasks: []domain.WorkerTask{{ID: "task-1"}}}

	recoverDurableResultHandoffs(context.Background(), queries, nil, time.Now())

	require.Zero(t, queries.claimCalls)
}

func TestRecoverDurableResultHandoffs_ResetsMalformedResult(t *testing.T) {
	t.Parallel()

	queries := &fakeSweepQueries{
		claimedTasks: []domain.WorkerTask{{ID: "task-malformed", RunID: "run-1"}},
	}

	recoverDurableResultHandoffs(context.Background(), queries, func() WorkerRunResultFinalizer {
		return &fakeWorkerRunResultFinalizer{}
	}, time.Now())

	require.Equal(t, []string{"task-malformed"}, queries.resetTaskIDs)
	require.Empty(t, queries.updatedTaskStatuses)
}

func TestRecoverDurableResultHandoffs_ResetsFinalizerError(t *testing.T) {
	t.Parallel()

	queries := &fakeSweepQueries{
		claimedTasks: []domain.WorkerTask{{
			ID:    "task-finalize",
			RunID: "run-finalize",
			Result: &domain.WorkerTaskResult{
				Status: "success",
				Output: json.RawMessage(`{"ok":true}`),
			},
		}},
	}

	recoverDurableResultHandoffs(context.Background(), queries, func() WorkerRunResultFinalizer {
		return &fakeWorkerRunResultFinalizer{err: errors.New("finalize failed")}
	}, time.Now())

	require.Equal(t, []string{"task-finalize"}, queries.resetTaskIDs)
	require.Empty(t, queries.updatedTaskStatuses)
}

func TestRecoverDurableResultHandoffs_UpdatesRecoveredTaskStatus(t *testing.T) {
	t.Parallel()

	queries := &fakeSweepQueries{
		claimedTasks: []domain.WorkerTask{{
			ID:    "task-complete",
			RunID: "run-complete",
			Result: &domain.WorkerTaskResult{
				Status: "success",
				Output: json.RawMessage(`{"ok":true}`),
			},
		}},
	}
	finalizer := &fakeWorkerRunResultFinalizer{status: domain.WorkerTaskStatusCompleted}

	recoverDurableResultHandoffs(context.Background(), queries, func() WorkerRunResultFinalizer {
		return finalizer
	}, time.Now())

	require.Equal(t, []finalizedRun{{
		runID:  "run-complete",
		status: "success",
		output: json.RawMessage(`{"ok":true}`),
	}}, finalizer.finalized)
	require.Equal(t, map[string]domain.WorkerTaskStatus{
		"task-complete": domain.WorkerTaskStatusCompleted,
	}, queries.updatedTaskStatuses)
	require.Empty(t, queries.resetTaskIDs)
}

type fakeSweepQueries struct {
	claimedTasks []domain.WorkerTask
	claimErr     error
	claimCalls   int
	claimLimit   int

	resetTaskIDs []string
	resetErr     error

	updatedTaskStatuses map[string]domain.WorkerTaskStatus
	updateErr           error

	recoverableRunIDs []string
	listErr           error
	listActiveWorkers []store.ActiveWorkerRef

	recoveredCount       int64
	recoverErr           error
	recoverCalls         int
	recoverCutoff        time.Time
	recoverReason        string
	recoverActiveWorkers []store.ActiveWorkerRef

	evictedCount       int64
	evictErr           error
	evictCalls         int
	evictActiveWorkers []store.ActiveWorkerRef

	deletedCount int64
	deleteErr    error
	deleteCalls  int
	deleteCutoff time.Time

	runsByID map[string]*domain.JobRun
}

func (f *fakeSweepQueries) ClaimRecoverableWorkerTaskResults(_ context.Context, _ time.Time, limit int) ([]domain.WorkerTask, error) {
	f.claimCalls++
	f.claimLimit = limit
	return f.claimedTasks, f.claimErr
}

func (f *fakeSweepQueries) ResetWorkerTaskFinalizingToResultReceived(_ context.Context, taskID string) error {
	f.resetTaskIDs = append(f.resetTaskIDs, taskID)
	return f.resetErr
}

func (f *fakeSweepQueries) UpdateWorkerTaskStatus(_ context.Context, taskID string, status domain.WorkerTaskStatus) error {
	if f.updatedTaskStatuses == nil {
		f.updatedTaskStatuses = map[string]domain.WorkerTaskStatus{}
	}
	f.updatedTaskStatuses[taskID] = status
	return f.updateErr
}

func (f *fakeSweepQueries) ListRecoverableStaleWorkerTaskRunIDs(_ context.Context, _ time.Time, activeWorkers []store.ActiveWorkerRef) ([]string, error) {
	f.listActiveWorkers = append([]store.ActiveWorkerRef(nil), activeWorkers...)
	return f.recoverableRunIDs, f.listErr
}

func (f *fakeSweepQueries) RecoverStaleWorkerTasksExceptRefs(_ context.Context, cutoff time.Time, reason string, activeWorkers []store.ActiveWorkerRef) (int64, error) {
	f.recoverCalls++
	f.recoverCutoff = cutoff
	f.recoverReason = reason
	f.recoverActiveWorkers = append([]store.ActiveWorkerRef(nil), activeWorkers...)
	return f.recoveredCount, f.recoverErr
}

func (f *fakeSweepQueries) EvictStaleWorkersExceptRefs(_ context.Context, _ time.Time, activeWorkers []store.ActiveWorkerRef) (int64, error) {
	f.evictCalls++
	f.evictActiveWorkers = append([]store.ActiveWorkerRef(nil), activeWorkers...)
	return f.evictedCount, f.evictErr
}

func (f *fakeSweepQueries) DeleteStaleOfflineWorkers(_ context.Context, cutoff time.Time) (int64, error) {
	f.deleteCalls++
	f.deleteCutoff = cutoff
	return f.deletedCount, f.deleteErr
}

func (f *fakeSweepQueries) GetRunsByIDs(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
	out := make(map[string]*domain.JobRun, len(ids))
	for _, id := range ids {
		if run := f.runsByID[id]; run != nil {
			out[id] = run
		}
	}
	return out, nil
}

type fakeReadyRunQueue struct {
	enqueuedRunIDs []string
	err            error
}

func (f *fakeReadyRunQueue) EnqueueExisting(_ context.Context, run *domain.JobRun) error {
	f.enqueuedRunIDs = append(f.enqueuedRunIDs, run.ID)
	return f.err
}

type finalizedRun struct {
	runID        string
	status       string
	errorMessage string
	output       json.RawMessage
}

type fakeWorkerRunResultFinalizer struct {
	status    domain.WorkerTaskStatus
	err       error
	finalized []finalizedRun
}

func (f *fakeWorkerRunResultFinalizer) FinalizeWorkerRunResult(_ context.Context, runID, status, errorMessage string, output json.RawMessage) (domain.WorkerTaskStatus, error) {
	f.finalized = append(f.finalized, finalizedRun{
		runID:        runID,
		status:       status,
		errorMessage: errorMessage,
		output:       append(json.RawMessage(nil), output...),
	})
	if f.err != nil {
		return "", f.err
	}
	return f.status, nil
}
