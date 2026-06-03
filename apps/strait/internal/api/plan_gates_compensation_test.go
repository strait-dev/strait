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

func TestCompensationPlan_FreeTierRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:              id,
				ProjectID:       "proj-1",
				WorkflowID:      "wf-1",
				WorkflowVersion: 1,
				Status:          domain.WfStatusFailed,
			}, nil
		},
		ListStepsByWorkflowVersionFunc: func(context.Context, string, int) ([]domain.WorkflowStep, error) {
			t.Fatal("ListStepsByWorkflowVersion must not be called when compensation-plan gate rejects")
			return nil, nil
		},
		ListStepRunsByWorkflowRunFunc: func(context.Context, string, int, *time.Time) ([]domain.WorkflowStepRun, error) {
			t.Fatal("ListStepRunsByWorkflowRun must not be called when compensation-plan gate rejects")
			return nil, nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-1/compensation-plan", "", "proj-1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("free-tier compensation plan must be 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Compensating transactions") {
		t.Fatalf("rejection must name the feature, got: %s", w.Body.String())
	}
}
