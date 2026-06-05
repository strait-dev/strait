package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fmt"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// newWorkflowIsolationStore creates a mock store with workflows and workflow runs
// scoped to projectA and projectB for tenant isolation testing.
func newWorkflowIsolationStore() *APIStoreMock {
	now := time.Now()
	return &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			switch id {
			case "wf-a":
				return &domain.Workflow{ID: "wf-a", ProjectID: projectA, Name: "Workflow A", Slug: "wf-a", Version: 1, CreatedAt: now, UpdatedAt: now}, nil
			case "wf-b":
				return &domain.Workflow{ID: "wf-b", ProjectID: projectB, Name: "Workflow B", Slug: "wf-b", Version: 1, CreatedAt: now, UpdatedAt: now}, nil
			}
			return nil, fmt.Errorf("workflow not found")
		},
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			switch id {
			case "wfr-a":
				return &domain.WorkflowRun{ID: "wfr-a", ProjectID: projectA, WorkflowID: "wf-a", WorkflowVersion: 1, Status: domain.WfStatusFailed, CreatedAt: now}, nil
			case "wfr-a2":
				return &domain.WorkflowRun{ID: "wfr-a2", ProjectID: projectA, WorkflowID: "wf-a", WorkflowVersion: 1, Status: domain.WfStatusCompleted, CreatedAt: now}, nil
			case "wfr-b":
				return &domain.WorkflowRun{ID: "wfr-b", ProjectID: projectB, WorkflowID: "wf-b", WorkflowVersion: 1, Status: domain.WfStatusFailed, CreatedAt: now}, nil
			}
			return nil, fmt.Errorf("workflow run not found")
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", StepRef: "step-one", WorkflowID: "wf-a"},
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{}, nil
		},
		CreateCanaryDeploymentFunc: func(_ context.Context, _ *domain.CanaryDeployment) error {
			return nil
		},
		GetActiveCanaryDeploymentFunc: func(_ context.Context, wfID string) (*domain.CanaryDeployment, error) {
			switch wfID {
			case "wf-a":
				return &domain.CanaryDeployment{ID: "canary-a", WorkflowID: "wf-a", ProjectID: projectA, SourceVersion: 1, TargetVersion: 2, TrafficPct: 10, Status: "active"}, nil
			case "wf-b":
				return &domain.CanaryDeployment{ID: "canary-b", WorkflowID: "wf-b", ProjectID: projectB, SourceVersion: 1, TargetVersion: 2, TrafficPct: 10, Status: "active"}, nil
			}
			return nil, store.ErrCanaryNotFound
		},
		UpdateCanaryDeploymentTrafficFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CompleteCanaryDeploymentFunc: func(_ context.Context, _ string, _ string) error {
			return nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _ domain.WorkflowRunStatus, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}
}

// B1: Canary deployment handlers.

func TestTenantIsolation_CreateCanaryDeployment_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"workflow_id":"wf-a","source_version":1,"target_version":2,"traffic_pct":10}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/canary-deployments", body, projectA))
	require.Equal(t, http.StatusCreated,
		w.Code)
}

