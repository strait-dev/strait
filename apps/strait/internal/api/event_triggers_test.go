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
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/sourcegraph/conc"
)

func newEventTriggersTestServer(t *testing.T, s APIStore, wfCallback WorkflowCallback) *Server {
	t.Helper()
	return newEventTriggersTestServerWithPubSub(t, s, wfCallback, &mockPublisher{})
}

func newEventTriggersTestServerWithPubSub(t *testing.T, s APIStore, wfCallback WorkflowCallback, ps pubsub.Publisher) *Server {
	t.Helper()
	if ms, ok := s.(*APIStoreMock); ok {
		installEventTriggerProjectLookupFallback(ms)
	}
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:           cfg,
		Store:            s,
		Queue:            &mockQueue{},
		PubSub:           ps,
		WorkflowCallback: wfCallback,
	})
	t.Cleanup(srv.Close)
	return srv
}

func installEventTriggerProjectLookupFallback(ms *APIStoreMock) {
	if ms.GetEventTriggerByEventKeyForProjectFunc == nil && ms.GetEventTriggerByEventKeyFunc != nil {
		ms.GetEventTriggerByEventKeyForProjectFunc = func(ctx context.Context, eventKey, projectID string) (*domain.EventTrigger, error) {
			trigger, err := ms.GetEventTriggerByEventKeyFunc(ctx, eventKey)
			if err != nil || trigger == nil {
				return trigger, err
			}
			if trigger.ProjectID != projectID {
				return nil, nil
			}
			return trigger, nil
		}
	}
}

func TestReceiveJobRunEventTrigger_EnqueuesExistingReadyRun(t *testing.T) {
	t.Parallel()

	trigger := &domain.EventTrigger{
		ID:         "evt-ready-run",
		ProjectID:  "proj-ready-run",
		SourceType: domain.EventSourceJobRun,
		JobRunID:   "run-ready",
	}
	run := &domain.JobRun{
		ID:        "run-ready",
		ProjectID: "proj-ready-run",
		Status:    domain.StatusQueued,
	}
	payload := json.RawMessage(`{"checkpoint":"resume"}`)

	var received bool
	var enqueuedRunID string
	ms := &APIStoreMock{
		ReceiveEventAndRequeueRunFunc: func(_ context.Context, triggerID string, gotPayload json.RawMessage, _ time.Time, jobRunID string) error {
			if triggerID != trigger.ID {
				t.Fatalf("triggerID = %q, want %q", triggerID, trigger.ID)
			}
			if jobRunID != trigger.JobRunID {
				t.Fatalf("jobRunID = %q, want %q", jobRunID, trigger.JobRunID)
			}
			if string(gotPayload) != string(payload) {
				t.Fatalf("payload = %s, want %s", string(gotPayload), string(payload))
			}
			received = true
			return nil
		},
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id != run.ID {
				t.Fatalf("GetRun id = %q, want %q", id, run.ID)
			}
			return run, nil
		},
	}
	queue := &mockQueue{
		enqueueExistingFn: func(_ context.Context, got *domain.JobRun) error {
			enqueuedRunID = got.ID
			return nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
		},
		Store:  ms,
		Queue:  queue,
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	if err := srv.receiveJobRunEventTrigger(context.Background(), trigger, payload, time.Now().UTC()); err != nil {
		t.Fatalf("receiveJobRunEventTrigger() error = %v", err)
	}
	if !received {
		t.Fatal("ReceiveEventAndRequeueRun was not called")
	}
	if enqueuedRunID != run.ID {
		t.Fatalf("EnqueueExisting run = %q, want %q", enqueuedRunID, run.ID)
	}
}

