package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/store"
)

// PriorityPromoter bumps priority on queued runs older than a
// threshold, so starvation is handled outside the dequeue hot path. This
// replaces the mutable ORDER BY (priority + age/3600 DESC) which forced a
// sort over every queued row.
//
// The promoter is designed to be boring: it runs on a fixed interval, holds
// an advisory lock to guarantee single-leader execution across replicas,
// and issues one bounded UPDATE per tick.

// promoterAdvisoryLockID is distinct from the reaper's lock so both can run
// concurrently. 0x5374706F6D6F7465 = "Stpomote".
const promoterAdvisoryLockID int64 = 0x5374706F6D6F7465

// PriorityPromoter periodically bumps priority on aged queued runs.
type PriorityPromoter struct {
	db                 store.DBTX
	advisoryLocker     AdvisoryLocker
	interval           time.Duration
	ageThreshold       time.Duration
	maxPriority        int
	batchLimit         int
	logger             *slog.Logger
	iterationsExecuted atomic.Int64
	rowsPromoted       atomic.Int64
}

// PriorityPromoterConfig tunes the promoter. Zero-value fields fall back to
// sensible defaults.
type PriorityPromoterConfig struct {
	Interval     time.Duration
	AgeThreshold time.Duration
	MaxPriority  int
	BatchLimit   int
	Logger       *slog.Logger
}

// NewPriorityPromoter builds a promoter with the given config and defaults.
func NewPriorityPromoter(db store.DBTX, cfg PriorityPromoterConfig) *PriorityPromoter {
	p := &PriorityPromoter{
		db:           db,
		interval:     cfg.Interval,
		ageThreshold: cfg.AgeThreshold,
		maxPriority:  cfg.MaxPriority,
		batchLimit:   cfg.BatchLimit,
		logger:       cfg.Logger,
	}
	if p.interval <= 0 {
		p.interval = 60 * time.Second
	}
	if p.ageThreshold <= 0 {
		p.ageThreshold = 5 * time.Minute
	}
	if p.maxPriority <= 0 {
		p.maxPriority = 1000
	}
	if p.batchLimit <= 0 {
		p.batchLimit = 500
	}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	return p
}

// WithAdvisoryLocker enables single-leader promotion across replicas.
func (p *PriorityPromoter) WithAdvisoryLocker(locker AdvisoryLocker) *PriorityPromoter {
	p.advisoryLocker = locker
	return p
}

// Iterations returns the number of completed sample iterations. For tests.
func (p *PriorityPromoter) Iterations() int64 { return p.iterationsExecuted.Load() }

// RowsPromoted returns the cumulative number of rows whose priority was
// bumped. For tests.
func (p *PriorityPromoter) RowsPromoted() int64 { return p.rowsPromoted.Load() }

// Run blocks until ctx is cancelled. First iteration runs immediately so
// tests do not have to wait.
func (p *PriorityPromoter) Run(ctx context.Context) {
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

// runOnce executes one promotion cycle. Exposed for tests.
func (p *PriorityPromoter) runOnce(ctx context.Context) error {
	defer func() {
		p.iterationsExecuted.Add(1)
		if r := recover(); r != nil {
			p.logger.Warn("priority promoter panic recovered", "panic", r)
		}
	}()

	acquired, err := runWithOptionalAdvisoryLock(ctx, p.advisoryLocker, promoterAdvisoryLockID, p.runLocked)
	if err != nil {
		p.logger.Debug("priority promoter lock cycle failed", "error", err)
		return err
	}
	if !acquired {
		return nil
	}
	return nil
}

func (p *PriorityPromoter) runLocked(ctx context.Context) error {
	const q = `
WITH candidates AS (
    SELECT id
    FROM job_runs
    WHERE status = 'queued'
      AND priority < $1
      AND created_at < NOW() - make_interval(secs => $2)
    ORDER BY created_at ASC
    LIMIT $3
),
promoted AS (
    UPDATE job_runs
    SET priority = LEAST(priority + 1, $1)
    WHERE id IN (SELECT id FROM candidates)
      AND status = 'queued'
    RETURNING id, priority
)
UPDATE job_run_queue q
SET priority = promoted.priority
FROM promoted
WHERE q.run_id = promoted.id
`
	tag, err := p.db.Exec(ctx, q, p.maxPriority, p.ageThreshold.Seconds(), p.batchLimit)
	if err != nil {
		p.logger.Warn("priority promoter update failed", "error", err)
		return err
	}
	rows := tag.RowsAffected()
	p.rowsPromoted.Add(rows)
	if rows > 0 {
		p.logger.Debug("priority promoter bumped rows", "rows", rows)
	}
	return nil
}
