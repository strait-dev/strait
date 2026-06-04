package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// Retry side table helpers.
//
// The job_retries table is append-only. The latest row for a run wins;
// clearing a retry appends a tombstone row. Writing here instead of UPDATE
// job_runs SET next_retry_at keeps job_runs rows HOT-update eligible, and
// appending instead of upserting avoids dead-tuple churn in the retry table.
//
// Ownership:
//   - ScheduleRetry writes on failure handling.
//   - Dequeue anti-joins against this table so a row is not claimed
//     before its retry fires.
//   - DelayedPoller (future integration) walks this table to promote.

// ScheduleRetry appends a retry record for the given run unless the latest
// retry row already represents the same schedule.
func (q *Queries) ScheduleRetry(ctx context.Context, runID string, at time.Time, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ScheduleRetry")
	defer span.End()

	const sql = `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
		SELECT $1, $2, $3, NOW(), FALSE
		WHERE NOT EXISTS (
		    SELECT 1
		    FROM job_retries r
		    WHERE r.run_id = $1
		      AND r.cleared = FALSE
		      AND r.next_retry_at IS NOT DISTINCT FROM $2
		      AND r.attempt = $3
		      AND NOT EXISTS (
		          SELECT 1
		          FROM job_retries newer
		          WHERE newer.run_id = r.run_id
		            AND newer.id > r.id
		      )
		)`
	if _, err := q.db.Exec(ctx, sql, runID, at, attempt); err != nil {
		return fmt.Errorf("schedule retry: %w", err)
	}
	return nil
}

// ClearRetry appends a tombstone record, typically on successful claim or
// explicit cancellation.
func (q *Queries) ClearRetry(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClearRetry")
	defer span.End()

	if _, err := q.db.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
		SELECT $1, NULL::timestamptz, 0, NOW(), TRUE
		WHERE EXISTS (
		    SELECT 1
		    FROM job_retries r
		    WHERE r.run_id = $1
		      AND r.cleared = FALSE
		      AND NOT EXISTS (
		          SELECT 1
		          FROM job_retries newer
		          WHERE newer.run_id = r.run_id
		            AND newer.id > r.id
		      )
		)`, runID); err != nil {
		return fmt.Errorf("clear retry: %w", err)
	}
	return nil
}

// ClearRetries appends tombstone records for a batch of run IDs. Used by the
// dequeue path to clear the side-table retry state once a run is claimed.
func (q *Queries) ClearRetries(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClearRetries")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
		SELECT DISTINCT input.run_id, NULL::timestamptz, 0, NOW(), TRUE
		FROM unnest($1::text[]) AS input(run_id)
		WHERE EXISTS (
		    SELECT 1
		    FROM job_retries r
		    WHERE r.run_id = input.run_id
		      AND r.cleared = FALSE
		      AND NOT EXISTS (
		          SELECT 1
		          FROM job_retries newer
		          WHERE newer.run_id = r.run_id
		            AND newer.id > r.id
		      )
		)`, ids); err != nil {
		return fmt.Errorf("clear retries: %w", err)
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
		SELECT r.run_id
		FROM job_retries r
		WHERE r.next_retry_at <= NOW()
		  AND r.cleared = FALSE
		  AND NOT EXISTS (
		      SELECT 1
		      FROM job_retries newer
		      WHERE newer.run_id = r.run_id
		        AND newer.id > r.id
		  )
		ORDER BY r.next_retry_at ASC, r.run_id ASC
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

// CountPendingRetries returns the number of latest retry records that still gate
// a run. Historical rows and clear tombstones are excluded.
func (q *Queries) CountPendingRetries(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountPendingRetries")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_retries r
		WHERE r.cleared = FALSE
		  AND r.next_retry_at IS NOT NULL
		  AND NOT EXISTS (
		      SELECT 1
		      FROM job_retries newer
		      WHERE newer.run_id = r.run_id
		        AND newer.id > r.id
		  )`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count retries: %w", err)
	}
	return count, nil
}

// CompactSupersededRetries physically removes retry history rows that are no
// longer the latest row for their run. Retry writes stay append-only; this
// bounded cold-path cleanup keeps the side table from growing without bound.
func (q *Queries) CompactSupersededRetries(ctx context.Context, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CompactSupersededRetries")
	defer span.End()

	if limit <= 0 {
		limit = 10000
	}
	const sql = `
		WITH victims AS (
			SELECT r.id
			FROM job_retries r
			WHERE EXISTS (
				SELECT 1
				FROM job_retries newer
				WHERE newer.run_id = r.run_id
				  AND newer.id > r.id
			)
			ORDER BY r.id ASC
			LIMIT $1
		)
		DELETE FROM job_retries r
		USING victims
		WHERE r.id = victims.id`
	tag, err := q.db.Exec(ctx, sql, limit)
	if err != nil {
		return 0, fmt.Errorf("compact superseded retries: %w", err)
	}
	return tag.RowsAffected(), nil
}
