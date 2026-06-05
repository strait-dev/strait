package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

func TestAuditLogs_FreeTierRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListAuditEventsFunc: func(context.Context, string, string, string, string, int, *time.Time, *time.Time, *time.Time, bool) ([]domain.AuditEvent, error) {
			t.Fatal("ListAuditEvents must not be called when audit-log gate rejects")
			return nil, nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/audit-events", "", "proj-1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("free-tier audit log access must be 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Audit logs") {
		t.Fatalf("rejection must name the feature, got: %s", w.Body.String())
	}
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

	if w.Code == http.StatusForbidden {
		t.Fatalf("scale-tier audit log access must pass the feature gate, got 403: %s", w.Body.String())
	}
}
