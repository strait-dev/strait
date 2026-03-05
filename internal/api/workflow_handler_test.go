package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"orchestrator/internal/config"
	"orchestrator/internal/domain"
	"orchestrator/internal/store"
)

type mockWorkflowTrigger struct {
	triggerWorkflowFn func(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string) (*domain.WorkflowRun, error)
	approveStepFn     func(ctx context.Context, workflowRunID, stepRef, approver string) error
	resumeWorkflowFn  func(ctx context.Context, workflowRunID string) error
	onJobRunTerminal  func(ctx context.Context, run *domain.JobRun) error
}

func (m *mockWorkflowTrigger) TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string) (*domain.WorkflowRun, error) {
	if m.triggerWorkflowFn != nil {
		return m.triggerWorkflowFn(ctx, workflowID, projectID, payload, triggeredBy)
	}
	return nil, nil
}

func (m *mockWorkflowTrigger) ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error {
	if m.approveStepFn != nil {
		return m.approveStepFn(ctx, workflowRunID, stepRef, approver)
	}
	return nil
}

func (m *mockWorkflowTrigger) ResumeWorkflowRun(ctx context.Context, workflowRunID string) error {
	if m.resumeWorkflowFn != nil {
		return m.resumeWorkflowFn(ctx, workflowRunID)
	}
	return nil
}

func (m *mockWorkflowTrigger) OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error {
	if m.onJobRunTerminal != nil {
		return m.onJobRunTerminal(ctx, run)
	}
	return nil
}

func newWorkflowTestServer(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher, trigger WorkflowTrigger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "test-jwt-key-must-be-32-chars-long",
	}
	return NewServer(cfg, s, q, pub, nil, nil, nil, trigger)
}

func newWorkflowTestServerWithCallback(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher, wfCallback WorkflowCallback, trigger WorkflowTrigger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "test-jwt-key-must-be-32-chars-long",
	}
	return NewServer(cfg, s, q, pub, nil, nil, wfCallback, trigger)
}

