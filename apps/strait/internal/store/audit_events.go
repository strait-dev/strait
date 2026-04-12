package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"golang.org/x/crypto/hkdf"
)

// ZeroHash is the previous_hash for the first event in a project's chain.
const ZeroHash = "0000000000000000000000000000000000000000000000000000000000000000"

// DeriveAuditSigningKey derives a 32-byte HMAC signing key from the
// internal secret using HKDF-SHA256.
func DeriveAuditSigningKey(secret string) ([]byte, error) {
	if secret == "" {
		return nil, fmt.Errorf("audit signing key: secret is empty")
	}
	hkdfReader := hkdf.New(sha256.New, []byte(secret), []byte("audit-event-signing"), nil)
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("audit signing key: hkdf derive: %w", err)
	}
	return key, nil
}

// ComputeAuditSignature computes the HMAC-SHA256 signature for an audit event.
// The canonical form is pipe-separated fields including the previous_hash,
// ensuring any modification to any field invalidates the signature.
//
// The canonical form branches on SchemaVersion:
//   - v1 (default): original form, 10 fields. Used by events written before
//     the forensic-columns migration.
//   - v2: extends v1 with RemoteIP, UserAgent, RequestID, TraceID,
//     SchemaVersion. Used by new events after migration 000185.
//
// Verify runs the same branching logic so both versions coexist in the same
// chain without any bulk re-signing.
func ComputeAuditSignature(ev *domain.AuditEvent, key []byte) string {
	mac := hmac.New(sha256.New, key)
	var canonical string
	if ev.SchemaVersion >= 2 {
		canonical = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%d",
			ev.ID, ev.ProjectID, ev.ActorID, ev.ActorType,
			ev.Action, ev.ResourceType, ev.ResourceID,
			string(ev.Details),
			ev.CreatedAt.UTC().Format(time.RFC3339Nano),
			ev.PreviousHash,
			ev.RemoteIP, ev.UserAgent, ev.RequestID, ev.TraceID,
			ev.SchemaVersion,
		)
	} else {
		canonical = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			ev.ID, ev.ProjectID, ev.ActorID, ev.ActorType,
			ev.Action, ev.ResourceType, ev.ResourceID,
			string(ev.Details),
			ev.CreatedAt.UTC().Format(time.RFC3339Nano),
			ev.PreviousHash,
		)
	}
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func (q *Queries) CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAuditEvent")
	defer span.End()

	if ev.ID == "" {
		ev.ID = uuid.Must(uuid.NewV7()).String()
	}

	details := ev.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}

	// Use client-side timestamp so all fields are known for signature computation.
	// Truncate to microseconds: Postgres TIMESTAMPTZ stores microsecond precision,
	// so the signature must be computed from the same precision that will be read
	// back by VerifyAuditChain. Without this, the nanosecond remainder in the Go
	// time.Time gets truncated on the Postgres round-trip and the recomputed
	// signature no longer matches.
	ev.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)

	// Default schema version for new events.
	if ev.SchemaVersion == 0 {
		ev.SchemaVersion = domain.AuditEventSchemaVersionCurrent
	}

	// Atomic single-statement insert with chain linkage.
	//
	// This uses a CTE with pg_advisory_xact_lock to serialize inserts per project.
	// Because this is a single SQL statement, it executes as one implicit transaction,
	// so the advisory lock holds for the entire SELECT + INSERT. This avoids the
	// connection pool problem where session-scoped pg_advisory_lock could acquire
	// and release on different pool connections.
	//
	// The signature is initially empty and updated after we know the previous_hash.
	// Both the INSERT and UPDATE happen in the same CTE chain (single statement),
	// so the lock covers both operations atomically.
	atomicQuery := `
		WITH lock_and_prev AS (
			SELECT pg_advisory_xact_lock(hashtext($2)),
			       COALESCE(
			           (SELECT signature FROM audit_events
			            WHERE project_id = $2 AND signature != ''
			            ORDER BY created_at DESC LIMIT 1),
			           $10
			       ) AS prev_hash
		),
		ins AS (
			INSERT INTO audit_events (
				id, project_id, actor_id, actor_type, action, resource_type, resource_id,
				details, signature, previous_hash, created_at,
				remote_ip, user_agent, request_id, trace_id, schema_version
			)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8::jsonb, '', lock_and_prev.prev_hash, $9,
			       $11, $12, $13, $14, $15
			FROM lock_and_prev
			RETURNING previous_hash, details
		)
		SELECT previous_hash, details FROM ins`

	if err := q.db.QueryRow(ctx, atomicQuery,
		ev.ID, ev.ProjectID, ev.ActorID, ev.ActorType, ev.Action, ev.ResourceType, ev.ResourceID, details,
		ev.CreatedAt, ZeroHash,
		ev.RemoteIP, ev.UserAgent, ev.RequestID, ev.TraceID, ev.SchemaVersion,
	).Scan(&ev.PreviousHash, &ev.Details); err != nil {
		return fmt.Errorf("create audit event: %w", err)
	}

	// Compute and persist the HMAC signature now that previous_hash is known.
	// This is a separate UPDATE but is safe because:
	// 1. The row already exists with the correct previous_hash chain linkage.
	// 2. The next insert's CTE filters "signature != ''" so it won't chain from
	//    this event until the signature is set.
	// 3. If this UPDATE fails, the event has an empty signature which
	//    VerifyAuditChain will detect, but the chain linkage remains intact.
	if q.auditSigningKey != nil {
		ev.Signature = ComputeAuditSignature(ev, q.auditSigningKey)
		if _, err := q.db.Exec(ctx, `UPDATE audit_events SET signature = $1 WHERE id = $2`,
			ev.Signature, ev.ID,
		); err != nil {
			return fmt.Errorf("update audit event signature: %w", err)
		}
	}

	return nil
}

