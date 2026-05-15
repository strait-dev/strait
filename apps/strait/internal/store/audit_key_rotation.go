package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	mrand "math/rand/v2"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"golang.org/x/crypto/hkdf"
)

// DeriveAuditSigningKeyForEpoch derives a 32-byte HMAC signing key for a
// specific (project, epoch) pair from the internal secret using HKDF-SHA256.
// The epoch and project are mixed into the HKDF info parameter so every
// rotation produces a cryptographically independent key.
//
// The info parameter is deliberately a colon-separated literal rather than
// a structured encoding: both signer (resolveSigningKeyForEpoch bootstrap,
// RotateAuditSigningKey) and verifier (indirectly, via the stored key
// re-read) must derive identical bytes, and the simplest format that has
// shipped in prior releases is a stable choice. Project ids are UUIDs and
// cannot contain literal colons, so the historical collision concern does
// not apply at runtime.
func DeriveAuditSigningKeyForEpoch(secret, projectID string, epoch int) ([]byte, error) {
	if secret == "" {
		return nil, fmt.Errorf("audit signing key: secret is empty")
	}
	root, err := DeriveAuditSigningKey(secret)
	if err != nil {
		return nil, err
	}
	return DeriveAuditSigningKeyForEpochFromRoot(root, projectID, epoch)
}

func DeriveAuditSigningKeyForEpochFromRoot(root []byte, projectID string, epoch int) ([]byte, error) {
	if len(root) == 0 {
		return nil, fmt.Errorf("audit signing key: root key is empty")
	}
	info := fmt.Appendf(nil, "audit-event-signing:epoch:%d:project:%s", epoch, projectID)
	reader := hkdf.New(sha256.New, root, info, nil)
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("audit signing key: hkdf derive: %w", err)
	}
	return key, nil
}

// GetAuditSigningKey decrypts and returns the per-epoch HMAC signing key
// for (projectID, epoch). Returns (nil, nil) when no row exists — the
// caller is expected to fall back to the global signing key for the
// pre-rotation epoch (epoch 0) to preserve backwards compatibility with
// installations that existed before per-epoch keys.
func (q *Queries) GetAuditSigningKey(ctx context.Context, projectID string, epoch int) ([]byte, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAuditSigningKey")
	defer span.End()

	var ciphertext []byte
	err := q.db.QueryRow(ctx, `
		SELECT key_material
		FROM audit_signing_keys
		WHERE project_id = $1 AND rotation_epoch = $2
	`, projectID, epoch).Scan(&ciphertext)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get audit signing key: %w", err)
	}

	var firstErr error
	for i, candidate := range q.secretEncryptionKeyCandidates() {
		envelopeKey, keyErr := deriveSecretKey(candidate)
		if keyErr != nil {
			if firstErr == nil {
				firstErr = keyErr
			}
			continue
		}
		plaintextHex, decryptErr := decryptAuditKey(ciphertext, envelopeKey)
		if decryptErr == nil {
			if i > 0 {
				slog.Warn("decrypted audit signing key using old encryption key; rotate audit key to re-encrypt", "project_id", projectID, "epoch", epoch)
			}
			return plaintextHex, nil
		}
		if firstErr == nil {
			firstErr = decryptErr
		}
	}
	if firstErr != nil {
		return nil, fmt.Errorf("get audit signing key: decrypt: %w", firstErr)
	}
	return nil, fmt.Errorf("get audit signing key: envelope key: secret encryption key is not configured")
}

// storeAuditSigningKey encrypts and inserts the per-epoch HMAC signing
// key. Must be called inside the same transaction as the anchor insert
// so the key and anchor commit together. actorID is persisted on the
// row so the forensic trail of who triggered the rotation survives even
// if the corresponding audit.key_rotated chain event is lost.
func (q *Queries) storeAuditSigningKey(ctx context.Context, projectID string, epoch int, key []byte, actorID string) error {
	envelopeKey, err := q.secretKey()
	if err != nil {
		return fmt.Errorf("store audit signing key: envelope key: %w", err)
	}
	ciphertext, err := encryptAuditKey(key, envelopeKey)
	if err != nil {
		return fmt.Errorf("store audit signing key: encrypt: %w", err)
	}
	if _, err := q.db.Exec(ctx, `
		INSERT INTO audit_signing_keys (project_id, rotation_epoch, key_material, created_by)
		VALUES ($1, $2, $3, NULLIF($4, ''))
	`, projectID, epoch, ciphertext, actorID); err != nil {
		return fmt.Errorf("store audit signing key: insert: %w", err)
	}
	return nil
}

