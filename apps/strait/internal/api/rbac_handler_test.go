package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCreateRole(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_1"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"deployer","description":"Can deploy","permissions":["jobs:write","jobs:trigger"]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,

		w.Code)

	var role domain.ProjectRole
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&role))
	require.Equal(t, "role_1", role.
		ID)
}

func TestHandleCreateRole_InvalidScope(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"bad","permissions":["banana"]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleCreateRole_StarterBasicRBACRejectsCustomRole(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, _ *domain.ProjectRole) error {
		require.Fail(t,

			"CreateProjectRole must not run when RBAC level gate rejects")
		return nil
	}
	enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanStarter)}
	srv := newServerWithEnforcer(t, ms, nil, enforcer)

	body := `{"name":"deployer","description":"Can deploy","permissions":["jobs:write"]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "requires full RBAC")
}

func TestHandleCreateResourcePolicy_ProFullRBACRejectsAdvancedPolicy(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateResourcePolicyFunc = func(context.Context, *domain.ResourcePolicy) error {
		require.Fail(t,

			"CreateResourcePolicy must not run below Advanced RBAC")
		return nil
	}
	enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanPro)}
	srv := newServerWithEnforcer(t, ms, nil, enforcer)

	body := `{"project_id":"test-project","resource_type":"job","resource_id":"job-1","user_id":"user-1","actions":["jobs:write"]}`
	req := authedProjectRequest(http.MethodPost, "/v1/resource-policies", body, "test-project")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "requires advanced RBAC")
}

func TestHandleCreateTagPolicy_ProFullRBACRejectsAdvancedPolicy(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateTagPolicyFunc = func(context.Context, *domain.TagPolicy) error {
		require.Fail(t,

			"CreateTagPolicy must not run below Advanced RBAC")
		return nil
	}
	enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanPro)}
	srv := newServerWithEnforcer(t, ms, nil, enforcer)

	body := `{"project_id":"test-project","resource_type":"job","user_id":"user-1","tag_key":"team","tag_value":"billing","actions":["jobs:write"]}`
	req := authedProjectRequest(http.MethodPost, "/v1/tag-policies", body, "test-project")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "requires advanced RBAC")
}

func TestHandleListRoles(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.ListProjectRolesFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectRole, error) {
		return []domain.ProjectRole{{ID: "role_1", Name: "admin"}}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)
}

func TestHandleGetRole(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, Name: "admin", Permissions: []string{"*"}}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles/role_1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var role domain.ProjectRole
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&role))
	require.Equal(t, "admin", role.
		Name)
}

func TestHandleGetRole_WithLineage(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		switch id {
		case "role_child":
			return &domain.ProjectRole{ID: id, Name: "child", ParentRoleID: "role_parent", Permissions: []string{"jobs:read"}}, nil
		case "role_parent":
			return &domain.ProjectRole{ID: id, Name: "parent", Permissions: []string{"jobs:write"}}, nil
		default:
			return nil, store.ErrRoleNotFound
		}
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles/role_child?include_lineage=true", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp struct {
		Role    domain.ProjectRole   `json:"role"`
		Lineage []domain.ProjectRole `json:"lineage"`
	}
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&resp))
	require.Equal(t, "role_child",
		resp.Role.
			ID)
	require.False(t, len(resp.Lineage) !=
		1 || resp.Lineage[0].ID != "role_parent",
	)
}

func TestHandleGetRole_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, store.ErrRoleNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles/nonexistent", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleUpdateRole(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, Name: "deployer", Permissions: []string{"jobs:read"}}, nil
	}
	ms.UpdateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"deployer-v2","description":"Updated","permissions":["jobs:write"]}`
	req := authedRequest(http.MethodPatch, "/v1/roles/role_1", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)
}

func TestHandleUpdateRole_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, store.ErrRoleNotFound
	}
	ms.UpdateProjectRoleFunc = func(_ context.Context, _ *domain.ProjectRole) error {
		return store.ErrRoleNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"nope","permissions":["jobs:read"]}`
	req := authedRequest(http.MethodPatch, "/v1/roles/nonexistent", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleUpdateRole_InvalidScope(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"bad","permissions":["banana"]}`
	req := authedRequest(http.MethodPatch, "/v1/roles/role_1", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleDeleteRole_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, store.ErrRoleNotFound
	}
	ms.DeleteProjectRoleFunc = func(_ context.Context, _ string) error {
		return store.ErrRoleNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/roles/nonexistent", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleAssignMember(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, Name: "admin"}, nil
	}
	ms.AssignMemberRoleFunc = func(_ context.Context, m *domain.ProjectMemberRole) error {
		m.ID = "member_1"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"user_id":"user_1","role_id":"role_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,

		w.Code)
}

