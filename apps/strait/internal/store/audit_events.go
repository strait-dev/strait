package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// auditEventPostInsertHook is a test-only seam invoked inside
// CreateAuditEvent immediately AFTER the signed INSERT statement succeeds
// but BEFORE the surrounding transaction commits. When non-nil and it
// returns a non-nil error, CreateAuditEvent propagates the error and the
// transaction rolls back — leaving no audit_events row behind. Production
// builds leave this nil; do not expose through any public API.
//
//nolint:gochecknoglobals // test seam
var auditEventPostInsertHook func(ctx context.Context) error

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
	ev.Details = details

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

	// Atomic signed insert.
	//
	// The chain lock, the previous-hash read, the signature computation and
	// the INSERT all run inside a single transaction. Every committed row
	// carries a non-empty signature — there is no observable intermediate
	// "empty signature" state that VerifyAuditChain could later flag as a
	// broken chain. If any step fails, the tx rolls back and no row is left
	// behind.
	//
	// withTxInheritKeys opens a fresh tx when q is pool-backed, or runs
	// inline when q is already tx-backed (e.g. called from rotation or
	// from the retention tombstone path inside an outer transaction).
	return q.withTxInheritKeys(ctx, func(tx *Queries) error {
		// Chain lock. Prefix "audit_chain:" namespaces the lock hash so it
		// cannot collide with rotation's "audit_rotate:" prefix — both
		// touch audit_events for the same project but serialize on their
		// own domain. The lock is transaction-scoped.
		if _, err := tx.db.Exec(ctx, `
			SELECT pg_advisory_xact_lock(hashtext('audit_chain:' || $1::text))
		`, ev.ProjectID); err != nil {
			return fmt.Errorf("create audit event: chain lock: %w", err)
		}

		// Read the tail signature under the lock so no concurrent writer
		// can slip a row between this read and our insert.
		var prevHash string
		if err := tx.db.QueryRow(ctx, `
			SELECT COALESCE(
			    (SELECT signature FROM audit_events
			     WHERE project_id = $1 AND signature != ''
			     ORDER BY rotation_epoch DESC, created_at DESC LIMIT 1),
			    $2
			)
		`, ev.ProjectID, ZeroHash).Scan(&prevHash); err != nil {
			return fmt.Errorf("create audit event: read prev hash: %w", err)
		}
		ev.PreviousHash = prevHash

		// Compute the HMAC signature in Go against the fully-populated
		// event (including previous_hash). When no signing key is
		// configured — legacy unit-test / bootstrap installs — we insert
		// with an empty signature; VerifyAuditChain gates on
		// q.auditSigningKey != nil and is never called in that mode.
		signature := ""
		if tx.auditSigningKey != nil {
			signingKey, err := tx.resolveSigningKeyForEpoch(ctx, ev.ProjectID, ev.RotationEpoch)
			if err != nil {
				return err
			}
			signature = ComputeAuditSignature(ev, signingKey)
		}
		ev.Signature = signature

		if _, err := tx.db.Exec(ctx, `
			INSERT INTO audit_events (
				id, project_id, actor_id, actor_type, action, resource_type, resource_id,
				details, signature, previous_hash, created_at,
				remote_ip, user_agent, request_id, trace_id, schema_version,
				is_anchor, rotation_epoch
			)
			VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8::jsonb, $9, $10, $11,
				$12, $13, $14, $15, $16,
				$17, $18
			)
		`,
			ev.ID, ev.ProjectID, ev.ActorID, ev.ActorType, ev.Action, ev.ResourceType, ev.ResourceID,
			details, signature, ev.PreviousHash, ev.CreatedAt,
			ev.RemoteIP, ev.UserAgent, ev.RequestID, ev.TraceID, ev.SchemaVersion,
			ev.IsAnchor, ev.RotationEpoch,
		); err != nil {
			return fmt.Errorf("create audit event: insert: %w", err)
		}

		if hook := auditEventPostInsertHook; hook != nil {
			if err := hook(ctx); err != nil {
				return fmt.Errorf("create audit event: post-insert hook: %w", err)
			}
		}
		return nil
	})
}

