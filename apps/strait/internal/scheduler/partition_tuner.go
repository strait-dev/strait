package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alitto/pond/v2"

	"strait/internal/store"
)

// partitionTunerPoolSize bounds the concurrent ALTER TABLE calls. Four
// is enough to overlap IO without saturating the DB lock manager.
const partitionTunerPoolSize = 4

// Per-partition autovacuum tuner.
//
// Migration 180 sets aggressive autovacuum on the job_runs
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
	// pool is allocated once in NewPartitionTuner and reused across
	// every tick to avoid the per-iteration allocation churn of
	// pond.NewPool. Close tears it down cleanly at scheduler stop.
	pool       pond.Pool
	closeOnce  sync.Once
	iterations atomic.Int64
	hotCount   atomic.Int64
	coldCount  atomic.Int64
}

// PartitionTunerStore is the minimal store interface the tuner needs.
type PartitionTunerStore interface {
	ListJobRunsPartitions(ctx context.Context) ([]string, error)
	ExecDDL(ctx context.Context, sql string) error
	PartitionExists(ctx context.Context, name string) (bool, error)
	PartitionReloption(ctx context.Context, name, option string) (string, error)
}

// jobRunsFillfactor is the page-fill target for partitions: leaves 15% free
// space so HOT updates can succeed on hot rows. New pg_partman partitions
// inherit the parent's fillfactor=100, so the tuner backfills this on every
// partition it sees, idempotently.
const jobRunsFillfactor = "85"

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
	t.pool = pond.NewPool(partitionTunerPoolSize)
	return t
}

// Close tears down the reusable pond pool. Safe to call multiple times;
// subsequent runOnce invocations after Close will no-op their submits.
func (t *PartitionTuner) Close() {
	t.closeOnce.Do(func() {
		if t.pool != nil {
			t.pool.StopAndWait()
		}
	})
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
	defer t.Close()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	runSchedulerCycleCheckIn(ctx, t.interval, func() {
		_ = t.runOnce(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, t.interval, func() {
				_ = t.runOnce(ctx)
			})
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

	acquired, err := runWithOptionalAdvisoryLock(ctx, t.advisoryLocker, partitionTunerAdvisoryLockID, t.runLocked)
	if err != nil || !acquired {
		return err
	}
	return nil
}

func (t *PartitionTuner) runLocked(ctx context.Context) error {
	partitions, err := t.store.ListJobRunsPartitions(ctx)
	if err != nil {
		return fmt.Errorf("list partitions: %w", err)
	}

	hot := hotPartitionNames(t.clock())

	// Apply per-partition ALTER TABLE calls through a bounded
	// pond pool so a long list does not serialize behind each DDL round
	// trip. The work is ordering-independent; we aggregate counters via
	// atomics and log errors per partition. The pool is owned by the
	// tuner (allocated in NewPartitionTuner, torn down in Close) so we
	// reuse it across ticks instead of reallocating every iteration.
	var hotN, coldN atomic.Int64
	group := t.pool.NewGroup()
	for _, p := range partitions {
		group.Submit(func() {
			if ctx.Err() != nil {
				return
			}
			exists, err := t.store.PartitionExists(ctx, p)
			if err != nil {
				t.logger.Warn("partition existence check failed", "partition", p, "error", err)
				return
			}
			if !exists {
				t.logger.Info("partition vanished between list and alter, skipping", "partition", p)
				return
			}
			if _, isHot := hot[p]; isHot {
				sql, err := hotSettingsSQL(p)
				if err != nil {
					t.logger.Warn("build hot settings SQL failed", "partition", p, "error", err)
					return
				}
				if err := t.store.ExecDDL(ctx, sql); err != nil {
					t.logger.Warn("apply hot settings failed", "partition", p, "error", err)
					return
				}
				hotN.Add(1)
			} else {
				sql, err := resetSettingsSQL(p)
				if err != nil {
					t.logger.Warn("build reset settings SQL failed", "partition", p, "error", err)
					return
				}
				if err := t.store.ExecDDL(ctx, sql); err != nil {
					t.logger.Warn("reset settings failed", "partition", p, "error", err)
					return
				}
				coldN.Add(1)
			}

			// Backfill fillfactor=85 on partitions that inherited the parent's
			// default. Skipping the ALTER when already set avoids the brief
			// AccessExclusiveLock taken by ALTER TABLE on every weekly tick.
			current, err := t.store.PartitionReloption(ctx, p, "fillfactor")
			if err != nil {
				t.logger.Debug("read fillfactor failed", "partition", p, "error", err)
				return
			}
			if current == jobRunsFillfactor {
				return
			}
			ffSQL, err := fillfactorSQL(p)
			if err != nil {
				t.logger.Warn("build fillfactor SQL failed", "partition", p, "error", err)
				return
			}
			if err := t.store.ExecDDL(ctx, ffSQL); err != nil {
				t.logger.Warn("apply fillfactor failed", "partition", p, "error", err)
			}
		})
	}
	_ = group.Wait()
	t.hotCount.Store(hotN.Load())
	t.coldCount.Store(coldN.Load())
	t.logger.Info("partition tuner complete",
		"hot", hotN.Load(), "cold", coldN.Load(), "total", len(partitions),
	)
	return nil
}

// hotPartitionNames returns the set of partition names that should be
// treated as "hot" relative to the given clock. Hot = current month and
// the immediately preceding month.
func hotPartitionNames(now time.Time) map[string]struct{} {
	out := make(map[string]struct{}, 2)
	cur := now.UTC()
	currentMonth := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, time.UTC)
	prev := currentMonth.AddDate(0, -1, 0)
	out[fmt.Sprintf("job_runs_p%04d_%02d", cur.Year(), int(cur.Month()))] = struct{}{}
	out[fmt.Sprintf("job_runs_p%04d_%02d", prev.Year(), int(prev.Month()))] = struct{}{}
	return out
}

// hotSettingsSQL/resetSettingsSQL/fillfactorSQL return an error when the
// partition identifier fails validation instead of returning an empty string.
// An empty string previously made the caller's ExecDDL(ctx, "") fail with a
// generic Postgres "empty query" error, masking the real identifier-validation
// failure.
func hotSettingsSQL(partition string) (string, error) {
	quoted, err := store.SafeQuoteIdent(partition)
	if err != nil {
		return "", fmt.Errorf("quote partition identifier: %w", err)
	}
	return fmt.Sprintf(`ALTER TABLE %s SET (
        autovacuum_vacuum_scale_factor = 0.01,
        autovacuum_analyze_scale_factor = 0.005,
        autovacuum_vacuum_cost_delay = 2,
        autovacuum_vacuum_cost_limit = 1000,
        autovacuum_vacuum_insert_scale_factor = 0.01
    )`, quoted), nil
}

func fillfactorSQL(partition string) (string, error) {
	quoted, err := store.SafeQuoteIdent(partition)
	if err != nil {
		return "", fmt.Errorf("quote partition identifier: %w", err)
	}
	return fmt.Sprintf(`ALTER TABLE %s SET (fillfactor = %s)`, quoted, jobRunsFillfactor), nil
}

func resetSettingsSQL(partition string) (string, error) {
	quoted, err := store.SafeQuoteIdent(partition)
	if err != nil {
		return "", fmt.Errorf("quote partition identifier: %w", err)
	}
	return fmt.Sprintf(`ALTER TABLE %s RESET (
        autovacuum_vacuum_scale_factor,
        autovacuum_analyze_scale_factor,
        autovacuum_vacuum_cost_delay,
        autovacuum_vacuum_cost_limit,
        autovacuum_vacuum_insert_scale_factor
    )`, quoted), nil
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
