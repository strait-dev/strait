package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"
)

// ---------------------------------------------------------------------------.
// 1. projectContextMiddleware
// ---------------------------------------------------------------------------.

// mockProjectContextSetter implements ProjectContextSetter for testing.
type mockProjectContextSetter struct {
	APIStore
	setErr   error
	clearErr error
	setCalls int
}

func (m *mockProjectContextSetter) SetProjectContext(_ context.Context, _ string) error {
	m.setCalls++
	return m.setErr
}

func (m *mockProjectContextSetter) ClearProjectContext(_ context.Context) error {
	return m.clearErr
}

func TestProjectContextMiddleware_NoProjectID(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{APIStore: &APIStoreMock{}}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No project ID in context -- middleware should still call next.
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("expected next handler to be called when no project ID is set")
	}
	if setter.setCalls != 0 {
		t.Fatalf("expected SetProjectContext to not be called, got %d calls", setter.setCalls)
	}
}

func TestProjectContextMiddleware_WithProjectID(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{APIStore: &APIStoreMock{}}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("expected next handler to be called")
	}
	if setter.setCalls != 1 {
		t.Fatalf("expected SetProjectContext to be called once, got %d", setter.setCalls)
	}
}

func TestProjectContextMiddleware_SetError(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{
		APIStore: &APIStoreMock{},
		setErr:   errors.New("db connection lost"),
	}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	handler.ServeHTTP(w, r)

	// Even on SetProjectContext error, the middleware should still call next.
	if !called {
		t.Fatal("expected next handler to be called even when SetProjectContext fails")
	}
}

func TestProjectContextMiddleware_ClearError(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{
		APIStore: &APIStoreMock{},
		clearErr: errors.New("clear failed"),
	}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	// Should not panic even when ClearProjectContext returns an error.
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("expected next handler to be called")
	}
}

func TestProjectContextMiddleware_StoreDoesNotImplementSetter(t *testing.T) {
	t.Parallel()
	// When the store does not implement ProjectContextSetter, the middleware
	// should pass through directly (return next).
	srv := &Server{store: &APIStoreMock{}}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("expected next handler to be called when store does not implement ProjectContextSetter")
	}
}

// ---------------------------------------------------------------------------.
// 2. requestMetrics
// ---------------------------------------------------------------------------.

