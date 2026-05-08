package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alitto/pond/v2"

	"strait/internal/queue"
)

// dlqAgeOutScanPoolSize bounds the concurrent per-partition candidate
// scans. The subsequent UPDATE is single-threaded to avoid lock
// contention.
const dlqAgeOutScanPoolSize = 4

// DLQ age-out archiver.
//
// DLQ depth is capped per job and per project. Rows stay forever, so
// day 90 looks like day 1 to the cap. A steady trickle of failures
// eventually saturates the cap even though most of the rows are stale
// and not useful for debugging.
//
// The age-out archiver soft-deletes DLQ rows older than a configurable
// retention via the visible_until column. The dlq_counts trigger
// already decrements the counter on mask, so DLQ caps free up
// automatically.

const dlqAgeOutAdvisoryLockID int64 = 0x5374446C5130 // "StDlQ0"

// DLQAgeOutStore is the minimal store interface the archiver needs.
type DLQAgeOutStore interface {
	MaskOldDLQRows(ctx context.Context, retention time.Duration, limit int) (int64, error)
}

// DLQPartitionScanner is an optional extension to DLQAgeOutStore. When
// implemented, the archiver fans per-partition candidate scans out
// across a bounded pool before invoking the serial MaskOldDLQRows. The
// scan itself is a hint that primes caches / warms statistics — the
// canonical delete still runs through MaskOldDLQRows.
type DLQPartitionScanner interface {
	ListDLQPartitions(ctx context.Context) ([]string, error)
	ScanDLQPartitionCandidates(ctx context.Context, partition string, retention time.Duration, limit int) (int64, error)
}

// DLQAgeOut archives stale dead_letter rows.
type DLQAgeOut struct {
	store          DLQAgeOutStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	retention      time.Duration
	batchLimit     int
	logger         *slog.Logger
	// pool is allocated once in NewDLQAgeOut and reused across every
	// tick to avoid the per-iteration allocation churn of pond.NewPool.
	// Close tears it down cleanly at scheduler stop.
	pool       pond.Pool
	closeOnce  sync.Once
	iterations atomic.Int64
	masked     atomic.Int64
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
	a.pool = pond.NewPool(dlqAgeOutScanPoolSize)
	return a
}

// Close tears down the reusable pond pool. Safe to call multiple times.
func (a *DLQAgeOut) Close() {
	a.closeOnce.Do(func() {
		if a.pool != nil {
			a.pool.StopAndWait()
		}
	})
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
	defer a.Close()
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	runSchedulerCycleCheckIn(ctx, a.interval, func() {
		_ = a.runOnce(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, a.interval, func() {
				_ = a.runOnce(ctx)
			})
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

	a.scanPartitionsParallel(ctx)

	n, err := a.store.MaskOldDLQRows(ctx, a.retention, a.batchLimit)
	if err != nil {
		return fmt.Errorf("mask old dlq rows: %w", err)
	}
	a.masked.Add(n)
	if n > 0 {
		a.logger.Info("dlq age-out masked rows", "count", n, "retention", a.retention)
	}
	// Sample the oldest unmasked DLQ row age so Grafana
	// can alert when age-out is falling behind.
	a.sampleOldestUnmaskedAge(ctx)
	return nil
}

// scanPartitionsParallel runs per-partition read-only candidate scans
// through a bounded pond pool. It logs candidate counts but does not
// mutate any row — the canonical MaskOldDLQRows update runs serially
// afterwards.
func (a *DLQAgeOut) scanPartitionsParallel(ctx context.Context) {
	scanner, ok := a.store.(DLQPartitionScanner)
	if !ok {
		return
	}
	parts, err := scanner.ListDLQPartitions(ctx)
	if err != nil {
		a.logger.Debug("list dlq partitions failed", "error", err)
		return
	}
	if len(parts) == 0 {
		return
	}
	var candidates atomic.Int64
	group := a.pool.NewGroup()
	for _, p := range parts {
		group.Submit(func() {
			if ctx.Err() != nil {
				return
			}
			n, err := scanner.ScanDLQPartitionCandidates(ctx, p, a.retention, a.batchLimit)
			if err != nil {
				a.logger.Debug("dlq partition scan failed", "partition", p, "error", err)
				return
			}
			candidates.Add(n)
		})
	}
	_ = group.Wait()
	a.logger.Debug("dlq partition scan complete", "partitions", len(parts), "candidates", candidates.Load())
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
