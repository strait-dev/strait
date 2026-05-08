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