func TestHandleSendEvent_Success(t *testing.T) {
	t.Parallel()

	var updatedStatus string
	var updatedPayload json.RawMessage
	getTrigger := func(_ context.Context, key string) (*domain.EventTrigger, error) {
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
	}

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: getTrigger,
		GetEventTriggerByEventKeyForProjectFunc: func(ctx context.Context, key, _ string) (*domain.EventTrigger, error) {
			return getTrigger(ctx, key)
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, status string, payload json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			updatedStatus = status
			updatedPayload = payload
			return nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
	}

	wfCallback := &mockWorkflowTrigger{}
	srv := newEventTriggersTestServer(t, ms, wfCallback)

	body := `{"payload":{"result":"approved"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/aml-check:app-123/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

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

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/nonexistent/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestHandleSendEvent_AlreadyReceived_DifferentPayload(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:              "evt-1",
				EventKey:        "some-key",
				ProjectID:       "proj-1",
				Status:          domain.EventTriggerStatusReceived,
				ResponsePayload: json.RawMessage(`{"original":true}`),
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	// Different payload -> 409.
	req := httptest.NewRequest(http.MethodPost, "/v1/events/some-key/send", strings.NewReader(`{"payload":{"different":true}}`))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

func TestHandleSendEvent_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/some-key/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleGetEventTrigger_SuccessInternalSecret(t *testing.T) {
	t.Parallel()

	now := time.Now()
	getTrigger := func(_ context.Context, key string) (*domain.EventTrigger, error) {
		return &domain.EventTrigger{
			ID:          "evt-1",
			EventKey:    key,
			ProjectID:   "proj-1",
			SourceType:  "workflow_step",
			Status:      domain.EventTriggerStatusWaiting,
			RequestedAt: now,
		}, nil
	}
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: getTrigger,
		GetEventTriggerByEventKeyForProjectFunc: func(ctx context.Context, key, _ string) (*domain.EventTrigger, error) {
			return getTrigger(ctx, key)
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/aml-check:app-123", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

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

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/nonexistent", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandleListEventTriggers_Success(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := &APIStoreMock{
		ListEventTriggersByProjectFunc: func(_ context.Context, projectID, _, _, _, _ string, _ int, _ *time.Time) ([]domain.EventTrigger, error) {
			if projectID == "proj-1" {
				return []domain.EventTrigger{
					{ID: "evt-1", EventKey: "aml:app-1", ProjectID: "proj-1", Status: domain.EventTriggerStatusWaiting, RequestedAt: now},
					{ID: "evt-2", EventKey: "aml:app-2", ProjectID: "proj-1", Status: domain.EventTriggerStatusReceived, RequestedAt: now},
				}, nil
			}
			return nil, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
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

func TestHandleListEventTriggers_EnvironmentScopedCallerPassesEnvironmentFilter(t *testing.T) {
	t.Parallel()

	var gotProjectID, gotEnvironmentID string
	ms := &APIStoreMock{
		ListEventTriggersByProjectFunc: func(_ context.Context, projectID, environmentID, _, _, _ string, _ int, _ *time.Time) ([]domain.EventTrigger, error) {
			gotProjectID = projectID
			gotEnvironmentID = environmentID
			return []domain.EventTrigger{}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-1", EnvironmentID: "env-prod", Scopes: []string{domain.ScopeJobsRead}}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}

	srv := newEventTriggersTestServer(t, ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/events", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if gotProjectID != "proj-1" {
		t.Fatalf("projectID = %q, want proj-1", gotProjectID)
	}
	if gotEnvironmentID != "env-prod" {
		t.Fatalf("environmentID = %q, want env-prod", gotEnvironmentID)
	}
}

func TestHandleSendEvent_EmptyBody(t *testing.T) {
	t.Parallel()

	var capturedTrigger *domain.EventTrigger

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-1",
				EventKey:   "my-event",
				ProjectID:  "proj-1",
				SourceType: "job_run",
				JobRunID:   "run-1",
				Status:     domain.EventTriggerStatusWaiting,
			}, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			return nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
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
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
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

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
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
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			return nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
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
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !callbackCalled {
		t.Fatal("expected workflow callback to be called for workflow_step source")
	}
	// With runInTx, both trigger and step status are updated atomically
	// by the handler (even in pass-through mode without a real TxPool).
	if !stepRunStatusUpdatedDirectly {
		t.Fatal("step run status should be updated by handler inside runInTx")
	}
}

func TestHandleSendEvent_UpdateStatusConflictReturns409(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-conflict",
				EventKey:   "race-event",
				ProjectID:  "proj-1",
				SourceType: "external",
				Status:     domain.EventTriggerStatusWaiting,
			}, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			return store.ErrEventTriggerConflict
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/events/race-event/send", strings.NewReader(`{"payload":{"ok":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleSendEvent_IdempotentResend(t *testing.T) {
	t.Parallel()

	receivedAt := time.Now()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
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
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for idempotent resend, got %d: %s", w.Code, w.Body.String())
	}

	// Different payload -> 409.
	req2 := httptest.NewRequest(http.MethodPost, "/v1/events/my-event/send", strings.NewReader(`{"payload":{"ok":false}}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Internal-Secret", "test-secret-value")
	req2.Header.Set("X-Project-Id", "proj-1")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for different payload, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleSendEventByPrefix_ResolvesMultiple(t *testing.T) {
	t.Parallel()

	now := time.Now().Add(-time.Minute)
	var batchResolvedIDs []string

	ms := &APIStoreMock{
		ListEventTriggersByKeyPrefixFunc: func(_ context.Context, prefix string, _ string) ([]domain.EventTrigger, error) {
			if prefix == "order:" {
				return []domain.EventTrigger{
					{ID: "evt-1", EventKey: "order:100", ProjectID: "proj-1", SourceType: "job_run", JobRunID: "run-1", Status: domain.EventTriggerStatusWaiting, RequestedAt: now},
					{ID: "evt-2", EventKey: "order:200", ProjectID: "proj-1", SourceType: "job_run", JobRunID: "run-2", Status: domain.EventTriggerStatusWaiting, RequestedAt: now},
				}, nil
			}
			return nil, nil
		},
		BatchReceiveEventTriggersFunc: func(_ context.Context, ids []string, _ json.RawMessage, _ time.Time, _ string) ([]string, error) {
			batchResolvedIDs = ids
			return ids, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/prefix/order:/send", strings.NewReader(`{"payload":{"batch":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if len(batchResolvedIDs) != 2 {
		t.Fatalf("batch resolved count = %d, want 2", len(batchResolvedIDs))
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

	ms := &APIStoreMock{
		ListEventTriggersByKeyPrefixFunc: func(_ context.Context, _ string, _ string) ([]domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/prefix/nonexistent:/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")

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

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-1",
				EventKey:  "my-event",
				ProjectID: "proj-other",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-mine"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
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

	// Returns 404 (not 403) to avoid leaking resource existence to other projects.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusNotFound, rr.Body.String())
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
		{"key order diff", json.RawMessage(`{"a":1,"b":2}`), json.RawMessage(`{"b":2,"a":1}`), true},
		{"number equivalent", json.RawMessage(`{"n":1}`), json.RawMessage(`{"n":1.0}`), true},
		{"invalid json", json.RawMessage(`{"a":`), json.RawMessage(`{"a":`), true},
		{"invalid json different bytes", json.RawMessage(`{"a":`), json.RawMessage(`{"a":1}`), false},
		{"array whitespace diff", json.RawMessage(`[1, 2, {"a": true}]`), json.RawMessage(`[1,2,{"a":true}]`), true},
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

func BenchmarkPayloadsMatch(b *testing.B) {
	identical := json.RawMessage(`{"key":"value","count":42}`)
	whitespaceA := json.RawMessage(`{ "key" : "value", "count" : 42, "nested" : { "enabled" : true } }`)
	whitespaceB := json.RawMessage(`{"key":"value","count":42,"nested":{"enabled":true}}`)
	semanticA := json.RawMessage(`{"key":"value","count":42}`)
	semanticB := json.RawMessage(`{"count":42,"key":"value"}`)
	numberA := json.RawMessage(`{"count":1}`)
	numberB := json.RawMessage(`{"count":1.0}`)
	different := json.RawMessage(`{"key":"other"}`)
	invalid := json.RawMessage(`{"key":`)

	b.Run("identical", func(b *testing.B) {
		for range b.N {
			payloadsMatch(identical, identical)
		}
	})
	b.Run("whitespace_equal", func(b *testing.B) {
		for range b.N {
			payloadsMatch(whitespaceA, whitespaceB)
		}
	})
	b.Run("semantic_equal", func(b *testing.B) {
		for range b.N {
			payloadsMatch(semanticA, semanticB)
		}
	})
	b.Run("number_equal", func(b *testing.B) {
		for range b.N {
			payloadsMatch(numberA, numberB)
		}
	})
	b.Run("different", func(b *testing.B) {
		for range b.N {
			payloadsMatch(identical, different)
		}
	})
	b.Run("invalid", func(b *testing.B) {
		for range b.N {
			payloadsMatch(invalid, different)
		}
	})
}

func TestHandleCancelEventTrigger(t *testing.T) {
	t.Parallel()

	var canceledTriggerStatus string
	var failedStepRunID string
	var onStepFailedCalled bool

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, eventKey string) (*domain.EventTrigger, error) {
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
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			canceledTriggerStatus = status
			return nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, id string, _ domain.StepRunStatus, _ map[string]any) error {
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
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
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

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
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
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

// SSE stream tests.

func TestHandleEventTriggerStream_TerminalState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:          "evt-terminal",
				EventKey:    key,
				ProjectID:   "proj-1",
				Status:      domain.EventTriggerStatusReceived,
				RequestedAt: now,
				ReceivedAt:  &now,
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/done-key/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: status") {
		t.Fatalf("expected SSE status event, got: %s", body)
	}
	if !strings.Contains(body, "evt-terminal") {
		t.Fatalf("expected trigger ID in response, got: %s", body)
	}
}

func TestHandleEventTriggerStream_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/nonexistent/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleEventTriggerStream_ProjectMismatch(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-other",
				EventKey:  "other-key",
				ProjectID: "proj-other",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-mine"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/other-key/stream", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// Stats endpoint tests.

func TestHandleGetEventTriggerStats_RequiresProject(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stats", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetEventTriggerStats_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-stats"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
			return nil
		},
		GetEventTriggerStatsFunc: func(_ context.Context, _, _ string) (*store.EventTriggerStats, error) {
			return &store.EventTriggerStats{TotalCount: 5}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stats", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["total_count"]; !ok {
		t.Fatal("expected total_count in response")
	}
}

func TestHandleGetEventTriggerStats_EnvironmentScopedCallerPassesEnvironmentFilter(t *testing.T) {
	t.Parallel()

	var gotProjectID, gotEnvironmentID string
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-stats", EnvironmentID: "env-prod", Scopes: []string{domain.ScopeJobsRead}}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
		GetEventTriggerStatsFunc: func(_ context.Context, projectID, environmentID string) (*store.EventTriggerStats, error) {
			gotProjectID = projectID
			gotEnvironmentID = environmentID
			return &store.EventTriggerStats{TotalCount: 2}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/events/stats", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if gotProjectID != "proj-stats" {
		t.Fatalf("projectID = %q, want proj-stats", gotProjectID)
	}
	if gotEnvironmentID != "env-prod" {
		t.Fatalf("environmentID = %q, want env-prod", gotEnvironmentID)
	}
}

// Get trigger project mismatch.

func TestHandleGetEventTrigger_ProjectMismatchWithAPIKey(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-1",
				EventKey:  "my-key",
				ProjectID: "proj-other",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-mine"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/my-key", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// List triggers tests.

func TestHandleListEventTriggers_WithFilters(t *testing.T) {
	t.Parallel()

	var calledStatus, calledWfRunID, calledSourceType string
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-list"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
			return nil
		},
		ListEventTriggersByProjectFunc: func(_ context.Context, _, _, status, wfRunID, sourceType string, _ int, _ *time.Time) ([]domain.EventTrigger, error) {
			calledStatus = status
			calledWfRunID = wfRunID
			calledSourceType = sourceType
			return []domain.EventTrigger{}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/?status=waiting&workflow_run_id=wfr-1&source_type=workflow_step", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if calledStatus != "waiting" {
		t.Fatalf("expected status=waiting, got %q", calledStatus)
	}
	if calledWfRunID != "wfr-1" {
		t.Fatalf("expected workflow_run_id=wfr-1, got %q", calledWfRunID)
	}
	if calledSourceType != "workflow_step" {
		t.Fatalf("expected source_type=workflow_step, got %q", calledSourceType)
	}
}

// Cancel with job_run source.

func TestHandleCancelEventTrigger_JobRunSource(t *testing.T) {
	t.Parallel()

	var canceledRunStatus domain.RunStatus
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:          "evt-jr",
				EventKey:    "cancel-job",
				ProjectID:   "proj-1",
				SourceType:  domain.EventSourceJobRun,
				JobRunID:    "run-1",
				Status:      domain.EventTriggerStatusWaiting,
				RequestedAt: time.Now(),
			}, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			return nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			canceledRunStatus = to
			return nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/cancel-job", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if canceledRunStatus != domain.StatusCanceled {
		t.Fatalf("run status = %q, want %q", canceledRunStatus, domain.StatusCanceled)
	}
}

// Send event with workflow step resume.

func TestHandleSendEvent_WorkflowStepResume(t *testing.T) {
	t.Parallel()

	var receivedCalled bool
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:                "evt-wf",
				EventKey:          "wf-event",
				ProjectID:         "proj-1",
				SourceType:        domain.EventSourceWorkflowStep,
				WorkflowRunID:     "wfr-1",
				WorkflowStepRunID: "wsr-1",
				Status:            domain.EventTriggerStatusWaiting,
				RequestedAt:       time.Now(),
			}, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			return nil
		},
	}

	wfCallback := &mockWorkflowTrigger{
		onEventReceivedFn: func(_ context.Context, _ *domain.EventTrigger) error {
			receivedCalled = true
			return nil
		},
	}
	srv := newEventTriggersTestServer(t, ms, wfCallback)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/wf-event/send", strings.NewReader(`{"payload":{"approved":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !receivedCalled {
		t.Fatal("expected OnEventReceived to be called for workflow step")
	}
}

// Idempotent re-send with matching payload.

func TestHandleSendEvent_IdempotentResendMatchingPayload(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:              "evt-idem",
				EventKey:        "idem-key",
				ProjectID:       "proj-1",
				Status:          domain.EventTriggerStatusReceived,
				ResponsePayload: json.RawMessage(`{"ok":true}`),
				RequestedAt:     time.Now(),
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/idem-key/send", strings.NewReader(`{"payload":{"ok":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleSendEvent_ConflictDifferentPayload(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:              "evt-conf",
				EventKey:        "conf-key",
				ProjectID:       "proj-1",
				Status:          domain.EventTriggerStatusReceived,
				ResponsePayload: json.RawMessage(`{"ok":true}`),
				RequestedAt:     time.Now(),
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/conf-key/send", strings.NewReader(`{"payload":{"ok":false}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

// Store error.

func TestHandleSendEvent_GetTriggerStoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/events/any-key/send", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
}

// SSE stream: full long-poll lifecycle with mock pubsub.
func TestHandleEventTriggerStream_ReceivesMessage(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-stream",
				EventKey:  "stream-key",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}

	ch := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	sub := pubsub.NewSubscription(ch, cancel)

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return sub, nil
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, ms, nil, pub)

	// Send a terminal message on the channel so the stream reads it and exits.
	ch <- []byte(`{"id":"evt-stream","project_id":"proj-1","status":"received"}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream-key/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	// Cancel request after we get the message to avoid hanging.
	reqCtx, reqCancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer reqCancel()
	req = req.WithContext(reqCtx)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	_ = ctx // keep cancel reference alive
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"status":"received"`) {
		t.Fatalf("expected received status in SSE body, got: %s", body)
	}
	if !strings.Contains(body, "event: status") {
		t.Fatalf("expected SSE event line, got: %s", body)
	}
}

func TestHandleEventTriggerStream_IgnoresGenericRequestTimeout(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-no-generic-timeout",
				EventKey:  "no-generic-timeout",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}
	installEventTriggerProjectLookupFallback(ms)

	ch := make(chan []byte, 1)
	_, cancel := context.WithCancel(context.Background())
	sub := pubsub.NewSubscription(ch, cancel)
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			var concWG conc.WaitGroup
			defer concWG.Wait()
			concWG.Go(func() {
				time.Sleep(50 * time.Millisecond)
				ch <- []byte(`{"id":"evt-no-generic-timeout","project_id":"proj-1","status":"received"}`)
			})
			return sub, nil
		},
	}

	cfg := &config.Config{
		InternalSecret:       "test-secret-value",
		MaxBulkTriggerItems:  500,
		JWTSigningKey:        testJWTSigningKey,
		RequestTimeout:       10 * time.Millisecond,
		SSEMaxConnDuration:   time.Second,
		SSEKeepaliveInterval: time.Second,
	}
	srv := NewServer(ServerDeps{
		Config: cfg,
		Store:  ms,
		Queue:  &mockQueue{},
		PubSub: pub,
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/no-generic-timeout/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"received"`) {
		t.Fatalf("stream was cut off before terminal event; body: %s", rr.Body.String())
	}
}

func TestHandleEventTriggerStream_DropsForeignEnvironmentMessage(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:            "evt-env-stream",
				EventKey:      "env-stream-key",
				ProjectID:     "proj-1",
				EnvironmentID: "env-prod",
				Status:        domain.EventTriggerStatusWaiting,
			}, nil
		},
	}

	ch := make(chan []byte, 1)
	ch <- []byte(`{"id":"evt-env-stream","project_id":"proj-1","environment_id":"env-staging","status":"received"}`)
	close(ch)
	_, cancel := context.WithCancel(context.Background())
	sub := pubsub.NewSubscription(ch, cancel)

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return sub, nil
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, ms, nil, pub)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("eventKey", "env-stream-key")
	req := httptest.NewRequest(http.MethodGet, "/v1/events/env-stream-key/stream", nil)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.handleEventTriggerStream(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, `"environment_id":"env-staging"`) || strings.Contains(body, `"status":"received"`) {
		t.Fatalf("foreign environment message was forwarded: %s", body)
	}
	if !strings.Contains(body, `"environment_id":"env-prod"`) || !strings.Contains(body, `"status":"waiting"`) {
		t.Fatalf("expected only initial scoped trigger state, got: %s", body)
	}
}

func TestHandleEventTriggerStream_ForwardsMatchingEnvironmentMessage(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:            "evt-env-stream-ok",
				EventKey:      "env-stream-ok-key",
				ProjectID:     "proj-1",
				EnvironmentID: "env-prod",
				Status:        domain.EventTriggerStatusWaiting,
			}, nil
		},
	}

	ch := make(chan []byte, 1)
	ch <- []byte(`{"id":"evt-env-stream-ok","project_id":"proj-1","environment_id":"env-prod","status":"received"}`)
	_, cancel := context.WithCancel(context.Background())
	sub := pubsub.NewSubscription(ch, cancel)

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return sub, nil
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, ms, nil, pub)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("eventKey", "env-stream-ok-key")
	req := httptest.NewRequest(http.MethodGet, "/v1/events/env-stream-ok-key/stream", nil)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.handleEventTriggerStream(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"id":"evt-env-stream-ok"`) ||
		!strings.Contains(body, `"environment_id":"env-prod"`) ||
		!strings.Contains(body, `"status":"received"`) {
		t.Fatalf("expected matching environment message to be forwarded, got: %s", body)
	}
}

