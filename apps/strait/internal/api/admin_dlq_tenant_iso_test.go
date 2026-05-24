package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

// TestTenantIso_AdminDLQ_Unmask_UsesScopedFetch verifies that the unmask
// handler routes the run lookup through getRunForAccess (single fetch +
// scoped) rather than the prior pattern of requireRunAccess + raw GetRun.
// Cross-project runs return 404 and the mutation does not run.
func TestTenantIso_AdminDLQ_Unmask_UsesScopedFetch(t *testing.T) {
	t.Parallel()

	getCalls := 0
	unmaskCalled := false
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			return &domain.JobRun{ID: id, ProjectID: "proj-other", JobID: "job-1", Status: domain.StatusDeadLetter}, nil
		},
		UnmaskDLQRunFunc: func(_ context.Context, _ string) error {
			unmaskCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/admin/dlq/run-foreign/unmask", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-project, got %d: %s", w.Code, w.Body.String())
	}
	if unmaskCalled {
		t.Fatal("UnmaskDLQRun must not run on cross-project access")
	}
	if getCalls != 1 {
		t.Fatalf("expected exactly 1 GetRun call (single scoped fetch), got %d", getCalls)
	}
}

// TestTenantIso_AdminDLQ_Purge_UsesScopedFetch is the symmetric cross-
// project test for the purge endpoint.
func TestTenantIso_AdminDLQ_Purge_UsesScopedFetch(t *testing.T) {
	t.Parallel()

	getCalls := 0
	purgeCalled := false
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getCalls++
			return &domain.JobRun{ID: id, ProjectID: "proj-other", JobID: "job-1", Status: domain.StatusDeadLetter}, nil
		},
		PurgeDLQRunFunc: func(_ context.Context, _ string) error {
			purgeCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/admin/dlq/run-foreign/purge", "", "proj-mine")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-project, got %d: %s", w.Code, w.Body.String())
	}
	if purgeCalled {
		t.Fatal("PurgeDLQRun must not run on cross-project access")
	}
	if getCalls != 1 {
		t.Fatalf("expected exactly 1 GetRun call (single scoped fetch), got %d", getCalls)
	}
}

func TestRequireAdminScope_UserOIDCMustPassProjectRBAC(t *testing.T) {
	t.Parallel()

	var permissionChecks int
	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, projectID, userID string) ([]string, error) {
			permissionChecks++
			if projectID != "proj-1" || userID != "user-1" {
				t.Fatalf("unexpected permission lookup: project=%q user=%q", projectID, userID)
			}
			return []string{domain.ScopeDLQPurge}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeDLQPurge})
	ctx = context.WithValue(ctx, ctxOIDCScopeClaimPresentKey, true)

	if err := srv.requireAdminScope(ctx, domain.ScopeDLQPurge); err == nil {
		t.Fatal("expected scoped OIDC admin request without project context to be rejected")
	}
	if permissionChecks != 0 {
		t.Fatalf("permission lookup should not run without project context, got %d calls", permissionChecks)
	}

	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	if err := srv.requireAdminScope(ctx, domain.ScopeDLQPurge); err != nil {
		t.Fatalf("expected scoped OIDC admin request with matching RBAC to pass, got %v", err)
	}
	if permissionChecks != 1 {
		t.Fatalf("expected one RBAC permission lookup, got %d", permissionChecks)
	}
}

func TestRequireAdminScope_UserOIDCScopesDoNotBypassRBAC(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{domain.ScopeJobsRead}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeDLQPurge})
	ctx = context.WithValue(ctx, ctxOIDCScopeClaimPresentKey, true)

	if err := srv.requireAdminScope(ctx, domain.ScopeDLQPurge); err == nil {
		t.Fatal("expected OIDC admin scope to remain bounded by project RBAC")
	}
}

func TestRequireAdminScope_APIKeyRequiresProjectContext(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeDLQReplay})

	if err := srv.requireAdminScope(ctx, domain.ScopeDLQReplay); err == nil {
		t.Fatal("expected API-key admin request without project context to be rejected")
	}

	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	if err := srv.requireAdminScope(ctx, domain.ScopeDLQReplay); err != nil {
		t.Fatalf("expected scoped API-key admin request with project context to pass, got %v", err)
	}
}
