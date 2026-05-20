package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func waitingTrigger(projectID, envID string) *domain.EventTrigger {
	return &domain.EventTrigger{
		ID:            "evt-1",
		EventKey:      "user.signup",
		ProjectID:     projectID,
		EnvironmentID: envID,
		SourceType:    domain.EventSourceJobRun,
		Status:        domain.EventTriggerStatusWaiting,
		RequestedAt:   time.Now(),
	}
}

func eventTriggerAPIKeyCtx(scopes ...string) context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	ctx = context.WithValue(ctx, ctxScopesKey, scopes)
	return ctx
}

func TestTenantIso_EventTrigger_Send_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			t.Fatal("store must not be called when project ctx is empty")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	_, err := srv.handleSendEvent(context.Background(), &SendEventInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusBadRequest) {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Send_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return waitingTrigger("proj-bbb", ""), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleSendEvent(ctx, &SendEventInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Send_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return waitingTrigger("proj-aaa", "env-staging"), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleSendEvent(ctx, &SendEventInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Send_WorkflowStepRequiresWorkflowTrigger(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			trigger := waitingTrigger("proj-aaa", "")
			trigger.SourceType = domain.EventSourceWorkflowStep
			trigger.WorkflowRunID = "wfr-1"
			trigger.WorkflowStepRunID = "wsr-1"
			return trigger, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			t.Fatal("UpdateEventTriggerStatusFrom must not be called when caller only has jobs:trigger")
			return nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			t.Fatal("UpdateStepRunStatus must not be called when caller only has jobs:trigger")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	_, err := srv.handleSendEvent(eventTriggerAPIKeyCtx(domain.ScopeJobsTrigger), &SendEventInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Send_WorkflowTriggerAllowsWorkflowStep(t *testing.T) {
	t.Parallel()
	var statusUpdated bool
	var stepUpdated bool
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			trigger := waitingTrigger("proj-aaa", "")
			trigger.SourceType = domain.EventSourceWorkflowStep
			trigger.WorkflowRunID = "wfr-1"
			trigger.WorkflowStepRunID = "wsr-1"
			return trigger, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			statusUpdated = true
			if from != domain.EventTriggerStatusWaiting || status != domain.EventTriggerStatusReceived {
				t.Fatalf("status transition = %s -> %s", from, status)
			}
			return nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			stepUpdated = true
			if id != "wsr-1" || status != domain.StepCompleted {
				t.Fatalf("step update = %s %s", id, status)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	_, err := srv.handleSendEvent(eventTriggerAPIKeyCtx(domain.ScopeWorkflowsTrigger), &SendEventInput{EventKey: "user.signup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !statusUpdated || !stepUpdated {
		t.Fatalf("expected workflow trigger status and step updates, statusUpdated=%v stepUpdated=%v", statusUpdated, stepUpdated)
	}
}

func TestTenantIso_EventTrigger_Get_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	_, err := srv.handleGetEventTrigger(context.Background(), &GetEventTriggerInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusBadRequest) {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Get_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return waitingTrigger("proj-bbb", ""), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleGetEventTrigger(ctx, &GetEventTriggerInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Get_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return waitingTrigger("proj-aaa", "env-staging"), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleGetEventTrigger(ctx, &GetEventTriggerInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Cancel_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	_, err := srv.handleCancelEventTrigger(context.Background(), &CancelEventTriggerInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusBadRequest) {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Cancel_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return waitingTrigger("proj-bbb", ""), nil
		},
		UpdateEventTriggerStatusFunc: func(_ context.Context, _, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			t.Fatal("UpdateEventTriggerStatus must not be called for cross-project cancel")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleCancelEventTrigger(ctx, &CancelEventTriggerInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Cancel_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return waitingTrigger("proj-aaa", "env-staging"), nil
		},
		UpdateEventTriggerStatusFunc: func(_ context.Context, _, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			t.Fatal("UpdateEventTriggerStatus must not be called for cross-env cancel")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleCancelEventTrigger(ctx, &CancelEventTriggerInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Cancel_WorkflowStepRequiresWorkflowWrite(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			trigger := waitingTrigger("proj-aaa", "")
			trigger.SourceType = domain.EventSourceWorkflowStep
			trigger.WorkflowRunID = "wfr-1"
			trigger.WorkflowStepRunID = "wsr-1"
			return trigger, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			t.Fatal("UpdateEventTriggerStatusFrom must not be called when caller only has jobs:write")
			return nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			t.Fatal("UpdateStepRunStatus must not be called when caller only has jobs:write")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	_, err := srv.handleCancelEventTrigger(eventTriggerAPIKeyCtx(domain.ScopeJobsWrite), &CancelEventTriggerInput{EventKey: "user.signup"})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestTenantIso_EventTrigger_Cancel_WorkflowWriteAllowsWorkflowStep(t *testing.T) {
	t.Parallel()
	var statusUpdated bool
	var stepUpdated bool
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			trigger := waitingTrigger("proj-aaa", "")
			trigger.SourceType = domain.EventSourceWorkflowStep
			trigger.WorkflowRunID = "wfr-1"
			trigger.WorkflowStepRunID = "wsr-1"
			return trigger, nil
		},
		UpdateEventTriggerStatusFromFunc: func(_ context.Context, _ string, from string, status string, _ json.RawMessage, _ *time.Time, errMsg string) error {
			statusUpdated = true
			if from != domain.EventTriggerStatusWaiting || status != domain.EventTriggerStatusCanceled || errMsg == "" {
				t.Fatalf("status transition = %s -> %s err=%q", from, status, errMsg)
			}
			return nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			stepUpdated = true
			if id != "wsr-1" || status != domain.StepFailed {
				t.Fatalf("step update = %s %s", id, status)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	_, err := srv.handleCancelEventTrigger(eventTriggerAPIKeyCtx(domain.ScopeWorkflowsWrite), &CancelEventTriggerInput{EventKey: "user.signup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !statusUpdated || !stepUpdated {
		t.Fatalf("expected workflow cancel status and step updates, statusUpdated=%v stepUpdated=%v", statusUpdated, stepUpdated)
	}
}

func TestTenantIso_EventTrigger_Stream_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return waitingTrigger("proj-bbb", ""), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("eventKey", "user.signup")
	req := httptest.NewRequest(http.MethodGet, "/v1/events/user.signup/stream", nil)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-aaa")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	srv.handleEventTriggerStream(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTenantIso_EventTrigger_SendByPrefix_DropsForeignEnv(t *testing.T) {
	t.Parallel()
	own := domain.EventTrigger{
		ID: "evt-own", EventKey: "user.signup.a", ProjectID: "proj-aaa",
		EnvironmentID: "env-prod", SourceType: domain.EventSourceJobRun,
		Status: domain.EventTriggerStatusWaiting, RequestedAt: time.Now(),
	}
	foreignEnv := domain.EventTrigger{
		ID: "evt-foreign", EventKey: "user.signup.b", ProjectID: "proj-aaa",
		EnvironmentID: "env-staging", SourceType: domain.EventSourceJobRun,
		Status: domain.EventTriggerStatusWaiting, RequestedAt: time.Now(),
	}

	var capturedIDs []string
	ms := &APIStoreMock{
		ListEventTriggersByKeyPrefixFunc: func(_ context.Context, _, _ string) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{own, foreignEnv}, nil
		},
		BatchReceiveEventTriggersFunc: func(_ context.Context, ids []string, _ json.RawMessage, _ time.Time, _ string) ([]string, error) {
			capturedIDs = ids
			return ids, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsTrigger})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	_, err := srv.handleSendEventByPrefix(ctx, &SendEventByPrefixInput{Prefix: "user.signup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedIDs) != 1 || capturedIDs[0] != "evt-own" {
		t.Fatalf("expected only evt-own to be batched, got %v", capturedIDs)
	}
}

func TestTenantIso_EventTrigger_SendByPrefix_FiltersBySourcePermission(t *testing.T) {
	t.Parallel()
	jobTrigger := domain.EventTrigger{
		ID:          "evt-job",
		EventKey:    "user.signup.job",
		ProjectID:   "proj-aaa",
		SourceType:  domain.EventSourceJobRun,
		Status:      domain.EventTriggerStatusWaiting,
		RequestedAt: time.Now(),
	}
	workflowTrigger := domain.EventTrigger{
		ID:                "evt-workflow",
		EventKey:          "user.signup.workflow",
		ProjectID:         "proj-aaa",
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowRunID:     "wfr-1",
		WorkflowStepRunID: "wsr-1",
		Status:            domain.EventTriggerStatusWaiting,
		RequestedAt:       time.Now(),
	}

	var capturedIDs []string
	ms := &APIStoreMock{
		ListEventTriggersByKeyPrefixFunc: func(_ context.Context, _, _ string) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{jobTrigger, workflowTrigger}, nil
		},
		BatchReceiveEventTriggersFunc: func(_ context.Context, ids []string, _ json.RawMessage, _ time.Time, _ string) ([]string, error) {
			capturedIDs = ids
			return ids, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleSendEventByPrefix(eventTriggerAPIKeyCtx(domain.ScopeJobsTrigger), &SendEventByPrefixInput{Prefix: "user.signup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedIDs) != 1 || capturedIDs[0] != "evt-job" {
		t.Fatalf("expected only job trigger to be resolved, got %v", capturedIDs)
	}
}