// SSE stream: context cancellation closes cleanly.
func TestHandleEventTriggerStream_ContextCancel(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-cancel-stream",
				EventKey:  "cancel-stream-key",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}

	ch := make(chan []byte)
	_, cancel := context.WithCancel(context.Background())
	sub := pubsub.NewSubscription(ch, cancel)

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return sub, nil
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, ms, nil, pub)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/cancel-stream-key/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	// Very short timeout to trigger context.Done branch.
	reqCtx, reqCancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
	defer reqCancel()
	req = req.WithContext(reqCtx)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	// Should contain the initial state message at minimum.
	if !strings.Contains(rr.Body.String(), "event: status") {
		t.Fatalf("expected initial SSE event, got: %s", rr.Body.String())
	}
}

// SSE stream: closed channel exits cleanly.
func TestHandleEventTriggerStream_ClosedChannel(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-closed",
				EventKey:  "closed-key",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}

	ch := make(chan []byte)
	close(ch) // Closed immediately — the !ok branch.
	_, cancel := context.WithCancel(context.Background())
	sub := pubsub.NewSubscription(ch, cancel)

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return sub, nil
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, ms, nil, pub)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/closed-key/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

// SSE stream: nil pubsub returns 503.
func TestHandleEventTriggerStream_NilPubsub(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-nopub",
				EventKey:  "nopub-key",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, ms, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/nopub-key/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rr.Code, rr.Body.String())
	}
}

