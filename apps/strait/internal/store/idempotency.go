package store

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	mrand "math/rand/v2"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

// IdempotencyStatus represents the result of trying to acquire an idempotency key.
const (
	IdempotencyAcquired = "acquired"  // We inserted a pending row; caller should execute the handler.
	IdempotencyPending  = "pending"   // Another request owns the key and is still processing.
	IdempotencyComplete = "completed" // A previous request completed; cached response is available.
)

// idempotencyMaxAttempts bounds the retry budget when Postgres reports a
// transient failure (serialization_failure, deadlock_detected, lock_timeout)
// during the advisory-lock-protected insert path.
const idempotencyMaxAttempts = 3

type encryptedIdempotencyResponseBody struct {
	Encrypted bool   `json:"encrypted"`
	Version   int    `json:"version"`
	Body      string `json:"body"`
}

// idempotencyBackoff is the per-attempt base sleep before retrying a
// transient failure. The first attempt has no preceding sleep; the second
// waits 5ms, the third 20ms. Total worst-case retry wait is ~25ms before
// jitter. See idempotencyBackoffWithJitter for the actual sleep duration.
var idempotencyBackoff = [idempotencyMaxAttempts]time.Duration{
	0,
	5 * time.Millisecond,
	20 * time.Millisecond,
}

// idempotencyBackoffJitter is the ±fraction applied to each non-zero
// backoff duration. ±20% breaks up converging retry storms when many
// concurrent requests collide on the same advisory-lock key and all
// receive a transient error at the same instant. Without jitter, all
// retriers would sleep for exactly the same window and re-collide.
const idempotencyBackoffJitter = 0.2

// idempotencyBackoffWithJitter returns the sleep duration for attempt n,
// applying ±idempotencyBackoffJitter to the base value. Attempt 0 always
// returns 0 (no sleep). Uses math/rand/v2 which is goroutine-safe and
// auto-seeded; this is not a security-sensitive RNG so crypto/rand is
// unnecessary overhead.
func idempotencyBackoffWithJitter(attempt int) time.Duration {
	if attempt <= 0 || attempt >= len(idempotencyBackoff) {
		return 0
	}
	base := idempotencyBackoff[attempt]
	if base <= 0 {
		return 0
	}
	delta := (mrand.Float64()*2 - 1) * idempotencyBackoffJitter //nolint:gosec // math/rand/v2 is appropriate for retry jitter; not security-sensitive
	return time.Duration(float64(base) * (1 + delta))
}

// idempotencyAdvisoryKey derives a stable 64-bit advisory-lock key from
// (projectID, key). Length-prefixing each segment removes ambiguity from a
// shared separator byte (e.g., ("a", "b:c") vs ("a:b", "c")).
//
// Collision math: SHA-256 truncated to 64 bits behaves as a uniform hash
// over the advisory-lock keyspace (2^64). By the birthday bound, the
// probability of any collision among N distinct (project, key) pairs is
// roughly N^2 / 2^65. Examples:
//
//	N = 1,000,000      -> ~2.7e-8  (1 chance in 37 million)
//	N = 100,000,000    -> ~2.7e-4  (1 chance in ~3,700)
//	N = 1,000,000,000  -> ~2.7e-2  (~3% — exceptionally large tenant scale)
//
// A collision means two unrelated (project, key) pairs serialize on the
// same pg_advisory_xact_lock, costing one extra microsecond of contention
// (project_id, key) PRIMARY KEY constraint on idempotency_keys; the
// advisory lock is purely a contention reducer that lets us avoid the
// row-lock + deadlock-prone INSERT-then-SELECT pattern.
//
// If the platform ever scales past ~10^8 active idempotency keys in a
// single instance, revisit: either widen the advisory key (e.g., go to
// 128-bit advisory keys via the two-bigint API) or shard by project_id.
func idempotencyAdvisoryKey(projectID, key string) int64 {
	h := sha256.New()
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(projectID))) //nolint:gosec // projectID is a bounded identifier (well under 2^32 bytes)
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(projectID))
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(key))) //nolint:gosec // key is request-validated (256-byte cap, well under 2^32)
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(key))
	sum := h.Sum(nil)
	return int64(binary.BigEndian.Uint64(sum[:8])) //nolint:gosec // intentional truncation to advisory-lock key space
}

