package scheduler

import (
	"context"
	"log/slog"
	"regexp"
	"sync/atomic"
	"time"

	"strait/internal/store"
)

const partitionReclaimerAdvisoryLockID int64 = 0x5374507274526563 // "StPrtRec"

var partitionMonthRe = regexp.MustCompile(`^(?:job_runs|enqueue_outbox_history)_p(\d{4})_(\d{2})$`)

type PartitionReclaimer struct {
	store          PartitionReclaimerStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	safetyMonths   int
	logger         *slog.Logger
	iterations     atomic.Int64
	dropped        atomic.Int64
	errors         atomic.Int64
}

type PartitionReclaimerStore interface {
	ListJobRunsPartitions(ctx context.Context) ([]string, error)
	ListOutboxHistoryPartitions(ctx context.Context) ([]string, error)
	PartitionRowCount(ctx context.Context, partition string) (int64, error)
	PartitionEstimatedRowCount(ctx context.Context, partition string) (int64, error)
	DropPartitionWithTimeout(ctx context.Context, partition string, timeout time.Duration) error
}

type PartitionReclaimerConfig struct {
	Interval     time.Duration
	SafetyMonths int
	Logger       *slog.Logger
}

func NewPartitionReclaimer(s PartitionReclaimerStore, cfg PartitionReclaimerConfig) *PartitionReclaimer {
	p := &PartitionReclaimer{
		store:        s,
		interval:     cfg.Interval,
		safetyMonths: cfg.SafetyMonths,
		logger:       cfg.Logger,
	}
	if p.interval <= 0 {
		p.interval = 24 * time.Hour
	}
	if p.safetyMonths < 1 {
		p.safetyMonths = 2
	}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	return p
}

func (p *PartitionReclaimer) WithAdvisoryLocker(locker AdvisoryLocker) *PartitionReclaimer {
	p.advisoryLocker = locker
	return p
}

func (p *PartitionReclaimer) Iterations() int64 { return p.iterations.Load() }
func (p *PartitionReclaimer) Dropped() int64    { return p.dropped.Load() }
func (p *PartitionReclaimer) Errors() int64     { return p.errors.Load() }

func (p *PartitionReclaimer) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	_ = p.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = p.runOnce(ctx)
		}
	}
}

func (p *PartitionReclaimer) RunOnceForTest(ctx context.Context) error {
	return p.runOnce(ctx)
}

func (p *PartitionReclaimer) runOnce(ctx context.Context) error {
	defer func() {
		p.iterations.Add(1)
		if r := recover(); r != nil {
			p.logger.Warn("partition reclaimer panic recovered", "panic", r)
			p.errors.Add(1)
		}
	}()

	if p.advisoryLocker != nil {
		acquired, err := p.advisoryLocker.TryAdvisoryLock(ctx, partitionReclaimerAdvisoryLockID)
		if err != nil {
			p.errors.Add(1)
			return err
		}
		if !acquired {
			return nil
		}
		defer func() {
			if err := p.advisoryLocker.ReleaseAdvisoryLock(ctx, partitionReclaimerAdvisoryLockID); err != nil {
				p.logger.Debug("partition reclaimer lock release failed", "error", err)
			}
		}()
	}

	jobPartitions, err := p.store.ListJobRunsPartitions(ctx)
	if err != nil {
		p.logger.Warn("partition reclaimer: list job_runs partitions failed", "error", err)
		p.errors.Add(1)
		return err
	}
	p.reclaimPartitions(ctx, jobPartitions)

	outboxPartitions, err := p.store.ListOutboxHistoryPartitions(ctx)
	if err != nil {
		p.logger.Warn("partition reclaimer: list outbox history partitions failed", "error", err)
		p.errors.Add(1)
		return err
	}
	p.reclaimPartitions(ctx, outboxPartitions)

	return nil
}

func (p *PartitionReclaimer) reclaimPartitions(ctx context.Context, partitions []string) {
	cutoff := time.Now().UTC().AddDate(0, -p.safetyMonths, 0)
	cutoffMonth := time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, time.UTC)

	for _, name := range partitions {
		m := partitionMonthRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		partMonth, err := time.Parse("2006_01", m[1]+"_"+m[2])
		if err != nil {
			continue
		}
		if !partMonth.Before(cutoffMonth) {
			continue
		}

		est, err := p.store.PartitionEstimatedRowCount(ctx, name)
		if err == nil && est > 0 {
			p.logger.Debug("partition reclaimer: skipping non-empty partition (estimate)", "partition", name, "estimated_rows", est)
			continue
		}

		count, err := p.store.PartitionRowCount(ctx, name)
		if err != nil {
			p.logger.Warn("partition reclaimer: row count failed", "partition", name, "error", err)
			p.errors.Add(1)
			continue
		}
		if count > 0 {
			p.logger.Debug("partition reclaimer: skipping non-empty partition", "partition", name, "rows", count)
			continue
		}

		if err := p.store.DropPartitionWithTimeout(ctx, name, 5*time.Second); err != nil {
			p.logger.Warn("partition reclaimer: drop failed", "partition", name, "error", err)
			p.errors.Add(1)
			continue
		}
		p.dropped.Add(1)
		p.logger.Info("partition reclaimer: dropped empty partition", "partition", name)
	}
}

var _ PartitionReclaimerStore = (*store.Queries)(nil)
