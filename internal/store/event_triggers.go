package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
		       notify_url, notify_status, trigger_type, sent_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`

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
		defaultIfEmpty(trigger.NotifyStatus, ""),
		defaultIfEmpty(trigger.TriggerType, "event"),
		defaultIfEmpty(trigger.SentBy, ""),
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
		       notify_url, notify_status, trigger_type, sent_by
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
		       notify_url, notify_status, trigger_type, sent_by
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
		       notify_url, notify_status, trigger_type, sent_by
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

// SetEventTriggerSentBy records who resolved an event trigger (audit trail).
func (q *Queries) SetEventTriggerSentBy(ctx context.Context, id, sentBy string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetEventTriggerSentBy")
	defer span.End()

	_, err := q.db.Exec(ctx, `UPDATE event_triggers SET sent_by = $1 WHERE id = $2`, sentBy, id)
	if err != nil {
		return fmt.Errorf("set event trigger sent_by: %w", err)
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
		       notify_url, notify_status, trigger_type, sent_by
		FROM event_triggers
		WHERE status = 'waiting' AND expires_at <= NOW()
		ORDER BY expires_at ASC
		LIMIT 1000`

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

// ListEventTriggersByProject returns event triggers for a project, optionally filtered by status,
// workflow run ID, and/or source type.
func (q *Queries) ListEventTriggersByProject(ctx context.Context, projectID, status, workflowRunID, sourceType string, limit int, cursor *time.Time) ([]domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventTriggersByProject")
	defer span.End()

	query := `
		SELECT id, event_key, project_id, source_type,
		       workflow_run_id, workflow_step_run_id, job_run_id,
		       status, request_payload, response_payload,
		       timeout_secs, requested_at, received_at, expires_at, error,
		       notify_url, notify_status, trigger_type, sent_by
		FROM event_triggers
		WHERE project_id = $1`

	args := []any{projectID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	if workflowRunID != "" {
		query += fmt.Sprintf(" AND workflow_run_id = $%d", argIdx)
		args = append(args, workflowRunID)
		argIdx++
	}

	if sourceType != "" {
		query += fmt.Sprintf(" AND source_type = $%d", argIdx)
		args = append(args, sourceType)
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

// escapeLikePattern escapes LIKE wildcards (%, _, \) in a user-supplied string
// to prevent SQL LIKE pattern injection.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// ListEventTriggersByKeyPrefix returns all waiting triggers whose event_key starts with the given prefix.
// Uses the text_pattern_ops index for efficient prefix matching.
// When projectID is non-empty, results are scoped to that project.
func (q *Queries) ListEventTriggersByKeyPrefix(ctx context.Context, prefix string, projectID string) ([]domain.EventTrigger, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEventTriggersByKeyPrefix")
	defer span.End()

	escapedPrefix := escapeLikePattern(prefix) + "%"

	var query string
	var args []any

	if projectID != "" {
		query = `
			SELECT id, event_key, project_id, source_type,
			       workflow_run_id, workflow_step_run_id, job_run_id,
			       status, request_payload, response_payload,
			       timeout_secs, requested_at, received_at, expires_at, error,
			       notify_url, notify_status, trigger_type, sent_by
			FROM event_triggers
			WHERE event_key LIKE $1 ESCAPE '\'
			  AND status = 'waiting'
			  AND project_id = $2
			ORDER BY requested_at ASC
			LIMIT 1000`
		args = []any{escapedPrefix, projectID}
	} else {
		query = `
			SELECT id, event_key, project_id, source_type,
			       workflow_run_id, workflow_step_run_id, job_run_id,
			       status, request_payload, response_payload,
			       timeout_secs, requested_at, received_at, expires_at, error,
			       notify_url, notify_status, trigger_type, sent_by
			FROM event_triggers
			WHERE event_key LIKE $1 ESCAPE '\'
			  AND status = 'waiting'
			ORDER BY requested_at ASC
			LIMIT 1000`
		args = []any{escapedPrefix}
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list event triggers by key prefix: %w", err)
	}
	defer rows.Close()

	triggers := make([]domain.EventTrigger, 0, 1000)
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
		       et.notify_url, et.notify_status, et.trigger_type, et.sent_by
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
		       et.notify_url, et.notify_status, et.trigger_type, et.sent_by
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
func (q *Queries) CountEventTriggersFinishedBefore(ctx context.Context, before time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountEventTriggersFinishedBefore")
	defer span.End()

	query := `
		SELECT COUNT(*) FROM event_triggers
		WHERE status IN ('received', 'timed_out', 'canceled')
		  AND COALESCE(received_at, expires_at) < $1`

	var count int64
	if err := q.db.QueryRow(ctx, query, before).Scan(&count); err != nil {
		return 0, fmt.Errorf("count old event triggers: %w", err)
	}
	return count, nil
}

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
	var sentBy *string

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
		&sentBy,
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
	if sentBy != nil {
		trigger.SentBy = *sentBy
	}

	return &trigger, nil
}

// ReceiveEventAndRequeueRun atomically marks a trigger as received and
// re-queues the associated job run (waiting → queued) with checkpoint data.
// This prevents a crash between the two steps from leaving the system
// inconsistent. Only applicable to job_run source type triggers.
func (q *Queries) ReceiveEventAndRequeueRun(ctx context.Context, triggerID string, payload json.RawMessage, receivedAt time.Time, jobRunID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReceiveEventAndRequeueRun")
	defer span.End()

	txb, ok := q.db.(TxBeginner)
	if !ok {
		// Fallback: not a pool (e.g., already in a tx). Execute sequentially.
		if err := q.UpdateEventTriggerStatus(ctx, triggerID, domain.EventTriggerStatusReceived, payload, &receivedAt, ""); err != nil {
			return fmt.Errorf("update trigger status: %w", err)
		}
		return q.UpdateRunStatus(ctx, jobRunID, domain.StatusWaiting, domain.StatusQueued, map[string]any{
			"checkpoint_data": payload,
		})
	}

	return WithTx(ctx, txb, func(txQ *Queries) error {
		if err := txQ.UpdateEventTriggerStatus(ctx, triggerID, domain.EventTriggerStatusReceived, payload, &receivedAt, ""); err != nil {
			return fmt.Errorf("update trigger status: %w", err)
		}
		return txQ.UpdateRunStatus(ctx, jobRunID, domain.StatusWaiting, domain.StatusQueued, map[string]any{
			"checkpoint_data": payload,
		})
	})
}

// BatchReceiveEventTriggers atomically marks multiple triggers as received
// within a single transaction. Returns the list of trigger IDs that were
// successfully updated. If the underlying DBTX doesn't support transactions,
// falls back to sequential updates.
func (q *Queries) BatchReceiveEventTriggers(ctx context.Context, triggerIDs []string, payload json.RawMessage, receivedAt time.Time, sentBy string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BatchReceiveEventTriggers")
	defer span.End()

	if len(triggerIDs) == 0 {
		return nil, nil
	}

	do := func(txQ *Queries) ([]string, error) {
		var resolved []string
		for _, id := range triggerIDs {
			if err := txQ.UpdateEventTriggerStatus(ctx, id, domain.EventTriggerStatusReceived, payload, &receivedAt, ""); err != nil {
				return resolved, fmt.Errorf("update trigger %s: %w", id, err)
			}
			if sentBy != "" {
				_ = txQ.SetEventTriggerSentBy(ctx, id, sentBy) // non-fatal
			}
			resolved = append(resolved, id)
		}
		return resolved, nil
	}

	txb, ok := q.db.(TxBeginner)
	if !ok {
		return do(q)
	}

	var resolved []string
	err := WithTx(ctx, txb, func(txQ *Queries) error {
		var txErr error
		resolved, txErr = do(txQ)
		return txErr
	})
	return resolved, err
}

func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (q *Queries) CountActiveEventTriggersByProject(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountActiveEventTriggersByProject")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM event_triggers WHERE project_id = $1 AND status = 'waiting'`,
		projectID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active event triggers: %w", err)
	}
	return count, nil
}
