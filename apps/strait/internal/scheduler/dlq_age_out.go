package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/queue"
)

// R3 Phase 5: DLQ age-out archiver.
//
// Phase 9 (Round 1) caps DLQ depth per job and per project. Rows stay
// forever, so day 90 looks like day 1 to the cap. A steady trickle of
// failures eventually saturates the cap even though most of the rows
// are stale and not useful for debugging.
//
// The age-out archiver soft-deletes DLQ rows older than a configurable
// retention via the visible_until column added in Round 1 Phase 7. The
// dlq_counts trigger from Phase 9 already decrements the counter on
// mask, so DLQ caps free up automatically.

const dlqAgeOutAdvisoryLockID int64 = 0x5374446C5130 // "StDlQ0"

// DLQAgeOutStore is the minimal store interface the archiver needs.
type DLQAgeOutStore interface {
	MaskOldDLQRows(ctx context.Context, retention time.Duration, limit int) (int64, error)
}

// DLQAgeOut archives stale dead_letter rows.
type DLQAgeOut struct {
	store          DLQAgeOutStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	retention      time.Duration
	batchLimit     int
	logger         *slog.Logger
	iterations     atomic.Int64
	masked         atomic.Int64
}

// DLQAgeOutConfig configures the archiver.
type DLQAgeOutConfig struct {
	Interval   time.Duration
	Retention  time.Duration
	BatchLimit int
	Logger     *slog.Logger
}

// NewDLQAgeOut builds the archiver with defaults.
func NewDLQAgeOut(s DLQAgeOutStore, cfg DLQAgeOutConfig) *DLQAgeOut {
	a := &DLQAgeOut{
		store:      s,
		interval:   cfg.Interval,
		retention:  cfg.Retention,
		batchLimit: cfg.BatchLimit,
		logger:     cfg.Logger,
	}
	if a.interval <= 0 {
		a.interval = 24 * time.Hour
	}
	if a.retention <= 0 {
		a.retention = 30 * 24 * time.Hour
	}
	if a.batchLimit <= 0 {
		a.batchLimit = 1000
	}
	if a.logger == nil {
		a.logger = slog.Default()
	}
	return a
}

// WithAdvisoryLocker enables single-leader execution.
func (a *DLQAgeOut) WithAdvisoryLocker(locker AdvisoryLocker) *DLQAgeOut {
	a.advisoryLocker = locker
	return a
}

func (a *DLQAgeOut) Iterations() int64  { return a.iterations.Load() }
func (a *DLQAgeOut) TotalMasked() int64 { return a.masked.Load() }

// Run blocks until ctx is cancelled.
func (a *DLQAgeOut) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	_ = a.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = a.runOnce(ctx)
		}
	}
}

// RunOnceForTest exposes a single pass to integration tests.
func (a *DLQAgeOut) RunOnceForTest(ctx context.Context) error {
	return a.runOnce(ctx)
}

func (a *DLQAgeOut) runOnce(ctx context.Context) error {
	defer func() {
		a.iterations.Add(1)
		if r := recover(); r != nil {
			a.logger.Warn("dlq age-out panic recovered", "panic", r)
		}
	}()

	if a.advisoryLocker != nil {
		acquired, err := a.advisoryLocker.TryAdvisoryLock(ctx, dlqAgeOutAdvisoryLockID)
		if err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		defer func() {
			if err := a.advisoryLocker.ReleaseAdvisoryLock(ctx, dlqAgeOutAdvisoryLockID); err != nil {
				a.logger.Debug("dlq age-out lock release failed", "error", err)
			}
		}()
	}

	n, err := a.store.MaskOldDLQRows(ctx, a.retention, a.batchLimit)
	if err != nil {
		return fmt.Errorf("mask old dlq rows: %w", err)
	}
	a.masked.Add(n)
	if n > 0 {
		a.logger.Info("dlq age-out masked rows", "count", n, "retention", a.retention)
	}
	// R4 Phase 11: sample the oldest unmasked DLQ row age so Grafana
	// can alert when age-out is falling behind.
	a.sampleOldestUnmaskedAge(ctx)
	return nil
}

func (a *DLQAgeOut) sampleOldestUnmaskedAge(ctx context.Context) {
	if a.store == nil {
		return
	}
	type ager interface {
		OldestUnmaskedDLQAge(ctx context.Context) (float64, error)
	}
	if s, ok := a.store.(ager); ok {
		age, err := s.OldestUnmaskedDLQAge(ctx)
		if err != nil {
			a.logger.Debug("dlq oldest age sample failed", "error", err)
			return
		}
		a.logger.Debug("dlq oldest unmasked age", "seconds", age)
		if qm, qmErr := queue.Metrics(); qmErr == nil && qm != nil && qm.DLQOldestUnmaskedAge != nil {
			qm.DLQOldestUnmaskedAge.Record(ctx, age)
		}
	}
}
