package grpc

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/store"
)

// runSweep periodically deletes workers rows whose last heartbeat is older
// than heartbeatTimeout, indicating the worker disconnected without a clean
// deregister (e.g. network partition). This loop complements the in-memory
// Deregister path — it cleans up DB rows that outlive a crashed replica.
func runSweep(ctx context.Context, reg *ConnectionRegistry, q *store.Queries, heartbeatTimeout, interval time.Duration) {
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
			n, err := q.EvictStaleWorkers(ctx, cutoff)
			if err != nil {
				slog.Warn("grpc sweep: evict stale workers failed", "error", err)
				continue
			}
			if n > 0 {
				slog.Info("grpc sweep: evicted stale workers", "count", n)
			}
		}
	}
}
