package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/store"
)

// Daily partition ensurer.
//
// cmd/strait calls store.EnsureJobRunsPartitions at startup. This
// goroutine re-runs it daily so a long-lived service never drifts into
// the "no partition for current month" failure mode.

const partitionEnsurerAdvisoryLockID int64 = 0x5450727448454E00 // "StPrtHEN"

// PartitionEnsurer runs periodically to keep job_runs partitions current.
type PartitionEnsurer struct {
	store          PartitionEnsurerStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	monthsAhead    int
	logger         *slog.Logger
	iterations     atomic.Int64
	errors         atomic.Int64
}

// PartitionEnsurerStore is the minimal store surface the ensurer needs.
// Satisfied by *store.Queries.
type PartitionEnsurerStore interface {
	EnsureJobRunsPartitions(ctx context.Context, monthsAhead int) error
	EnsureOutboxHistoryPartitions(ctx context.Context, monthsAhead int) error
}

// PartitionEnsurerConfig configures the ensurer.
type PartitionEnsurerConfig struct {
	Interval    time.Duration
	MonthsAhead int
	Logger      *slog.Logger
}

// NewPartitionEnsurer builds the ensurer with defaults.
func NewPartitionEnsurer(s PartitionEnsurerStore, cfg PartitionEnsurerConfig) *PartitionEnsurer {
	p := &PartitionEnsurer{
		store:       s,
		interval:    cfg.Interval,
		monthsAhead: cfg.MonthsAhead,
		logger:      cfg.Logger,
	}
	if p.interval <= 0 {
		p.interval = 24 * time.Hour
	}
	if p.monthsAhead < 1 {
		p.monthsAhead = 2
	}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	return p
}

// WithAdvisoryLocker enables single-leader ensurer across replicas.
func (p *PartitionEnsurer) WithAdvisoryLocker(locker AdvisoryLocker) *PartitionEnsurer {
	p.advisoryLocker = locker
	return p
}

// Iterations returns the number of completed ensurer cycles. For tests.
func (p *PartitionEnsurer) Iterations() int64 { return p.iterations.Load() }

// Errors returns the cumulative number of failed ensurer cycles.
func (p *PartitionEnsurer) Errors() int64 { return p.errors.Load() }

// Run blocks until ctx is cancelled.
func (p *PartitionEnsurer) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	runSchedulerCycleCheckIn(ctx, p.interval, func() {
		_ = p.runOnce(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, p.interval, func() {
				_ = p.runOnce(ctx)
			})
		}
	}
}

// RunOnceForTest is an exported shim for integration tests.
func (p *PartitionEnsurer) RunOnceForTest(ctx context.Context) error {
	return p.runOnce(ctx)
}

func (p *PartitionEnsurer) runOnce(ctx context.Context) error {
	defer func() {
		p.iterations.Add(1)
		if r := recover(); r != nil {
			p.logger.Warn("partition ensurer panic recovered", "panic", r)
			p.errors.Add(1)
		}
	}()

	if p.advisoryLocker != nil {
		acquired, err := p.advisoryLocker.TryAdvisoryLock(ctx, partitionEnsurerAdvisoryLockID)
		if err != nil {
			p.errors.Add(1)
			return err
		}
		if !acquired {
			return nil
		}
		defer func() {
			if err := p.advisoryLocker.ReleaseAdvisoryLock(ctx, partitionEnsurerAdvisoryLockID); err != nil {
				p.logger.Debug("partition ensurer lock release failed", "error", err)
			}
		}()
	}

	if err := p.store.EnsureJobRunsPartitions(ctx, p.monthsAhead); err != nil {
		p.logger.Warn("ensure partitions failed", "error", err)
		p.errors.Add(1)
		return err
	}
	if err := p.store.EnsureOutboxHistoryPartitions(ctx, p.monthsAhead); err != nil {
		p.logger.Warn("ensure outbox history partitions failed", "error", err)
		p.errors.Add(1)
		return err
	}
	return nil
}

// Compile-time assertion that *store.Queries satisfies the interface.
var _ PartitionEnsurerStore = (*store.Queries)(nil)