// logIdempotencyRollbackErr emits a structured warning when a deferred
// tx.Rollback returns a non-nil error that isn't pgx.ErrTxClosed
// (already-committed transactions report ErrTxClosed on Rollback, which is
// the expected no-op path and not worth logging).
func logIdempotencyRollbackErr(err error) {
	if err == nil || errors.Is(err, pgx.ErrTxClosed) {
		return
	}
	slog.Warn("failed to rollback idempotency transaction", "error", err)
}

// isIdempotencyTransientError reports whether err is a Postgres transient
// failure that we retry: serialization_failure (40001), deadlock_detected
// (40P01), or lock_timeout (55P03).
func isIdempotencyTransientError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", "40P01", "55P03":
		return true
	}
	return false
}

// TryAcquireIdempotencyKey attempts to claim an idempotency key for exclusive
// processing. If the key does not exist, a pending row is inserted and
// "acquired" is returned. If the key exists and is completed, the cached
// response is returned. If the key exists and is pending, "pending" is
// returned (caller should respond 409).
//
// Hot keys are serialized via pg_advisory_xact_lock keyed on a hash of
// (projectID, key), which bounds contention by lock acquisition rather than
// row-lock waits on the (project_id, key) primary key. Transient failures
// (40001/40P01/55P03) are retried up to idempotencyMaxAttempts times.
func (q *Queries) TryAcquireIdempotencyKey(ctx context.Context, projectID, key string, ttl time.Duration) (string, int, http.Header, []byte, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.TryAcquireIdempotencyKey")
	defer span.End()

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return "", 0, nil, nil, errors.New("idempotency acquire requires transactional database handle")
	}

	advisoryKey := idempotencyAdvisoryKey(projectID, key)
	expiresAt := time.Now().Add(ttl)

	var lastErr error
	for attempt := range idempotencyMaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", 0, nil, nil, ctx.Err()
			case <-time.After(idempotencyBackoffWithJitter(attempt)):
			}
		}
		status, rs, hdr, body, err := q.tryAcquireWithAdvisoryLock(ctx, beginner, advisoryKey, projectID, key, expiresAt)
		if err == nil {
			return status, rs, hdr, body, nil
		}
		if !isIdempotencyTransientError(err) {
			return "", 0, nil, nil, err
		}
		lastErr = err
	}
	return "", 0, nil, nil, fmt.Errorf("idempotency acquire: retries exhausted: %w", lastErr)
}

func (q *Queries) TryAcquireCronFire(ctx context.Context, projectID, key string, ttl time.Duration) (bool, error) {
	status, _, _, _, err := q.TryAcquireIdempotencyKey(ctx, projectID, key, ttl)
	if err != nil {
		return false, err
	}
	return status == IdempotencyAcquired, nil
}

