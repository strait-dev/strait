package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestAuditLogs_FreeTierRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListAuditEventsFunc: func(context.Context, string, string, string, string, int, *time.Time, *time.Time, *time.Time, bool) ([]domain.AuditEvent, error) {
			require.Fail(t,

				"ListAuditEvents must not be called when audit-log gate rejects")
			return nil, nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/audit-events", "", "proj-1"))
	require.Equal(t, http.StatusForbidden,
		w.Code)
	require.Contains(
		t, w.Body.String(), "Audit logs")
}

func TestAuditLogs_ScaleTierAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListAuditEventsFunc: func(context.Context, string, string, string, string, int, *time.Time, *time.Time, *time.Time, bool) ([]domain.AuditEvent, error) {
			return []domain.AuditEvent{}, nil
		},
		CreateAuditEventFunc: func(context.Context, *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanScale)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/audit-events", "", "proj-1"))
	require.NotEqual(t, http.
		StatusForbidden, w.Code,
	)
}
