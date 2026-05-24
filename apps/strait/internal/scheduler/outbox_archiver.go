package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

type OutboxArchiveStore interface {
	ArchiveConsumedOutboxBatch(ctx context.Context, olderThan time.Duration, batchSize int) (int64, error)
}

type OutboxArchiver struct {
	store      OutboxArchiveStore
	interval   time.Duration
	olderThan  time.Duration
	batchSize  int
	logger     *slog.Logger
	iterations atomic.Int64
	archived   atomic.Int64
	errors     atomic.Int64
}

type OutboxArchiverConfig struct {
	Interval  time.Duration
	OlderThan time.Duration
	BatchSize int
	Logger    *slog.Logger
}

func NewOutboxArchiver(store OutboxArchiveStore, cfg OutboxArchiverConfig) *OutboxArchiver {
	a := &OutboxArchiver{
		store:     store,
		interval:  cfg.Interval,
		olderThan: cfg.OlderThan,
		batchSize: cfg.BatchSize,
		logger:    cfg.Logger,
	}
	if a.interval <= 0 {
		a.interval = time.Second
	}
	if a.batchSize <= 0 {
		a.batchSize = 500
	}
	if a.logger == nil {
		a.logger = slog.Default()
	}
	return a
}

func (a *OutboxArchiver) Iterations() int64 { return a.iterations.Load() }
func (a *OutboxArchiver) Archived() int64   { return a.archived.Load() }
func (a *OutboxArchiver) Errors() int64     { return a.errors.Load() }

func (a *OutboxArchiver) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	_ = a.archiveOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = a.archiveOnce(ctx)
		}
	}
}

func (a *OutboxArchiver) ArchiveOnceForTest(ctx context.Context) error {
	return a.archiveOnce(ctx)
}

func (a *OutboxArchiver) archiveOnce(ctx context.Context) error {
	defer func() {
		a.iterations.Add(1)
		if rec := recover(); rec != nil {
			a.errors.Add(1)
			a.logger.Warn("outbox archiver panic recovered", "panic", rec)
		}
	}()

	archived, err := a.store.ArchiveConsumedOutboxBatch(ctx, a.olderThan, a.batchSize)
	if err != nil {
		a.errors.Add(1)
		a.logger.Warn("outbox archiver failed", "error", err)
		return err
	}
	a.archived.Add(archived)
	if archived > 0 {
		a.logger.Debug("outbox archiver archived rows", "count", archived)
	}
	return nil
}
