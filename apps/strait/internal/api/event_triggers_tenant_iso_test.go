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
	"strait/internal/pubsub"

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

// failIfUnscopedLookupUsed guards the regression contract that a handler with a
// project context resolves event triggers via the project-scoped query, never
// the unscoped GetEventTriggerByEventKey (which can return another tenant's row
// when keys collide per migration 000284).
func failIfUnscopedLookupUsed(t *testing.T) func(context.Context, string) (*domain.EventTrigger, error) {
	t.Helper()
	return func(_ context.Context, _ string) (*domain.EventTrigger, error) {
		t.Fatal("project-scoped caller must not use the unscoped GetEventTriggerByEventKey")
		return nil, nil
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
	// A trigger with this key exists only in proj-bbb. The project-scoped
	// lookup for proj-aaa finds nothing, so the caller gets a deterministic 404
	// for their own project rather than racing on the colliding row.
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
			return nil, nil
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
			return nil, nil
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
			return nil, nil
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
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
		GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
		GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, _, _ string) (*domain.EventTrigger, error) {
			return nil, nil
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

// TestEventTriggerHandlers_ProjectScopedResolution is the Phase 1 regression
// guard: when two projects own a trigger with the SAME event key (legal since
// migration 000284 made keys unique per project), every handler must resolve
// the caller's OWN project's trigger deterministically and must never consult
// the unscoped lookup that could return the colliding tenant's row.
func TestEventTriggerHandlers_ProjectScopedResolution(t *testing.T) {
	t.Parallel()

	const sharedKey = "order.created"
	projATrigger := func() *domain.EventTrigger {
		return &domain.EventTrigger{
			ID: "evt-a", EventKey: sharedKey, ProjectID: "proj-aaa",
			SourceType: domain.EventSourceJobRun, Status: domain.EventTriggerStatusWaiting,
			RequestedAt: time.Now(),
		}
	}

	// The scoped lookup returns only proj-aaa's row, exactly as the SQL
	// `WHERE event_key = $1 AND project_id = $2` would; proj-bbb's colliding
	// row is never visible to proj-aaa.
	newStore := func(t *testing.T) *APIStoreMock {
		return &APIStoreMock{
			GetEventTriggerByEventKeyFunc: failIfUnscopedLookupUsed(t),
			GetEventTriggerByEventKeyForProjectFunc: func(_ context.Context, key, projectID string) (*domain.EventTrigger, error) {
				if key != sharedKey {
					t.Fatalf("unexpected key %q", key)
				}
				if projectID != "proj-aaa" {
					return nil, nil
				}
				return projATrigger(), nil
			},
		}
	}

	apiKeyCtx := func() context.Context {
		ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
		ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsTrigger, domain.ScopeJobsRead})
		ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
		ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
		return ctx
	}

	t.Run("get", func(t *testing.T) {
		t.Parallel()
		srv := newTestServer(t, newStore(t), &mockQueue{}, nil)
		out, err := srv.handleGetEventTrigger(apiKeyCtx(), &GetEventTriggerInput{EventKey: sharedKey})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Body.ID != "evt-a" || out.Body.ProjectID != "proj-aaa" {
			t.Fatalf("resolved wrong trigger: %+v", out.Body)
		}
	})

	t.Run("send", func(t *testing.T) {
		t.Parallel()
		ms := newStore(t)
		var receivedID string
		ms.UpdateEventTriggerStatusFromFunc = func(_ context.Context, id, _, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			receivedID = id
			return nil
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		_, err := srv.handleSendEvent(apiKeyCtx(), &SendEventInput{EventKey: sharedKey})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedID != "evt-a" {
			t.Fatalf("send resolved wrong trigger id %q", receivedID)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		t.Parallel()
		ms := newStore(t)
		var canceledID string
		ms.UpdateEventTriggerStatusFromFunc = func(_ context.Context, id, _, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			canceledID = id
			return nil
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		_, err := srv.handleCancelEventTrigger(apiKeyCtx(), &CancelEventTriggerInput{EventKey: sharedKey})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if canceledID != "evt-a" {
			t.Fatalf("cancel resolved wrong trigger id %q", canceledID)
		}
	})

	t.Run("stream", func(t *testing.T) {
		t.Parallel()

		ch := make(chan []byte, 1)
		_, cancel := context.WithCancel(context.Background())
		pub := &mockPublisher{
			subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
				return pubsub.NewSubscription(ch, cancel), nil
			},
		}
		srv := newEventTriggersTestServerWithPubSub(t, newStore(t), nil, pub)

		// A terminal status on proj-aaa's own trigger lets the stream read one
		// message and exit instead of blocking on the live SSE loop.
		ch <- []byte(`{"id":"evt-a","project_id":"proj-aaa","environment_id":"","status":"received"}`)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("eventKey", sharedKey)
		req := httptest.NewRequest(http.MethodGet, "/v1/events/"+sharedKey+"/stream", nil)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-aaa")
		reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
		defer reqCancel()
		req = req.WithContext(reqCtx)

		rr := httptest.NewRecorder()
		srv.handleEventTriggerStream(rr, req)

		// proj-aaa owns a waiting trigger with this colliding key, so the stream
		// must resolve it (HTTP 200 with evt-a in the body) and never 404 -- the
		// pre-fix unscoped lookup could resolve proj-bbb's row and trip the
		// tenant guard.
		if rr.Code != http.StatusOK {
			t.Fatalf("stream status = %d, want 200; body=%s", rr.Code, rr.Body.String())
		}
		if body := rr.Body.String(); !strings.Contains(body, `"id":"evt-a"`) {
			t.Fatalf("stream did not resolve proj-aaa's own trigger; body=%s", body)
		}
	})
}
