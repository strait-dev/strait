package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

// CreateEventTrigger inserts a new event trigger row.
func (q *Queries) CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateEventTrigger")
	defer span.End()

	query := `
		INSERT INTO event_triggers (
			id, event_key, project_id, source_type,
			workflow_run_id, workflow_step_run_id, job_run_id,
			status, request_payload, response_payload,
			timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`

	if _, err := q.db.Exec(
		ctx,
		query,
		trigger.ID,
		trigger.EventKey,
		trigger.ProjectID,
		trigger.SourceType,
		dbscan.NilIfEmptyString(trigger.WorkflowRunID),
		dbscan.NilIfEmptyString(trigger.WorkflowStepRunID),
		dbscan.NilIfEmptyString(trigger.JobRunID),
		trigger.Status,
		dbscan.NilIfEmptyRawMessage(trigger.RequestPayload),
		dbscan.NilIfEmptyRawMessage(trigger.ResponsePayload),
		trigger.TimeoutSecs,
		trigger.RequestedAt,
		trigger.ReceivedAt,
		trigger.ExpiresAt,
		dbscan.NilIfEmptyString(trigger.Error),
		dbscan.NilIfEmptyString(trigger.NotifyURL),
		dbscan.NilIfEmptyString(trigger.NotifyStatus),
		dbscan.NilIfEmptyString(trigger.TriggerType),
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("event key %q already exists: %w", trigger.EventKey, ErrEventKeyConflict)
		}
		return fmt.Errorf("create event trigger: %w", err)
	}

	return nil
}

