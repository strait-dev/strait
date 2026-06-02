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
// archiver. Visibility changes are append-only so the fat run ledger is
// not dirtied by retention sweeps.
func (q *Queries) MaskOldDLQRows(ctx context.Context, retention time.Duration, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MaskOldDLQRows")
	defer span.End()

	if limit <= 0 {
		limit = 1000
	}
	cutoff := time.Now().UTC().Add(-retention)
	const sql = `
		WITH victims AS (
			SELECT jr.id, jr.project_id, jr.job_id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			LEFT JOIN LATERAL (
				SELECT e.visible_until, TRUE AS has_event
				FROM job_run_visibility_events e
				WHERE e.run_id = jr.id
				ORDER BY e.id DESC
				LIMIT 1
			) visibility ON TRUE
			WHERE COALESCE(s.status, jr.status) = 'dead_letter'
			  AND CASE WHEN COALESCE(visibility.has_event, FALSE)
			           THEN visibility.visible_until IS NULL
			           ELSE jr.visible_until IS NULL
			      END
			  AND COALESCE(s.finished_at, jr.finished_at) IS NOT NULL
			  AND COALESCE(s.finished_at, jr.finished_at) < $1
			ORDER BY COALESCE(s.finished_at, jr.finished_at) ASC
			LIMIT $2
		),
		masked AS (
			INSERT INTO job_run_visibility_events (run_id, visible_until)
			SELECT id, NOW()
			FROM victims
			RETURNING run_id
		),
		masked_counts AS (
			SELECT project_id, job_id, COUNT(*)::int AS count
			FROM victims
			GROUP BY project_id, job_id
		),
		decremented AS (
			UPDATE dlq_counts c
			SET count = GREATEST(c.count - mc.count, 0),
			    updated_at = NOW()
			FROM masked_counts mc
			WHERE c.project_id = mc.project_id
			  AND c.job_id = mc.job_id
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
		LEFT JOIN LATERAL (
			SELECT e.visible_until, TRUE AS has_event
			FROM job_run_visibility_events e
			WHERE e.run_id = jr.id
			ORDER BY e.id DESC
			LIMIT 1
		) visibility ON TRUE
		WHERE COALESCE(s.status, jr.status) = 'dead_letter'
		  AND CASE WHEN COALESCE(visibility.has_event, FALSE)
		           THEN visibility.visible_until IS NULL
		           ELSE jr.visible_until IS NULL
		      END
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
			SELECT jr.id, jr.project_id, jr.job_id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			LEFT JOIN LATERAL (
				SELECT e.visible_until, TRUE AS has_event
				FROM job_run_visibility_events e
				WHERE e.run_id = jr.id
				ORDER BY e.id DESC
				LIMIT 1
			) visibility ON TRUE
			WHERE jr.project_id = $1
			  AND jr.job_id = $2
			  AND COALESCE(s.status, jr.status) = 'dead_letter'
			  AND CASE WHEN COALESCE(visibility.has_event, FALSE)
			           THEN visibility.visible_until IS NULL
			           ELSE jr.visible_until IS NULL
			      END
			ORDER BY COALESCE(s.finished_at, jr.finished_at) ASC
			LIMIT 1
		),
		masked AS (
			INSERT INTO job_run_visibility_events (run_id, visible_until)
			SELECT id, NOW()
			FROM victim
			RETURNING run_id
		),
		decremented AS (
			UPDATE dlq_counts c
			SET count = GREATEST(c.count - 1, 0),
			    updated_at = NOW()
			FROM victim v
			WHERE c.project_id = v.project_id
			  AND c.job_id = v.job_id
			RETURNING 1
		)
		SELECT run_id FROM masked`
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
		SELECT jr.project_id, jr.job_id, 1
		FROM job_runs jr
		LEFT JOIN LATERAL (
			SELECT e.visible_until, TRUE AS has_event
			FROM job_run_visibility_events e
			WHERE e.run_id = jr.id
			ORDER BY e.id DESC
			LIMIT 1
		) visibility ON TRUE
		WHERE jr.id = $1
		  AND CASE WHEN COALESCE(visibility.has_event, FALSE)
		           THEN (visibility.visible_until IS NULL OR visibility.visible_until > NOW())
		           ELSE (jr.visible_until IS NULL OR jr.visible_until > NOW())
		      END
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
		LEFT JOIN LATERAL (
			SELECT e.visible_until, TRUE AS has_event
			FROM job_run_visibility_events e
			WHERE e.run_id = jr.id
			ORDER BY e.id DESC
			LIMIT 1
		) visibility ON TRUE
		WHERE jr.id = $1
		  AND CASE WHEN COALESCE(visibility.has_event, FALSE)
		           THEN (visibility.visible_until IS NULL OR visibility.visible_until > NOW())
		           ELSE (jr.visible_until IS NULL OR jr.visible_until > NOW())
		      END
		  AND c.project_id = jr.project_id
		  AND c.job_id = jr.job_id`,
		runID,
	)
	if err != nil {
		return fmt.Errorf("decrement dlq count: %w", err)
	}
	return nil
}
