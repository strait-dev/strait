package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func rbacGrantCtx(scopes ...string) context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-rbac")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:rbac")
	ctx = context.WithValue(ctx, ctxScopesKey, scopes)
	return ctx
}

func TestRBACInheritance_CreateRoleRequiresGrantOfInheritedParentPermissions(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			require.Equal(t, "role-parent",
				id)

			return &domain.ProjectRole{
				ID:          "role-parent",
				ProjectID:   "proj-rbac",
				Permissions: []string{domain.ScopeJobsWrite},
			}, nil
		},
		CreateProjectRoleFunc: func(_ context.Context, _ *domain.ProjectRole) error {
			require.Fail(t,

				"CreateProjectRole must not run when parent grants permissions the caller lacks")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleCreateRole(rbacGrantCtx(domain.ScopeJobsRead), &CreateRoleInput{Body: createRoleRequest{
		Name:         "child",
		Permissions:  []string{domain.ScopeJobsRead},
		ParentRoleID: "role-parent",
	}})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestRBACInheritance_UpdateRoleRequiresGrantOfInheritedParentPermissions(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			switch id {
			case "role-child":
				return &domain.ProjectRole{
					ID:          "role-child",
					ProjectID:   "proj-rbac",
					Permissions: []string{domain.ScopeJobsRead},
				}, nil
			case "role-parent":
				return &domain.ProjectRole{
					ID:          "role-parent",
					ProjectID:   "proj-rbac",
					Permissions: []string{domain.ScopeJobsWrite},
				}, nil
			default:
				require.Failf(t, "test failure", "unexpected role lookup %q", id)
				return nil, nil
			}
		},
		UpdateProjectRoleFunc: func(_ context.Context, _ *domain.ProjectRole) error {
			require.Fail(t,

				"UpdateProjectRole must not run when parent grants permissions the caller lacks")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleUpdateRole(rbacGrantCtx(domain.ScopeJobsRead), &UpdateRoleInput{
		RoleID: "role-child",
		Body: updateRoleRequest{
			Name:         "child",
			Permissions:  []string{domain.ScopeJobsRead},
			ParentRoleID: "role-parent",
		},
	})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestRBACInheritance_AssignMemberRequiresGrantOfInheritedParentPermissions(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			switch id {
			case "role-child":
				return &domain.ProjectRole{
					ID:           "role-child",
					ProjectID:    "proj-rbac",
					Permissions:  []string{domain.ScopeJobsRead},
					ParentRoleID: "role-parent",
				}, nil
			case "role-parent":
				return &domain.ProjectRole{
					ID:          "role-parent",
					ProjectID:   "proj-rbac",
					Permissions: []string{domain.ScopeJobsWrite},
				}, nil
			default:
				require.Failf(t, "test failure", "unexpected role lookup %q", id)
				return nil, nil
			}
		},
		AssignMemberRoleFunc: func(_ context.Context, _ *domain.ProjectMemberRole) error {
			require.Fail(t,

				"AssignMemberRole must not run when parent grants permissions the caller lacks")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleAssignMember(rbacGrantCtx(domain.ScopeJobsRead), &AssignMemberInput{Body: assignMemberRequest{
		UserID: "user-target",
		RoleID: "role-child",
	}})
	require.True(
		t, isHumaStatusError(err, http.
			StatusForbidden,
		))

}

func TestRBACInheritance_BulkAssignRejectsInheritedParentEscalation(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetProjectRoleFunc: func(_ context.Context, id string) (*domain.ProjectRole, error) {
			switch id {
			case "role-child":
				return &domain.ProjectRole{
					ID:           "role-child",
					ProjectID:    "proj-rbac",
					Permissions:  []string{domain.ScopeJobsRead},
					ParentRoleID: "role-parent",
				}, nil
			case "role-parent":
				return &domain.ProjectRole{
					ID:          "role-parent",
					ProjectID:   "proj-rbac",
					Permissions: []string{domain.ScopeJobsWrite},
				}, nil
			default:
				require.Failf(t, "test failure", "unexpected role lookup %q", id)
				return nil, nil
			}
		},
		AssignMemberRoleFunc: func(_ context.Context, _ *domain.ProjectMemberRole) error {
			require.Fail(t,

				"AssignMemberRole must not run when parent grants permissions the caller lacks")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	out, err := srv.handleBulkAssignMembers(rbacGrantCtx(domain.ScopeJobsRead), &BulkAssignMembersInput{Body: bulkAssignMembersRequest{
		Items: []assignMemberRequest{{UserID: "user-target", RoleID: "role-child"}},
	}})
	require.NoError(t, err)

	body, ok := out.Body.(map[string]any)
	require.True(
		t, ok)

	raw, err := json.Marshal(body["results"])
	require.NoError(t, err)

	var results []bulkAssignMemberResult
	require.NoError(t, json.Unmarshal(raw, &results))
	require.False(t, len(results) !=
		1 || results[0].Status !=
		"error" || results[0].
		Error ==
		"")

}
