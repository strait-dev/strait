package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
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
// Ordered by queued_at ASC (oldest first). Pagination uses a composite
// queued_at|id cursor so rows sharing the same queued_at timestamp cannot be
// skipped between pages. For compatibility with older clients, a bare
// RFC3339Nano cursor is still accepted and behaves as queued_at-only.
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
		cursorTime, cursorID, parseErr := parseAuditDeadletterCursor(cursor)
		if parseErr != nil {
			return nil, nil, nil, fmt.Errorf("invalid cursor: %w", parseErr)
		}
		if cursorID == "" {
			rows, err = q.db.Query(ctx, `
				SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
				       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version,
				       queued_at
				FROM audit_events_deadletter
				WHERE project_id = $1 AND queued_at > $2
				ORDER BY queued_at ASC, id ASC
				LIMIT $3
			`, projectID, cursorTime, limit)
		} else {
			rows, err = q.db.Query(ctx, `
				SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
				       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version,
				       queued_at
				FROM audit_events_deadletter
				WHERE project_id = $1
				  AND (queued_at > $2 OR (queued_at = $2 AND id > $3))
				ORDER BY queued_at ASC, id ASC
				LIMIT $4
			`, projectID, cursorTime, cursorID, limit)
		}
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
		cursors = append(cursors, auditDeadletterCursor(queuedAt, ev.ID))
	}
	return events, ids, cursors, rows.Err()
}

func auditDeadletterCursor(queuedAt time.Time, id string) string {
	return queuedAt.UTC().Format(time.RFC3339Nano) + "|" + id
}

func parseAuditDeadletterCursor(cursor string) (time.Time, string, error) {
	if idx := strings.LastIndex(cursor, "|"); idx > 0 {
		ts, err := time.Parse(time.RFC3339Nano, cursor[:idx])
		if err != nil {
			return time.Time{}, "", err
		}
		return ts, cursor[idx+1:], nil
	}
	ts, err := time.Parse(time.RFC3339Nano, cursor)
	return ts, "", err
}

