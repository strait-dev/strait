package store

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateAPIKey")
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetAPIKeyByHash")
	defer span.End()

	query := `SELECT id, project_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at
			  FROM api_keys WHERE key_hash = $1`

	var key domain.APIKey
	err := q.db.QueryRow(ctx, query, keyHash).Scan(
		&key.ID, &key.ProjectID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.Scopes, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}

	return &key, nil
}

func (q *Queries) ListAPIKeysByProject(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListAPIKeysByProject")
	defer span.End()

	query := `SELECT id, project_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at
			  FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL
			  ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0)
	for rows.Next() {
		var key domain.APIKey
		if err := rows.Scan(
			&key.ID, &key.ProjectID, &key.Name, &key.KeyHash, &key.KeyPrefix,
			&key.Scopes, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("list api keys scan: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}

func (q *Queries) RevokeAPIKey(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.RevokeAPIKey")
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.TouchAPIKeyLastUsed")
	defer span.End()

	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("touch api key last used: %w", err)
	}

	return nil
}
