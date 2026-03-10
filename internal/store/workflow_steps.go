package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowStep")
	defer span.End()

	if step.ID == "" {
		step.ID = uuid.Must(uuid.NewV7()).String()
	}
	if step.OnFailure == "" {
		step.OnFailure = domain.FailWorkflow
	}
	if step.StepType == "" {
		step.StepType = domain.WorkflowStepTypeJob
	}
	if step.RetryBackoff == "" {
		step.RetryBackoff = domain.RetryBackoffExponential
	}
	if step.DependsOn == nil {
		step.DependsOn = []string{}
	}
	if step.ApprovalApprovers == nil {
		step.ApprovalApprovers = []string{}
	}

	query := `
		INSERT INTO workflow_steps (
			id, workflow_id, job_id, step_ref, depends_on, condition, on_failure, payload,
			step_type, approval_timeout_secs, approval_approvers,
			retry_max_attempts, retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
			timeout_secs_override, output_transform,
			sub_workflow_id, max_nesting_depth,
			event_key, event_timeout_secs, event_notify_url, sleep_duration_secs, event_emit_key,
			concurrency_key
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		step.ID,
		step.WorkflowID,
		dbscan.NilIfEmptyString(step.JobID),
		step.StepRef,
		step.DependsOn,
		dbscan.NilIfEmptyRawMessage(step.Condition),
		string(step.OnFailure),
		dbscan.NilIfEmptyRawMessage(step.Payload),
		string(step.StepType),
		step.ApprovalTimeoutSecs,
		step.ApprovalApprovers,
		step.RetryMaxAttempts,
		string(step.RetryBackoff),
		step.RetryInitialDelaySecs,
		step.RetryMaxDelaySecs,
		step.TimeoutSecsOverride,
		step.OutputTransform,
		dbscan.NilIfEmptyString(step.SubWorkflowID),
		step.MaxNestingDepth,
		dbscan.NilIfEmptyString(step.EventKey),
		step.EventTimeoutSecs,
		dbscan.NilIfEmptyString(step.EventNotifyURL),
		step.SleepDurationSecs,
		dbscan.NilIfEmptyString(step.EventEmitKey),
		dbscan.NilIfEmptyString(step.ConcurrencyKey),
	).Scan(&step.CreatedAt)
	if err != nil {
		return fmt.Errorf("create workflow step: %w", err)
	}

	return nil
}

func (q *Queries) ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStepsByWorkflow")
	defer span.End()

	query := `
		SELECT id, workflow_id, job_id, step_ref, depends_on, condition, on_failure, payload,
		       step_type, approval_timeout_secs, approval_approvers,
		       retry_max_attempts, retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
		       timeout_secs_override, output_transform,
		       sub_workflow_id, max_nesting_depth,
		       event_key, event_timeout_secs, event_notify_url, sleep_duration_secs, event_emit_key,
		       concurrency_key,
		       created_at
		FROM workflow_steps
		WHERE workflow_id = $1
		ORDER BY created_at ASC
		LIMIT 500`

	rows, err := q.db.Query(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps: %w", err)
	}
	defer rows.Close()

	steps := make([]domain.WorkflowStep, 0, 16)
	for rows.Next() {
		step, scanErr := scanWorkflowStep(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflow steps scan: %w", scanErr)
		}
		steps = append(steps, *step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow steps rows: %w", err)
	}

	return steps, nil
}

func (q *Queries) GetWorkflowStep(ctx context.Context, id string) (*domain.WorkflowStep, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowStep")
	defer span.End()

	query := `
		SELECT id, workflow_id, job_id, step_ref, depends_on, condition, on_failure, payload,
		       step_type, approval_timeout_secs, approval_approvers,
		       retry_max_attempts, retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
		       timeout_secs_override, output_transform,
		       sub_workflow_id, max_nesting_depth,
		       event_key, event_timeout_secs, event_notify_url, sleep_duration_secs, event_emit_key,
		       concurrency_key,
		       created_at
		FROM workflow_steps
		WHERE id = $1`

	step, err := scanWorkflowStep(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowStepNotFound
		}
		return nil, fmt.Errorf("get workflow step: %w", err)
	}

	return step, nil
}

func (q *Queries) DeleteStepsByWorkflow(ctx context.Context, workflowID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteStepsByWorkflow")
	defer span.End()

	query := `DELETE FROM workflow_steps WHERE workflow_id = $1`

	if _, err := q.db.Exec(ctx, query, workflowID); err != nil {
		return fmt.Errorf("delete workflow steps by workflow: %w", err)
	}

	return nil
}

func scanWorkflowStep(scanner scanTarget) (*domain.WorkflowStep, error) {
	var step domain.WorkflowStep
	var jobID *string
	var dependsOn []string
	var condition []byte
	var onFailure string
	var payload []byte
	var stepType string
	var approvalApprovers []string
	var retryBackoff string
	var subWorkflowID *string
	var eventKey *string
	var eventNotifyURL *string
	var eventEmitKey *string
	var concurrencyKey *string

	err := scanner.Scan(
		&step.ID,
		&step.WorkflowID,
		&jobID,
		&step.StepRef,
		&dependsOn,
		&condition,
		&onFailure,
		&payload,
		&stepType,
		&step.ApprovalTimeoutSecs,
		&approvalApprovers,
		&step.RetryMaxAttempts,
		&retryBackoff,
		&step.RetryInitialDelaySecs,
		&step.RetryMaxDelaySecs,
		&step.TimeoutSecsOverride,
		&step.OutputTransform,
		&subWorkflowID,
		&step.MaxNestingDepth,
		&eventKey,
		&step.EventTimeoutSecs,
		&eventNotifyURL,
		&step.SleepDurationSecs,
		&eventEmitKey,
		&concurrencyKey,
		&step.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	step.DependsOn = dependsOn
	if jobID != nil {
		step.JobID = *jobID
	}
	step.OnFailure = domain.FailurePolicy(onFailure)
	step.StepType = domain.WorkflowStepType(stepType)
	step.ApprovalApprovers = approvalApprovers
	step.RetryBackoff = domain.RetryBackoffPolicy(retryBackoff)
	if condition != nil {
		step.Condition = json.RawMessage(condition)
	}
	if payload != nil {
		step.Payload = json.RawMessage(payload)
	}
	if subWorkflowID != nil {
		step.SubWorkflowID = *subWorkflowID
	}
	if eventKey != nil {
		step.EventKey = *eventKey
	}
	if eventNotifyURL != nil {
		step.EventNotifyURL = *eventNotifyURL
	}
	if eventEmitKey != nil {
		step.EventEmitKey = *eventEmitKey
	}
	if concurrencyKey != nil {
		step.ConcurrencyKey = *concurrencyKey
	}

	return &step, nil
}
