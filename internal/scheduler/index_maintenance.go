package scheduler

import (
	"context"
	"log/slog"
	"time"
)

const defaultIndexMaintenanceInterval = 24 * time.Hour

var defaultReindexTargets = []string{
	"idx_runs_queue_covering",
	"idx_webhook_deliveries_pending",
}

type IndexMaintenanceStore interface {
	ReindexIndexConcurrently(ctx context.Context, indexName string) error
}

type IndexMaintainer struct {
	store    IndexMaintenanceStore
	interval time.Duration
	indexes  []string
	logger   *slog.Logger
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

func (m *IndexMaintainer) Run(ctx context.Context) {
	loop := NewMaintenanceLoop("index-maintenance", m.interval, m.logger, func(loopCtx context.Context) {
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