// resolveSigningKeyForEpoch returns the per-epoch HMAC signing key used for
// both signing and verification. When secretEncryptionKey is configured, the
// key is looked up in audit_signing_keys and, on first write for an epoch,
// bootstrapped from q.auditSigningKey so signer and verifier converge on a
// stable per-epoch key independent of in-memory mutations. When no encryption
// key is configured (unit-test / pre-rotation installs), the in-memory
// q.auditSigningKey is used directly — matching the legacy behavior.
func (q *Queries) resolveSigningKeyForEpoch(ctx context.Context, projectID string, epoch int) ([]byte, error) {
	if q.secretEncryptionKey == "" {
		return q.auditSigningKey, nil
	}
	stored, err := q.GetAuditSigningKey(ctx, projectID, epoch)
	if err != nil {
		return nil, fmt.Errorf("resolve signing key: %w", err)
	}
	if stored != nil {
		return stored, nil
	}
	// Bootstrap: derive a per-(project, epoch) HMAC key from INTERNAL_SECRET
	// and persist it as the stable per-epoch key. This guarantees that every
	// project receives a cryptographically independent signing key even for
	// the pre-rotation epoch — the global q.auditSigningKey is identical
	// across projects and must not be used as the per-epoch key material.
	// The global key remains only as the legacy fallback in VerifyAuditChain
	// for chains written before per-epoch keys existed (epoch 0 with no row).
	// Races are resolved by ON CONFLICT DO NOTHING followed by a re-read.
	derivedKey, err := DeriveAuditSigningKeyForEpoch(q.secretEncryptionKey, projectID, epoch)
	if err != nil {
		return nil, fmt.Errorf("resolve signing key: derive: %w", err)
	}
	envelopeKey, err := q.secretKey()
	if err != nil {
		return nil, fmt.Errorf("resolve signing key: envelope: %w", err)
	}
	ciphertext, err := encryptAuditKey(derivedKey, envelopeKey)
	if err != nil {
		return nil, fmt.Errorf("resolve signing key: encrypt: %w", err)
	}
	if _, err := q.db.Exec(ctx, `
		INSERT INTO audit_signing_keys (project_id, rotation_epoch, key_material)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, rotation_epoch) DO NOTHING
	`, projectID, epoch, ciphertext); err != nil {
		return nil, fmt.Errorf("resolve signing key: bootstrap insert: %w", err)
	}
	// Re-read: on conflict, the winning row's ciphertext is what both future
	// signers and verifiers must use.
	stored, err = q.GetAuditSigningKey(ctx, projectID, epoch)
	if err != nil {
		return nil, fmt.Errorf("resolve signing key: re-read: %w", err)
	}
	if stored == nil {
		// Should not happen — we just inserted. Fall back to in-memory.
		return q.auditSigningKey, nil
	}
	return stored, nil
}

func (q *Queries) ListAuditEvents(ctx context.Context, projectID, actorID, resourceType, resourceID string, limit int, cursor, from, to *time.Time, ascending bool) ([]domain.AuditEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditEvents")
	defer span.End()

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch
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
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		events = append(events, ev)
	}

	return events, rows.Err()
}

// GetAuditEvent fetches a single audit event by id, scoped to the
// caller's project. Returns ErrAuditEventNotFound when the row does
// not exist within the project — cross-tenant reads are surfaced as
// a plain not-found to avoid leaking existence of rows in other
// projects.
func (q *Queries) GetAuditEvent(ctx context.Context, projectID, id string) (*domain.AuditEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAuditEvent")
	defer span.End()

	const query = `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch
		FROM audit_events
		WHERE id = $1 AND project_id = $2`

	var ev domain.AuditEvent
	err := q.db.QueryRow(ctx, query, id, projectID).Scan(
		&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType,
		&ev.Action, &ev.ResourceType, &ev.ResourceID,
		&ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt,
		&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID,
		&ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAuditEventNotFound
		}
		return nil, fmt.Errorf("get audit event: %w", err)
	}
	return &ev, nil
}

func (q *Queries) StreamAuditEvents(ctx context.Context, projectID, actorID, resourceType string, from, to time.Time, fn func(*domain.AuditEvent) error) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.StreamAuditEvents")
	defer span.End()

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch
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
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch); err != nil {
			return fmt.Errorf("scan audit event: %w", err)
		}
		if err := fn(&ev); err != nil {
			return err
		}
	}

	return rows.Err()
}

// withTxInheritKeys runs fn inside a fresh transaction when q is pool-backed,
// or inline on q when it is already tx-backed. The nested Queries inherits the
// audit signing key and secret encryption key so audit writes produced by fn
// (e.g. the retention tombstone) sign correctly.
func (q *Queries) withTxInheritKeys(ctx context.Context, fn func(*Queries) error) error {
	begin, ok := q.db.(TxBeginner)
	if !ok {
		return fn(q)
	}
	return WithTx(ctx, begin, func(txQ *Queries) error {
		txQ.auditSigningKey = q.auditSigningKey
		txQ.secretEncryptionKey = q.secretEncryptionKey
		return fn(txQ)
	})
}

// tombstoneInsertHook is a test-only injection point invoked immediately
// before the tombstone anchor insert inside writeRetentionTombstone. When
// non-nil and it returns a non-nil error, writeRetentionTombstone aborts
// with that error — which (because the tombstone runs inside the same
// transaction as the DELETE) triggers a rollback of the trim. Production
// builds leave this nil and it is a true no-op. Do not expose through any
// public API.
//
//nolint:gochecknoglobals // test seam
var tombstoneInsertHook func(ctx context.Context) error

