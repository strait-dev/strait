package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func activeCanary(workflowID string) *domain.CanaryDeployment {
	return &domain.CanaryDeployment{
		ID:            "canary-1",
		WorkflowID:    workflowID,
		ProjectID:     "proj-1",
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    50,
		Status:        "active",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func TestHandleUpdateCanaryDeployment_CompletesAt100Pct(t *testing.T) {
	t.Parallel()

	var completeCalledWith string
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "Workflow", Slug: "workflow", Version: 2}, nil
		},
		GetActiveCanaryDeploymentFunc: func(_ context.Context, workflowID string) (*domain.CanaryDeployment, error) {
			return activeCanary(workflowID), nil
		},
		UpdateCanaryDeploymentTrafficFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CompleteCanaryDeploymentFunc: func(_ context.Context, _ string, status string) error {
			completeCalledWith = status
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanScale)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-1/canary", `{"traffic_pct":100}`, "proj-1"))

	require.Equal(t, http.StatusOK, w.Code, "expected 200 OK, body: %s", w.Body.String())

	var resp domain.CanaryDeployment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotEmpty(t, resp.ID, "response body must not be null or empty")
	assert.Equal(t, "completed", resp.Status)
	assert.Equal(t, 100, resp.TrafficPct)
	assert.Equal(t, "completed", completeCalledWith, "CompleteCanaryDeployment must be called with status 'completed'")
}

func TestHandleUpdateCanaryDeployment_PartialUpdate(t *testing.T) {
	t.Parallel()

	var completeCalled bool
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "Workflow", Slug: "workflow", Version: 2}, nil
		},
		GetActiveCanaryDeploymentFunc: func(_ context.Context, workflowID string) (*domain.CanaryDeployment, error) {
			return activeCanary(workflowID), nil
		},
		UpdateCanaryDeploymentTrafficFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CompleteCanaryDeploymentFunc: func(_ context.Context, _ string, _ string) error {
			completeCalled = true
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanScale)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-1/canary", `{"traffic_pct":50}`, "proj-1"))

	require.Equal(t, http.StatusOK, w.Code, "expected 200 OK, body: %s", w.Body.String())

	var resp domain.CanaryDeployment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "active", resp.Status)
	assert.Equal(t, 50, resp.TrafficPct)
	assert.False(t, completeCalled, "CompleteCanaryDeployment must not be called when traffic_pct < 100")
}

func TestHandleUpdateCanaryDeployment_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "Workflow", Slug: "workflow", Version: 2}, nil
		},
		GetActiveCanaryDeploymentFunc: func(_ context.Context, _ string) (*domain.CanaryDeployment, error) {
			return nil, store.ErrCanaryNotFound
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanScale)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-1/canary", `{"traffic_pct":50}`, "proj-1"))

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "no active canary deployment")
}
