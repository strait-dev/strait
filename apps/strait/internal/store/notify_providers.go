package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"strait/internal/crypto"
	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateNotificationProvider(ctx context.Context, provider *domain.NotificationProvider) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotificationProvider")
	defer span.End()

	if provider.ID == "" {
		provider.ID = uuid.Must(uuid.NewV7()).String()
	}
	if provider.Health == "" {
		provider.Health = "healthy"
	}

	configBytes := provider.ConfigEnc
	if q.secretEncryptionKey != "" && len(configBytes) > 0 {
		enc, err := crypto.NewEncryptor(q.secretEncryptionKey)
		if err != nil {
			slog.Warn("failed to create encryptor for notification provider", "provider_id", provider.ID, "error", err)
		} else {
			encrypted, encryptErr := enc.Encrypt(configBytes)
			if encryptErr != nil {
				slog.Warn("failed to encrypt notification provider config", "provider_id", provider.ID, "error", encryptErr)
			} else {
				configBytes = encrypted
			}
		}
	}

	query := `
		INSERT INTO notification_providers (
			id, project_id, channel, provider, name, config_enc, is_default, fallback_id, health, rate_limit
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		provider.ID,
		provider.ProjectID,
		provider.Channel,
		provider.Provider,
		provider.Name,
		configBytes,
		provider.IsDefault,
		dbscan.NilIfEmptyString(provider.FallbackID),
		provider.Health,
		provider.RateLimit,
	).Scan(&provider.CreatedAt, &provider.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create notification provider: %w", err)
	}

	return nil
}

func (q *Queries) GetNotificationProvider(ctx context.Context, id, projectID string) (*domain.NotificationProvider, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotificationProvider")
	defer span.End()

	query := `
		SELECT id, project_id, channel, provider, name, config_enc, is_default, fallback_id, health, rate_limit, created_at, updated_at
		FROM notification_providers
		WHERE id = $1 AND project_id = $2`

	provider, err := scanNotificationProvider(q.db.QueryRow(ctx, query, id, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationProviderNotFound
		}
		return nil, fmt.Errorf("get notification provider: %w", err)
	}

	provider.ConfigEnc = q.decryptNotificationProviderConfig(provider.ID, provider.ConfigEnc)
	return provider, nil
}

func (q *Queries) ListNotificationProviders(ctx context.Context, projectID, channel string) ([]domain.NotificationProvider, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotificationProviders")
	defer span.End()

	query := `
		SELECT id, project_id, channel, provider, name, config_enc, is_default, fallback_id, health, rate_limit, created_at, updated_at
		FROM notification_providers
		WHERE project_id = $1
		  AND ($2::text IS NULL OR channel = $2)
		ORDER BY created_at DESC`

	var channelValue any
	if channel != "" {
		channelValue = channel
	}

	rows, err := q.db.Query(ctx, query, projectID, channelValue)
	if err != nil {
		return nil, fmt.Errorf("list notification providers: %w", err)
	}
	defer rows.Close()

	providers := make([]domain.NotificationProvider, 0, 8)
	for rows.Next() {
		provider, scanErr := scanNotificationProvider(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notification providers scan: %w", scanErr)
		}
		provider.ConfigEnc = q.decryptNotificationProviderConfig(provider.ID, provider.ConfigEnc)
		providers = append(providers, *provider)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification providers rows: %w", err)
	}

	return providers, nil
}

func (q *Queries) UpdateNotificationProvider(ctx context.Context, provider *domain.NotificationProvider) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateNotificationProvider")
	defer span.End()

	configBytes := provider.ConfigEnc
	if q.secretEncryptionKey != "" && len(configBytes) > 0 {
		enc, err := crypto.NewEncryptor(q.secretEncryptionKey)
		if err != nil {
			slog.Warn("failed to create encryptor for notification provider", "provider_id", provider.ID, "error", err)
		} else {
			encrypted, encryptErr := enc.Encrypt(configBytes)
			if encryptErr != nil {
				slog.Warn("failed to encrypt notification provider config", "provider_id", provider.ID, "error", encryptErr)
			} else {
				configBytes = encrypted
			}
		}
	}

	query := `
		UPDATE notification_providers
		SET channel = $3,
			provider = $4,
			name = $5,
			config_enc = $6,
			is_default = $7,
			fallback_id = $8,
			health = $9,
			rate_limit = $10,
			updated_at = NOW()
		WHERE id = $1 AND project_id = $2
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		provider.ID,
		provider.ProjectID,
		provider.Channel,
		provider.Provider,
		provider.Name,
		configBytes,
		provider.IsDefault,
		dbscan.NilIfEmptyString(provider.FallbackID),
		provider.Health,
		provider.RateLimit,
	).Scan(&provider.CreatedAt, &provider.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotificationProviderNotFound
		}
		return fmt.Errorf("update notification provider: %w", err)
	}

	return nil
}

func scanNotificationProvider(scanner scanTarget) (*domain.NotificationProvider, error) {
	var provider domain.NotificationProvider
	var fallbackID *string

	err := scanner.Scan(
		&provider.ID,
		&provider.ProjectID,
		&provider.Channel,
		&provider.Provider,
		&provider.Name,
		&provider.ConfigEnc,
		&provider.IsDefault,
		&fallbackID,
		&provider.Health,
		&provider.RateLimit,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if fallbackID != nil {
		provider.FallbackID = *fallbackID
	}

	return &provider, nil
}

func (q *Queries) decryptNotificationProviderConfig(providerID string, config []byte) []byte {
	if len(config) == 0 || q.secretEncryptionKey == "" {
		return config
	}

	enc, err := crypto.NewEncryptor(q.secretEncryptionKey)
	if err != nil {
		slog.Warn("failed to create encryptor for notification provider decryption", "provider_id", providerID, "error", err)
		return config
	}

	decrypted, decryptErr := enc.Decrypt(config)
	if decryptErr != nil {
		slog.Warn("failed to decrypt notification provider config, returning raw bytes", "provider_id", providerID, "error", decryptErr)
		return config
	}
	return decrypted
}
