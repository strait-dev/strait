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

	"github.com/go-chi/chi/v5"
	"github.com/sourcegraph/conc"
)

// 1. handleProjectActivityStream (SSE streaming)

func TestHandlerActivityStream_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/projects//activity/stream/", "")
	// Manually invoke the handler without chi routing so projectID is empty.
	srv.handleProjectActivityStream(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing projectID, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerActivityStream_NoPubSub(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/projects/proj-1/activity/stream/", "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-1")
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	r = r.WithContext(ctx)

	srv.handleProjectActivityStream(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when pubsub is nil, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerActivityStream_EnvironmentScopedCallerRejectedBeforeSubscribe(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			t.Fatal("environment-scoped activity stream must be rejected before subscribing")
			return nil, nil
		},
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/projects/proj-1/activity/stream/", "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-1")
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	r = r.WithContext(ctx)

	srv.handleProjectActivityStream(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for environment-scoped activity stream, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "project-wide key") {
		t.Fatalf("expected project-wide key error, got: %s", w.Body.String())
	}
}

func TestHandlerActivityStream_RequiresWorkflowAndJobReadScopes(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			t.Fatal("activity stream must not subscribe before all stream scopes are authorized")
			return nil, nil
		},
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, pub)
	handler := srv.requireActivityStreamPermissions(http.HandlerFunc(srv.handleProjectActivityStream))

	tests := []struct {
		name   string
		scopes []string
	}{
		{name: "runs only", scopes: []string{domain.ScopeRunsRead}},
		{name: "runs and workflows only", scopes: []string{domain.ScopeRunsRead, domain.ScopeWorkflowsRead}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodGet, "/v1/projects/proj-1/activity/stream/", "")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("projectID", "proj-1")
			ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
			ctx = context.WithValue(ctx, ctxScopesKey, tt.scopes)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
			ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
			r = r.WithContext(ctx)

			handler.ServeHTTP(w, r)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandlerActivityStream_SubscribeError(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return nil, errors.New("subscribe failed")
		},
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/projects/proj-1/activity/stream/", "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-1")
	base := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	base = context.WithValue(base, ctxProjectIDKey, "proj-1")
	ctx, cancel := context.WithCancel(base)
	r = r.WithContext(ctx)

	// Cancel immediately so the handler exits its event loop after subscribe attempts.
	cancel()
	srv.handleProjectActivityStream(w, r)

	// Handler sets SSE headers and flushes 200 even when subscribes fail.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SSE headers already sent), got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}
}

func TestHandlerActivityStream_ReceivesMessage(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	ch := make(chan []byte, 1)
	_, subCancel := context.WithCancel(context.Background())
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return pubsub.NewSubscription(ch, subCancel), nil
		},
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/projects/proj-1/activity/stream/", "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-1")
	base := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	base = context.WithValue(base, ctxProjectIDKey, "proj-1")
	ctx, cancel := context.WithCancel(base)
	r = r.WithContext(ctx)

	ch <- []byte(`{"type":"run_completed"}`)
	close(ch)
	concWG.
		// The handler will read the message then exit when merged channel is closed.
		// Cancel context to break the keepalive loop.
		Go(func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		})
	srv.handleProjectActivityStream(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "event: activity") {
		t.Fatalf("expected SSE activity event in body, got: %s", body)
	}
	if !strings.Contains(body, `{"type":"run_completed"}`) {
		t.Fatalf("expected message payload in body, got: %s", body)
	}
}

// 2. handleRollbackDeploymentVersion

func TestHandlerRollbackDeploymentVersion_HappyPath(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RollbackDeploymentVersionFunc: func(_ context.Context, deploymentID, projectID, env, updatedBy string) (*domain.DeploymentVersion, error) {
			return &domain.DeploymentVersion{
				ID:          deploymentID,
				ProjectID:   projectID,
				Environment: env,
				Status:      domain.DeploymentVersionStatusDraft,
				UpdatedBy:   updatedBy,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","environment":"production"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "dep-1" {
		t.Fatalf("expected id=dep-1, got %v", resp["id"])
	}
}

func TestHandlerRollbackDeploymentVersion_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"environment":"production"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing project_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRollbackDeploymentVersion_MissingEnvironment(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing environment, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRollbackDeploymentVersion_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RollbackDeploymentVersionFunc: func(_ context.Context, _, _, _, _ string) (*domain.DeploymentVersion, error) {
			return nil, store.ErrDeploymentVersionNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","environment":"production"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-404/rollback", body))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRollbackDeploymentVersion_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RollbackDeploymentVersionFunc: func(_ context.Context, _, _, _, _ string) (*domain.DeploymentVersion, error) {
			return nil, errors.New("database unavailable")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","environment":"production"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", body))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRollbackDeploymentVersion_MalformedJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", `{broken`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d: %s", w.Code, w.Body.String())
	}
}

