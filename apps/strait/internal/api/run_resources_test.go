package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleListRunResources_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		listRunResourceSnapshotsFn: func(_ context.Context, _ string, _, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			return []domain.RunResourceSnapshot{
				{ID: "snap-1", RunID: "run-1", CPUPercent: 50.0, MemoryMB: 256},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-A"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-B"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-missing/resources", "", "proj-1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_EmptySnapshots(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		listRunResourceSnapshotsFn: func(_ context.Context, _ string, _, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			return []domain.RunResourceSnapshot{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_MissingProjectID(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_InvalidFromParam(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?from=not-a-date", "", "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_InvalidToParam(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?to=not-a-date", "", "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_InvalidLimit(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	for _, limit := range []string{"0", "-1"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?limit="+limit, "", "proj-1"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for limit=%s, got %d: %s", limit, w.Code, w.Body.String())
		}
	}
}

func TestHandleListRunResources_LimitCapped(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		listRunResourceSnapshotsFn: func(_ context.Context, _ string, _, _ *time.Time, limit int) ([]domain.RunResourceSnapshot, error) {
			capturedLimit = limit
			return []domain.RunResourceSnapshot{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?limit=5000", "", "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedLimit != 1000 {
		t.Fatalf("expected limit capped to 1000, got %d", capturedLimit)
	}
}

func TestHandleListRunResources_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		listRunResourceSnapshotsFn: func(_ context.Context, _ string, _, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-1"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}
