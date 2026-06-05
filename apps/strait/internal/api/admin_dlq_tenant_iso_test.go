package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, unmaskCalled)
	require.Equal(t, 1, getCalls)
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
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, purgeCalled)
	require.Equal(t, 1, getCalls)
}

func TestRequireAdminScope_UserOIDCMustPassProjectRBAC(t *testing.T) {
	t.Parallel()

	var permissionChecks int
	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, projectID, userID string) ([]string, error) {
			permissionChecks++
			require.False(t, projectID !=
				"proj-1" || userID !=
				"user-1",
			)

			return []string{domain.ScopeDLQPurge}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeDLQPurge})
	ctx = context.WithValue(ctx, ctxOIDCScopeClaimPresentKey, true)
	require.Error(t, srv.requireAdminScope(ctx, domain.
		ScopeDLQPurge,
	))
	require.Equal(t, 0, permissionChecks)

	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	require.NoError(t, srv.requireAdminScope(ctx,
		domain.ScopeDLQPurge,
	))
	require.Equal(t, 1, permissionChecks)
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
	require.Error(t, srv.requireAdminScope(ctx, domain.
		ScopeDLQPurge,
	))
}

func TestRequireAdminScope_APIKeyRequiresProjectContext(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeDLQReplay})
	require.Error(t, srv.requireAdminScope(ctx, domain.
		ScopeDLQReplay,
	))

	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	require.NoError(t, srv.requireAdminScope(ctx,
		domain.ScopeDLQReplay,
	))
}
