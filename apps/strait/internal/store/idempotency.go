package store

import (
	"context"
	"errors"
	"fmt"
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
func (q *Queries) TryAcquireIdempotencyKey(ctx context.Context, projectID, key string, ttl time.Duration) (string, int, []byte, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.TryAcquireIdempotencyKey")
	defer span.End()

	expiresAt := time.Now().Add(ttl)

	// Attempt insert; ON CONFLICT DO NOTHING means we can detect whether we won the race
	// by checking if the row was actually inserted (via RETURNING), then fall back to SELECT.
	var status string
	var responseStatus *int
	var responseBody []byte

	// Try to insert first.
	tag, err := q.db.Exec(ctx, `
		INSERT INTO idempotency_keys (project_id, key, status, expires_at)
		VALUES ($1, $2, 'pending', $3)
		ON CONFLICT (project_id, key) DO NOTHING`,
		projectID, key, expiresAt,
	)
	if err != nil {
		return "", 0, nil, fmt.Errorf("insert idempotency key: %w", err)
	}

	// If we inserted, we own the lock.
	if tag.RowsAffected() == 1 {
		return IdempotencyAcquired, 0, nil, nil
	}

	// Row already exists -- read its state.
	err = q.db.QueryRow(ctx, `
		SELECT status, response_status, response_body
		FROM idempotency_keys
		WHERE project_id = $1 AND key = $2
		  AND expires_at > NOW()`,
		projectID, key,
	).Scan(&status, &responseStatus, &responseBody)
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
				return "", 0, nil, fmt.Errorf("retry insert idempotency key: %w", retryErr)
			}
			if tag2.RowsAffected() == 1 {
				return IdempotencyAcquired, 0, nil, nil
			}
			// Another request beat us on retry -- fall through to pending.
			return IdempotencyPending, 0, nil, nil
		}
		return "", 0, nil, fmt.Errorf("select idempotency key: %w", err)
	}

	rs := 0
	if responseStatus != nil {
		rs = *responseStatus
	}

	return status, rs, responseBody, nil
}

// CompleteIdempotencyKey updates a pending idempotency key with the handler's response.
func (q *Queries) CompleteIdempotencyKey(ctx context.Context, projectID, key string, responseStatus int, responseBody []byte) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CompleteIdempotencyKey")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		UPDATE idempotency_keys
		SET status = 'completed', response_status = $3, response_body = $4
		WHERE project_id = $1 AND key = $2 AND status = 'pending'`,
		projectID, key, responseStatus, responseBody,
	)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}

// CleanExpiredIdempotencyKeys removes idempotency keys that have passed their TTL.
func (q *Queries) CleanExpiredIdempotencyKeys(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CleanExpiredIdempotencyKeys")
	defer span.End()

	tag, err := q.db.Exec(ctx, `DELETE FROM idempotency_keys WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("clean expired idempotency keys: %w", err)
	}
	return tag.RowsAffected(), nil
}
