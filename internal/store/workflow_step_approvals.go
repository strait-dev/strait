package store

import (
	"context"
	"fmt"
	"time"

	"orchestrator/internal/domain"

	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateWorkflowStepApproval")
	defer span.End()

	query := `
		INSERT INTO workflow_step_approvals (
			id, workflow_run_id, workflow_step_run_id, approvers, status,
			approved_by, requested_at, approved_at, expires_at, error
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	if _, err := q.db.Exec(
		ctx,
		query,
		approval.ID,
		approval.WorkflowRunID,
		approval.WorkflowStepRunID,
		approval.Approvers,
		approval.Status,
		nilIfEmptyString(approval.ApprovedBy),
		approval.RequestedAt,
		approval.ApprovedAt,
		approval.ExpiresAt,
		nilIfEmptyString(approval.Error),
	); err != nil {
		return fmt.Errorf("create workflow step approval: %w", err)
	}

	return nil
}

func (q *Queries) GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetWorkflowStepApprovalByStepRunID")
	defer span.End()

	query := `
		SELECT id, workflow_run_id, workflow_step_run_id, approvers, status,
		       approved_by, requested_at, approved_at, expires_at, error
		FROM workflow_step_approvals
		WHERE workflow_step_run_id = $1`

	return scanWorkflowStepApproval(q.db.QueryRow(ctx, query, stepRunID))
}

func (q *Queries) UpdateWorkflowStepApproval(
	ctx context.Context,
	id string,
	status string,
	approvedBy string,
	approvedAt *time.Time,
	errMsg string,
) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateWorkflowStepApproval")
	defer span.End()

	query := `
		UPDATE workflow_step_approvals
		SET status = $1,
		    approved_by = $2,
		    approved_at = $3,
		    error = $4
		WHERE id = $5`

	tag, err := q.db.Exec(
		ctx,
		query,
		status,
		nilIfEmptyString(approvedBy),
		approvedAt,
		nilIfEmptyString(errMsg),
		id,
	)
	if err != nil {
		return fmt.Errorf("update workflow step approval: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrWorkflowStepRunNotFound
	}

	return nil
}

func (q *Queries) ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListExpiredWorkflowStepApprovals")
	defer span.End()

	query := `
		SELECT id, workflow_run_id, workflow_step_run_id, approvers, status,
		       approved_by, requested_at, approved_at, expires_at, error
		FROM workflow_step_approvals
		WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at <= NOW()
		ORDER BY expires_at ASC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list expired workflow step approvals: %w", err)
	}
	defer rows.Close()

	approvals := make([]domain.WorkflowStepApproval, 0, 8)
	for rows.Next() {
		approval, scanErr := scanWorkflowStepApproval(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan workflow step approval: %w", scanErr)
		}
		approvals = append(approvals, *approval)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expired workflow step approvals rows: %w", err)
	}

	return approvals, nil
}

func (q *Queries) GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetStepRunByWorkflowRunAndRef")
	defer span.End()

	query := `
		SELECT id, workflow_run_id, workflow_step_id, step_ref, job_run_id, status,
		       deps_completed, deps_required, output, error, started_at, finished_at, created_at
		FROM workflow_step_runs
		WHERE workflow_run_id = $1 AND step_ref = $2`

	return scanWorkflowStepRun(q.db.QueryRow(ctx, query, workflowRunID, stepRef))
}

func scanWorkflowStepApproval(scanner scanTarget) (*domain.WorkflowStepApproval, error) {
	var approval domain.WorkflowStepApproval
	var approvedBy *string
	var errText *string

	err := scanner.Scan(
		&approval.ID,
		&approval.WorkflowRunID,
		&approval.WorkflowStepRunID,
		&approval.Approvers,
		&approval.Status,
		&approvedBy,
		&approval.RequestedAt,
		&approval.ApprovedAt,
		&approval.ExpiresAt,
		&errText,
	)
	if err != nil {
		return nil, err
	}
	if approvedBy != nil {
		approval.ApprovedBy = *approvedBy
	}
	if errText != nil {
		approval.Error = *errText
	}

	return &approval, nil
}

func nilIfEmptyString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
