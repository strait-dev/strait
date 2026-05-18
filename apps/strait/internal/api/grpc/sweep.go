package grpc

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/store"
)

const staleOfflineWorkerDeleteAfter = 24 * time.Hour

// runSweep periodically deletes workers rows whose last heartbeat is older
// than heartbeatTimeout, indicating the worker disconnected without a clean
// deregister (e.g. network partition). This loop complements the in-memory
// Deregister path — it cleans up DB rows that outlive a crashed replica.
func runSweep(ctx context.Context, registry *ConnectionRegistry, q *store.Queries, heartbeatTimeout, interval time.Duration) {
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
			cutoff := time.Now().Add(-heartbeatTimeout)
			connectedIDs := connectedWorkerIDs(registry)
			recovered, err := q.RecoverStaleWorkerTasksExcept(ctx, cutoff, "worker heartbeat expired before reporting result", connectedIDs)
			if err != nil {
				slog.Warn("grpc sweep: recover stale worker tasks failed", "error", err)
				continue
			}
			if recovered > 0 {
				slog.Info("grpc sweep: recovered stale worker tasks", "count", recovered)
			}
			n, err := q.EvictStaleWorkersExcept(ctx, cutoff, connectedIDs)
			if err != nil {
				slog.Warn("grpc sweep: evict stale workers failed", "error", err)
				continue
			}
			if n > 0 {
				slog.Info("grpc sweep: evicted stale workers", "count", n)
			}
			deleteCutoff := time.Now().Add(-staleOfflineWorkerDeleteAfter)
			deleted, err := q.DeleteStaleOfflineWorkers(ctx, deleteCutoff)
			if err != nil {
				slog.Warn("grpc sweep: delete stale offline workers failed", "error", err)
				continue
			}
			if deleted > 0 {
				slog.Info("grpc sweep: deleted stale offline workers", "count", deleted)
			}
		}
	}
}

func connectedWorkerIDs(registry *ConnectionRegistry) []string {
	if registry == nil {
		return nil
	}
	workers := registry.Snapshot()
	ids := make([]string, 0, len(workers))
	for _, worker := range workers {
		if worker.WorkerID != "" {
			ids = append(ids, worker.WorkerID)
		}
	}
	return ids
}
