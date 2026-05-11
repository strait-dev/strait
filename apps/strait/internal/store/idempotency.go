package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// IdempotencyStatus represents the result of trying to acquire an idempotency key.
const (
	IdempotencyAcquired = "acquired"  // We inserted a pending row; caller should execute the handler.
	IdempotencyPending  = "pending"   // Another request owns the key and is still processing.
	IdempotencyComplete = "completed" // A previous request completed; cached response is available.
)

// TryAcquireIdempotencyKey attempts to claim an idempotency key for exclusive processing.
// If the key does not exist, a pending row is inserted and "acquired" is returned.
// If the key exists and is completed, the cached response is returned.
// If the key exists and is pending, "pending" is returned (caller should 409).
func (q *Queries) TryAcquireIdempotencyKey(ctx context.Context, projectID, key string, ttl time.Duration) (string, int, http.Header, []byte, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.TryAcquireIdempotencyKey")
	defer span.End()

	expiresAt := time.Now().Add(ttl)

	// Attempt insert; ON CONFLICT DO NOTHING means we can detect whether we won the race
	// by checking if the row was actually inserted (via RETURNING), then fall back to SELECT.
	var status string
	var responseStatus *int
	var responseHeaders []byte
	var responseBody []byte

	// Try to insert first.
	tag, err := q.db.Exec(ctx, `
		INSERT INTO idempotency_keys (project_id, key, status, expires_at)
		VALUES ($1, $2, 'pending', $3)
		ON CONFLICT (project_id, key) DO NOTHING`,
		projectID, key, expiresAt,
	)
	if err != nil {
		return "", 0, nil, nil, fmt.Errorf("insert idempotency key: %w", err)
	}

	// If we inserted, we own the lock.
	if tag.RowsAffected() == 1 {
		return IdempotencyAcquired, 0, nil, nil, nil
	}

	// Row already exists -- read its state.
	err = q.db.QueryRow(ctx, `
		SELECT status, response_status, response_headers, response_body
		FROM idempotency_keys
		WHERE project_id = $1 AND key = $2
		  AND expires_at > NOW()`,
		projectID, key,
	).Scan(&status, &responseStatus, &responseHeaders, &responseBody)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Row existed but expired between the INSERT and SELECT.
			// Delete the stale row and retry the insert.
			_, _ = q.db.Exec(ctx, `
				DELETE FROM idempotency_keys
				WHERE project_id = $1 AND key = $2 AND expires_at <= NOW()`,
				projectID, key,
			)
			// Retry: insert again now that the expired row is gone.
			tag2, retryErr := q.db.Exec(ctx, `
				INSERT INTO idempotency_keys (project_id, key, status, expires_at)
				VALUES ($1, $2, 'pending', $3)
				ON CONFLICT (project_id, key) DO NOTHING`,
				projectID, key, expiresAt,
			)
			if retryErr != nil {
				return "", 0, nil, nil, fmt.Errorf("retry insert idempotency key: %w", retryErr)
			}
			if tag2.RowsAffected() == 1 {
				return IdempotencyAcquired, 0, nil, nil, nil
			}
			// Another request beat us on retry -- fall through to pending.
			return IdempotencyPending, 0, nil, nil, nil
		}
		return "", 0, nil, nil, fmt.Errorf("select idempotency key: %w", err)
	}

	rs := 0
	if responseStatus != nil {
		rs = *responseStatus
	}

	hdr, err := unmarshalIdempotencyHeaders(responseHeaders)
	if err != nil {
		return "", 0, nil, nil, fmt.Errorf("decode idempotency headers: %w", err)
	}

	return status, rs, hdr, responseBody, nil
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

	_, err = q.db.Exec(ctx, `
		UPDATE idempotency_keys
		SET status = 'completed', response_status = $3, response_headers = $4, response_body = $5
		WHERE project_id = $1 AND key = $2 AND status = 'pending'`,
		projectID, key, responseStatus, hdrJSON, responseBody,
	)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}

// marshalIdempotencyHeaders serializes an http.Header to JSON for the
// response_headers column. Returns nil for empty inputs so the column
// stores NULL rather than {}.
func marshalIdempotencyHeaders(h http.Header) ([]byte, error) {
	if len(h) == 0 {
		return nil, nil
	}
	return json.Marshal(h)
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
	return h, nil
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
