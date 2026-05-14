package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateAPIKey_ExpiresInOverflowRejected(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 365)
	w := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":1000000}`)

	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/422; body: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_ExpiresInBoundary(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, maxAPIKeyDurationDays+1)
	wOK := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":36500}`)
	if wOK.Code != http.StatusCreated {
		t.Fatalf("36500 status = %d, want 201; body: %s", wOK.Code, wOK.Body.String())
	}

	wTooMuch := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":36501}`)
	if wTooMuch.Code != http.StatusUnprocessableEntity && wTooMuch.Code != http.StatusBadRequest {
		t.Fatalf("36501 status = %d, want 400/422; body: %s", wTooMuch.Code, wTooMuch.Body.String())
	}
}

func TestCreateAPIKey_RotationIntervalOverflowRejected(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 365)
	w := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30,"rotation_interval_days":1000000}`)
	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400/422; body: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_QuotaMaxKeyLifetimeClampedDoesNotWrap(t *testing.T) {
	t.Parallel()

	// quota.MaxKeyLifetimeDays is set absurdly high; the server must clamp
	// it to maxAPIKeyDurationDays before computing time.Now().Add(days*24h),
	// so the resulting ExpiresAt is in the future, not in the past.
	var captured *domain.APIKey
	ms := &APIStoreMock{
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{
				ProjectID:          "proj-1",
				MaxKeyLifetimeDays: 1_000_000_000,
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			captured = key
			key.ID = "key-x"
			key.CreatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	now := time.Now()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/",
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":365}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if captured == nil || captured.ExpiresAt == nil {
		t.Fatal("expected expires_at to be persisted")
	}
	if captured.ExpiresAt.Before(now) {
		t.Fatalf("expires_at %v is in the past — quota clamp likely wrapped", captured.ExpiresAt)
	}
	earliest := now.Add(364 * 24 * time.Hour)
	latest := now.Add(366 * 24 * time.Hour)
	if captured.ExpiresAt.Before(earliest) || captured.ExpiresAt.After(latest) {
		t.Fatalf("expires_at %v is not ~365 days out", captured.ExpiresAt)
	}

	var resp CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	_ = fmt.Sprintf("%v", resp)
}
