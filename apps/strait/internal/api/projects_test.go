package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleCreateProject_Success(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, p *domain.Project) error {
			created.Store(true)
			p.CreatedAt = time.Now()
			p.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"My Project"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.True(
		t, created.Load(),
	)

	var p domain.Project
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&p))
	require.Equal(t, "proj-1", p.ID)
	require.Equal(t, "org-1", p.OrgID)
	require.Equal(t, "My Project",
		p.Name,
	)

}

func TestHandleCreateProject_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	tests := []struct {
		name string
		body string
	}{
		{"missing_id", `{"org_id":"org-1","name":"My Project"}`},
		{"missing_org_id", `{"id":"proj-1","name":"My Project"}`},
		{"missing_name", `{"id":"proj-1","org_id":"org-1"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", tc.body))
			require.Equal(t, http.StatusUnprocessableEntity,

				w.Code)

		})
	}
}

func TestHandleCreateProject_NameTooShort(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"X"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

}

func TestHandleCreateProject_Idempotent(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int32
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, p *domain.Project) error {
			callCount.Add(1)
			p.CreatedAt = time.Now()
			p.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"My Project"}`
	for range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
		require.Equal(t, http.StatusCreated,

			w.Code)

	}
	require.EqualValues(t, 2, callCount.
		Load())

}

func TestHandleCreateProject_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, _ *domain.Project) error {
			return fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"My Project"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestHandleGetProject_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	ms := &APIStoreMock{
		GetProjectFunc: func(_ context.Context, id string) (*domain.Project, error) {
			return &domain.Project{
				ID: id, OrgID: "org-1", Name: "Test",
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/proj-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var p domain.Project
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&p))
	require.Equal(t, "proj-1", p.ID)

}

func TestHandleGetProject_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetProjectFunc: func(_ context.Context, _ string) (*domain.Project, error) {
			return nil, store.ErrProjectNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/nonexistent", ""))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

func TestHandleGetProject_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetProjectFunc: func(_ context.Context, _ string) (*domain.Project, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/proj-1", ""))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestHandleListProjects_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	ms := &APIStoreMock{
		ListProjectsByOrgFunc: func(_ context.Context, orgID string) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "p1", OrgID: orgID, Name: "Project 1", CreatedAt: now, UpdatedAt: now},
				{ID: "p2", OrgID: orgID, Name: "Project 2", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/?org_id=org-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var projects []domain.Project
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&projects))
	require.Len(t,
		projects, 2)

}

func TestHandleListProjects_MissingOrgID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/", ""))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleListProjects_Empty(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListProjectsByOrgFunc: func(_ context.Context, _ string) ([]domain.Project, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/?org_id=org-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var projects []domain.Project
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&projects))
	require.Len(t,
		projects, 0)

}

func TestHandleDeleteProject_Success(t *testing.T) {
	t.Parallel()
	var deleted atomic.Bool
	ms := &APIStoreMock{
		DeleteProjectFunc: func(_ context.Context, id string) error {
			if id != "proj-1" {
				return fmt.Errorf("unexpected id: %s", id)
			}
			deleted.Store(true)
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/projects/proj-1", ""))
	require.Equal(t, http.StatusNoContent,

		w.Code)
	require.True(
		t, deleted.Load(),
	)

}

func TestHandleDeleteProject_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteProjectFunc: func(_ context.Context, _ string) error {
			return store.ErrProjectNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/projects/nonexistent", ""))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

func TestHandleDeleteProject_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteProjectFunc: func(_ context.Context, _ string) error {
			return fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/projects/proj-1", ""))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestProjectEndpoints_RequireAuth(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/projects/"},
		{http.MethodGet, "/v1/projects/?org_id=org-1"},
		{http.MethodGet, "/v1/projects/proj-1"},
		{http.MethodDelete, "/v1/projects/proj-1"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(ep.method, ep.path, nil)
			// No auth header
			srv.ServeHTTP(w, r)
			require.Equal(t, http.StatusUnauthorized,

				w.Code)

		})
	}
}

func TestProjectEndpoints_APIKey_CreateForbidden(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-1",
				ProjectID: "proj-1",
				Scopes:    []string{domain.ScopeProjectsManage, domain.ScopeProjectsRead},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-2","org_id":"org-1","name":"New Project"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer strait_testkey")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)

}

