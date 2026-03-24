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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.LogDrain
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Name != "my-drain" {
		t.Fatalf("expected name=my-drain, got %q", resp.Name)
	}
}

func TestHandleCreateLogDrain_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", "not json"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateLogDrain_MissingRequired(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	// Missing name, drain_type, endpoint_url, auth_type.
	body := `{"project_id": "proj-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "validation") {
		t.Fatalf("expected validation error, got %s", w.Body.String())
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []domain.LogDrain
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 drains, got %d", len(resp))
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.LogDrain
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.ID != "drain-1" {
		t.Fatalf("expected id=drain-1, got %q", resp.ID)
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.LogDrain
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Name != "updated-drain" {
		t.Fatalf("expected name=updated-drain, got %q", resp.Name)
	}
}

func TestHandleUpdateLogDrain_EmptyPatch(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/log-drains/drain-1", `{}`, "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no fields to update") {
		t.Fatalf("expected 'no fields to update' error, got %s", w.Body.String())
	}
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

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if int(resp["replayed"].(float64)) != 1 {
		t.Fatalf("expected replayed=1, got %v", resp["replayed"])
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	results := resp["results"].([]any)
	first := results[0].(map[string]any)
	if first["status"] != "skipped" {
		t.Fatalf("expected status=skipped, got %v", first["status"])
	}
}

func TestHandleBulkReplayRuns_EmptyRunIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"run_ids": []}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-replay", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
