package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleDeviceCode_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/device-code", nil)
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp deviceCodeResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.NotEqual(t, "", resp.DeviceCode)
	require.Len(t,
		resp.DeviceCode,
		64)
	require.NotEqual(t, "", resp.UserCode)
	require.Len(t,
		resp.UserCode,
		8)
	require.False(t, resp.ExpiresIn <=
		0)
	require.False(t, resp.Interval <=
		0)
	require.NotEqual(t, "", resp.VerificationURL)

}

func TestDeviceCodePollIntervalFitsPublicRouteRateLimit(t *testing.T) {
	t.Parallel()
	require.False(t, deviceCodePollInterval <=
		0)

	pollsPerWindow := 1 // the initial /device-code request shares the same public route bucket.
	pollsPerWindow += int(cliAuthRateLimitWindow / (time.Duration(deviceCodePollInterval) * time.Second))
	if cliAuthRateLimitWindow%(time.Duration(deviceCodePollInterval)*time.Second) != 0 {
		pollsPerWindow++
	}
	require.LessOrEqual(t, pollsPerWindow,
		cliAuthRateLimitRequests,
	)

}

func TestHandleDeviceCode_CodesAreUnique(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Make two requests and verify the codes differ.
	var codes [2]deviceCodeResponse
	for i := range codes {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/device-code", nil)
		r.Header.Set("Content-Type", "application/json")

		srv.ServeHTTP(w, r)
		require.Equal(t, http.StatusOK,
			w.Code)
		require.NoError(t, json.Unmarshal(w.Body.
			Bytes(), &codes[i]))

	}
	require.NotEqual(t, codes[1].DeviceCode,

		codes[0].DeviceCode,
	)
	require.NotEqual(t, codes[1].UserCode,
		codes[0].UserCode,
	)

}

func TestHandleDeviceCode_CleansExpiredCodesBeforeCreate(t *testing.T) {
	t.Parallel()

	var cleanupCalled bool
	var createSawCleanup bool
	ms := &APIStoreMock{
		CleanupExpiredDeviceCodesFunc: func(context.Context) (int64, error) {
			cleanupCalled = true
			return 3, nil
		},
		CreateDeviceCodeFunc: func(_ context.Context, _, _ string, _ string, _ []string, _ time.Time) error {
			createSawCleanup = cleanupCalled
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/device-code", nil)
	r.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, cleanupCalled)
	require.True(
		t, createSawCleanup,
	)

}

func TestHandleDeviceCode_CleanupFailurePreventsCreate(t *testing.T) {
	t.Parallel()

	var createCalled bool
	ms := &APIStoreMock{
		CleanupExpiredDeviceCodesFunc: func(context.Context) (int64, error) {
			return 0, errors.New("cleanup unavailable")
		},
		CreateDeviceCodeFunc: func(context.Context, string, string, string, []string, time.Time) error {
			createCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/device-code", nil)
	r.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
	require.False(t, createCalled)

}

func TestHandleDeviceToken_CleansExpiredCodesBeforeLookup(t *testing.T) {
	t.Parallel()

	var cleanupCalled bool
	var lookupSawCleanup bool
	ms := &APIStoreMock{
		CleanupExpiredDeviceCodesFunc: func(context.Context) (int64, error) {
			cleanupCalled = true
			return 1, nil
		},
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			lookupSawCleanup = cleanupCalled
			return &store.DeviceCodeRow{
				ID:        "dc-1",
				Status:    "pending",
				ExpiresAt: time.Now().Add(10 * time.Minute),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"test-device-code","grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.True(
		t, cleanupCalled)
	require.True(
		t, lookupSawCleanup,
	)

}

func TestHandleDeviceToken_Pending(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", nil)
	r.Header.Set("Content-Type", "application/json")

	body := `{"device_code":"test-device-code","grant_type":"device_code"}`
	r = httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "authorization_pending",

		resp["error"])

}

func TestHandleDeviceToken_Approved(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				ProjectID:  "proj-1",
				APIKeyID:   "key-1",
				RawAPIKey:  "strait_testapikey1234567890abcdef",
				Status:     "approved",
				Scopes:     []string{},
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		ExchangeDeviceCodeFunc: func(_ context.Context, _ string) (string, error) {
			return "key-1", nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"test-device-code","grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp deviceTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "strait_testapikey1234567890abcdef",

		resp.APIKey)
	require.Equal(t, "proj-1", resp.
		ProjectID,
	)

}

func TestHandleDeviceToken_Expired(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(-1 * time.Minute),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"test-device-code","grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "expired_token",
		resp["error"])

}

func TestHandleDeviceToken_AlreadyUsed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				Status:     "used",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"test-device-code","grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "token_already_exchanged",

		resp["error"])

}