// SSE stream: subscribe error returns 500.
func TestHandleEventTriggerStream_SubscribeError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-suberr",
				EventKey:  "suberr-key",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return nil, errors.New("redis down")
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, ms, nil, pub)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/suberr-key/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
}

// SSE stream: store error on get trigger.
func TestHandleEventTriggerStream_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/bad-key/stream", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
}

// Stats: store error returns 500.
func TestHandleGetEventTriggerStats_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-err"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
		GetEventTriggerStatsFunc: func(_ context.Context, _, _ string) (*store.EventTriggerStats, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stats", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
}

// Cancel: not found returns 404.
func TestHandleCancelEventTrigger_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/ghost-key", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// Cancel: project forbidden.
func TestHandleCancelEventTrigger_ProjectForbidden(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-proj",
				EventKey:  "proj-key",
				ProjectID: "proj-other",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-mine"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/proj-key", nil)
	req.Header.Set("Authorization", "Bearer strait_testapikey123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Returns 404 (not 403) to avoid leaking resource existence to other projects.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// Cancel: store error on status update returns 500.
func TestHandleCancelEventTrigger_UpdateStatusError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-upderr",
				EventKey:  "upderr-key",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return errors.New("update failed")
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/upderr-key", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCancelEventTrigger_UpdateStatusConflictReturns409(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-cancel-conflict",
				EventKey:  "cancel-race",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			if from != domain.EventTriggerStatusWaiting {
				t.Fatalf("from = %q, want %q", from, domain.EventTriggerStatusWaiting)
			}
			return store.ErrEventTriggerConflict
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)
	req := httptest.NewRequest(http.MethodDelete, "/v1/events/cancel-race", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

// Get trigger: store error returns 500.
func TestHandleGetEventTrigger_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/bad-key", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
}

