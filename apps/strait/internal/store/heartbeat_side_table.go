package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// Side-table heartbeat API. These methods operate on the unlogged,
// append-only job_run_heartbeats table so the hot heartbeat path does not
// churn job_runs or create update dead tuples in the heartbeat table.

// UpsertHeartbeatSideTable appends a single heartbeat into the side table.
func (q *Queries) UpsertHeartbeatSideTable(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertHeartbeatSideTable")
	defer span.End()

	const sql = `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, NOW(), FALSE)`
	if _, err := q.db.Exec(ctx, sql, runID); err != nil {
		return fmt.Errorf("append heartbeat side table: %w", err)
	}
	return nil
}

// BatchUpsertHeartbeatSideTable appends N heartbeats in a single statement.
func (q *Queries) BatchUpsertHeartbeatSideTable(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BatchUpsertHeartbeatSideTable")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	const sql = `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		SELECT DISTINCT unnest($1::text[]), NOW(), FALSE`
	if _, err := q.db.Exec(ctx, sql, ids); err != nil {
		return fmt.Errorf("batch append heartbeat side table: %w", err)
	}
	return nil
}

// DeleteHeartbeatSideTable appends clear tombstones for terminal runs.
// Called on terminal transitions so latest-row reads stop seeing stale
// heartbeats without physically deleting hot-path history.
func (q *Queries) DeleteHeartbeatSideTable(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteHeartbeatSideTable")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		SELECT DISTINCT unnest($1::text[]), NOW(), TRUE`, ids); err != nil {
		return fmt.Errorf("clear heartbeat side table: %w", err)
	}
	return nil
}

// DeleteOrphanedHeartbeats removes side-table rows whose owning run is
// no longer in a heartbeat-capable active status. Used by the heartbeat GC
// to bound the unlogged table size when a terminal transition skipped
// the explicit delete.
func (q *Queries) DeleteOrphanedHeartbeats(ctx context.Context, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteOrphanedHeartbeats")
	defer span.End()

	if limit <= 0 {
		limit = 10000
	}
	const sql = `
		WITH victims AS MATERIALIZED (
			SELECT h.run_id
			FROM job_run_heartbeats h
			LEFT JOIN job_runs r ON r.id = h.run_id
			LEFT JOIN job_run_read_state s ON s.run_id = h.run_id
			WHERE h.cleared = FALSE
			  AND NOT EXISTS (
			      SELECT 1
			      FROM job_run_heartbeats newer
			      WHERE newer.run_id = h.run_id
			        AND newer.id > h.id
			  )
			  AND (
			      r.id IS NULL
			      OR COALESCE(s.status, r.status) NOT IN ('executing', 'waiting')
			  )
			LIMIT $1
		)
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		SELECT run_id, NOW(), TRUE
		FROM victims`
	tag, err := q.db.Exec(ctx, sql, limit)
	if err != nil {
		return 0, fmt.Errorf("clear orphaned heartbeats: %w", err)
	}
	return tag.RowsAffected(), nil
}

// StaleHeartbeatSideTable returns run_ids with a heartbeat older than the
// threshold. Paired with ListStaleRuns as a supplement when the side table
// path is enabled.
func (q *Queries) StaleHeartbeatSideTable(ctx context.Context, threshold time.Duration, limit int) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.StaleHeartbeatSideTable")
	defer span.End()

	if limit <= 0 {
		limit = 500
	}
	cutoff := time.Now().Add(-threshold)
	rows, err := q.db.Query(ctx, `
		SELECT h.run_id
		FROM job_run_heartbeats h
		WHERE h.cleared = FALSE
		  AND h.heartbeat_at < $1
		  AND NOT EXISTS (
		      SELECT 1
		      FROM job_run_heartbeats newer
		      WHERE newer.run_id = h.run_id
		        AND newer.id > h.id
		  )
		ORDER BY h.heartbeat_at ASC
		LIMIT $2
	`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("stale heartbeat side table: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stale heartbeat: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