func TestRequestMetrics_NilMetrics(t *testing.T) {
	t.Parallel()
	srv := &Server{metrics: nil}

	called := false
	handler := srv.requestMetrics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("expected next handler to be called when metrics is nil")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRequestMetrics_RecordsStatusOnSuccess(t *testing.T) {
	t.Parallel()
	// With nil metrics, the middleware skips instrumentation but still serves.
	// This verifies the pass-through path works correctly and records the
	// right status code on the wrapped response writer.
	srv := &Server{metrics: nil}

	handler := srv.requestMetrics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestRequestMetrics_RecordsErrorStatus(t *testing.T) {
	t.Parallel()
	srv := &Server{metrics: nil}

	handler := srv.requestMetrics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------.
// 3. normalizeAPIError
// ---------------------------------------------------------------------------.

func TestNormalizeAPIError_StringInput(t *testing.T) {
	t.Parallel()
	result := normalizeAPIError(http.StatusBadRequest, "bad input")
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "bad input" {
		t.Fatalf("expected 'bad input', got %q", s)
	}
}

func TestNormalizeAPIError_EmptyString(t *testing.T) {
	t.Parallel()
	result := normalizeAPIError(http.StatusNotFound, "")
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "Not Found" {
		t.Fatalf("expected 'Not Found', got %q", s)
	}
}

func TestNormalizeAPIError_NilError(t *testing.T) {
	t.Parallel()
	var err error
	result := normalizeAPIError(http.StatusInternalServerError, err)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "Internal Server Error" {
		t.Fatalf("expected 'Internal Server Error', got %q", s)
	}
}

func TestNormalizeAPIError_NonNilError(t *testing.T) {
	t.Parallel()
	result := normalizeAPIError(http.StatusBadRequest, errors.New("field missing"))
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "field missing" {
		t.Fatalf("expected 'field missing', got %q", s)
	}
}

func TestNormalizeAPIError_APIErrorValue(t *testing.T) {
	t.Parallel()
	ae := APIError{Code: "custom_code", Message: "custom message"}
	result := normalizeAPIError(http.StatusBadRequest, ae)
	got, ok := result.(APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", result)
	}
	if got.Code != "custom_code" {
		t.Fatalf("expected code=custom_code, got %q", got.Code)
	}
	if got.Message != "custom message" {
		t.Fatalf("expected message=custom message, got %q", got.Message)
	}
}

func TestNormalizeAPIError_APIErrorEmptyCode(t *testing.T) {
	t.Parallel()
	ae := APIError{Message: "some message"}
	result := normalizeAPIError(http.StatusNotFound, ae)
	got, ok := result.(APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", result)
	}
	if got.Code != ErrorCodeNotFound {
		t.Fatalf("expected code=%q, got %q", ErrorCodeNotFound, got.Code)
	}
}

func TestNormalizeAPIError_APIErrorEmptyMessage(t *testing.T) {
	t.Parallel()
	ae := APIError{Code: "custom"}
	result := normalizeAPIError(http.StatusForbidden, ae)
	got, ok := result.(APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", result)
	}
	if got.Message != "Forbidden" {
		t.Fatalf("expected message=Forbidden, got %q", got.Message)
	}
}

func TestNormalizeAPIError_APIErrorPointer(t *testing.T) {
	t.Parallel()
	ae := &APIError{Code: "ptr_code", Message: "ptr_msg"}
	result := normalizeAPIError(http.StatusConflict, ae)
	got, ok := result.(APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", result)
	}
	if got.Code != "ptr_code" || got.Message != "ptr_msg" {
		t.Fatalf("unexpected APIError: %+v", got)
	}
}

func TestNormalizeAPIError_NilAPIErrorPointer(t *testing.T) {
	t.Parallel()
	var ae *APIError
	result := normalizeAPIError(http.StatusBadRequest, ae)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "Bad Request" {
		t.Fatalf("expected 'Bad Request', got %q", s)
	}
}

func TestNormalizeAPIError_UnknownType(t *testing.T) {
	t.Parallel()
	result := normalizeAPIError(http.StatusTeapot, 42)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "I'm a teapot" {
		t.Fatalf("expected 'I'm a teapot', got %q", s)
	}
}

func TestNormalizeAPIError_WrappedError(t *testing.T) {
	t.Parallel()
	inner := errors.New("root cause")
	wrapped := fmt.Errorf("outer: %w", inner)
	result := normalizeAPIError(http.StatusInternalServerError, wrapped)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "outer: root cause" {
		t.Fatalf("expected 'outer: root cause', got %q", s)
	}
}

func TestNormalizeAPIError_JoinedErrors(t *testing.T) {
	t.Parallel()
	joined := errors.Join(errors.New("err1"), errors.New("err2"))
	result := normalizeAPIError(http.StatusBadRequest, joined)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(s, "err1") || !strings.Contains(s, "err2") {
		t.Fatalf("expected both errors in output, got %q", s)
	}
}

// ---------------------------------------------------------------------------.
// 4. validateTriggerRequest (dry-run validation)
// ---------------------------------------------------------------------------.

func newTriggerTestServer(t *testing.T, ms *APIStoreMock) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	var p pubsub.Publisher
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Queue:   &mockQueue{},
		PubSub:  p,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestValidateTriggerRequest_Valid(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{Payload: json.RawMessage(`{"key":"value"}`)}
	result, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Job.ID != "job-1" {
		t.Fatalf("expected job ID job-1, got %q", result.Job.ID)
	}
}

func TestValidateTriggerRequest_EmptyJobID(t *testing.T) {
	t.Parallel()
	srv := newTriggerTestServer(t, &APIStoreMock{})

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "", req)
	if err == nil {
		t.Fatal("expected error for empty job ID")
	}
}

func TestValidateTriggerRequest_WhitespaceJobID(t *testing.T) {
	t.Parallel()
	srv := newTriggerTestServer(t, &APIStoreMock{})

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "   ", req)
	if err == nil {
		t.Fatal("expected error for whitespace-only job ID")
	}
}

func TestValidateTriggerRequest_JobNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-missing", req)
	if err == nil {
		t.Fatal("expected error for missing job")
	}
}

