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
// maintained by the worker -- inserts on claim and heartbeat tick, clear
// tombstones on terminal transition, and bounded physical compaction in
// this GC. If a terminal transition skips the clear (historic bug, operator
// intervention, replica that misses a trigger) the row leaks and the table
// grows without bound.
//
// This GC runs hourly under an advisory lock, clears heartbeat rows whose
// owning run is no longer active, and deletes superseded heartbeat history.
// Both operations are bounded per tick so large cleanups spread across cycles.

const heartbeatGCAdvisoryLockID int64 = 0x53744842474300 // "StHbGC"

// HeartbeatGCStore is the minimal store interface the GC needs.
type HeartbeatGCStore interface {
	DeleteOrphanedHeartbeats(ctx context.Context, limit int) (int64, error)
	CompactSupersededHeartbeats(ctx context.Context, limit int) (int64, error)
	DeleteInactiveActiveClaims(ctx context.Context, limit int) (int64, error)
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

func (h *HeartbeatGC) runOnce(ctx context.Context) (err error) {
	defer func() {
		h.iterations.Add(1)
		if r := recover(); r != nil {
			h.logger.Warn("heartbeat GC panic recovered", "panic", r)
			err = fmt.Errorf("heartbeat GC panic: %v", r)
		}
	}()

	acquired, err := runWithOptionalAdvisoryLock(ctx, h.advisoryLocker, heartbeatGCAdvisoryLockID, h.runLocked)
	if err != nil || !acquired {
		return err
	}
	return nil
}

func (h *HeartbeatGC) runLocked(ctx context.Context) error {
	deleted, err := h.store.DeleteOrphanedHeartbeats(ctx, h.batchLimit)
	if err != nil {
		h.logger.Warn("heartbeat GC delete failed", "error", err)
		return err
	}
	compacted, err := h.store.CompactSupersededHeartbeats(ctx, h.batchLimit)
	if err != nil {
		h.logger.Warn("heartbeat GC compact failed", "error", err)
		return err
	}
	deletedClaims, err := h.store.DeleteInactiveActiveClaims(ctx, h.batchLimit)
	if err != nil {
		h.logger.Warn("active claim GC failed", "error", err)
		return err
	}
	total := deleted + compacted + deletedClaims
	h.totalDeleted.Add(total)
	if total > 0 {
		h.logger.Info("heartbeat GC cleaned rows", "cleared_heartbeats", deleted, "compacted_heartbeats", compacted, "deleted_active_claims", deletedClaims)
	}
	return nil
}

// EnsureQueueTriggersPresent fails loudly at startup if any of the
// critical queue triggers are missing. This prevents silent fallback
// from pg_notify-driven workers to poll-only mode when an operator
// inadvertently drops a trigger.
func EnsureQueueTriggersPresent(ctx context.Context, db store.DBTX) error {
	required := []struct {
		name     string
		relation string
		function string
	}{
		{name: "trg_job_runs_queue_wake_insert_notify", relation: "job_runs", function: "notify_queue_wake_insert_stmt"},
		{name: "trg_job_runs_queue_wake_update_notify", relation: "job_runs", function: "notify_queue_wake_update_stmt"},
		{name: "trg_queue_entries_claimable_wake_insert_notify", relation: "queue_entries", function: "notify_queue_entries_claimable_insert_stmt"},
		{name: "trg_queue_entries_claimable_wake_update_notify", relation: "queue_entries", function: "notify_queue_entries_claimable_update_stmt"},
		{name: "job_run_state_active_counts_trg", relation: "job_run_state", function: "job_active_counts_apply"},
		{name: "job_runs_dlq_counts_trg", relation: "job_runs", function: "dlq_counts_apply"},
		{name: "job_runs_seed_job_config_trg", relation: "job_runs", function: "seed_job_config_on_insert"},
		{name: "trg_job_runs_claim_queue_sync", relation: "job_runs", function: "trg_job_runs_sync_claim_queue"},
		{name: "trg_job_runs_claim_queue_sync_update", relation: "job_runs", function: "trg_job_runs_sync_claim_queue"},
		{name: "trg_jobs_fanout_queue", relation: "jobs", function: "trg_jobs_fanout_to_queue"},
	}
	for _, trigger := range required {
		var present bool
		err := db.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1
				FROM pg_trigger t
				JOIN pg_proc p ON p.oid = t.tgfoid
				WHERE t.tgname = $1
				  AND t.tgrelid = $2::regclass
				  AND p.proname = $3
				  AND NOT t.tgisinternal
				  AND t.tgenabled IN ('O', 'A')
			)`,
			trigger.name,
			trigger.relation,
			trigger.function,
		).Scan(&present)
		if err != nil {
			return fmt.Errorf("trigger presence check for %s: %w", trigger.name, err)
		}
		if !present {
			return fmt.Errorf("required queue trigger %q is missing, disabled, or misconfigured -- queue will silently degrade to poll-only; re-run migrations or investigate manual DDL", trigger.name)
		}
	}
	return nil
}
