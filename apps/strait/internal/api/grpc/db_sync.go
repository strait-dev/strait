package grpc

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/domain"
)

type workerRegistrar interface {
	RegisterWorker(ctx context.Context, worker *domain.Worker) error
}

// runDBSync periodically upserts all connected workers into the workers table
// so that the HTTP API can surface live worker state and so that other
// replicas can observe which workers are connected where.
func runDBSync(ctx context.Context, reg *ConnectionRegistry, q workerRegistrar, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			dbSyncOnce(ctx, reg, q)
		}
	}
}

func dbSyncOnce(ctx context.Context, reg *ConnectionRegistry, q workerRegistrar) {
	workers := reg.Snapshot()
	for _, w := range workers {
		queueName := ""
		if len(w.Queues) > 0 {
			queueName = w.Queues[0]
		}
		dw := &domain.Worker{
			ID:        w.WorkerID,
			ProjectID: w.ProjectID,
			QueueName: queueName,
			Hostname:  w.Hostname,
			Version:   w.SDKVersion,
			Status:    domain.WorkerStatus(w.Status),
		}
		if err := q.RegisterWorker(ctx, dw); err != nil {
			slog.Warn("grpc db sync: upsert worker failed",
				"worker_id", w.WorkerID,
				"error", err,
			)
		}
	}
}
