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

func (q *Queries) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAPIKey")
	defer span.End()

	if key.ID == "" {
		key.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO api_keys (id, project_id, name, key_hash, key_prefix, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		key.ID, key.ProjectID, key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.ExpiresAt,
	).Scan(&key.CreatedAt)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	return nil
}

func (q *Queries) GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAPIKeyByHash")
	defer span.End()

	query := `SELECT id, project_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at
			  FROM api_keys WHERE key_hash = $1`

	var key domain.APIKey
	err := q.db.QueryRow(ctx, query, keyHash).Scan(
		&key.ID, &key.ProjectID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.Scopes, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt, &key.ReplacedByKeyID, &key.GraceExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}

	return &key, nil
}

func (q *Queries) ListAPIKeysByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysByProject")
	defer span.End()

	query := `SELECT id, project_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at
			  FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL`

	args := []any{projectID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		var key domain.APIKey
		if err := rows.Scan(
			&key.ID, &key.ProjectID, &key.Name, &key.KeyHash, &key.KeyPrefix,
			&key.Scopes, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt, &key.ReplacedByKeyID, &key.GraceExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("list api keys scan: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}

func (q *Queries) RevokeAPIKey(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RevokeAPIKey")
	defer span.End()

	query := `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api key not found or already revoked")
	}

	return nil
}

func (q *Queries) TouchAPIKeyLastUsed(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.TouchAPIKeyLastUsed")
	defer span.End()

	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("touch api key last used: %w", err)
	}

	return nil
}

func (q *Queries) GetAPIKeyByID(ctx context.Context, id string) (*domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAPIKeyByID")
	defer span.End()

	query := `SELECT id, project_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at
			  FROM api_keys WHERE id = $1`

	var key domain.APIKey
	err := q.db.QueryRow(ctx, query, id).Scan(
		&key.ID, &key.ProjectID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.Scopes, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt, &key.ReplacedByKeyID, &key.GraceExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return &key, nil
}

func (q *Queries) MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkAPIKeyRotated")
	defer span.End()

	query := `
		UPDATE api_keys
		SET replaced_by_key_id = $2, grace_expires_at = $3
		WHERE id = $1 AND revoked_at IS NULL`
	tag, err := q.db.Exec(ctx, query, oldKeyID, newKeyID, graceExpiresAt)
	if err != nil {
		return fmt.Errorf("mark api key rotated: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api key not found or already revoked")
	}
	return nil
}
