package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateUnsubscribeToken(ctx context.Context, token *domain.UnsubscribeToken) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateUnsubscribeToken")
	defer span.End()

	if token.ID == "" {
		token.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO unsubscribe_tokens (id, project_id, subscriber_id, scope, token, used_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		token.ID,
		token.ProjectID,
		token.SubscriberID,
		token.Scope,
		token.Token,
		token.UsedAt,
		token.ExpiresAt,
	).Scan(&token.CreatedAt)
	if err != nil {
		return fmt.Errorf("create unsubscribe token: %w", err)
	}

	return nil
}

func (q *Queries) GetUnsubscribeToken(ctx context.Context, token string) (*domain.UnsubscribeToken, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetUnsubscribeToken")
	defer span.End()

	query := `
		SELECT id, project_id, subscriber_id, scope, token, used_at, expires_at, created_at
		FROM unsubscribe_tokens
		WHERE token = $1 AND used_at IS NULL AND expires_at > NOW()`

	var row domain.UnsubscribeToken
	err := q.db.QueryRow(ctx, query, token).Scan(
		&row.ID,
		&row.ProjectID,
		&row.SubscriberID,
		&row.Scope,
		&row.Token,
		&row.UsedAt,
		&row.ExpiresAt,
		&row.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUnsubscribeTokenNotFound
		}
		return nil, fmt.Errorf("get unsubscribe token: %w", err)
	}

	return &row, nil
}

func (q *Queries) UseUnsubscribeToken(ctx context.Context, token string, usedAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UseUnsubscribeToken")
	defer span.End()

	tag, err := q.db.Exec(ctx, `UPDATE unsubscribe_tokens SET used_at = $2 WHERE token = $1 AND used_at IS NULL`, token, usedAt)
	if err != nil {
		return fmt.Errorf("use unsubscribe token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUnsubscribeTokenNotFound
	}
	return nil
}

func (q *Queries) TryNotifyDedupKey(ctx context.Context, projectID, dedupKey string, ttl time.Duration) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.TryNotifyDedupKey")
	defer span.End()

	expiresAt := time.Now().UTC().Add(ttl)

	query := `
		INSERT INTO dedup_log (project_id, dedup_key, count, first_at, expires_at)
		VALUES ($1, $2, 1, NOW(), $3)
		ON CONFLICT (project_id, dedup_key)
		DO UPDATE SET
			count = CASE
				WHEN dedup_log.expires_at <= NOW() THEN 1
				ELSE dedup_log.count + 1
			END,
			first_at = CASE
				WHEN dedup_log.expires_at <= NOW() THEN NOW()
				ELSE dedup_log.first_at
			END,
			expires_at = CASE
				WHEN dedup_log.expires_at <= NOW() THEN EXCLUDED.expires_at
				ELSE dedup_log.expires_at
			END
		RETURNING (dedup_log.expires_at <= NOW()) AS was_expired`

	var wasExpired bool
	if err := q.db.QueryRow(ctx, query, projectID, dedupKey, expiresAt).Scan(&wasExpired); err != nil {
		// If insert path happened (no conflict), RETURNING still works and wasExpired=false by default semantics above.
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("try notify dedup key: %w", err)
	}

	// Allowed when key was expired (new window) or newly inserted (wasExpired false but first insert).
	// Since first insert doesn't hit conflict, dedup_log.expires_at refers to inserted row and comparison is false.
	// We treat false as allowed here and suppress only when active duplicate.
	if !wasExpired {
		// Determine if this was a duplicate within active window by checking count > 1.
		var count int
		err := q.db.QueryRow(ctx, `SELECT count FROM dedup_log WHERE project_id = $1 AND dedup_key = $2`, projectID, dedupKey).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("read dedup key count: %w", err)
		}
		return count == 1, nil
	}

	return true, nil
}
