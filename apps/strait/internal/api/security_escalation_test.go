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

// S1: Internal secret auth rate limiting.

func TestInternalSecretAuth_RateLimitedAfterFailures(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithRedis(t, &APIStoreMock{})

	// Send 10 requests with wrong internal secret from the same IP.
	for i := range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("X-Internal-Secret", "wrong-secret")
		req.RemoteAddr = "10.0.1.50:9999"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: status = %d, want 401", i+1, w.Code)
		}
	}

	// 11th request should be rate limited (429).
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("X-Internal-Secret", "another-wrong-secret")
	req.RemoteAddr = "10.0.1.50:9999"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("11th request: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429 response")
	}
}

func TestInternalSecretAuth_DifferentIP_NotBlocked(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithRedis(t, &APIStoreMock{})

	// Exhaust lockout for one IP with bad internal secrets.
	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("X-Internal-Secret", "wrong-secret")
		req.RemoteAddr = "10.0.2.1:9999"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}

	// Different IP should not be blocked.
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("X-Internal-Secret", "wrong-secret")
	req.RemoteAddr = "10.0.2.2:9999"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Error("different IP should not be rate limited")
	}
}

func TestInternalSecretAuth_ValidSecret_NotRateLimited(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithRedis(t, &APIStoreMock{})

	// Valid secret should always succeed even after many requests.
	for i := range 20 {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		req.RemoteAddr = "10.0.3.1:9999"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: got 429 for valid secret", i+1)
		}
	}
}

// S2: RBAC privilege escalation -- handler-level tests.

func TestHandleCreateRole_WildcardDeniedWithoutWildcardScope(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_1"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	// Simulate an API key caller with rbac:manage but not wildcard.
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:key-1")

	input := &CreateRoleInput{
		Body: createRoleRequest{
			Name:        "superadmin",
			Description: "All access",
			Permissions: []string{"*"},
		},
	}

	_, err := srv.handleCreateRole(ctx, input)
	if err == nil {
		t.Fatal("expected error for wildcard escalation, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 Forbidden, got: %v", err)
	}
}

func TestHandleCreateRole_WildcardAllowedWithWildcardScope(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_2"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:key-1")

	input := &CreateRoleInput{
		Body: createRoleRequest{
			Name:        "superadmin",
			Description: "All access",
			Permissions: []string{"*"},
		},
	}

	out, err := srv.handleCreateRole(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.ID != "role_2" {
		t.Fatalf("role ID = %q, want %q", out.Body.ID, "role_2")
	}
}

func TestHandleCreateRole_InternalSecretBypassesEscalationCheck(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_3"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	// Internal secret auth does not set scopes (nil).
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	input := &CreateRoleInput{
		Body: createRoleRequest{
			Name:        "superadmin",
			Description: "All access",
			Permissions: []string{"*"},
		},
	}

	out, err := srv.handleCreateRole(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.ID != "role_3" {
		t.Fatalf("role ID = %q, want %q", out.Body.ID, "role_3")
	}
}

func TestHandleCreateRole_SpecificScopeEscalationBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	// Caller has rbac:manage and jobs:read, tries to create role with jobs:write.
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	input := &CreateRoleInput{
		Body: createRoleRequest{
			Name:        "writer",
			Permissions: []string{domain.ScopeJobsWrite},
		},
	}

	_, err := srv.handleCreateRole(ctx, input)
	if err == nil {
		t.Fatal("expected error for scope escalation, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got: %v", err)
	}
}

func TestHandleUpdateRole_WildcardDeniedWithoutWildcardScope(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, id string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: id, ProjectID: "proj-1", Name: "old", Permissions: []string{domain.ScopeJobsRead}}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	input := &UpdateRoleInput{
		RoleID: "role-1",
		Body: updateRoleRequest{
			Name:        "escalated",
			Permissions: []string{"*"},
		},
	}

	_, err := srv.handleUpdateRole(ctx, input)
	if err == nil {
		t.Fatal("expected error for wildcard escalation in update, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got: %v", err)
	}
}

func TestHandleAssignMember_SelfAssignmentBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: "role-1", Permissions: []string{domain.ScopeJobsRead}}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1") // same as user_id below

	input := &AssignMemberInput{
		Body: assignMemberRequest{
			UserID: "user-1",
			RoleID: "role-1",
		},
	}

	_, err := srv.handleAssignMember(ctx, input)
	if err == nil {
		t.Fatal("expected error for self-assignment, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got: %v", err)
	}
}

func TestHandleAssignMember_EscalationViaRoleBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: "role-admin", Permissions: []string{"*"}}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "caller-user")

	input := &AssignMemberInput{
		Body: assignMemberRequest{
			UserID: "other-user",
			RoleID: "role-admin",
		},
	}

	_, err := srv.handleAssignMember(ctx, input)
	if err == nil {
		t.Fatal("expected error for escalation via role assignment, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got: %v", err)
	}
}

