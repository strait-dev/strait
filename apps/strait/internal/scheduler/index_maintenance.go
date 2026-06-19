package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"strait/internal/store"
)

// indexMaintainerAdvisoryLockID is the pg_advisory_lock key used to ensure
// only one worker instance runs REINDEX per cycle when multiple workers
// share a database. Equals "StraitIx" packed into an int64.
const indexMaintainerAdvisoryLockID int64 = 0x5374726169744978

// defaultReindexTargets are the partial indexes on job_runs that accumulate
// bloat fastest because they churn on every queue state transition. They
// benefit most from periodic REINDEX CONCURRENTLY.
var defaultReindexTargets = []string{
	"idx_runs_queue_covering",
	"idx_webhook_deliveries_pending",
	"idx_job_runs_retry",
	"idx_job_runs_inflight_started",
	"idx_job_runs_queue_priority",
	"idx_job_runs_job_id_created",
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
		interval = defaultIndexMaintenanceInterval()
	}

	return &IndexMaintainer{
		store:    store,
		interval: interval,
		indexes:  append([]string(nil), defaultReindexTargets...),
		logger:   slog.Default(),
	}
}

func defaultIndexMaintenanceInterval() time.Duration {
	return 24 * time.Hour
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
		acquired, err := runWithOptionalAdvisoryLock(loopCtx, m.advisoryLocker, indexMaintainerAdvisoryLockID, m.runLocked)
		if err != nil {
			m.logger.Error("index maintenance advisory lock cycle failed", "error", err)
			return
		}
		if !acquired {
			m.logger.Debug("index maintenance advisory lock held by another instance, skipping cycle")
		}
	})
	loop.Run(ctx)
}

func (m *IndexMaintainer) runLocked(ctx context.Context) error {
	for _, indexName := range m.indexes {
		if err := m.store.ReindexIndexConcurrently(ctx, indexName); err != nil {
			if errors.Is(err, store.ErrIndexNotFound) {
				m.logger.Debug("skipping missing reindex target", "index", indexName)
				continue
			}
			m.logger.Error("failed to reindex partial index", "index", indexName, "error", err)
			continue
		}
		m.logger.Info("reindexed partial index", "index", indexName)
	}
	return nil
}