func TestValidateTriggerRequest_JobDisabled(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: false}, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	if err == nil {
		t.Fatal("expected error for disabled job")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected 'disabled' in error, got %q", err.Error())
	}
}

func TestValidateTriggerRequest_JobPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, Paused: true}, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	if err == nil {
		t.Fatal("expected error for paused job")
	}
	if !strings.Contains(err.Error(), "paused") {
		t.Fatalf("expected 'paused' in error, got %q", err.Error())
	}
}

func TestValidateTriggerRequest_PayloadTooLarge(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	// maxPayloadSize is 5MB; generate something larger.
	largePayload := `{"data":"` + strings.Repeat("x", 6*1024*1024) + `"}`
	req := TriggerRequest{Payload: json.RawMessage(largePayload)}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected 'too large' in error, got %q", err.Error())
	}
}

func TestValidateTriggerRequest_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, errors.New("database unavailable")
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	if err == nil {
		t.Fatal("expected error for store failure")
	}
}

func TestValidateTriggerRequest_QuotaExceeded(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 30}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxQueuedRuns: 5}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	if err == nil {
		t.Fatal("expected error for quota exceeded")
	}
	if !strings.Contains(err.Error(), "quota") {
		t.Fatalf("expected 'quota' in error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------.
// 5. handleCreateProject
// ---------------------------------------------------------------------------.

func TestHandleCreateProject_StoreError_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, _ *domain.Project) error {
			return errors.New("connection refused")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"Test Project"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateProject_DuplicateName_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, _ *domain.Project) error {
			return errors.New("duplicate key value violates unique constraint")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-dup","org_id":"org-1","name":"Duplicate"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))

	// Without special duplicate handling, the store error maps to 500.
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateProject_ProjectLimitExceeded_Adversarial(t *testing.T) {
	t.Parallel()
	limitErr := &billing.LimitError{
		Code:         "project_limit_exceeded",
		Message:      "project limit reached",
		CurrentUsage: 5,
		Limit:        5,
		Plan:         "free",
		UpgradeURL:   "https://example.com/upgrade",
	}

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}

	ms := &APIStoreMock{
		// CreateProject should not be called when limit check fails.
		CreateProjectFunc: func(_ context.Context, _ *domain.Project) error {
			t.Fatal("CreateProject should not be called when limit is exceeded")
			return nil
		},
	}

	enforcer := &adversarialBillingEnforcer{
		checkProjectLimitFn: func(_ context.Context, _ string) error {
			return limitErr
		},
	}

	srv := NewServer(ServerDeps{
		Config:          cfg,
		Store:           ms,
		Queue:           &mockQueue{},
		Edition:         domain.EditionCloud,
		BillingEnforcer: enforcer,
	})
	t.Cleanup(srv.Close)

	body := `{"id":"proj-new","org_id":"org-1","name":"Over Limit"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// The error body should contain the limit error message.
	errBody, ok := resp["error"]
	if !ok {
		t.Fatal("expected 'error' key in response")
	}
	errStr, ok := errBody.(string)
	if !ok {
		t.Fatalf("expected error to be a string, got %T", errBody)
	}
	if errStr != "project limit reached" {
		t.Fatalf("expected 'project limit reached', got %q", errStr)
	}
}

func TestHandleCreateProject_InvalidBody_Adversarial(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", `{invalid json`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateProject_EmptyBody_Adversarial(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateProject_ForbiddenForAPIKeyAuth_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"My Project"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/projects/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	// Simulate a request with scopes set (API key auth) via internal secret + context.
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{"projects:write"})
	r = r.WithContext(ctx)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------.
// 6. handleDeleteSecret
// ---------------------------------------------------------------------------.

func TestHandleDeleteSecret_Success_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteJobSecretFunc: func(_ context.Context, id string) error {
			if id != "sec-123" {
				t.Fatalf("unexpected secret ID: %q", id)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/secrets/sec-123", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSecret_NotFound_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteJobSecretFunc: func(_ context.Context, _ string) error {
			return store.ErrJobSecretNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/secrets/sec-missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSecret_StoreError_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteJobSecretFunc: func(_ context.Context, _ string) error {
			return errors.New("unexpected IO error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/secrets/sec-err", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------.
// Additional edge cases for normalizeAPIError / defaultErrorCode
// ---------------------------------------------------------------------------.

func TestDefaultErrorCode_AllStatusCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, ErrorCodeValidationError},
		{http.StatusUnauthorized, ErrorCodeUnauthorized},
		{http.StatusForbidden, ErrorCodeForbidden},
		{http.StatusNotFound, ErrorCodeNotFound},
		{http.StatusConflict, ErrorCodeConflict},
		{http.StatusTooManyRequests, ErrorCodeRateLimited},
		{http.StatusInternalServerError, ErrorCodeInternalError},
		{http.StatusServiceUnavailable, ErrorCodeInternalError},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			t.Parallel()
			code := defaultErrorCode(tc.status)
			if code != tc.want {
				t.Fatalf("status %d: expected %q, got %q", tc.status, tc.want, code)
			}
		})
	}
}

func TestNormalizeAPIError_APIErrorPointerEmptyCodeAndMessage(t *testing.T) {
	t.Parallel()
	ae := &APIError{}
	result := normalizeAPIError(http.StatusConflict, ae)
	got, ok := result.(APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", result)
	}
	if got.Code != ErrorCodeConflict {
		t.Fatalf("expected code=%q, got %q", ErrorCodeConflict, got.Code)
	}
	if got.Message != "Conflict" {
		t.Fatalf("expected message=Conflict, got %q", got.Message)
	}
}

// ---------------------------------------------------------------------------.
// Cross-org access via requireProjectMatch
// ---------------------------------------------------------------------------.

func TestRequireProjectMatch_SameProject(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	err := requireProjectMatch(ctx, "proj-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRequireProjectMatch_DifferentProject(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	err := requireProjectMatch(ctx, "proj-2")
	if !errors.Is(err, errProjectMismatch) {
		t.Fatalf("expected errProjectMismatch, got %v", err)
	}
}

func TestRequireProjectMatch_NoProjectContext(t *testing.T) {
	t.Parallel()
	// Internal callers without project context should pass through.
	err := requireProjectMatch(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected no error for internal caller, got %v", err)
	}
}

// ---------------------------------------------------------------------------.
// ScheduledAt validation via trigger handler
// ---------------------------------------------------------------------------.

func TestTriggerJob_ScheduledAtInThePast(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"scheduled_at":"%s"}`, pastTime)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body, "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for past scheduled_at, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTriggerJob_ScheduledAtFarFuture(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	futureTime := time.Now().Add(60 * 24 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"scheduled_at":"%s"}`, futureTime)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body, "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for far-future scheduled_at, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------.
// Helper: mock billing enforcer
// ---------------------------------------------------------------------------.

// adversarialBillingEnforcer satisfies the BillingEnforcer interface for tests.
type adversarialBillingEnforcer struct {
	checkProjectLimitFn     func(ctx context.Context, orgID string) error
	ensureOrgSubscriptionFn func(ctx context.Context, orgID string) error
}

func (m *adversarialBillingEnforcer) CheckProjectLimit(ctx context.Context, orgID string) error {
	if m.checkProjectLimitFn != nil {
		return m.checkProjectLimitFn(ctx, orgID)
	}
	return nil
}

func (m *adversarialBillingEnforcer) CheckMemberLimit(_ context.Context, _ string) error {
	return nil
}

func (m *adversarialBillingEnforcer) CheckOrgCreationLimit(_ context.Context, _ string, _ domain.PlanTier) error {
	return nil
}

func (m *adversarialBillingEnforcer) CheckProjectBudgetLimit(_ context.Context, _ string) error {
	return nil
}

func (m *adversarialBillingEnforcer) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *adversarialBillingEnforcer) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *adversarialBillingEnforcer) GetOrgPlanLimits(_ context.Context, _ string) (billing.OrgPlanLimits, error) {
	return billing.OrgPlanLimits{}, nil
}

func (m *adversarialBillingEnforcer) GetDailyRunCount(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *adversarialBillingEnforcer) EnsureOrgSubscription(ctx context.Context, orgID string) error {
	if m.ensureOrgSubscriptionFn != nil {
		return m.ensureOrgSubscriptionFn(ctx, orgID)
	}
	return nil
}

