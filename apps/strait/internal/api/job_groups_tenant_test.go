package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func newJobGroupMock(ownerProject string) *APIStoreMock {
	return &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			if id == "grp_exists" {
				return &domain.JobGroup{
					ID:        id,
					ProjectID: ownerProject,
					Name:      "test-group",
					Slug:      "test-group",
					CreatedAt: time.Now().UTC(),
				}, nil
			}
			return nil, store.ErrJobGroupNotFound
		},
		CreateJobGroupFunc: func(_ context.Context, g *domain.JobGroup) error {
			g.ID = "grp_new"
			return nil
		},
		UpdateJobGroupFunc: func(_ context.Context, _ *domain.JobGroup) error {
			return nil
		},
		DeleteJobGroupFunc: func(_ context.Context, _ string) error {
			return nil
		},
		ListJobsByGroupFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{}, nil
		},
		PauseJobsByGroupFunc: func(_ context.Context, _ string) error {
			return nil
		},
		ResumeJobsByGroupFunc: func(_ context.Context, _ string) error {
			return nil
		},
		GetJobGroupStatsFunc: func(_ context.Context, _ string) (*store.JobGroupStats, error) {
			return &store.JobGroupStats{}, nil
		},
	}
}

func TestHandleGetJobGroup_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	ms := newJobGroupMock("proj-other")
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/job-groups/grp_exists", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestHandleGetJobGroup_SameProjectAllowed(t *testing.T) {
	t.Parallel()
	ms := newJobGroupMock("proj-mine")
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/job-groups/grp_exists", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestHandleUpdateJobGroup_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	ms := newJobGroupMock("proj-other")
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"hacked"}`
	req := authedProjectRequest(http.MethodPatch, "/v1/job-groups/grp_exists", body, "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestHandleDeleteJobGroup_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	deleteCalled := false
	ms := newJobGroupMock("proj-other")
	ms.DeleteJobGroupFunc = func(_ context.Context, _ string) error {
		deleteCalled = true
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodDelete, "/v1/job-groups/grp_exists", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, deleteCalled)
}

func TestHandleListJobsByGroup_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	ms := newJobGroupMock("proj-other")
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/job-groups/grp_exists/jobs", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	// Cross-project list returns 200 with empty results (not 404) because
	// the group may have been deleted and the endpoint gracefully degrades.
}

func TestHandlePauseAllJobsByGroup_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	pauseCalled := false
	ms := newJobGroupMock("proj-other")
	ms.PauseJobsByGroupFunc = func(_ context.Context, _ string) error {
		pauseCalled = true
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/job-groups/grp_exists/pause-all", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, pauseCalled)
}

func TestHandleResumeAllJobsByGroup_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	resumeCalled := false
	ms := newJobGroupMock("proj-other")
	ms.ResumeJobsByGroupFunc = func(_ context.Context, _ string) error {
		resumeCalled = true
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/job-groups/grp_exists/resume-all", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, resumeCalled)
}

func TestHandleGetJobGroupStats_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	ms := newJobGroupMock("proj-other")
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/job-groups/grp_exists/stats", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestHandleCreateJobGroup_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	createCalled := false
	ms := newJobGroupMock("proj-other")
	ms.CreateJobGroupFunc = func(_ context.Context, _ *domain.JobGroup) error {
		createCalled = true
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"project_id":"proj-other","name":"sneaky","slug":"sneaky"}`
	req := authedProjectRequest(http.MethodPost, "/v1/job-groups/", body, "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, createCalled)
}

func TestHandleCreateJobGroup_SameProjectAllowed(t *testing.T) {
	t.Parallel()
	ms := newJobGroupMock("proj-mine")
	srv := newTestServer(t, ms, nil, nil)

	body := `{"project_id":"proj-mine","name":"legit","slug":"legit"}`
	req := authedProjectRequest(http.MethodPost, "/v1/job-groups/", body, "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,
		w.Code)
}
