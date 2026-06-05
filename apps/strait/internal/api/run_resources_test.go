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

	"github.com/stretchr/testify/require"
)

func TestHandleListRunResources_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunResourceSnapshotsFunc: func(_ context.Context, _ string, _, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			return []domain.RunResourceSnapshot{
				{ID: "snap-1", RunID: "run-1", CPUPercent: 50.0, MemoryMB: 256},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-1"))
	require.Equal(t, http.
		StatusOK,
		w.Code)

}

func TestHandleListRunResources_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-A"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-B"))
	require.Equal(t, http.
		StatusNotFound,
		w.Code)

}

func TestHandleListRunResources_EnvironmentScopedCallerBlockedBeforeSnapshots(t *testing.T) {
	t.Parallel()
	snapshotsCalled := false
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-staging", ProjectID: "proj-1"}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		ListRunResourceSnapshotsFunc: func(_ context.Context, _ string, _, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			snapshotsCalled = true
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleListRunResources(ctx, &ListRunResourcesInput{RunID: "run-1"})
	require.True(
		t, isNotFound(err))
	require.False(t, snapshotsCalled)

}

func TestHandleListRunResources_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-missing/resources", "", "proj-1"))
	require.Equal(t, http.
		StatusNotFound,
		w.Code)

}

func TestHandleListRunResources_EmptySnapshots(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunResourceSnapshotsFunc: func(_ context.Context, _ string, _, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			return []domain.RunResourceSnapshot{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-1"))
	require.Equal(t, http.
		StatusOK,
		w.Code)

}

func TestHandleListRunResources_MissingProjectID(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunResourceSnapshotsFunc: func(_ context.Context, _ string, _ *time.Time, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", ""))
	require.Equal(t, http.
		StatusOK,
		w.Code)

	// Without a project ID in context, requireProjectMatch allows through (internal caller).

}

func TestHandleListRunResources_InvalidFromParam(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?from=not-a-date", "", "proj-1"))
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)

}

func TestHandleListRunResources_InvalidToParam(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?to=not-a-date", "", "proj-1"))
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)

}

func TestHandleListRunResources_InvalidLimit(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	for _, limit := range []string{"0", "-1"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?limit="+limit, "", "proj-1"))
		require.Equal(t, http.
			StatusBadRequest,
			w.Code)

	}
}

func TestHandleListRunResources_LimitCapped(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunResourceSnapshotsFunc: func(_ context.Context, _ string, _, _ *time.Time, limit int) ([]domain.RunResourceSnapshot, error) {
			capturedLimit = limit
			return []domain.RunResourceSnapshot{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources?limit=5000", "", "proj-1"))
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.EqualValues(t, 1000,
		capturedLimit,
	)

}

func TestHandleListRunResources_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		ListRunResourceSnapshotsFunc: func(_ context.Context, _ string, _, _ *time.Time, _ int) ([]domain.RunResourceSnapshot, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/resources", "", "proj-1"))
	require.Equal(t, http.
		StatusInternalServerError,
		w.Code,
	)

}