func TestHandleBulkAssignMembers_BlocksSelfAssignmentAndEscalation(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, roleID string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: roleID, ProjectID: "proj-1", Permissions: []string{domain.ScopeAll}}, nil
	}
	ms.AssignMemberRoleFunc = func(context.Context, *domain.ProjectMemberRole) error {
		t.Fatal("AssignMemberRole must not be called for self-assignment or escalation")
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "caller-user")

	out, err := srv.handleBulkAssignMembers(ctx, &BulkAssignMembersInput{Body: bulkAssignMembersRequest{Items: []assignMemberRequest{
		{UserID: "caller-user", RoleID: "role-admin"},
		{UserID: "other-user", RoleID: "role-admin"},
	}}})
	if err != nil {
		t.Fatalf("handleBulkAssignMembers() error = %v", err)
	}
	body, ok := out.Body.(map[string]any)
	if !ok {
		t.Fatalf("response body type = %T, want map[string]any", out.Body)
	}
	results, ok := body["results"].([]bulkAssignMemberResult)
	if !ok {
		t.Fatalf("results type = %T, want []bulkAssignMemberResult", body["results"])
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, result := range results {
		if result.Status != "error" {
			t.Fatalf("bulk result for %s status = %q, want error", result.UserID, result.Status)
		}
	}
}

func TestHandleBulkAssignMembers_BlocksCrossProjectRole(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, roleID string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: roleID, ProjectID: "other-project", Permissions: []string{domain.ScopeJobsRead}}, nil
	}
	ms.AssignMemberRoleFunc = func(context.Context, *domain.ProjectMemberRole) error {
		t.Fatal("AssignMemberRole must not be called for a cross-project role")
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "caller-user")

	out, err := srv.handleBulkAssignMembers(ctx, &BulkAssignMembersInput{Body: bulkAssignMembersRequest{Items: []assignMemberRequest{
		{UserID: "other-user", RoleID: "role-other-project"},
	}}})
	if err != nil {
		t.Fatalf("handleBulkAssignMembers() error = %v", err)
	}
	results := out.Body.(map[string]any)["results"].([]bulkAssignMemberResult)
	if results[0].Status != "error" || results[0].Error != "role not found" {
		t.Fatalf("result = %+v, want role not found error", results[0])
	}
}

func TestHandleAssignMember_InternalSecretAllowsWildcardRole(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return &domain.ProjectRole{ID: "role-admin", Permissions: []string{"*"}}, nil
	}
	ms.AssignMemberRoleFunc = func(_ context.Context, _ *domain.ProjectMemberRole) error {
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	// Internal secret: no scopes in context.
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	input := &AssignMemberInput{
		Body: assignMemberRequest{
			UserID: "user-1",
			RoleID: "role-admin",
		},
	}

	out, err := srv.handleAssignMember(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.RoleID != "role-admin" {
		t.Fatalf("role = %q, want %q", out.Body.RoleID, "role-admin")
	}
}

func TestHandleCreateResourcePolicy_BlocksCrossProjectAndEscalation(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateResourcePolicyFunc: func(context.Context, *domain.ResourcePolicy) error {
			t.Fatal("CreateResourcePolicy must not be called")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateResourcePolicy(ctx, &CreateResourcePolicyInput{Body: createResourcePolicyRequest{
		ProjectID:    "other-project",
		ResourceType: "job",
		ResourceID:   "job-1",
		UserID:       "user-1",
		Actions:      []string{domain.ScopeJobsRead},
	}})
	if err == nil || !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("cross-project resource policy error = %v, want 403", err)
	}

	_, err = srv.handleCreateResourcePolicy(ctx, &CreateResourcePolicyInput{Body: createResourcePolicyRequest{
		ProjectID:    "proj-1",
		ResourceType: "job",
		ResourceID:   "job-1",
		UserID:       "user-1",
		Actions:      []string{domain.ScopeJobsWrite},
	}})
	if err == nil || !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("escalating resource policy error = %v, want 403", err)
	}
}

func TestHandleCreateTagPolicy_BlocksCrossProjectAndEscalation(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateTagPolicyFunc: func(context.Context, *domain.TagPolicy) error {
			t.Fatal("CreateTagPolicy must not be called")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateTagPolicy(ctx, &CreateTagPolicyInput{Body: createTagPolicyRequest{
		ProjectID:    "other-project",
		ResourceType: "job",
		UserID:       "user-1",
		TagKey:       "team",
		TagValue:     "payments",
		Actions:      []string{domain.ScopeJobsRead},
	}})
	if err == nil || !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("cross-project tag policy error = %v, want 403", err)
	}

	_, err = srv.handleCreateTagPolicy(ctx, &CreateTagPolicyInput{Body: createTagPolicyRequest{
		ProjectID:    "proj-1",
		ResourceType: "job",
		UserID:       "user-1",
		TagKey:       "team",
		TagValue:     "payments",
		Actions:      []string{domain.ScopeJobsWrite},
	}})
	if err == nil || !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("escalating tag policy error = %v, want 403", err)
	}
}

// S3: API key scope escalation -- handler-level tests.

