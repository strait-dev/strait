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
	"strconv"
	"strings"
	"time"
	"unsafe"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
// The canonical form includes the previous_hash, ensuring any modification
// to any signed field invalidates the signature.
//
// The canonical form branches on SchemaVersion. All versions use
// length-delimited fields so adjacent values containing separator bytes cannot
// collide. v3 extends v2 with IsAnchor and RotationEpoch so forensic metadata
// is HMAC-bound.
//
// Verify runs the same branching logic so both versions coexist in the same
// chain without any bulk re-signing.
func ComputeAuditSignature(ev *domain.AuditEvent, key []byte) string {
	mac := hmac.New(sha256.New, key)
	var canonical string
	switch {
	case ev.SchemaVersion >= 4:
		canonical = auditSignatureCanonicalV4(ev)
	case ev.SchemaVersion >= 3:
		canonical = auditSignatureCanonicalV3(ev)
	case ev.SchemaVersion >= 2:
		canonical = auditSignatureCanonicalV2(ev)
	default:
		canonical = auditSignatureCanonicalV1(ev)
	}
	if canonical != "" {
		_, _ = mac.Write(unsafe.Slice(unsafe.StringData(canonical), len(canonical)))
	}
	var sum [sha256.Size]byte
	digest := mac.Sum(sum[:0])
	var out [sha256.Size * 2]byte
	hex.Encode(out[:], digest)
	return string(out[:])
}

func auditSignatureCanonicalV1(ev *domain.AuditEvent) string {
	return lengthDelimitedAuditCanonical("audit:v1\n", []string{
		ev.ID,
		ev.ProjectID,
		ev.ActorID,
		ev.ActorType,
		ev.Action,
		ev.ResourceType,
		ev.ResourceID,
		string(ev.Details),
		ev.CreatedAt.UTC().Format(time.RFC3339Nano),
		ev.PreviousHash,
	})
}

func auditSignatureCanonicalV2(ev *domain.AuditEvent) string {
	return lengthDelimitedAuditCanonical("audit:v2\n", []string{
		ev.ID,
		ev.ProjectID,
		ev.ActorID,
		ev.ActorType,
		ev.Action,
		ev.ResourceType,
		ev.ResourceID,
		string(ev.Details),
		ev.CreatedAt.UTC().Format(time.RFC3339Nano),
		ev.PreviousHash,
		ev.RemoteIP,
		ev.UserAgent,
		ev.RequestID,
		ev.TraceID,
		strconv.FormatUint(uint64(ev.SchemaVersion), 10),
	})
}

func auditSignatureCanonicalV3(ev *domain.AuditEvent) string {
	return lengthDelimitedAuditCanonical("audit:v3\n", []string{
		ev.ID,
		ev.ProjectID,
		ev.ActorID,
		ev.ActorType,
		ev.Action,
		ev.ResourceType,
		ev.ResourceID,
		string(ev.Details),
		ev.CreatedAt.UTC().Format(time.RFC3339Nano),
		ev.PreviousHash,
		ev.RemoteIP,
		ev.UserAgent,
		ev.RequestID,
		ev.TraceID,
		strconv.FormatUint(uint64(ev.SchemaVersion), 10),
		strconv.FormatBool(ev.IsAnchor),
		strconv.Itoa(ev.RotationEpoch),
	})
}

// v4 extends v3 with ShardID so the per-shard chain identity is bound to
// the HMAC. A row's shard_id cannot be flipped without invalidating its
// signature; the verifier's per-shard grouping is therefore tamper-evident
// and not just a soft convention.
func auditSignatureCanonicalV4(ev *domain.AuditEvent) string {
	return lengthDelimitedAuditCanonical("audit:v4\n", []string{
		ev.ID,
		ev.ProjectID,
		ev.ActorID,
		ev.ActorType,
		ev.Action,
		ev.ResourceType,
		ev.ResourceID,
		string(ev.Details),
		ev.CreatedAt.UTC().Format(time.RFC3339Nano),
		ev.PreviousHash,
		ev.RemoteIP,
		ev.UserAgent,
		ev.RequestID,
		ev.TraceID,
		strconv.FormatUint(uint64(ev.SchemaVersion), 10),
		strconv.FormatBool(ev.IsAnchor),
		strconv.Itoa(ev.RotationEpoch),
		ev.ShardID,
	})
}

func lengthDelimitedAuditCanonical(prefix string, fields []string) string {
	size := len(prefix)
	for _, field := range fields {
		size += decimalDigitCount(len(field)) + len(field) + len(":\n")
	}
	var b strings.Builder
	b.Grow(size)
	b.WriteString(prefix)
	var lenBuf [20]byte
	for _, field := range fields {
		b.Write(strconv.AppendInt(lenBuf[:0], int64(len(field)), 10))
		b.WriteByte(':')
		b.WriteString(field)
		b.WriteByte('\n')
	}
	return b.String()
}

