package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleGetRole_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			if id == "role_other" {
				return &domain.ProjectRole{ID: id, ProjectID: "proj-other", Name: "admin", Permissions: []string{"*"}}, nil
			}
			return nil, store.ErrRoleNotFound
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/roles/role_other", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestHandleGetRole_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			return &domain.ProjectRole{ID: id, ProjectID: "proj-mine", Name: "admin", Permissions: []string{"*"}}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/roles/role_1", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestHandleUpdateRole_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			return &domain.ProjectRole{ID: id, ProjectID: "proj-other", Name: "old", Permissions: []string{"jobs:read"}}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"hacked","permissions":["*"]}`
	req := authedProjectRequest(http.MethodPatch, "/v1/roles/role_other", body, "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestHandleDeleteRole_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	deleteCalled := false
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			return &domain.ProjectRole{ID: id, ProjectID: "proj-other", Name: "admin", Permissions: []string{"*"}}, nil
		},
		DeleteProjectRoleFunc: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodDelete, "/v1/roles/role_other", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, deleteCalled)
}

func TestHandleDeleteSecret_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	deleteCalled := false
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: id, ProjectID: "proj-other", SecretKey: "KEY"}, nil
		},
		DeleteJobSecretFunc: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodDelete, "/v1/secrets/sec_1", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, deleteCalled)
}

func TestHandleDeleteSecret_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: id, ProjectID: "proj-mine", SecretKey: "KEY"}, nil
		},
		DeleteJobSecretFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodDelete, "/v1/secrets/sec_1", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.False(t, w.Code !=
		http.StatusNoContent &&
		w.Code !=
			http.StatusOK)
}