func (q *Queries) tryAcquireWithAdvisoryLock(ctx context.Context, beginner TxBeginner, advisoryKey int64, projectID, key string, expiresAt time.Time) (string, int, http.Header, []byte, error) {
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return "", 0, nil, nil, fmt.Errorf("begin idempotency tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		logIdempotencyRollbackErr(tx.Rollback(ctx))
	}()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", advisoryKey); err != nil {
		return "", 0, nil, nil, fmt.Errorf("advisory lock: %w", err)
	}

	// Insert with RETURNING so the winner avoids the second round-trip.
	var insertedExpires time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO idempotency_keys (project_id, key, status, expires_at)
		VALUES ($1, $2, 'pending', $3)
		ON CONFLICT (project_id, key) DO NOTHING
		RETURNING expires_at`,
		projectID, key, expiresAt,
	).Scan(&insertedExpires)
	if err == nil {
		if cmErr := tx.Commit(ctx); cmErr != nil {
			return "", 0, nil, nil, fmt.Errorf("commit idempotency insert: %w", cmErr)
		}
		committed = true
		return IdempotencyAcquired, 0, nil, nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil, nil, fmt.Errorf("insert idempotency key: %w", err)
	}

	// Conflict: an existing row holds the key. Read it.
	var status string
	var responseStatus *int
	var responseHeaders []byte
	var responseBody []byte
	err = tx.QueryRow(ctx, `
		SELECT status, response_status, response_headers, response_body
		FROM idempotency_keys
		WHERE project_id = $1 AND key = $2 AND expires_at > NOW()`,
		projectID, key,
	).Scan(&status, &responseStatus, &responseHeaders, &responseBody)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", 0, nil, nil, fmt.Errorf("select idempotency key: %w", err)
		}
		// Existing row expired between INSERT and SELECT. The advisory lock
		// keeps this serialized; replace the stale row in-place.
		outStatus, replaceErr := replaceExpiredIdempotencyRow(ctx, tx, projectID, key, expiresAt)
		if replaceErr != nil {
			return "", 0, nil, nil, replaceErr
		}
		if cmErr := tx.Commit(ctx); cmErr != nil {
			return "", 0, nil, nil, fmt.Errorf("commit expired idempotency replace: %w", cmErr)
		}
		committed = true
		return outStatus, 0, nil, nil, nil
	}

	if cmErr := tx.Commit(ctx); cmErr != nil {
		return "", 0, nil, nil, fmt.Errorf("commit idempotency read: %w", cmErr)
	}
	committed = true

	rs := 0
	if responseStatus != nil {
		rs = *responseStatus
	}
	hdr, hdrErr := unmarshalIdempotencyHeaders(responseHeaders)
	if hdrErr != nil {
		return "", 0, nil, nil, fmt.Errorf("decode idempotency headers: %w", hdrErr)
	}
	body, bodyErr := q.decryptIdempotencyResponseBody(responseBody)
	if bodyErr != nil {
		return "", 0, nil, nil, fmt.Errorf("decode idempotency response body: %w", bodyErr)
	}
	return status, rs, hdr, body, nil
}

// replaceExpiredIdempotencyRow handles the rare race where the SELECT inside
// tryAcquireWithAdvisoryLock found no live row (the existing entry expired
// between the INSERT and SELECT). The advisory lock keeps this serialized:
// delete the stale row in-place and re-attempt the INSERT. Returns
// IdempotencyAcquired on insert win, IdempotencyPending when a concurrent
// winner inserted ahead of us, or an error otherwise. The caller is
// responsible for committing the transaction on a nil error return.
func replaceExpiredIdempotencyRow(ctx context.Context, tx pgx.Tx, projectID, key string, expiresAt time.Time) (string, error) {
	if _, err := tx.Exec(ctx, `
		DELETE FROM idempotency_keys
		WHERE project_id = $1 AND key = $2 AND expires_at <= NOW()`,
		projectID, key,
	); err != nil {
		return "", fmt.Errorf("delete expired idempotency key: %w", err)
	}
	var insertedExpires time.Time
	err := tx.QueryRow(ctx, `
		INSERT INTO idempotency_keys (project_id, key, status, expires_at)
		VALUES ($1, $2, 'pending', $3)
		ON CONFLICT (project_id, key) DO NOTHING
		RETURNING expires_at`,
		projectID, key, expiresAt,
	).Scan(&insertedExpires)
	if err == nil {
		return IdempotencyAcquired, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyPending, nil
	}
	return "", fmt.Errorf("retry insert idempotency key: %w", err)
}

// CompleteIdempotencyKey updates a pending idempotency key with the handler's response.
// responseHeaders may be nil for legacy callers that have no header
// snapshot to memoize; the column will be NULL and replays will fall
// back to whatever Content-Type the middleware computes. New code paths
// must always pass the captured headers so spec-compliant replay works.
func (q *Queries) CompleteIdempotencyKey(ctx context.Context, projectID, key string, responseStatus int, responseHeaders http.Header, responseBody []byte) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CompleteIdempotencyKey")
	defer span.End()

	hdrJSON, err := marshalIdempotencyHeaders(responseHeaders)
	if err != nil {
		return fmt.Errorf("encode idempotency headers: %w", err)
	}
	bodyJSON, err := q.encryptIdempotencyResponseBody(responseBody)
	if err != nil {
		return fmt.Errorf("encode idempotency response body: %w", err)
	}

	_, err = q.db.Exec(ctx, `
		UPDATE idempotency_keys
		SET status = 'completed', response_status = $3, response_headers = $4, response_body = $5
		WHERE project_id = $1 AND key = $2 AND status = 'pending'`,
		projectID, key, responseStatus, hdrJSON, bodyJSON,
	)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}

func (q *Queries) encryptIdempotencyResponseBody(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, nil
	}
	if q.secretEncryptionKey == "" {
		return nil, errors.New("secret encryption key is required")
	}
	enc, err := q.secretEncryptor()
	if err != nil {
		return nil, err
	}
	ciphertext, err := enc.Encrypt(body)
	if err != nil {
		return nil, err
	}
	return json.Marshal(encryptedIdempotencyResponseBody{
		Encrypted: true,
		Version:   1,
		Body:      base64.StdEncoding.EncodeToString(ciphertext),
	})
}

func (q *Queries) decryptIdempotencyResponseBody(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	// Encrypted bodies are a JSON object emitted by encryptIdempotencyResponseBody
	// carrying "encrypted":true. Detect by strict decode, not a substring match:
	// a legacy/plaintext body that is non-JSON, or JSON that merely contains the
	// word "encrypted", must pass through untouched rather than being misparsed
	// (which previously corrupted the replay or returned an error).
	var wrapper encryptedIdempotencyResponseBody
	if err := json.Unmarshal(raw, &wrapper); err != nil || !wrapper.Encrypted {
		return raw, nil
	}
	if wrapper.Version != 1 {
		return nil, fmt.Errorf("unsupported encrypted body version %d", wrapper.Version)
	}
	if q.secretEncryptionKey == "" {
		return nil, errors.New("secret encryption key is required")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(wrapper.Body)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted body: %w", err)
	}
	enc, err := q.secretEncryptor()
	if err != nil {
		return nil, err
	}
	return enc.Decrypt(ciphertext)
}

// marshalIdempotencyHeaders serializes an http.Header to JSON for the
// response_headers column. Returns nil for empty inputs so the column
// stores NULL rather than {}.
func marshalIdempotencyHeaders(h http.Header) ([]byte, error) {
	if len(h) == 0 {
		return nil, nil
	}
	safe := sanitizeIdempotencyHeaders(h)
	if len(safe) == 0 {
		return nil, nil
	}
	return json.Marshal(safe)
}

// unmarshalIdempotencyHeaders parses the response_headers JSONB into an
// http.Header. NULL or empty input returns nil (the caller must tolerate
// pre-migration rows that have no header snapshot).
func unmarshalIdempotencyHeaders(raw []byte) (http.Header, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var h http.Header
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, err
	}
	return sanitizeIdempotencyHeaders(h), nil
}

func sanitizeIdempotencyHeaders(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for key, values := range h {
		switch http.CanonicalHeaderKey(key) {
		case "Set-Cookie", "Set-Cookie2", "Authorization", "Proxy-Authorization":
			continue
		default:
			out[key] = append([]string(nil), values...)
		}
	}
	return out
}

// DeleteIdempotencyKey removes a single idempotency key. Used to clean up
// pending rows after handler errors or panics.
func (q *Queries) DeleteIdempotencyKey(ctx context.Context, projectID, key string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteIdempotencyKey")
	defer span.End()

	tag, err := q.db.Exec(ctx, `DELETE FROM idempotency_keys WHERE project_id = $1 AND key = $2`, projectID, key)
	if err != nil {
		return 0, fmt.Errorf("delete idempotency key: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CleanExpiredIdempotencyKeys removes idempotency keys that have passed their TTL.
// Deletes in batches of 10000 to avoid holding table-level locks for extended periods.
func (q *Queries) CleanExpiredIdempotencyKeys(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CleanExpiredIdempotencyKeys")
	defer span.End()

	var total int64
	for {
		tag, err := q.db.Exec(ctx, `
			DELETE FROM idempotency_keys WHERE ctid IN (
				SELECT ctid FROM idempotency_keys WHERE expires_at < NOW() LIMIT 10000
			)`)
		if err != nil {
			return total, fmt.Errorf("clean expired idempotency keys: %w", err)
		}
		n := tag.RowsAffected()
		total += n
		if n < 10000 {
			break
		}
	}
	return total, nil
}