// 3. handleGetJobVersion

func TestHandlerGetJobVersion_HappyPath(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		GetJobVersionByVersionIDFunc: func(_ context.Context, versionID string) (*domain.JobVersion, error) {
			return &domain.JobVersion{
				ID:    versionID,
				JobID: "job-1",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/versions/ver-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "ver-1" {
		t.Fatalf("expected id=ver-1, got %v", resp["id"])
	}
}

func TestHandlerGetJobVersion_VersionNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobVersionByVersionIDFunc: func(_ context.Context, _ string) (*domain.JobVersion, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/versions/ver-missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetJobVersion_JobIDMismatch(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobVersionByVersionIDFunc: func(_ context.Context, versionID string) (*domain.JobVersion, error) {
			return &domain.JobVersion{
				ID:    versionID,
				JobID: "other-job",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/versions/ver-1", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for job ID mismatch, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetJobVersion_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobVersionByVersionIDFunc: func(_ context.Context, _ string) (*domain.JobVersion, error) {
			return nil, errors.New("db crash")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-1/versions/ver-1", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// 4. handleListNotificationDeliveries

func TestHandlerListNotificationDeliveries_HappyPath(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListNotificationDeliveriesFunc: func(_ context.Context, projectID string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error) {
			if projectID != "proj-1" {
				t.Fatalf("unexpected project_id: %s", projectID)
			}
			return []domain.NotificationDelivery{
				{ID: "del-1", ProjectID: projectID, Status: "delivered"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/notification-deliveries", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 1 || resp[0]["id"] != "del-1" {
		t.Fatalf("unexpected response: %v", resp)
	}
}

func TestHandlerListNotificationDeliveries_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// No X-Project-Id header.
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/notification-deliveries", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing project_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerListNotificationDeliveries_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListNotificationDeliveriesFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.NotificationDelivery, error) {
			return nil, errors.New("store failure")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/notification-deliveries", "", "proj-1"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerListNotificationDeliveries_WithLimitAndCursor(t *testing.T) {
	t.Parallel()
	var gotLimit int
	var gotCursor *time.Time
	ms := &APIStoreMock{
		ListNotificationDeliveriesFunc: func(_ context.Context, _ string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error) {
			gotLimit = limit
			gotCursor = cursor
			return []domain.NotificationDelivery{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	cursorTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	url := fmt.Sprintf("/v1/notification-deliveries?limit=10&cursor=%s", cursorTime.Format(time.RFC3339Nano))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, url, "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotLimit != 10 {
		t.Fatalf("expected limit=10, got %d", gotLimit)
	}
	if gotCursor == nil {
		t.Fatal("expected non-nil cursor")
	}
}

// 6. handleRunChunkStream (SSE streaming)

func chunkStreamRequest(runID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/stream/chunks", nil)
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", runID)
	// These tests dispatch directly to the handler (bypassing auth middleware),
	// so seed the context with the project ID that the handler's BOLA check
	// expects to match the run's ProjectID.
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return r.WithContext(ctx)
}

func chunkStreamRequestForEnvironment(runID, environmentID string) *http.Request {
	r := chunkStreamRequest(runID)
	ctx := context.WithValue(r.Context(), ctxEnvironmentIDKey, environmentID)
	return r.WithContext(ctx)
}

func TestHandlerRunChunkStream_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.handleRunChunkStream(w, chunkStreamRequest("missing"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRunChunkStream_TerminalRun(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusCompleted, Attempt: 1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.handleRunChunkStream(w, chunkStreamRequest("run-done"))

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRunChunkStream_EnvironmentScopedCallerCannotStreamForeignEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-staging", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			t.Fatal("mismatched environment must be rejected before subscribing")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	srv.handleRunChunkStream(w, chunkStreamRequestForEnvironment("run-1", "env-prod"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for environment mismatch, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRunChunkStream_NoPubSub(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.handleRunChunkStream(w, chunkStreamRequest("run-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SSE headers), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "streaming not available") {
		t.Fatalf("expected streaming not available error in SSE body, got: %s", body)
	}
}

func TestHandlerRunChunkStream_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.handleRunChunkStream(w, chunkStreamRequest("run-1"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRunChunkStream_SubscribeError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return nil, errors.New("subscribe failed")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	srv.handleRunChunkStream(w, chunkStreamRequest("run-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SSE), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "failed to subscribe") {
		t.Fatalf("expected subscribe error in SSE body, got: %s", body)
	}
}

func TestHandlerRunChunkStream_ReceivesMessage(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	dataCh := make(chan []byte, 1)
	_, subCancel := context.WithCancel(context.Background())
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, channel string) (*pubsub.Subscription, error) {
			if !strings.HasPrefix(channel, "run_stream:") {
				t.Fatalf("unexpected channel: %s", channel)
			}
			return pubsub.NewSubscription(dataCh, subCancel), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)

	dataCh <- []byte(`{"chunk":"hello"}`)
	close(dataCh)

	w := httptest.NewRecorder()
	r := chunkStreamRequest("run-1")
	ctx, cancel := context.WithCancel(r.Context())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", "run-1")
	r = r.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	concWG.Go(func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	})
	srv.handleRunChunkStream(w, r)

	body := w.Body.String()
	if !strings.Contains(body, `{"chunk":"hello"}`) {
		t.Fatalf("expected chunk in SSE data, got: %s", body)
	}
}

// 7. handleBulkReplayWorkflowRuns

type advMockWorkflowEngine struct {
	retryFn func(ctx context.Context, originalRunID string) (*domain.WorkflowRun, error)
}

func (m *advMockWorkflowEngine) TriggerWorkflow(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, error) {
	return nil, errors.New("not implemented in test")
}

func (m *advMockWorkflowEngine) RetryWorkflowRun(ctx context.Context, originalRunID string) (*domain.WorkflowRun, error) {
	if m.retryFn != nil {
		return m.retryFn(ctx, originalRunID)
	}
	return &domain.WorkflowRun{ID: "new-" + originalRunID}, nil
}

func advNewTestServerWithWorkflowEngine(t *testing.T, s APIStore, we WorkflowTrigger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:         cfg,
		Store:          s,
		Queue:          &mockQueue{},
		WorkflowEngine: we,
		Edition:        domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandlerBulkReplayWorkflowRuns_HappyPath(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{
		retryFn: func(_ context.Context, originalRunID string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "new-" + originalRunID}, nil
		},
	}
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, ProjectID: "proj-1", WorkflowID: "wf-1", Status: domain.WfStatusFailed}, nil
		},
	}
	srv := advNewTestServerWithWorkflowEngine(t, ms, we)

	body := `{"workflow_run_ids":["wr-1","wr-2"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if int(resp["replayed"].(float64)) != 2 {
		t.Fatalf("expected replayed=2, got %v", resp["replayed"])
	}
}

func TestHandlerBulkReplayWorkflowRuns_EmptyIDs(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{}
	srv := advNewTestServerWithWorkflowEngine(t, &APIStoreMock{}, we)

	body := `{"workflow_run_ids":[]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty IDs, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerBulkReplayWorkflowRuns_NoWorkflowEngine(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"workflow_run_ids":["wr-1"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerBulkReplayWorkflowRuns_PartialFailure(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{
		retryFn: func(_ context.Context, originalRunID string) (*domain.WorkflowRun, error) {
			if originalRunID == "wr-bad" {
				return nil, errors.New("workflow not found")
			}
			return &domain.WorkflowRun{ID: "new-" + originalRunID}, nil
		},
	}
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, ProjectID: "proj-1", WorkflowID: "wf-1", Status: domain.WfStatusFailed}, nil
		},
	}
	srv := advNewTestServerWithWorkflowEngine(t, ms, we)

	body := `{"workflow_run_ids":["wr-1","wr-bad","wr-2"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if int(resp["replayed"].(float64)) != 2 {
		t.Fatalf("expected replayed=2, got %v", resp["replayed"])
	}
	if int(resp["total"].(float64)) != 3 {
		t.Fatalf("expected total=3, got %v", resp["total"])
	}
}

func TestHandlerBulkReplayWorkflowRuns_MalformedJSON(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{}
	srv := advNewTestServerWithWorkflowEngine(t, &APIStoreMock{}, we)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", `{broken`, "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerBulkReplayWorkflowRuns_MissingField(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{}
	srv := advNewTestServerWithWorkflowEngine(t, &APIStoreMock{}, we)

	body := `{}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing workflow_run_ids, got %d: %s", w.Code, w.Body.String())
	}
}

// 8. handleCheckOrgLimit

type advMockBillingEnforcer struct {
	checkOrgCreationLimitFn func(ctx context.Context, userID string, planTier domain.PlanTier) error
	getActiveProjectOrgIDFn func(ctx context.Context, projectID string) (string, error)
}

func (m *advMockBillingEnforcer) CheckProjectLimit(_ context.Context, _ string) error { return nil }
func (m *advMockBillingEnforcer) CheckMemberLimit(_ context.Context, _ string) error  { return nil }
func (m *advMockBillingEnforcer) CheckProjectBudgetLimit(_ context.Context, _ string) error {
	return nil
}
func (m *advMockBillingEnforcer) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *advMockBillingEnforcer) GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error) {
	if m.getActiveProjectOrgIDFn != nil {
		return m.getActiveProjectOrgIDFn(ctx, projectID)
	}
	return "org-1", nil
}
func (m *advMockBillingEnforcer) GetOrgPlanLimits(_ context.Context, _ string) (billing.OrgPlanLimits, error) {
	return billing.OrgPlanLimits{}, nil
}
func (m *advMockBillingEnforcer) GetMonthlyRunCount(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (m *advMockBillingEnforcer) EnsureOrgSubscription(_ context.Context, _ string) error { return nil }
func (m *advMockBillingEnforcer) CheckMaxDispatchPriority(_ context.Context, _ string, _ int) error {
	return nil
}
func (m *advMockBillingEnforcer) CheckOrgCreationLimit(ctx context.Context, userID string, planTier domain.PlanTier) error {
	if m.checkOrgCreationLimitFn != nil {
		return m.checkOrgCreationLimitFn(ctx, userID, planTier)
	}
	return nil
}
func (m *advMockBillingEnforcer) DispatchBilling(_ context.Context, _ string, _ domain.PlanTier, _ string, _ map[string]any) {
}

func advNewTestServerWithBilling(t *testing.T, s APIStore, be BillingEnforcer) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:          cfg,
		Store:           s,
		Queue:           &mockQueue{},
		BillingEnforcer: be,
		Edition:         domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandlerCheckOrgLimit_HappyPath(t *testing.T) {
	t.Parallel()
	be := &advMockBillingEnforcer{}
	srv := advNewTestServerWithBilling(t, &APIStoreMock{}, be)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1&plan_tier=free", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "allowed" {
		t.Fatalf("expected status=allowed, got %v", resp["status"])
	}
}

func TestHandlerCheckOrgLimit_MissingUserID(t *testing.T) {
	t.Parallel()
	be := &advMockBillingEnforcer{}
	srv := advNewTestServerWithBilling(t, &APIStoreMock{}, be)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?plan_tier=free", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing user_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerCheckOrgLimit_DefaultsToFreeTier(t *testing.T) {
	t.Parallel()
	var capturedTier domain.PlanTier
	be := &advMockBillingEnforcer{
		checkOrgCreationLimitFn: func(_ context.Context, _ string, tier domain.PlanTier) error {
			capturedTier = tier
			return nil
		},
	}
	srv := advNewTestServerWithBilling(t, &APIStoreMock{}, be)

	w := httptest.NewRecorder()
	// No plan_tier parameter.
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedTier != domain.PlanFree {
		t.Fatalf("expected plan_tier to default to free, got %s", capturedTier)
	}
}

func TestHandlerCheckOrgLimit_LimitExceeded(t *testing.T) {
	t.Parallel()
	be := &advMockBillingEnforcer{
		checkOrgCreationLimitFn: func(_ context.Context, _ string, _ domain.PlanTier) error {
			return &billing.LimitError{
				Code:         "org_limit_exceeded",
				Message:      "too many orgs",
				CurrentUsage: 5,
				Limit:        5,
				Plan:         "free",
			}
		},
	}
	srv := advNewTestServerWithBilling(t, &APIStoreMock{}, be)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1&plan_tier=free", ""))

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 for limit exceeded, got %d: %s", w.Code, w.Body.String())
	}

	var resp QuotaExceededBody
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Code != "quota_exceeded" {
		t.Fatalf("expected code 'quota_exceeded', got %q", resp.Code)
	}
	if resp.Kind != "org_limit_exceeded" {
		t.Fatalf("expected kind 'org_limit_exceeded', got %q", resp.Kind)
	}
}

func TestHandlerCheckOrgLimit_NoBillingEnforcer(t *testing.T) {
	t.Parallel()
	// Community edition has no cloud billing limits.
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerCheckOrgLimit_CloudNoBillingEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1", ""))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerCheckOrgLimit_StoreError(t *testing.T) {
	t.Parallel()
	be := &advMockBillingEnforcer{
		checkOrgCreationLimitFn: func(_ context.Context, _ string, _ domain.PlanTier) error {
			return errors.New("database unavailable")
		},
	}
	srv := advNewTestServerWithBilling(t, &APIStoreMock{}, be)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// 9. handleListRunState

func TestHandlerListRunState_HappyPath(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunStateFunc: func(_ context.Context, runID string) ([]domain.RunState, error) {
			if runID != "run-1" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			return []domain.RunState{
				{RunID: runID, StateKey: "key1", Value: json.RawMessage(`"val1"`)},
				{RunID: runID, StateKey: "key2", Value: json.RawMessage(`42`)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/state", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp))
	}
}

func TestHandlerListRunState_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunStateFunc: func(_ context.Context, _ string) ([]domain.RunState, error) {
			return nil, errors.New("db failure")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/state", "", "proj-1"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerListRunState_EmptyResult(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunStateFunc: func(_ context.Context, _ string) ([]domain.RunState, error) {
			return []domain.RunState{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/state", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// 10. handleSDKDeleteMemory

func TestHandlerSDKDeleteMemory_HappyPath(t *testing.T) {
	t.Parallel()
	var deletedJobID, deletedKey string
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		DeleteJobMemoryFunc: func(_ context.Context, jobID, key string) error {
			deletedJobID = jobID
			deletedKey = key
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodDelete, "/sdk/v1/runs/run-1/memory/cache-key", "run-1", "")
	chi.RouteContext(r.Context()).URLParams.Add("key", "cache-key")
	TypedHandler(srv, http.StatusNoContent, srv.handleSDKDeleteMemory)(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if deletedJobID != "job-1" {
		t.Fatalf("expected jobID=job-1, got %s", deletedJobID)
	}
	if deletedKey != "cache-key" {
		t.Fatalf("expected key=cache-key, got %s", deletedKey)
	}
}

func TestHandlerSDKDeleteMemory_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodDelete, "/sdk/v1/runs/run-missing/memory/k", "run-missing", "")
	chi.RouteContext(r.Context()).URLParams.Add("key", "k")
	TypedHandler(srv, http.StatusNoContent, srv.handleSDKDeleteMemory)(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerSDKDeleteMemory_GetRunStoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errors.New("pg connection refused")
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodDelete, "/sdk/v1/runs/run-1/memory/k", "run-1", "")
	chi.RouteContext(r.Context()).URLParams.Add("key", "k")
	TypedHandler(srv, http.StatusNoContent, srv.handleSDKDeleteMemory)(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerSDKDeleteMemory_DeleteStoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		DeleteJobMemoryFunc: func(_ context.Context, _, _ string) error {
			return errors.New("delete failed")
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodDelete, "/sdk/v1/runs/run-1/memory/k", "run-1", "")
	chi.RouteContext(r.Context()).URLParams.Add("key", "k")
	TypedHandler(srv, http.StatusNoContent, srv.handleSDKDeleteMemory)(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// 11. orgAdvisoryLockID (pure function)

func TestHandlerOrgAdvisoryLockID_Deterministic(t *testing.T) {
	t.Parallel()
	id1 := orgAdvisoryLockID("org-1")
	id2 := orgAdvisoryLockID("org-1")
	if id1 != id2 {
		t.Fatalf("expected deterministic result, got %d and %d", id1, id2)
	}
}

func TestHandlerOrgAdvisoryLockID_DifferentOrgsProduceDifferentIDs(t *testing.T) {
	t.Parallel()
	id1 := orgAdvisoryLockID("org-alpha")
	id2 := orgAdvisoryLockID("org-beta")
	if id1 == id2 {
		t.Fatalf("expected different IDs for different org IDs, both got %d", id1)
	}
}

func TestHandlerOrgAdvisoryLockID_EmptyString(t *testing.T) {
	t.Parallel()
	// Should not panic on empty string.
	id := orgAdvisoryLockID("")
	if id == 0 {
		// FNV-1a of empty string has a specific initial hash, unlikely to be 0
		// but the main point is no panic.
		t.Log("advisory lock ID for empty string is 0, which is valid")
	}
}

func TestHandlerOrgAdvisoryLockID_LongOrgID(t *testing.T) {
	t.Parallel()
	longID := strings.Repeat("x", 10000)
	id := orgAdvisoryLockID(longID)
	// Should not panic and should return a valid int64.
	_ = id
}

func TestHandlerOrgAdvisoryLockID_SpecialCharacters(t *testing.T) {
	t.Parallel()
	id1 := orgAdvisoryLockID("org/with/slashes")
	id2 := orgAdvisoryLockID("org-with-dashes")
	if id1 == id2 {
		t.Fatalf("expected different IDs for different org strings, both got %d", id1)
	}
}

func TestHandlerOrgAdvisoryLockID_UnicodeOrgID(t *testing.T) {
	t.Parallel()
	id := orgAdvisoryLockID("org-unicorn")
	// Should not panic.
	_ = id
}
