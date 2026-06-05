package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// newWorkflowRunIsolationStore creates a mock store with workflow runs scoped
// to projectA and projectB for tenant isolation testing of workflow run handlers.
func newWorkflowRunIsolationStore() *APIStoreMock {
	now := time.Now()
	startedAt := now.Add(-5 * time.Minute)
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
				return &domain.WorkflowRun{ID: "wfr-a", ProjectID: projectA, WorkflowID: "wf-a", WorkflowVersion: 1, Status: domain.WfStatusRunning, StartedAt: &startedAt, CreatedAt: now}, nil
			case "wfr-a-paused":
				return &domain.WorkflowRun{ID: "wfr-a-paused", ProjectID: projectA, WorkflowID: "wf-a", WorkflowVersion: 1, Status: domain.WfStatusPaused, StartedAt: &startedAt, CreatedAt: now}, nil
			case "wfr-a-failed":
				return &domain.WorkflowRun{ID: "wfr-a-failed", ProjectID: projectA, WorkflowID: "wf-a", WorkflowVersion: 1, Status: domain.WfStatusFailed, StartedAt: &startedAt, CreatedAt: now}, nil
			case "wfr-b":
				return &domain.WorkflowRun{ID: "wfr-b", ProjectID: projectB, WorkflowID: "wf-b", WorkflowVersion: 1, Status: domain.WfStatusRunning, StartedAt: &startedAt, CreatedAt: now}, nil
			case "wfr-b-paused":
				return &domain.WorkflowRun{ID: "wfr-b-paused", ProjectID: projectB, WorkflowID: "wf-b", WorkflowVersion: 1, Status: domain.WfStatusPaused, StartedAt: &startedAt, CreatedAt: now}, nil
			case "wfr-b-failed":
				return &domain.WorkflowRun{ID: "wfr-b-failed", ProjectID: projectB, WorkflowID: "wf-b", WorkflowVersion: 1, Status: domain.WfStatusFailed, StartedAt: &startedAt, CreatedAt: now}, nil
			}
			return nil, fmt.Errorf("workflow run not found")
		},
		ListWorkflowRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{}, nil
		},
		ListWorkflowRunLabelsFunc: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{"env": "prod"}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", StepRef: "step-one", WorkflowID: "wf-a"},
			}, nil
		},
		ListWorkflowStepDecisionsFunc: func(_ context.Context, _ string, _ string, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepDecision, error) {
			return []domain.WorkflowStepDecision{}, nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _ domain.WorkflowRunStatus, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		CancelNonTerminalStepRunsFunc: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
		CancelJobRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
		CancelEventTriggersByWorkflowRunFunc: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
		MarkJobRunsPausedByWorkflowRunFunc: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
		GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", StepRef: "step-one", Status: domain.StepFailed}, nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		GetWorkflowStepApprovalByStepRunIDFunc: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
			return &domain.WorkflowStepApproval{ID: "approval-1"}, nil
		},
	}
}

func newWorkflowRunTestServerWithCallback(t *testing.T, s APIStore) *Server {
	t.Helper()
	trigger := &mockWorkflowTrigger{
		retryWorkflowRunFn: func(_ context.Context, originalRunID string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wfr-new", ProjectID: projectA, WorkflowID: "wf-a", Status: domain.WfStatusRunning}, nil
		},
	}
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:           cfg,
		Store:            s,
		Queue:            &mockQueue{},
		WorkflowCallback: trigger,
		WorkflowEngine:   trigger,
		Edition:          domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

// handleListWorkflowRuns tenant isolation.

func TestTenantIsolation_ListWorkflowRuns_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows/wf-a/runs", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_ListWorkflowRuns_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows/wf-a/runs", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleGetWorkflowRun tenant isolation.

func TestTenantIsolation_GetWorkflowRun_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_GetWorkflowRun_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleCancelWorkflowRun tenant isolation.

func TestTenantIsolation_CancelWorkflowRun_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/workflow-runs/wfr-a", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_CancelWorkflowRun_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/workflow-runs/wfr-a", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handlePauseWorkflowRun tenant isolation.

func TestTenantIsolation_PauseWorkflowRun_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/pause", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_PauseWorkflowRun_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/pause", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleResumeWorkflowRun tenant isolation.

func TestTenantIsolation_ResumeWorkflowRun_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a-paused/resume", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_ResumeWorkflowRun_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a-paused/resume", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleGetWorkflowRunLabels tenant isolation.

func TestTenantIsolation_GetWorkflowRunLabels_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/labels", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_GetWorkflowRunLabels_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/labels", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleListWorkflowStepRuns tenant isolation.

func TestTenantIsolation_ListWorkflowStepRuns_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/steps", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_ListWorkflowStepRuns_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/steps", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleGetWorkflowRunGraph tenant isolation.

func TestTenantIsolation_GetWorkflowRunGraph_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/graph", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_GetWorkflowRunGraph_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/graph", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleGetWorkflowRunExplain tenant isolation.

func TestTenantIsolation_GetWorkflowRunExplain_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/explain", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_GetWorkflowRunExplain_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/explain", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleGetWorkflowRunTimeline tenant isolation.

func TestTenantIsolation_GetWorkflowRunTimeline_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/timeline", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_GetWorkflowRunTimeline_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-a/timeline", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleRetryWorkflowRun tenant isolation.

func TestTenantIsolation_RetryWorkflowRun_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a-failed/retry", "", projectA))
	require.Equal(t, http.StatusCreated,

		w.Code)

}

func TestTenantIsolation_RetryWorkflowRun_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a-failed/retry", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleRetryWorkflowStep tenant isolation.

func TestTenantIsolation_RetryWorkflowStep_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/steps/step-one/retry", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_RetryWorkflowStep_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/steps/step-one/retry", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleReplayWorkflowSubtree tenant isolation.

func TestTenantIsolation_ReplayWorkflowSubtree_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/steps/step-one/replay-subtree", "", projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_ReplayWorkflowSubtree_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/steps/step-one/replay-subtree", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleForceCompleteWorkflowStep tenant isolation.

func TestTenantIsolation_ForceCompleteWorkflowStep_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/steps/step-one/force-complete", `{"result":{}}`, projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

func TestTenantIsolation_ForceCompleteWorkflowStep_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-a/steps/step-one/force-complete", `{"result":{}}`, projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

// handleBulkReplayWorkflowRuns tenant isolation.

func TestTenantIsolation_BulkReplayWorkflowRuns_OwnProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	body := `{"workflow_run_ids":["wfr-a-failed"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.EqualValues(t, 1, int(resp["replayed"].(float64)))

}

func TestTenantIsolation_BulkReplayWorkflowRuns_CrossProject(t *testing.T) {
	t.Parallel()
	ms := newWorkflowRunIsolationStore()
	srv := newWorkflowRunTestServerWithCallback(t, ms)

	// Project B tries to replay wfr-a-failed (owned by project A).
	body := `{"workflow_run_ids":["wfr-a-failed"]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-replay", body, projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.EqualValues(t, 0, int(resp["replayed"].(float64)))

	// Cross-project run should NOT be replayed.

}
