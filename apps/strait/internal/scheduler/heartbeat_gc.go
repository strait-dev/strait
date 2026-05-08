package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/store"
)

// Orphan heartbeat GC.
//
// The unlogged heartbeat side table `job_run_heartbeats` is
// maintained by the worker -- inserts on claim, updates on heartbeat
// tick, deletes on terminal transition. If a terminal transition skips
// the delete (historic bug, operator intervention, replica that misses
// a trigger) the row leaks and the table grows without bound.
//
// This GC runs hourly under an advisory lock and deletes heartbeat rows
// whose owning run is no longer executing. It is bounded per tick so a
// large mass deletion is spread across multiple cycles.

const heartbeatGCAdvisoryLockID int64 = 0x53744842474300 // "StHbGC"

// HeartbeatGCStore is the minimal store interface the GC needs.
type HeartbeatGCStore interface {
	DeleteOrphanedHeartbeats(ctx context.Context, limit int) (int64, error)
}

// HeartbeatGC periodically deletes leaked rows from job_run_heartbeats.
type HeartbeatGC struct {
	store          HeartbeatGCStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	batchLimit     int
	logger         *slog.Logger
	iterations     atomic.Int64
	totalDeleted   atomic.Int64
}

// HeartbeatGCConfig configures the GC.
type HeartbeatGCConfig struct {
	Interval   time.Duration
	BatchLimit int
	Logger     *slog.Logger
}

// NewHeartbeatGC builds a GC with the given config.
func NewHeartbeatGC(s HeartbeatGCStore, cfg HeartbeatGCConfig) *HeartbeatGC {
	h := &HeartbeatGC{
		store:      s,
		interval:   cfg.Interval,
		batchLimit: cfg.BatchLimit,
		logger:     cfg.Logger,
	}
	if h.interval <= 0 {
		h.interval = time.Hour
	}
	if h.batchLimit <= 0 {
		h.batchLimit = 10000
	}
	if h.logger == nil {
		h.logger = slog.Default()
	}
	return h
}

// WithAdvisoryLocker enables single-leader execution.
func (h *HeartbeatGC) WithAdvisoryLocker(locker AdvisoryLocker) *HeartbeatGC {
	h.advisoryLocker = locker
	return h
}

func (h *HeartbeatGC) Iterations() int64   { return h.iterations.Load() }
func (h *HeartbeatGC) TotalDeleted() int64 { return h.totalDeleted.Load() }

// Run blocks until ctx is cancelled.
func (h *HeartbeatGC) Run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	runSchedulerCycleCheckIn(ctx, h.interval, func() {
		_ = h.runOnce(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, h.interval, func() {
				_ = h.runOnce(ctx)
			})
		}
	}
}

// RunOnceForTest exposes a single iteration to integration tests.
func (h *HeartbeatGC) RunOnceForTest(ctx context.Context) error {
	return h.runOnce(ctx)
}

func (h *HeartbeatGC) runOnce(ctx context.Context) error {
	defer func() {
		h.iterations.Add(1)
		if r := recover(); r != nil {
			h.logger.Warn("heartbeat GC panic recovered", "panic", r)
		}
	}()

	if h.advisoryLocker != nil {
		acquired, err := h.advisoryLocker.TryAdvisoryLock(ctx, heartbeatGCAdvisoryLockID)
		if err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		defer func() {
			if err := h.advisoryLocker.ReleaseAdvisoryLock(ctx, heartbeatGCAdvisoryLockID); err != nil {
				h.logger.Debug("heartbeat GC lock release failed", "error", err)
			}
		}()
	}

	deleted, err := h.store.DeleteOrphanedHeartbeats(ctx, h.batchLimit)
	if err != nil {
		h.logger.Warn("heartbeat GC delete failed", "error", err)
		return err
	}
	h.totalDeleted.Add(deleted)
	if deleted > 0 {
		h.logger.Info("heartbeat GC deleted orphaned rows", "deleted", deleted)
	}
	return nil
}

// EnsureQueueTriggersPresent fails loudly at startup if any of the
// critical queue triggers are missing. This prevents silent fallback
// from pg_notify-driven workers to poll-only mode when an operator
// inadvertently drops a trigger.
func EnsureQueueTriggersPresent(ctx context.Context, db store.DBTX) error {
	required := []string{
		"trg_job_runs_queue_wake_notify",
		"job_runs_active_counts_trg",
		"job_runs_dlq_counts_trg",
		"job_runs_seed_job_config_trg",
	}
	for _, name := range required {
		var present bool
		err := db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal)`,
			name,
		).Scan(&present)
		if err != nil {
			return fmt.Errorf("trigger presence check for %s: %w", name, err)
		}
		if !present {
			return fmt.Errorf("required queue trigger %q is missing -- queue will silently degrade to poll-only; re-run migrations or investigate manual DDL", name)
		}
	}
	return nil
}
