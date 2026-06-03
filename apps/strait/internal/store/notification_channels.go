package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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

	configBytes := append([]byte(nil), ch.Config...)
	if q.secretEncryptionKey != "" && len(configBytes) > 0 {
		enc, encErr := q.secretEncryptor()
		if encErr != nil {
			return fmt.Errorf("create notification channel config encryptor: %w", encErr)
		}
		encrypted, encryptErr := enc.Encrypt(configBytes)
		if encryptErr != nil {
			return fmt.Errorf("encrypt notification channel config: %w", encryptErr)
		}
		configBytes = encrypted
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

// CreateNotificationChannelWithProjectLimit serializes project-scoped channel
// quota enforcement with row creation.
func (q *Queries) CreateNotificationChannelWithProjectLimit(ctx context.Context, ch *domain.NotificationChannel, maxChannels int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotificationChannelWithProjectLimit")
	defer span.End()

	if maxChannels < 0 {
		return q.CreateNotificationChannel(ctx, ch)
	}

	if _, ok := TxFromContext(ctx); ok {
		return q.createNotificationChannelWithProjectLimitLocked(ctx, ch, maxChannels)
	}
	if _, ok := q.db.(pgx.Tx); ok {
		return q.createNotificationChannelWithProjectLimitLocked(ctx, ch, maxChannels)
	}
	if _, ok := q.db.(TxBeginner); !ok {
		return q.createNotificationChannelWithProjectLimitLocked(ctx, ch, maxChannels)
	}

	return q.withTx(ctx, func(txq *Queries) error {
		return txq.createNotificationChannelWithProjectLimitLocked(ctx, ch, maxChannels)
	})
}

func (q *Queries) createNotificationChannelWithProjectLimitLocked(ctx context.Context, ch *domain.NotificationChannel, maxChannels int) error {
	if err := q.acquirePlanLimitLock(ctx, "notification_channel_limit:"+ch.ProjectID); err != nil {
		return err
	}

	count, err := q.CountNotificationChannelsByProject(ctx, ch.ProjectID)
	if err != nil {
		return fmt.Errorf("count notification channels before create: %w", err)
	}
	if count >= maxChannels {
		return ErrNotificationChannelLimitExceeded
	}

	return q.CreateNotificationChannel(ctx, ch)
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

// ListEnabledNotificationChannelsByProjectIDs fetches all enabled channels for
// multiple projects in a single query, returning them grouped by project_id.
// This eliminates N+1 queries in notification dispatch loops.
func (q *Queries) ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEnabledNotificationChannelsByProjectIDs")
	defer span.End()

	if len(projectIDs) == 0 {
		return map[string][]domain.NotificationChannel{}, nil
	}

	query := `
		SELECT id, project_id, channel_type, name, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE project_id = ANY($1) AND enabled = true
		ORDER BY project_id, created_at DESC`

	rows, err := q.db.Query(ctx, query, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("list enabled channels by project IDs: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]domain.NotificationChannel, len(projectIDs))
	for rows.Next() {
		var ch domain.NotificationChannel
		if err := rows.Scan(
			&ch.ID, &ch.ProjectID, &ch.ChannelType, &ch.Name,
			&ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list enabled channels by project IDs scan: %w", err)
		}
		ch.Config = q.decryptNotificationConfig(ch.ID, ch.Config)
		result[ch.ProjectID] = append(result[ch.ProjectID], ch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list enabled channels by project IDs rows: %w", err)
	}

	return result, nil
}

func (q *Queries) UpdateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateNotificationChannel")
	defer span.End()

	configBytes := append([]byte(nil), ch.Config...)
	if q.secretEncryptionKey != "" && len(configBytes) > 0 {
		enc, encErr := q.secretEncryptor()
		if encErr != nil {
			return fmt.Errorf("update notification channel config encryptor: %w", encErr)
		}
		encrypted, encryptErr := enc.Encrypt(configBytes)
		if encryptErr != nil {
			return fmt.Errorf("encrypt notification channel config: %w", encryptErr)
		}
		configBytes = encrypted
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
		INSERT INTO notification_deliveries (id, channel_id, project_id, event_type, payload, status, max_attempts, next_retry_at, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''))
		ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL AND dedupe_key <> '' DO NOTHING
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
		d.DedupeKey,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) && d.DedupeKey != "" {
			return nil
		}
		return fmt.Errorf("create notification delivery: %w", err)
	}

	return nil
}

func scanNotificationDelivery(scanner scanTarget, includeClaimFields bool) (*domain.NotificationDelivery, error) {
	var d domain.NotificationDelivery
	var lastError *string

	if includeClaimFields {
		if err := scanner.Scan(
			&d.ID, &d.ChannelID, &d.ProjectID, &d.EventType, &d.Payload,
			&d.Status, &d.Attempts, &d.MaxAttempts,
			&lastError, &d.NextRetryAt, &d.DeliveredAt, &d.ClaimToken, &d.LeaseExpiry, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
	} else {
		if err := scanner.Scan(
			&d.ID, &d.ChannelID, &d.ProjectID, &d.EventType, &d.Payload,
			&d.Status, &d.Attempts, &d.MaxAttempts,
			&lastError, &d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
	}

	if lastError != nil {
		d.LastError = *lastError
	}

	return &d, nil
}

func (q *Queries) ClaimPendingNotificationDeliveries(ctx context.Context, limit int, leaseDuration time.Duration) ([]domain.NotificationDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimPendingNotificationDeliveries")
	defer span.End()

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("claim pending notification deliveries: db does not support transactions")
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("claim pending notification deliveries: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	claimToken := uuid.Must(uuid.NewV7()).String()
	leaseExpiry := time.Now().UTC().Add(leaseDuration)

	query := `
		WITH claimable AS (
			SELECT id
			FROM notification_deliveries
			WHERE (
				status = 'pending'
				AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			) OR (
				status = 'processing'
				AND lease_expires_at IS NOT NULL
				AND lease_expires_at <= NOW()
			)
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE notification_deliveries d
		SET status = 'processing',
		    claim_token = $2,
		    lease_expires_at = $3,
		    updated_at = NOW()
		FROM claimable
		WHERE d.id = claimable.id
		RETURNING d.id, d.channel_id, d.project_id, d.event_type, d.payload, d.status, d.attempts, d.max_attempts,
		          d.last_error, d.next_retry_at, d.delivered_at, d.claim_token, d.lease_expires_at, d.created_at, d.updated_at`

	rows, err := tx.Query(ctx, query, limit, claimToken, leaseExpiry)
	if err != nil {
		return nil, fmt.Errorf("claim pending notification deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.NotificationDelivery, 0, limit)
	for rows.Next() {
		d, err := scanNotificationDelivery(rows, true)
		if err != nil {
			return nil, fmt.Errorf("claim pending notification deliveries scan: %w", err)
		}
		deliveries = append(deliveries, *d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim pending notification deliveries rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("claim pending notification deliveries: commit tx: %w", err)
	}

	return deliveries, nil
}

func (q *Queries) UpdateClaimedNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateClaimedNotificationDelivery")
	defer span.End()

	query := `
		UPDATE notification_deliveries
		SET status = $3,
		    attempts = $4,
		    last_error = $5,
		    next_retry_at = $6,
		    delivered_at = $7,
		    claim_token = NULL,
		    lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND claim_token = $2`

	tag, err := q.db.Exec(ctx, query, d.ID, d.ClaimToken, d.Status, d.Attempts, d.LastError, d.NextRetryAt, d.DeliveredAt)
	if err != nil {
		return false, fmt.Errorf("update claimed notification delivery: %w", err)
	}

	return tag.RowsAffected() > 0, nil
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
		d, err := scanNotificationDelivery(rows, false)
		if err != nil {
			return nil, fmt.Errorf("list notification deliveries scan: %w", err)
		}
		deliveries = append(deliveries, *d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification deliveries rows: %w", err)
	}

	return deliveries, nil
}

func (q *Queries) decryptNotificationConfig(channelID string, config []byte) []byte {
	if len(config) == 0 || q.secretEncryptionKey == "" {
		return append([]byte(nil), config...)
	}
	enc, encErr := q.secretEncryptor()
	if encErr != nil {
		slog.Warn("failed to create encryptor for notification config decryption", "channel_id", channelID, "error", encErr)
		return config
	}
	decrypted, decErr := enc.Decrypt(config)
	if decErr != nil {
		slog.Warn("failed to decrypt notification config, returning raw bytes", "channel_id", channelID, "error", decErr)
		return append([]byte(nil), config...)
	}
	return decrypted
}