// writeRetentionTombstone inserts a tombstone anchor row for a project that
// just had rows trimmed. It is called inside the same transaction as the
// DELETE so the tombstone and the trim commit (or roll back) together.
//
// The tombstone's previous_hash is the most recent surviving row's signature
// (or ZeroHash if the trim removed every row in the project's chain). Its
// details carry {deleted_count, trimmed_before, previous_hash}. The row is
// inserted via CreateAuditEvent so it obtains a real HMAC signature chained
// from the surviving tail — VerifyAuditChain then naturally accepts it as a
// chain-valid forensic marker.
func (q *Queries) writeRetentionTombstone(ctx context.Context, projectID string, cutoff time.Time, deleted int64) error {
	// Read the max rotation_epoch for the project so the tombstone lives in
	// the current epoch. If there are no surviving rows, default to 0.
	var rotationEpoch int
	if err := q.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(rotation_epoch), 0)
		FROM audit_events
		WHERE project_id = $1
	`, projectID).Scan(&rotationEpoch); err != nil {
		return fmt.Errorf("tombstone: read rotation_epoch: %w", err)
	}

	// Capture the surviving chain tail signature for informational display in
	// details. CreateAuditEvent will independently re-read and chain from the
	// same tail via its CTE under pg_advisory_xact_lock.
	var prevHash string
	if err := q.db.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT signature FROM audit_events
			 WHERE project_id = $1 AND signature != ''
			 ORDER BY created_at DESC LIMIT 1),
			$2
		)
	`, projectID, ZeroHash).Scan(&prevHash); err != nil {
		return fmt.Errorf("tombstone: read prev_hash: %w", err)
	}

	details, err := json.Marshal(map[string]any{
		"deleted_count":  deleted,
		"trimmed_before": cutoff.UTC().Format(time.RFC3339),
		"previous_hash":  prevHash,
	})
	if err != nil {
		return fmt.Errorf("tombstone: marshal details: %w", err)
	}

	ev := &domain.AuditEvent{
		ProjectID:     projectID,
		ActorID:       "system",
		ActorType:     "system",
		Action:        domain.AuditActionRetentionTrimmed,
		ResourceType:  "audit_events",
		ResourceID:    fmt.Sprintf("retention-%s", cutoff.UTC().Format(time.RFC3339)),
		Details:       json.RawMessage(details),
		IsAnchor:      true,
		RotationEpoch: rotationEpoch,
	}
	if hook := tombstoneInsertHook; hook != nil {
		if err := hook(ctx); err != nil {
			return fmt.Errorf("tombstone: pre-insert hook: %w", err)
		}
	}
	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		return fmt.Errorf("tombstone: create anchor: %w", err)
	}
	return nil
}

