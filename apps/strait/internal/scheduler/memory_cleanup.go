package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// MemoryCleanupStore defines the store operations needed by MemoryCleanup.
type MemoryCleanupStore interface {
	DeleteExpiredJobMemory(ctx context.Context) (int64, error)
}

// MemoryCleanup periodically deletes expired job memory entries.
type MemoryCleanup struct {
	store    MemoryCleanupStore
	interval time.Duration
	logger   *slog.Logger
}

// NewMemoryCleanup creates a new memory cleanup scheduler.
func NewMemoryCleanup(s MemoryCleanupStore, interval time.Duration) *MemoryCleanup {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &MemoryCleanup{
		store:    s,
		interval: interval,
		logger:   slog.Default(),
	}
}

// Run starts the memory cleanup loop. Blocks until ctx is canceled.
func (mc *MemoryCleanup) Run(ctx context.Context) {
	ticker := time.NewTicker(mc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mc.cleanup(ctx)
		}
	}
}

func (mc *MemoryCleanup) cleanup(ctx context.Context) {
	deleted, err := mc.store.DeleteExpiredJobMemory(ctx)
	if err != nil {
		mc.logger.Warn("memory cleanup: failed to delete expired entries", "error", err)
		return
	}
	if deleted > 0 {
		mc.logger.Info("memory cleanup: deleted expired entries", "count", deleted)
	}
}