func TestTenantIsolation_CreateCanaryDeployment_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to create a canary for wf-a (owned by project A).
	body := `{"workflow_id":"wf-a","source_version":1,"target_version":2,"traffic_pct":10}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/canary-deployments", body, projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_UpdateCanaryDeployment_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"traffic_pct":50}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-a/canary", body, projectA))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestTenantIsolation_UpdateCanaryDeployment_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"traffic_pct":50}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-a/canary", body, projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_RollbackCanaryDeployment_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/canary/rollback", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestTenantIsolation_RollbackCanaryDeployment_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/canary/rollback", "", projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_GetCanaryStatus_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows/wf-a/canary", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestTenantIsolation_GetCanaryStatus_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows/wf-a/canary", "", projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

// B2: Compensation handlers.

func TestTenantIsolation_CompensateWorkflowRun_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/compensate", "", projectA))
	require.NotEqual(t, http.
		StatusNotFound, w.Code,
	)

	// May return 400 (no steps require compensation) since our mock has empty step runs.
	// The key check: it must NOT be 404 for own-project.
}

func TestTenantIsolation_CompensateWorkflowRun_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/compensate", "", projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_GetCompensationPlan_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compensation-plan", "", projectA))
	require.NotEqual(t, http.
		StatusForbidden, w.Code,
	)

	// May return 404 "no compensation plan available" since mock has empty step runs.
	// The important thing is it must not reject for project mismatch reasons.
}

func TestTenantIsolation_GetCompensationPlan_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compensation-plan", "", projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

// B3: Debug handlers.

func TestTenantIsolation_GetWorkflowRunDebug_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/debug", "", projectA))
	require.NotEqual(t, http.
		StatusNotFound, w.Code,
	)
}

func TestTenantIsolation_GetWorkflowRunDebug_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/debug", "", projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_CompareWorkflowRuns_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compare/wfr-a2", "", projectA))
	require.NotEqual(t, http.
		StatusNotFound, w.Code,
	)
}

func TestTenantIsolation_CompareWorkflowRuns_CrossProject_RunA(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to compare wfr-a (project A) with wfr-b (project B).
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compare/wfr-b", "", projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_CompareWorkflowRuns_CrossProject_RunB(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project A tries to compare wfr-a (own) with wfr-b (project B).
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compare/wfr-b", "", projectA))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_CompareWorkflowRuns_ResourceScopedUserRequiresOtherRun(t *testing.T) {
	t.Parallel()

	ms := newWorkflowIsolationStore()
	ms.GetUserPermissionsFunc = func(context.Context, string, string) ([]string, error) {
		return nil, nil
	}
	ms.GetResourcePoliciesFunc = func(_ context.Context, projectID, resourceType, resourceID, userID string) ([]string, error) {
		require.False(t, projectID !=
			projectA || resourceType !=
			"workflow_run" ||
			userID != "user-1")

		if resourceID == "wfr-a" {
			return []string{domain.ScopeRunsRead}, nil
		}
		return nil, nil
	}
	ms.ListStepRunsByWorkflowRunFunc = func(context.Context, string, int, *time.Time) ([]domain.WorkflowStepRun, error) {
		require.Fail(t,

			"ListStepRunsByWorkflowRun must not run when otherRunID lacks resource access")
		return nil, nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectA)
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")

	_, err := srv.handleCompareWorkflowRuns(ctx, &CompareWorkflowRunsInput{
		WorkflowRunID: "wfr-a",
		OtherRunID:    "wfr-a2",
	})
	require.True(
		t, isHumaStatusError(err, http.StatusForbidden))
}

func TestTenantIsolation_CompareWorkflowRuns_ResourceScopedUserWithBothRunsAllowed(t *testing.T) {
	t.Parallel()

	ms := newWorkflowIsolationStore()
	ms.GetUserPermissionsFunc = func(context.Context, string, string) ([]string, error) {
		return nil, nil
	}
	ms.GetResourcePoliciesFunc = func(_ context.Context, projectID, resourceType, resourceID, userID string) ([]string, error) {
		require.False(t, projectID !=
			projectA || resourceType !=
			"workflow_run" ||
			userID != "user-1")

		switch resourceID {
		case "wfr-a", "wfr-a2":
			return []string{domain.ScopeRunsRead}, nil
		default:
			return nil, nil
		}
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectA)
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")

	out, err := srv.handleCompareWorkflowRuns(ctx, &CompareWorkflowRunsInput{
		WorkflowRunID: "wfr-a",
		OtherRunID:    "wfr-a2",
	})
	require.NoError(t, err)
	require.False(t, out == nil ||
		out.Body == nil,
	)
}

// B4: Simulate workflow.

func TestTenantIsolation_SimulateWorkflow_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/simulate", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestTenantIsolation_SimulateWorkflow_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/simulate", "", projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

// Adversarial: non-existent resource IDs return 404 regardless.

func TestTenantIsolation_Canary_NonexistentWorkflow(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"workflow_id":"wf-nonexistent","source_version":1,"target_version":2,"traffic_pct":10}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/canary-deployments", body, projectA))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_Debug_NonexistentRun(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-nonexistent/debug", "", projectA))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestTenantIsolation_Simulate_NonexistentWorkflow(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-nonexistent/simulate", "", projectA))
	require.Equal(t, http.StatusNotFound,
		w.Code)
}