// GetAuditEventDeadletter fetches a single deadletter event by id,
// constrained to projectID for tenant isolation. Returns ErrNotFound-style
// nil, nil when the row doesn't exist or belongs to a different project —
// callers must map nil to a 404 without leaking cross-tenant existence.
func (q *Queries) GetAuditEventDeadletter(ctx context.Context, id, projectID string) (*domain.AuditEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAuditEventDeadletter")
	defer span.End()

	if !auditDeadletterScopedIDValid(id, projectID) {
		return nil, fmt.Errorf("id and project_id are required")
	}

	var ev domain.AuditEvent
	err := q.db.QueryRow(ctx, `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
		       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version
		FROM audit_events_deadletter
		WHERE id = $1 AND project_id = $2 AND reclaimed_event_id IS NULL
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

// ReplayAuditEventDeadletter atomically moves one deadletter row into the
// signed audit chain and removes the DLQ row. The deadletter row is locked with
// FOR UPDATE before insertion so concurrent admin/reaper attempts cannot insert
// duplicate audit-chain events for the same DLQ id.
func (q *Queries) ReplayAuditEventDeadletter(ctx context.Context, id, projectID, newEventID string) (*domain.AuditEvent, bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReplayAuditEventDeadletter")
	defer span.End()

	if !auditDeadletterReplayIDsValid(id, projectID, newEventID) {
		return nil, false, fmt.Errorf("id, project_id, and new_event_id are required")
	}

	if tx, ok := TxFromContext(ctx); ok {
		return q.replayAuditEventDeadletterTx(ctx, tx, id, projectID, newEventID)
	}

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return nil, false, fmt.Errorf("replay audit deadletter: db does not support transactions")
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("replay audit deadletter: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	ev, replayed, err := q.replayAuditEventDeadletterTx(ctx, tx, id, projectID, newEventID)
	if err != nil || !replayed {
		return ev, replayed, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("replay audit deadletter: commit tx: %w", err)
	}
	return ev, true, nil
}

func (q *Queries) replayAuditEventDeadletterTx(ctx context.Context, tx pgx.Tx, id, projectID, newEventID string) (*domain.AuditEvent, bool, error) {
	txq := q.withDB(tx)
	var ev domain.AuditEvent
	err := tx.QueryRow(ctx, `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
		       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version
		FROM audit_events_deadletter
		WHERE id = $1 AND project_id = $2 AND reclaimed_event_id IS NULL
		FOR UPDATE
	`, id, projectID).Scan(
		&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action,
		&ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.CreatedAt,
		&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("replay audit deadletter: lock row: %w", err)
	}

	ev.ID = newEventID
	if err := txq.CreateAuditEvent(ctx, &ev); err != nil {
		return nil, false, fmt.Errorf("replay audit deadletter: create audit event: %w", err)
	}

	if tag, err := tx.Exec(ctx, `
		UPDATE audit_events_deadletter
		SET reclaimed_event_id = $3
		WHERE id = $1 AND project_id = $2 AND reclaimed_event_id IS NULL
	`, id, projectID, newEventID); err != nil {
		return nil, false, fmt.Errorf("replay audit deadletter: mark reclaimed: %w", err)
	} else if tag.RowsAffected() == 0 {
		return nil, false, fmt.Errorf("replay audit deadletter: lost deadletter row after lock")
	}

	if tag, err := tx.Exec(ctx, `DELETE FROM audit_events_deadletter WHERE id = $1 AND project_id = $2`, id, projectID); err != nil {
		return nil, false, fmt.Errorf("replay audit deadletter: delete dlq row: %w", err)
	} else if tag.RowsAffected() == 0 {
		return nil, false, fmt.Errorf("replay audit deadletter: delete matched no row")
	}

	return &ev, true, nil
}

// DeleteAuditEventDeadletter removes a single row from the deadletter
// after successful reclamation into the primary audit_events table.
// projectID is required for tenant isolation: the DELETE is scoped to
// both id and project_id so a caller with a cross-tenant id cannot
// accidentally (or maliciously) remove another project's DLQ row.
func (q *Queries) DeleteAuditEventDeadletter(ctx context.Context, id, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditEventDeadletter")
	defer span.End()

	if !auditDeadletterScopedIDValid(id, projectID) {
		return fmt.Errorf("id and project_id are required")
	}

	tag, err := q.db.Exec(ctx, `DELETE FROM audit_events_deadletter WHERE id = $1 AND project_id = $2`, id, projectID)
	if err != nil {
		return fmt.Errorf("delete audit deadletter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete audit deadletter: no row matched id=%s project_id=%s", id, projectID)
	}
	return nil
}

func auditDeadletterScopedIDValid(id, projectID string) bool {
	return id != "" && projectID != ""
}

func auditDeadletterReplayIDsValid(id, projectID, newEventID string) bool {
	return auditDeadletterScopedIDValid(id, projectID) && newEventID != ""
}

// DropAuditEventDeadletterWithAudit atomically records the operator drop in
// the signed audit chain and removes the deadletter row. The audit insert is
// deliberately inside the same transaction as the delete so an operator cannot
// permanently discard DLQ evidence without a durable self-audit record.
func (q *Queries) DropAuditEventDeadletterWithAudit(ctx context.Context, id, projectID string, auditEvent *domain.AuditEvent) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DropAuditEventDeadletterWithAudit")
	defer span.End()

	if !auditDeadletterScopedIDValid(id, projectID) {
		return false, fmt.Errorf("id and project_id are required")
	}
	if auditEvent == nil {
		return false, fmt.Errorf("audit event is required")
	}
	if auditEvent.ProjectID != "" && auditEvent.ProjectID != projectID {
		return false, fmt.Errorf("audit event project_id %q does not match deadletter project_id %q", auditEvent.ProjectID, projectID)
	}
	auditEvent.ProjectID = projectID

	if tx, ok := TxFromContext(ctx); ok {
		return q.dropAuditEventDeadletterWithAuditTx(ctx, tx, id, projectID, auditEvent)
	}

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return false, fmt.Errorf("drop audit deadletter with audit: db does not support transactions")
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("drop audit deadletter with audit: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	dropped, err := q.dropAuditEventDeadletterWithAuditTx(ctx, tx, id, projectID, auditEvent)
	if err != nil || !dropped {
		return dropped, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("drop audit deadletter with audit: commit tx: %w", err)
	}
	return true, nil
}

func (q *Queries) dropAuditEventDeadletterWithAuditTx(ctx context.Context, tx pgx.Tx, id, projectID string, auditEvent *domain.AuditEvent) (bool, error) {
	var lockedID string
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM audit_events_deadletter
		WHERE id = $1 AND project_id = $2 AND reclaimed_event_id IS NULL
		FOR UPDATE
	`, id, projectID).Scan(&lockedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("drop audit deadletter with audit: lock row: %w", err)
	}

	if err := q.withDB(tx).CreateAuditEvent(ctx, auditEvent); err != nil {
		return false, fmt.Errorf("drop audit deadletter with audit: create audit event: %w", err)
	}
	if tag, err := tx.Exec(ctx, `
		DELETE FROM audit_events_deadletter
		WHERE id = $1 AND project_id = $2 AND reclaimed_event_id IS NULL
	`, id, projectID); err != nil {
		return false, fmt.Errorf("drop audit deadletter with audit: delete dlq row: %w", err)
	} else if tag.RowsAffected() == 0 {
		return false, fmt.Errorf("drop audit deadletter with audit: delete matched no row")
	}
	return true, nil
}