// encryptAuditKey seals the 32-byte HMAC signing key with AES-256-GCM
// using the HKDF-derived envelope key. Output layout is nonce || ciphertext,
// matching the on-disk format used for job_secrets.
func encryptAuditKey(plaintext, envelopeKey []byte) ([]byte, error) {
	if len(envelopeKey) != 32 {
		return nil, fmt.Errorf("encrypt audit key: envelope key must be 32 bytes, got %d", len(envelopeKey))
	}
	block, err := aes.NewCipher(envelopeKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt audit key: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encrypt audit key: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("encrypt audit key: nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

func decryptAuditKey(ciphertext, envelopeKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(envelopeKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt audit key: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("decrypt audit key: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("decrypt audit key: ciphertext too short")
	}
	nonce := ciphertext[:nonceSize]
	encrypted := ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt audit key: %w", err)
	}
	return plaintext, nil
}

// RotateAuditSigningKey derives a new per-epoch HMAC signing key for the
// project, stores it encrypted in audit_signing_keys, and emits an
// is_anchor=TRUE audit event signed under that new key. The entire
// sequence (advisory lock → read max epoch → store key → emit anchor)
// runs inside one transaction so a second-loser rotation either observes
// the first rotation's committed state (and advances past it) or aborts
// without effect.
//
// Serialization: a per-project pg_advisory_xact_lock is taken at the
// head of the transaction. Even so, a torn race between two rotations
// that read the same max(rotation_epoch) is structurally impossible:
// the unique partial index idx_audit_events_anchor_unique on
// (project_id, rotation_epoch) WHERE is_anchor rejects duplicate
// anchors with 23505; the loser's transaction aborts and retries under
// a fresh advisory lock, observing the winner's anchor and advancing.
func (q *Queries) RotateAuditSigningKey(ctx context.Context, projectID, actorID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RotateAuditSigningKey")
	defer span.End()

	if projectID == "" {
		return 0, fmt.Errorf("rotate audit signing key: project id is empty")
	}
	if q.secretEncryptionKey == "" {
		return 0, fmt.Errorf("rotate audit signing key: secret encryption key is not configured")
	}

	// Retry budget: a bounded number of unique-violation retries handles
	// genuine contention without masking a permanent schema fault.
	// Between attempts we sleep with exponential backoff + full jitter
	// so a burst of contended rotations does not lock-step retry against
	// the same anchor row and force every retry through the same hot
	// advisory key. Without the sleep a busy-loop on 23505 would also
	// hammer the database under contention.
	const (
		maxAttempts = 8
		baseDelay   = 10 * time.Millisecond
		maxDelay    = 200 * time.Millisecond
	)
	var lastErr error
	for attempt := range maxAttempts {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return 0, fmt.Errorf("rotate audit signing key: %w", errors.Join(err, lastErr))
			}
			return 0, fmt.Errorf("rotate audit signing key: %w", err)
		}
		newEpoch, err := q.rotateAuditSigningKeyOnce(ctx, projectID, actorID)
		if err == nil {
			return newEpoch, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Lost the anchor-uniqueness race. Another rotation committed
			// a new epoch between our max-epoch read and our insert.
			// Retry — the advisory lock is transaction-scoped so the
			// next attempt takes a fresh one and will read the new max.
			lastErr = err
			if attempt == maxAttempts-1 {
				break
			}
			shift := min(attempt, 4)
			delay := min(baseDelay<<shift, maxDelay)
			half := delay / 2
			jitter := time.Duration(mrand.Int64N(int64(half) + 1)) //nolint:gosec // jitter only; not used for any security boundary
			sleep := half + jitter
			t := time.NewTimer(sleep)
			select {
			case <-ctx.Done():
				t.Stop()
				return 0, fmt.Errorf("rotate audit signing key: %w", errors.Join(ctx.Err(), lastErr))
			case <-t.C:
			}
			continue
		}
		return 0, err
	}
	return 0, fmt.Errorf("rotate audit signing key: exhausted retries: %w", lastErr)
}

func (q *Queries) rotateAuditSigningKeyOnce(ctx context.Context, projectID, actorID string) (int, error) {
	var newEpoch int
	err := q.withTxInheritKeys(ctx, func(txQ *Queries) error {
		// Serialize per-project rotations and tombstone anchors for the
		// duration of the tx via the shared advisory lock helper. See
		// acquireProjectRotationLock for rationale.
		if err := acquireProjectRotationLock(ctx, txQ, projectID); err != nil {
			return err
		}

		var currentEpoch int
		if err := txQ.db.QueryRow(ctx, `
			SELECT COALESCE(MAX(rotation_epoch), 0)
			FROM audit_events
			WHERE project_id = $1
		`, projectID).Scan(&currentEpoch); err != nil {
			return fmt.Errorf("read max epoch: %w", err)
		}
		newEpoch = currentEpoch + 1

		// Derive and store the new per-epoch HMAC key from the audit
		// signing root. secretEncryptionKey is only the envelope key for
		// the encrypted row and must not be able to derive signatures.
		newKey, err := DeriveAuditSigningKeyForEpochFromRoot(txQ.auditSigningKey, projectID, newEpoch)
		if err != nil {
			return fmt.Errorf("derive new epoch key: %w", err)
		}
		if err := txQ.storeAuditSigningKey(ctx, projectID, newEpoch, newKey, actorID); err != nil {
			return err
		}

		// Enumerate the shards that exist for this project at the moment
		// of rotation. Each shard chain transitions to the new epoch via
		// its own anchor; without a per-shard anchor a shard with prior
		// rows would have no row carrying the new epoch and the chain
		// would silently stall at the boundary. If no rows exist yet we
		// still emit a single legacy-shard anchor so the rotation is
		// recorded forensically and any future event in any shard can
		// chain from a verified anchor.
		//
		// The query is split so the non-empty shard scan can ride the
		// partial index idx_audit_events_shard_chain (WHERE shard_id != '')
		// instead of forcing a sequential scan over the entire project's
		// audit_events history. The empty-shard probe is a cheap EXISTS
		// over the same project_id, served by the project_id+created_at
		// integrity index that exists for the legacy chain.
		rows, qErr := txQ.db.Query(ctx, `
			SELECT shard_id FROM (
				SELECT DISTINCT shard_id
				FROM audit_events
				WHERE project_id = $1 AND shard_id != ''
				UNION ALL
				SELECT '' AS shard_id
				WHERE EXISTS (
					SELECT 1 FROM audit_events
					WHERE project_id = $1 AND shard_id = ''
				)
			) s
		`, projectID)
		if qErr != nil {
			return fmt.Errorf("read shards: %w", qErr)
		}
		var shards []string
		for rows.Next() {
			var sid string
			if scanErr := rows.Scan(&sid); scanErr != nil {
				rows.Close()
				return fmt.Errorf("scan shard: %w", scanErr)
			}
			shards = append(shards, sid)
		}
		rows.Close()
		if rowsErr := rows.Err(); rowsErr != nil {
			return fmt.Errorf("rows err: %w", rowsErr)
		}
		if len(shards) == 0 {
			shards = []string{""}
		}

		// Sign the anchors under the new key by swapping the tx Queries'
		// signing key for the duration of the CreateAuditEvent calls.
		// The tx Queries is already a fresh instance produced by WithTx,
		// so the swap is confined to this tx.
		prevKey := txQ.auditSigningKey
		txQ.auditSigningKey = newKey
		defer func() { txQ.auditSigningKey = prevKey }()

		for _, shardID := range shards {
			// Emit the anchor signed under the NEW epoch's key. Post-
			// rotation events in this shard chain from this anchor and
			// verify under the same key, so the anchor's own signature
			// must also be bound to the new epoch — otherwise a
			// compromise of the old key would let an attacker forge
			// the anchor itself.
			details, err := json.Marshal(map[string]any{
				"previous_epoch": currentEpoch,
				"new_epoch":      newEpoch,
				"rotated_by":     actorID,
				"shard_id":       shardID,
			})
			if err != nil {
				return fmt.Errorf("marshal details: %w", err)
			}
			resourceID := fmt.Sprintf("epoch-%d", newEpoch)
			if shardID != "" {
				resourceID = fmt.Sprintf("epoch-%d-%s", newEpoch, shardID)
			}
			ev := &domain.AuditEvent{
				ProjectID:     projectID,
				ShardID:       shardID,
				ActorID:       actorID,
				ActorType:     "user",
				Action:        domain.AuditActionKeyRotated,
				ResourceType:  "audit_signing_key",
				ResourceID:    resourceID,
				Details:       json.RawMessage(details),
				IsAnchor:      true,
				RotationEpoch: newEpoch,
			}
			// Fail closed if a future edit drops IsAnchor: the auto-
			// derivation in CreateAuditEvent would otherwise route the
			// rotation anchor into a shard literally named after its
			// resource_type ("audit_signing_key") and orphan it from the
			// shard whose chain it is supposed to anchor.
			if !ev.IsAnchor {
				return fmt.Errorf("rotate audit signing key: refusing to emit non-anchor row (project %s, shard %q)", projectID, shardID)
			}
			if err := txQ.CreateAuditEvent(ctx, ev); err != nil {
				return fmt.Errorf("write anchor (shard %q): %w", shardID, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newEpoch, nil
}
