package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ErrDeviceCodeNotFound is returned when a device code lookup finds no rows.
var ErrDeviceCodeNotFound = errors.New("device code not found")

// DeviceCodeRow represents a row from the cli_device_codes table.
type DeviceCodeRow struct {
	ID         string
	DeviceCode string
	UserCode   string
	ProjectID  string
	APIKeyID   string
	RawAPIKey  string
	Status     string
	Scopes     []string
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

// CreateDeviceCode inserts a new device code record.
func (q *Queries) CreateDeviceCode(ctx context.Context, deviceCode, userCode, projectID string, scopes []string, expiresAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateDeviceCode")
	defer span.End()

	query := `
		INSERT INTO cli_device_codes (device_code, user_code, project_id, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := q.db.Exec(ctx, query, deviceCode, userCode, projectID, scopes, expiresAt)
	if err != nil {
		return fmt.Errorf("create device code: %w", err)
	}
	return nil
}

// GetDeviceCodeByDeviceCode looks up a device code row by its device_code value.
func (q *Queries) GetDeviceCodeByDeviceCode(ctx context.Context, deviceCode string) (*DeviceCodeRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetDeviceCodeByDeviceCode")
	defer span.End()

	query := `
		SELECT id, device_code, user_code, project_id,
		       COALESCE(api_key_id, ''), COALESCE(raw_api_key, ''),
		       status, scopes, expires_at, created_at
		FROM cli_device_codes
		WHERE device_code = $1`

	row := &DeviceCodeRow{}
	err := q.db.QueryRow(ctx, query, deviceCode).Scan(
		&row.ID, &row.DeviceCode, &row.UserCode, &row.ProjectID,
		&row.APIKeyID, &row.RawAPIKey,
		&row.Status, &row.Scopes, &row.ExpiresAt, &row.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeviceCodeNotFound
		}
		return nil, fmt.Errorf("get device code: %w", err)
	}
	return row, nil
}

// ApproveDeviceCode transitions a device code from pending to approved,
// sets the api_key_id, and stores the raw API key for later retrieval.
func (q *Queries) ApproveDeviceCode(ctx context.Context, deviceCode, apiKeyID, rawAPIKey string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ApproveDeviceCode")
	defer span.End()

	query := `
		UPDATE cli_device_codes
		SET status = 'approved', api_key_id = $2, raw_api_key = $3
		WHERE device_code = $1 AND status = 'pending' AND expires_at > NOW()`

	tag, err := q.db.Exec(ctx, query, deviceCode, apiKeyID, rawAPIKey)
	if err != nil {
		return fmt.Errorf("approve device code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceCodeNotFound
	}
	return nil
}

// ExchangeDeviceCode atomically transitions an approved device code to used
// and returns the api_key_id. The raw_api_key is cleared on exchange.
func (q *Queries) ExchangeDeviceCode(ctx context.Context, deviceCode string) (string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ExchangeDeviceCode")
	defer span.End()

	query := `
		UPDATE cli_device_codes
		SET status = 'used', raw_api_key = NULL
		WHERE device_code = $1 AND status = 'approved'
		RETURNING api_key_id`

	var apiKeyID string
	err := q.db.QueryRow(ctx, query, deviceCode).Scan(&apiKeyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrDeviceCodeNotFound
		}
		return "", fmt.Errorf("exchange device code: %w", err)
	}
	return apiKeyID, nil
}

// CleanupExpiredDeviceCodes deletes device codes that have passed their expiration time.
func (q *Queries) CleanupExpiredDeviceCodes(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CleanupExpiredDeviceCodes")
	defer span.End()

	query := `DELETE FROM cli_device_codes WHERE expires_at < NOW()`

	tag, err := q.db.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired device codes: %w", err)
	}
	return tag.RowsAffected(), nil
}
