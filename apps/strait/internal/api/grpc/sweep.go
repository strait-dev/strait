package grpc

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

const staleOfflineWorkerDeleteAfter = 24 * time.Hour

type workerSweepQueries interface {
	workerResultRecoveryQueries
	recoveredRunLoader
	ListRecoverableStaleWorkerTaskRunIDs(ctx context.Context, cutoff time.Time, activeWorkers []store.ActiveWorkerRef) ([]string, error)
	RecoverStaleWorkerTasksExceptRefs(ctx context.Context, cutoff time.Time, reason string, activeWorkers []store.ActiveWorkerRef) (int64, error)
	EvictStaleWorkersExceptRefs(ctx context.Context, cutoff time.Time, activeWorkers []store.ActiveWorkerRef) (int64, error)
	DeleteStaleOfflineWorkers(ctx context.Context, cutoff time.Time) (int64, error)
}

type workerResultRecoveryQueries interface {
	ClaimRecoverableWorkerTaskResults(ctx context.Context, cutoff time.Time, limit int) ([]domain.WorkerTask, error)
	ResetWorkerTaskFinalizingToResultReceived(ctx context.Context, taskID string) error
	UpdateWorkerTaskStatus(ctx context.Context, taskID string, status domain.WorkerTaskStatus) error
}

// runSweep periodically deletes workers rows whose last heartbeat is older
// than heartbeatTimeout, indicating the worker disconnected without a clean
// deregister (e.g. network partition). This loop complements the in-memory
// Deregister path — it cleans up DB rows that outlive a crashed replica.
func runSweep(
	ctx context.Context,
	registry *ConnectionRegistry,
	q workerSweepQueries,
	heartbeatTimeout time.Duration,
	interval time.Duration,
	finalizer func() WorkerRunResultFinalizer,
	readyRunQueue ReadyRunEnqueuer,
) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweepOnce(ctx, registry, q, heartbeatTimeout, finalizer, readyRunQueue)
		}
	}
}

func sweepOnce(
	ctx context.Context,
	registry *ConnectionRegistry,
	q workerSweepQueries,
	heartbeatTimeout time.Duration,
	finalizer func() WorkerRunResultFinalizer,
	readyRunQueue ReadyRunEnqueuer,
) {
	cutoff := time.Now().Add(-heartbeatTimeout)
	recoverDurableResultHandoffs(ctx, q, finalizer, cutoff)
	connectedWorkers := connectedWorkerRefs(registry)
	recoveredRunIDs, listErr := q.ListRecoverableStaleWorkerTaskRunIDs(ctx, cutoff, connectedWorkers)
	if listErr != nil {
		slog.Warn("grpc sweep: list recoverable stale worker task runs failed", "error", listErr)
	}
	recovered, err := q.RecoverStaleWorkerTasksExceptRefs(ctx, cutoff, "worker heartbeat expired before reporting result", connectedWorkers)
	if err != nil {
		slog.Warn("grpc sweep: recover stale worker tasks failed", "error", err)
		return
	}
	if recovered > 0 {
		slog.Info("grpc sweep: recovered stale worker tasks", "count", recovered)
		enqueueRecoveredWorkerRuns(ctx, q, readyRunQueue, recoveredRunIDs, "grpc sweep")
	}
	n, err := q.EvictStaleWorkersExceptRefs(ctx, cutoff, connectedWorkers)
	if err != nil {
		slog.Warn("grpc sweep: evict stale workers failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("grpc sweep: evicted stale workers", "count", n)
	}
	deleteCutoff := time.Now().Add(-staleOfflineWorkerDeleteAfter)
	deleted, err := q.DeleteStaleOfflineWorkers(ctx, deleteCutoff)
	if err != nil {
		slog.Warn("grpc sweep: delete stale offline workers failed", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("grpc sweep: deleted stale offline workers", "count", deleted)
	}
}

func recoverDurableResultHandoffs(
	ctx context.Context,
	q workerResultRecoveryQueries,
	finalizer func() WorkerRunResultFinalizer,
	cutoff time.Time,
) {
	if finalizer == nil {
		return
	}
	runFinalizer := finalizer()
	if runFinalizer == nil {
		return
	}
	tasks, err := q.ClaimRecoverableWorkerTaskResults(ctx, cutoff, 100)
	if err != nil {
		slog.Warn("grpc sweep: claim recoverable worker results failed", "error", err)
		return
	}
	for _, task := range tasks {
		if task.Result == nil {
			if resetErr := q.ResetWorkerTaskFinalizingToResultReceived(ctx, task.ID); resetErr != nil {
				slog.Warn("grpc sweep: reset malformed worker result claim failed", "task_id", task.ID, "error", resetErr)
			}
			continue
		}
		taskStatus, err := runFinalizer.FinalizeWorkerRunResult(ctx, task.RunID, task.Result.Status, task.Result.Error, task.Result.Output)
		if err != nil {
			slog.Warn("grpc sweep: finalize recoverable worker result failed",
				"task_id", task.ID,
				"run_id", task.RunID,
				"error", err,
			)
			if resetErr := q.ResetWorkerTaskFinalizingToResultReceived(ctx, task.ID); resetErr != nil {
				slog.Warn("grpc sweep: reset worker result recovery claim failed", "task_id", task.ID, "error", resetErr)
			}
			continue
		}
		if err := q.UpdateWorkerTaskStatus(ctx, task.ID, taskStatus); err != nil {
			slog.Warn("grpc sweep: update recovered worker task status failed",
				"task_id", task.ID,
				"run_id", task.RunID,
				"status", taskStatus,
				"error", err,
			)
		}
	}
}

func connectedWorkerRefs(registry *ConnectionRegistry) []store.ActiveWorkerRef {
	if registry == nil {
		return nil
	}
	workers := registry.Snapshot()
	refs := make([]store.ActiveWorkerRef, 0, len(workers))
	for _, worker := range workers {
		if worker.WorkerID != "" && worker.ProjectID != "" {
			refs = append(refs, store.ActiveWorkerRef{
				WorkerID:  worker.WorkerID,
				ProjectID: worker.ProjectID,
			})
		}
	}
	return refs
}