// GetEventTriggerByEventKey retrieves an event trigger by its unique event key.
func (q *Queries) GetEventTriggerByEventKey(ctx context.Context, eventKey string) (*domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEventTriggerByEventKey")
	defer span.End()

	query := `
		SELECT id, event_key, project_id, source_type,
		       workflow_run_id, workflow_step_run_id, job_run_id,
		       status, request_payload, response_payload,
		       timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type
		FROM event_triggers
		WHERE event_key = $1`

	trigger, err := scanEventTrigger(q.db.QueryRow(ctx, query, eventKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get event trigger by event key: %w", err)
	}

	return trigger, nil
}

// GetEventTriggerByStepRunID retrieves an event trigger by its workflow step run ID.
func (q *Queries) GetEventTriggerByStepRunID(ctx context.Context, stepRunID string) (*domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEventTriggerByStepRunID")
	defer span.End()

	query := `
		SELECT id, event_key, project_id, source_type,
		       workflow_run_id, workflow_step_run_id, job_run_id,
		       status, request_payload, response_payload,
		       timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type
		FROM event_triggers
		WHERE workflow_step_run_id = $1`

	trigger, err := scanEventTrigger(q.db.QueryRow(ctx, query, stepRunID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get event trigger by step run id: %w", err)
	}

	return trigger, nil
}

// GetEventTriggerByJobRunID retrieves an event trigger by its job run ID.
func (q *Queries) GetEventTriggerByJobRunID(ctx context.Context, jobRunID string) (*domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEventTriggerByJobRunID")
	defer span.End()

	query := `
		SELECT id, event_key, project_id, source_type,
		       workflow_run_id, workflow_step_run_id, job_run_id,
		       status, request_payload, response_payload,
		       timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type
		FROM event_triggers
		WHERE job_run_id = $1`

	trigger, err := scanEventTrigger(q.db.QueryRow(ctx, query, jobRunID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get event trigger by job run id: %w", err)
	}

	return trigger, nil
}

// UpdateEventTriggerStatus updates the status and related fields of an event trigger.
func (q *Queries) UpdateEventTriggerStatus(
	ctx context.Context,
	id string,
	status string,
	responsePayload json.RawMessage,
	receivedAt *time.Time,
	errMsg string,
) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateEventTriggerStatus")
	defer span.End()

	query := `
		UPDATE event_triggers
		SET status = $1,
		    response_payload = $2,
		    received_at = $3,
		    error = $4
		WHERE id = $5`

	tag, err := q.db.Exec(
		ctx,
		query,
		status,
		dbscan.NilIfEmptyRawMessage(responsePayload),
		receivedAt,
		dbscan.NilIfEmptyString(errMsg),
		id,
	)
	if err != nil {
		return fmt.Errorf("update event trigger status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("event trigger not found: %s", id)
	}

	return nil
}

// ListExpiredEventTriggers returns all event triggers in waiting status whose expires_at has passed.
func (q *Queries) ListExpiredEventTriggers(ctx context.Context) ([]domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListExpiredEventTriggers")
	defer span.End()

	query := `
		SELECT id, event_key, project_id, source_type,
		       workflow_run_id, workflow_step_run_id, job_run_id,
		       status, request_payload, response_payload,
		       timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type
		FROM event_triggers
		WHERE status = 'waiting' AND expires_at <= NOW()
		ORDER BY expires_at ASC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list expired event triggers: %w", err)
	}
	defer rows.Close()

	triggers := make([]domain.EventTrigger, 0, 8)
	for rows.Next() {
		trigger, scanErr := scanEventTrigger(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan expired event trigger: %w", scanErr)
		}
		triggers = append(triggers, *trigger)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expired event triggers rows: %w", err)
	}

	return triggers, nil
}

// ListEventTriggersByProject returns event triggers for a project, optionally filtered by status.
func (q *Queries) ListEventTriggersByProject(ctx context.Context, projectID string, status string, limit int, cursor *time.Time) ([]domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventTriggersByProject")
	defer span.End()

	query := `
		SELECT id, event_key, project_id, source_type,
		       workflow_run_id, workflow_step_run_id, job_run_id,
		       status, request_payload, response_payload,
		       timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type
		FROM event_triggers
		WHERE project_id = $1`

	args := []any{projectID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	if cursor != nil {
		query += fmt.Sprintf(" AND requested_at < $%d", argIdx)
		args = append(args, *cursor)
		argIdx++
	}

	query += " ORDER BY requested_at DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list event triggers by project: %w", err)
	}
	defer rows.Close()

	triggers := make([]domain.EventTrigger, 0, limit)
	for rows.Next() {
		trigger, scanErr := scanEventTrigger(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan event trigger: %w", scanErr)
		}
		triggers = append(triggers, *trigger)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event triggers by project rows: %w", err)
	}

	return triggers, nil
}

// ListEventTriggersByKeyPrefix returns all waiting triggers whose event_key starts with the given prefix.
// Uses the text_pattern_ops index for efficient prefix matching.
func (q *Queries) ListEventTriggersByKeyPrefix(ctx context.Context, prefix string) ([]domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventTriggersByKeyPrefix")
	defer span.End()

	query := `
		SELECT id, event_key, project_id, source_type,
		       workflow_run_id, workflow_step_run_id, job_run_id,
		       status, request_payload, response_payload,
		       timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type
		FROM event_triggers
		WHERE event_key LIKE $1 || '%'
		  AND status = 'waiting'
		ORDER BY requested_at ASC`

	rows, err := q.db.Query(ctx, query, prefix)
	if err != nil {
		return nil, fmt.Errorf("list event triggers by key prefix: %w", err)
	}
	defer rows.Close()

	var triggers []domain.EventTrigger
	for rows.Next() {
		trigger, scanErr := scanEventTrigger(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan event trigger by key prefix: %w", scanErr)
		}
		triggers = append(triggers, *trigger)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event triggers by key prefix rows: %w", err)
	}

	return triggers, nil
}

// ListReceivedEventTriggersWithStaleSteps returns triggers that are marked 'received' but whose
// associated step run or job run is still in a non-terminal 'waiting' state. This indicates
// a crash between the trigger update and step/run completion (reconciliation target).
func (q *Queries) ListReceivedEventTriggersWithStaleSteps(ctx context.Context) ([]domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListReceivedEventTriggersWithStaleSteps")
	defer span.End()

	query := `
		SELECT et.id, et.event_key, et.project_id, et.source_type,
		       et.workflow_run_id, et.workflow_step_run_id, et.job_run_id,
		       et.status, et.request_payload, et.response_payload,
		       et.timeout_secs, et.requested_at, et.received_at, et.expires_at, et.error,
		       et.notify_url, et.notify_status, et.trigger_type
		FROM event_triggers et
		JOIN workflow_step_runs wsr ON wsr.id = et.workflow_step_run_id
		WHERE et.status = 'received'
		  AND et.source_type = 'workflow_step'
		  AND wsr.status = 'waiting'
		  AND et.received_at < NOW() - INTERVAL '30 seconds'

		UNION ALL

		SELECT et.id, et.event_key, et.project_id, et.source_type,
		       et.workflow_run_id, et.workflow_step_run_id, et.job_run_id,
		       et.status, et.request_payload, et.response_payload,
		       et.timeout_secs, et.requested_at, et.received_at, et.expires_at, et.error,
		       et.notify_url, et.notify_status, et.trigger_type
		FROM event_triggers et
		JOIN runs r ON r.id = et.job_run_id
		WHERE et.status = 'received'
		  AND et.source_type = 'job_run'
		  AND r.status = 'waiting'
		  AND et.received_at < NOW() - INTERVAL '30 seconds'
	`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list received event triggers with stale steps: %w", err)
	}
	defer rows.Close()

	triggers := make([]domain.EventTrigger, 0, 4)
	for rows.Next() {
		trigger, scanErr := scanEventTrigger(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan stale event trigger: %w", scanErr)
		}
		triggers = append(triggers, *trigger)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list received event triggers with stale steps rows: %w", err)
	}

	return triggers, nil
}

// CancelEventTriggersByWorkflowRun cancels all waiting event triggers for a given workflow run.
func (q *Queries) CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelEventTriggersByWorkflowRun")
	defer span.End()

	query := `
		UPDATE event_triggers
		SET status = 'canceled', error = 'workflow canceled'
		WHERE workflow_run_id = $1
		  AND status = 'waiting'`

	tag, err := q.db.Exec(ctx, query, workflowRunID)
	if err != nil {
		return 0, fmt.Errorf("cancel event triggers for workflow run: %w", err)
	}

	return tag.RowsAffected(), nil
}

// CancelEventTriggerByJobRun cancels any waiting event trigger for a given job run.
func (q *Queries) CancelEventTriggerByJobRun(ctx context.Context, jobRunID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelEventTriggerByJobRun")
	defer span.End()

	query := `
		UPDATE event_triggers
		SET status = 'canceled', error = 'job run canceled'
		WHERE job_run_id = $1
		  AND status = 'waiting'`

	if _, err := q.db.Exec(ctx, query, jobRunID); err != nil {
		return fmt.Errorf("cancel event trigger for job run: %w", err)
	}

	return nil
}

// UpdateEventTriggerNotifyStatus updates only the notify_status field of an event trigger.
func (q *Queries) UpdateEventTriggerNotifyStatus(ctx context.Context, id string, notifyStatus string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateEventTriggerNotifyStatus")
	defer span.End()

	query := `UPDATE event_triggers SET notify_status = $1 WHERE id = $2`
	tag, err := q.db.Exec(ctx, query, notifyStatus, id)
	if err != nil {
		return fmt.Errorf("update event trigger notify status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("event trigger not found: %s", id)
	}
	return nil
}

// DeleteEventTriggersFinishedBefore deletes terminal event triggers older than the given time.
func (q *Queries) DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteEventTriggersFinishedBefore")
	defer span.End()

	query := `
		DELETE FROM event_triggers
		WHERE id IN (
			SELECT id FROM event_triggers
			WHERE status IN ('received', 'timed_out', 'canceled')
			  AND COALESCE(received_at, expires_at) < $1
			LIMIT $2
		)`

	tag, err := q.db.Exec(ctx, query, before, limit)
	if err != nil {
		return 0, fmt.Errorf("delete old event triggers: %w", err)
	}

	return tag.RowsAffected(), nil
}

func scanEventTrigger(scanner scanTarget) (*domain.EventTrigger, error) {
	var trigger domain.EventTrigger
	var workflowRunID *string
	var workflowStepRunID *string
	var jobRunID *string
	var requestPayload []byte
	var responsePayload []byte
	var errText *string
	var notifyURL *string
	var notifyStatus *string
	var triggerType *string

	err := scanner.Scan(
		&trigger.ID,
		&trigger.EventKey,
		&trigger.ProjectID,
		&trigger.SourceType,
		&workflowRunID,
		&workflowStepRunID,
		&jobRunID,
		&trigger.Status,
		&requestPayload,
		&responsePayload,
		&trigger.TimeoutSecs,
		&trigger.RequestedAt,
		&trigger.ReceivedAt,
		&trigger.ExpiresAt,
		&errText,
		&notifyURL,
		&notifyStatus,
		&triggerType,
	)
	if err != nil {
		return nil, err
	}

	if workflowRunID != nil {
		trigger.WorkflowRunID = *workflowRunID
	}
	if workflowStepRunID != nil {
		trigger.WorkflowStepRunID = *workflowStepRunID
	}
	if jobRunID != nil {
		trigger.JobRunID = *jobRunID
	}
	if requestPayload != nil {
		trigger.RequestPayload = json.RawMessage(requestPayload)
	}
	if responsePayload != nil {
		trigger.ResponsePayload = json.RawMessage(responsePayload)
	}
	if errText != nil {
		trigger.Error = *errText
	}
	if notifyURL != nil {
		trigger.NotifyURL = *notifyURL
	}
	if notifyStatus != nil {
		trigger.NotifyStatus = *notifyStatus
	}
	if triggerType != nil {
		trigger.TriggerType = *triggerType
	}

	return &trigger, nil
}
