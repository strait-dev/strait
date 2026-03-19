package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleCreateProject_Success(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	ms := &mockAPIStore{
		createProjectFn: func(_ context.Context, p *domain.Project) error {
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !created.Load() {
		t.Fatal("CreateProject was not called")
	}

	var p domain.Project
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if p.ID != "proj-1" {
		t.Fatalf("expected id=proj-1, got %q", p.ID)
	}
	if p.OrgID != "org-1" {
		t.Fatalf("expected org_id=org-1, got %q", p.OrgID)
	}
	if p.Name != "My Project" {
		t.Fatalf("expected name=My Project, got %q", p.Name)
	}
}

func TestHandleCreateProject_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

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
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleCreateProject_NameTooShort(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"X"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateProject_Idempotent(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int32
	ms := &mockAPIStore{
		createProjectFn: func(_ context.Context, p *domain.Project) error {
			callCount.Add(1)
			p.CreatedAt = time.Now()
			p.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"My Project"}`
	for i := range 2 {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
		if w.Code != http.StatusCreated {
			t.Fatalf("attempt %d: expected 201, got %d", i+1, w.Code)
		}
	}
	if callCount.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", callCount.Load())
	}
}

func TestHandleCreateProject_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		createProjectFn: func(_ context.Context, _ *domain.Project) error {
			return fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"My Project"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetProject_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	ms := &mockAPIStore{
		getProjectFn: func(_ context.Context, id string) (*domain.Project, error) {
			return &domain.Project{
				ID: id, OrgID: "org-1", Name: "Test",
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/proj-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var p domain.Project
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if p.ID != "proj-1" {
		t.Fatalf("expected id=proj-1, got %q", p.ID)
	}
}

func TestHandleGetProject_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getProjectFn: func(_ context.Context, _ string) (*domain.Project, error) {
			return nil, store.ErrProjectNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/nonexistent", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetProject_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getProjectFn: func(_ context.Context, _ string) (*domain.Project, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/proj-1", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListProjects_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	ms := &mockAPIStore{
		listProjectsByOrgFn: func(_ context.Context, orgID string) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "p1", OrgID: orgID, Name: "Project 1", CreatedAt: now, UpdatedAt: now},
				{ID: "p2", OrgID: orgID, Name: "Project 2", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/?org_id=org-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var projects []domain.Project
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestHandleListProjects_MissingOrgID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListProjects_Empty(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]domain.Project, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/?org_id=org-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var projects []domain.Project
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

func TestHandleDeleteProject_Success(t *testing.T) {
	t.Parallel()
	var deleted atomic.Bool
	ms := &mockAPIStore{
		deleteProjectFn: func(_ context.Context, id string) error {
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

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if !deleted.Load() {
		t.Fatal("DeleteProject was not called")
	}
}

func TestHandleDeleteProject_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		deleteProjectFn: func(_ context.Context, _ string) error {
			return store.ErrProjectNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/projects/nonexistent", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteProject_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		deleteProjectFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/projects/proj-1", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProjectEndpoints_RequireAuth(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

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
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", w.Code)
			}
		})
	}
}

func TestProjectEndpoints_InternalSecret(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	ms := &mockAPIStore{
		createProjectFn: func(_ context.Context, p *domain.Project) error {
			p.CreatedAt = now
			p.UpdatedAt = now
			return nil
		},
		getProjectFn: func(_ context.Context, id string) (*domain.Project, error) {
			return &domain.Project{ID: id, OrgID: "org-1", Name: "Test", CreatedAt: now, UpdatedAt: now}, nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]domain.Project, error) {
			return []domain.Project{}, nil
		},
		deleteProjectFn: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// POST create
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", `{"id":"p1","org_id":"o1","name":"Test"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// GET list
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/?org_id=o1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("GET list: expected 200, got %d", w.Code)
	}

	// GET single
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/p1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("GET single: expected 200, got %d", w.Code)
	}

	// DELETE
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/projects/p1", ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: expected 204, got %d", w.Code)
	}
}