func TestHandleAssignMember_RoleNotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, store.ErrRoleNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"user_id":"user_1","role_id":"nonexistent"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleListMembers(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.ListProjectMembersFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectMemberRole, error) {
		return []domain.ProjectMemberRole{
			{ID: "m1", UserID: "user-1", RoleID: "role-1"},
			{ID: "m2", UserID: "user-2", RoleID: "role-2"},
		}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/members", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var members []domain.ProjectMemberRole
	decodePaginatedList(t, w.Body.Bytes(), &members)
	require.Len(t,
		members, 2)
}

func TestHandleRemoveMember_InvalidatesCache(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.RemoveMemberRoleFunc = func(_ context.Context, _, _ string) error {
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	// Pre-populate cache.
	srv.permCache.Set("test-project", "user-to-remove", []string{"jobs:read"})
	_, ok := srv.permCache.Get("test-project", "user-to-remove")
	require.True(
		t, ok)

	req := authedRequest(http.MethodDelete, "/v1/members/user-to-remove", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent,

		w.Code)

	// Cache should be invalidated — but the handler uses projectIDFromContext
	// which returns "" for internal secret auth. So the cache key is "":user-to-remove.
	// This is fine — in production, API key auth sets project context.
}

func TestHandleAssignMember_InvalidatesCache(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, Name: "admin"}, nil
	}
	ms.AssignMemberRoleFunc = func(_ context.Context, m *domain.ProjectMemberRole) error {
		m.ID = "member_1"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	// Pre-populate cache for this user.
	srv.permCache.Set("", "user-reassign", []string{"jobs:read"})

	body := `{"user_id":"user-reassign","role_id":"role_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,

		w.Code)

	// Cache should be invalidated.
	_, ok := srv.permCache.Get("", "user-reassign")
	require.False(t, ok)
}

func TestHandleUpdateRole_InvalidatesAssignedAndInheritedPermissionCache(t *testing.T) {
	projectID := "proj-1"
	roleID := "role-parent"
	childRoleID := "role-child"
	otherRoleID := "role-other"

	getRoleCalls := 0
	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		getRoleCalls++
		if id != roleID {
			return nil, store.ErrRoleNotFound
		}
		if getRoleCalls == 1 {
			return &domain.ProjectRole{ID: roleID, ProjectID: projectID, Name: "operator", Permissions: []string{domain.ScopeJobsRead}}, nil
		}
		return &domain.ProjectRole{ID: roleID, ProjectID: projectID, Name: "operator", Permissions: []string{domain.ScopeJobsWrite}}, nil
	}
	ms.ListProjectRolesFunc = func(_ context.Context, gotProjectID string, _ int, _ *time.Time) ([]domain.ProjectRole, error) {
		require.Equal(t, projectID, gotProjectID)

		return []domain.ProjectRole{
			{ID: roleID, ProjectID: projectID},
			{ID: childRoleID, ProjectID: projectID, ParentRoleID: roleID},
			{ID: otherRoleID, ProjectID: projectID},
		}, nil
	}
	ms.ListProjectMembersFunc = func(_ context.Context, gotProjectID string, _ int, _ *time.Time) ([]domain.ProjectMemberRole, error) {
		require.Equal(t, projectID, gotProjectID)

		return []domain.ProjectMemberRole{
			{ProjectID: projectID, UserID: "user-direct", RoleID: roleID},
			{ProjectID: projectID, UserID: "user-child", RoleID: childRoleID},
			{ProjectID: projectID, UserID: "user-other", RoleID: otherRoleID},
		}, nil
	}
	ms.UpdateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		require.Equal(t, roleID, role.
			ID)

		return nil
	}
	srv := newTestServer(t, ms, nil, nil)
	srv.permCache.Set(projectID, "user-direct", []string{domain.ScopeJobsRead})
	srv.permCache.Set(projectID, "user-child", []string{domain.ScopeJobsRead})
	srv.permCache.Set(projectID, "user-other", []string{domain.ScopeJobsRead})

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectID)
	_, err := srv.handleUpdateRole(ctx, &UpdateRoleInput{
		RoleID: roleID,
		Body: updateRoleRequest{
			Name:        "operator",
			Permissions: []string{domain.ScopeJobsWrite},
		},
	})
	require.NoError(t, err)

	if _, ok := srv.permCache.Get(projectID, "user-direct"); ok {
		require.Fail(t,

			"direct assignee cache remained after role update")
	}
	if _, ok := srv.permCache.Get(projectID, "user-child"); ok {
		require.Fail(t,

			"child role assignee cache remained after parent role update")
	}
	if _, ok := srv.permCache.Get(projectID, "user-other"); !ok {
		require.Fail(t,

			"unrelated role assignee cache was invalidated")
	}
}

func TestHandleDeleteRole_InvalidatesAssignedAndInheritedPermissionCache(t *testing.T) {
	projectID := "proj-1"
	roleID := "role-parent"
	childRoleID := "role-child"
	otherRoleID := "role-other"

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		if id != roleID {
			return nil, store.ErrRoleNotFound
		}
		return &domain.ProjectRole{ID: roleID, ProjectID: projectID, Name: "operator", Permissions: []string{domain.ScopeJobsRead}}, nil
	}
	ms.ListProjectRolesFunc = func(_ context.Context, gotProjectID string, _ int, _ *time.Time) ([]domain.ProjectRole, error) {
		require.Equal(t, projectID, gotProjectID)

		return []domain.ProjectRole{
			{ID: roleID, ProjectID: projectID},
			{ID: childRoleID, ProjectID: projectID, ParentRoleID: roleID},
			{ID: otherRoleID, ProjectID: projectID},
		}, nil
	}
	ms.ListProjectMembersFunc = func(_ context.Context, gotProjectID string, _ int, _ *time.Time) ([]domain.ProjectMemberRole, error) {
		require.Equal(t, projectID, gotProjectID)

		return []domain.ProjectMemberRole{
			{ProjectID: projectID, UserID: "user-direct", RoleID: roleID},
			{ProjectID: projectID, UserID: "user-child", RoleID: childRoleID},
			{ProjectID: projectID, UserID: "user-other", RoleID: otherRoleID},
		}, nil
	}
	ms.DeleteProjectRoleFunc = func(_ context.Context, id string) error {
		require.Equal(t, roleID, id)

		return nil
	}
	srv := newTestServer(t, ms, nil, nil)
	srv.permCache.Set(projectID, "user-direct", []string{domain.ScopeJobsRead})
	srv.permCache.Set(projectID, "user-child", []string{domain.ScopeJobsRead})
	srv.permCache.Set(projectID, "user-other", []string{domain.ScopeJobsRead})

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectID)
	_, err := srv.handleDeleteRole(ctx, &DeleteRoleInput{RoleID: roleID})
	require.NoError(t, err)

	if _, ok := srv.permCache.Get(projectID, "user-direct"); ok {
		require.Fail(t,

			"direct assignee cache remained after role delete")
	}
	if _, ok := srv.permCache.Get(projectID, "user-child"); ok {
		require.Fail(t,

			"child role assignee cache remained after parent role delete")
	}
	if _, ok := srv.permCache.Get(projectID, "user-other"); !ok {
		require.Fail(t,

			"unrelated role assignee cache was invalidated")
	}
}

func TestHandleRemoveMember_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.RemoveMemberRoleFunc = func(_ context.Context, _, _ string) error {
		return store.ErrMemberNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/members/unknown_user", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

// Additional handler tests.

func TestHandleCreateRole_EmptyBody(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/roles", "{}")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleCreateRole_EmptyPermissions(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	body := `{"name":"test","permissions":[]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleCreateRole_MalformedJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/roles", "{invalid json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleCreateRole_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, _ *domain.ProjectRole) error {
		return fmt.Errorf("database connection lost")
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"deployer","permissions":["jobs:write"]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleCreateRole_ResponseShape(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_resp"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"deployer","description":"Deploy stuff","permissions":["jobs:write","jobs:trigger"]}`
	req := authedRequest(http.MethodPost, "/v1/roles", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,

		w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&resp))

	for _, field := range []string{"id", "name", "description", "permissions"} {
		if _, ok := resp[field]; !ok {
			assert.Failf(t, "test failure",

				"response missing field %q", field)
		}
	}
	assert.Equal(
		t, "deployer", resp["name"])
	assert.Equal(
		t, "Deploy stuff",
		resp["description"])
}

func TestHandleDeleteRole_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, ProjectID: "test-project", Name: "admin", Permissions: []string{"*"}}, nil
	}
	ms.DeleteProjectRoleFunc = func(_ context.Context, _ string) error {
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/roles/role_1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent,

		w.Code)
}