func TestHandleDeviceToken_InvalidGrantType(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"test-device-code","grant_type":"authorization_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleDeviceToken_MissingDeviceCode(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"grant_type":"device_code"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleApproveDeviceCode_Success(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey
	var approvedUserCode string
	var approvedAPIKeyID string

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, userCode string) (*store.DeviceCodeRow, error) {
			require.Equal(t, "ABCD1234", userCode)

			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-generated"
			key.CreatedAt = time.Now()
			createdKey = key
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, userCode, apiKeyID, _ string, projectID string, scopes []string) error {
			approvedUserCode = userCode
			approvedAPIKeyID = apiKeyID
			require.Equal(t, "proj-1", projectID)
			require.NotEmpty(t, scopes)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"user_code":"ABCD1234","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotNil(t, createdKey)
	require.Equal(t, "proj-1", createdKey.
		ProjectID,
	)
	require.Equal(t, "ABCD1234", approvedUserCode)
	require.Equal(t, "key-generated",
		approvedAPIKeyID,
	)

}

func TestHandleApproveDeviceCode_EnvironmentScopedCallerCreatesEnvironmentScopedCLIKey(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey
	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-env",
				DeviceCode: "test-device-code",
				UserCode:   "ENV12345",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			require.Equal(t, "proj-1", projectID)

			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-env"
			key.CreatedAt = time.Now()
			createdKey = key
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")
	ctx = context.WithValue(ctx, ctxScopesKey, domain.CLIDefaultScopes)

	_, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "ABCD1234",
		ProjectID: "proj-1",
	}})
	require.NoError(t, err)
	require.NotNil(t, createdKey)
	require.Equal(t, "env-staging",
		createdKey.
			EnvironmentID,
	)

}

func TestHandleApproveDeviceCode_AppliesProjectMaxKeyLifetime(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey
	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-lifetime",
				DeviceCode: "test-device-code",
				UserCode:   "LIFE1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxKeyLifetimeDays: 7}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-lifetime"
			key.CreatedAt = time.Now()
			createdKey = key
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, domain.CLIDefaultScopes)

	_, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "ABCD1234",
		ProjectID: "proj-1",
	}})
	require.NoError(t, err)
	require.False(t, createdKey ==
		nil || createdKey.
		ExpiresAt ==
		nil)
	require.False(t, createdKey.ExpiresAt.
		After(time.Now().Add(8*24*time.
			Hour)))

}

func TestHandleApproveDeviceCode_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return nil, store.ErrDeviceCodeNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"user_code":"nonexistent","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

}

func TestHandleApproveDeviceCode_RejectsDeviceCodeOnly(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(context.Context, string) (*store.DeviceCodeRow, error) {
			require.Fail(t,

				"device-code-only approval must not reach the store")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"secret-device-code","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)
	require.False(t, w.Code != http.
		StatusUnprocessableEntity &&
		w.Code != http.
			StatusBadRequest,
	)

}

func TestHandleApproveDeviceCode_LimitedCallerCannotGrantCLIScopes(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "USER-1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		CreateAPIKeyFunc: func(context.Context, *domain.APIKey) error {
			require.Fail(t,

				"api key should not be created when caller cannot grant CLI scopes")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	_, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "ABCD1234",
		ProjectID: "proj-1",
	}})
	require.Error(t, err)

}
