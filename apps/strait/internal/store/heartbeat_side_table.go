package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// Phase 8: side-table heartbeat API. These methods operate on the unlogged
// job_run_heartbeats table so the hot heartbeat path does not churn
// job_runs and defeat Phase 3's HOT-update wins.

// UpsertHeartbeatSideTable writes a single heartbeat into the side table.
// The PK conflict path keeps latency constant regardless of whether the row
// already exists.
func (q *Queries) UpsertHeartbeatSideTable(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertHeartbeatSideTable")
	defer span.End()

	const sql = `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at)
		VALUES ($1, NOW())
		ON CONFLICT (run_id) DO UPDATE SET heartbeat_at = EXCLUDED.heartbeat_at`
	if _, err := q.db.Exec(ctx, sql, runID); err != nil {
		return fmt.Errorf("upsert heartbeat side table: %w", err)
	}
	return nil
}

// BatchUpsertHeartbeatSideTable writes N heartbeats in a single statement.
func (q *Queries) BatchUpsertHeartbeatSideTable(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BatchUpsertHeartbeatSideTable")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	const sql = `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at)
		SELECT unnest($1::text[]), NOW()
		ON CONFLICT (run_id) DO UPDATE SET heartbeat_at = EXCLUDED.heartbeat_at`
	if _, err := q.db.Exec(ctx, sql, ids); err != nil {
		return fmt.Errorf("batch upsert heartbeat side table: %w", err)
	}
	return nil
}

// DeleteHeartbeatSideTable removes heartbeat entries for terminal runs.
// Called on terminal transitions so the side table does not grow unbounded.
func (q *Queries) DeleteHeartbeatSideTable(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteHeartbeatSideTable")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM job_run_heartbeats WHERE run_id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("delete heartbeat side table: %w", err)
	}
	return nil
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
		SELECT run_id FROM job_run_heartbeats
		WHERE heartbeat_at < $1
		ORDER BY heartbeat_at ASC
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
