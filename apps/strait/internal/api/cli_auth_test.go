package api

import (
	"context"
	"encoding/json"
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
	var approvedDeviceCode string
	var approvedAPIKeyID string

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
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-generated"
			key.CreatedAt = time.Now()
			createdKey = key
			return nil
		},
		ApproveDeviceCodeFunc: func(_ context.Context, deviceCode, apiKeyID, _ string, projectID string, scopes []string) error {
			approvedDeviceCode = deviceCode
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

	body := `{"device_code":"test-device-code","project_id":"proj-1"}`
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
	if approvedDeviceCode != "test-device-code" {
		t.Fatalf("expected device code to be approved, got %q", approvedDeviceCode)
	}
	if approvedAPIKeyID != "key-generated" {
		t.Fatalf("expected api key id key-generated, got %q", approvedAPIKeyID)
	}
}

func TestHandleApproveDeviceCode_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return nil, store.ErrDeviceCodeNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"device_code":"nonexistent","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleApproveDeviceCode_LimitedCallerCannotGrantCLIScopes(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByDeviceCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
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
		DeviceCode: "test-device-code",
		ProjectID:  "proj-1",
	}})
	if err == nil {
		t.Fatal("expected approval to fail when caller cannot grant CLI scopes")
	}
}
