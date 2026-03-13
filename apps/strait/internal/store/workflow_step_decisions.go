package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowStepDecision(ctx context.Context, d *domain.WorkflowStepDecision) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowStepDecision")
	defer span.End()

	if d.ID == "" {
		d.ID = uuid.Must(uuid.NewV7()).String()
	}

	err := q.db.QueryRow(ctx, `
		INSERT INTO workflow_step_decisions (
			id, workflow_run_id, step_run_id, step_ref,
			decision_type, decision, explanation, details
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING created_at`,
		d.ID,
		d.WorkflowRunID,
		d.StepRunID,
		d.StepRef,
		d.DecisionType,
		d.Decision,
		d.Explanation,
		dbscan.NilIfEmptyRawMessage(d.Details),
	).Scan(&d.CreatedAt)
	if err != nil {
		return fmt.Errorf("create workflow step decision: %w", err)
	}
	return nil
}

func (q *Queries) ListWorkflowStepDecisions(ctx context.Context, workflowRunID, stepRef, decisionType string, limit int, cursor *time.Time) ([]domain.WorkflowStepDecision, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowStepDecisions")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, workflow_run_id, step_run_id, step_ref, decision_type, decision, explanation, details, created_at
		FROM workflow_step_decisions
		WHERE workflow_run_id = $1`
	args := []any{workflowRunID}
	idx := 2

	if stepRef != "" {
		query += fmt.Sprintf(" AND step_ref = $%d", idx)
		args = append(args, stepRef)
		idx++
	}
	if decisionType != "" {
		query += fmt.Sprintf(" AND decision_type = $%d", idx)
		args = append(args, decisionType)
		idx++
	}
	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", idx)
		args = append(args, *cursor)
		idx++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflow step decisions: %w", err)
	}
	defer rows.Close()

	out := make([]domain.WorkflowStepDecision, 0, limit)
	for rows.Next() {
		var d domain.WorkflowStepDecision
		var details []byte
		if scanErr := rows.Scan(&d.ID, &d.WorkflowRunID, &d.StepRunID, &d.StepRef, &d.DecisionType, &d.Decision, &d.Explanation, &details, &d.CreatedAt); scanErr != nil {
			return nil, fmt.Errorf("list workflow step decisions scan: %w", scanErr)
		}
		if details != nil {
			d.Details = json.RawMessage(details)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow step decisions rows: %w", err)
	}
	return out, nil
}
