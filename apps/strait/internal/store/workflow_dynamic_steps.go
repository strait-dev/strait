package store

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowDynamicExpansion(
	ctx context.Context,
	workflowRunID, parentStepRunID string,
	expansions []DynamicWorkflowExpansion,
) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowDynamicExpansion")
	defer span.End()

	if len(expansions) == 0 {
		return nil
	}

	txb, ok := q.db.(TxBeginner)
	if !ok {
		return q.createWorkflowDynamicExpansion(ctx, workflowRunID, parentStepRunID, expansions)
	}

	return WithTx(ctx, txb, func(txQ *Queries) error {
		return txQ.createWorkflowDynamicExpansion(ctx, workflowRunID, parentStepRunID, expansions)
	})
}

func (q *Queries) createWorkflowDynamicExpansion(
	ctx context.Context,
	workflowRunID, parentStepRunID string,
	expansions []DynamicWorkflowExpansion,
) error {
	dynamicDepth, err := q.getDynamicStepDepth(ctx, workflowRunID, parentStepRunID)
	if err != nil {
		return fmt.Errorf("get dynamic step depth: %w", err)
	}

	const insertDynamicStepQuery = `
		INSERT INTO workflow_dynamic_steps (
			workflow_run_id, parent_step_run_id, step_ref, depends_on, definition, dynamic_depth
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`

	for i := range expansions {
		expansion := expansions[i]
		definition, marshalErr := json.Marshal(expansion.Step)
		if marshalErr != nil {
			return fmt.Errorf("marshal dynamic step %s: %w", expansion.Step.StepRef, marshalErr)
		}

		if _, execErr := q.db.Exec(
			ctx,
			insertDynamicStepQuery,
			workflowRunID,
			parentStepRunID,
			expansion.Step.StepRef,
			expansion.Step.DependsOn,
			definition,
			dynamicDepth,
		); execErr != nil {
			return fmt.Errorf("insert dynamic step %s: %w", expansion.Step.StepRef, execErr)
		}

		stepRun := expansion.StepRun
		stepRun.WorkflowRunID = workflowRunID
		stepRun.StepRef = expansion.Step.StepRef
		if err := q.CreateWorkflowStepRun(ctx, &stepRun); err != nil {
			return fmt.Errorf("create dynamic step run %s: %w", expansion.Step.StepRef, err)
		}
	}

	return nil
}

func (q *Queries) getDynamicStepDepth(ctx context.Context, workflowRunID, parentStepRunID string) (int, error) {
	const query = `
		SELECT COALESCE(wds.dynamic_depth, 0)
		FROM workflow_step_runs wsr
		LEFT JOIN workflow_dynamic_steps wds
		  ON wds.workflow_run_id = wsr.workflow_run_id
		 AND wds.step_ref = wsr.step_ref
		WHERE wsr.workflow_run_id = $1
		  AND wsr.id = $2`

	var parentDepth int
	if err := q.db.QueryRow(ctx, query, workflowRunID, parentStepRunID).Scan(&parentDepth); err != nil {
		return 0, fmt.Errorf("scan parent dynamic depth: %w", err)
	}
	return parentDepth + 1, nil
}

func (q *Queries) ListDynamicWorkflowStepsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStep, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDynamicWorkflowStepsByWorkflowRun")
	defer span.End()

	const query = `
		SELECT definition
		FROM workflow_dynamic_steps
		WHERE workflow_run_id = $1
		ORDER BY created_at ASC, step_ref ASC`

	rows, err := q.db.Query(ctx, query, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("list dynamic workflow steps: %w", err)
	}
	defer rows.Close()

	steps := make([]domain.WorkflowStep, 0, 8)
	for rows.Next() {
		var definition []byte
		if err := rows.Scan(&definition); err != nil {
			return nil, fmt.Errorf("scan dynamic workflow step: %w", err)
		}
		var step domain.WorkflowStep
		if err := json.Unmarshal(definition, &step); err != nil {
			return nil, fmt.Errorf("unmarshal dynamic workflow step: %w", err)
		}
		steps = append(steps, step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dynamic workflow steps: %w", err)
	}

	return steps, nil
}

var _ WorkflowStepRunStore = (*Queries)(nil)
