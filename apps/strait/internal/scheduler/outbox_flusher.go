package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type outboxEnqueueDisposition int

const (
	outboxEnqueueRetryable outboxEnqueueDisposition = iota
	outboxEnqueueTerminal
)

// Outbox flusher.
//
// Promotes pending enqueue_outbox rows into job_runs. Each tick opens a
// transaction, claims up to BatchSize rows via SKIP LOCKED, enqueues each
// as a JobRun, marks them consumed, and commits. On any individual row
// error the row is skipped (logged) and the rest of the batch proceeds.

// OutboxFlusher promotes outbox entries into job_runs.
type OutboxFlusher struct {
	pool                  *pgxpool.Pool
	queue                 queue.Queue
	interval              time.Duration
	batchSize             int
	engine                string
	leaseDuration         time.Duration
	reclaimInterval       time.Duration
	logger                *slog.Logger
	iterations            atomic.Int64
	flushed               atomic.Int64
	errors                atomic.Int64
	lastReclaimUnixNano   atomic.Int64
	reclaimedExpiredLease atomic.Int64
}

// OutboxFlusherConfig configures the flusher.
type OutboxFlusherConfig struct {
	Interval        time.Duration
	BatchSize       int
	Engine          string
	LeaseDuration   time.Duration
	ReclaimInterval time.Duration
	Logger          *slog.Logger
}

// NewOutboxFlusher builds a flusher. Zero values fall back to defaults.
func NewOutboxFlusher(pool *pgxpool.Pool, q queue.Queue, cfg OutboxFlusherConfig) *OutboxFlusher {
	f := &OutboxFlusher{
		pool:            pool,
		queue:           q,
		interval:        cfg.Interval,
		batchSize:       cfg.BatchSize,
		engine:          cfg.Engine,
		leaseDuration:   cfg.LeaseDuration,
		reclaimInterval: cfg.ReclaimInterval,
		logger:          cfg.Logger,
	}
	if f.interval <= 0 {
		f.interval = time.Second
	}
	if f.batchSize <= 0 {
		f.batchSize = 500
	}
	if f.logger == nil {
		f.logger = slog.Default()
	}
	if f.engine == "" {
		f.engine = "legacy"
	}
	if f.leaseDuration <= 0 {
		f.leaseDuration = 30 * time.Second
	}
	if f.reclaimInterval <= 0 {
		f.reclaimInterval = f.leaseDuration
	}
	return f
}

func (f *OutboxFlusher) Iterations() int64 { return f.iterations.Load() }
func (f *OutboxFlusher) Flushed() int64    { return f.flushed.Load() }
func (f *OutboxFlusher) Errors() int64     { return f.errors.Load() }
func (f *OutboxFlusher) ReclaimedExpiredLeases() int64 {
	return f.reclaimedExpiredLease.Load()
}

// Run blocks until ctx is cancelled.
func (f *OutboxFlusher) Run(ctx context.Context) {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	if err := runSchedulerCycleCheckInWithError(ctx, f.interval, func() error {
		return f.flushOnce(ctx)
	}); err != nil {
		f.logger.Warn("outbox flusher cycle failed", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := runSchedulerCycleCheckInWithError(ctx, f.interval, func() error {
				return f.flushOnce(ctx)
			}); err != nil {
				f.logger.Warn("outbox flusher cycle failed", "error", err)
			}
		}
	}
}

// FlushOnceForTest exposes a single flush for integration tests.
func (f *OutboxFlusher) FlushOnceForTest(ctx context.Context) error {
	return f.flushOnce(ctx)
}

