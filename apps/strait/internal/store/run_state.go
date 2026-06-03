package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertRunState(ctx context.Context, s *domain.RunState) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertRunState")
	defer span.End()

	query := `
		WITH inserted AS (
			INSERT INTO run_state (run_id, state_key, value)
			VALUES ($1, $2, $3::jsonb)
			ON CONFLICT (run_id, state_key) DO NOTHING
			RETURNING updated_at
		),
		updated AS (
			UPDATE run_state
			SET value = $3::jsonb,
			    updated_at = NOW()
			WHERE run_id = $1
			  AND state_key = $2
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND value IS DISTINCT FROM $3::jsonb
			RETURNING updated_at
		),
		selected AS (
			SELECT updated_at FROM inserted
			UNION ALL
			SELECT updated_at FROM updated
			UNION ALL
			SELECT updated_at
			FROM run_state
			WHERE run_id = $1
			  AND state_key = $2
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT updated_at FROM selected LIMIT 1`

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
		WITH active_run AS MATERIALIZED (
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $1
			  AND COALESCE(s.attempt, jr.attempt) = $4
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
		),
		inserted AS (
			INSERT INTO run_state (run_id, state_key, value)
			SELECT id, $2, $3::jsonb
			FROM active_run
			ON CONFLICT (run_id, state_key) DO NOTHING
			RETURNING updated_at
		),
		updated AS (
			UPDATE run_state
			SET value = $3::jsonb,
			    updated_at = NOW()
			WHERE run_id = $1
			  AND state_key = $2
			  AND EXISTS (SELECT 1 FROM active_run)
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND value IS DISTINCT FROM $3::jsonb
			RETURNING updated_at
		),
		selected AS (
			SELECT updated_at FROM inserted
			UNION ALL
			SELECT updated_at FROM updated
			UNION ALL
			SELECT updated_at
			FROM run_state
			WHERE run_id = $1
			  AND state_key = $2
			  AND EXISTS (SELECT 1 FROM active_run)
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT EXISTS(SELECT 1 FROM active_run), (SELECT updated_at FROM selected LIMIT 1)`

	var active bool
	var updatedAt *time.Time
	err := q.db.QueryRow(ctx, query, s.RunID, s.StateKey, s.Value, attempt).Scan(&active, &updatedAt)
	if err != nil {
		return fmt.Errorf("upsert active run state: %w", err)
	}
	if !active {
		return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, s.RunID, attempt)
	}
	if updatedAt != nil {
		s.UpdatedAt = *updatedAt
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

// GetRunStateForActiveRun returns a single state row only if the run is
// active for the supplied attempt; mirrors UpsertRunStateForActiveRun and
// guards SDK reads after a run has reached terminal state or been retried.
func (q *Queries) GetRunStateForActiveRun(ctx context.Context, runID, key string, attempt int) (*domain.RunState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunStateForActiveRun")
	defer span.End()

	query := `
		SELECT rs.run_id, rs.state_key, rs.value, rs.updated_at
		FROM run_state rs
		WHERE rs.run_id = $1
		  AND rs.state_key = $2
		  AND EXISTS (
			SELECT 1
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $1
			  AND COALESCE(s.attempt, jr.attempt) = $3
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
		  )`
	var s domain.RunState
	err := q.db.QueryRow(ctx, query, runID, key, attempt).Scan(&s.RunID, &s.StateKey, &s.Value, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var active bool
			activeErr := q.db.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM job_runs jr
					LEFT JOIN job_run_read_state s ON s.run_id = jr.id
					WHERE jr.id = $1
					  AND COALESCE(s.attempt, jr.attempt) = $2
					  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
				)`, runID, attempt).Scan(&active)
			if activeErr != nil {
				return nil, fmt.Errorf("check run active for attempt: %w", activeErr)
			}
			if !active {
				return nil, fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, runID, attempt)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("get active run state: %w", err)
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

// ListRunStateForActiveRun returns all state rows for the run only when the
// run is active for the supplied attempt. SDK readers should prefer this
// over the unscoped ListRunState so terminal/retried tokens cannot fan-out
// reads against historical state.
func (q *Queries) ListRunStateForActiveRun(ctx context.Context, runID string, attempt int) ([]domain.RunState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunStateForActiveRun")
	defer span.End()

	var active bool
	if err := q.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $1
			  AND COALESCE(s.attempt, jr.attempt) = $2
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
		)`, runID, attempt).Scan(&active); err != nil {
		return nil, fmt.Errorf("check run active for attempt: %w", err)
	}
	if !active {
		return nil, fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, runID, attempt)
	}
	query := `SELECT run_id, state_key, value, updated_at FROM run_state WHERE run_id = $1 ORDER BY state_key ASC LIMIT 10000`
	rows, err := q.db.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("list active run state: %w", err)
	}
	defer rows.Close()

	items := make([]domain.RunState, 0, 16)
	for rows.Next() {
		var s domain.RunState
		if err := rows.Scan(&s.RunID, &s.StateKey, &s.Value, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list active run state scan: %w", err)
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
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $1
			  AND COALESCE(s.attempt, jr.attempt) = $3
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
			FOR UPDATE OF jr
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
