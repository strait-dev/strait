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

	"github.com/stretchr/testify/require"
)

func TestCreateAPIKey_ExpiresInOverflowRejected(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 365)
	w := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":1000000}`)
	require.False(t, w.Code != http.
		StatusUnprocessableEntity &&
		w.Code !=
			http.StatusBadRequest,
	)
}

func TestCreateAPIKey_ExpiresInBoundary(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, maxAPIKeyDurationDays+1)
	wOK := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":36500}`)
	require.Equal(t, http.StatusCreated,

		wOK.Code)

	wTooMuch := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":36501}`)
	require.False(t, wTooMuch.Code !=
		http.
			StatusUnprocessableEntity &&
		wTooMuch.
			Code !=
			http.
				StatusBadRequest)
}

func TestCreateAPIKey_RotationIntervalOverflowRejected(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 365)
	w := createKeyRequest(t, srv,
		`{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30,"rotation_interval_days":1000000}`)
	require.False(t, w.Code != http.
		StatusUnprocessableEntity &&
		w.Code !=
			http.StatusBadRequest,
	)
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
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.False(t, captured == nil ||
		captured.
			ExpiresAt ==
			nil)
	require.False(t, captured.ExpiresAt.
		Before(now))

	earliest := now.Add(364 * 24 * time.Hour)
	latest := now.Add(366 * 24 * time.Hour)
	require.False(t, captured.ExpiresAt.
		Before(earliest) || captured.ExpiresAt.
		After(
			latest))

	var resp CreateAPIKeyResponse
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&resp))

	_ = fmt.Sprintf("%v", resp)
}