func TestHandleCreateWorkflow_SuccessWithSteps(t *testing.T) {
	createStepCalls := 0
	ms := &mockAPIStore{
		createWorkflowFn: func(_ context.Context, wf *domain.Workflow) error {
			wf.ID = "wf-1"
			return nil
		},
		createWorkflowStepFn: func(_ context.Context, step *domain.WorkflowStep) error {
			createStepCalls++
			if step.WorkflowID != "wf-1" {
				t.Fatalf("step workflow_id = %q, want wf-1", step.WorkflowID)
			}
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":"s1"},{"job_id":"job-2","step_ref":"s2","depends_on":["s1"]}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if createStepCalls != 2 {
		t.Fatalf("create step calls = %d, want 2", createStepCalls)
	}
}

func TestHandleCreateWorkflow_MissingFields(t *testing.T) {
	srv := newWorkflowTestServer(t, &mockAPIStore{}, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateWorkflow_InvalidStep(t *testing.T) {
	srv := newWorkflowTestServer(t, &mockAPIStore{}, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":"s1","depends_on":[""]}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleGetWorkflow_FoundWithSteps(t *testing.T) {
	ms := &mockAPIStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "wf", ProjectID: "proj-1"}, nil
		},
		listStepsByWorkflowFn: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-1", WorkflowID: workflowID, StepRef: "s1"}}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetWorkflow_NotFound(t *testing.T) {
	ms := &mockAPIStore{
		getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return nil, store.ErrWorkflowNotFound
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListWorkflows_Success(t *testing.T) {
	ms := &mockAPIStore{
		listWorkflowsFn: func(_ context.Context, projectID string) ([]domain.Workflow, error) {
			return []domain.Workflow{{ID: "wf-1", ProjectID: projectID}, {ID: "wf-2", ProjectID: projectID}}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows?project_id=proj-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleListWorkflows_MissingProjectID(t *testing.T) {
	srv := newWorkflowTestServer(t, &mockAPIStore{}, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateWorkflow_SuccessWithStepReplacement(t *testing.T) {
	deleteCalled := false
	createStepCalls := 0
	ms := &mockAPIStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", Enabled: true}, nil
		},
		updateWorkflowFn: func(_ context.Context, wf *domain.Workflow) error {
			if wf.Name != "new" || wf.Slug != "new-slug" || wf.Enabled {
				t.Fatalf("unexpected updated workflow: %+v", wf)
			}
			return nil
		},
		deleteStepsByWorkflowFn: func(_ context.Context, workflowID string) error {
			deleteCalled = true
			if workflowID != "wf-1" {
				t.Fatalf("workflow_id = %q, want wf-1", workflowID)
			}
			return nil
		},
		createWorkflowStepFn: func(_ context.Context, _ *domain.WorkflowStep) error {
			createStepCalls++
			return nil
		},
		listStepsByWorkflowFn: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "s1", WorkflowID: workflowID, StepRef: "s1"}}, nil
		},
	}

	body := `{"name":"new","slug":"new-slug","enabled":false,"steps":[{"job_id":"job-1","step_ref":"s1"}]}`
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !deleteCalled {
		t.Fatal("expected DeleteStepsByWorkflow to be called")
	}
	if createStepCalls != 1 {
		t.Fatalf("create step calls = %d, want 1", createStepCalls)
	}
}

func TestHandleUpdateWorkflow_NotFound(t *testing.T) {
	ms := &mockAPIStore{
		getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return nil, store.ErrWorkflowNotFound
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/missing", `{"name":"new"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteWorkflow(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockAPIStore{
			deleteWorkflowFn: func(_ context.Context, _ string) error { return nil },
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflows/wf-1", ""))

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockAPIStore{
			deleteWorkflowFn: func(_ context.Context, _ string) error { return errors.New("delete failed") },
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflows/wf-1", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleTriggerWorkflow(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		labelsSaved := false
		published := map[string]int{}
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, workflowID, projectID string, _ json.RawMessage, triggeredBy string) (*domain.WorkflowRun, error) {
				if workflowID != "wf-1" || projectID != "proj-1" || triggeredBy != "manual" {
					t.Fatalf("unexpected trigger args: %s %s %s", workflowID, projectID, triggeredBy)
				}
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: workflowID, ProjectID: projectID, Status: domain.WfStatusRunning}, nil
			},
		}
		ms := &mockAPIStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
			createWorkflowRunLabelsFn: func(_ context.Context, workflowRunID string, labels map[string]string) error {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
				}
				if labels["env"] != "test" {
					t.Fatalf("labels env = %q, want test", labels["env"])
				}
				labelsSaved = true
				return nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"project_id":"proj-1","labels":{"env":"test"}}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if !labelsSaved {
			t.Fatal("expected workflow run labels to be persisted")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})

	t.Run("workflow not found", func(t *testing.T) {
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowNotFound
			},
		}
		ms := &mockAPIStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-missing/trigger", `{}`))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("engine unavailable", func(t *testing.T) {
		srv := newWorkflowTestServer(t, &mockAPIStore{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{}`))

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("workflow disabled", func(t *testing.T) {
		triggerCalled := false
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string) (*domain.WorkflowRun, error) {
				triggerCalled = true
				return &domain.WorkflowRun{ID: "wr-1"}, nil
			},
		}
		ms := &mockAPIStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: false}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{}`))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
		if triggerCalled {
			t.Fatal("expected trigger not to be called for disabled workflow")
		}
	})
}

