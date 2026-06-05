package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// handleCreateLogDrain.

func TestHandleCreateLogDrain_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.ID = "drain-1"
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "my-drain",
		"drain_type": "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type": "bearer",
		"auth_config": {"token": "abc"},
		"level_filter": ["error","warn"]
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp domain.LogDrain
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "my-drain", resp.
		Name)
}

func TestHandleCreateLogDrain_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", "not json"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateLogDrain_MissingRequired(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	// Missing name, drain_type, endpoint_url, auth_type.
	body := `{"project_id": "proj-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
	require.Contains(
		t, w.Body.String(), "validation",
	)
}

// handleListLogDrains.

func TestHandleListLogDrains_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &APIStoreMock{
		ListLogDrainsFunc: func(_ context.Context, projectID string) ([]domain.LogDrain, error) {
			return []domain.LogDrain{
				{ID: "drain-1", ProjectID: projectID, Name: "d1", DrainType: "http", Enabled: true, CreatedAt: now},
				{ID: "drain-2", ProjectID: projectID, Name: "d2", DrainType: "http", Enabled: false, CreatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/log-drains", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []domain.LogDrain
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t,
		resp, 2)
}

// handleGetLogDrain.

func TestHandleGetLogDrain_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetLogDrainFunc: func(_ context.Context, drainID, projectID string) (*domain.LogDrain, error) {
			return &domain.LogDrain{
				ID: drainID, ProjectID: projectID, Name: "my-drain",
				DrainType: "http", Enabled: true, CreatedAt: time.Now(),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/log-drains/drain-1", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp domain.LogDrain
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "drain-1", resp.
		ID)
}

func TestHandleGetLogDrain_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetLogDrainFunc: func(_ context.Context, _, _ string) (*domain.LogDrain, error) {
			return nil, store.ErrLogDrainNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/log-drains/drain-999", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// handleUpdateLogDrain.

func TestHandleUpdateLogDrain_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		UpdateLogDrainFunc: func(_ context.Context, _, _ string, _ map[string]any) error {
			return nil
		},
		GetLogDrainFunc: func(_ context.Context, drainID, projectID string) (*domain.LogDrain, error) {
			return &domain.LogDrain{
				ID: drainID, ProjectID: projectID, Name: "updated-drain",
				DrainType: "http", Enabled: true, CreatedAt: time.Now(),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"name": "updated-drain"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/log-drains/drain-1", body, "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp domain.LogDrain
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "updated-drain",
		resp.Name,
	)
}

func TestHandleUpdateLogDrain_EmptyPatch(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/log-drains/drain-1", `{}`, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
	require.Contains(
		t, w.Body.String(), "no fields to update")
}

// handleDeleteLogDrain.

func TestHandleDeleteLogDrain_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteLogDrainFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/log-drains/drain-1", "", "proj-1"))
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)
}

func TestHandleDeleteLogDrain_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteLogDrainFunc: func(_ context.Context, _, _ string) error {
			return store.ErrLogDrainNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/log-drains/drain-999", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// Regression: auth_config secrets must never be returned in API responses.
// All four read paths (create, list, get, update) must redact values.

func TestHandleLogDrain_AuthConfigRedactedOnCreate(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.ID = "drain-1"
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{
		"project_id": "proj-1",
		"name": "my-drain",
		"drain_type": "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type": "bearer",
		"auth_config": {"token": "super-secret-bearer-token"}
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotContains(t, w.Body.String(), "super-secret-bearer-token")

	var resp domain.LogDrain
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "***", resp.AuthConfig["token"])
}

func TestHandleLogDrain_AuthConfigRedactedOnGet(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetLogDrainFunc: func(_ context.Context, drainID, projectID string) (*domain.LogDrain, error) {
			return &domain.LogDrain{
				ID: drainID, ProjectID: projectID, Name: "my-drain",
				DrainType: "http", AuthType: "bearer",
				AuthConfig: map[string]string{"token": "stored-secret-from-db"},
				Enabled:    true, CreatedAt: time.Now(),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/log-drains/drain-1", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotContains(t, w.Body.String(), "stored-secret-from-db")
}

func TestHandleLogDrain_AuthConfigRedactedOnList(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListLogDrainsFunc: func(_ context.Context, projectID string) ([]domain.LogDrain, error) {
			return []domain.LogDrain{{
				ID: "d1", ProjectID: projectID, Name: "d1",
				DrainType: "http", AuthType: "header",
				AuthConfig: map[string]string{"X-Api-Key": "list-leak-secret"},
				Enabled:    true,
			}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/log-drains", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotContains(t, w.Body.String(), "list-leak-secret")
}

// handleBulkReplayRuns.

func TestHandleBulkReplayRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusFailed, Payload: json.RawMessage(`{}`),
				TriggeredBy: domain.TriggerManual, JobVersion: 1, JobVersionID: "jv-1",
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, Enabled: true, Version: 1, VersionID: "jv-1", TimeoutSecs: 30,
			}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "new-run-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"run_ids": ["run-1"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-replay", body))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1, int(resp["replayed"].(float64)))
}

func TestHandleBulkReplayRuns_NotReplayable(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusQueued,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"run_ids": ["run-1"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-replay", body))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	results := resp["results"].([]any)
	first := results[0].(map[string]any)
	require.Equal(t, "skipped", first["status"])
}

func TestHandleBulkReplayRuns_NilJobReturnsItemFailure(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-missing", ProjectID: "proj-1",
				Status: domain.StatusFailed, Payload: json.RawMessage(`{}`),
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, "job-missing",
				id)

			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			require.Fail(t,

				"queue.Enqueue must not be called when replay job is nil")
			return nil
		},
	}, nil)

	body := `{"run_ids": ["run-1"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-replay", body))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, int(resp["replayed"].(float64)))

	results := resp["results"].([]any)
	require.Len(t,
		results, 1)

	result := results[0].(map[string]any)
	require.False(t, result["status"] != "failed" ||
		result["error"] != "job not found or disabled",
	)
}

func TestHandleBulkReplayRuns_EmptyRunIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"run_ids": []}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-replay", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}
