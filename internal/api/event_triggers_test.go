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

func TestHandleSendEvent_AlreadyReceived_DifferentPayload(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:              "evt-1",
				EventKey:        "some-key",
				Status:          domain.EventTriggerStatusReceived,
				ResponsePayload: json.RawMessage(`{"original":true}`),
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	// Different payload -> 409.
	req := httptest.NewRequest(http.MethodPost, "/v1/events/some-key/send", strings.NewReader(`{"payload":{"different":true}}`))
	req.Header.Set("X-Internal-Secret", "test-secret")
	req.Header.Set("Content-Type", "application/json")

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
		listEventTriggersByProjectFn: func(_ context.Context, projectID, _, _, _ string, _ int, _ *time.Time) ([]domain.EventTrigger, error) {
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
	req.Header.Set("Authorization", "Bearer strait_testapikey123")

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

func TestHandleSendEvent_IdempotentResend(t *testing.T) {
	t.Parallel()

	receivedAt := time.Now()
	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:              "evt-1",
				EventKey:        "my-event",
				ProjectID:       "proj-1",
				SourceType:      "workflow_step",
				Status:          domain.EventTriggerStatusReceived,
				ResponsePayload: json.RawMessage(`{"ok":true}`),
				ReceivedAt:      &receivedAt,
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	// Same payload -> 200 (idempotent).
	req := httptest.NewRequest(http.MethodPost, "/v1/events/my-event/send", strings.NewReader(`{"payload":{"ok":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for idempotent resend, got %d: %s", w.Code, w.Body.String())
	}

	// Different payload -> 409.
	req2 := httptest.NewRequest(http.MethodPost, "/v1/events/my-event/send", strings.NewReader(`{"payload":{"ok":false}}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Internal-Secret", "test-secret")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for different payload, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleSendEventByPrefix_ResolvesMultiple(t *testing.T) {
	t.Parallel()

	now := time.Now().Add(-time.Minute)
	var resolvedCount int

	ms := &mockAPIStore{
		listEventTriggersByKeyPrefixFn: func(_ context.Context, prefix string, _ string) ([]domain.EventTrigger, error) {
			if prefix == "order:" {
				return []domain.EventTrigger{
					{ID: "evt-1", EventKey: "order:100", ProjectID: "proj-1", SourceType: "job_run", JobRunID: "run-1", Status: domain.EventTriggerStatusWaiting, RequestedAt: now},
					{ID: "evt-2", EventKey: "order:200", ProjectID: "proj-1", SourceType: "job_run", JobRunID: "run-2", Status: domain.EventTriggerStatusWaiting, RequestedAt: now},
				}, nil
			}
			return nil, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			resolvedCount++
			return nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/prefix/order:/send", strings.NewReader(`{"payload":{"batch":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if resolvedCount != 2 {
		t.Fatalf("resolved count = %d, want 2", resolvedCount)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["resolved"].(float64) != 2 {
		t.Fatalf("resolved = %v, want 2", resp["resolved"])
	}
}

func TestHandleSendEventByPrefix_NoMatches(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		listEventTriggersByKeyPrefixFn: func(_ context.Context, _ string, _ string) ([]domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/prefix/nonexistent:/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["resolved"].(float64) != 0 {
		t.Fatalf("resolved = %v, want 0", resp["resolved"])
	}
}

func TestHandleSendEvent_ProjectScoping_Forbidden(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-1",
				EventKey:  "my-event",
				ProjectID: "proj-other",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		getAPIKeyByHashFn: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-mine"}, nil
		},
		touchAPIKeyLastUsedFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	// Use API key auth to set project context.
	req := httptest.NewRequest(http.MethodPost, "/v1/events/my-event/send", strings.NewReader(`{"payload":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer strait_testapikey123")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestPayloadsMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b json.RawMessage
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", json.RawMessage(``), json.RawMessage(``), true},
		{"equal", json.RawMessage(`{"k":"v"}`), json.RawMessage(`{"k":"v"}`), true},
		{"whitespace diff", json.RawMessage(`{ "k" : "v" }`), json.RawMessage(`{"k":"v"}`), true},
		{"different", json.RawMessage(`{"a":1}`), json.RawMessage(`{"a":2}`), false},
		{"one nil", json.RawMessage(`{"a":1}`), nil, false},
		{"nil vs null literal", nil, json.RawMessage(`null`), false},
		{"null vs null", json.RawMessage(`null`), json.RawMessage(`null`), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := payloadsMatch(tt.a, tt.b); got != tt.want {
				t.Fatalf("payloadsMatch(%s, %s) = %v, want %v", string(tt.a), string(tt.b), got, tt.want)
			}
		})
	}
}

func TestHandleCancelEventTrigger(t *testing.T) {
	t.Parallel()

	var canceledTriggerStatus string
	var failedStepRunID string
	var onStepFailedCalled bool

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, eventKey string) (*domain.EventTrigger, error) {
			if eventKey == "cancel-me" {
				return &domain.EventTrigger{
					ID:                "evt-cancel",
					EventKey:          "cancel-me",
					ProjectID:         "proj-1",
					SourceType:        domain.EventSourceWorkflowStep,
					WorkflowRunID:     "wfr-1",
					WorkflowStepRunID: "wsr-1",
					Status:            domain.EventTriggerStatusWaiting,
					RequestedAt:       time.Now(),
				}, nil
			}
			return nil, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			canceledTriggerStatus = status
			return nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, _ domain.StepRunStatus, _ map[string]any) error {
			failedStepRunID = id
			return nil
		},
	}

	wfCallback := &mockWorkflowTrigger{
		onStepFailedFn: func(_ context.Context, _ string, _ string) {
			onStepFailedCalled = true
		},
	}
	srv := newEventTriggersTestServer(t, ms, wfCallback)

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/cancel-me", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if canceledTriggerStatus != domain.EventTriggerStatusCanceled {
		t.Fatalf("trigger status = %q, want %q", canceledTriggerStatus, domain.EventTriggerStatusCanceled)
	}
	if failedStepRunID != "wsr-1" {
		t.Fatalf("failed step run = %q, want %q", failedStepRunID, "wsr-1")
	}
	if !onStepFailedCalled {
		t.Fatal("expected OnStepFailed to be called")
	}
}

func TestHandleCancelEventTrigger_NotWaiting(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-done",
				EventKey:  "already-received",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusReceived,
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, &mockWorkflowTrigger{})

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/already-received", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}
