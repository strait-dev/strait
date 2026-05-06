package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

func newSDKWaitEventTestServer(t *testing.T, s APIStore) *Server {
	t.Helper()
	cfg := &config.Config{
		JWTSigningKey:       testJWTSigningKey,
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
	}
	srv := NewServer(ServerDeps{
		Config: cfg,
		Store:  s,
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)
	return srv
}

func makeSDKRunToken(t *testing.T, runID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &runTokenClaims{
		Attempt: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "strait:run-token",
			Subject:   runID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(testJWTSigningKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestHandleSDKWaitForEvent_Success(t *testing.T) {
	t.Parallel()

	var createdTrigger *domain.EventTrigger
	var statusFrom, statusTo domain.RunStatus

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			statusFrom = from
			statusTo = to
			return nil
		},
		CreateEventTriggerFunc: func(_ context.Context, trigger *domain.EventTrigger) error {
			createdTrigger = trigger
			return nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"aml:app-123","timeout_secs":7200}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if statusFrom != domain.StatusExecuting || statusTo != domain.StatusWaiting {
		t.Fatalf("status transition = %s→%s, want executing→waiting", statusFrom, statusTo)
	}

	if createdTrigger == nil {
		t.Fatal("expected event trigger to be created")
	}
	if createdTrigger.EventKey != "aml:app-123" {
		t.Fatalf("event_key = %q, want %q", createdTrigger.EventKey, "aml:app-123")
	}
	if createdTrigger.SourceType != "job_run" {
		t.Fatalf("source_type = %q, want %q", createdTrigger.SourceType, "job_run")
	}
	if createdTrigger.JobRunID != "run-1" {
		t.Fatalf("job_run_id = %q, want %q", createdTrigger.JobRunID, "run-1")
	}
	if createdTrigger.TimeoutSecs != 7200 {
		t.Fatalf("timeout_secs = %d, want 7200", createdTrigger.TimeoutSecs)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "waiting" {
		t.Fatalf("response status = %v, want waiting", resp["status"])
	}
	if resp["event_key"] != "aml:app-123" {
		t.Fatalf("response event_key = %v, want aml:app-123", resp["event_key"])
	}
}

func TestHandleSDKWaitForEvent_RunNotExecuting(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1", Status: domain.StatusCompleted}, nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key"}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

func TestHandleSDKWaitForEvent_MissingEventKey(t *testing.T) {
	t.Parallel()

	srv := newSDKWaitEventTestServer(t, &APIStoreMock{})

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusUnprocessableEntity, rr.Body.String())
	}
}

func TestHandleSDKWaitForEvent_DefaultTimeout(t *testing.T) {
	t.Parallel()

	var createdTrigger *domain.EventTrigger

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		CreateEventTriggerFunc: func(_ context.Context, trigger *domain.EventTrigger) error {
			createdTrigger = trigger
			return nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key"}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if createdTrigger.TimeoutSecs != domain.DefaultEventTimeoutSecs {
		t.Fatalf("timeout_secs = %d, want %d", createdTrigger.TimeoutSecs, domain.DefaultEventTimeoutSecs)
	}
}

func TestHandleSDKWaitForEvent_RejectsTimeoutAboveMaximum(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(context.Context, string) (*domain.JobRun, error) {
			t.Fatal("timeout must be rejected before loading run")
			return nil, nil
		},
		CreateEventTriggerFunc: func(context.Context, *domain.EventTrigger) error {
			t.Fatal("timeout above maximum must not create an event trigger")
			return nil
		},
	}
	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key","timeout_secs":2592001}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestHandleSDKWaitForEvent_RunNotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key"}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