func (f *OutboxFlusher) flushOnce(ctx context.Context) (err error) {
	defer func() {
		f.iterations.Add(1)
		if r := recover(); r != nil {
			f.errors.Add(1)
			f.logger.Warn("outbox flusher panic recovered", "panic", r)
			err = fmt.Errorf("outbox flusher panic: %v", r)
		}
	}()

	tx, err := f.pool.Begin(ctx)
	if err != nil {
		f.errors.Add(1)
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := f.claimRows(ctx, tx)
	if err != nil {
		f.errors.Add(1)
		return err
	}
	if len(rows) == 0 {
		return tx.Commit(ctx)
	}

	promoted := make([]string, 0, len(rows))
	qm, _ := queue.Metrics()
	for i, row := range rows {
		savepoint := fmt.Sprintf("outbox_row_%d", i)
		if err := execSavepoint(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
			f.errors.Add(1)
			return fmt.Errorf("outbox flusher: create savepoint for row %s: %w", row.ID, err)
		}

		run, err := f.toJobRun(row)
		if err != nil {
			f.errors.Add(1)
			if rollbackErr := rollbackAndReleaseSavepoint(ctx, tx, savepoint); rollbackErr != nil {
				return fmt.Errorf("outbox flusher: rollback failed row %s: %w", row.ID, rollbackErr)
			}
			msg := store.TruncateOutboxError(err.Error())
			if markErr := store.MarkOutboxErroredInTx(ctx, tx, row.ID, msg); markErr != nil {
				return fmt.Errorf("outbox flusher: quarantine row %s: %w", row.ID, markErr)
			}
			if markErr := f.ackClaim(ctx, tx, row.ID); markErr != nil {
				return fmt.Errorf("outbox flusher: ack quarantined row %s: %w", row.ID, markErr)
			}
			if qm != nil && qm.OutboxQuarantinedTotal != nil {
				qm.OutboxQuarantinedTotal.Add(ctx, 1)
			}
			f.logger.Warn("outbox flusher: invalid row metadata, row quarantined",
				"outbox_id", row.ID, "job_id", row.JobID, "error", msg,
			)
			continue
		}
		if err := f.queue.EnqueueInTx(ctx, tx, run); err != nil {
			f.errors.Add(1)
			if rollbackErr := rollbackAndReleaseSavepoint(ctx, tx, savepoint); rollbackErr != nil {
				return fmt.Errorf("outbox flusher: rollback failed row %s: %w", row.ID, rollbackErr)
			}
			if classifyOutboxEnqueueError(err) == outboxEnqueueRetryable {
				f.releaseRetryableClaim(ctx, tx, row.ID)
				f.logger.Warn("outbox flusher: enqueue failed, will retry",
					"outbox_id", row.ID, "job_id", row.JobID, "error", err,
				)
				continue
			}
			msg := store.TruncateOutboxError(err.Error())
			if markErr := store.MarkOutboxErroredInTx(ctx, tx, row.ID, msg); markErr != nil {
				return fmt.Errorf("outbox flusher: quarantine row %s: %w", row.ID, markErr)
			}
			if markErr := f.ackClaim(ctx, tx, row.ID); markErr != nil {
				return fmt.Errorf("outbox flusher: ack quarantined row %s: %w", row.ID, markErr)
			}
			if qm != nil && qm.OutboxQuarantinedTotal != nil {
				qm.OutboxQuarantinedTotal.Add(ctx, 1)
			}
			f.logger.Warn("outbox flusher: enqueue failed, row quarantined",
				"outbox_id", row.ID, "job_id", row.JobID, "error", msg,
			)
			continue
		}
		if err := execSavepoint(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			f.errors.Add(1)
			return fmt.Errorf("outbox flusher: release savepoint for row %s: %w", row.ID, err)
		}
		if qm != nil && qm.OutboxLag != nil && !row.CreatedAt.IsZero() {
			qm.OutboxLag.Record(ctx, time.Since(row.CreatedAt).Seconds())
		}
		promoted = append(promoted, row.ID)
	}
	if len(promoted) > 0 {
		if err := f.markPromoted(ctx, tx, promoted); err != nil {
			f.errors.Add(1)
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		f.errors.Add(1)
		return err
	}
	f.flushed.Add(int64(len(promoted)))
	if len(promoted) > 0 {
		f.logger.Debug("outbox flusher promoted", "count", len(promoted))
	}
	return nil
}

func (f *OutboxFlusher) claimRows(ctx context.Context, tx pgx.Tx) ([]store.OutboxRow, error) {
	if f.engine == queue.EngineBatchlog {
		if err := f.reclaimExpiredClaimsIfDue(ctx, tx); err != nil {
			return nil, err
		}
		return store.ClaimOutboxBatchlogInTx(ctx, tx, f.batchSize, "outbox-flusher", f.leaseDuration)
	}
	return store.ClaimUnconsumedOutboxInTx(ctx, tx, f.batchSize)
}

func (f *OutboxFlusher) releaseRetryableClaim(ctx context.Context, tx pgx.Tx, id string) {
	if f.engine == queue.EngineBatchlog {
		_ = store.MarkOutboxClaimsReadyInTx(ctx, tx, []string{id})
	}
}

func (f *OutboxFlusher) ackClaim(ctx context.Context, tx pgx.Tx, id string) error {
	if f.engine != queue.EngineBatchlog {
		return nil
	}
	return store.MarkOutboxClaimsAckedInTx(ctx, tx, []string{id})
}

func (f *OutboxFlusher) markPromoted(ctx context.Context, tx pgx.Tx, ids []string) error {
	if f.engine == queue.EngineBatchlog {
		return store.MarkOutboxClaimsAckedInTx(ctx, tx, ids)
	}
	return store.MarkOutboxConsumedInTx(ctx, tx, ids)
}

func (f *OutboxFlusher) reclaimExpiredClaimsIfDue(ctx context.Context, tx pgx.Tx) error {
	if f.reclaimInterval <= 0 {
		return nil
	}
	now := time.Now()
	nowNano := now.UnixNano()
	for {
		last := f.lastReclaimUnixNano.Load()
		if last > 0 && now.Sub(time.Unix(0, last)) < f.reclaimInterval {
			return nil
		}
		if !f.lastReclaimUnixNano.CompareAndSwap(last, nowNano) {
			continue
		}
		reclaimed, err := store.ReclaimExpiredOutboxBatchlogClaimsInTx(ctx, tx)
		if err != nil {
			return err
		}
		if reclaimed > 0 {
			f.reclaimedExpiredLease.Add(reclaimed)
		}
		return nil
	}
}

func classifyOutboxEnqueueError(err error) outboxEnqueueDisposition {
	if err == nil {
		return outboxEnqueueRetryable
	}
	if errors.Is(err, domain.ErrIdempotencyConflict) {
		return outboxEnqueueTerminal
	}
	if errors.Is(err, queue.ErrEnqueueThrottled) {
		return outboxEnqueueRetryable
	}
	if _, ok := queue.AsTerminalEnqueue(err); ok {
		return outboxEnqueueTerminal
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return outboxEnqueueRetryable
	}
	if pgconn.Timeout(err) || pgconn.SafeToRetry(err) {
		return outboxEnqueueRetryable
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01", "55P03", "57014":
			return outboxEnqueueRetryable
		}
		if len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08" {
			return outboxEnqueueRetryable
		}
		if len(pgErr.Code) >= 2 && (pgErr.Code[:2] == "22" || pgErr.Code[:2] == "23") {
			return outboxEnqueueTerminal
		}
	}

	return outboxEnqueueRetryable
}

func execSavepoint(ctx context.Context, tx pgx.Tx, sql string) error {
	_, err := tx.Exec(ctx, sql)
	return err
}

func rollbackAndReleaseSavepoint(ctx context.Context, tx pgx.Tx, name string) error {
	if err := execSavepoint(ctx, tx, "ROLLBACK TO SAVEPOINT "+name); err != nil {
		return err
	}
	return execSavepoint(ctx, tx, "RELEASE SAVEPOINT "+name)
}

func (f *OutboxFlusher) toJobRun(row store.OutboxRow) (*domain.JobRun, error) {
	run := &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         row.JobID,
		ProjectID:     row.ProjectID,
		Payload:       row.Payload,
		Priority:      row.Priority,
		ScheduledAt:   row.ScheduledAt,
		TriggeredBy:   domain.TriggerManual,
		ExecutionMode: domain.ExecutionMode(row.ExecutionMode),
		QueueName:     row.QueueName,
	}
	if row.IdempotencyKey != nil {
		run.IdempotencyKey = *row.IdempotencyKey
	}
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &run.Metadata); err != nil {
			return nil, fmt.Errorf("decode outbox metadata for row %s: %w", row.ID, err)
		}
	}
	return run, nil
}