// Get trigger: success verifies response body structure.
func TestHandleGetEventTrigger_ResponseBody(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:          "evt-ok",
				EventKey:    "ok-key",
				ProjectID:   "proj-1",
				SourceType:  domain.EventSourceWorkflowStep,
				Status:      domain.EventTriggerStatusWaiting,
				RequestedAt: time.Now(),
			}, nil
		},
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/ok-key", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Project-Id", "proj-1")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["event_key"] != "ok-key" {
		t.Fatalf("expected event_key=ok-key, got %v", resp["event_key"])
	}
	if resp["source_type"] != domain.EventSourceWorkflowStep {
		t.Fatalf("expected source_type=%s, got %v", domain.EventSourceWorkflowStep, resp["source_type"])
	}
}

// publishTriggerStatusChange: nil pubsub is a no-op.
func TestPublishTriggerStatusChange_NilPubsub(t *testing.T) {
	t.Parallel()

	srv := newEventTriggersTestServerWithPubSub(t, &APIStoreMock{}, nil, nil)
	// Should not panic.
	srv.publishTriggerStatusChange(context.Background(), &domain.EventTrigger{
		ID:       "evt-1",
		EventKey: "key-1",
		Status:   domain.EventTriggerStatusReceived,
	})
}

// publishTriggerStatusChange: publish error logs but doesn't panic.
func TestPublishTriggerStatusChange_PublishError(t *testing.T) {
	t.Parallel()

	pub := &mockPublisher{
		publishFn: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("redis down")
		},
	}

	srv := newEventTriggersTestServerWithPubSub(t, &APIStoreMock{}, nil, pub)
	// Should not panic.
	srv.publishTriggerStatusChange(context.Background(), &domain.EventTrigger{
		ID:       "evt-1",
		EventKey: "key-1",
		Status:   domain.EventTriggerStatusReceived,
	})
}

