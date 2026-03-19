package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/crypto"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotificationChannel")
	defer span.End()

	if ch.ID == "" {
		ch.ID = uuid.Must(uuid.NewV7()).String()
	}

	configBytes := ch.Config
	if q.secretEncryptionKey != "" && len(configBytes) > 0 {
		enc, encErr := crypto.NewEncryptor(q.secretEncryptionKey)
		if encErr != nil {
			slog.Warn("failed to create encryptor for notification channel config", "channel_id", ch.ID, "error", encErr)
		} else {
			encrypted, encryptErr := enc.Encrypt(configBytes)
			if encryptErr != nil {
				slog.Warn("failed to encrypt notification channel config", "channel_id", ch.ID, "error", encryptErr)
			} else {
				configBytes = encrypted
			}
		}
	}

	query := `
		INSERT INTO notification_channels (id, project_id, channel_type, name, config, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		ch.ID,
		ch.ProjectID,
		ch.ChannelType,
		ch.Name,
		configBytes,
		ch.Enabled,
	).Scan(&ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create notification channel: %w", err)
	}

	return nil
}

func (q *Queries) GetNotificationChannel(ctx context.Context, id, projectID string) (*domain.NotificationChannel, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotificationChannel")
	defer span.End()

	query := `
		SELECT id, project_id, channel_type, name, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE id = $1 AND project_id = $2`

	var ch domain.NotificationChannel
	err := q.db.QueryRow(ctx, query, id, projectID).Scan(
		&ch.ID, &ch.ProjectID, &ch.ChannelType, &ch.Name,
		&ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationChannelNotFound
		}
		return nil, fmt.Errorf("get notification channel: %w", err)
	}

	ch.Config = q.decryptNotificationConfig(ch.ID, ch.Config)

	return &ch, nil
}

func (q *Queries) ListNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotificationChannels")
	defer span.End()

	query := `
		SELECT id, project_id, channel_type, name, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list notification channels: %w", err)
	}
	defer rows.Close()

	channels := make([]domain.NotificationChannel, 0, 32)
	for rows.Next() {
		var ch domain.NotificationChannel
		if err := rows.Scan(
			&ch.ID, &ch.ProjectID, &ch.ChannelType, &ch.Name,
			&ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list notification channels scan: %w", err)
		}
		ch.Config = q.decryptNotificationConfig(ch.ID, ch.Config)
		channels = append(channels, ch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification channels rows: %w", err)
	}

	return channels, nil
}

