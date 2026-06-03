package queue

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func (q *PgQueQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	ready := normalizePgQueEnqueueStatus(run)
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		if err := q.runWriter.Enqueue(ctx, run); err != nil {
			return err
		}
		if ready {
			if err := q.sendReadyEvent(ctx, q.db, run); err != nil {
				return err
			}
		}
		return nil
	}

	if ready {
		if err := q.ensureRunRouteCached(ctx, run); err != nil {
			return err
		}
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgque enqueue: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := q.EnqueueInTx(ctx, tx, run); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgque enqueue: commit: %w", err)
	}
	return nil
}

func (q *PgQueQueue) EnqueueInTx(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	ready := normalizePgQueEnqueueStatus(run)
	if err := q.markPgQueStorage(ctx, tx); err != nil {
		return err
	}
	if err := q.runWriter.EnqueueInTx(ctx, tx, run); err != nil {
		return err
	}
	if !ready {
		return nil
	}
	return q.sendReadyEvent(ctx, tx, run)
}

func (q *PgQueQueue) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	if len(runs) == 0 {
		return 0, nil
	}
	normalizePgQueEnqueueStatuses(runs)
	if err := q.ensureRunRoutesCached(ctx, runs); err != nil {
		return 0, err
	}
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		inserted, err := q.runWriter.EnqueueBatch(ctx, runs)
		if err != nil {
			return 0, err
		}
		if err := q.sendReadyEvents(ctx, q.db, runs); err != nil {
			return 0, err
		}
		return inserted, nil
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("pgque enqueue batch: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRunWriter := NewPostgresRunWriter(tx)
	if err := q.markPgQueStorage(ctx, tx); err != nil {
		return 0, err
	}
	inserted, err := txRunWriter.EnqueueBatch(ctx, runs)
	if err != nil {
		return 0, err
	}
	if err := q.sendReadyEvents(ctx, tx, runs); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("pgque enqueue batch: commit: %w", err)
	}
	return inserted, nil
}

func normalizePgQueEnqueueStatus(run *domain.JobRun) bool {
	if run == nil {
		return false
	}
	if run.Status != "" && run.Status != domain.StatusQueued {
		return false
	}
	if run.ScheduledAt != nil && run.ScheduledAt.After(time.Now()) {
		run.Status = domain.StatusDelayed
		return false
	}
	run.Status = domain.StatusQueued
	return true
}

func normalizePgQueEnqueueStatuses(runs []*domain.JobRun) {
	for _, run := range runs {
		normalizePgQueEnqueueStatus(run)
	}
}

func (q *PgQueQueue) EnqueueExisting(ctx context.Context, run *domain.JobRun) error {
	if run == nil || run.Status != domain.StatusQueued {
		return nil
	}
	if err := q.ensureRunRouteCached(ctx, run); err != nil {
		return err
	}
	if err := q.sendReadyEvent(ctx, q.db, run); err != nil {
		return err
	}
	return q.tickReadyRoute(ctx, run)
}