func TestHandleListWorkflowRuns(t *testing.T) {
	t.Run("success with pagination", func(t *testing.T) {
		ms := &mockAPIStore{
			listWorkflowRunsFn: func(_ context.Context, workflowID string, limit, offset int) ([]domain.WorkflowRun, error) {
				if workflowID != "wf-1" || limit != 10 || offset != 5 {
					t.Fatalf("unexpected args: %s %d %d", workflowID, limit, offset)
				}
				return []domain.WorkflowRun{{ID: "wr-1"}}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/runs?limit=10&offset=5", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("invalid params", func(t *testing.T) {
		srv := newWorkflowTestServer(t, &mockAPIStore{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/runs?limit=0", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleListWorkflowRunsByProject(t *testing.T) {
	t.Run("success with status filter", func(t *testing.T) {
		ms := &mockAPIStore{
			listWorkflowRunsByProjFn: func(_ context.Context, projectID string, status *domain.WorkflowRunStatus, limit int) ([]domain.WorkflowRun, error) {
				if projectID != "proj-1" || limit != 20 {
					t.Fatalf("unexpected args: %s %d", projectID, limit)
				}
				if status == nil || *status != domain.WfStatusRunning {
					t.Fatalf("unexpected status filter: %v", status)
				}
				return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs?project_id=proj-1&status=running&limit=20", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("missing project id", func(t *testing.T) {
		srv := newWorkflowTestServer(t, &mockAPIStore{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleGetWorkflowRun(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockAPIStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		ms := &mockAPIStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-missing", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestHandlePauseWorkflowRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		getCalls := 0
		published := map[string]int{}
		ms := &mockAPIStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getCalls++
				if getCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusPaused {
					t.Fatalf("unexpected transition %s -> %s", from, to)
				}
				return nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})
}

func TestHandleResumeWorkflowRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resumeCalled := false
		getCalls := 0
		published := map[string]int{}
		cb := &mockWorkflowTrigger{
			resumeWorkflowFn: func(_ context.Context, workflowRunID string) error {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
				}
				resumeCalled = true
				return nil
			},
		}
		ms := &mockAPIStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getCalls++
				if getCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, pub, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/resume", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !resumeCalled {
			t.Fatal("expected resume callback to be called")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})
}

func TestHandleGetWorkflowRunLabels(t *testing.T) {
	ms := &mockAPIStore{
		listWorkflowRunLabelsFn: func(_ context.Context, workflowRunID string) (map[string]string, error) {
			if workflowRunID != "wr-1" {
				t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
			}
			return map[string]string{"env": "test"}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/labels", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDryRunWorkflow(t *testing.T) {
	ms := &mockAPIStore{}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	t.Run("valid DAG", func(t *testing.T) {
		w := httptest.NewRecorder()
		body := `{"steps":[{"job_id":"job-1","step_ref":"a"},{"job_id":"job-2","step_ref":"b","depends_on":["a"]}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", body))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cycle", func(t *testing.T) {
		w := httptest.NewRecorder()
		body := `{"steps":[{"job_id":"job-1","step_ref":"a","depends_on":["b"]},{"job_id":"job-2","step_ref":"b","depends_on":["a"]}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleWorkflowGraph(t *testing.T) {
	ms := &mockAPIStore{
		listStepsByWorkflowFn: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			if workflowID != "wf-1" {
				t.Fatalf("workflowID = %q, want wf-1", workflowID)
			}
			return []domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "b", DependsOn: []string{"a"}},
			}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/graph", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/graph?format=dot", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCancelWorkflowRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		getWorkflowRunCalls := 0
		stepStatusUpdates := 0
		runStatusUpdates := 0
		published := map[string]int{}
		ms := &mockAPIStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getWorkflowRunCalls++
				if getWorkflowRunCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusCanceled}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusCanceled {
					t.Fatalf("unexpected workflow transition: %s -> %s", from, to)
				}
				return nil
			},
			listStepRunsByRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-running", StepRef: "s1", Status: domain.StepRunning, JobRunID: "run-1"},
					{ID: "sr-done", StepRef: "s2", Status: domain.StepCompleted, JobRunID: "run-2"},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id != "sr-running" || status != domain.StepCanceled {
					t.Fatalf("unexpected step status update: %s %s", id, status)
				}
				stepStatusUpdates++
				return nil
			},
			getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
				if id == "run-1" {
					return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
				}
				return &domain.JobRun{ID: id, Status: domain.StatusCompleted}, nil
			},
			updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
				if id != "run-1" || from != domain.StatusExecuting || to != domain.StatusCanceled {
					t.Fatalf("unexpected run status update: %s %s -> %s", id, from, to)
				}
				runStatusUpdates++
				return nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if stepStatusUpdates != 1 {
			t.Fatalf("step status updates = %d, want 1", stepStatusUpdates)
		}
		if runStatusUpdates != 1 {
			t.Fatalf("job run status updates = %d, want 1", runStatusUpdates)
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		ms := &mockAPIStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/missing", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("already terminal", func(t *testing.T) {
		ms := &mockAPIStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusCompleted}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleListWorkflowStepRuns(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockAPIStore{
			listStepRunsByRunFn: func(_ context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflow_run_id = %q, want wr-1", workflowRunID)
				}
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1"}}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/steps", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		ms := &mockAPIStore{
			listStepRunsByRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return nil, errors.New("db down")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/steps", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}
