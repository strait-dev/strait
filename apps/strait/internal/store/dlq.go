package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// DLQ admin helpers power the /v1/admin/dlq HTTP endpoints. Listing and
// replay reuse the existing ListDeadLetterRuns / ReplayDeadLetterRun
// helpers on Queries; this file adds the mutations that did not previously
// have dedicated entry points (unmask + purge).

// UnmaskDLQRun clears visible_until on a dead-letter run so it is no
// longer hidden by the DLQ age-out process. The run must already be in
// dead_letter status; otherwise ErrRunConflict is returned. Returns
// ErrRunNotFound if the run does not exist.
func (q *Queries) UnmaskDLQRun(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UnmaskDLQRun")
	defer span.End()

	var status domain.RunStatus
	err := q.db.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = $1`, runID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("unmask dlq run: load status: %w", err)
	}
	if status != domain.StatusDeadLetter {
		return fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, status)
	}

	tag, err := q.db.Exec(ctx, `UPDATE job_runs SET visible_until = NULL WHERE id = $1 AND status = 'dead_letter'`, runID)
	if err != nil {
		return fmt.Errorf("unmask dlq run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

// PurgeDLQRun hard-deletes a dead-letter run. Requires the caller to
// already hold the dlq:purge scope. Returns ErrRunNotFound if the run
// does not exist or is not in dead_letter status.
func (q *Queries) PurgeDLQRun(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PurgeDLQRun")
	defer span.End()

	var status domain.RunStatus
	err := q.db.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = $1`, runID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("purge dlq run: load status: %w", err)
	}
	if status != domain.StatusDeadLetter {
		return fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, status)
	}

	tag, err := q.db.Exec(ctx, `DELETE FROM job_runs WHERE id = $1 AND status = 'dead_letter'`, runID)
	if err != nil {
		return fmt.Errorf("purge dlq run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

// MarkRunReplayed sets replayed_run_id on an original dead-letter run to
// record the lineage of a replay. Called by the admin DLQ replay handler
// after a successful re-enqueue. replayedByRunID may equal originalRunID
// when the existing ReplayDeadLetterRun behavior reuses the same run ID.
func (q *Queries) MarkRunReplayed(ctx context.Context, originalRunID, replayedByRunID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkRunReplayed")
	defer span.End()

	tag, err := q.db.Exec(ctx, `UPDATE job_runs SET replayed_run_id = $1 WHERE id = $2`, replayedByRunID, originalRunID)
	if err != nil {
		return fmt.Errorf("mark run replayed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}