// AuditDeadletterAttemptInfo carries the fields the reclaimer needs to make
// idempotency and max-attempts decisions without re-reading the row.
type AuditDeadletterAttemptInfo struct {
	AttemptCount     int
	ReclaimedEventID *string
}

// ListAuditEventsDeadletterWithAttempts returns the oldest deadletter events
// for reclamation along with each row's current attempt_count and any
// previously-set reclaimed_event_id. Behaves like ListAuditEventsDeadletter
// but the extra columns let the reclaimer enforce a max-attempts cap and
// detect previously-reclaimed rows that only need the DLQ delete.
func (q *Queries) ListAuditEventsDeadletterWithAttempts(ctx context.Context, limit int) ([]domain.AuditEvent, []string, []AuditDeadletterAttemptInfo, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditEventsDeadletterWithAttempts")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
		       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version,
		       attempt_count, reclaimed_event_id
		FROM audit_events_deadletter
		ORDER BY queued_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list audit deadletter with attempts: %w", err)
	}
	defer rows.Close()

	var (
		events []domain.AuditEvent
		ids    []string
		info   []AuditDeadletterAttemptInfo
	)
	for rows.Next() {
		var ev domain.AuditEvent
		var attemptCount int
		var reclaimedID *string
		if err := rows.Scan(
			&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action,
			&ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.CreatedAt,
			&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion,
			&attemptCount, &reclaimedID,
		); err != nil {
			return nil, nil, nil, fmt.Errorf("scan audit deadletter row: %w", err)
		}
		events = append(events, ev)
		ids = append(ids, ev.ID)
		info = append(info, AuditDeadletterAttemptInfo{
			AttemptCount:     attemptCount,
			ReclaimedEventID: reclaimedID,
		})
	}
	return events, ids, info, rows.Err()
}

