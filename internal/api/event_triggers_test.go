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

	"strait/internal/config"
	"strait/internal/domain"
)

func newEventTriggersTestServer(t *testing.T, s APIStore, wfCallback WorkflowCallback) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "test-jwt-key-must-be-32-chars-long",
	}
	return NewServer(ServerDeps{
		Config:           cfg,
		Store:            s,
		Queue:            &mockQueue{},
		PubSub:           &mockPublisher{},
		WorkflowCallback: wfCallback,
	})
}

func TestHandleSendEvent_Success(t *testing.T) {
	t.Parallel()

	var updatedStatus string
	var updatedPayload json.RawMessage

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			if key == "aml-check:app-123" {
				return &domain.EventTrigger{
					ID:                "evt-1",
					EventKey:          "aml-check:app-123",
					ProjectID:         "proj-1",
					SourceType:        "workflow_step",
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-1",
					Status:            domain.EventTriggerStatusWaiting,
				}, nil
			}
			return nil, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, status string, payload json.RawMessage, _ *time.Time, _ string) error {
			updatedStatus = status
			updatedPayload = payload
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}

	wfCallback := &mockWorkflowTrigger{}
	srv := newEventTriggersTestServer(t, ms, wfCallback)

	body := `{"payload":{"result":"approved"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/aml-check:app-123/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if updatedStatus != domain.EventTriggerStatusReceived {
		t.Fatalf("trigger status = %q, want %q", updatedStatus, domain.EventTriggerStatusReceived)
	}

	var result map[string]any
	if err := json.Unmarshal(updatedPayload, &result); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	if result["result"] != "approved" {
		t.Fatalf("payload result = %v, want approved", result["result"])
	}
}

func TestHandleSendEvent_NotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/nonexistent/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestHandleSendEvent_AlreadyReceived(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:       "evt-1",
				EventKey: "some-key",
				Status:   domain.EventTriggerStatusReceived,
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/some-key/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

func TestHandleSendEvent_StoreError(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/some-key/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleGetEventTrigger_Success(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:          "evt-1",
				EventKey:    key,
				ProjectID:   "proj-1",
				SourceType:  "workflow_step",
				Status:      domain.EventTriggerStatusWaiting,
				RequestedAt: now,
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/aml-check:app-123", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var trigger domain.EventTrigger
	if err := json.NewDecoder(rr.Body).Decode(&trigger); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if trigger.EventKey != "aml-check:app-123" {
		t.Fatalf("event_key = %q, want %q", trigger.EventKey, "aml-check:app-123")
	}
}

func TestHandleGetEventTrigger_NotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/nonexistent", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandleListEventTriggers_Success(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := &mockAPIStore{
		listEventTriggersByProjectFn: func(_ context.Context, projectID string, _ string, _ int, _ *time.Time) ([]domain.EventTrigger, error) {
			if projectID == "proj-1" {
				return []domain.EventTrigger{
					{ID: "evt-1", EventKey: "aml:app-1", ProjectID: "proj-1", Status: domain.EventTriggerStatusWaiting, RequestedAt: now},
					{ID: "evt-2", EventKey: "aml:app-2", ProjectID: "proj-1", Status: domain.EventTriggerStatusReceived, RequestedAt: now},
				}, nil
			}
			return nil, nil
		},
		getAPIKeyByHashFn: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-1"}, nil
		},
		touchAPIKeyLastUsedFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleSendEvent_EmptyBody(t *testing.T) {
	t.Parallel()

	var capturedTrigger *domain.EventTrigger

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-1",
				EventKey:   "my-event",
				ProjectID:  "proj-1",
				SourceType: "job_run",
				JobRunID:   "run-1",
				Status:     domain.EventTriggerStatusWaiting,
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}

	wfCallback := &mockWorkflowTrigger{
		onEventReceivedFn: func(_ context.Context, trigger *domain.EventTrigger) error {
			capturedTrigger = trigger
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, wfCallback)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/my-event/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")
	req.ContentLength = 0

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// For job_run source type, callback shouldn't be called
	if capturedTrigger != nil {
		t.Fatal("workflow callback should not be called for job_run source type")
	}
}

func TestHandleSendEvent_WorkflowStepCallsCallback(t *testing.T) {
	t.Parallel()

	var callbackCalled bool
	var stepRunStatusUpdatedDirectly bool

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:                "evt-1",
				EventKey:          "my-event",
				ProjectID:         "proj-1",
				SourceType:        "workflow_step",
				WorkflowRunID:     "wr-1",
				WorkflowStepRunID: "sr-1",
				Status:            domain.EventTriggerStatusWaiting,
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			stepRunStatusUpdatedDirectly = true
			return nil
		},
	}

	wfCallback := &mockWorkflowTrigger{
		onEventReceivedFn: func(_ context.Context, _ *domain.EventTrigger) error {
			callbackCalled = true
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, wfCallback)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/my-event/send", strings.NewReader(`{"payload":{"ok":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !callbackCalled {
		t.Fatal("expected workflow callback to be called for workflow_step source")
	}
	// Step status should NOT be updated directly by the handler —
	// that's the callback's responsibility (avoids double-update).
	if stepRunStatusUpdatedDirectly {
		t.Fatal("step run status should not be updated directly by handler; callback handles it")
	}
}
