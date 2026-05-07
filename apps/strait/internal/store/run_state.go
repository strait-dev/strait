package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertRunState(ctx context.Context, s *domain.RunState) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertRunState")
	defer span.End()

	query := `
		INSERT INTO run_state (run_id, state_key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (run_id, state_key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		RETURNING updated_at`

	err := q.db.QueryRow(ctx, query, s.RunID, s.StateKey, s.Value).Scan(&s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert run state: %w", err)
	}
	return nil
}

func (q *Queries) UpsertRunStateForActiveRun(ctx context.Context, s *domain.RunState, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertRunStateForActiveRun")
	defer span.End()

	query := `
		WITH active_run AS (
			SELECT id
			FROM job_runs
			WHERE id = $1
			  AND attempt = $4
			  AND status IN ('executing', 'waiting')
		)
		INSERT INTO run_state (run_id, state_key, value)
		SELECT id, $2, $3
		FROM active_run
		ON CONFLICT (run_id, state_key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		RETURNING updated_at`

	err := q.db.QueryRow(ctx, query, s.RunID, s.StateKey, s.Value, attempt).Scan(&s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, s.RunID, attempt)
		}
		return fmt.Errorf("upsert active run state: %w", err)
	}
	return nil
}

func (q *Queries) GetRunState(ctx context.Context, runID, key string) (*domain.RunState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunState")
	defer span.End()

	query := `SELECT run_id, state_key, value, updated_at FROM run_state WHERE run_id = $1 AND state_key = $2`
	var s domain.RunState
	err := q.db.QueryRow(ctx, query, runID, key).Scan(&s.RunID, &s.StateKey, &s.Value, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run state: %w", err)
	}
	return &s, nil
}

func (q *Queries) ListRunState(ctx context.Context, runID string) ([]domain.RunState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunState")
	defer span.End()

	query := `SELECT run_id, state_key, value, updated_at FROM run_state WHERE run_id = $1 ORDER BY state_key ASC LIMIT 10000`
	rows, err := q.db.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("list run state: %w", err)
	}
	defer rows.Close()

	items := make([]domain.RunState, 0, 16)
	for rows.Next() {
		var s domain.RunState
		if err := rows.Scan(&s.RunID, &s.StateKey, &s.Value, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list run state scan: %w", err)
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// CopyRunState copies all state KV pairs from one run to another.
// Existing keys on the target run are not overwritten (ON CONFLICT DO NOTHING).
func (q *Queries) CopyRunState(ctx context.Context, fromRunID, toRunID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CopyRunState")
	defer span.End()

	query := `
		INSERT INTO run_state (run_id, state_key, value, updated_at)
		SELECT $2, state_key, value, NOW()
		FROM run_state
		WHERE run_id = $1
		ON CONFLICT DO NOTHING`

	_, err := q.db.Exec(ctx, query, fromRunID, toRunID)
	if err != nil {
		return fmt.Errorf("copy run state: %w", err)
	}
	return nil
}

func (q *Queries) DeleteRunState(ctx context.Context, runID, key string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteRunState")
	defer span.End()

	_, err := q.db.Exec(ctx, `DELETE FROM run_state WHERE run_id = $1 AND state_key = $2`, runID, key)
	if err != nil {
		return fmt.Errorf("delete run state: %w", err)
	}
	return nil
}

func (q *Queries) DeleteRunStateForActiveRun(ctx context.Context, runID, key string, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteRunStateForActiveRun")
	defer span.End()

	var active bool
	query := `
		WITH active_run AS (
			SELECT id
			FROM job_runs
			WHERE id = $1
			  AND attempt = $3
			  AND status IN ('executing', 'waiting')
			FOR UPDATE
		),
		deleted AS (
			DELETE FROM run_state
			WHERE run_id = $1
			  AND state_key = $2
			  AND EXISTS (SELECT 1 FROM active_run)
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM active_run)`
	if err := q.db.QueryRow(ctx, query, runID, key, attempt).Scan(&active); err != nil {
		return fmt.Errorf("delete active run state: %w", err)
	}
	if !active {
		return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, runID, attempt)
	}
	return nil
}
