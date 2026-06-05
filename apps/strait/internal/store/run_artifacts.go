package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunCheckpoint")
	defer span.End()

	if checkpoint.ID == "" {
		checkpoint.ID = uuid.Must(uuid.NewV7()).String()
	}
	if checkpoint.Source == "" {
		checkpoint.Source = "sdk"
	}

	err := q.WithTxQueries(ctx, func(txQ *Queries) error {
		return txQ.createRunCheckpointLocked(ctx, checkpoint, 0, false)
	})
	if err != nil {
		return fmt.Errorf("create run checkpoint: %w", err)
	}

	return nil
}

func (q *Queries) CreateRunCheckpointForActiveRun(ctx context.Context, checkpoint *domain.RunCheckpoint, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunCheckpointForActiveRun")
	defer span.End()

	if checkpoint.ID == "" {
		checkpoint.ID = uuid.Must(uuid.NewV7()).String()
	}
	if checkpoint.Source == "" {
		checkpoint.Source = "sdk"
	}

	err := q.WithTxQueries(ctx, func(txQ *Queries) error {
		return txQ.createRunCheckpointLocked(ctx, checkpoint, attempt, true)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, checkpoint.RunID, attempt)
		}
		return fmt.Errorf("create active run checkpoint: %w", err)
	}

	return nil
}

func (q *Queries) createRunCheckpointLocked(ctx context.Context, checkpoint *domain.RunCheckpoint, attempt int, requireActive bool) error {
	lockQuery := `
		SELECT id
		FROM job_runs
		WHERE id = $1
		FOR UPDATE`
	args := []any{checkpoint.RunID}
	if requireActive {
		lockQuery = `
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $1
			  AND COALESCE(s.attempt, jr.attempt) = $2
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
			FOR UPDATE OF jr`
		args = append(args, attempt)
	}

	var lockedRunID string
	if err := q.db.QueryRow(ctx, lockQuery, args...).Scan(&lockedRunID); err != nil {
		return err
	}

	query := `
		WITH next_seq AS (
			SELECT COALESCE(MAX(sequence), 0) + 1 AS seq
			FROM run_checkpoints
			WHERE run_id = $1
		)
		INSERT INTO run_checkpoints (id, run_id, sequence, source, state)
		VALUES ($2, $1, (SELECT seq FROM next_seq), $3, $4)
		RETURNING sequence, created_at`

	return q.db.QueryRow(
		ctx,
		query,
		checkpoint.RunID,
		checkpoint.ID,
		checkpoint.Source,
		dbscan.NilIfEmptyRawMessage(checkpoint.State),
	).Scan(&checkpoint.Sequence, &checkpoint.CreatedAt)
}

func (q *Queries) ListRunCheckpoints(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunCheckpoints")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, run_id, sequence, source, state, created_at
		FROM run_checkpoints
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY sequence DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list run checkpoints: %w", err)
	}
	defer rows.Close()

	checkpoints := make([]domain.RunCheckpoint, 0)
	for rows.Next() {
		cp, scanErr := scanRunCheckpoint(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list run checkpoints scan: %w", scanErr)
		}
		checkpoints = append(checkpoints, *cp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run checkpoints rows: %w", err)
	}

	return checkpoints, nil
}

func (q *Queries) GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLatestCheckpoint")
	defer span.End()

	query := `
		SELECT id, run_id, sequence, source, state, created_at
		FROM run_checkpoints
		WHERE run_id = $1
		ORDER BY sequence DESC
		LIMIT 1`

	row := q.db.QueryRow(ctx, query, runID)
	cp, err := scanRunCheckpoint(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest checkpoint: %w", err)
	}

	return cp, nil
}

