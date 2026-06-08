//go:build integration

package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storedDeviceCodeForTest(deviceCode string) string {
	sum := sha256.Sum256([]byte(deviceCode))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestCreateDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	userCode := "ABCD-1234"
	projectID := "project-device-code-create"
	scopes := []string{"read", "write"}
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, deviceCode,
		userCode,
		projectID, scopes,
		expiresAt,
	))

	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	require.NoError(t, err)
	require.Equal(t, deviceCode,

		got.
			DeviceCode)
	require.Equal(t, userCode,

		got.UserCode,
	)
	require.Equal(t, projectID,

		got.ProjectID,
	)
	require.Equal(t, "pending",

		got.Status,
	)
	require.Len(t, got.Scopes,

		2)

	var storedDeviceCode string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT device_code FROM cli_device_codes WHERE user_code = $1`,

		userCode,
	).Scan(&storedDeviceCode),
	)
	require.NotEqual(t, deviceCode,

		storedDeviceCode,
	)
	require.Equal(t, storedDeviceCodeForTest(deviceCode), storedDeviceCode)

}

func TestGetDeviceCodeByDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetDeviceCodeByDeviceCode(ctx, "nonexistent")
	require.True(t, errors.Is(err, store.
		ErrDeviceCodeNotFound,
	))

}

func TestGetDeviceCodeByUserCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	userCode := "USER-CODE"
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, deviceCode,
		userCode,
		"project-user-code",
		[]string{"read"}, expiresAt,
	))

	got, err := q.GetDeviceCodeByUserCode(ctx, userCode)
	require.NoError(t, err)
	require.Equal(t, userCode,

		got.UserCode,
	)
	require.NotEqual(t, deviceCode,

		got.
			DeviceCode,
	)
	require.Equal(t, storedDeviceCodeForTest(deviceCode), got.DeviceCode)
	require.Equal(t, "pending",

		got.Status,
	)

}

func TestGetDeviceCodeByUserCode_NotFoundForNonPendingRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)
	t.Cleanup(func() {
		mustClean(t, ctx)
	})

	approvedDeviceCode := newID()
	require.NoError(t, q.CreateDeviceCode(ctx, approvedDeviceCode,

		"USER-APPROVED",
		"project-approved-user-code",

		[]string{"read"}, time.Now().UTC().
			Add(10*time.Minute)))
	require.NoError(t, q.ApproveDeviceCode(ctx,
		approvedDeviceCode,
		newID(), "raw-key",
		"project-approved-user-code",

		[]string{"read"}))

	if _, err := q.GetDeviceCodeByUserCode(ctx, "USER-APPROVED"); !errors.Is(err, store.ErrDeviceCodeNotFound) {
		require.Failf(t, "test failure",

			"GetDeviceCodeByUserCode(approved) error = %v, want ErrDeviceCodeNotFound", err)
	}
	require.NoError(t, q.CreateDeviceCode(ctx, newID(), "USER-EXPIRED",
		"project-expired-user-code",

		[]string{"read"}, time.
			Now().UTC().Add(-time.Minute)))

	if _, err := q.GetDeviceCodeByUserCode(ctx, "USER-EXPIRED"); !errors.Is(err, store.ErrDeviceCodeNotFound) {
		require.Failf(t, "test failure",

			"GetDeviceCodeByUserCode(expired) error = %v, want ErrDeviceCodeNotFound", err)
	}
}

func TestApproveDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, deviceCode,
		"USER-1",
		"project-approve",
		[]string{"read"}, expiresAt,
	))

	apiKeyID := newID()
	rawAPIKey := "sk-test-raw-key"
	require.NoError(t, q.ApproveDeviceCode(ctx,
		deviceCode, apiKeyID,
		rawAPIKey,
		"project-approve",
		[]string{"read"}))

	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	require.NoError(t, err)
	require.Equal(t, "approved",

		got.
			Status)
	require.Equal(t, apiKeyID,

		got.APIKeyID,
	)
	require.Equal(t, rawAPIKey,

		got.RawAPIKey,
	)

	var storedRawAPIKey string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT raw_api_key FROM cli_device_codes WHERE user_code = $1`,

		"USER-1",
	).Scan(&storedRawAPIKey))
	require.NotEqual(t, rawAPIKey,

		storedRawAPIKey,
	)
	require.True(t, strings.HasPrefix(storedRawAPIKey,
		"enc:v1:"),
	)
	require.Equal(t, "project-approve",

		got.ProjectID,
	)
	require.False(t, len(got.
		Scopes) !=
		1 || got.
		Scopes[0] != "read",
	)

}

func TestApproveDeviceCodeByUserCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	userCode := "USER-APPROVE"
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, deviceCode,
		userCode,
		"project-approve-user-code",

		[]string{"read"}, expiresAt,
	))

	apiKeyID := newID()
	rawAPIKey := "sk-test-user-code-raw-key"
	require.NoError(t, q.ApproveDeviceCodeByUserCode(ctx, userCode,
		apiKeyID, rawAPIKey,
		"project-approve-user-code",

		[]string{"read", "runs:read"},
	))

	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	require.NoError(t, err)
	require.Equal(t, "approved",

		got.
			Status)
	require.Equal(t, apiKeyID,

		got.APIKeyID,
	)
	require.Equal(t, rawAPIKey,

		got.RawAPIKey,
	)

	var storedDeviceCode, storedRawAPIKey string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT device_code, raw_api_key FROM cli_device_codes WHERE user_code = $1`,

		userCode).Scan(&storedDeviceCode,

		&storedRawAPIKey))
	require.NotEqual(t, deviceCode,

		storedDeviceCode,
	)
	require.NotEqual(t, rawAPIKey,

		storedRawAPIKey,
	)
	require.True(t, strings.HasPrefix(storedRawAPIKey,
		"enc:v1:"),
	)

}

func TestApproveDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	err := q.ApproveDeviceCode(ctx, "nonexistent", newID(), "key", "project-missing", []string{"read"})
	require.True(t, errors.Is(err, store.
		ErrDeviceCodeNotFound,
	))

}

func TestApproveDeviceCodeByUserCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	err := q.ApproveDeviceCodeByUserCode(ctx, "missing-user-code", newID(), "key", "project-missing", []string{"read"})
	require.True(t, errors.Is(err, store.
		ErrDeviceCodeNotFound,
	))

}

func TestExchangeDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, deviceCode,
		"USER-2",
		"project-exchange",
		[]string{"write"}, expiresAt,
	))

	apiKeyID := newID()
	require.NoError(t, q.ApproveDeviceCode(ctx,
		deviceCode, apiKeyID,
		"raw-key",
		"project-exchange",
		[]string{"write"}))

	gotKeyID, err := q.ExchangeDeviceCode(ctx, deviceCode)
	require.NoError(t, err)
	require.Equal(t, apiKeyID,

		gotKeyID,
	)

	// After exchange, status should be 'used' and raw_api_key cleared.
	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	require.NoError(t, err)
	require.Equal(t, "used",

		got.Status,
	)
	require.Equal(t, "", got.
		RawAPIKey,
	)

	var storedRawAPIKey *string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT raw_api_key FROM cli_device_codes WHERE user_code = $1`,

		"USER-2",
	).Scan(&storedRawAPIKey))
	require.Nil(t, storedRawAPIKey)

}

func TestExchangeDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	_, err := q.ExchangeDeviceCode(ctx, "nonexistent")
	require.True(t, errors.Is(err, store.
		ErrDeviceCodeNotFound,
	))

}

func TestGetDeviceCodeByDeviceCode_RejectsPlaintextRawAPIKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO cli_device_codes (device_code, user_code, project_id, api_key_id, raw_api_key, status, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, 'approved', $6, $7)`,
		storedDeviceCodeForTest(deviceCode), "USER-PLAIN", "project-plaintext", newID(), "plaintext-live-key", []string{"read"}, time.Now().UTC().Add(10*time.Minute),
	)
	require.NoError(t, err)

	_, err = q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "not encrypted",
	)

}

func TestApproveDeviceCode_RequiresSecretEncryptionKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, deviceCode,
		"USER-NOKEY",
		"project-no-key",

		[]string{"read"}, expiresAt,
	))

	err := q.ApproveDeviceCode(ctx, deviceCode, newID(), "raw-key", "project-no-key", []string{"read"})
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "secret encryption key is not configured",
	)

	var storedRawAPIKey *string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT raw_api_key FROM cli_device_codes WHERE user_code = $1`,

		"USER-NOKEY",
	).Scan(&storedRawAPIKey),
	)
	require.Nil(t, storedRawAPIKey)

}

func TestCleanupExpiredDeviceCodes(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Create an already-expired device code.
	expired := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, newID(), "USER-3",
		"project-cleanup",
		[]string{}, expired,
	))

	// Create a non-expired device code.
	notExpired := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, q.CreateDeviceCode(ctx, newID(), "USER-4",
		"project-cleanup",
		[]string{}, notExpired,
	))

	deleted, err := q.CleanupExpiredDeviceCodes(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

}

// TestExchangeDeviceCode_ExpiredReturnsExpiredError guards the race-window
// classification: an approved code that expires before the atomic exchange must
// surface ErrDeviceCodeExpired (not ErrDeviceCodeNotFound), so the API can
// report expired_token rather than the misleading token_already_exchanged.
func TestExchangeDeviceCode_ExpiredReturnsExpiredError(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	require.NoError(t, q.CreateDeviceCode(ctx, deviceCode, "USER-EXP-EXCH",
		"project-exchange", []string{"write"}, time.Now().UTC().Add(10*time.Minute)))
	require.NoError(t, q.ApproveDeviceCode(ctx, deviceCode, newID(),
		"raw-key", "project-exchange", []string{"write"}))

	// Force the approved code to be expired, simulating expiry racing past the
	// handler's pre-check.
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE cli_device_codes SET expires_at = NOW() - interval '1 minute' WHERE user_code = $1`,
		"USER-EXP-EXCH")
	require.NoError(t, err)

	_, err = q.ExchangeDeviceCode(ctx, deviceCode)
	require.ErrorIs(t, err, store.ErrDeviceCodeExpired)
}