func decimalDigitCount(n int) int {
	digits := 1
	for n >= 10 {
		n /= 10
		digits++
	}
	return digits
}

func (q *Queries) CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAuditEvent")
	defer span.End()

	if ev == nil {
		return fmt.Errorf("create audit event: event is nil")
	}

	working := *ev
	if working.ID == "" {
		working.ID = uuid.Must(uuid.NewV7()).String()
	}

	details := working.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	working.Details = details

	// Default schema version for new events.
	if working.SchemaVersion == 0 {
		working.SchemaVersion = domain.AuditEventSchemaVersionCurrent
	}
	// Auto-derive shard_id from resource_type for normal events. The chain
	// identity is (project_id, shard_id); routing each resource type into
	// its own sub-chain caps per-shard write contention to that resource's
	// write rate and lets independent resources verify in parallel. Anchor
	// rows (key rotation, retention tombstones) carry their own explicit
	// shard_id — either '' for the legacy chain or the affected shard for
	// per-shard anchors — so we never overwrite it. Callers that need the
	// legacy unsharded chain must clear ShardID explicitly and pass an
	// anchor; ordinary writers do not opt out.
	if !working.IsAnchor && working.ShardID == "" && working.ResourceType != "" {
		working.ShardID = working.ResourceType
	}
	// Shard-aware writes require at least v4 because the canonical form
	// binds ShardID. Force the bump so a caller cannot accidentally chain
	// a shard row under a v3 signature that omits the shard binding —
	// that would allow flipping shard_id without invalidating the HMAC.
	if working.ShardID != "" && working.SchemaVersion < 4 {
		working.SchemaVersion = 4
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
	err := q.withTxInheritKeys(ctx, func(tx *Queries) error {
		if err := acquireProjectRotationLock(ctx, tx, working.ProjectID); err != nil {
			return fmt.Errorf("create audit event: rotation lock: %w", err)
		}

		// Chain lock. Legacy (empty shard_id) writes serialize under
		// AdvisoryLockNsAuditChain (per-project) so the pre-shard lock
		// domain is unchanged for callers that do not set ev.ShardID.
		// Non-empty shards serialize under AdvisoryLockNsAuditChainShard
		// with key `<projectID>:<shardID>` so unrelated shards in the
		// same project do not contend on a single advisory key.
		//
		// Both namespaces are distinct from AdvisoryLockNsAuditRotate so
		// rotation + retention writes serialize against each other (and
		// against chain inserts via acquireProjectRotationLock above)
		// without forcing two unrelated shards in the same project to
		// queue on the same chain lock. The lock is transaction-scoped
		// via pg_advisory_xact_lock.
		chainLockNs := AdvisoryLockNsAuditChain
		chainLockKey := working.ProjectID
		if working.ShardID != "" {
			chainLockNs = AdvisoryLockNsAuditChainShard
			chainLockKey = working.ProjectID + ":" + working.ShardID
		}
		if err := AcquireAdvisoryLock(ctx, tx, chainLockNs, chainLockKey); err != nil {
			return fmt.Errorf("create audit event: chain lock: %w", err)
		}

		// Assign the timestamp under the chain lock so chronological verifier
		// order matches serialized insertion order even when concurrent writers
		// waited on the same project lock.
		working.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)

		// Read the tail signature under the lock so no concurrent writer
		// can slip a row between this read and our insert. The shard_id
		// filter scopes the chain tail to the row's own sub-chain:
		// legacy writers never chain from a sharded row, and shard
		// writers never chain from a legacy row. Combined with the v4
		// HMAC binding of shard_id this makes shards cryptographically
		// independent sub-chains within a single project.
		var prevHash string
		if working.RotationEpoch == 0 && !working.IsAnchor {
			if err := tx.db.QueryRow(ctx, `
				SELECT
					COALESCE(MAX(rotation_epoch), 0) AS rotation_epoch,
					COALESCE(
						(SELECT signature FROM audit_events
						 WHERE project_id = $1 AND shard_id = $2 AND signature != ''
						 ORDER BY rotation_epoch DESC, created_at DESC, id DESC LIMIT 1),
						$3
					) AS previous_hash
				FROM audit_events
				WHERE project_id = $1
			`, working.ProjectID, working.ShardID, ZeroHash).Scan(&working.RotationEpoch, &prevHash); err != nil {
				return fmt.Errorf("create audit event: read epoch and prev hash: %w", err)
			}
		} else {
			if err := tx.db.QueryRow(ctx, `
				SELECT COALESCE(
				    (SELECT signature FROM audit_events
				     WHERE project_id = $1 AND shard_id = $2 AND signature != ''
				     ORDER BY rotation_epoch DESC, created_at DESC, id DESC LIMIT 1),
				    $3
				)
			`, working.ProjectID, working.ShardID, ZeroHash).Scan(&prevHash); err != nil {
				return fmt.Errorf("create audit event: read prev hash: %w", err)
			}
		}
		working.PreviousHash = prevHash

		// INSERT the row with an empty signature and RETURNING details so
		// the canonical (JSONB-normalized) bytes — the exact form the
		// verifier will later SELECT — feed the HMAC computation. Postgres
		// rewrites JSONB text on storage (whitespace normalization, key
		// reorder) so any signature computed against the raw input bytes
		// would not match the bytes the verifier reads back. Round-tripping
		// through RETURNING closes the gap.
		//
		// The empty-signature INSERT is NOT observable outside this tx: the
		// containing withTxInheritKeys runs INSERT + UPDATE in a single
		// transaction that commits together. A crash or cancellation between
		// the two statements rolls the tx back and leaves no row behind, so
		// every committed row still carries a non-empty signature by
		// construction.
		//
		// The tail-read query above also filters "signature != ''" so an
		// in-flight sibling that sees a partial state would never chain
		// from an unsigned row anyway; combined with the tx atomicity this
		// preserves the chain invariant both intra- and inter-tx.
		if err := tx.db.QueryRow(ctx, `
			INSERT INTO audit_events (
				id, project_id, actor_id, actor_type, action, resource_type, resource_id,
				details, signature, previous_hash, created_at,
				remote_ip, user_agent, request_id, trace_id, schema_version,
				is_anchor, rotation_epoch, shard_id
			)
			VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8::jsonb, '', $9, $10,
				$11, $12, $13, $14, $15,
				$16, $17, $18
			)
			RETURNING details
		`,
			working.ID, working.ProjectID, working.ActorID, working.ActorType, working.Action, working.ResourceType, working.ResourceID,
			details, working.PreviousHash, working.CreatedAt,
			working.RemoteIP, working.UserAgent, working.RequestID, working.TraceID, working.SchemaVersion,
			working.IsAnchor, working.RotationEpoch, working.ShardID,
		).Scan(&working.Details); err != nil {
			return fmt.Errorf("create audit event: insert: %w", err)
		}

		// Compute and persist the HMAC signature now that working.Details holds
		// the canonical bytes. When no signing key is configured — legacy
		// unit-test / bootstrap installs — leave the empty sentinel;
		// VerifyAuditChain gates on q.auditSigningKey != nil and is never
		// called in that mode.
		if tx.auditSigningKey != nil {
			signingKey, err := tx.resolveSigningKeyForEpoch(ctx, working.ProjectID, working.RotationEpoch)
			if err != nil {
				return err
			}
			working.Signature = ComputeAuditSignature(&working, signingKey)
			if _, err := tx.db.Exec(ctx, `
				UPDATE audit_events SET signature = $1 WHERE id = $2
			`, working.Signature, working.ID); err != nil {
				return fmt.Errorf("create audit event: update signature: %w", err)
			}
		}

		if hook := tx.auditEventPostInsertHook; hook != nil {
			if err := hook(ctx); err != nil {
				return fmt.Errorf("create audit event: post-insert hook: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	*ev = working
	return nil
}

// resolveSigningKeyForEpoch returns the per-epoch HMAC signing key used for
// both signing and verification. When secretEncryptionKey is configured, the
// key is looked up in audit_signing_keys and, on first write for an epoch,
// bootstrapped from q.auditSigningKey so signer and verifier converge on a
// stable per-epoch key independent of in-memory mutations. When no encryption
// key is configured (unit-test / pre-rotation installs), the in-memory
// q.auditSigningKey is used directly — matching the legacy behavior.
//
// NOTE: This method MUST be called from within a transaction (via
// withTxInheritKeys in CreateAuditEvent) so the bootstrap INSERT and
// subsequent re-read are atomic with the chain insert. q.db is the tx
// handle in that context.
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
	if epoch != 0 {
		return nil, fmt.Errorf("resolve signing key: no stored key for project %s epoch %d", projectID, epoch)
	}
	// Bootstrap: derive a per-(project, epoch) HMAC key from INTERNAL_SECRET
	// and persist it as the stable per-epoch key. This guarantees that every
	// project receives a cryptographically independent signing key even for
	// the pre-rotation epoch — the global q.auditSigningKey is identical
	// across projects and must not be used as the per-epoch key material.
	// The global key remains only as the legacy fallback in VerifyAuditChain
	// for chains written before per-epoch keys existed (epoch 0 with no row).
	// Races are resolved by ON CONFLICT DO NOTHING followed by a re-read.
	derivedKey, err := DeriveAuditSigningKeyForEpochFromRoot(q.auditSigningKey, projectID, epoch)
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
		// Hard fail: we just inserted ON CONFLICT DO NOTHING and the
		// subsequent read returned nothing. That is only possible if the
		// row was deleted out from under us (impossible inside a tx) or
		// the insert silently succeeded but the read filter is wrong.
		// Falling back to the global q.auditSigningKey would have the
		// signer use a key that the verifier will never look up — future
		// VerifyAuditChain calls would then fail signature comparison on
		// every row we sign here. Refuse instead.
		return nil, fmt.Errorf("resolve signing key: bootstrap inserted for project %s epoch %d but subsequent read returned nothing; refusing to sign under global key to avoid signer/verifier key divergence", projectID, epoch)
	}
	return stored, nil
}

func (q *Queries) ListAuditEvents(ctx context.Context, projectID, actorID, resourceType, resourceID string, limit int, cursor, from, to *time.Time, ascending bool) ([]domain.AuditEvent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditEvents")
	defer span.End()

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch, shard_id
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
	query += fmt.Sprintf(" ORDER BY created_at %s, id %s LIMIT $%d", order, order, param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.AuditEvent, 0, limit)
	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch, &ev.ShardID); err != nil {
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
		       is_anchor, rotation_epoch, shard_id
		FROM audit_events
		WHERE id = $1 AND project_id = $2`

	var ev domain.AuditEvent
	err := q.db.QueryRow(ctx, query, id, projectID).Scan(
		&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType,
		&ev.Action, &ev.ResourceType, &ev.ResourceID,
		&ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt,
		&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID,
		&ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch, &ev.ShardID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAuditEventNotFound
		}
		return nil, fmt.Errorf("get audit event: %w", err)
	}
	return &ev, nil
}

// StreamAuditEvents streams audit events matching the given filters to fn.
// There is no SQL LIMIT clause in this query — the caller is responsible for
// capping the number of rows consumed via the fn callback (e.g. by returning
// errExportCapReached after the desired row count). The export handler in
// api/audit_export.go enforces a per-project row cap through this mechanism.
func (q *Queries) StreamAuditEvents(ctx context.Context, projectID, actorID, resourceType string, from, to time.Time, fn func(*domain.AuditEvent) error) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.StreamAuditEvents")
	defer span.End()

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch, shard_id
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
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch, &ev.ShardID); err != nil {
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
	_, ok := q.db.(TxBeginner)
	if !ok {
		return fn(q)
	}
	return q.withTx(ctx, fn)
}

// acquireProjectRotationLock takes a per-project transaction-scoped advisory
// lock under AdvisoryLockNsAuditRotate. Both RotateAuditSigningKey and
// writeRetentionTombstone call this so a tombstone insert cannot race with
// an in-progress rotation and capture the wrong rotation_epoch.
//
// Without this lock, the tombstone path could:
//   - read MAX(rotation_epoch) = N
//   - rotation commits epoch N+1 with a new key
//   - tombstone insert proceeds, but signs the row under epoch N's key
//     while CreateAuditEvent's chain-tail read returns the rotation
//     anchor (epoch N+1). The chain becomes unverifiable.
//
// The lock is transaction-scoped: it auto-releases on COMMIT or ROLLBACK
// and serializes only against other holders of the same advisory key.
func acquireProjectRotationLock(ctx context.Context, q *Queries, projectID string) error {
	if err := AcquireAdvisoryLock(ctx, q, AdvisoryLockNsAuditRotate, projectID); err != nil {
		return fmt.Errorf("acquire project rotation lock: %w", err)
	}
	return nil
}

// writeRetentionTombstone inserts a tombstone anchor row for a (project,
// shard) sub-chain that just had rows trimmed. It is called inside the same
// transaction as the DELETE so the tombstone and the trim commit (or roll
// back) together. The tombstone is pinned to shardID so it lives in the same
// sub-chain as the rows it justifies; a tombstone in shard A does not
// justify a non-zero chain start in shard B.
//
// The tombstone's previous_hash is the most recent surviving row's signature
// in the same shard (or ZeroHash if the trim removed every row in the
// shard). Its details carry {deleted_count, trimmed_before, previous_hash}.
// The row is inserted via CreateAuditEvent so it obtains a real HMAC
// signature chained from the surviving tail — VerifyAuditChain then
// naturally accepts it as a chain-valid forensic marker per shard.
//
// Serialization: takes the same per-project rotation advisory lock as
// RotateAuditSigningKey so an in-progress rotation cannot interleave between
// the rotation_epoch read and the chain insert. The rotation_epoch and
// surviving-head lookups run AFTER the lock is acquired so they reflect
// the latest committed state under the lock (the enclosing tx uses the
// default READ COMMITTED isolation so each statement re-reads the latest
// committed snapshot — REPEATABLE READ would pin the snapshot before this
// helper runs and re-introduce the rotation-staleness race).
func (q *Queries) writeRetentionTombstone(ctx context.Context, projectID, shardID string, cutoff time.Time, deleted int64) error {
	if err := acquireProjectRotationLock(ctx, q, projectID); err != nil {
		return fmt.Errorf("tombstone: %w", err)
	}

	// Single round-trip: rotation epoch (project-scoped), tail signature
	// (shard-scoped, signed rows only), first surviving id and chain_start
	// (shard-scoped, any signature state). One SELECT avoids three serial
	// roundtrips per tombstone, which compounds across shards in a busy
	// retention run.
	var (
		rotationEpoch    int
		prevHash         string
		firstSurvivingID string
		chainStart       string
	)
	if err := q.db.QueryRow(ctx, `
		SELECT
			COALESCE(
				(SELECT MAX(rotation_epoch) FROM audit_events WHERE project_id = $1),
				0
			) AS rotation_epoch,
			COALESCE(
				(SELECT signature FROM audit_events
				 WHERE project_id = $1 AND shard_id = $2 AND signature != ''
				 ORDER BY rotation_epoch DESC, created_at DESC, id DESC LIMIT 1),
				$3
			) AS tail_sig,
			COALESCE(
				(SELECT id FROM audit_events
				 WHERE project_id = $1 AND shard_id = $2
				 ORDER BY rotation_epoch ASC, created_at ASC, id ASC LIMIT 1),
				''
			) AS first_surviving_id,
			COALESCE(
				(SELECT previous_hash FROM audit_events
				 WHERE project_id = $1 AND shard_id = $2
				 ORDER BY rotation_epoch ASC, created_at ASC, id ASC LIMIT 1),
				$3
			) AS chain_start
	`, projectID, shardID, ZeroHash).Scan(&rotationEpoch, &prevHash, &firstSurvivingID, &chainStart); err != nil {
		return fmt.Errorf("tombstone: read tombstone context: %w", err)
	}

	// json.Marshal of map[string]any sorts keys alphabetically. The exact
	// bytes signed by ComputeAuditSignature come from RETURNING details in
	// CreateAuditEvent (Postgres JSONB normalizes whitespace and key order
	// on storage), so map iteration order here does not affect HMAC
	// stability — but a stable input still makes diff review easier.
	details, err := json.Marshal(map[string]any{
		"chain_start":              chainStart,
		"deleted_count":            deleted,
		"first_surviving_event_id": firstSurvivingID,
		"shard_id":                 shardID,
		"surviving_tail_signature": prevHash,
		"trimmed_before":           cutoff.UTC().Format(time.RFC3339),
		"previous_hash":            prevHash,
	})
	if err != nil {
		return fmt.Errorf("tombstone: marshal details: %w", err)
	}

	resourceID := fmt.Sprintf("retention-%s", cutoff.UTC().Format(time.RFC3339))
	if shardID != "" {
		resourceID = fmt.Sprintf("retention-%s-%s", shardID, cutoff.UTC().Format(time.RFC3339))
	}
	ev := &domain.AuditEvent{
		ProjectID:     projectID,
		ShardID:       shardID,
		ActorID:       "system",
		ActorType:     "system",
		Action:        domain.AuditActionRetentionTrimmed,
		ResourceType:  "audit_events",
		ResourceID:    resourceID,
		Details:       json.RawMessage(details),
		IsAnchor:      true,
		RotationEpoch: rotationEpoch,
	}
	// Fail closed if a future edit drops IsAnchor: the auto-derivation in
	// CreateAuditEvent would otherwise route the tombstone into a shard
	// literally named "audit_events" (its resource_type), orphaning it
	// from the shard whose deletion it is supposed to justify.
	if !ev.IsAnchor {
		return fmt.Errorf("tombstone: refusing to emit non-anchor retention row (project %s, shard %q)", projectID, shardID)
	}
	if hook := q.tombstoneInsertHook; hook != nil {
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

	// Reject the empty-projectID shortcut explicitly. Historically an
	// empty projectID silently fell through to a cross-tenant DELETE,
	// which defeats per-tenant isolation and can wipe unrelated
	// projects on a buggy call site. Callers that need a cross-tenant
	// trim must use DeleteAuditEventsBeforeExcluding — it emits one
	// retention tombstone per affected project inside a single
	// transaction.
	if projectID == "" {
		return 0, fmt.Errorf("DeleteAuditEventsBefore: projectID required; use DeleteAuditEventsBeforeExcluding for cross-tenant trim")
	}

	var deleted int64
	err := q.withTxInheritKeys(ctx, func(tx *Queries) error {
		// Enumerate affected shards before the DELETE so we can emit one
		// tombstone per shard. Per-(project, shard) chains verify
		// independently, so a shard with no rows trimmed must not receive a
		// spurious anchor — the affected set is exactly the shards with at
		// least one row < cutoff.
		//
		// READ COMMITTED is intentional: the tombstone's MAX(rotation_epoch)
		// read happens under acquireProjectRotationLock inside
		// writeRetentionTombstone, so it always reflects the latest
		// committed state. REPEATABLE READ would pin the tx snapshot at the
		// first SELECT, letting a rotation commit between this enumeration
		// and the tombstone insert leave the tombstone bound to a stale
		// rotation_epoch — VerifyAuditChain's (rotation_epoch ASC, ...)
		// ordering would then place the tombstone before the rotation
		// anchor and break the chain.
		//
		// A concurrent insert into a previously-empty shard simply lands
		// AFTER the cutoff (NOW() > cutoff) and is never part of the trim
		// set, so RC does not produce orphan deletions either.
		rows, qErr := tx.db.Query(ctx, `
			SELECT DISTINCT shard_id
			FROM audit_events
			WHERE project_id = $1 AND created_at < $2
		`, projectID, cutoff)
		if qErr != nil {
			return fmt.Errorf("discover affected shards: %w", qErr)
		}
		var shards []string
		for rows.Next() {
			var sid string
			if scanErr := rows.Scan(&sid); scanErr != nil {
				rows.Close()
				return fmt.Errorf("scan affected shard: %w", scanErr)
			}
			shards = append(shards, sid)
		}
		rows.Close()
		if rowsErr := rows.Err(); rowsErr != nil {
			return fmt.Errorf("rows err: %w", rowsErr)
		}

		for _, sid := range shards {
			tag, execE := tx.db.Exec(ctx, `
				DELETE FROM audit_events
				WHERE project_id = $1 AND shard_id = $2 AND created_at < $3
			`, projectID, sid, cutoff)
			if execE != nil {
				return fmt.Errorf("delete audit events before (shard %q): %w", sid, execE)
			}
			n := tag.RowsAffected()
			if n == 0 {
				continue
			}
			deleted += n
			if err := tx.writeRetentionTombstone(ctx, projectID, sid, cutoff, n); err != nil {
				return err
			}
		}
		return nil
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
// Emits one tombstone anchor row per affected (project, shard) pair. Per
// project the trim + tombstones run in their own transaction by calling
// DeleteAuditEventsBefore. Chunking per project bounds the largest
// transaction's footprint (rows held, advisory locks held, WAL volume) to a
// single tenant's retention window — a fleet-wide reaper sweep no longer
// produces one mega-tx that holds every project's chain lock simultaneously
// and risks long autovacuum delays / replication lag.
//
// The cross-project consistency story: each project's trim is atomic with
// its own tombstones (DeleteAuditEventsBefore wraps a single tx), and the
// enumeration is taken in autocommit. A project that grows a row < cutoff
// after enumeration but before we reach it will be skipped this tick and
// picked up on the next reaper run — identical to the per-project
// DeleteAuditEventsBefore contract.
func (q *Queries) DeleteAuditEventsBeforeExcluding(ctx context.Context, cutoff time.Time, excludeProjectIDs []string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditEventsBeforeExcluding")
	defer span.End()

	// Enumerate distinct projects in autocommit. Reading without a tx
	// means we do not hold any advisory locks or write locks while the
	// per-project trims run, so unrelated tenants are never blocked by
	// a long enumeration.
	var (
		rows pgx.Rows
		e    error
	)
	if len(excludeProjectIDs) == 0 {
		rows, e = q.db.Query(ctx, `
			SELECT DISTINCT project_id
			FROM audit_events
			WHERE created_at < $1
		`, cutoff)
	} else {
		rows, e = q.db.Query(ctx, `
			SELECT DISTINCT project_id
			FROM audit_events
			WHERE created_at < $1 AND project_id <> ALL($2::text[])
		`, cutoff, excludeProjectIDs)
	}
	if e != nil {
		return 0, fmt.Errorf("discover affected projects: %w", e)
	}
	var projects []string
	for rows.Next() {
		var p string
		if scanErr := rows.Scan(&p); scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("scan affected project: %w", scanErr)
		}
		projects = append(projects, p)
	}
	rows.Close()
	if rowsErr := rows.Err(); rowsErr != nil {
		return 0, fmt.Errorf("rows err: %w", rowsErr)
	}

	var total int64
	for _, p := range projects {
		n, err := q.DeleteAuditEventsBefore(ctx, p, cutoff)
		if err != nil {
			return total, fmt.Errorf("delete audit events before (project %s): %w", p, err)
		}
		total += n
	}
	return total, nil
}

// VerifyAuditChain replays the audit event chain for a project in chronological
// order and verifies that each event's HMAC signature is valid and that the
// previous_hash linkage is unbroken.
//
// Sharded chains: rows are grouped by shard_id and each shard is verified as
// an independent sub-chain. The (project_id, shard_id) tuple is the chain
// identity — within a shard, previous_hash must form an unbroken cryptographic
// linkage starting from either ZeroHash or a retention tombstone that justifies
// the non-zero start; across shards there is no linkage. Legacy rows (empty
// shard_id) form their own sub-chain and verify identically to the pre-shard
// behavior.
func (q *Queries) VerifyAuditChain(ctx context.Context, projectID string) (*domain.AuditChainVerification, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.VerifyAuditChain")
	defer span.End()

	if q.auditSigningKey == nil {
		return nil, fmt.Errorf("audit signing key is not configured")
	}

	// Ordering: shard_id ASC first so each shard's rows are contiguous in
	// the cursor, then rotation_epoch ASC and created_at ASC within a
	// shard. Anchor rows are the first row of their new epoch by
	// construction (they are inserted under the newly-bumped epoch before
	// any other post-rotation writes). Within an epoch, created_at
	// preserves insertion order as the chain serialization is HMAC-bound
	// to previous_hash.
	//
	// Per-epoch keys: rows are grouped by rotation_epoch and verified
	// under the key stored in audit_signing_keys for that (project, epoch).
	// Epoch 0 has no stored key — we fall back to the configured global
	// q.auditSigningKey for backwards compatibility with chains that
	// predate per-epoch key derivation. Anchor rows must verify under
	// their OWN epoch's (new) key, not the previous epoch's, because
	// post-rotation events chain from the anchor and verify under the
	// same new key. Signing keys remain per-(project, epoch) — all shards
	// in a project share the same key material.
	epochKeyCache, err := q.preloadEpochKeys(ctx, projectID, "")
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch, shard_id
		FROM audit_events
		WHERE project_id = $1
		ORDER BY shard_id ASC, rotation_epoch ASC, created_at ASC, id ASC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("verify audit chain: %w", err)
	}
	defer rows.Close()

	result := &domain.AuditChainVerification{
		ProjectID: projectID,
		Valid:     true,
	}

	var (
		shardSeen               bool
		currentShard            string
		shardExpectedPrevHash   string
		shardFirstEventID       string
		shardChainStart         string
		shardTombstoneJustifies bool
	)

	// closeShard runs end-of-shard validation: a non-zero chain start must
	// be justified by a retention tombstone within the same shard. Each
	// shard is its own sub-chain so a tombstone in one shard does not
	// justify the start of another.
	closeShard := func() {
		if shardFirstEventID == "" {
			return
		}
		if shardChainStart != ZeroHash && !shardTombstoneJustifies && result.Valid {
			result.Valid = false
			result.BrokenAtID = shardFirstEventID
			result.Error = fmt.Sprintf("chain starts from non-zero previous_hash %s without a matching signed retention tombstone", shardChainStart)
		}
	}

	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch, &ev.ShardID); err != nil {
			return nil, fmt.Errorf("verify audit chain scan: %w", err)
		}

		if !shardSeen || ev.ShardID != currentShard {
			// Boundary: close the previous shard and reset chain state
			// for the new shard. expectedPrevHash resets so cross-shard
			// linkage is never required (or accepted).
			if shardSeen {
				closeShard()
				if !result.Valid {
					return result, nil
				}
			}
			shardSeen = true
			currentShard = ev.ShardID
			shardExpectedPrevHash = ev.PreviousHash
			shardFirstEventID = ev.ID
			shardChainStart = ev.PreviousHash
			shardTombstoneJustifies = false
		}

		result.EventsChecked++
		if result.FirstEventID == "" {
			result.FirstEventID = ev.ID
			result.ChainStart = ev.PreviousHash
		}
		result.LastEventID = ev.ID

		if ev.Action == domain.AuditActionRetentionTrimmed && ev.IsAnchor {
			shardTombstoneJustifies = shardTombstoneJustifies ||
				auditRetentionTombstoneJustifiesStart(ev, shardFirstEventID, shardChainStart)
		}

		if ev.PreviousHash != shardExpectedPrevHash {
			result.Valid = false
			result.BrokenAtID = ev.ID
			result.Error = fmt.Sprintf("chain broken at event %s: previous_hash mismatch (expected %s, got %s)", ev.ID, shardExpectedPrevHash, ev.PreviousHash)
			return result, nil
		}

		key, keyErr := q.keyForEpoch(epochKeyCache, ev.RotationEpoch)
		if keyErr != nil {
			return nil, keyErr
		}
		if !q.auditSignatureMatchesEpoch(&ev, key) {
			result.Valid = false
			result.BrokenAtID = ev.ID
			result.Error = fmt.Sprintf("signature mismatch at event %s: event may have been tampered with", ev.ID)
			return result, nil
		}

		shardExpectedPrevHash = ev.Signature
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("verify audit chain rows: %w", err)
	}
	if shardSeen {
		closeShard()
	}

	return result, nil
}

func (q *Queries) auditSignatureMatchesEpoch(ev *domain.AuditEvent, key []byte) bool {
	expected := ComputeAuditSignature(ev, key)
	// Constant-time comparison to avoid leaking the HMAC digest via a
	// byte-wise early-return timing side channel.
	if hmac.Equal([]byte(ev.Signature), []byte(expected)) {
		return true
	}
	if ev.RotationEpoch != 0 || q.auditSigningKey == nil {
		return false
	}
	// Legacy epoch-0 rows created before per-project audit_signing_keys
	// existed were signed with q.auditSigningKey. A later bootstrap row for
	// epoch 0 must not invalidate those historical rows; mixed epoch-0 chains
	// can contain legacy rows followed by newly bootstrapped per-project rows.
	legacyExpected := ComputeAuditSignature(ev, q.auditSigningKey)
	return hmac.Equal([]byte(ev.Signature), []byte(legacyExpected))
}

func auditRetentionTombstoneJustifiesStart(ev domain.AuditEvent, firstEventID, chainStart string) bool {
	if firstEventID == "" || chainStart == "" {
		return false
	}
	var details struct {
		ChainStart            string `json:"chain_start"`
		FirstSurvivingEventID string `json:"first_surviving_event_id"`
		ShardID               string `json:"shard_id"`
	}
	if err := json.Unmarshal(ev.Details, &details); err != nil {
		return false
	}
	// Cross-check the tombstone's recorded shard_id against the row's
	// own shard_id column. Both are HMAC-bound (details via RETURNING,
	// shard_id via v4 canonical), so a mismatch means the row was
	// hand-crafted or replayed under the wrong shard — refuse to let
	// such a row justify a non-zero chain start.
	if details.ShardID != ev.ShardID {
		return false
	}
	return details.ChainStart == chainStart && details.FirstSurvivingEventID == firstEventID
}

// preloadEpochKeys fetches all distinct rotation epochs for a project
// and pre-loads their signing keys. This must be called before opening a
// rows cursor on the same connection: pgx doesn't support concurrent
// operations on a single connection, and the RLS transaction middleware
// pins all queries to one tx.
func (q *Queries) preloadEpochKeys(ctx context.Context, projectID string, extraFilter string, extraArgs ...any) (map[int][]byte, error) {
	query := `SELECT DISTINCT rotation_epoch FROM audit_events WHERE project_id = $1`
	args := []any{projectID}
	if extraFilter != "" {
		query += " AND (" + extraFilter + ")"
		args = append(args, extraArgs...)
	}
	epochRows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("preload epoch keys: %w", err)
	}
	var epochs []int
	for epochRows.Next() {
		var e int
		if scanErr := epochRows.Scan(&e); scanErr != nil {
			epochRows.Close()
			return nil, fmt.Errorf("preload epoch keys scan: %w", scanErr)
		}
		epochs = append(epochs, e)
	}
	epochRows.Close()

	cache := make(map[int][]byte, len(epochs))
	for _, epoch := range epochs {
		stored, keyErr := q.GetAuditSigningKey(ctx, projectID, epoch)
		if keyErr != nil {
			return nil, keyErr
		}
		cache[epoch] = stored
	}
	return cache, nil
}

func (q *Queries) keyForEpoch(cache map[int][]byte, epoch int) ([]byte, error) {
	if k, ok := cache[epoch]; ok {
		if k != nil {
			return k, nil
		}
		if epoch != 0 {
			return nil, fmt.Errorf("verify audit chain: no stored key for epoch %d", epoch)
		}
		return q.auditSigningKey, nil
	}
	if epoch != 0 {
		return nil, fmt.Errorf("verify audit chain: no stored key for epoch %d", epoch)
	}
	return q.auditSigningKey, nil
}

// AuditEventsDMLRestricted reports whether the current database role lacks
// destructive DML privileges on audit_events. When migration 000187 has taken
// effect (the strait_app role exists and is the current role), the REVOKE
// UPDATE/DELETE sequence and column-scoped UPDATE(signature) grant leave
// has_table_privilege returning false for unqualified UPDATE and DELETE checks.
// We also require TRUNCATE to be unavailable and reject column-level UPDATE
// privileges on every column except signature. When the current role is a
// superuser or the migration was skipped (strait_app not provisioned), one of
// these checks returns true and we report restricted = false, letting the DML
// guard probe surface a degraded state.
//
// The table-level UPDATE check is deliberately unqualified and the
// has_column_privilege scan is deliberately scoped to non-signature columns:
// the tamper-evident path requires every audit field except signature to be
// read-only to the application role.
func (q *Queries) AuditEventsDMLRestricted(ctx context.Context) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AuditEventsDMLRestricted")
	defer span.End()

	var hasUpdate, hasDelete, hasTruncate, hasUnsafeColumnUpdate bool
	if err := q.db.QueryRow(ctx, `
		SELECT
			has_table_privilege(current_user, 'audit_events', 'UPDATE'),
			has_table_privilege(current_user, 'audit_events', 'DELETE'),
			has_table_privilege(current_user, 'audit_events', 'TRUNCATE'),
			EXISTS (
				SELECT 1
				FROM pg_attribute
				WHERE attrelid = 'audit_events'::regclass
				  AND attnum > 0
				  AND NOT attisdropped
				  AND attname != 'signature'
				  AND has_column_privilege(current_user, 'audit_events', attname, 'UPDATE')
			)
	`).Scan(&hasUpdate, &hasDelete, &hasTruncate, &hasUnsafeColumnUpdate); err != nil {
		return false, fmt.Errorf("audit dml privilege check: %w", err)
	}
	return !hasUpdate && !hasDelete && !hasTruncate && !hasUnsafeColumnUpdate, nil
}
