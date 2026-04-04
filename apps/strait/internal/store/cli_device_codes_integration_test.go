//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/store"
)

func TestCreateDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
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

func TestApproveDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, deviceCode, "USER-1", "project-approve", []string{"read"}, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	apiKeyID := newID()
	rawAPIKey := "sk-test-raw-key"
	if err := q.ApproveDeviceCode(ctx, deviceCode, apiKeyID, rawAPIKey); err != nil {
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
}

func TestApproveDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ApproveDeviceCode(ctx, "nonexistent", newID(), "key")
	if !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("ApproveDeviceCode(notfound) error = %v, want ErrDeviceCodeNotFound", err)
	}
}

func TestExchangeDeviceCode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	deviceCode := newID()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := q.CreateDeviceCode(ctx, deviceCode, "USER-2", "project-exchange", []string{"write"}, expiresAt); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	apiKeyID := newID()
	if err := q.ApproveDeviceCode(ctx, deviceCode, apiKeyID, "raw-key"); err != nil {
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
}

func TestExchangeDeviceCode_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.ExchangeDeviceCode(ctx, "nonexistent")
	if !errors.Is(err, store.ErrDeviceCodeNotFound) {
		t.Fatalf("ExchangeDeviceCode(notfound) error = %v, want ErrDeviceCodeNotFound", err)
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