func TestProjectEndpoints_APIKey_ListForbidden(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-1",
				ProjectID: "proj-1",
				Scopes:    []string{domain.ScopeProjectsRead},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/?org_id=org-1", nil)
	req.Header.Set("Authorization", "Bearer strait_testkey")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)

}

func TestProjectEndpoints_APIKey_GetOwnProject(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-1",
				ProjectID: "proj-1",
				Scopes:    []string{domain.ScopeProjectsRead},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
		GetProjectFunc: func(_ context.Context, id string) (*domain.Project, error) {
			return &domain.Project{ID: id, OrgID: "org-1", Name: "Test", CreatedAt: now, UpdatedAt: now}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Own project: allowed
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1", nil)
	req.Header.Set("Authorization", "Bearer strait_testkey")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestProjectEndpoints_APIKey_GetCrossProjectForbidden(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-1",
				ProjectID: "proj-1",
				Scopes:    []string{domain.ScopeProjectsRead},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Different project: forbidden
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-OTHER", nil)
	req.Header.Set("Authorization", "Bearer strait_testkey")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)

}

func TestProjectEndpoints_APIKey_DeleteCrossProjectForbidden(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-1",
				ProjectID: "proj-1",
				Scopes:    []string{domain.ScopeProjectsManage},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/projects/proj-OTHER", nil)
	req.Header.Set("Authorization", "Bearer strait_testkey")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)

}

func TestProjectEndpoints_InternalSecret(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, p *domain.Project) error {
			p.CreatedAt = now
			p.UpdatedAt = now
			return nil
		},
		GetProjectFunc: func(_ context.Context, id string) (*domain.Project, error) {
			return &domain.Project{ID: id, OrgID: "org-1", Name: "Test", CreatedAt: now, UpdatedAt: now}, nil
		},
		ListProjectsByOrgFunc: func(_ context.Context, _ string) ([]domain.Project, error) {
			return []domain.Project{}, nil
		},
		DeleteProjectFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// POST create
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", `{"id":"p1","org_id":"o1","name":"Test"}`))
	require.Equal(t, http.StatusCreated,

		w.Code)

	// GET list
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/?org_id=o1", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	// GET single
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/p1", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	// DELETE
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/projects/p1", ""))
	require.Equal(t, http.StatusNoContent,

		w.Code)

}

func TestGetProject_InternalSecret_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	ms := &APIStoreMock{
		GetProjectFunc: func(_ context.Context, id string) (*domain.Project, error) {
			return &domain.Project{ID: id, OrgID: "org-B", Name: "Test"}, nil
		},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		store:    ms,
	})
	req := internalSecretRequestWithProject(http.MethodGet, "/v1/projects/proj-B", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)

}

func TestGetProject_InternalSecret_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A":  "org-A",
			"proj-A2": "org-A",
		},
	}
	ms := &APIStoreMock{
		GetProjectFunc: func(_ context.Context, id string) (*domain.Project, error) {
			return &domain.Project{ID: id, OrgID: "org-A", Name: "Test", CreatedAt: now, UpdatedAt: now}, nil
		},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		store:    ms,
	})
	req := internalSecretRequestWithProject(http.MethodGet, "/v1/projects/proj-A2", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestDeleteProject_InternalSecret_CrossOrgForbidden(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A": "org-A",
			"proj-B": "org-B",
		},
	}
	ms := &APIStoreMock{
		DeleteProjectFunc: func(_ context.Context, _ string) error {
			require.Fail(t,

				"delete should not be called for cross-org access")
			return nil
		},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		store:    ms,
	})
	req := internalSecretRequestWithProject(http.MethodDelete, "/v1/projects/proj-B", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)

}

func TestDeleteProject_InternalSecret_SameOrgAllowed(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		activeProjectOrgMap: map[string]string{
			"proj-A":  "org-A",
			"proj-A2": "org-A",
		},
	}
	ms := &APIStoreMock{
		DeleteProjectFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newUsageTestServerFull(t, usageTestServerOpts{
		enforcer: enforcer,
		store:    ms,
	})
	req := internalSecretRequestWithProject(http.MethodDelete, "/v1/projects/proj-A2", "", "proj-A")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent,

		w.Code)

}
