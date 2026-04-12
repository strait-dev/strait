package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateAuditEventDeadletter writes an audit event that failed to persist
// to the primary audit_events table into the deadletter table. Never
// participates in the HMAC chain — these events are recovery material for
// a future reclaimer, not part of the signed log.
//
// This is the escape hatch for the async emit path in internal/api:
// after in-memory retries are exhausted, the event lands here so it
// survives process restart and can be replayed later.
func (q *Queries) CreateAuditEventDeadletter(ctx context.Context, ev *domain.AuditEvent, lastErr string, retryCount int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAuditEventDeadletter")
	defer span.End()

	if ev.ID == "" {
		ev.ID = uuid.Must(uuid.NewV7()).String()
	}
	details := ev.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}

	if ev.SchemaVersion == 0 {
		ev.SchemaVersion = domain.AuditEventSchemaVersionCurrent
	}

	_, err := q.db.Exec(ctx, `
		INSERT INTO audit_events_deadletter (
			id, project_id, actor_id, actor_type, action,
			resource_type, resource_id, details, created_at,
			last_error, retry_count,
			remote_ip, user_agent, request_id, trace_id, schema_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12, $13, $14, $15, $16)
	`,
		ev.ID, ev.ProjectID, ev.ActorID, ev.ActorType, ev.Action,
		ev.ResourceType, ev.ResourceID, details, ev.CreatedAt,
		lastErr, retryCount,
		ev.RemoteIP, ev.UserAgent, ev.RequestID, ev.TraceID, ev.SchemaVersion,
	)
	if err != nil {
		return fmt.Errorf("create audit event deadletter: %w", err)
	}
	return nil
}

// CountAuditEventsDeadletter returns the number of rows currently in the
// deadletter table. Used by the audit health probe to alert when any
// event is waiting for reclamation.
func (q *Queries) CountAuditEventsDeadletter(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountAuditEventsDeadletter")
	defer span.End()

	var n int64
	if err := q.db.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events_deadletter`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count audit deadletter: %w", err)
	}
	return n, nil
}

// ListAuditEventsDeadletter returns the oldest deadletter events for
// reclamation. Results are ordered by queued_at ASC (oldest first).
func (q *Queries) ListAuditEventsDeadletter(ctx context.Context, limit int) ([]domain.AuditEvent, []string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditEventsDeadletter")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
		       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version
		FROM audit_events_deadletter
		ORDER BY queued_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("list audit deadletter: %w", err)
	}
	defer rows.Close()

	var events []domain.AuditEvent
	var dlqIDs []string
	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(
			&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action,
			&ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.CreatedAt,
			&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion,
		); err != nil {
			return nil, nil, fmt.Errorf("scan audit deadletter: %w", err)
		}
		events = append(events, ev)
		dlqIDs = append(dlqIDs, ev.ID)
	}
	return events, dlqIDs, rows.Err()
}

// ListAuditEventsDeadletterByProject returns deadletter events filtered by
// project_id for admin inspection. Tenant isolation is enforced structurally:
// the project_id is a required filter, not an optional one.
//
// Ordered by queued_at ASC (oldest first). Pagination uses a queued_at cursor
// encoded as RFC3339Nano; an empty cursor starts from the oldest row.
func (q *Queries) ListAuditEventsDeadletterByProject(ctx context.Context, projectID string, limit int, cursor string) ([]domain.AuditEvent, []string, []string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditEventsDeadletterByProject")
	defer span.End()

	if projectID == "" {
		return nil, nil, nil, fmt.Errorf("project_id is required")
	}
	if limit <= 0 {
		limit = 50
	}

	var rows pgx.Rows
	var err error
	if cursor == "" {
		rows, err = q.db.Query(ctx, `
			SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
			       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version,
			       queued_at
			FROM audit_events_deadletter
			WHERE project_id = $1
			ORDER BY queued_at ASC, id ASC
			LIMIT $2
		`, projectID, limit)
	} else {
		cursorTime, parseErr := time.Parse(time.RFC3339Nano, cursor)
		if parseErr != nil {
			return nil, nil, nil, fmt.Errorf("invalid cursor: %w", parseErr)
		}
		rows, err = q.db.Query(ctx, `
			SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
			       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version,
			       queued_at
			FROM audit_events_deadletter
			WHERE project_id = $1 AND queued_at > $2
			ORDER BY queued_at ASC, id ASC
			LIMIT $3
		`, projectID, cursorTime, limit)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list audit deadletter by project: %w", err)
	}
	defer rows.Close()

	var events []domain.AuditEvent
	var ids []string
	var cursors []string
	for rows.Next() {
		var ev domain.AuditEvent
		var queuedAt time.Time
		if scanErr := rows.Scan(
			&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action,
			&ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.CreatedAt,
			&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion,
			&queuedAt,
		); scanErr != nil {
			return nil, nil, nil, fmt.Errorf("scan audit deadletter row: %w", scanErr)
		}
		events = append(events, ev)
		ids = append(ids, ev.ID)
		cursors = append(cursors, queuedAt.Format(time.RFC3339Nano))
	}
	return events, ids, cursors, rows.Err()
}

// GetAuditEventDeadletter fetches a single deadletter event by id,
// constrained to projectID for tenant isolation. Returns ErrNotFound-style
// nil, nil when the row doesn't exist or belongs to a different project —
// callers must map nil to a 404 without leaking cross-tenant existence.
func (q *Queries) GetAuditEventDeadletter(ctx context.Context, id, projectID string) (*domain.AuditEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAuditEventDeadletter")
	defer span.End()

	if id == "" || projectID == "" {
		return nil, fmt.Errorf("id and project_id are required")
	}

	var ev domain.AuditEvent
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
		       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version
		FROM audit_events_deadletter
		WHERE id = $1 AND project_id = $2
	`, id, projectID).Scan(
		&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action,
		&ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.CreatedAt,
		&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get audit deadletter: %w", err)
	}
	return &ev, nil
}

// DeleteAuditEventDeadletter removes a single row from the deadletter
// after successful reclamation into the primary audit_events table.
func (q *Queries) DeleteAuditEventDeadletter(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditEventDeadletter")
	defer span.End()

	_, err := q.db.Exec(ctx, `DELETE FROM audit_events_deadletter WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete audit deadletter: %w", err)
	}
	return nil
}
