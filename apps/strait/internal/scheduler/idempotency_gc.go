package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Idempotency dedup GC.
//
// job_run_idempotency is a small global dedup table that records
// (job_id, idempotency_key, run_id) so cross-partition deduplication
// can outlive any single job_runs partition. Rows accumulate as
// idempotent triggers fire; without a GC the table grows without
// bound, even though entries past their expires_at are no longer
// consulted by any reader.
//
// This GC runs hourly under an advisory lock and deletes rows whose
// expires_at has passed. It is bounded per tick so a large mass
// deletion is spread across multiple cycles.

const idempotencyGCAdvisoryLockID int64 = 0x5374496447430000 // "StIdGC\0\0"

// IdempotencyGCStore is the minimal store interface the GC needs.
type IdempotencyGCStore interface {
	DeleteExpiredIdempotencyEntries(ctx context.Context, limit int) (int64, error)
}

// IdempotencyGC periodically deletes expired rows from job_run_idempotency.
type IdempotencyGC struct {
	store          IdempotencyGCStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	batchLimit     int
	logger         *slog.Logger
	iterations     atomic.Int64
	totalDeleted   atomic.Int64
}

// IdempotencyGCConfig configures the GC.
type IdempotencyGCConfig struct {
	Interval   time.Duration
	BatchLimit int
	Logger     *slog.Logger
}

// NewIdempotencyGC builds a GC with the given config.
func NewIdempotencyGC(s IdempotencyGCStore, cfg IdempotencyGCConfig) *IdempotencyGC {
	g := &IdempotencyGC{
		store:      s,
		interval:   cfg.Interval,
		batchLimit: cfg.BatchLimit,
		logger:     cfg.Logger,
	}
	if g.interval <= 0 {
		g.interval = time.Hour
	}
	if g.batchLimit <= 0 {
		g.batchLimit = 10000
	}
	if g.logger == nil {
		g.logger = slog.Default()
	}
	return g
}

// WithAdvisoryLocker enables single-leader execution.
func (g *IdempotencyGC) WithAdvisoryLocker(locker AdvisoryLocker) *IdempotencyGC {
	g.advisoryLocker = locker
	return g
}

func (g *IdempotencyGC) Iterations() int64   { return g.iterations.Load() }
func (g *IdempotencyGC) TotalDeleted() int64 { return g.totalDeleted.Load() }

// Run blocks until ctx is cancelled.
func (g *IdempotencyGC) Run(ctx context.Context) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	runSchedulerCycleCheckIn(ctx, g.interval, func() {
		_ = g.runOnce(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, g.interval, func() {
				_ = g.runOnce(ctx)
			})
		}
	}
}

// RunOnceForTest exposes a single iteration to integration tests.
func (g *IdempotencyGC) RunOnceForTest(ctx context.Context) error {
	return g.runOnce(ctx)
}

func (g *IdempotencyGC) runOnce(ctx context.Context) error {
	defer func() {
		g.iterations.Add(1)
		if r := recover(); r != nil {
			g.logger.Warn("idempotency GC panic recovered", "panic", r)
		}
	}()

	acquired, err := runWithOptionalAdvisoryLock(ctx, g.advisoryLocker, idempotencyGCAdvisoryLockID, g.runLocked)
	if err != nil || !acquired {
		return err
	}
	return nil
}

func (g *IdempotencyGC) runLocked(ctx context.Context) error {
	deleted, err := g.store.DeleteExpiredIdempotencyEntries(ctx, g.batchLimit)
	if err != nil {
		g.logger.Warn("idempotency GC delete failed", "error", err)
		return err
	}
	g.totalDeleted.Add(deleted)
	if deleted > 0 {
		g.logger.Info("idempotency GC deleted expired rows", "deleted", deleted)
	}
	return nil
}