// resumeEventSource: nil callback for workflow step is a no-op.
func TestResumeEventSource_NilCallback(t *testing.T) {
	t.Parallel()

	srv := newEventTriggersTestServer(t, &APIStoreMock{}, nil)
	err := srv.resumeEventSource(context.Background(), &domain.EventTrigger{
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowStepRunID: "wsr-1",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// resumeEventSource: empty step run ID is a no-op.
func TestResumeEventSource_EmptyStepRunID(t *testing.T) {
	t.Parallel()

	srv := newEventTriggersTestServer(t, &APIStoreMock{}, nil)
	err := srv.resumeEventSource(context.Background(), &domain.EventTrigger{
		SourceType: domain.EventSourceWorkflowStep,
	})
	if err != nil {
		t.Fatalf("expected no error for empty step run ID, got %v", err)
	}
}

// resumeEventSource: empty job run ID is a no-op.
func TestResumeEventSource_EmptyJobRunID(t *testing.T) {
	t.Parallel()

	srv := newEventTriggersTestServer(t, &APIStoreMock{}, nil)
	err := srv.resumeEventSource(context.Background(), &domain.EventTrigger{
		SourceType: domain.EventSourceJobRun,
	})
	if err != nil {
		t.Fatalf("expected no error for empty job run ID, got %v", err)
	}
}

// resumeEventSource: unknown source type is a no-op.
func TestResumeEventSource_UnknownSourceType(t *testing.T) {
	t.Parallel()

	srv := newEventTriggersTestServer(t, &APIStoreMock{}, nil)
	err := srv.resumeEventSource(context.Background(), &domain.EventTrigger{
		SourceType: "unknown",
	})
	if err != nil {
		t.Fatalf("expected no error for unknown source, got %v", err)
	}
}

func TestValidateEventKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid simple", "aml:user-123", false},
		{"valid with dots", "app.check.v2:order-456", false},
		{"valid 512 chars", string(make([]byte, 512)), true}, // all null bytes → control char
		{"empty", "", true},
		{"too long", string(make([]byte, 513)), true},
		{"null byte", "key\x00bad", true},
		{"newline", "key\nbad", true},
		{"tab", "key\tbad", true},
		{"carriage return", "key\rbad", true},
		{"valid unicode", "clé:événement-42", false},
		{"valid 512 exactly", func() string {
			b := make([]byte, 512)
			for i := range b {
				b[i] = 'a'
			}
			return string(b)
		}(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := validateEventKey(tt.key)
			if tt.wantErr && result == "" {
				t.Fatalf("expected error for key %q, got none", tt.key)
			}
			if !tt.wantErr && result != "" {
				t.Fatalf("expected no error for key %q, got %q", tt.key, result)
			}
		})
	}
}

// SSE stream: raw API keys in query params are rejected to avoid credential
// leakage through browser history, logs, and referrers.
func TestHandleEventTriggerStream_RawQueryParamAuthRejected(t *testing.T) {
	t.Parallel()

	keyLookupCalled := false
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-qp",
				EventKey:  "qp-key",
				ProjectID: "proj-1",
				Status:    domain.EventTriggerStatusReceived, // terminal — immediate SSE response
			}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			keyLookupCalled = true
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}

	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/events/qp-key/stream?token=strait_testapikey123", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", rr.Code, rr.Body.String())
	}
	if keyLookupCalled {
		t.Fatal("raw query API key should be rejected before store lookup")
	}
}

