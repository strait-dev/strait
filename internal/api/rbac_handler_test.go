package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleCreateRole(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.createProjectRoleFn = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_1"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"deployer","description":"Can deploy","permissions":["jobs:write","jobs:trigger"]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var role domain.ProjectRole
	if err := json.NewDecoder(w.Body).Decode(&role); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if role.ID != "role_1" {
		t.Fatalf("role.ID = %q, want %q", role.ID, "role_1")
	}
}

func TestHandleCreateRole_InvalidScope(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"bad","permissions":["banana"]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleListRoles(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.listProjectRolesFn = func(_ context.Context, _ string) ([]domain.ProjectRole, error) {
		return []domain.ProjectRole{{ID: "role_1", Name: "admin"}}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleDeleteRole_NotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.deleteProjectRoleFn = func(_ context.Context, _ string) error {
		return store.ErrRoleNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/roles/nonexistent", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAssignMember(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getProjectRoleFn = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, Name: "admin"}, nil
	}
	ms.assignMemberRoleFn = func(_ context.Context, m *domain.ProjectMemberRole) error {
		m.ID = "member_1"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"user_id":"user_1","role_id":"role_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestHandleAssignMember_RoleNotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.getProjectRoleFn = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, store.ErrRoleNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"user_id":"user_1","role_id":"nonexistent"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleRemoveMember_NotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	ms.removeMemberRoleFn = func(_ context.Context, _, _ string) error {
		return store.ErrMemberNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/members/unknown_user", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
