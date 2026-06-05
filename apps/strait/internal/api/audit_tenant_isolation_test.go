package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// TestAuditList_CrossTenantIsolation verifies that a caller querying
// the audit log for a project they don't own never sees the other
// project's rows. The handler uses projectIDFromContext — the cross-
// tenant query param path is not exposed, but we assert the store is
// called with the authenticated project id, not whatever the caller
// might have passed.
func TestAuditList_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	var storeProjectID atomic.Value
	storeProjectID.Store("")
	ms := &APIStoreMock{
		ListAuditEventsFunc: func(_ context.Context, projectID, _, _, _ string, _ int, _, _, _ *time.Time, _ bool) ([]domain.AuditEvent, error) {
			storeProjectID.Store(projectID)
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/audit-events?limit=10", "", "proj-a")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(
		t, "proj-a",
		storeProjectID.Load().(string))

	// The store must have been called with proj-a (from the context),
	// not anything the URL might have attempted to inject.
}

// TestAuditList_InjectsFiltersParameterized verifies that resource_type
// and resource_id query params are passed as parameterized arguments —
// the store interface accepts them as strings, not concatenated into
// raw SQL, so injection is impossible at the API layer. This test
// locks the behavior in case someone refactors to string-concat.
func TestAuditList_InjectsFiltersParameterized(t *testing.T) {
	t.Parallel()

	var storeResourceID atomic.Value
	storeResourceID.Store("")
	ms := &APIStoreMock{
		ListAuditEventsFunc: func(_ context.Context, _, _, _, resourceID string, _ int, _, _, _ *time.Time, _ bool) ([]domain.AuditEvent, error) {
			storeResourceID.Store(resourceID)
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	// Attempt classic SQL injection payload — the store should receive
	// the literal value, not a parsed SQL fragment.
	payload := "'; DROP TABLE audit_events --"
	req := authedProjectRequest(http.MethodGet,
		"/v1/audit-events?resource_type=job&resource_id="+url.QueryEscape(payload),
		"", "proj-a")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	got := storeResourceID.Load().(string)
	assert.Equal(
		t, payload,
		got)
}

// TestAuditExport_CrossTenantIsolation verifies the export path also
// uses the context project id, not a caller-supplied one.
func TestAuditExport_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	var streamProject atomic.Value
	streamProject.Store("")
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, projectID, _, _ string, _, _ time.Time, _ func(*domain.AuditEvent) error) error {
			streamProject.Store(projectID)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-04-01T00:00:00Z&to=2026-04-11T00:00:00Z&format=ndjson",
		"", "proj-export")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(
		t, "proj-export",
		streamProject.Load().(string))
}

// TestAuditVerify_CrossTenantIsolation verifies the verify path uses
// the context project id.
func TestAuditVerify_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	var verifyProject atomic.Value
	verifyProject.Store("")
	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			verifyProject.Store(projectID)
			return &domain.AuditChainVerification{ProjectID: projectID, Valid: true}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/audit-events/verify", "", "proj-verify")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(
		t, "proj-verify",
		verifyProject.Load().(string))
}