func TestHandleCreateAPIKey_WildcardScopeDeniedWithoutWildcard(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	input := &CreateAPIKeyInput{
		Body: CreateAPIKeyRequest{
			ProjectID: "proj-1",
			Name:      "escalated key",
			Scopes:    []string{"*"},
		},
	}

	_, err := srv.handleCreateAPIKey(ctx, input)
	if err == nil {
		t.Fatal("expected error for wildcard scope escalation, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got: %v", err)
	}
}

func TestHandleCreateAPIKey_WildcardScopeAllowedWithWildcard(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-new"
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{"*"})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	input := &CreateAPIKeyInput{
		Body: CreateAPIKeyRequest{
			ProjectID: "proj-1",
			Name:      "admin key",
			Scopes:    []string{"*"},
		},
	}

	out, err := srv.handleCreateAPIKey(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.ID != "key-new" {
		t.Fatalf("key ID = %q, want %q", out.Body.ID, "key-new")
	}
}

func TestHandleCreateAPIKey_ScopeEscalationDenied(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, nil, nil)

	// Caller has api-keys:manage and jobs:read, tries to create key with jobs:write.
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	input := &CreateAPIKeyInput{
		Body: CreateAPIKeyRequest{
			ProjectID: "proj-1",
			Name:      "sneaky key",
			Scopes:    []string{domain.ScopeJobsWrite},
		},
	}

	_, err := srv.handleCreateAPIKey(ctx, input)
	if err == nil {
		t.Fatal("expected error for scope escalation, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got: %v", err)
	}
}

func TestHandleCreateAPIKey_SubsetScopesAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-sub"
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	input := &CreateAPIKeyInput{
		Body: CreateAPIKeyRequest{
			ProjectID: "proj-1",
			Name:      "subset key",
			Scopes:    []string{domain.ScopeJobsRead},
		},
	}

	out, err := srv.handleCreateAPIKey(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.ID != "key-sub" {
		t.Fatalf("key ID = %q, want %q", out.Body.ID, "key-sub")
	}
}

func TestHandleCreateAPIKey_InternalSecretBypassesEscalationCheck(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-internal"
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	// Internal secret: no scopes in context (nil).
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	input := &CreateAPIKeyInput{
		Body: CreateAPIKeyRequest{
			ProjectID: "proj-1",
			Name:      "internal key",
			Scopes:    []string{"*"},
		},
	}

	out, err := srv.handleCreateAPIKey(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.ID != "key-internal" {
		t.Fatalf("key ID = %q, want %q", out.Body.ID, "key-internal")
	}
}

// S2: User with DB permissions (OIDC empty scopes) escalation check.

func TestHandleCreateRole_UserDBPermissions_EscalationBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeRBACManage, domain.ScopeJobsRead}, nil
	}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_x"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	// OIDC user with empty scopes -- triggers DB permission lookup.
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")

	input := &CreateRoleInput{
		Body: createRoleRequest{
			Name:        "power role",
			Permissions: []string{"*"},
		},
	}

	_, err := srv.handleCreateRole(ctx, input)
	if err == nil {
		t.Fatal("expected error for wildcard escalation via DB permissions, got nil")
	}
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got: %v", err)
	}
}

func TestHandleCreateRole_UserDBPermissions_SubsetAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return []string{domain.ScopeRBACManage, domain.ScopeJobsRead, domain.ScopeJobsWrite}, nil
	}
	ms.CreateProjectRoleFunc = func(_ context.Context, role *domain.ProjectRole) error {
		role.ID = "role_y"
		return nil
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")

	input := &CreateRoleInput{
		Body: createRoleRequest{
			Name:        "reader role",
			Permissions: []string{domain.ScopeJobsRead},
		},
	}

	out, err := srv.handleCreateRole(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.ID != "role_y" {
		t.Fatalf("role ID = %q, want %q", out.Body.ID, "role_y")
	}
}

// isHumaStatusError checks if the error has a GetStatus() method returning the expected code.
func isHumaStatusError(err error, status int) bool {
	type hasStatus interface {
		GetStatus() int
	}
	if se, ok := err.(hasStatus); ok {
		return se.GetStatus() == status
	}
	return false
}

// Verify existing tests still pass: handleAssignMember with role not found.
func TestHandleAssignMember_RoleNotFound_WithEscalationCheck(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetProjectRoleFunc = func(_ context.Context, _ string) (*domain.ProjectRole, error) {
		return nil, store.ErrRoleNotFound
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "caller")

	input := &AssignMemberInput{
		Body: assignMemberRequest{
			UserID: "other-user",
			RoleID: "nonexistent",
		},
	}

	_, err := srv.handleAssignMember(ctx, input)
	if err == nil {
		t.Fatal("expected error for missing role, got nil")
	}
	if !isHumaStatusError(err, http.StatusBadRequest) {
		// Check error message contains expected text.
		errMsg := err.Error()
		if !json.Valid([]byte(errMsg)) {
			// Not JSON, check raw error string.
			if errMsg != "role not found" {
				t.Logf("got error: %v (type: %T)", err, err)
			}
		}
	}
}
