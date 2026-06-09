package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ErrDeviceCodeNotFound is returned when a device code lookup finds no rows.
var ErrDeviceCodeNotFound = errors.New("device code not found")

// ErrDeviceCodeExpired is returned by ExchangeDeviceCode when the atomic
// exchange matched no row specifically because the code expired (rather than
// having already been exchanged), so callers can report the accurate error.
var ErrDeviceCodeExpired = errors.New("device code expired")

const encryptedDeviceAPIKeyPrefix = "enc:v1:"
const hashedDeviceCodePrefix = "sha256:"

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

	storedDeviceCode := hashDeviceCode(deviceCode)
	query := `
		INSERT INTO cli_device_codes (device_code, user_code, project_id, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := q.db.Exec(ctx, query, storedDeviceCode, userCode, projectID, scopes, expiresAt)
	if err != nil {
		return fmt.Errorf("create device code: %w", err)
	}
	return nil
}

// GetDeviceCodeByDeviceCode looks up a device code row by its device_code value.
func (q *Queries) GetDeviceCodeByDeviceCode(ctx context.Context, deviceCode string) (*DeviceCodeRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetDeviceCodeByDeviceCode")
	defer span.End()

	storedDeviceCode := hashDeviceCode(deviceCode)
	query := `
		SELECT id, device_code, user_code, project_id,
		       COALESCE(api_key_id, ''), COALESCE(raw_api_key, ''),
		       status, scopes, expires_at, created_at
		FROM cli_device_codes
		WHERE device_code = $1`

	row := &DeviceCodeRow{}
	err := q.db.QueryRow(ctx, query, storedDeviceCode).Scan(
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
	row.DeviceCode = deviceCode
	if row.RawAPIKey != "" {
		rawAPIKey, decryptErr := q.decryptDeviceAPIKey(row.RawAPIKey)
		if decryptErr != nil {
			return nil, fmt.Errorf("get device code: %w", decryptErr)
		}
		row.RawAPIKey = rawAPIKey
	}
	return row, nil
}

// GetDeviceCodeByUserCode looks up a pending browser approval row by user_code.
func (q *Queries) GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*DeviceCodeRow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetDeviceCodeByUserCode")
	defer span.End()

	query := `
		SELECT id, device_code, user_code, project_id,
		       COALESCE(api_key_id, ''), COALESCE(raw_api_key, ''),
		       status, scopes, expires_at, created_at
		FROM cli_device_codes
		WHERE user_code = $1 AND status = 'pending' AND expires_at > NOW()`

	row := &DeviceCodeRow{}
	err := q.db.QueryRow(ctx, query, userCode).Scan(
		&row.ID, &row.DeviceCode, &row.UserCode, &row.ProjectID,
		&row.APIKeyID, &row.RawAPIKey,
		&row.Status, &row.Scopes, &row.ExpiresAt, &row.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeviceCodeNotFound
		}
		return nil, fmt.Errorf("get device code by user code: %w", err)
	}
	if row.RawAPIKey != "" {
		rawAPIKey, decryptErr := q.decryptDeviceAPIKey(row.RawAPIKey)
		if decryptErr != nil {
			return nil, fmt.Errorf("get device code by user code: %w", decryptErr)
		}
		row.RawAPIKey = rawAPIKey
	}
	return row, nil
}

// ApproveDeviceCode transitions a device code from pending to approved,
// sets the api_key_id, and stores the raw API key for later retrieval.
func (q *Queries) ApproveDeviceCode(ctx context.Context, deviceCode, apiKeyID, rawAPIKey, projectID string, scopes []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ApproveDeviceCode")
	defer span.End()

	encryptedAPIKey, err := q.encryptDeviceAPIKey(rawAPIKey)
	if err != nil {
		return err
	}

	query := `
		UPDATE cli_device_codes
		SET status = 'approved', api_key_id = $2, raw_api_key = $3, project_id = $4, scopes = $5
		WHERE device_code = $1 AND status = 'pending' AND expires_at > NOW()`

	tag, err := q.db.Exec(ctx, query, hashDeviceCode(deviceCode), apiKeyID, encryptedAPIKey, projectID, scopes)
	if err != nil {
		return fmt.Errorf("approve device code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceCodeNotFound
	}
	return nil
}

// ApproveDeviceCodeByUserCode approves a pending device flow by its user_code.
// The browser approval flow must not receive the secret polling device_code.
func (q *Queries) ApproveDeviceCodeByUserCode(ctx context.Context, userCode, apiKeyID, rawAPIKey, projectID string, scopes []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ApproveDeviceCodeByUserCode")
	defer span.End()

	encryptedAPIKey, err := q.encryptDeviceAPIKey(rawAPIKey)
	if err != nil {
		return err
	}

	query := `
		UPDATE cli_device_codes
		SET status = 'approved', api_key_id = $2, raw_api_key = $3, project_id = $4, scopes = $5
		WHERE user_code = $1 AND status = 'pending' AND expires_at > NOW()`

	tag, err := q.db.Exec(ctx, query, userCode, apiKeyID, encryptedAPIKey, projectID, scopes)
	if err != nil {
		return fmt.Errorf("approve device code by user code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceCodeNotFound
	}
	return nil
}

func (q *Queries) encryptDeviceAPIKey(rawAPIKey string) (string, error) {
	if rawAPIKey == "" {
		return "", nil
	}
	key, err := q.secretKey()
	if err != nil {
		return "", fmt.Errorf("encrypt device api key: %w", err)
	}
	ciphertext, err := encryptSecret(rawAPIKey, key)
	if err != nil {
		return "", fmt.Errorf("encrypt device api key: %w", err)
	}
	return encryptedDeviceAPIKeyPrefix + ciphertext, nil
}

func (q *Queries) decryptDeviceAPIKey(storedAPIKey string) (string, error) {
	if storedAPIKey == "" {
		return "", nil
	}
	if !strings.HasPrefix(storedAPIKey, encryptedDeviceAPIKeyPrefix) {
		return "", fmt.Errorf("stored device api key is not encrypted")
	}
	ciphertext := strings.TrimPrefix(storedAPIKey, encryptedDeviceAPIKeyPrefix)
	plaintext, err := q.decryptSecretWithFallback(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt device api key: %w", err)
	}
	return plaintext, nil
}

// ExchangeDeviceCode atomically transitions an approved device code to used
// and returns the api_key_id. The raw_api_key is cleared on exchange.
func (q *Queries) ExchangeDeviceCode(ctx context.Context, deviceCode string) (string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ExchangeDeviceCode")
	defer span.End()

	query := `
		UPDATE cli_device_codes
		SET status = 'used', raw_api_key = NULL
		WHERE device_code = $1 AND status = 'approved' AND expires_at > NOW()
		RETURNING api_key_id`

	var apiKeyID string
	err := q.db.QueryRow(ctx, query, hashDeviceCode(deviceCode)).Scan(&apiKeyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The atomic UPDATE matched nothing: the code either expired between
			// the caller's check and now, or was already exchanged/never existed.
			// Diagnose so the caller can return the accurate error instead of a
			// misleading "already exchanged".
			return "", q.classifyFailedDeviceCodeExchange(ctx, deviceCode)
		}
		return "", fmt.Errorf("exchange device code: %w", err)
	}
	return apiKeyID, nil
}

// classifyFailedDeviceCodeExchange determines why an exchange UPDATE matched no
// row: an expired (but still-present) code yields ErrDeviceCodeExpired; anything
// else (already used, or gone) yields ErrDeviceCodeNotFound.
func (q *Queries) classifyFailedDeviceCodeExchange(ctx context.Context, deviceCode string) error {
	var expiresAt time.Time
	var status string
	err := q.db.QueryRow(ctx,
		`SELECT expires_at, status FROM cli_device_codes WHERE device_code = $1`,
		hashDeviceCode(deviceCode),
	).Scan(&expiresAt, &status)
	if err != nil {
		return ErrDeviceCodeNotFound
	}
	if status == "approved" && !time.Now().After(expiresAt) {
		// Still approved and unexpired: a concurrent exchange won the race.
		return ErrDeviceCodeNotFound
	}
	if time.Now().After(expiresAt) {
		return ErrDeviceCodeExpired
	}
	return ErrDeviceCodeNotFound
}

func hashDeviceCode(deviceCode string) string {
	sum := sha256.Sum256([]byte(deviceCode))
	return hashedDeviceCodePrefix + hex.EncodeToString(sum[:])
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
