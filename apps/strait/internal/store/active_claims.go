package store

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
)

// DeleteInactiveActiveClaims physically removes active-claim history that no
// longer contributes to the run read model. Claim, pause, requeue, and terminal
// paths intentionally avoid deleting from job_run_active_claims on the hot path;
// this bounded cold-path cleanup keeps the table size under control.
func (q *Queries) DeleteInactiveActiveClaims(ctx context.Context, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteInactiveActiveClaims")
	defer span.End()

	if limit <= 0 {
		limit = 10000
	}
	const query = `
		WITH victims AS (
			SELECT c.run_id, c.ready_generation
			FROM job_run_active_claims c
			LEFT JOIN job_run_state s ON s.run_id = c.run_id
			LEFT JOIN job_run_terminal_state t ON t.run_id = c.run_id
			WHERE t.run_id IS NOT NULL
			   OR s.run_id IS NULL
			   OR s.ready_generation <> c.ready_generation
			   OR NOT (
			       s.status IN ('queued', 'delayed')
			       OR (
			           s.status = 'paused'
			           AND EXISTS (
			               SELECT 1
			               FROM job_run_ready_events ready
			               WHERE ready.run_id = s.run_id
			                 AND ready.ready_generation = s.ready_generation
			                 AND ready.reason = 'paused_resume'
			           )
			       )
			   )
			ORDER BY c.run_id ASC, c.ready_generation ASC
			LIMIT $1
		)
		DELETE FROM job_run_active_claims c
		USING victims v
		WHERE c.run_id = v.run_id
		  AND c.ready_generation = v.ready_generation`
	tag, err := q.db.Exec(ctx, query, limit)
	if err != nil {
		return 0, fmt.Errorf("delete inactive active claims: %w", err)
	}
	return tag.RowsAffected(), nil
}