func TestHandlePurgeEventTriggers(t *testing.T) {
	t.Parallel()

	t.Run("invalid request body", func(t *testing.T) {
		t.Parallel()
		srv := newEventTriggersTestServer(t, &APIStoreMock{}, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader("{"))
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("X-Project-Id", "proj-1")
		req.Header.Set("Content-Type", "application/json")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("older_than_days must be >= 1", func(t *testing.T) {
		t.Parallel()
		srv := newEventTriggersTestServer(t, &APIStoreMock{}, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader(`{"older_than_days":0}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("X-Project-Id", "proj-1")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("older_than_days overflow rejected", func(t *testing.T) {
		t.Parallel()
		srv := newEventTriggersTestServer(t, &APIStoreMock{}, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader(`{"older_than_days":36501}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("X-Project-Id", "proj-1")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("dry run success", func(t *testing.T) {
		t.Parallel()
		countCalled := false
		deleteCalled := false
		ms := &APIStoreMock{
			CountEventTriggersFinishedBeforeForProjectFunc: func(_ context.Context, projectID, environmentID string, _ time.Time) (int64, error) {
				countCalled = true
				if projectID != "proj-1" {
					t.Fatalf("projectID = %q, want proj-1", projectID)
				}
				if environmentID != "" {
					t.Fatalf("environmentID = %q, want empty", environmentID)
				}
				return 7, nil
			},
			DeleteEventTriggersFinishedBeforeForProjectFunc: func(_ context.Context, _, _ string, _ time.Time, _ int) (int64, error) {
				deleteCalled = true
				return 0, nil
			},
		}
		srv := newEventTriggersTestServer(t, ms, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader(`{"older_than_days":30,"dry_run":true}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("X-Project-Id", "proj-1")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		if !countCalled {
			t.Fatal("expected count method to be called")
		}
		if deleteCalled {
			t.Fatal("did not expect delete method on dry run")
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["dry_run"] != true {
			t.Fatalf("dry_run = %v, want true", resp["dry_run"])
		}
		if resp["would_delete"] != float64(7) {
			t.Fatalf("would_delete = %v, want 7", resp["would_delete"])
		}
	})

	t.Run("dry run count error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			CountEventTriggersFinishedBeforeForProjectFunc: func(_ context.Context, _, _ string, _ time.Time) (int64, error) {
				return 0, errors.New("count failed")
			},
		}
		srv := newEventTriggersTestServer(t, ms, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader(`{"older_than_days":30,"dry_run":true}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("X-Project-Id", "proj-1")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("delete success", func(t *testing.T) {
		t.Parallel()
		deleteCalled := false
		ms := &APIStoreMock{
			DeleteEventTriggersFinishedBeforeForProjectFunc: func(_ context.Context, projectID, environmentID string, _ time.Time, limit int) (int64, error) {
				deleteCalled = true
				if projectID != "proj-1" {
					t.Fatalf("projectID = %q, want proj-1", projectID)
				}
				if environmentID != "" {
					t.Fatalf("environmentID = %q, want empty", environmentID)
				}
				if limit != 10000 {
					t.Fatalf("limit = %d, want 10000", limit)
				}
				return 11, nil
			},
		}
		srv := newEventTriggersTestServer(t, ms, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader(`{"older_than_days":30}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("X-Project-Id", "proj-1")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		if !deleteCalled {
			t.Fatal("expected delete method to be called")
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["deleted"] != float64(11) {
			t.Fatalf("deleted = %v, want 11", resp["deleted"])
		}
	})

	t.Run("delete error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			DeleteEventTriggersFinishedBeforeForProjectFunc: func(_ context.Context, _, _ string, _ time.Time, _ int) (int64, error) {
				return 0, errors.New("delete failed")
			},
		}
		srv := newEventTriggersTestServer(t, ms, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader(`{"older_than_days":30}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.Header.Set("X-Project-Id", "proj-1")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("environment scoped dry run passes environment to store", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
				return &domain.APIKey{ID: "key-1", ProjectID: "proj-1", EnvironmentID: "env-prod", Scopes: []string{domain.ScopeJobsWrite}}, nil
			},
			TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
			CountEventTriggersFinishedBeforeForProjectFunc: func(_ context.Context, projectID, environmentID string, _ time.Time) (int64, error) {
				if projectID != "proj-1" {
					t.Fatalf("projectID = %q, want proj-1", projectID)
				}
				if environmentID != "env-prod" {
					t.Fatalf("environmentID = %q, want env-prod", environmentID)
				}
				return 3, nil
			},
		}
		srv := newEventTriggersTestServer(t, ms, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events/purge", strings.NewReader(`{"older_than_days":30,"dry_run":true}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer strait_testapikey123")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
	})
}
