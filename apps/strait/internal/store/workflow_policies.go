package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertWorkflowPolicy(ctx context.Context, p *domain.WorkflowPolicy) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertWorkflowPolicy")
	defer span.End()
	if p.ID == "" {
		p.ID = uuid.Must(uuid.NewV7()).String()
	}
	if p.ForbiddenStepTypes == nil {
		p.ForbiddenStepTypes = []string{}
	}
	err := q.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO workflow_policies (id, project_id, max_fan_out, max_depth, forbidden_step_types, require_approval_for_deploy)
			VALUES ($1, $2, $3, $4, $5::text[], $6)
			ON CONFLICT (project_id) DO NOTHING
			RETURNING id, created_at, updated_at
		),
		updated AS (
			UPDATE workflow_policies
			SET max_fan_out = $3,
			    max_depth = $4,
			    forbidden_step_types = $5::text[],
			    require_approval_for_deploy = $6,
			    updated_at = NOW()
			WHERE project_id = $2
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND (
			      max_fan_out IS DISTINCT FROM $3
			      OR max_depth IS DISTINCT FROM $4
			      OR forbidden_step_types IS DISTINCT FROM $5::text[]
			      OR require_approval_for_deploy IS DISTINCT FROM $6
			  )
			RETURNING id, created_at, updated_at
		),
		selected AS (
			SELECT id, created_at, updated_at FROM inserted
			UNION ALL
			SELECT id, created_at, updated_at FROM updated
			UNION ALL
			SELECT id, created_at, updated_at
			FROM workflow_policies
			WHERE project_id = $2
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT id, created_at, updated_at FROM selected LIMIT 1`,
		p.ID, p.ProjectID, p.MaxFanOut, p.MaxDepth, p.ForbiddenStepTypes, p.RequireApprovalForDeploy,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert workflow policy: %w", err)
	}
	return nil
}

func (q *Queries) GetWorkflowPolicyByProject(ctx context.Context, projectID string) (*domain.WorkflowPolicy, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowPolicyByProject")
	defer span.End()
	var p domain.WorkflowPolicy
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, max_fan_out, max_depth, forbidden_step_types, require_approval_for_deploy, created_at, updated_at
		FROM workflow_policies
		WHERE project_id = $1`, projectID).
		Scan(&p.ID, &p.ProjectID, &p.MaxFanOut, &p.MaxDepth, &p.ForbiddenStepTypes, &p.RequireApprovalForDeploy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get workflow policy by project: %w", err)
	}
	return &p, nil
}
