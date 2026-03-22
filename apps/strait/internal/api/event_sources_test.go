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

// handleCreateEventSource.

func TestHandleCreateEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateEventSourceFunc: func(_ context.Context, src *domain.EventSource) error {
			src.ID = "src-1"
			src.CreatedAt = time.Now()
			src.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "deploy-events",
		"description": "Deploy lifecycle events"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.EventSource
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Name != "deploy-events" {
		t.Fatalf("expected name=deploy-events, got %q", resp.Name)
	}
	if !resp.Enabled {
		t.Fatal("expected enabled=true by default")
	}
}

func TestHandleCreateEventSource_MissingName(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id": "proj-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "validation") {
		t.Fatalf("expected validation error, got %s", w.Body.String())
	}
}

// handleListEventSources.

func TestHandleListEventSources_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &APIStoreMock{
		ListEventSourcesFunc: func(_ context.Context, projectID string) ([]domain.EventSource, error) {
			return []domain.EventSource{
				{ID: "src-1", ProjectID: projectID, Name: "src-a", Enabled: true, CreatedAt: now},
				{ID: "src-2", ProjectID: projectID, Name: "src-b", Enabled: true, CreatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []domain.EventSource
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(resp))
	}
}

func TestHandleListEventSources_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/event-sources", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// handleGetEventSource.

func TestHandleGetEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: sourceID, ProjectID: projectID, Name: "deploy-events",
				Enabled: true, CreatedAt: time.Now(),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources/src-1", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.EventSource
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.ID != "src-1" {
		t.Fatalf("expected id=src-1, got %q", resp.ID)
	}
}

func TestHandleGetEventSource_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, _, _ string) (*domain.EventSource, error) {
			return nil, store.ErrEventSourceNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources/src-999", "", "proj-1"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// handleUpdateEventSource.

func TestHandleUpdateEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		UpdateEventSourceFunc: func(_ context.Context, _, _ string, _ map[string]any) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"name": "renamed-source"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/event-sources/src-1", body, "proj-1"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateEventSource_EmptyPatch(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/event-sources/src-1", `{}`, "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no fields to update") {
		t.Fatalf("expected 'no fields to update' error, got %s", w.Body.String())
	}
}

// handleDeleteEventSource.

func TestHandleDeleteEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteEventSourceFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/src-1", "", "proj-1"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteEventSource_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteEventSourceFunc: func(_ context.Context, _, _ string) error {
			return store.ErrEventSourceNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/src-999", "", "proj-1"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// handleSubscribeToEventSource.

func TestHandleSubscribeToEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateEventSubscriptionFunc: func(_ context.Context, sub *domain.EventSubscription) error {
			sub.ID = "sub-1"
			sub.CreatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"target_type": "job",
		"target_id": "job-1",
		"filter_expr": {"eq":[["type","deploy"]]}
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources/src-1/subscribe", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.EventSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.TargetType != "job" {
		t.Fatalf("expected target_type=job, got %q", resp.TargetType)
	}
	if resp.SourceID != "src-1" {
		t.Fatalf("expected source_id=src-1, got %q", resp.SourceID)
	}
}

func TestHandleSubscribeToEventSource_MissingTargetType(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"target_id": "job-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources/src-1/subscribe", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// handleListEventSourceSubscriptions.

func TestHandleListEventSourceSubscriptions_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{ID: "sub-1", SourceID: sourceID, TargetType: "job", TargetID: "job-1", Enabled: true},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/event-sources/src-1/subscriptions", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []domain.EventSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(resp))
	}
}

// handleDeleteEventSubscription.

func TestHandleDeleteEventSubscription_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteEventSubscriptionFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/event-sources/src-1/subscriptions/sub-1", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteEventSubscription_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteEventSubscriptionFunc: func(_ context.Context, _ string) error {
			return store.ErrEventSubscriptionNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/event-sources/src-1/subscriptions/sub-999", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// handleDispatchEvent.

func TestHandleDispatchEvent_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-1", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{
					ID: "sub-1", SourceID: sourceID, TargetType: "job", TargetID: "job-1",
					FilterExpr: json.RawMessage(`{"eq":[["type","deploy"]]}`), Enabled: true,
				},
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, Enabled: true, Version: 1, VersionID: "jv-1",
				ProjectID: "proj-1",
			}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "dispatched-run-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"source":"my-source","project_id":"proj-1","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if int(resp["dispatched"].(float64)) != 1 {
		t.Fatalf("expected dispatched=1, got %v", resp["dispatched"])
	}
}

func TestHandleDispatchEvent_SourceNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, _, _ string) (*domain.EventSource, error) {
			return nil, store.ErrEventSourceNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"source":"nonexistent","project_id":"proj-1","payload":{}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDispatchEvent_SourceDisabled(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-1", ProjectID: projectID, Name: name, Enabled: false,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"source":"my-source","project_id":"proj-1","payload":{}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("expected 'disabled' in error, got %s", w.Body.String())
	}
}

func TestHandleDispatchEvent_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", "not json"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