func (q *Queries) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEnabledNotificationChannels")
	defer span.End()

	query := `
		SELECT id, project_id, channel_type, name, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE project_id = $1 AND enabled = true
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list enabled notification channels: %w", err)
	}
	defer rows.Close()

	channels := make([]domain.NotificationChannel, 0, 16)
	for rows.Next() {
		var ch domain.NotificationChannel
		if err := rows.Scan(
			&ch.ID, &ch.ProjectID, &ch.ChannelType, &ch.Name,
			&ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list enabled notification channels scan: %w", err)
		}
		ch.Config = q.decryptNotificationConfig(ch.ID, ch.Config)
		channels = append(channels, ch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list enabled notification channels rows: %w", err)
	}

	return channels, nil
}

func (q *Queries) UpdateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateNotificationChannel")
	defer span.End()

	configBytes := ch.Config
	if q.secretEncryptionKey != "" && len(configBytes) > 0 {
		enc, encErr := crypto.NewEncryptor(q.secretEncryptionKey)
		if encErr != nil {
			slog.Warn("failed to create encryptor for notification channel config", "channel_id", ch.ID, "error", encErr)
		} else {
			encrypted, encryptErr := enc.Encrypt(configBytes)
			if encryptErr != nil {
				slog.Warn("failed to encrypt notification channel config", "channel_id", ch.ID, "error", encryptErr)
			} else {
				configBytes = encrypted
			}
		}
	}

	query := `
		UPDATE notification_channels
		SET name = $2, channel_type = $3, config = $4, enabled = $5, updated_at = NOW()
		WHERE id = $1 AND project_id = $6
		RETURNING updated_at`

	err := q.db.QueryRow(ctx, query, ch.ID, ch.Name, ch.ChannelType, configBytes, ch.Enabled, ch.ProjectID).Scan(&ch.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotificationChannelNotFound
		}
		return fmt.Errorf("update notification channel: %w", err)
	}

	return nil
}

func (q *Queries) DeleteNotificationChannel(ctx context.Context, id, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteNotificationChannel")
	defer span.End()

	query := `DELETE FROM notification_channels WHERE id = $1 AND project_id = $2`
	tag, err := q.db.Exec(ctx, query, id, projectID)
	if err != nil {
		return fmt.Errorf("delete notification channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationChannelNotFound
	}

	return nil
}

func (q *Queries) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotificationDelivery")
	defer span.End()

	if d.ID == "" {
		d.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO notification_deliveries (id, channel_id, project_id, event_type, payload, status, max_attempts, next_retry_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		d.ID,
		d.ChannelID,
		d.ProjectID,
		d.EventType,
		d.Payload,
		d.Status,
		d.MaxAttempts,
		d.NextRetryAt,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create notification delivery: %w", err)
	}

	return nil
}

func (q *Queries) ListPendingNotificationDeliveries(ctx context.Context, limit int) ([]domain.NotificationDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListPendingNotificationDeliveries")
	defer span.End()

	query := `
		SELECT id, channel_id, project_id, event_type, payload, status, attempts, max_attempts,
		       last_error, next_retry_at, delivered_at, created_at, updated_at
		FROM notification_deliveries
		WHERE status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT $1`

	rows, err := q.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending notification deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.NotificationDelivery, 0, limit)
	for rows.Next() {
		var d domain.NotificationDelivery
		if err := rows.Scan(
			&d.ID, &d.ChannelID, &d.ProjectID, &d.EventType, &d.Payload,
			&d.Status, &d.Attempts, &d.MaxAttempts,
			&d.LastError, &d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list pending notification deliveries scan: %w", err)
		}
		deliveries = append(deliveries, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending notification deliveries rows: %w", err)
	}

	return deliveries, nil
}

func (q *Queries) UpdateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateNotificationDelivery")
	defer span.End()

	query := `
		UPDATE notification_deliveries
		SET status = $2, attempts = $3, last_error = $4, next_retry_at = $5, delivered_at = $6, updated_at = NOW()
		WHERE id = $1`

	_, err := q.db.Exec(ctx, query, d.ID, d.Status, d.Attempts, d.LastError, d.NextRetryAt, d.DeliveredAt)
	if err != nil {
		return fmt.Errorf("update notification delivery: %w", err)
	}

	return nil
}

func (q *Queries) ListNotificationDeliveries(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotificationDeliveries")
	defer span.End()

	var query string
	var args []any

	if cursor != nil {
		query = `
			SELECT id, channel_id, project_id, event_type, payload, status, attempts, max_attempts,
			       last_error, next_retry_at, delivered_at, created_at, updated_at
			FROM notification_deliveries
			WHERE project_id = $1 AND created_at < $2
			ORDER BY created_at DESC
			LIMIT $3`
		args = []any{projectID, *cursor, limit}
	} else {
		query = `
			SELECT id, channel_id, project_id, event_type, payload, status, attempts, max_attempts,
			       last_error, next_retry_at, delivered_at, created_at, updated_at
			FROM notification_deliveries
			WHERE project_id = $1
			ORDER BY created_at DESC
			LIMIT $2`
		args = []any{projectID, limit}
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list notification deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.NotificationDelivery, 0, limit)
	for rows.Next() {
		var d domain.NotificationDelivery
		if err := rows.Scan(
			&d.ID, &d.ChannelID, &d.ProjectID, &d.EventType, &d.Payload,
			&d.Status, &d.Attempts, &d.MaxAttempts,
			&d.LastError, &d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list notification deliveries scan: %w", err)
		}
		deliveries = append(deliveries, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification deliveries rows: %w", err)
	}

	return deliveries, nil
}

func (q *Queries) decryptNotificationConfig(channelID string, config []byte) []byte {
	if len(config) == 0 || q.secretEncryptionKey == "" {
		return config
	}
	enc, encErr := crypto.NewEncryptor(q.secretEncryptionKey)
	if encErr != nil {
		slog.Warn("failed to create encryptor for notification config decryption", "channel_id", channelID, "error", encErr)
		return config
	}
	decrypted, decErr := enc.Decrypt(config)
	if decErr != nil {
		slog.Warn("failed to decrypt notification config, returning raw bytes", "channel_id", channelID, "error", decErr)
		return config
	}
	return decrypted
}