// IncrementAuditDeadletterAttempt bumps attempt_count by one for the
// supplied DLQ row id. Used by the reclaimer to track failed replay
// attempts so the max-attempts cap eventually retires permanently
// poisoned rows.
func (q *Queries) IncrementAuditDeadletterAttempt(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.IncrementAuditDeadletterAttempt")
	defer span.End()

	_, err := q.db.Exec(ctx,
		`UPDATE audit_events_deadletter SET attempt_count = attempt_count + 1 WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("increment audit deadletter attempt: %w", err)
	}
	return nil
}

// MarkAuditDeadletterReclaimed records the new chain event id on the DLQ row
// so a subsequent replay (admin or reclaimer) can detect that the chain
// insert already happened and skip it, performing only the DLQ delete. This
// is the idempotency anchor for at-least-once replay semantics.
func (q *Queries) MarkAuditDeadletterReclaimed(ctx context.Context, dlqID, newEventID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkAuditDeadletterReclaimed")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`UPDATE audit_events_deadletter SET reclaimed_event_id = $2 WHERE id = $1`,
		dlqID, newEventID,
	)
	if err != nil {
		return fmt.Errorf("mark audit deadletter reclaimed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark audit deadletter reclaimed: no row matched id=%s", dlqID)
	}
	return nil
}

// DeleteAuditDeadletterOlderThan removes deadletter rows whose original
// event timestamp (created_at) is older than cutoff. Returns per-project
// counts so the caller can emit one audit.deadletter_aged event per
// affected project. Bounded by limit so a single sweep cannot lock the
// table for too long.
//
// We key on created_at, not queued_at: an event that arrived in the DLQ
// late but whose original event is recent should not be aged out. The
// retention contract is "drop deadlettered events whose original creation
// time is older than N days".
func (q *Queries) DeleteAuditDeadletterOlderThan(ctx context.Context, cutoff time.Time) (map[string]int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditDeadletterOlderThan")
	defer span.End()

	const batchLimit = 1000
	rows, err := q.db.Query(ctx, `
		WITH to_delete AS (
			SELECT id FROM audit_events_deadletter
			WHERE created_at >= TIMESTAMPTZ '2000-01-01'
			  AND created_at < $1
			LIMIT $2
		),
		deleted AS (
			DELETE FROM audit_events_deadletter
			WHERE id IN (SELECT id FROM to_delete)
			RETURNING project_id
		)
		SELECT project_id, COUNT(*) AS dropped
		FROM deleted
		GROUP BY project_id
	`, cutoff, batchLimit)
	if err != nil {
		return nil, fmt.Errorf("delete audit deadletter older than: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64)
	for rows.Next() {
		var pid string
		var dropped int64
		if err := rows.Scan(&pid, &dropped); err != nil {
			return nil, fmt.Errorf("scan deadletter retention row: %w", err)
		}
		out[pid] = dropped
	}
	return out, rows.Err()
}

// DeleteAuditDeadletterOlderThanWithAudit atomically drops aged deadletter rows
// and writes one audit.deadletter_aged marker per affected project. If any
// marker cannot be signed or inserted, the transaction rolls back and the DLQ
// evidence remains for a later retry.
func (q *Queries) DeleteAuditDeadletterOlderThanWithAudit(ctx context.Context, cutoff time.Time, maxAgeDays int) (map[string]int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditDeadletterOlderThanWithAudit")
	defer span.End()

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("delete audit deadletter older than with audit: db does not support transactions")
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete audit deadletter older than with audit: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	dropped, err := q.deleteAuditDeadletterOlderThanWithAuditTx(ctx, tx, cutoff, maxAgeDays)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("delete audit deadletter older than with audit: commit tx: %w", err)
	}
	return dropped, nil
}

func (q *Queries) deleteAuditDeadletterOlderThanWithAuditTx(ctx context.Context, tx pgx.Tx, cutoff time.Time, maxAgeDays int) (map[string]int64, error) {
	const batchLimit = 1000
	rows, err := tx.Query(ctx, `
		SELECT id, project_id
		FROM audit_events_deadletter
		WHERE created_at >= TIMESTAMPTZ '2000-01-01'
		  AND created_at < $1
		ORDER BY created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, cutoff, batchLimit)
	if err != nil {
		return nil, fmt.Errorf("delete audit deadletter older than with audit: select rows: %w", err)
	}

	idsByProject := make(map[string][]string)
	for rows.Next() {
		var id, projectID string
		if err := rows.Scan(&id, &projectID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("delete audit deadletter older than with audit: scan row: %w", err)
		}
		idsByProject[projectID] = append(idsByProject[projectID], id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("delete audit deadletter older than with audit: iterate rows: %w", err)
	}
	rows.Close()

	dropped := make(map[string]int64, len(idsByProject))
	txQ := q.withDB(tx)
	for projectID, ids := range idsByProject {
		if len(ids) == 0 {
			continue
		}
		details, err := json.Marshal(map[string]any{
			"dropped_count":  len(ids),
			"reason":         "max_age_exceeded",
			"max_age_cutoff": cutoff.Format(time.RFC3339),
			"max_age_days":   maxAgeDays,
		})
		if err != nil {
			return nil, fmt.Errorf("delete audit deadletter older than with audit: marshal marker: %w", err)
		}
		ev := &domain.AuditEvent{
			ID:           uuid.Must(uuid.NewV7()).String(),
			ProjectID:    projectID,
			ActorID:      "system",
			ActorType:    "system",
			Action:       domain.AuditActionDeadletterAged,
			ResourceType: "audit_events_deadletter",
			ResourceID:   "retention",
			Details:      json.RawMessage(details),
		}
		if err := txQ.CreateAuditEvent(ctx, ev); err != nil {
			return nil, fmt.Errorf("delete audit deadletter older than with audit: create audit event: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			DELETE FROM audit_events_deadletter
			WHERE project_id = $1 AND id = ANY($2)
		`, projectID, ids)
		if err != nil {
			return nil, fmt.Errorf("delete audit deadletter older than with audit: delete rows: %w", err)
		}
		dropped[projectID] = tag.RowsAffected()
	}
	return dropped, nil
}
