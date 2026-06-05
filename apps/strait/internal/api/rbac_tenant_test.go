package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-project role access, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for same-project role access, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-project role update, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-project role delete, got %d: %s", w.Code, w.Body.String())
	}
	if deleteCalled {
		t.Fatal("DeleteProjectRole should not have been called for cross-project access")
	}
}

func TestHandleDeleteSecret_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	deleteCalled := false
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string, _ string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: id, ProjectID: "proj-other", SecretKey: "KEY"}, nil
		},
		DeleteJobSecretFunc: func(_ context.Context, _ string, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodDelete, "/v1/secrets/sec_1", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-project secret delete, got %d: %s", w.Code, w.Body.String())
	}
	if deleteCalled {
		t.Fatal("DeleteJobSecret should not have been called for cross-project access")
	}
}

func TestHandleDeleteSecret_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string, _ string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: id, ProjectID: "proj-mine", SecretKey: "KEY"}, nil
		},
		DeleteJobSecretFunc: func(_ context.Context, _ string, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodDelete, "/v1/secrets/sec_1", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("expected 200 or 204 for same-project secret delete, got %d: %s", w.Code, w.Body.String())
	}
}
