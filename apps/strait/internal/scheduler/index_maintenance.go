package scheduler

import (
	"context"
	"log/slog"
	"time"
)

const defaultIndexMaintenanceInterval = 24 * time.Hour

// indexMaintainerAdvisoryLockID is the pg_advisory_lock key used to ensure
// only one worker instance runs REINDEX per cycle when multiple workers
// share a database. Equals "StraitIx" packed into an int64.
const indexMaintainerAdvisoryLockID int64 = 0x5374726169744978

// defaultReindexTargets are the partial indexes on job_runs that accumulate
// bloat fastest because they churn on every queue state transition. They
// benefit most from periodic REINDEX CONCURRENTLY.
//
// Note: idx_job_runs_active_by_job and idx_job_runs_concurrency_key_active
// were dropped in migration 000221 (denormalized dequeue is now default).
var defaultReindexTargets = []string{
	"idx_runs_queue_covering",
	"idx_webhook_deliveries_pending",
	"idx_job_runs_retry",
	"idx_job_runs_inflight_started",
	"idx_job_runs_queue_priority",
	"idx_job_runs_job_id_created",
	"idx_job_run_queue_dequeue",
}

type IndexMaintenanceStore interface {
	ReindexIndexConcurrently(ctx context.Context, indexName string) error
}

type IndexMaintainer struct {
	store          IndexMaintenanceStore
	interval       time.Duration
	indexes        []string
	logger         *slog.Logger
	advisoryLocker AdvisoryLocker
}

func NewIndexMaintainer(store IndexMaintenanceStore, interval time.Duration) *IndexMaintainer {
	if interval <= 0 {
		interval = defaultIndexMaintenanceInterval
	}

	return &IndexMaintainer{
		store:    store,
		interval: interval,
		indexes:  append([]string(nil), defaultReindexTargets...),
		logger:   slog.Default(),
	}
}

// WithAdvisoryLocker enables single-leader execution across multiple worker
// instances sharing the same database. When set, each cycle acquires a
// PostgreSQL advisory lock and skips the cycle if another instance holds it.
// Matches the reaper's single-leader pattern.
func (m *IndexMaintainer) WithAdvisoryLocker(locker AdvisoryLocker) *IndexMaintainer {
	m.advisoryLocker = locker
	return m
}

func (m *IndexMaintainer) Run(ctx context.Context) {
	loop := NewMaintenanceLoop("index-maintenance", m.interval, m.logger, func(loopCtx context.Context) {
		if m.advisoryLocker != nil {
			acquired, err := m.advisoryLocker.TryAdvisoryLock(loopCtx, indexMaintainerAdvisoryLockID)
			if err != nil {
				m.logger.Error("index maintenance advisory lock check failed, skipping cycle", "error", err)
				return
			}
			if !acquired {
				m.logger.Debug("index maintenance advisory lock held by another instance, skipping cycle")
				return
			}
			defer func() {
				if err := m.advisoryLocker.ReleaseAdvisoryLock(loopCtx, indexMaintainerAdvisoryLockID); err != nil {
					m.logger.Warn("failed to release index maintenance advisory lock", "error", err)
				}
			}()
		}

		for _, indexName := range m.indexes {
			if err := m.store.ReindexIndexConcurrently(loopCtx, indexName); err != nil {
				m.logger.Error("failed to reindex partial index", "index", indexName, "error", err)
				continue
			}
			m.logger.Info("reindexed partial index", "index", indexName)
		}
	})
	loop.Run(ctx)
}
