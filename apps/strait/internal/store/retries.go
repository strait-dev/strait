package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// Retry side table helpers.
//
// The job_retries table holds pending retries for runs that are currently
// in status 'queued'. Writing here instead of UPDATE job_runs SET
// next_retry_at keeps job_runs rows HOT-update eligible because the
// retry timestamp is not indexed on job_runs.
//
// Ownership:
//   - ScheduleRetry writes on failure handling.
//   - Dequeue anti-joins against this table so a row is not claimed
//     before its retry fires.
//   - DelayedPoller (future integration) walks this table to promote.

// ScheduleRetry upserts a retry record for the given run.
func (q *Queries) ScheduleRetry(ctx context.Context, runID string, at time.Time, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ScheduleRetry")
	defer span.End()

	const sql = `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (run_id) DO UPDATE
		  SET next_retry_at = EXCLUDED.next_retry_at,
		      attempt = EXCLUDED.attempt,
		      scheduled_at = NOW()`
	if _, err := q.db.Exec(ctx, sql, runID, at, attempt); err != nil {
		return fmt.Errorf("schedule retry: %w", err)
	}
	return nil
}

// ClearRetry removes a retry record, typically on successful claim or
// explicit cancellation.
func (q *Queries) ClearRetry(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClearRetry")
	defer span.End()

	if _, err := q.db.Exec(ctx, `DELETE FROM job_retries WHERE run_id = $1`, runID); err != nil {
		return fmt.Errorf("clear retry: %w", err)
	}
	return nil
}

// ReadyRetries returns up to limit run_ids whose next_retry_at has passed.
func (q *Queries) ReadyRetries(ctx context.Context, limit int) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReadyRetries")
	defer span.End()

	if limit <= 0 {
		limit = 500
	}
	rows, err := q.db.Query(ctx, `
		SELECT run_id FROM job_retries
		WHERE next_retry_at <= NOW()
		ORDER BY next_retry_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("ready retries: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CountPendingRetries returns the total size of the retry side table.
// For observability.
func (q *Queries) CountPendingRetries(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountPendingRetries")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `SELECT COUNT(*) FROM job_retries`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count retries: %w", err)
	}
	return count, nil
}
