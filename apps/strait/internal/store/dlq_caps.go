package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// DLQ cap helpers. The trigger in migration 000202 keeps a
// (project_id, job_id) -> count counter current; this file is the Go API
// for reading and enforcing it.

// DLQDepth returns the count of visible dead_letter rows for (projectID, jobID).
// Reads the counter table (O(1)) rather than SELECT COUNT(*) over job_runs.
func (q *Queries) DLQDepth(ctx context.Context, projectID, jobID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DLQDepth")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx,
		`SELECT COALESCE(count, 0) FROM dlq_counts WHERE project_id = $1 AND job_id = $2`,
		projectID, jobID,
	).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("dlq depth: %w", err)
	}
	return count, nil
}

// DLQDepthByProject aggregates all jobs in a project.
func (q *Queries) DLQDepthByProject(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DLQDepthByProject")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM dlq_counts WHERE project_id = $1`,
		projectID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("dlq depth by project: %w", err)
	}
	return count, nil
}

// MaskOldDLQRows soft-deletes up to `limit` dead_letter rows whose
// finished_at is older than `retention`. Used by the DLQ age-out
// archiver. The dlq_counts trigger decrements
// the counter on mask so DLQ caps free up automatically.
func (q *Queries) MaskOldDLQRows(ctx context.Context, retention time.Duration, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MaskOldDLQRows")
	defer span.End()

	if limit <= 0 {
		limit = 1000
	}
	cutoff := time.Now().UTC().Add(-retention)
	const sql = `
		WITH victims AS (
			SELECT jr.id, jr.project_id, jr.job_id, jr.status AS ledger_status
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE COALESCE(s.status, jr.status) = 'dead_letter'
			  AND jr.visible_until IS NULL
			  AND COALESCE(s.finished_at, jr.finished_at) IS NOT NULL
			  AND COALESCE(s.finished_at, jr.finished_at) < $1
			ORDER BY COALESCE(s.finished_at, jr.finished_at) ASC
			LIMIT $2
		),
		masked AS (
			UPDATE job_runs jr
			SET visible_until = NOW()
			FROM victims v
			WHERE jr.id = v.id
			RETURNING v.id, v.project_id, v.job_id, v.ledger_status
		),
		split_counts AS (
			SELECT project_id, job_id, COUNT(*)::int AS count
			FROM masked
			WHERE ledger_status <> 'dead_letter'
			GROUP BY project_id, job_id
		),
		decremented AS (
			UPDATE dlq_counts c
			SET count = GREATEST(c.count - sc.count, 0),
			    updated_at = NOW()
			FROM split_counts sc
			WHERE c.project_id = sc.project_id
			  AND c.job_id = sc.job_id
			RETURNING 1
		)
		SELECT COUNT(*) FROM masked`
	var masked int64
	err := q.db.QueryRow(ctx, sql, cutoff, limit).Scan(&masked)
	if err != nil {
		return 0, fmt.Errorf("mask old dlq rows: %w", err)
	}
	return masked, nil
}

// OldestUnmaskedDLQAge returns the age in seconds of the oldest visible
// dead_letter row (finished_at). Returns 0 if no visible DLQ rows exist.
// The DLQ age gauge feeds this into Grafana so alerts fire when age-out
// falls behind.
func (q *Queries) OldestUnmaskedDLQAge(ctx context.Context) (float64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.OldestUnmaskedDLQAge")
	defer span.End()
	var age float64
	err := q.db.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(COALESCE(s.finished_at, jr.finished_at)))), 0)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE COALESCE(s.status, jr.status) = 'dead_letter'
		  AND jr.visible_until IS NULL
		  AND COALESCE(s.finished_at, jr.finished_at) IS NOT NULL
	`).Scan(&age)
	if err != nil {
		return 0, fmt.Errorf("oldest unmasked dlq age: %w", err)
	}
	return age, nil
}

// MaskOldestDLQRow soft-deletes the single oldest dead_letter row for
// (projectID, jobID). Used by the drop_oldest overflow policy to make room
// for a new failure without letting the DLQ grow unbounded.
func (q *Queries) MaskOldestDLQRow(ctx context.Context, projectID, jobID string) (string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MaskOldestDLQRow")
	defer span.End()

	const sql = `
		WITH victim AS (
			SELECT jr.id, jr.project_id, jr.job_id, jr.status AS ledger_status
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.project_id = $1
			  AND jr.job_id = $2
			  AND COALESCE(s.status, jr.status) = 'dead_letter'
			  AND jr.visible_until IS NULL
			ORDER BY COALESCE(s.finished_at, jr.finished_at) ASC
			LIMIT 1
		),
		masked AS (
			UPDATE job_runs jr
			SET visible_until = NOW()
			FROM victim v
			WHERE jr.id = v.id
			RETURNING v.id, v.project_id, v.job_id, v.ledger_status
		),
		decremented AS (
			UPDATE dlq_counts c
			SET count = GREATEST(c.count - 1, 0),
			    updated_at = NOW()
			FROM masked m
			WHERE m.ledger_status <> 'dead_letter'
			  AND c.project_id = m.project_id
			  AND c.job_id = m.job_id
			RETURNING 1
		)
		SELECT id FROM masked`
	var id string
	err := q.db.QueryRow(ctx, sql, projectID, jobID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("mask oldest dlq row: %w", err)
	}
	return id, nil
}

func (q *Queries) incrementVisibleDLQCountForRun(ctx context.Context, runID string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO dlq_counts (project_id, job_id, count)
		SELECT project_id, job_id, 1
		FROM job_runs
		WHERE id = $1
		  AND (visible_until IS NULL OR visible_until > NOW())
		ON CONFLICT (project_id, job_id)
		DO UPDATE SET count = dlq_counts.count + 1, updated_at = NOW()`,
		runID,
	)
	if err != nil {
		return fmt.Errorf("increment dlq count: %w", err)
	}
	return nil
}

func (q *Queries) decrementVisibleDLQCountForRun(ctx context.Context, runID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE dlq_counts c
		SET count = GREATEST(c.count - 1, 0),
		    updated_at = NOW()
		FROM job_runs jr
		WHERE jr.id = $1
		  AND (jr.visible_until IS NULL OR jr.visible_until > NOW())
		  AND c.project_id = jr.project_id
		  AND c.job_id = jr.job_id`,
		runID,
	)
	if err != nil {
		return fmt.Errorf("decrement dlq count: %w", err)
	}
	return nil
}
