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
)

func TestHandleDeviceCode_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/cli/auth/device-code", nil)
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp deviceCodeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.DeviceCode == "" {
		t.Fatal("expected non-empty device_code")
	}
	if len(resp.DeviceCode) != 64 {
		t.Fatalf("expected 64-char hex device_code, got %d chars", len(resp.DeviceCode))
	}
	if resp.UserCode == "" {
		t.Fatal("expected non-empty user_code")
	}
	if len(resp.UserCode) != 8 {
		t.Fatalf("expected 8-char user_code, got %d chars", len(resp.UserCode))
	}
	if resp.ExpiresIn <= 0 {
		t.Fatal("expected positive expires_in")
	}
	if resp.Interval <= 0 {
		t.Fatal("expected positive interval")
	}
	if resp.VerificationURL == "" {
		t.Fatal("expected non-empty verification_url")
	}
}

func TestDeviceCodePollIntervalFitsPublicRouteRateLimit(t *testing.T) {
	t.Parallel()

	if deviceCodePollInterval <= 0 {
		t.Fatal("deviceCodePollInterval must be positive")
	}
	pollsPerWindow := 1 // the initial /device-code request shares the same public route bucket.
	pollsPerWindow += int(cliAuthRateLimitWindow / (time.Duration(deviceCodePollInterval) * time.Second))
	if cliAuthRateLimitWindow%(time.Duration(deviceCodePollInterval)*time.Second) != 0 {
		pollsPerWindow++
	}
	if pollsPerWindow > cliAuthRateLimitRequests {
		t.Fatalf("advertised polling interval allows %d polls per window, route limit is %d", pollsPerWindow, cliAuthRateLimitRequests)
	}
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

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &codes[i]); err != nil {
			t.Fatalf("request %d: invalid JSON: %v", i, err)
		}
	}

	if codes[0].DeviceCode == codes[1].DeviceCode {
		t.Fatal("expected different device_codes")
	}
	if codes[0].UserCode == codes[1].UserCode {
		t.Fatal("expected different user_codes")
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !cleanupCalled {
		t.Fatal("CleanupExpiredDeviceCodes was not called")
	}
	if !createSawCleanup {
		t.Fatal("CreateDeviceCode ran before expired code cleanup")
	}
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

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if createCalled {
		t.Fatal("CreateDeviceCode must not run when expired code cleanup fails")
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected authorization_pending 400, got %d: %s", w.Code, w.Body.String())
	}
	if !cleanupCalled {
		t.Fatal("CleanupExpiredDeviceCodes was not called")
	}
	if !lookupSawCleanup {
		t.Fatal("GetDeviceCodeByDeviceCode ran before expired code cleanup")
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "authorization_pending" {
		t.Fatalf("expected authorization_pending error, got %q", resp["error"])
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp deviceTokenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.APIKey != "strait_testapikey1234567890abcdef" {
		t.Fatalf("expected raw api key, got %q", resp.APIKey)
	}
	if resp.ProjectID != "proj-1" {
		t.Fatalf("expected project_id proj-1, got %q", resp.ProjectID)
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "expired_token" {
		t.Fatalf("expected expired_token error, got %q", resp["error"])
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "token_already_exchanged" {
		t.Fatalf("expected token_already_exchanged error, got %q", resp["error"])
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleApproveDeviceCode_Success(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey
	var approvedUserCode string
	var approvedAPIKeyID string

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, userCode string) (*store.DeviceCodeRow, error) {
			if userCode != "ABCD1234" {
				t.Fatalf("lookup user_code = %q, want ABCD1234", userCode)
			}
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
			if projectID != "proj-1" {
				t.Fatalf("approved project_id = %q, want proj-1", projectID)
			}
			if len(scopes) == 0 {
				t.Fatal("approved scopes should not be empty")
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"user_code":"ABCD1234","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if createdKey == nil {
		t.Fatal("expected API key to be created")
	}
	if createdKey.ProjectID != "proj-1" {
		t.Fatalf("expected project_id proj-1, got %q", createdKey.ProjectID)
	}
	if approvedUserCode != "ABCD1234" {
		t.Fatalf("expected user code to be approved, got %q", approvedUserCode)
	}
	if approvedAPIKeyID != "key-generated" {
		t.Fatalf("expected api key id key-generated, got %q", approvedAPIKeyID)
	}
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
			if projectID != "proj-1" {
				t.Fatalf("quota project_id = %q, want proj-1", projectID)
			}
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
	if err != nil {
		t.Fatalf("handleApproveDeviceCode() error = %v", err)
	}
	if createdKey == nil {
		t.Fatal("expected API key to be created")
	}
	if createdKey.EnvironmentID != "env-staging" {
		t.Fatalf("created key environment_id = %q, want env-staging", createdKey.EnvironmentID)
	}
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
	if err != nil {
		t.Fatalf("handleApproveDeviceCode() error = %v", err)
	}
	if createdKey == nil || createdKey.ExpiresAt == nil {
		t.Fatal("expected CLI key to be created with an expiry")
	}
	if createdKey.ExpiresAt.After(time.Now().Add(8 * 24 * time.Hour)) {
		t.Fatalf("created key expiry = %v, want capped near 7 days", createdKey.ExpiresAt)
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleApproveDeviceCode_RejectsDeviceCodeOnly(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(context.Context, string) (*store.DeviceCodeRow, error) {
			t.Fatal("device-code-only approval must not reach the store")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"secret-device-code","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error, got %d: %s", w.Code, w.Body.String())
	}
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
			t.Fatal("api key should not be created when caller cannot grant CLI scopes")
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
	if err == nil {
		t.Fatal("expected approval to fail when caller cannot grant CLI scopes")
	}
}
