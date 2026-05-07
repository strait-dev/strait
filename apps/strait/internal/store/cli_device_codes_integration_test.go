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

	if err := q.CreateDeviceCode(ctx, deviceCode, userCode, projectID, scopes, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByDeviceCode() error = %v", err)
	}
	if got.DeviceCode != deviceCode {
		t.Fatalf("DeviceCode = %q, want %q", got.DeviceCode, deviceCode)
	}
	if got.UserCode != userCode {
		t.Fatalf("UserCode = %q, want %q", got.UserCode, userCode)
	}
	if got.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.Status != "pending" {
		t.Fatalf("Status = %q, want pending", got.Status)
	}
	if len(got.Scopes) != 2 {
		t.Fatalf("Scopes len = %d, want 2", len(got.Scopes))
	}
	var storedDeviceCode string
	if err := testDB.Pool.QueryRow(ctx, `SELECT device_code FROM cli_device_codes WHERE user_code = $1`, userCode).Scan(&storedDeviceCode); err != nil {
		t.Fatalf("query stored device_code: %v", err)
	}
	if storedDeviceCode == deviceCode {
		t.Fatal("device_code was stored in plaintext")
	}
	if storedDeviceCode != storedDeviceCodeForTest(deviceCode) {
		t.Fatalf("stored device_code = %q, want hashed value", storedDeviceCode)
	}
}

func TestGetDeviceCodeByDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetDeviceCodeByDeviceCode(ctx, "nonexistent")
	if !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("GetDeviceCodeByDeviceCode() error = %v, want ErrDeviceCodeNotFound", err)
	}
}

func TestGetDeviceCodeByUserCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	userCode := "USER-CODE"
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, deviceCode, userCode, "project-user-code", []string{"read"}, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	got, err := q.GetDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByUserCode() error = %v", err)
	}
	if got.UserCode != userCode {
		t.Fatalf("UserCode = %q, want %q", got.UserCode, userCode)
	}
	if got.DeviceCode == deviceCode {
		t.Fatal("GetDeviceCodeByUserCode exposed the raw device code")
	}
	if got.DeviceCode != storedDeviceCodeForTest(deviceCode) {
		t.Fatalf("DeviceCode = %q, want hashed stored value", got.DeviceCode)
	}
	if got.Status != "pending" {
		t.Fatalf("Status = %q, want pending", got.Status)
	}
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
	if err := q.CreateDeviceCode(ctx, approvedDeviceCode, "USER-APPROVED", "project-approved-user-code", []string{"read"}, time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceCode(approved) error = %v", err)
	}
	if err := q.ApproveDeviceCode(ctx, approvedDeviceCode, newID(), "raw-key", "project-approved-user-code", []string{"read"}); err != nil {
		t.Fatalf("ApproveDeviceCode() error = %v", err)
	}
	if _, err := q.GetDeviceCodeByUserCode(ctx, "USER-APPROVED"); !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("GetDeviceCodeByUserCode(approved) error = %v, want ErrDeviceCodeNotFound", err)
	}

	if err := q.CreateDeviceCode(ctx, newID(), "USER-EXPIRED", "project-expired-user-code", []string{"read"}, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateDeviceCode(expired) error = %v", err)
	}
	if _, err := q.GetDeviceCodeByUserCode(ctx, "USER-EXPIRED"); !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("GetDeviceCodeByUserCode(expired) error = %v, want ErrDeviceCodeNotFound", err)
	}
}

func TestApproveDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, deviceCode, "USER-1", "project-approve", []string{"read"}, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	apiKeyID := newID()
	rawAPIKey := "sk-test-raw-key"
	if err := q.ApproveDeviceCode(ctx, deviceCode, apiKeyID, rawAPIKey, "project-approve", []string{"read"}); err != nil {
		t.Fatalf("ApproveDeviceCode() error = %v", err)
	}

	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByDeviceCode() error = %v", err)
	}
	if got.Status != "approved" {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	if got.APIKeyID != apiKeyID {
		t.Fatalf("APIKeyID = %q, want %q", got.APIKeyID, apiKeyID)
	}
	if got.RawAPIKey != rawAPIKey {
		t.Fatalf("RawAPIKey = %q, want %q", got.RawAPIKey, rawAPIKey)
	}
	var storedRawAPIKey string
	if err := testDB.Pool.QueryRow(ctx, `SELECT raw_api_key FROM cli_device_codes WHERE user_code = $1`, "USER-1").Scan(&storedRawAPIKey); err != nil {
		t.Fatalf("query stored raw_api_key: %v", err)
	}
	if storedRawAPIKey == rawAPIKey {
		t.Fatal("raw_api_key was stored in plaintext")
	}
	if !strings.HasPrefix(storedRawAPIKey, "enc:v1:") {
		t.Fatalf("stored raw_api_key prefix = %q, want enc:v1", storedRawAPIKey)
	}
	if got.ProjectID != "project-approve" {
		t.Fatalf("ProjectID = %q, want project-approve", got.ProjectID)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "read" {
		t.Fatalf("Scopes = %#v, want [read]", got.Scopes)
	}
}

func TestApproveDeviceCodeByUserCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	userCode := "USER-APPROVE"
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, deviceCode, userCode, "project-approve-user-code", []string{"read"}, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	apiKeyID := newID()
	rawAPIKey := "sk-test-user-code-raw-key"
	if err := q.ApproveDeviceCodeByUserCode(ctx, userCode, apiKeyID, rawAPIKey, "project-approve-user-code", []string{"read", "runs:read"}); err != nil {
		t.Fatalf("ApproveDeviceCodeByUserCode() error = %v", err)
	}

	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByDeviceCode() error = %v", err)
	}
	if got.Status != "approved" {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	if got.APIKeyID != apiKeyID {
		t.Fatalf("APIKeyID = %q, want %q", got.APIKeyID, apiKeyID)
	}
	if got.RawAPIKey != rawAPIKey {
		t.Fatalf("RawAPIKey = %q, want %q", got.RawAPIKey, rawAPIKey)
	}

	var storedDeviceCode, storedRawAPIKey string
	if err := testDB.Pool.QueryRow(ctx, `SELECT device_code, raw_api_key FROM cli_device_codes WHERE user_code = $1`, userCode).Scan(&storedDeviceCode, &storedRawAPIKey); err != nil {
		t.Fatalf("query stored device code approval: %v", err)
	}
	if storedDeviceCode == deviceCode {
		t.Fatal("ApproveDeviceCodeByUserCode left device_code in plaintext")
	}
	if storedRawAPIKey == rawAPIKey {
		t.Fatal("ApproveDeviceCodeByUserCode stored raw_api_key in plaintext")
	}
	if !strings.HasPrefix(storedRawAPIKey, "enc:v1:") {
		t.Fatalf("stored raw_api_key prefix = %q, want enc:v1", storedRawAPIKey)
	}
}

func TestApproveDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	err := q.ApproveDeviceCode(ctx, "nonexistent", newID(), "key", "project-missing", []string{"read"})
	if !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("ApproveDeviceCode(notfound) error = %v, want ErrDeviceCodeNotFound", err)
	}
}

func TestApproveDeviceCodeByUserCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	err := q.ApproveDeviceCodeByUserCode(ctx, "missing-user-code", newID(), "key", "project-missing", []string{"read"})
	if !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("ApproveDeviceCodeByUserCode(notfound) error = %v, want ErrDeviceCodeNotFound", err)
	}
}

func TestExchangeDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, deviceCode, "USER-2", "project-exchange", []string{"write"}, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	apiKeyID := newID()
	if err := q.ApproveDeviceCode(ctx, deviceCode, apiKeyID, "raw-key", "project-exchange", []string{"write"}); err != nil {
		t.Fatalf("ApproveDeviceCode() error = %v", err)
	}

	gotKeyID, err := q.ExchangeDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode() error = %v", err)
	}
	if gotKeyID != apiKeyID {
		t.Fatalf("ExchangeDeviceCode() = %q, want %q", gotKeyID, apiKeyID)
	}

	// After exchange, status should be 'used' and raw_api_key cleared.
	got, err := q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByDeviceCode() error = %v", err)
	}
	if got.Status != "used" {
		t.Fatalf("Status = %q, want used", got.Status)
	}
	if got.RawAPIKey != "" {
		t.Fatalf("RawAPIKey = %q, want empty", got.RawAPIKey)
	}
	var storedRawAPIKey *string
	if err := testDB.Pool.QueryRow(ctx, `SELECT raw_api_key FROM cli_device_codes WHERE user_code = $1`, "USER-2").Scan(&storedRawAPIKey); err != nil {
		t.Fatalf("query stored raw_api_key after exchange: %v", err)
	}
	if storedRawAPIKey != nil {
		t.Fatalf("stored raw_api_key after exchange = %q, want NULL", *storedRawAPIKey)
	}
}

func TestExchangeDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-device-flow-key")
	mustClean(t, ctx)

	_, err := q.ExchangeDeviceCode(ctx, "nonexistent")
	if !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("ExchangeDeviceCode(notfound) error = %v, want ErrDeviceCodeNotFound", err)
	}
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
	if err != nil {
		t.Fatalf("insert plaintext device code: %v", err)
	}

	_, err = q.GetDeviceCodeByDeviceCode(ctx, deviceCode)
	if err == nil || !strings.Contains(err.Error(), "not encrypted") {
		t.Fatalf("GetDeviceCodeByDeviceCode() error = %v, want not encrypted", err)
	}
}

func TestApproveDeviceCode_RequiresSecretEncryptionKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, deviceCode, "USER-NOKEY", "project-no-key", []string{"read"}, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	err := q.ApproveDeviceCode(ctx, deviceCode, newID(), "raw-key", "project-no-key", []string{"read"})
	if err == nil || !strings.Contains(err.Error(), "secret encryption key is not configured") {
		t.Fatalf("ApproveDeviceCode() error = %v, want missing encryption key", err)
	}

	var storedRawAPIKey *string
	if err := testDB.Pool.QueryRow(ctx, `SELECT raw_api_key FROM cli_device_codes WHERE user_code = $1`, "USER-NOKEY").Scan(&storedRawAPIKey); err != nil {
		t.Fatalf("query stored raw_api_key: %v", err)
	}
	if storedRawAPIKey != nil {
		t.Fatalf("stored raw_api_key = %q, want NULL when approval fails", *storedRawAPIKey)
	}
}

func TestCleanupExpiredDeviceCodes(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Create an already-expired device code.
	expired := time.Now().UTC().Add(-1 * time.Minute)
	if err := q.CreateDeviceCode(ctx, newID(), "USER-3", "project-cleanup", []string{}, expired); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	// Create a non-expired device code.
	notExpired := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, newID(), "USER-4", "project-cleanup", []string{}, notExpired); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	deleted, err := q.CleanupExpiredDeviceCodes(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredDeviceCodes() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("CleanupExpiredDeviceCodes() = %d, want 1", deleted)
	}
}
