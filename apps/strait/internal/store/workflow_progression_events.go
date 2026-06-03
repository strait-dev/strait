package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

type WorkflowProgressionEvent struct {
	ID            int64
	WorkflowRunID string
	StepRunID     string
	StepRef       string
	Status        string
	Attempts      int
	CreatedAt     time.Time
}

func (q *Queries) CreateWorkflowProgressionEvent(ctx context.Context, workflowRunID, stepRunID, stepRef, status string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowProgressionEvent")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		INSERT INTO workflow_progression_events (workflow_run_id, step_run_id, step_ref, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (step_run_id, status) DO NOTHING
	`, workflowRunID, stepRunID, stepRef, status)
	if err != nil {
		return fmt.Errorf("create workflow progression event: %w", err)
	}
	return nil
}

func (q *Queries) ClaimWorkflowProgressionEvents(ctx context.Context, limit int) ([]WorkflowProgressionEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimWorkflowProgressionEvents")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}
	rows, err := q.db.Query(ctx, `
		WITH deleted_stale_claims AS (
			DELETE FROM workflow_progression_event_claims claim
			USING workflow_progression_events wpe
			WHERE claim.event_id = wpe.id
			  AND claim.locked_at < NOW() - INTERVAL '30 seconds'
			  AND wpe.processed_at IS NULL
			  AND NOT EXISTS (
			      SELECT 1
			      FROM workflow_progression_event_processed processed
			      WHERE processed.event_id = wpe.id
			  )
		),
		candidates AS (
			SELECT wpe.id
			FROM workflow_progression_events wpe
			LEFT JOIN workflow_progression_event_claims claim ON claim.event_id = wpe.id
			LEFT JOIN workflow_progression_event_processed processed ON processed.event_id = wpe.id
			WHERE wpe.processed_at IS NULL
			  AND processed.event_id IS NULL
			  AND claim.event_id IS NULL
			ORDER BY wpe.created_at ASC
			LIMIT $1
			FOR UPDATE OF wpe SKIP LOCKED
		),
		claimed AS (
			INSERT INTO workflow_progression_event_claims (event_id, locked_at, attempts)
			SELECT id, NOW(), 1
			FROM candidates
			ON CONFLICT (event_id) DO NOTHING
			RETURNING event_id, attempts
		)
		SELECT wpe.id, wpe.workflow_run_id, wpe.step_run_id, wpe.step_ref, wpe.status, claimed.attempts, wpe.created_at
		FROM claimed
		JOIN workflow_progression_events wpe ON wpe.id = claimed.event_id
		ORDER BY wpe.created_at ASC
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim workflow progression events: %w", err)
	}
	defer rows.Close()

	events := make([]WorkflowProgressionEvent, 0, limit)
	for rows.Next() {
		var ev WorkflowProgressionEvent
		if err := rows.Scan(&ev.ID, &ev.WorkflowRunID, &ev.StepRunID, &ev.StepRef, &ev.Status, &ev.Attempts, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workflow progression event: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (q *Queries) MarkWorkflowProgressionEventProcessed(ctx context.Context, id int64) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkWorkflowProgressionEventProcessed")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO workflow_progression_event_processed (event_id, processed_at)
			SELECT id, NOW()
			FROM workflow_progression_events
			WHERE id = $1
			  AND processed_at IS NULL
			ON CONFLICT (event_id) DO NOTHING
			RETURNING event_id
		)
		DELETE FROM workflow_progression_event_claims claim
		USING inserted
		WHERE claim.event_id = inserted.event_id
	`, id)
	if err != nil {
		return fmt.Errorf("mark workflow progression event processed: %w", err)
	}
	return nil
}

func (q *Queries) MarkWorkflowProgressionEventsProcessed(ctx context.Context, ids []int64) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkWorkflowProgressionEventsProcessed")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}
	_, err := q.db.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO workflow_progression_event_processed (event_id, processed_at)
			SELECT id, NOW()
			FROM workflow_progression_events
			WHERE id = ANY($1)
			  AND processed_at IS NULL
			ON CONFLICT (event_id) DO NOTHING
			RETURNING event_id
		)
		DELETE FROM workflow_progression_event_claims claim
		USING inserted
		WHERE claim.event_id = inserted.event_id
	`, ids)
	if err != nil {
		return fmt.Errorf("mark workflow progression events processed: %w", err)
	}
	return nil
}

func (q *Queries) ReleaseWorkflowProgressionEvent(ctx context.Context, id int64) error {
	_, err := q.db.Exec(ctx, `
		DELETE FROM workflow_progression_event_claims claim
		USING workflow_progression_events wpe
		WHERE claim.event_id = wpe.id
		  AND claim.event_id = $1
		  AND wpe.processed_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1
		      FROM workflow_progression_event_processed processed
		      WHERE processed.event_id = claim.event_id
		  )
	`, id)
	if err != nil {
		return fmt.Errorf("release workflow progression event: %w", err)
	}
	return nil
}

func (q *Queries) ReleaseWorkflowProgressionEvents(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := q.db.Exec(ctx, `
		DELETE FROM workflow_progression_event_claims claim
		USING workflow_progression_events wpe
		WHERE claim.event_id = wpe.id
		  AND claim.event_id = ANY($1)
		  AND wpe.processed_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1
		      FROM workflow_progression_event_processed processed
		      WHERE processed.event_id = claim.event_id
		  )
	`, ids)
	if err != nil {
		return fmt.Errorf("release workflow progression events: %w", err)
	}
	return nil
}

func (q *Queries) DeleteProcessedWorkflowProgressionEvents(ctx context.Context, olderThan time.Duration, limit int) (int64, error) {
	if olderThan < 0 {
		olderThan = 0
	}
	if limit <= 0 {
		limit = 1000
	}
	tag, err := q.db.Exec(ctx, `
		WITH doomed AS (
			SELECT wpe.id
			FROM workflow_progression_events wpe
			WHERE (
			      wpe.processed_at IS NOT NULL
			      AND wpe.processed_at <= NOW() - $1::interval
			  )
			   OR EXISTS (
			      SELECT 1
			      FROM workflow_progression_event_processed processed
			      WHERE processed.event_id = wpe.id
			        AND processed.processed_at <= NOW() - $1::interval
			  )
			ORDER BY COALESCE(
			    (
			        SELECT processed.processed_at
			        FROM workflow_progression_event_processed processed
			        WHERE processed.event_id = wpe.id
			    ),
			    wpe.processed_at
			) ASC, wpe.id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM workflow_progression_events wpe
		USING doomed
		WHERE wpe.id = doomed.id
	`, olderThan, limit)
	if err != nil {
		return 0, fmt.Errorf("delete processed workflow progression events: %w", err)
	}
	return tag.RowsAffected(), nil
}