// DeleteAuditEventsBefore deletes audit events older than cutoff for a
// project and, if any rows were trimmed, writes a tombstone anchor row
// (action=audit.retention_trimmed, is_anchor=true) inside the same
// transaction. The tombstone gives a SOC 2 auditor positive forensic proof
// that a retention trim happened.
//
// This is tail-only: only the oldest rows are removed, which preserves chain
// verifiability (the earliest surviving event's previous_hash becomes the
// chain anchor in VerifyAuditChain, followed by the tombstone as a positive
// marker).
//
// Returns the number of event rows deleted (the tombstone is not counted).
func (q *Queries) DeleteAuditEventsBefore(ctx context.Context, projectID string, cutoff time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditEventsBefore")
	defer span.End()

	var deleted int64
	err := q.withTxInheritKeys(ctx, func(tx *Queries) error {
		var (
			tag   pgconn.CommandTag
			execE error
		)
		if projectID == "" {
			tag, execE = tx.db.Exec(ctx, `
				DELETE FROM audit_events
				WHERE created_at < $1
			`, cutoff)
		} else {
			tag, execE = tx.db.Exec(ctx, `
				DELETE FROM audit_events
				WHERE project_id = $1 AND created_at < $2
			`, projectID, cutoff)
		}
		if execE != nil {
			return fmt.Errorf("delete audit events before: %w", execE)
		}
		deleted = tag.RowsAffected()
		if deleted == 0 || projectID == "" {
			// No tombstone on zero deletes. An empty projectID is a legacy
			// "trim globally" path kept for callers that don't know the
			// per-project membership; the scheduler uses
			// DeleteAuditEventsBeforeExcluding which emits one tombstone per
			// affected project instead.
			return nil
		}
		return tx.writeRetentionTombstone(ctx, projectID, cutoff, deleted)
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// DeleteAuditEventsBeforeExcluding trims audit events across all projects
// except those listed in excludeProjectIDs. Used by the retention reaper to
// apply the server-wide default to every project that does not have a
// per-project override in project_quotas.audit_retention_days.
//
// Emits one tombstone anchor row per affected project. The set of affected
// projects is computed inside the transaction (distinct project_ids with
// rows < cutoff and not excluded) so the tombstone set exactly mirrors the
// trim scope.
func (q *Queries) DeleteAuditEventsBeforeExcluding(ctx context.Context, cutoff time.Time, excludeProjectIDs []string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditEventsBeforeExcluding")
	defer span.End()

	var total int64
	err := q.withTxInheritKeys(ctx, func(tx *Queries) error {
		// Discover affected projects before the DELETE so we can emit one
		// tombstone per project after trimming.
		var (
			rows pgx.Rows
			e    error
		)
		if len(excludeProjectIDs) == 0 {
			rows, e = tx.db.Query(ctx, `
				SELECT DISTINCT project_id
				FROM audit_events
				WHERE created_at < $1
			`, cutoff)
		} else {
			rows, e = tx.db.Query(ctx, `
				SELECT DISTINCT project_id
				FROM audit_events
				WHERE created_at < $1 AND project_id <> ALL($2::text[])
			`, cutoff, excludeProjectIDs)
		}
		if e != nil {
			return fmt.Errorf("discover affected projects: %w", e)
		}
		var affected []string
		for rows.Next() {
			var pid string
			if scanErr := rows.Scan(&pid); scanErr != nil {
				rows.Close()
				return fmt.Errorf("scan affected project: %w", scanErr)
			}
			affected = append(affected, pid)
		}
		rows.Close()
		if rowsErr := rows.Err(); rowsErr != nil {
			return fmt.Errorf("rows err: %w", rowsErr)
		}

		// Trim per-project so we know the exact delete count for each
		// tombstone. One statement per project keeps the tombstone's
		// deleted_count honest.
		for _, pid := range affected {
			tag, execE := tx.db.Exec(ctx, `
				DELETE FROM audit_events
				WHERE project_id = $1 AND created_at < $2
			`, pid, cutoff)
			if execE != nil {
				return fmt.Errorf("delete audit events before (project %s): %w", pid, execE)
			}
			n := tag.RowsAffected()
			if n == 0 {
				continue
			}
			total += n
			if err := tx.writeRetentionTombstone(ctx, pid, cutoff, n); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
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

	// Ordering: rotation_epoch ASC, then created_at ASC. Anchor rows are
	// the first row of their new epoch by construction (they are inserted
	// under the newly-bumped epoch before any other post-rotation writes).
	// Within an epoch, created_at preserves insertion order as the chain
	// serialization is HMAC-bound to previous_hash.
	//
	// Per-epoch keys: rows are grouped by rotation_epoch and verified
	// under the key stored in audit_signing_keys for that (project, epoch).
	// Epoch 0 has no stored key — we fall back to the configured global
	// q.auditSigningKey for backwards compatibility with chains that
	// predate per-epoch key derivation. Anchor rows must verify under
	// their OWN epoch's (new) key, not the previous epoch's, because
	// post-rotation events chain from the anchor and verify under the
	// same new key.
	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch
		FROM audit_events
		WHERE project_id = $1
		ORDER BY rotation_epoch ASC, created_at ASC`

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

	// epochKeyCache memoizes the per-epoch signing key for the scan so we
	// don't re-decrypt on every row. A nil value means "no stored key —
	// use the global fallback" (only valid for epoch 0).
	epochKeyCache := make(map[int][]byte)
	keyFor := func(epoch int) ([]byte, error) {
		if k, ok := epochKeyCache[epoch]; ok {
			if k != nil {
				return k, nil
			}
			return q.auditSigningKey, nil
		}
		stored, err := q.GetAuditSigningKey(ctx, projectID, epoch)
		if err != nil {
			return nil, err
		}
		epochKeyCache[epoch] = stored
		if stored != nil {
			return stored, nil
		}
		if epoch != 0 {
			return nil, fmt.Errorf("verify audit chain: no stored key for epoch %d", epoch)
		}
		return q.auditSigningKey, nil
	}

	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch); err != nil {
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

		if ev.PreviousHash != expectedPrevHash {
			result.Valid = false
			result.BrokenAtID = ev.ID
			result.Error = fmt.Sprintf("chain broken at event %s: previous_hash mismatch (expected %s, got %s)", ev.ID, expectedPrevHash, ev.PreviousHash)
			return result, nil
		}

		key, keyErr := keyFor(ev.RotationEpoch)
		if keyErr != nil {
			return nil, keyErr
		}
		expected := ComputeAuditSignature(&ev, key)
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
