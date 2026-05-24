package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"strait/internal/domain"
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
			if id != "role-parent" {
				t.Fatalf("unexpected role lookup %q", id)
			}
			return &domain.ProjectRole{
				ID:          "role-parent",
				ProjectID:   "proj-rbac",
				Permissions: []string{domain.ScopeJobsWrite},
			}, nil
		},
		CreateProjectRoleFunc: func(_ context.Context, _ *domain.ProjectRole) error {
			t.Fatal("CreateProjectRole must not run when parent grants permissions the caller lacks")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleCreateRole(rbacGrantCtx(domain.ScopeJobsRead), &CreateRoleInput{Body: createRoleRequest{
		Name:         "child",
		Permissions:  []string{domain.ScopeJobsRead},
		ParentRoleID: "role-parent",
	}})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
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
				t.Fatalf("unexpected role lookup %q", id)
				return nil, nil
			}
		},
		UpdateProjectRoleFunc: func(_ context.Context, _ *domain.ProjectRole) error {
			t.Fatal("UpdateProjectRole must not run when parent grants permissions the caller lacks")
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
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
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
				t.Fatalf("unexpected role lookup %q", id)
				return nil, nil
			}
		},
		AssignMemberRoleFunc: func(_ context.Context, _ *domain.ProjectMemberRole) error {
			t.Fatal("AssignMemberRole must not run when parent grants permissions the caller lacks")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleAssignMember(rbacGrantCtx(domain.ScopeJobsRead), &AssignMemberInput{Body: assignMemberRequest{
		UserID: "user-target",
		RoleID: "role-child",
	}})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
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
				t.Fatalf("unexpected role lookup %q", id)
				return nil, nil
			}
		},
		AssignMemberRoleFunc: func(_ context.Context, _ *domain.ProjectMemberRole) error {
			t.Fatal("AssignMemberRole must not run when parent grants permissions the caller lacks")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	out, err := srv.handleBulkAssignMembers(rbacGrantCtx(domain.ScopeJobsRead), &BulkAssignMembersInput{Body: bulkAssignMembersRequest{
		Items: []assignMemberRequest{{UserID: "user-target", RoleID: "role-child"}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, ok := out.Body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T", out.Body)
	}
	raw, err := json.Marshal(body["results"])
	if err != nil {
		t.Fatalf("marshal results: %v", err)
	}
	var results []bulkAssignMemberResult
	if err := json.Unmarshal(raw, &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	if len(results) != 1 || results[0].Status != "error" || results[0].Error == "" {
		t.Fatalf("results = %+v, want inherited permission error", results)
	}
}
