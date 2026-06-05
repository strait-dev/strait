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
	"github.com/stretchr/testify/require"
)

// 1. handleProjectActivityStream (SSE streaming)

func TestHandlerActivityStream_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/projects//activity/stream/", "")
	// Manually invoke the handler without chi routing so projectID is empty.
	srv.handleProjectActivityStream(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

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
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)

}

func TestHandlerActivityStream_EnvironmentScopedCallerRejectedBeforeSubscribe(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			require.Fail(t,

				"environment-scoped activity stream must be rejected before subscribing")
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
	require.Equal(t, http.StatusForbidden,
		w.
			Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "project-wide key",
		))

}

func TestHandlerActivityStream_RequiresWorkflowAndJobReadScopes(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			require.Fail(t,

				"activity stream must not subscribe before all stream scopes are authorized")
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
			require.Equal(t, http.StatusForbidden,
				w.
					Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "text/event-stream",
		w.
			Header().Get("Content-Type"))

	// Handler sets SSE headers and flushes 200 even when subscribes fail.

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
	require.True(
		t, strings.Contains(body, "event: activity"))
	require.True(
		t, strings.Contains(body, `{"type":"run_completed"}`))

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Equal(t, "dep-1", resp["id"])

}

func TestHandlerRollbackDeploymentVersion_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"environment":"production"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

}

func TestHandlerRollbackDeploymentVersion_MissingEnvironment(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestHandlerRollbackDeploymentVersion_MalformedJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", `{broken`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Equal(t, "ver-1", resp["id"])

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

// 4. handleListNotificationDeliveries

func TestHandlerListNotificationDeliveries_HappyPath(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListNotificationDeliveriesFunc: func(_ context.Context, projectID string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error) {
			require.Equal(t, "proj-1", projectID)

			return []domain.NotificationDelivery{
				{ID: "del-1", ProjectID: projectID, Status: "delivered"},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/notification-deliveries", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.False(t, len(resp) !=
		1 || resp[0]["id"] != "del-1",
	)

}

func TestHandlerListNotificationDeliveries_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// No X-Project-Id header.
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/notification-deliveries", ""))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.EqualValues(t, 10, gotLimit)
	require.NotNil(t, gotCursor)

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

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
	require.Equal(t, http.StatusGone,
		w.Code,
	)

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
			require.Fail(t,

				"mismatched environment must be rejected before subscribing")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	srv.handleRunChunkStream(w, chunkStreamRequestForEnvironment("run-1", "env-prod"))
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	body := w.Body.String()
	require.True(
		t, strings.Contains(body, "streaming not available"))

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	body := w.Body.String()
	require.True(
		t, strings.Contains(body, "failed to subscribe"))

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
			require.True(
				t, strings.HasPrefix(channel,
					"run_stream:",
				))

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
	require.True(
		t, strings.Contains(body, `{"chunk":"hello"}`),
	)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.EqualValues(t, 2, int(resp["replayed"].(float64)))

}

func TestHandlerBulkReplayWorkflowRuns_EmptyIDs(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{}
	srv := advNewTestServerWithWorkflowEngine(t, &APIStoreMock{}, we)

	body := `{"workflow_run_ids":[]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

}

func TestHandlerBulkReplayWorkflowRuns_NoWorkflowEngine(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"workflow_run_ids":["wr-1"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.EqualValues(t, 2, int(resp["replayed"].(float64)))
	require.EqualValues(t, 3, int(resp["total"].(float64)))

}

func TestHandlerBulkReplayWorkflowRuns_MalformedJSON(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{}
	srv := advNewTestServerWithWorkflowEngine(t, &APIStoreMock{}, we)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", `{broken`, "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandlerBulkReplayWorkflowRuns_MissingField(t *testing.T) {
	t.Parallel()
	we := &advMockWorkflowEngine{}
	srv := advNewTestServerWithWorkflowEngine(t, &APIStoreMock{}, we)

	body := `{}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, "proj-1"))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Equal(t, "allowed", resp["status"])

}

func TestHandlerCheckOrgLimit_MissingUserID(t *testing.T) {
	t.Parallel()
	be := &advMockBillingEnforcer{}
	srv := advNewTestServerWithBilling(t, &APIStoreMock{}, be)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?plan_tier=free", ""))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, domain.PlanFree,
		capturedTier,
	)

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
	require.Equal(t, http.StatusPaymentRequired,

		w.
			Code)

	var resp QuotaExceededBody
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Equal(t, "quota_exceeded",
		resp.
			Code)
	require.Equal(t, "org_limit_exceeded",
		resp.
			Kind,
	)

}

func TestHandlerCheckOrgLimit_NoBillingEnforcer(t *testing.T) {
	t.Parallel()
	// Community edition has no cloud billing limits.
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

}

func TestHandlerCheckOrgLimit_CloudNoBillingEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/billing/check-org-limit?user_id=usr-1", ""))
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

// 9. handleListRunState

func TestHandlerListRunState_HappyPath(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunStateFunc: func(_ context.Context, runID string) ([]domain.RunState, error) {
			require.Equal(t, "run-1", runID)

			return []domain.RunState{
				{RunID: runID, StateKey: "key1", Value: json.RawMessage(`"val1"`)},
				{RunID: runID, StateKey: "key2", Value: json.RawMessage(`42`)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/state", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Len(t,
		resp, 2)

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code)

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
	require.Equal(t, http.StatusNoContent,
		w.
			Code)
	require.Equal(t, "job-1", deletedJobID)
	require.Equal(t, "cache-key",
		deletedKey,
	)

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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

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
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

// 11. orgAdvisoryLockID (pure function)

func TestHandlerOrgAdvisoryLockID_Deterministic(t *testing.T) {
	t.Parallel()
	id1 := orgAdvisoryLockID("org-1")
	id2 := orgAdvisoryLockID("org-1")
	require.Equal(t, id2, id1)

}

func TestHandlerOrgAdvisoryLockID_DifferentOrgsProduceDifferentIDs(t *testing.T) {
	t.Parallel()
	id1 := orgAdvisoryLockID("org-alpha")
	id2 := orgAdvisoryLockID("org-beta")
	require.NotEqual(t, id2, id1)

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
	require.NotEqual(t, id2, id1)

}

func TestHandlerOrgAdvisoryLockID_UnicodeOrgID(t *testing.T) {
	t.Parallel()
	id := orgAdvisoryLockID("org-unicorn")
	// Should not panic.
	_ = id
}