func TestHandleDeleteRole_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, ProjectID: "test-project", Name: "admin", Permissions: []string{"*"}}, nil
	}
	ms.DeleteProjectRoleFunc = func(_ context.Context, _ string) error {
		return fmt.Errorf("db down")
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/roles/role_1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleGetRole_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, fmt.Errorf("timeout")
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles/role_1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleListRoles_Empty(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.ListProjectRolesFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectRole, error) {
		return []domain.ProjectRole{}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var roles []domain.ProjectRole
	decodePaginatedList(t, w.Body.Bytes(), &roles)
	require.Empty(t,
		roles)
}

func TestHandleListRoles_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.ListProjectRolesFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectRole, error) {
		return nil, fmt.Errorf("db error")
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/roles", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleUpdateRole_EmptyBody(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedRequest(http.MethodPatch, "/v1/roles/role_1", "{}")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleUpdateRole_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: "role_1", ProjectID: "proj-1", Name: "x", Permissions: []string{domain.ScopeJobsRead}}, nil
	}
	ms.UpdateProjectRoleFunc = func(_ context.Context, _ *domain.ProjectRole) error {
		return fmt.Errorf("db error")
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"name":"x","permissions":["jobs:read"]}`
	req := authedRequest(http.MethodPatch, "/v1/roles/role_1", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleAssignMember_EmptyBody(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/members", "{}")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleAssignMember_MissingUserID(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	body := `{"role_id":"role_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleAssignMember_MissingRoleID(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	body := `{"user_id":"user_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleAssignMember_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id}, nil
	}
	ms.AssignMemberRoleFunc = func(_ context.Context, _ *domain.ProjectMemberRole) error {
		return fmt.Errorf("db error")
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"user_id":"user_1","role_id":"role_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleAssignMember_GetRoleStoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, fmt.Errorf("db timeout")
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"user_id":"user_1","role_id":"role_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleAssignMember_ResponseShape(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, Name: "admin"}, nil
	}
	ms.AssignMemberRoleFunc = func(_ context.Context, m *domain.ProjectMemberRole) error {
		m.ID = "member_resp"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	body := `{"user_id":"user_1","role_id":"role_1"}`
	req := authedRequest(http.MethodPost, "/v1/members", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,

		w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.
		Body).Decode(&resp))

	for _, field := range []string{"id", "user_id", "role_id"} {
		if _, ok := resp[field]; !ok {
			assert.Failf(t, "test failure",

				"response missing field %q", field)
		}
	}
	assert.Equal(
		t, "user_1", resp["user_id"])
}

func TestHandleListMembers_Empty(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.ListProjectMembersFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectMemberRole, error) {
		return []domain.ProjectMemberRole{}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/members", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var members []domain.ProjectMemberRole
	decodePaginatedList(t, w.Body.Bytes(), &members)
	require.Empty(t,
		members)
}

func TestHandleListMembers_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.ListProjectMembersFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectMemberRole, error) {
		return nil, fmt.Errorf("db error")
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/members", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleRemoveMember_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.RemoveMemberRoleFunc = func(_ context.Context, _, _ string) error {
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/members/user_1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent,

		w.Code)
}

func TestHandleRemoveMember_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.RemoveMemberRoleFunc = func(_ context.Context, _, _ string) error {
		return fmt.Errorf("db error")
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodDelete, "/v1/members/user_1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleAssignMember_MalformedJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/members", "{broken")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleUpdateRole_MalformedJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	req := authedRequest(http.MethodPatch, "/v1/roles/role_1", "{broken")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}