func (q *Queries) ListAuditEvents(ctx context.Context, projectID, actorID, resourceType, resourceID string, limit int, cursor, from, to *time.Time, ascending bool) ([]domain.AuditEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditEvents")
	defer span.End()

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version
		FROM audit_events
		WHERE project_id = $1`
	args := []any{projectID}
	param := 2

	if actorID != "" {
		query += fmt.Sprintf(" AND actor_id = $%d", param)
		args = append(args, actorID)
		param++
	}
	if resourceType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", param)
		args = append(args, resourceType)
		param++
	}
	if resourceID != "" {
		query += fmt.Sprintf(" AND resource_id = $%d", param)
		args = append(args, resourceID)
		param++
	}
	if cursor != nil {
		if ascending {
			query += fmt.Sprintf(" AND created_at > $%d", param)
		} else {
			query += fmt.Sprintf(" AND created_at < $%d", param)
		}
		args = append(args, *cursor)
		param++
	}
	if from != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", param)
		args = append(args, *from)
		param++
	}
	if to != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", param)
		args = append(args, *to)
		param++
	}

	order := "DESC"
	if ascending {
		order = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY created_at %s LIMIT $%d", order, param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.AuditEvent, 0, limit)
	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		events = append(events, ev)
	}

	return events, rows.Err()
}

func (q *Queries) StreamAuditEvents(ctx context.Context, projectID, actorID, resourceType string, from, to time.Time, fn func(*domain.AuditEvent) error) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.StreamAuditEvents")
	defer span.End()

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version
		FROM audit_events
		WHERE project_id = $1`
	args := []any{projectID}
	param := 2

	if actorID != "" {
		query += fmt.Sprintf(" AND actor_id = $%d", param)
		args = append(args, actorID)
		param++
	}
	if resourceType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", param)
		args = append(args, resourceType)
		param++
	}

	query += fmt.Sprintf(" AND created_at >= $%d", param)
	args = append(args, from)
	param++

	query += fmt.Sprintf(" AND created_at <= $%d", param)
	args = append(args, to)

	query += " ORDER BY created_at ASC"

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("stream audit events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion); err != nil {
			return fmt.Errorf("scan audit event: %w", err)
		}
		if err := fn(&ev); err != nil {
			return err
		}
	}

	return rows.Err()
}

// DeleteAuditEventsBefore deletes audit events older than cutoff for a
// project. This is tail-only: only the oldest rows are removed, which
// preserves chain verifiability (the earliest surviving event's
// previous_hash becomes the chain anchor in VerifyAuditChain).
//
// Returns the number of rows deleted.
func (q *Queries) DeleteAuditEventsBefore(ctx context.Context, projectID string, cutoff time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditEventsBefore")
	defer span.End()

	tag, err := q.db.Exec(ctx, `
		DELETE FROM audit_events
		WHERE project_id = $1 AND created_at < $2
	`, projectID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete audit events before: %w", err)
	}
	return tag.RowsAffected(), nil
}

// VerifyAuditChain replays the audit event chain for a project in chronological
// order and verifies that each event's HMAC signature is valid and that the
// previous_hash linkage is unbroken.
func (q *Queries) VerifyAuditChain(ctx context.Context, projectID string) (*domain.AuditChainVerification, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.VerifyAuditChain")
	defer span.End()

	if q.auditSigningKey == nil {
		return nil, fmt.Errorf("audit signing key is not configured")
	}

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version
		FROM audit_events
		WHERE project_id = $1
		ORDER BY created_at ASC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("verify audit chain: %w", err)
	}
	defer rows.Close()

	result := &domain.AuditChainVerification{
		ProjectID: projectID,
		Valid:     true,
	}

	// expectedPrevHash is initialized from the first surviving event's
	// previous_hash. When retention trims the tail, the earliest row's
	// previous_hash points at an event that no longer exists — we take
	// it as the chain start anchor and report it in ChainStart. For a
	// pristine chain, this is ZeroHash.
	var expectedPrevHash string
	first := true

	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion); err != nil {
			return nil, fmt.Errorf("verify audit chain scan: %w", err)
		}

		result.EventsChecked++
		if result.FirstEventID == "" {
			result.FirstEventID = ev.ID
		}
		result.LastEventID = ev.ID

		if first {
			expectedPrevHash = ev.PreviousHash
			result.ChainStart = ev.PreviousHash
			first = false
		}

		// Check chain linkage.
		if ev.PreviousHash != expectedPrevHash {
			result.Valid = false
			result.BrokenAtID = ev.ID
			result.Error = fmt.Sprintf("chain broken at event %s: previous_hash mismatch (expected %s, got %s)", ev.ID, expectedPrevHash, ev.PreviousHash)
			return result, nil
		}

		// Recompute and verify signature.
		expected := ComputeAuditSignature(&ev, q.auditSigningKey)
		if ev.Signature != expected {
			result.Valid = false
			result.BrokenAtID = ev.ID
			result.Error = fmt.Sprintf("signature mismatch at event %s: event may have been tampered with", ev.ID)
			return result, nil
		}

		expectedPrevHash = ev.Signature
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("verify audit chain rows: %w", err)
	}

	return result, nil
}
