package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertEscalationState(ctx context.Context, state *domain.EscalationState) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertEscalationState")
	defer span.End()

	if state.ID == "" {
		state.ID = uuid.Must(uuid.NewV7()).String()
	}
	if state.Status == "" {
		state.Status = domain.NotifyEscalationStatusActive
	}

	query := `
		INSERT INTO escalation_states (
			id, project_id, step_run_id, workflow_run_id,
			current_tier, total_tiers, acknowledged, acknowledged_by,
			acknowledged_at, next_escalation_at, status
		)
		VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11
		)
		ON CONFLICT (project_id, step_run_id)
			WHERE status = 'active'
		DO UPDATE SET
			workflow_run_id = EXCLUDED.workflow_run_id,
			total_tiers = EXCLUDED.total_tiers,
			next_escalation_at = COALESCE(EXCLUDED.next_escalation_at, escalation_states.next_escalation_at),
			updated_at = NOW()
		RETURNING id, project_id, step_run_id, workflow_run_id,
		          current_tier, total_tiers, acknowledged, acknowledged_by,
		          acknowledged_at, next_escalation_at, status, created_at, updated_at`

	stored, err := scanEscalationState(q.db.QueryRow(ctx, query,
		state.ID,
		state.ProjectID,
		state.StepRunID,
		state.WorkflowRunID,
		state.CurrentTier,
		state.TotalTiers,
		state.Acknowledged,
		dbscan.NilIfEmptyString(state.AcknowledgedBy),
		state.AcknowledgedAt,
		state.NextEscalationAt,
		state.Status,
	))
	if err != nil {
		return fmt.Errorf("upsert escalation state: %w", err)
	}

	*state = *stored
	return nil
}

func (q *Queries) GetActiveEscalationStateByStepRun(ctx context.Context, projectID, stepRunID string) (*domain.EscalationState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetActiveEscalationStateByStepRun")
	defer span.End()

	query := `
		SELECT id, project_id, step_run_id, workflow_run_id,
		       current_tier, total_tiers, acknowledged, acknowledged_by,
		       acknowledged_at, next_escalation_at, status, created_at, updated_at
		FROM escalation_states
		WHERE project_id = $1 AND step_run_id = $2 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1`

	state, err := scanEscalationState(q.db.QueryRow(ctx, query, projectID, stepRunID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEscalationStateNotFound
		}
		return nil, fmt.Errorf("get active escalation state by step run: %w", err)
	}

	return state, nil
}

func (q *Queries) ClaimDueEscalationStates(ctx context.Context, limit int) ([]domain.EscalationState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimDueEscalationStates")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		WITH due AS (
			SELECT id
			FROM escalation_states
			WHERE status = 'active'
			  AND acknowledged = FALSE
			  AND next_escalation_at IS NOT NULL
			  AND next_escalation_at <= NOW()
			ORDER BY next_escalation_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE escalation_states es
		SET status = $2,
		    updated_at = NOW()
		FROM due
		WHERE es.id = due.id
		RETURNING es.id, es.project_id, es.step_run_id, es.workflow_run_id,
		          es.current_tier, es.total_tiers, es.acknowledged, es.acknowledged_by,
		          es.acknowledged_at, es.next_escalation_at, es.status, es.created_at, es.updated_at`

	rows, err := q.db.Query(ctx, query, limit, domain.NotifyEscalationStatusProcessing)
	if err != nil {
		return nil, fmt.Errorf("claim due escalation states: %w", err)
	}
	defer rows.Close()

	states := make([]domain.EscalationState, 0, limit)
	for rows.Next() {
		state, scanErr := scanEscalationState(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("claim due escalation states scan: %w", scanErr)
		}
		states = append(states, *state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim due escalation states rows: %w", err)
	}

	return states, nil
}

func (q *Queries) AdvanceEscalationState(ctx context.Context, id, projectID string, currentTier int, nextEscalationAt *time.Time, status string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AdvanceEscalationState")
	defer span.End()

	if status == "" {
		status = domain.NotifyEscalationStatusActive
	}

	tag, err := q.db.Exec(ctx,
		`UPDATE escalation_states
		 SET current_tier = $3,
		     next_escalation_at = $4,
		     status = $5,
		     updated_at = NOW()
		 WHERE id = $1 AND project_id = $2`,
		id, projectID, currentTier, nextEscalationAt, status,
	)
	if err != nil {
		return fmt.Errorf("advance escalation state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEscalationStateNotFound
	}

	return nil
}

func (q *Queries) AcknowledgeEscalationState(ctx context.Context, id, projectID, acknowledgedBy string, acknowledgedAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AcknowledgeEscalationState")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`UPDATE escalation_states
		 SET acknowledged = TRUE,
		     acknowledged_by = $3,
		     acknowledged_at = $4,
		     status = $5,
		     updated_at = NOW()
		 WHERE id = $1 AND project_id = $2`,
		id, projectID, dbscan.NilIfEmptyString(acknowledgedBy), acknowledgedAt, domain.NotifyEscalationStatusAcknowledged,
	)
	if err != nil {
		return fmt.Errorf("acknowledge escalation state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEscalationStateNotFound
	}

	return nil
}

func scanEscalationState(scanner scanTarget) (*domain.EscalationState, error) {
	var state domain.EscalationState
	var acknowledgedBy *string

	err := scanner.Scan(
		&state.ID,
		&state.ProjectID,
		&state.StepRunID,
		&state.WorkflowRunID,
		&state.CurrentTier,
		&state.TotalTiers,
		&state.Acknowledged,
		&acknowledgedBy,
		&state.AcknowledgedAt,
		&state.NextEscalationAt,
		&state.Status,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if acknowledgedBy != nil {
		state.AcknowledgedBy = *acknowledgedBy
	}

	return &state, nil
}
