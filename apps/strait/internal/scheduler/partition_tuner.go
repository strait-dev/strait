package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

// R3 Phase 4: per-partition autovacuum tuner.
//
// Migration 180 (Round 1) sets aggressive autovacuum on the job_runs
// parent, which propagates to every partition including cold ones that
// see almost no UPDATEs. This wastes vacuum worker cycles on partitions
// that don't need them and starves the hot partition of attention.
//
// This tuner walks the partition list weekly and applies:
//   - Hot settings to the current month's partition and the previous one.
//   - Reset to defaults on every older (cold) partition.
//
// The "hot" set rotates naturally as the month advances.

const partitionTunerAdvisoryLockID int64 = 0x5453745062546E00 // "StSpBTn"

// PartitionTuner periodically re-tunes job_runs partitions.
type PartitionTuner struct {
	store          PartitionTunerStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	clock          func() time.Time
	logger         *slog.Logger
	iterations     atomic.Int64
	hotCount       atomic.Int64
	coldCount      atomic.Int64
}

// PartitionTunerStore is the minimal store interface the tuner needs.
type PartitionTunerStore interface {
	ListJobRunsPartitions(ctx context.Context) ([]string, error)
	ExecDDL(ctx context.Context, sql string) error
}

// PartitionTunerConfig configures the tuner.
type PartitionTunerConfig struct {
	Interval time.Duration
	Clock    func() time.Time
	Logger   *slog.Logger
}

// NewPartitionTuner builds the tuner with defaults.
func NewPartitionTuner(s PartitionTunerStore, cfg PartitionTunerConfig) *PartitionTuner {
	t := &PartitionTuner{
		store:    s,
		interval: cfg.Interval,
		clock:    cfg.Clock,
		logger:   cfg.Logger,
	}
	if t.interval <= 0 {
		t.interval = 7 * 24 * time.Hour
	}
	if t.clock == nil {
		t.clock = time.Now
	}
	if t.logger == nil {
		t.logger = slog.Default()
	}
	return t
}

// WithAdvisoryLocker enables single-leader execution.
func (t *PartitionTuner) WithAdvisoryLocker(locker AdvisoryLocker) *PartitionTuner {
	t.advisoryLocker = locker
	return t
}

func (t *PartitionTuner) Iterations() int64 { return t.iterations.Load() }
func (t *PartitionTuner) HotCount() int     { return int(t.hotCount.Load()) }
func (t *PartitionTuner) ColdCount() int    { return int(t.coldCount.Load()) }

// Run blocks until ctx is cancelled.
func (t *PartitionTuner) Run(ctx context.Context) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	_ = t.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = t.runOnce(ctx)
		}
	}
}

// RunOnceForTest exposes a single tuning cycle to integration tests.
func (t *PartitionTuner) RunOnceForTest(ctx context.Context) error {
	return t.runOnce(ctx)
}

func (t *PartitionTuner) runOnce(ctx context.Context) error {
	defer func() {
		t.iterations.Add(1)
		if r := recover(); r != nil {
			t.logger.Warn("partition tuner panic recovered", "panic", r)
		}
	}()

	if t.advisoryLocker != nil {
		acquired, err := t.advisoryLocker.TryAdvisoryLock(ctx, partitionTunerAdvisoryLockID)
		if err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		defer func() {
			if err := t.advisoryLocker.ReleaseAdvisoryLock(ctx, partitionTunerAdvisoryLockID); err != nil {
				t.logger.Debug("tuner lock release failed", "error", err)
			}
		}()
	}

	partitions, err := t.store.ListJobRunsPartitions(ctx)
	if err != nil {
		return fmt.Errorf("list partitions: %w", err)
	}

	hot := hotPartitionNames(t.clock())
	var hotN, coldN int64
	for _, p := range partitions {
		if _, isHot := hot[p]; isHot {
			if err := t.store.ExecDDL(ctx, hotSettingsSQL(p)); err != nil {
				t.logger.Warn("apply hot settings failed", "partition", p, "error", err)
				continue
			}
			hotN++
		} else {
			if err := t.store.ExecDDL(ctx, resetSettingsSQL(p)); err != nil {
				t.logger.Warn("reset settings failed", "partition", p, "error", err)
				continue
			}
			coldN++
		}
	}
	t.hotCount.Store(hotN)
	t.coldCount.Store(coldN)
	t.logger.Info("partition tuner complete",
		"hot", hotN, "cold", coldN, "total", len(partitions),
	)
	return nil
}

// hotPartitionNames returns the set of partition names that should be
// treated as "hot" relative to the given clock. Hot = current month and
// the immediately preceding month.
func hotPartitionNames(now time.Time) map[string]struct{} {
	out := make(map[string]struct{}, 2)
	cur := now.UTC()
	prev := cur.AddDate(0, -1, 0)
	out[fmt.Sprintf("job_runs_p%04d_%02d", cur.Year(), int(cur.Month()))] = struct{}{}
	out[fmt.Sprintf("job_runs_p%04d_%02d", prev.Year(), int(prev.Month()))] = struct{}{}
	return out
}

func hotSettingsSQL(partition string) string {
	return fmt.Sprintf(`ALTER TABLE %s SET (
        autovacuum_vacuum_scale_factor = 0.01,
        autovacuum_analyze_scale_factor = 0.005,
        autovacuum_vacuum_cost_delay = 2,
        autovacuum_vacuum_insert_scale_factor = 0.01
    )`, quoteIdent(partition))
}

func resetSettingsSQL(partition string) string {
	return fmt.Sprintf(`ALTER TABLE %s RESET (
        autovacuum_vacuum_scale_factor,
        autovacuum_analyze_scale_factor,
        autovacuum_vacuum_cost_delay,
        autovacuum_vacuum_insert_scale_factor
    )`, quoteIdent(partition))
}

func quoteIdent(s string) string {
	return `"` + s + `"`
}

// parsePartitionMonth extracts (year, month) from names like
// "job_runs_p2026_04". Returns (0, 0) on parse failure.
var partitionNameRE = regexp.MustCompile(`^job_runs_p(\d{4})_(\d{2})$`)

func parsePartitionMonth(name string) (int, int) {
	m := partitionNameRE.FindStringSubmatch(name)
	if len(m) != 3 {
		return 0, 0
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	return y, mo
}
