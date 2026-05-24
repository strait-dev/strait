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
		WITH claimed AS (
			SELECT id
			FROM workflow_progression_events
			WHERE processed_at IS NULL
			  AND (locked_at IS NULL OR locked_at < NOW() - INTERVAL '30 seconds')
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		),
		updated AS (
			UPDATE workflow_progression_events wpe
			SET locked_at = NOW(),
			    attempts = attempts + 1,
			    updated_at = NOW()
			FROM claimed
			WHERE wpe.id = claimed.id
			RETURNING wpe.id, wpe.workflow_run_id, wpe.step_run_id, wpe.step_ref, wpe.status, wpe.attempts, wpe.created_at
		)
		SELECT id, workflow_run_id, step_run_id, step_ref, status, attempts, created_at
		FROM updated
		ORDER BY created_at ASC
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
		UPDATE workflow_progression_events
		SET processed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("mark workflow progression event processed: %w", err)
	}
	return nil
}

func (q *Queries) ReleaseWorkflowProgressionEvent(ctx context.Context, id int64) error {
	_, err := q.db.Exec(ctx, `
		UPDATE workflow_progression_events
		SET locked_at = NULL, updated_at = NOW()
		WHERE id = $1 AND processed_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("release workflow progression event: %w", err)
	}
	return nil
}
