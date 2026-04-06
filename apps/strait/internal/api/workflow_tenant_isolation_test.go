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
	if w.Code != http.StatusCreated {
		t.Fatalf("own-project create canary: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_CreateCanaryDeployment_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to create a canary for wf-a (owned by project A).
	body := `{"workflow_id":"wf-a","source_version":1,"target_version":2,"traffic_pct":10}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/canary-deployments", body, projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project create canary: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_UpdateCanaryDeployment_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"traffic_pct":50}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-a/canary", body, projectA))
	if w.Code != http.StatusOK {
		t.Fatalf("own-project update canary: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_UpdateCanaryDeployment_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"traffic_pct":50}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-a/canary", body, projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project update canary: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_RollbackCanaryDeployment_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/canary/rollback", "", projectA))
	if w.Code != http.StatusOK {
		t.Fatalf("own-project rollback canary: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_RollbackCanaryDeployment_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/canary/rollback", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project rollback canary: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_GetCanaryStatus_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows/wf-a/canary", "", projectA))
	if w.Code != http.StatusOK {
		t.Fatalf("own-project get canary status: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_GetCanaryStatus_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows/wf-a/canary", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project get canary status: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// B2: Compensation handlers.

func TestTenantIsolation_CompensateWorkflowRun_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/compensate", "", projectA))
	// May return 400 (no steps require compensation) since our mock has empty step runs.
	// The key check: it must NOT be 404 for own-project.
	if w.Code == http.StatusNotFound {
		t.Fatalf("own-project compensate: should not return 404, got: %s", w.Body.String())
	}
}

func TestTenantIsolation_CompensateWorkflowRun_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/compensate", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project compensate: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_GetCompensationPlan_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compensation-plan", "", projectA))
	// May return 404 "no compensation plan available" since mock has empty step runs.
	// The important thing is it must not reject for project mismatch reasons.
	if w.Code == http.StatusForbidden {
		t.Fatalf("own-project compensation plan: should not return 403, got: %s", w.Body.String())
	}
}

func TestTenantIsolation_GetCompensationPlan_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compensation-plan", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project compensation plan: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// B3: Debug handlers.

func TestTenantIsolation_GetWorkflowRunDebug_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/debug", "", projectA))
	if w.Code == http.StatusNotFound {
		t.Fatalf("own-project debug: should not return 404, got: %s", w.Body.String())
	}
}

func TestTenantIsolation_GetWorkflowRunDebug_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/debug", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project debug: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_CompareWorkflowRuns_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compare/wfr-a2", "", projectA))
	if w.Code == http.StatusNotFound {
		t.Fatalf("own-project compare: should not return 404, got: %s", w.Body.String())
	}
}

func TestTenantIsolation_CompareWorkflowRuns_CrossProject_RunA(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to compare wfr-a (project A) with wfr-b (project B).
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compare/wfr-b", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project compare (run A foreign): expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_CompareWorkflowRuns_CrossProject_RunB(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project A tries to compare wfr-a (own) with wfr-b (project B).
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/compare/wfr-b", "", projectA))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project compare (run B foreign): expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// B4: Simulate workflow.

func TestTenantIsolation_SimulateWorkflow_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/simulate", "", projectA))
	if w.Code != http.StatusOK {
		t.Fatalf("own-project simulate: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_SimulateWorkflow_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-a/simulate", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project simulate: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// Adversarial: non-existent resource IDs return 404 regardless.

func TestTenantIsolation_Canary_NonexistentWorkflow(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"workflow_id":"wf-nonexistent","source_version":1,"target_version":2,"traffic_pct":10}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/canary-deployments", body, projectA))
	if w.Code != http.StatusNotFound {
		t.Fatalf("nonexistent workflow create canary: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_Debug_NonexistentRun(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-nonexistent/debug", "", projectA))
	if w.Code != http.StatusNotFound {
		t.Fatalf("nonexistent run debug: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_Simulate_NonexistentWorkflow(t *testing.T) {
	t.Parallel()
	ms := newWorkflowIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflows/wf-nonexistent/simulate", "", projectA))
	if w.Code != http.StatusNotFound {
		t.Fatalf("nonexistent workflow simulate: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
