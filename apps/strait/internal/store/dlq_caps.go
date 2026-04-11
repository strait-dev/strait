package store

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
)

// Phase 9: DLQ cap helpers. The trigger in migration 000189 keeps a
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
		// pgx returns ErrNoRows for missing rows — treat as zero.
		return 0, nil //nolint:nilerr // missing row means depth=0
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

// MaskOldestDLQRow soft-deletes the single oldest dead_letter row for
// (projectID, jobID). Used by the drop_oldest overflow policy to make room
// for a new failure without letting the DLQ grow unbounded.
func (q *Queries) MaskOldestDLQRow(ctx context.Context, projectID, jobID string) (string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MaskOldestDLQRow")
	defer span.End()

	const sql = `
		WITH victim AS (
			SELECT id FROM job_runs
			WHERE project_id = $1
			  AND job_id = $2
			  AND status = 'dead_letter'
			  AND visible_until IS NULL
			ORDER BY finished_at ASC
			LIMIT 1
		)
		UPDATE job_runs SET visible_until = NOW()
		WHERE id IN (SELECT id FROM victim)
		RETURNING id`
	var id string
	err := q.db.QueryRow(ctx, sql, projectID, jobID).Scan(&id)
	if err != nil {
		return "", nil //nolint:nilerr // no victim means no-op
	}
	return id, nil
}