func (q *Queries) UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertRunOutput")
	defer span.End()

	if output.ID == "" {
		output.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		WITH inserted AS (
			INSERT INTO run_outputs (id, run_id, output_key, schema, value)
			VALUES ($1, $2, $3, $4::jsonb, $5::jsonb)
			ON CONFLICT (run_id, output_key) DO NOTHING
			RETURNING id, created_at
		),
		updated AS (
			UPDATE run_outputs
			SET schema = $4::jsonb,
			    value = $5::jsonb,
			    created_at = NOW()
			WHERE run_id = $2
			  AND output_key = $3
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND (
			      schema IS DISTINCT FROM $4::jsonb
			      OR value IS DISTINCT FROM $5::jsonb
			  )
			RETURNING id, created_at
		),
		selected AS (
			SELECT id, created_at FROM inserted
			UNION ALL
			SELECT id, created_at FROM updated
			UNION ALL
			SELECT id, created_at
			FROM run_outputs
			WHERE run_id = $2
			  AND output_key = $3
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT id, created_at FROM selected LIMIT 1`

	err := q.db.QueryRow(
		ctx,
		query,
		output.ID,
		output.RunID,
		output.OutputKey,
		dbscan.NilIfEmptyRawMessage(output.Schema),
		dbscan.NilIfEmptyRawMessage(output.Value),
	).Scan(&output.ID, &output.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert run output: %w", err)
	}

	return nil
}

func (q *Queries) UpsertRunOutputForActiveRun(ctx context.Context, output *domain.RunOutput, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertRunOutputForActiveRun")
	defer span.End()

	if output.ID == "" {
		output.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		WITH active_run AS MATERIALIZED (
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $2
			  AND COALESCE(s.attempt, jr.attempt) = $6
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
		),
		inserted AS (
			INSERT INTO run_outputs (id, run_id, output_key, schema, value)
			SELECT $1, id, $3, $4::jsonb, $5::jsonb
			FROM active_run
			ON CONFLICT (run_id, output_key) DO NOTHING
			RETURNING id, created_at
		),
		updated AS (
			UPDATE run_outputs
			SET schema = $4::jsonb,
			    value = $5::jsonb,
			    created_at = NOW()
			WHERE run_id = $2
			  AND output_key = $3
			  AND EXISTS (SELECT 1 FROM active_run)
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND (
			      schema IS DISTINCT FROM $4::jsonb
			      OR value IS DISTINCT FROM $5::jsonb
			  )
			RETURNING id, created_at
		),
		selected AS (
			SELECT id, created_at FROM inserted
			UNION ALL
			SELECT id, created_at FROM updated
			UNION ALL
			SELECT id, created_at
			FROM run_outputs
			WHERE run_id = $2
			  AND output_key = $3
			  AND EXISTS (SELECT 1 FROM active_run)
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT EXISTS(SELECT 1 FROM active_run),
		       COALESCE((SELECT id FROM selected LIMIT 1), ''),
		       (SELECT created_at FROM selected LIMIT 1)`

	var active bool
	var outputID string
	var createdAt *time.Time
	err := q.db.QueryRow(ctx, query, output.ID, output.RunID, output.OutputKey, dbscan.NilIfEmptyRawMessage(output.Schema), dbscan.NilIfEmptyRawMessage(output.Value), attempt).Scan(&active, &outputID, &createdAt)
	if err != nil {
		return fmt.Errorf("upsert active run output: %w", err)
	}
	if !active {
		return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, output.RunID, attempt)
	}
	if outputID != "" {
		output.ID = outputID
	}
	if createdAt != nil {
		output.CreatedAt = *createdAt
	}
	return nil
}

func (q *Queries) ListRunOutputs(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunOutputs")
	defer span.End()

	query := `
		SELECT id, run_id, output_key, schema, value, created_at
		FROM run_outputs
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY output_key ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list run outputs: %w", err)
	}
	defer rows.Close()

	outputs := make([]domain.RunOutput, 0)
	for rows.Next() {
		o, scanErr := scanRunOutput(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list run outputs scan: %w", scanErr)
		}
		outputs = append(outputs, *o)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run outputs rows: %w", err)
	}

	return outputs, nil
}

func scanRunCheckpoint(scanner scanTarget) (*domain.RunCheckpoint, error) {
	var cp domain.RunCheckpoint
	var state []byte
	err := scanner.Scan(&cp.ID, &cp.RunID, &cp.Sequence, &cp.Source, &state, &cp.CreatedAt)
	if err != nil {
		return nil, err
	}
	if state != nil {
		cp.State = json.RawMessage(state)
	}
	return &cp, nil
}

func scanRunOutput(scanner scanTarget) (*domain.RunOutput, error) {
	var output domain.RunOutput
	var schema []byte
	var value []byte
	err := scanner.Scan(&output.ID, &output.RunID, &output.OutputKey, &schema, &value, &output.CreatedAt)
	if err != nil {
		return nil, err
	}
	if schema != nil {
		output.Schema = json.RawMessage(schema)
	}
	if value != nil {
		output.Value = json.RawMessage(value)
	}
	return &output, nil
}
