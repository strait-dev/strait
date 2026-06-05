package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestRequirePermission_AdminAllowsAll(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{"*"}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj_1", "user_1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
}

func TestRequirePermission_ViewerBlocksWrite(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return domain.SystemRolePermissions["viewer"], nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj_1", "user_1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusForbidden,
		w.Code)
}

func TestRequirePermission_OperatorCanTrigger(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return domain.SystemRolePermissions["operator"], nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsTrigger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj_1", "user_1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
}

func TestRequirePermission_APIKeyUsesScopes(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(r.Context(), ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
}

func TestRequirePermission_UnknownUserDenied(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := userCtx(httptest.NewRequest(http.MethodGet, "/", nil), "proj_1", "user_unknown")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusForbidden,
		w.Code)
}

func TestRequirePermission_InternalSecretAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	handler := srv.requirePermission(domain.ScopeJobsWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No scopes = internal auth
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxInternalCallerKey, true))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
}

func TestUsersAffectedByRoleMutation_DeepRoleChain(t *testing.T) {
	t.Parallel()
	const projectID = "proj-1"
	roles := make([]domain.ProjectRole, 8)
	members := make([]domain.ProjectMemberRole, 0, len(roles))
	for i := range roles {
		roleID := fmt.Sprintf("role-%d", i)
		parentID := ""
		if i > 0 {
			parentID = fmt.Sprintf("role-%d", i-1)
		}
		roles[i] = domain.ProjectRole{ID: roleID, ProjectID: projectID, ParentRoleID: parentID, CreatedAt: time.Unix(int64(i), 0)}
		members = append(members, domain.ProjectMemberRole{UserID: fmt.Sprintf("user-%d", i), RoleID: roleID, ProjectID: projectID, CreatedAt: time.Unix(int64(i), 0)})
	}
	s := rbacInvalidationTestServer(roles, members)

	users, err := s.usersAffectedByRoleMutation(context.Background(), projectID, "role-0")
	require.NoError(t, err)
	require.Len(t,
		users,
		len(roles),
	)
}

func TestUsersAffectedByRoleMutation_WideRoleTreeIgnoresUnrelatedBranches(t *testing.T) {
	t.Parallel()
	const projectID = "proj-1"
	roles := []domain.ProjectRole{
		{ID: "root", ProjectID: projectID, CreatedAt: time.Unix(1, 0)},
		{ID: "child-a", ProjectID: projectID, ParentRoleID: "root", CreatedAt: time.Unix(2, 0)},
		{ID: "child-b", ProjectID: projectID, ParentRoleID: "root", CreatedAt: time.Unix(3, 0)},
		{ID: "grandchild-a", ProjectID: projectID, ParentRoleID: "child-a", CreatedAt: time.Unix(4, 0)},
		{ID: "unrelated", ProjectID: projectID, CreatedAt: time.Unix(5, 0)},
	}
	members := []domain.ProjectMemberRole{
		{UserID: "user-root", RoleID: "root", ProjectID: projectID},
		{UserID: "user-a", RoleID: "child-a", ProjectID: projectID},
		{UserID: "user-b", RoleID: "child-b", ProjectID: projectID},
		{UserID: "user-grandchild", RoleID: "grandchild-a", ProjectID: projectID},
		{UserID: "user-unrelated", RoleID: "unrelated", ProjectID: projectID},
	}
	s := rbacInvalidationTestServer(roles, members)

	users, err := s.usersAffectedByRoleMutation(context.Background(), projectID, "child-a")
	require.NoError(t, err)

	slices.Sort(users)

	want := []string{"user-a", "user-grandchild"}
	require.True(
		t, slices.
			Equal(users,
				want))
}

func BenchmarkUsersAffectedByRoleMutation(b *testing.B) {
	for _, tc := range []struct {
		name  string
		roles []domain.ProjectRole
	}{
		{name: "deep_chain", roles: benchmarkRoleChain(96)},
		{name: "wide_tree", roles: benchmarkWideRoleTree(96)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			members := make([]domain.ProjectMemberRole, len(tc.roles))
			for i, role := range tc.roles {
				members[i] = domain.ProjectMemberRole{UserID: fmt.Sprintf("user-%03d", i), RoleID: role.ID, ProjectID: role.ProjectID}
			}
			s := rbacInvalidationTestServer(tc.roles, members)

			b.ReportAllocs()
			for b.Loop() {
				_, _ = s.usersAffectedByRoleMutation(context.Background(), "proj-1", "role-000")
			}
		})
	}
}

func rbacInvalidationTestServer(roles []domain.ProjectRole, members []domain.ProjectMemberRole) *Server {
	ms := &APIStoreMock{}
	ms.ListProjectRolesFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectRole, error) {
		return roles, nil
	}
	ms.ListProjectMembersFunc = func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.ProjectMemberRole, error) {
		return members, nil
	}
	return &Server{store: ms}
}

func benchmarkRoleChain(n int) []domain.ProjectRole {
	roles := make([]domain.ProjectRole, n)
	for i := range roles {
		roleID := fmt.Sprintf("role-%03d", i)
		parentID := ""
		if i > 0 {
			parentID = fmt.Sprintf("role-%03d", i-1)
		}
		roles[i] = domain.ProjectRole{ID: roleID, ProjectID: "proj-1", ParentRoleID: parentID, CreatedAt: time.Unix(int64(i), 0)}
	}
	return roles
}

func benchmarkWideRoleTree(n int) []domain.ProjectRole {
	roles := make([]domain.ProjectRole, n)
	roles[0] = domain.ProjectRole{ID: "role-000", ProjectID: "proj-1", CreatedAt: time.Unix(0, 0)}
	for i := 1; i < n; i++ {
		roles[i] = domain.ProjectRole{
			ID:           fmt.Sprintf("role-%03d", i),
			ProjectID:    "proj-1",
			ParentRoleID: "role-000",
			CreatedAt:    time.Unix(int64(i), 0),
		}
	}
	return roles
}
