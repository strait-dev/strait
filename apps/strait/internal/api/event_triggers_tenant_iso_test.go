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
	_, err := srv.handleSendEventByPrefix(ctx, &SendEventByPrefixInput{Prefix: "user.signup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedIDs) != 1 || capturedIDs[0] != "evt-own" {
		t.Fatalf("expected only evt-own to be batched, got %v", capturedIDs)
	}
}
