package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"
)

type mockEngineStore struct {
	getWorkflowFn                func(ctx context.Context, id string) (*domain.Workflow, error)
	listStepsByWorkflowVerFn     func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	countRunningWorkflowRunsFn   func(ctx context.Context, workflowID string) (int, error)
	createWorkflowRunFn          func(ctx context.Context, run *domain.WorkflowRun) error
	createWorkflowStepRunFn      func(ctx context.Context, sr *domain.WorkflowStepRun) error
	createWorkflowStepApprovalFn func(ctx context.Context, approval *domain.WorkflowStepApproval) error
	updateWorkflowRunStatusFn    func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	updateStepRunStatusFn        func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	getStepOutputsFn             func(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	getWorkflowRunFn             func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listStepRunsByWorkflowRunFn  func(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	getWorkflowRunsByParentFn    func(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
}

func (m *mockEngineStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	if m.getWorkflowFn != nil {
		return m.getWorkflowFn(ctx, id)
	}
	return nil, nil
}

func (m *mockEngineStore) ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	if m.listStepsByWorkflowVerFn != nil {
		return m.listStepsByWorkflowVerFn(ctx, workflowID, version)
	}
	return nil, nil
}

func (m *mockEngineStore) CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error) {
	if m.countRunningWorkflowRunsFn != nil {
		return m.countRunningWorkflowRunsFn(ctx, workflowID)
	}
	return 0, nil
}

func (m *mockEngineStore) CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error {
	if m.createWorkflowRunFn != nil {
		return m.createWorkflowRunFn(ctx, run)
	}
	return nil
}

func (m *mockEngineStore) CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error {
	if m.createWorkflowStepRunFn != nil {
		return m.createWorkflowStepRunFn(ctx, sr)
	}
	return nil
}

func (m *mockEngineStore) CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error {
	if m.createWorkflowStepApprovalFn != nil {
		return m.createWorkflowStepApprovalFn(ctx, approval)
	}
	return nil
}

func (m *mockEngineStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	if m.updateWorkflowRunStatusFn != nil {
		return m.updateWorkflowRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockEngineStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, status, fields)
	}
	return nil
}

func (m *mockEngineStore) GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error) {
	if m.getStepOutputsFn != nil {
		return m.getStepOutputsFn(ctx, workflowRunID, stepRefs)
	}
	return nil, nil
}

func (m *mockEngineStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockEngineStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByWorkflowRunFn != nil {
		return m.listStepRunsByWorkflowRunFn(ctx, workflowRunID)
	}
	return nil, nil
}

func (m *mockEngineStore) GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error) {
	if m.getWorkflowRunsByParentFn != nil {
		return m.getWorkflowRunsByParentFn(ctx, parentWorkflowRunID)
	}
	return nil, nil
}

type mockEngineQueue struct {
	enqueueFn func(ctx context.Context, run *domain.JobRun) error
}

func (m *mockEngineQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, run)
	}
	return nil
}

func TestTriggerWorkflow(t *testing.T) {
	t.Run("happy path starts root steps only", func(t *testing.T) {
		stepRunsCreated := make(map[string]domain.WorkflowStepRun)
		enqueued := 0
		updateStepCalls := 0
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-1"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusPending || to != domain.WfStatusRunning {
					t.Fatalf("unexpected workflow transition %s -> %s", from, to)
				}
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = *sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-a" {
					if status != domain.StepRunning {
						t.Fatalf("root step should move to running, got %s", status)
					}
					updateStepCalls++
				}
				return nil
			},
		}
		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueued++
				run.ID = "run-a"
				if run.JobID != "job-a" || run.WorkflowStepRunID != "sr-a" {
					t.Fatalf("unexpected enqueued run: %+v", run)
				}
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		wfRun, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", json.RawMessage(`{"k":"v"}`), "manual", nil)
		if err != nil {
			t.Fatalf("TriggerWorkflow() error = %v", err)
		}
		if wfRun == nil || wfRun.ID != "wr-1" || wfRun.Status != domain.WfStatusRunning {
			t.Fatalf("unexpected workflow run: %+v", wfRun)
		}
		if enqueued != 1 {
			t.Fatalf("enqueued = %d, want 1", enqueued)
		}
		if updateStepCalls == 0 {
			t.Fatal("expected root step status update")
		}
		if stepRunsCreated["b"].Status != domain.StepWaiting {
			t.Fatalf("dependent step status = %s, want waiting", stepRunsCreated["b"].Status)
		}
	})

	t.Run("disabled workflow", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: false}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil)
		if err == nil || !strings.Contains(err.Error(), "disabled") {
			t.Fatalf("expected disabled error, got %v", err)
		}
	})

	t.Run("empty steps", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: true}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil)
		if err == nil || !strings.Contains(err.Error(), "at least one step") {
			t.Fatalf("expected empty steps error, got %v", err)
		}
	})

	t.Run("project mismatch", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-a", Enabled: true}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-b", nil, "", nil)
		if err == nil || !strings.Contains(err.Error(), "does not belong") {
			t.Fatalf("expected project mismatch error, got %v", err)
		}
	})

	t.Run("GetWorkflow error", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return nil, errors.New("db get workflow failed")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil)
		if err == nil || !strings.Contains(err.Error(), "get workflow") {
			t.Fatalf("expected get workflow error, got %v", err)
		}
	})

	t.Run("ListStepsByWorkflowVersion error", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, errors.New("db list steps failed")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil)
		if err == nil || !strings.Contains(err.Error(), "list workflow steps by version") {
			t.Fatalf("expected list steps error, got %v", err)
		}
	})

	t.Run("CreateWorkflowStepRun error", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf", ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-1"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, _ *domain.WorkflowStepRun) error {
				return errors.New("db create step run failed")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "", nil)
		if err == nil || !strings.Contains(err.Error(), "create step run") {
			t.Fatalf("expected create step run error, got %v", err)
		}
	})
}

func TestMergePayloads(t *testing.T) {
	t.Run("object merge with parent outputs", func(t *testing.T) {
		out := mergePayloads(
			json.RawMessage(`{"a":1,"shared":"trigger"}`),
			json.RawMessage(`{"b":2,"shared":"step"}`),
			json.RawMessage(`{"p":{"ok":true}}`),
		)

		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		if got["shared"] != "step" {
			t.Fatalf("shared = %v, want step", got["shared"])
		}
		if got["a"] != float64(1) || got["b"] != float64(2) {
			t.Fatalf("missing merged keys: %+v", got)
		}
		if _, ok := got["parent_outputs"]; !ok {
			t.Fatalf("missing parent_outputs: %+v", got)
		}
	})

	t.Run("step payload overrides non-object fallback", func(t *testing.T) {
		out := mergePayloads(json.RawMessage(`{"a":1}`), json.RawMessage(`"step"`), nil)
		if string(out) != `"step"` {
			t.Fatalf("got %s, want step payload", string(out))
		}
	})

	t.Run("empty step payload keeps trigger payload", func(t *testing.T) {
		out := mergePayloads(json.RawMessage(`{"a":1}`), nil, nil)
		if string(out) != `{"a":1}` {
			t.Fatalf("got %s, want trigger payload", string(out))
		}
	})
}

type mockCallbackStore struct {
	getStepRunByJobRunIDFn       func(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error)
	updateStepRunStatusFn        func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	incrementStepDepsFn          func(ctx context.Context, workflowRunID string, completedStepRef string) ([]store.StepDepResult, error)
	incrementStepRunAttemptFn    func(ctx context.Context, id string, newAttempt int) error
	getWorkflowRunFn             func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	updateWorkflowRunStatusFn    func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	listStepRunsByWorkflowRun    func(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	getStepOutputsFn             func(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	listStepsByWorkflowVerFn     func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	getWorkflowFn                func(ctx context.Context, id string) (*domain.Workflow, error)
	getStepRunByRunAndRefFn      func(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	createWorkflowStepApprovalFn func(ctx context.Context, approval *domain.WorkflowStepApproval) error
	getWorkflowStepApprovalFn    func(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	updateWorkflowStepApprovalFn func(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	updateRunStatusFn            func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	getWorkflowRunsByParentFn    func(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
}

func (m *mockCallbackStore) GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
	if m.getStepRunByJobRunIDFn != nil {
		return m.getStepRunByJobRunIDFn(ctx, jobRunID)
	}
	return nil, nil
}

func (m *mockCallbackStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, status, fields)
	}
	return nil
}

func (m *mockCallbackStore) IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]store.StepDepResult, error) {
	if m.incrementStepDepsFn != nil {
		return m.incrementStepDepsFn(ctx, workflowRunID, completedStepRef)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockCallbackStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	if m.updateWorkflowRunStatusFn != nil {
		return m.updateWorkflowRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockCallbackStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByWorkflowRun != nil {
		return m.listStepRunsByWorkflowRun(ctx, workflowRunID)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error) {
	if m.getStepOutputsFn != nil {
		return m.getStepOutputsFn(ctx, workflowRunID, stepRefs)
	}
	return nil, nil
}

func (m *mockCallbackStore) ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	if m.listStepsByWorkflowVerFn != nil {
		return m.listStepsByWorkflowVerFn(ctx, workflowID, version)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	if m.getWorkflowFn != nil {
		return m.getWorkflowFn(ctx, id)
	}
	return nil, nil
}

func (m *mockCallbackStore) GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
	if m.getStepRunByRunAndRefFn != nil {
		return m.getStepRunByRunAndRefFn(ctx, workflowRunID, stepRef)
	}
	return nil, nil
}

func (m *mockCallbackStore) CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error {
	if m.createWorkflowStepApprovalFn != nil {
		return m.createWorkflowStepApprovalFn(ctx, approval)
	}
	return nil
}

func (m *mockCallbackStore) GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error) {
	if m.getWorkflowStepApprovalFn != nil {
		return m.getWorkflowStepApprovalFn(ctx, stepRunID)
	}
	return nil, nil
}

func (m *mockCallbackStore) UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
	if m.updateWorkflowStepApprovalFn != nil {
		return m.updateWorkflowStepApprovalFn(ctx, id, status, approvedBy, approvedAt, errMsg)
	}
	return nil
}

func (m *mockCallbackStore) IncrementStepRunAttempt(ctx context.Context, id string, newAttempt int) error {
	if m.incrementStepRunAttemptFn != nil {
		return m.incrementStepRunAttemptFn(ctx, id, newAttempt)
	}
	return nil
}

func (m *mockCallbackStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockCallbackStore) GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error) {
	if m.getWorkflowRunsByParentFn != nil {
		return m.getWorkflowRunsByParentFn(ctx, parentWorkflowRunID)
	}
	return nil, nil
}

func TestStepCallback_OnJobRunTerminal(t *testing.T) {
	t.Run("nil run no-op", func(t *testing.T) {
		cb := NewStepCallback(&mockCallbackStore{}, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.OnJobRunTerminal(context.Background(), nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing workflow step run id no-op", func(t *testing.T) {
		cb := NewStepCallback(&mockCallbackStore{}, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("already terminal step no-op", func(t *testing.T) {
		getCalled := 0
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				getCalled++
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if getCalled != 1 {
			t.Fatalf("GetStepRunByJobRunID called %d times, want 1", getCalled)
		}
	})

	t.Run("completed run updates step and workflow", func(t *testing.T) {
		workflowUpdated := false
		stepUpdated := false
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
				if id == "sr-1" {
					if status != domain.StepCompleted {
						t.Fatalf("status = %s, want completed", status)
					}
					if _, ok := fields["output"]; !ok {
						t.Fatalf("expected output field: %+v", fields)
					}
					stepUpdated = true
				}
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "s1", StepRef: "s1"}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusCompleted {
					t.Fatalf("unexpected workflow transition %s -> %s", from, to)
				}
				workflowUpdated = true
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted, Result: json.RawMessage(`{"ok":true}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !stepUpdated || !workflowUpdated {
			t.Fatalf("expected step and workflow updates, step=%v workflow=%v", stepUpdated, workflowUpdated)
		}
	})

	t.Run("failed run applies fail_workflow policy", func(t *testing.T) {
		workflowFailed := false
		canceledDependents := 0
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-fail", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-fail" && status != domain.StepFailed {
					t.Fatalf("failed step status = %s, want failed", status)
				}
				if id == "sr-other" && status == domain.StepCanceled {
					canceledDependents++
				}
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "s1", OnFailure: domain.FailWorkflow},
					{StepRef: "s2"},
				}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow transition %s -> %s", from, to)
				}
				workflowFailed = true
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-fail", StepRef: "s1", Status: domain.StepFailed},
					{ID: "sr-other", StepRef: "s2", Status: domain.StepWaiting},
				}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-fail", Status: domain.StatusFailed, Error: "boom"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !workflowFailed {
			t.Fatal("expected workflow to fail")
		}
		if canceledDependents != 1 {
			t.Fatalf("canceled dependents = %d, want 1", canceledDependents)
		}
	})

	t.Run("canceled run maps to step canceled", func(t *testing.T) {
		statusSeen := domain.StepPending
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				statusSeen = status
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCanceled}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCanceled})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if statusSeen != domain.StepCanceled {
			t.Fatalf("status = %s, want canceled", statusSeen)
		}
	})
}

func TestStepCallback_OnJobRunTerminal_PausedWorkflowDoesNotScheduleChildren(t *testing.T) {
	enqueueCalled := false
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-parent", WorkflowRunID: "wr-1", StepRef: "parent", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id == "sr-parent" && status != domain.StepCompleted {
				t.Fatalf("parent status = %s, want completed", status)
			}
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return []store.StepDepResult{{StepRunID: "sr-child", StepRef: "child", DepsCompleted: 1, DepsRequired: 1}}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusPaused}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-parent", StepRef: "parent"}, {ID: "step-child", StepRef: "child", JobID: "job-1", DependsOn: []string{"parent"}}}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-parent", StepRef: "parent", Status: domain.StepCompleted},
				{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting},
			}, nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCalled = true
		return nil
	}}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-parent", Status: domain.StatusCompleted})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enqueueCalled {
		t.Fatal("expected no child step scheduling while workflow is paused")
	}
}

func TestMapRunStatusToStepStatus(t *testing.T) {
	tests := []struct {
		name      string
		runStatus domain.RunStatus
		want      domain.StepRunStatus
	}{
		{name: "completed", runStatus: domain.StatusCompleted, want: domain.StepCompleted},
		{name: "canceled", runStatus: domain.StatusCanceled, want: domain.StepCanceled},
		{name: "failed", runStatus: domain.StatusFailed, want: domain.StepFailed},
		{name: "timed_out", runStatus: domain.StatusTimedOut, want: domain.StepFailed},
		{name: "crashed", runStatus: domain.StatusCrashed, want: domain.StepFailed},
		{name: "system_failed", runStatus: domain.StatusSystemFailed, want: domain.StepFailed},
		{name: "expired", runStatus: domain.StatusExpired, want: domain.StepFailed},
		{name: "unexpected queued", runStatus: domain.StatusQueued, want: domain.StepFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := mapRunStatusToStepStatus(&domain.JobRun{Status: tt.runStatus, Error: "err", Result: json.RawMessage(`{"ok":true}`)})
			if status != tt.want {
				t.Fatalf("mapRunStatusToStepStatus(%s) = %s, want %s", tt.runStatus, status, tt.want)
			}
		})
	}
}

func TestMapRunStatusToStepStatus_Exhaustive(t *testing.T) {
	t.Run("StatusCompleted includes output when result present", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{
			Status: domain.StatusCompleted,
			Result: json.RawMessage(`{"ok":true}`),
		})
		if status != domain.StepCompleted {
			t.Fatalf("status = %s, want %s", status, domain.StepCompleted)
		}
		output, ok := fields["output"]
		if !ok {
			t.Fatalf("expected output field, got %+v", fields)
		}
		raw, ok := output.(json.RawMessage)
		if !ok || string(raw) != `{"ok":true}` {
			t.Fatalf("output = %T %v, want json.RawMessage", output, output)
		}
	})

	t.Run("StatusCompleted with empty result has no output", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusCompleted})
		if status != domain.StepCompleted {
			t.Fatalf("status = %s, want %s", status, domain.StepCompleted)
		}
		if _, ok := fields["output"]; ok {
			t.Fatalf("did not expect output field, got %+v", fields)
		}
	})

	t.Run("StatusCanceled maps to StepCanceled", func(t *testing.T) {
		status, _ := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusCanceled})
		if status != domain.StepCanceled {
			t.Fatalf("status = %s, want %s", status, domain.StepCanceled)
		}
	})

	t.Run("StatusFailed maps to StepFailed and sets error", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusFailed})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
		errVal, ok := fields["error"].(string)
		if !ok || !strings.Contains(errVal, "job run ended with status") {
			t.Fatalf("error field = %v, want fallback status error", fields["error"])
		}
	})

	t.Run("StatusTimedOut maps to StepFailed and sets error", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusTimedOut})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
		if _, ok := fields["error"]; !ok {
			t.Fatalf("expected error field, got %+v", fields)
		}
	})

	t.Run("StatusCrashed maps to StepFailed and sets error", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusCrashed})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
		if _, ok := fields["error"]; !ok {
			t.Fatalf("expected error field, got %+v", fields)
		}
	})

	t.Run("StatusSystemFailed maps to StepFailed and sets error", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusSystemFailed})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
		if _, ok := fields["error"]; !ok {
			t.Fatalf("expected error field, got %+v", fields)
		}
	})

	t.Run("StatusExpired maps to StepFailed and sets error", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusExpired})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
		if _, ok := fields["error"]; !ok {
			t.Fatalf("expected error field, got %+v", fields)
		}
	})

	t.Run("StatusFailed with explicit Error uses that string", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusFailed, Error: "boom"})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
		if fields["error"] != "boom" {
			t.Fatalf("error = %v, want boom", fields["error"])
		}
	})

	t.Run("StatusFailed with empty Error uses fallback message", func(t *testing.T) {
		status, fields := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.StatusFailed})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
		errVal, ok := fields["error"].(string)
		if !ok || !strings.Contains(errVal, "job run ended with status") {
			t.Fatalf("error field = %v, want fallback status error", fields["error"])
		}
	})

	t.Run("unknown status defaults to StepFailed", func(t *testing.T) {
		status, _ := mapRunStatusToStepStatus(&domain.JobRun{Status: domain.RunStatus("bogus")})
		if status != domain.StepFailed {
			t.Fatalf("status = %s, want %s", status, domain.StepFailed)
		}
	})
}

func TestCancelRemainingSteps(t *testing.T) {
	t.Run("cancels non-terminal steps", func(t *testing.T) {
		updated := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-completed", Status: domain.StepCompleted},
					{ID: "sr-running", Status: domain.StepRunning},
					{ID: "sr-pending", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				updated[id] = status
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		if err != nil {
			t.Fatalf("cancelRemainingSteps() error = %v", err)
		}
		if len(updated) != 2 {
			t.Fatalf("updated len = %d, want 2", len(updated))
		}
		if updated["sr-running"] != domain.StepCanceled {
			t.Fatalf("running step status = %s, want %s", updated["sr-running"], domain.StepCanceled)
		}
		if updated["sr-pending"] != domain.StepCanceled {
			t.Fatalf("pending step status = %s, want %s", updated["sr-pending"], domain.StepCanceled)
		}
		if _, ok := updated["sr-completed"]; ok {
			t.Fatal("completed step should not be canceled")
		}
	})

	t.Run("skips all terminal", func(t *testing.T) {
		updateCalls := 0
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-completed", Status: domain.StepCompleted},
					{ID: "sr-failed", Status: domain.StepFailed},
					{ID: "sr-skipped", Status: domain.StepSkipped},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				updateCalls++
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		if err != nil {
			t.Fatalf("cancelRemainingSteps() error = %v", err)
		}
		if updateCalls != 0 {
			t.Fatalf("update calls = %d, want 0", updateCalls)
		}
	})

	t.Run("store list error", func(t *testing.T) {
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return nil, errors.New("list failed")
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		if err == nil || !strings.Contains(err.Error(), "list step runs by workflow run") {
			t.Fatalf("expected list error, got %v", err)
		}
	})

	t.Run("store update error", func(t *testing.T) {
		ms := &mockCallbackStore{
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-running", Status: domain.StepRunning}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return errors.New("update failed")
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.cancelRemainingSteps(context.Background(), "wr-1")
		if err == nil || !strings.Contains(err.Error(), "cancel step run") {
			t.Fatalf("expected update error, got %v", err)
		}
	})
}

func TestStepCallback_OnJobRunTerminal_GetStepRunError(t *testing.T) {
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return nil, errors.New("boom")
		},
	}
	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	if err == nil || !strings.Contains(err.Error(), "get step run by job run id") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestStepCallback_OnJobRunTerminal_UpdateStepRunStatusErrorWrapped(t *testing.T) {
	baseErr := errors.New("write failed")
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return baseErr
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "update step run terminal status") {
		t.Errorf("error = %v, want update step context", err)
	}
	if !errors.Is(err, baseErr) {
		t.Errorf("errors.Is(err, baseErr) = false, err = %v", err)
	}
}

func TestStepCallback_OnJobRunTerminal_CheckStepRetryErrorWrapped(t *testing.T) {
	baseErr := errors.New("workflow lookup failed")
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Attempt: 1, Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return nil, baseErr
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusFailed, Error: "boom"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "check step retry") {
		t.Errorf("error = %v, want check step retry context", err)
	}
	if !errors.Is(err, baseErr) {
		t.Errorf("errors.Is(err, baseErr) = false, err = %v", err)
	}
}

func TestStepCallback_OnJobRunTerminal_ProcessCompletedStepErrorWrapped(t *testing.T) {
	baseErr := errors.New("deps update failed")
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, baseErr
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "process completed step s1") {
		t.Errorf("error = %v, want process completed step context", err)
	}
	if !errors.Is(err, baseErr) {
		t.Errorf("errors.Is(err, baseErr) = false, err = %v", err)
	}
}

func TestStepCallback_OnJobRunTerminal_ProcessFailedStepErrorWrapped(t *testing.T) {
	baseErr := errors.New("get workflow run failed")
	getWorkflowRunCalls := 0
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Attempt: 1, Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			getWorkflowRunCalls++
			if getWorkflowRunCalls == 1 {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusRunning}, nil
			}
			return nil, baseErr
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 0, OnFailure: domain.FailWorkflow}}, nil
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusFailed, Error: "boom"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "process failed step s1") {
		t.Errorf("error = %v, want process failed step context", err)
	}
	if !errors.Is(err, baseErr) {
		t.Errorf("errors.Is(err, baseErr) = false, err = %v", err)
	}
}

func TestStepCallback_OnJobRunTerminal_FanInStartsChildren(t *testing.T) {
	startCalls := 0
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id == "sr-a" && status != domain.StepCompleted {
				t.Fatalf("expected parent to complete, got %s", status)
			}
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return []store.StepDepResult{{StepRunID: "sr-b", StepRef: "b", DepsCompleted: 1, DepsRequired: 1}}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning, ProjectID: "proj-1"}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-a", StepRef: "a", JobID: "job-a"}, {ID: "step-b", StepRef: "b", JobID: "job-b", DependsOn: []string{"a"}}}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted}, {ID: "sr-b", StepRef: "b", Status: domain.StepWaiting, WorkflowStepID: "step-b"}}, nil
		},
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			return map[string]json.RawMessage{"a": json.RawMessage(`{"ok":true}`)}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}
	engStore := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
			if id == "sr-b" && status == domain.StepRunning {
				startCalls++
			}
			if id == "sr-b" && fields["job_run_id"] != nil {
				startCalls++
			}
			return nil
		},
	}
	mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		run.ID = "job-run-b"
		if run.JobID != "job-b" {
			return fmt.Errorf("unexpected job id %s", run.JobID)
		}
		return nil
	}}
	engine := NewWorkflowEngine(engStore, mq, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-a", WorkflowStepRunID: "sr-a", Status: domain.StatusCompleted})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if startCalls == 0 {
		t.Fatal("expected child step to be started")
	}
}

func TestStepCallback_checkStepRetry(t *testing.T) {
	tests := []struct {
		name              string
		stepRun           *domain.WorkflowStepRun
		getWorkflowRunFn  func(ctx context.Context, id string) (*domain.WorkflowRun, error)
		listStepsFn       func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
		wantShouldRetry   bool
		wantNewAttempt    int
		wantErrContains   string
		assertNextRetryAt func(t *testing.T, got time.Time, before, after time.Time)
	}{
		{
			name: "no_retry_policy",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 0}}, nil
			},
			wantShouldRetry: false,
			wantNewAttempt:  0,
		},
		{
			name: "first_attempt_with_retry_policy",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					RetryMaxAttempts:      3,
					RetryBackoff:          domain.RetryBackoffExponential,
					RetryInitialDelaySecs: 2,
					RetryMaxDelaySecs:     120,
				}}, nil
			},
			wantShouldRetry: true,
			wantNewAttempt:  2,
			assertNextRetryAt: func(t *testing.T, got time.Time, before, after time.Time) {
				t.Helper()
				if got.IsZero() {
					t.Fatal("nextRetryAt is zero")
				}
				if !got.After(before) {
					t.Fatalf("nextRetryAt %v is not after start %v", got, before)
				}
				if !got.After(after) {
					t.Fatalf("nextRetryAt %v is not after end %v", got, after)
				}
			},
		},
		{
			name: "exhausted_attempts",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       2,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 2}}, nil
			},
			wantShouldRetry: false,
			wantNewAttempt:  0,
		},
		{
			name: "get_workflow_run_error",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, errors.New("boom")
			},
			wantErrContains: "get workflow run",
		},
		{
			name: "list_steps_error",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, errors.New("boom")
			},
			wantErrContains: "list workflow steps",
		},
		{
			name: "step_not_found",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "missing",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1", RetryMaxAttempts: 3}}, nil
			},
			wantErrContains: "step definition not found",
		},
		{
			name: "exponential_backoff_delay",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       1,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					RetryMaxAttempts:      4,
					RetryBackoff:          domain.RetryBackoffExponential,
					RetryInitialDelaySecs: 10,
					RetryMaxDelaySecs:     120,
				}}, nil
			},
			wantShouldRetry: true,
			wantNewAttempt:  2,
			assertNextRetryAt: func(t *testing.T, got time.Time, before, _ time.Time) {
				t.Helper()
				delay := got.Sub(before)
				if delay < 15*time.Second || delay > 25*time.Second {
					t.Fatalf("delay = %v, want roughly 20s (+-20%%)", delay)
				}
			},
		},
		{
			name: "fixed_backoff_delay",
			stepRun: &domain.WorkflowStepRun{
				WorkflowRunID: "wr-1",
				StepRef:       "s1",
				Attempt:       4,
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					RetryMaxAttempts:      10,
					RetryBackoff:          domain.RetryBackoffFixed,
					RetryInitialDelaySecs: 8,
					RetryMaxDelaySecs:     120,
				}}, nil
			},
			wantShouldRetry: true,
			wantNewAttempt:  5,
			assertNextRetryAt: func(t *testing.T, got time.Time, before, _ time.Time) {
				t.Helper()
				delay := got.Sub(before)
				if delay < 6*time.Second || delay > 10*time.Second {
					t.Fatalf("delay = %v, want roughly 8s (+-20%%)", delay)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockCallbackStore{
				getWorkflowRunFn:         tt.getWorkflowRunFn,
				listStepsByWorkflowVerFn: tt.listStepsFn,
			}
			cb := NewStepCallback(store, nil, slog.Default())

			before := time.Now()
			shouldRetry, nextRetryAt, newAttempt, err := cb.checkStepRetry(context.Background(), tt.stepRun, &domain.JobRun{})
			after := time.Now()

			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("checkStepRetry() error = %v", err)
			}
			if shouldRetry != tt.wantShouldRetry {
				t.Fatalf("shouldRetry = %v, want %v", shouldRetry, tt.wantShouldRetry)
			}
			if newAttempt != tt.wantNewAttempt {
				t.Fatalf("newAttempt = %d, want %d", newAttempt, tt.wantNewAttempt)
			}

			if tt.assertNextRetryAt != nil {
				tt.assertNextRetryAt(t, nextRetryAt, before, after)
			}
		})
	}
}

func TestStepCallback_scheduleStepRetry(t *testing.T) {
	tests := []struct {
		name                 string
		incrementErr         error
		updateRunStatusErr   error
		wantErrContains      string
		wantUpdateRunInvoked bool
	}{
		{
			name:                 "success",
			wantUpdateRunInvoked: true,
		},
		{
			name:                 "increment_attempt_error",
			incrementErr:         errors.New("boom"),
			wantErrContains:      "increment step run attempt",
			wantUpdateRunInvoked: false,
		},
		{
			name:                 "update_run_status_error",
			updateRunStatusErr:   errors.New("boom"),
			wantErrContains:      "update job run status for retry",
			wantUpdateRunInvoked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			incrementCalled := 0
			updateRunCalled := 0

			store := &mockCallbackStore{
				incrementStepRunAttemptFn: func(_ context.Context, id string, newAttempt int) error {
					incrementCalled++
					if id != "sr-1" || newAttempt != 2 {
						t.Fatalf("unexpected increment args: id=%s newAttempt=%d", id, newAttempt)
					}
					return tt.incrementErr
				},
				updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
					updateRunCalled++
					if id != "run-1" {
						t.Fatalf("unexpected run id %s", id)
					}
					if from != domain.StatusFailed || to != domain.StatusDelayed {
						t.Fatalf("unexpected status transition %s -> %s", from, to)
					}
					if fields["attempt"] != 2 {
						t.Fatalf("expected attempt=2, got %+v", fields)
					}
					if _, ok := fields["next_retry_at"].(time.Time); !ok {
						t.Fatalf("expected next_retry_at time.Time, got %+v", fields["next_retry_at"])
					}
					return tt.updateRunStatusErr
				},
			}

			cb := NewStepCallback(store, nil, slog.Default())
			err := cb.scheduleStepRetry(
				context.Background(),
				&domain.JobRun{ID: "run-1", Status: domain.StatusFailed},
				&domain.WorkflowStepRun{ID: "sr-1"},
				time.Now().Add(2*time.Second),
				2,
			)

			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrContains, err)
				}
			} else if err != nil {
				t.Fatalf("scheduleStepRetry() error = %v", err)
			}

			if incrementCalled != 1 {
				t.Fatalf("IncrementStepRunAttempt called %d times, want 1", incrementCalled)
			}
			if tt.wantUpdateRunInvoked && updateRunCalled != 1 {
				t.Fatalf("UpdateRunStatus called %d times, want 1", updateRunCalled)
			}
			if !tt.wantUpdateRunInvoked && updateRunCalled != 0 {
				t.Fatalf("UpdateRunStatus called %d times, want 0", updateRunCalled)
			}
		})
	}
}

func TestStepCallback_OnJobRunTerminal_RetryIntegration(t *testing.T) {
	t.Run("failed_run_triggers_retry", func(t *testing.T) {
		incrementCalled := 0
		updateRunCalled := 0

		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{
					ID:            "sr-1",
					WorkflowRunID: "wr-1",
					StepRef:       "s1",
					Attempt:       1,
					Status:        domain.StepRunning,
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-1" && status != domain.StepFailed {
					t.Fatalf("unexpected step status: %s", status)
				}
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:               "s1",
					OnFailure:             domain.FailWorkflow,
					RetryMaxAttempts:      3,
					RetryBackoff:          domain.RetryBackoffFixed,
					RetryInitialDelaySecs: 1,
					RetryMaxDelaySecs:     5,
				}}, nil
			},
			incrementStepRunAttemptFn: func(_ context.Context, id string, newAttempt int) error {
				incrementCalled++
				if id != "sr-1" || newAttempt != 2 {
					t.Fatalf("unexpected increment args id=%s attempt=%d", id, newAttempt)
				}
				return nil
			},
			updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
				updateRunCalled++
				if id != "run-1" || from != domain.StatusFailed || to != domain.StatusDelayed {
					t.Fatalf("unexpected run transition id=%s %s->%s", id, from, to)
				}
				if fields["attempt"] != 2 {
					t.Fatalf("expected attempt=2, got %+v", fields)
				}
				if _, ok := fields["next_retry_at"].(time.Time); !ok {
					t.Fatalf("missing/invalid next_retry_at: %+v", fields)
				}
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				t.Fatal("UpdateWorkflowRunStatus should not be called when retry is scheduled")
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				t.Fatal("ListStepRunsByWorkflowRun should not be called when retry is scheduled")
				return nil, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusFailed, Error: "boom"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if incrementCalled != 1 {
			t.Fatalf("IncrementStepRunAttempt called %d times, want 1", incrementCalled)
		}
		if updateRunCalled != 1 {
			t.Fatalf("UpdateRunStatus called %d times, want 1", updateRunCalled)
		}
	})

	t.Run("failed_run_no_retry_falls_through", func(t *testing.T) {
		workflowFailed := 0
		canceledDependents := 0

		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{
					ID:            "sr-fail",
					WorkflowRunID: "wr-1",
					StepRef:       "s1",
					Attempt:       1,
					Status:        domain.StepRunning,
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-fail" && status != domain.StepFailed {
					t.Fatalf("failed step status = %s, want failed", status)
				}
				if id == "sr-other" && status == domain.StepCanceled {
					canceledDependents++
				}
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{
					StepRef:          "s1",
					OnFailure:        domain.FailWorkflow,
					RetryMaxAttempts: 0,
				}}, nil
			},
			incrementStepRunAttemptFn: func(_ context.Context, _ string, _ int) error {
				t.Fatal("IncrementStepRunAttempt should not be called when retry is disabled")
				return nil
			},
			updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
				t.Fatal("UpdateRunStatus should not be called when retry is disabled")
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusFailed {
					t.Fatalf("unexpected workflow transition %s -> %s", from, to)
				}
				workflowFailed++
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-fail", StepRef: "s1", Status: domain.StepFailed},
					{ID: "sr-other", StepRef: "s2", Status: domain.StepWaiting},
				}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-fail", Status: domain.StatusFailed, Error: "boom"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if workflowFailed != 1 {
			t.Fatalf("workflow failed updates = %d, want 1", workflowFailed)
		}
		if canceledDependents != 1 {
			t.Fatalf("canceled dependents = %d, want 1", canceledDependents)
		}
	})
}

func TestStepCallback_skipDependentSteps(t *testing.T) {
	t.Run("chain_A_B_C", func(t *testing.T) {
		skipCalls := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "b", DependsOn: []string{"a"}},
					{StepRef: "c", DependsOn: []string{"b"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-b", StepRef: "b", Status: domain.StepPending},
					{ID: "sr-c", StepRef: "c", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				skipCalls[id] = status
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(skipCalls) != 2 {
			t.Fatalf("skip calls = %d, want 2", len(skipCalls))
		}
		if skipCalls["sr-b"] != domain.StepSkipped {
			t.Fatalf("sr-b status = %s, want skipped", skipCalls["sr-b"])
		}
		if skipCalls["sr-c"] != domain.StepSkipped {
			t.Fatalf("sr-c status = %s, want skipped", skipCalls["sr-c"])
		}
	})

	t.Run("diamond_A_BC_D", func(t *testing.T) {
		skipCalls := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "b", DependsOn: []string{"a"}},
					{StepRef: "c", DependsOn: []string{"a"}},
					{StepRef: "d", DependsOn: []string{"b", "c"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-b", StepRef: "b", Status: domain.StepPending},
					{ID: "sr-c", StepRef: "c", Status: domain.StepPending},
					{ID: "sr-d", StepRef: "d", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				skipCalls[id] = status
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(skipCalls) != 3 {
			t.Fatalf("skip calls = %d, want 3", len(skipCalls))
		}
		for _, id := range []string{"sr-b", "sr-c", "sr-d"} {
			if skipCalls[id] != domain.StepSkipped {
				t.Fatalf("%s status = %s, want skipped", id, skipCalls[id])
			}
		}
	})

	t.Run("leaf_node_no_dependents", func(t *testing.T) {
		updateCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "leaf", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-leaf", StepRef: "leaf", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				updateCalled = true
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		// Fail "leaf" which has no dependents
		if err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "leaf"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updateCalled {
			t.Fatal("expected no UpdateStepRunStatus calls for leaf node")
		}
	})

	t.Run("already_terminal_not_skipped", func(t *testing.T) {
		skipCalls := make(map[string]domain.StepRunStatus)
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "a"},
					{StepRef: "b", DependsOn: []string{"a"}},
					{StepRef: "c", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted},
					{ID: "sr-c", StepRef: "c", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				skipCalls[id] = status
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(skipCalls) != 1 {
			t.Fatalf("skip calls = %d, want 1 (only sr-c)", len(skipCalls))
		}
		if _, ok := skipCalls["sr-b"]; ok {
			t.Fatal("sr-b is already terminal and should not be skipped")
		}
		if skipCalls["sr-c"] != domain.StepSkipped {
			t.Fatalf("sr-c status = %s, want skipped", skipCalls["sr-c"])
		}
	})

	t.Run("get_workflow_run_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a")
		if err == nil || !strings.Contains(err.Error(), "get workflow run") {
			t.Fatalf("expected wrapped error containing 'get workflow run', got %v", err)
		}
	})

	t.Run("list_steps_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a")
		if err == nil || !strings.Contains(err.Error(), "list workflow steps") {
			t.Fatalf("expected wrapped error containing 'list workflow steps', got %v", err)
		}
	})

	t.Run("list_step_runs_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b", DependsOn: []string{"a"}}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return nil, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a")
		if err == nil || !strings.Contains(err.Error(), "list step runs by workflow run") {
			t.Fatalf("expected wrapped error containing 'list step runs by workflow run', got %v", err)
		}
	})

	t.Run("update_step_run_status_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b", DependsOn: []string{"a"}}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
					{ID: "sr-b", StepRef: "b", Status: domain.StepPending},
				}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return errors.New("write failed")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a")
		if err == nil || !strings.Contains(err.Error(), "skip step run") {
			t.Fatalf("expected wrapped error containing 'skip step run', got %v", err)
		}
	})
}

func TestStepCallback_ApproveStep(t *testing.T) {
	t.Run("empty_approver", func(t *testing.T) {
		cb := NewStepCallback(&mockCallbackStore{}, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "")
		if err == nil || !strings.Contains(err.Error(), "approver is required") {
			t.Fatalf("expected 'approver is required' error, got %v", err)
		}
	})

	t.Run("get_step_run_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return nil, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		if err == nil || !strings.Contains(err.Error(), "get step run") {
			t.Fatalf("expected 'get step run' error, got %v", err)
		}
	})

	t.Run("step_not_found", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		if err == nil || !strings.Contains(err.Error(), "step run not found") {
			t.Fatalf("expected 'step run not found' error, got %v", err)
		}
	})

	t.Run("step_already_terminal", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		if err == nil || !strings.Contains(err.Error(), "already in terminal state") {
			t.Fatalf("expected 'already in terminal state' error, got %v", err)
		}
	})

	t.Run("approval_not_found", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		if err == nil || !strings.Contains(err.Error(), "approval not found") {
			t.Fatalf("expected 'approval not found' error, got %v", err)
		}
	})

	t.Run("approval_already_approved", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "approved", Approvers: []string{"alice"}}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		if err == nil || !strings.Contains(err.Error(), "already approved") {
			t.Fatalf("expected 'already approved' error, got %v", err)
		}
	})

	t.Run("unauthorized_approver", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice"}}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "bob")
		if err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("expected 'not allowed' error, got %v", err)
		}
	})

	t.Run("update_approval_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice"}}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ string) error {
				return errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		if err == nil || !strings.Contains(err.Error(), "update approval") {
			t.Fatalf("expected 'update approval' error, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		approvalUpdated := false
		stepCompleted := false
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			getWorkflowStepApprovalFn: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "apr-1", Status: "pending", Approvers: []string{"alice", "bob"}}, nil
			},
			updateWorkflowStepApprovalFn: func(_ context.Context, id string, status string, approvedBy string, _ *time.Time, _ string) error {
				if id != "apr-1" || status != "approved" || approvedBy != "alice" {
					t.Fatalf("unexpected approval update: id=%s status=%s approvedBy=%s", id, status, approvedBy)
				}
				approvalUpdated = true
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-1" && status == domain.StepCompleted {
					stepCompleted = true
				}
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted}}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "s1"}}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ApproveStep(context.Background(), "wr-1", "s1", "alice")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !approvalUpdated {
			t.Fatal("expected approval to be updated")
		}
		if !stepCompleted {
			t.Fatal("expected step to be marked completed")
		}
	})
}

func TestStepCallback_SkipStep(t *testing.T) {
	t.Run("step in pending status succeeds", func(t *testing.T) {
		updated := false
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				if workflowRunID != "wr-1" || stepRef != "s1" {
					t.Fatalf("unexpected lookup args: %s %s", workflowRunID, stepRef)
				}
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
				if id != "sr-1" || status != domain.StepSkipped {
					t.Fatalf("unexpected step update: id=%s status=%s", id, status)
				}
				if _, ok := fields["finished_at"]; !ok {
					t.Fatalf("expected finished_at field, got %+v", fields)
				}
				if fields["error"] != "manual" {
					t.Fatalf("error field = %v, want manual", fields["error"])
				}
				updated = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.SkipStep(context.Background(), "wr-1", "s1", "manual"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected step update")
		}
	})

	t.Run("step in waiting status succeeds", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				if status != domain.StepSkipped {
					t.Fatalf("status = %s, want skipped", status)
				}
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.SkipStep(context.Background(), "wr-1", "s1", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("step in running status returns error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepRunning}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.SkipStep(context.Background(), "wr-1", "s1", "")
		if err == nil || !strings.Contains(err.Error(), "cannot skip step in running status") {
			t.Fatalf("expected running-status error, got %v", err)
		}
	})

	t.Run("step in completed status returns error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.SkipStep(context.Background(), "wr-1", "s1", "")
		if err == nil || !strings.Contains(err.Error(), "cannot skip step in completed status") {
			t.Fatalf("expected completed-status error, got %v", err)
		}
	})
}

func TestStepCallback_ForceCompleteStep(t *testing.T) {
	t.Run("step in pending status succeeds", func(t *testing.T) {
		updated := false
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				if workflowRunID != "wr-1" || stepRef != "s1" {
					t.Fatalf("unexpected lookup args: %s %s", workflowRunID, stepRef)
				}
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepPending}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
				if id != "sr-1" || status != domain.StepCompleted {
					t.Fatalf("unexpected step update: id=%s status=%s", id, status)
				}
				if _, ok := fields["finished_at"]; !ok {
					t.Fatalf("expected finished_at field, got %+v", fields)
				}
				if string(fields["output"].(json.RawMessage)) != `{"ok":true}` {
					t.Fatalf("output field = %s, want {\"ok\":true}", string(fields["output"].(json.RawMessage)))
				}
				updated = true
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.ForceCompleteStep(context.Background(), "wr-1", "s1", json.RawMessage(`{"ok":true}`)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected step update")
		}
	})

	t.Run("step in waiting status succeeds", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepWaiting}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				if status != domain.StepCompleted {
					t.Fatalf("status = %s, want completed", status)
				}
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "child", Status: domain.StepWaiting}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		if err := cb.ForceCompleteStep(context.Background(), "wr-1", "s1", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("step in running status returns error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepRunning}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ForceCompleteStep(context.Background(), "wr-1", "s1", nil)
		if err == nil || !strings.Contains(err.Error(), "cannot force-complete step in running status") {
			t.Fatalf("expected running-status error, got %v", err)
		}
	})

	t.Run("step in completed status returns error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", Status: domain.StepCompleted}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ForceCompleteStep(context.Background(), "wr-1", "s1", nil)
		if err == nil || !strings.Contains(err.Error(), "cannot force-complete step in completed status") {
			t.Fatalf("expected completed-status error, got %v", err)
		}
	})
}

func TestStepCallback_ResumeWorkflowRun(t *testing.T) {
	t.Run("workflow_run_not_found", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err == nil || !strings.Contains(err.Error(), "workflow run not found") {
			t.Fatalf("expected 'workflow run not found' error, got %v", err)
		}
	})

	t.Run("workflow_run_not_paused", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusRunning}, nil
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err == nil || !strings.Contains(err.Error(), "not paused") {
			t.Fatalf("expected 'not paused' error, got %v", err)
		}
	})

	t.Run("get_workflow_run_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err == nil || !strings.Contains(err.Error(), "get workflow run") {
			t.Fatalf("expected 'get workflow run' error, got %v", err)
		}
	})

	t.Run("update_workflow_run_status_error", func(t *testing.T) {
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return errors.New("db down")
			},
		}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err == nil || !strings.Contains(err.Error(), "resume workflow run") {
			t.Fatalf("expected 'resume workflow run' error, got %v", err)
		}
	})

	t.Run("success_starts_ready_steps", func(t *testing.T) {
		enqueueCalled := false
		engStepUpdated := false
		engStore := &mockEngineStore{
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if id == "sr-root" && status == domain.StepRunning {
					engStepUpdated = true
				}
				return nil
			},
		}
		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				run.ID = "jr-1"
				if run.JobID != "job-root" {
					t.Fatalf("unexpected job id: %s", run.JobID)
				}
				enqueueCalled = true
				return nil
			},
		}
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, ProjectID: "proj-1", Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-root", StepRef: "root", JobID: "job-root"}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-root", StepRef: "root", WorkflowStepID: "step-root", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 0}}, nil
			},
		}
		engine := NewWorkflowEngine(engStore, mq, slog.Default())
		cb := NewStepCallback(ms, engine, slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !enqueueCalled {
			t.Fatal("expected step job to be enqueued")
		}
		if !engStepUpdated {
			t.Fatal("expected engine to update step run status to running")
		}
	})

	t.Run("skips_terminal_steps", func(t *testing.T) {
		enqueueCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", StepRef: "a", JobID: "job-a"}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, DepsCompleted: 0, DepsRequired: 0}}, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		}}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if enqueueCalled {
			t.Fatal("terminal step should not be enqueued")
		}
	})

	t.Run("skips_deps_not_met", func(t *testing.T) {
		enqueueCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-b", StepRef: "b", JobID: "job-b", DependsOn: []string{"a"}}}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-b", StepRef: "b", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 1}}, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		}}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if enqueueCalled {
			t.Fatal("step with unmet deps should not be enqueued")
		}
	})

	t.Run("respects_max_parallel_steps", func(t *testing.T) {
		enqueueCalled := false
		ms := &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, MaxParallelSteps: 1, Status: domain.WfStatusPaused}, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", StepRef: "a", JobID: "job-a"},
					{ID: "step-b", StepRef: "b", JobID: "job-b"},
				}, nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "sr-a", StepRef: "a", Status: domain.StepRunning, DepsCompleted: 0, DepsRequired: 0},
					{ID: "sr-b", StepRef: "b", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 0},
				}, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCalled = true
			return nil
		}}
		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, mq, slog.Default()), slog.Default())
		err := cb.ResumeWorkflowRun(context.Background(), "wr-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if enqueueCalled {
			t.Fatal("should not start step when max_parallel_steps reached")
		}
	})
}

func TestRetryWorkflowRun(t *testing.T) {
	// Helper: build a standard 3-step DAG (a -> b -> c) for retry tests.
	buildSteps := func() []domain.WorkflowStep {
		return []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
			{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"b"}},
		}
	}

	t.Run("retry from failed step b in a->b->c DAG", func(t *testing.T) {
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		enqueuedJobs := make([]string, 0)
		steps := buildSteps()

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID:              "orig-run-1",
					WorkflowID:      "wf-1",
					ProjectID:       "proj-1",
					Status:          domain.WfStatusFailed,
					TriggeredBy:     domain.TriggerManual,
					WorkflowVersion: 1,
					Payload:         json.RawMessage(`{"input":"data"}`),
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return steps, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"result":"ok"}`)},
					{ID: "orig-sr-b", StepRef: "b", Status: domain.StepFailed, Error: "timeout"},
					{ID: "orig-sr-c", StepRef: "c", Status: domain.StepCanceled},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run-1"
				if run.RetryOfRunID != "orig-run-1" {
					t.Fatalf("RetryOfRunID = %q, want orig-run-1", run.RetryOfRunID)
				}
				if run.TriggeredBy != domain.TriggerRetry {
					t.Fatalf("TriggeredBy = %q, want retry", run.TriggeredBy)
				}
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return map[string]json.RawMessage{"a": json.RawMessage(`{"result":"ok"}`)}, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueuedJobs = append(enqueuedJobs, run.JobID)
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run-1")
		if err != nil {
			t.Fatalf("RetryWorkflowRun() error = %v", err)
		}

		// Verify retry run properties.
		if newRun.ID != "retry-run-1" {
			t.Fatalf("new run ID = %q, want retry-run-1", newRun.ID)
		}
		if newRun.Status != domain.WfStatusRunning {
			t.Fatalf("new run status = %q, want running", newRun.Status)
		}
		if newRun.RetryOfRunID != "orig-run-1" {
			t.Fatalf("RetryOfRunID = %q, want orig-run-1", newRun.RetryOfRunID)
		}

		// Step a should be pre-completed (copied from original).
		if sr, ok := stepRunsCreated["a"]; !ok {
			t.Fatal("step run 'a' not created")
		} else {
			if sr.Status != domain.StepCompleted {
				t.Fatalf("step a status = %q, want completed", sr.Status)
			}
			if string(sr.Output) != `{"result":"ok"}` {
				t.Fatalf("step a output = %q, want original output", string(sr.Output))
			}
		}

		// Step b should be fresh (was failed, now re-executed).
		if sr, ok := stepRunsCreated["b"]; !ok {
			t.Fatal("step run 'b' not created")
		} else if sr.DepsCompleted != 1 || sr.DepsRequired != 1 {
			// Step b deps are all complete (a is pre-completed), so it should be started.
			t.Fatalf("step b deps: completed=%d required=%d, want 1/1", sr.DepsCompleted, sr.DepsRequired)
		}

		// Step c should be waiting (its dep b was not completed in original).
		if sr, ok := stepRunsCreated["c"]; !ok {
			t.Fatal("step run 'c' not created")
		} else if sr.Status != domain.StepWaiting {
			t.Fatalf("step c status = %q, want waiting", sr.Status)
		}

		// Only job-b should be enqueued (step a pre-completed, step c waiting).
		if len(enqueuedJobs) != 1 || enqueuedJobs[0] != "job-b" {
			t.Fatalf("enqueued = %v, want [job-b]", enqueuedJobs)
		}
	})

	t.Run("cannot retry non-terminal run", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "run-1", Status: domain.WfStatusRunning}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "run-1")
		if err == nil || !strings.Contains(err.Error(), "must be terminal") {
			t.Fatalf("expected terminal state error, got %v", err)
		}
	})

	t.Run("cannot retry when workflow is disabled", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "run-1", WorkflowID: "wf-1", Status: domain.WfStatusFailed, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: false}, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "run-1")
		if err == nil || !strings.Contains(err.Error(), "disabled") {
			t.Fatalf("expected disabled error, got %v", err)
		}
	})

	t.Run("retry run not found", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "no-such-run")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("retry all-completed run re-starts root steps", func(t *testing.T) {
		// If the original run completed successfully but user wants to retry,
		// all steps should be re-executed since there's no failed step.
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		enqueueCount := 0

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusCompleted, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"x":1}`)},
					{ID: "orig-sr-b", StepRef: "b", Status: domain.StepCompleted, Output: json.RawMessage(`{"y":2}`)},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueueCount++
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		if err != nil {
			t.Fatalf("RetryWorkflowRun() error = %v", err)
		}
		if newRun.ID != "retry-run" {
			t.Fatalf("run ID = %q", newRun.ID)
		}

		// All steps completed in original — both should be pre-completed.
		for _, ref := range []string{"a", "b"} {
			sr, ok := stepRunsCreated[ref]
			if !ok {
				t.Fatalf("step %s not created", ref)
			}
			if sr.Status != domain.StepCompleted {
				t.Fatalf("step %s status = %q, want completed", ref, sr.Status)
			}
		}

		// No new jobs should be enqueued since all steps were pre-completed.
		if enqueueCount != 0 {
			t.Fatalf("enqueueCount = %d, want 0", enqueueCount)
		}
	})

	t.Run("retry respects max parallel steps", func(t *testing.T) {
		enqueueCount := 0
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusFailed, WorkflowVersion: 1,
					MaxParallelSteps: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				// Two independent root steps (no deps on each other).
				return []domain.WorkflowStep{
					{ID: "step-x", JobID: "job-x", StepRef: "x"},
					{ID: "step-y", JobID: "job-y", StepRef: "y"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-x", StepRef: "x", Status: domain.StepFailed},
					{ID: "orig-sr-y", StepRef: "y", Status: domain.StepCanceled},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueueCount++
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		if err != nil {
			t.Fatalf("RetryWorkflowRun() error = %v", err)
		}
		if newRun == nil {
			t.Fatal("expected non-nil run")
		}

		// With max_parallel_steps=1, only 1 step should be enqueued.
		if enqueueCount != 1 {
			t.Fatalf("enqueueCount = %d, want 1 (max_parallel_steps=1)", enqueueCount)
		}
	})

	t.Run("retry with timeout sets expires_at", func(t *testing.T) {
		var createdRun *domain.WorkflowRun
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusTimedOut, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1, TimeoutSecs: 300}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepFailed},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				createdRun = run
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		if err != nil {
			t.Fatalf("RetryWorkflowRun() error = %v", err)
		}
		if createdRun == nil || createdRun.ExpiresAt == nil {
			t.Fatal("expected expires_at to be set for workflow with timeout")
		}
	})

	t.Run("retry preserves original payload", func(t *testing.T) {
		var capturedPayload json.RawMessage
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusFailed, WorkflowVersion: 1,
					Payload: json.RawMessage(`{"env":"prod","batch_id":42}`),
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepFailed},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				capturedPayload = run.Payload
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		if err != nil {
			t.Fatalf("RetryWorkflowRun() error = %v", err)
		}
		if string(capturedPayload) != `{"env":"prod","batch_id":42}` {
			t.Fatalf("payload = %q, want original payload", string(capturedPayload))
		}
	})

	t.Run("retry canceled run with all steps completed", func(t *testing.T) {
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusCanceled, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					// Canceled run but step completed before cancellation
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"v":1}`)},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return nil, nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		newRun, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		if err != nil {
			t.Fatalf("RetryWorkflowRun() error = %v", err)
		}
		if newRun == nil {
			t.Fatal("expected non-nil run")
		}
		// Step a was completed, so should be pre-completed.
		if sr, ok := stepRunsCreated["a"]; !ok {
			t.Fatal("step a not created")
		} else if sr.Status != domain.StepCompleted {
			t.Fatalf("step a status = %q, want completed", sr.Status)
		}
	})

	t.Run("retry store error on get workflow run", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, fmt.Errorf("database connection error")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "run-1")
		if err == nil || !strings.Contains(err.Error(), "database connection error") {
			t.Fatalf("expected database error, got %v", err)
		}
	})

	t.Run("retry with fan-out DAG: a->{b,c} where c failed", func(t *testing.T) {
		stepRunsCreated := make(map[string]*domain.WorkflowStepRun)
		enqueuedJobs := make([]string, 0)

		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "orig-run", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusFailed, WorkflowVersion: 1,
				}, nil
			},
			getWorkflowFn: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
					{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"a"}},
				}, nil
			},
			listStepRunsByWorkflowRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{
					{ID: "orig-sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"a":1}`)},
					{ID: "orig-sr-b", StepRef: "b", Status: domain.StepCompleted, Output: json.RawMessage(`{"b":2}`)},
					{ID: "orig-sr-c", StepRef: "c", Status: domain.StepFailed, Error: "oom"},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "retry-run"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "retry-sr-" + sr.StepRef
				stepRunsCreated[sr.StepRef] = sr
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
				return map[string]json.RawMessage{"a": json.RawMessage(`{"a":1}`)}, nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueuedJobs = append(enqueuedJobs, run.JobID)
				run.ID = "job-run-" + run.JobID
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.RetryWorkflowRun(context.Background(), "orig-run")
		if err != nil {
			t.Fatalf("RetryWorkflowRun() error = %v", err)
		}

		// Step a: pre-completed. Step b: pre-completed. Step c: re-executed.
		if stepRunsCreated["a"].Status != domain.StepCompleted {
			t.Fatalf("step a should be pre-completed")
		}
		if stepRunsCreated["b"].Status != domain.StepCompleted {
			t.Fatalf("step b should be pre-completed")
		}
		// Only step c should be enqueued.
		if len(enqueuedJobs) != 1 || enqueuedJobs[0] != "job-c" {
			t.Fatalf("enqueued = %v, want [job-c]", enqueuedJobs)
		}
	})
}

func TestTriggerSubWorkflow(t *testing.T) {
	t.Run("happy path triggers child workflow", func(t *testing.T) {
		var createdRun *domain.WorkflowRun
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "child-run-1"
				createdRun = run
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		wfRun, err := engine.TriggerSubWorkflow(context.Background(), "wf-child", "proj-1", json.RawMessage(`{"from":"parent"}`), domain.TriggerWorkflow, "parent-run-1")
		if err != nil {
			t.Fatalf("TriggerSubWorkflow() error = %v", err)
		}
		if wfRun == nil || wfRun.ID != "child-run-1" {
			t.Fatalf("unexpected workflow run: %+v", wfRun)
		}
		if createdRun == nil {
			t.Fatal("expected child workflow run to be created")
		}
		if createdRun.ParentWorkflowRunID != "parent-run-1" {
			t.Fatalf("ParentWorkflowRunID = %q, want parent-run-1", createdRun.ParentWorkflowRunID)
		}
	})

	t.Run("inherits project ID from parent", func(t *testing.T) {
		parentRun := &domain.WorkflowRun{ID: "parent-run-1", ProjectID: "proj-parent"}
		var createdRun *domain.WorkflowRun

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: parentRun.ProjectID, Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "child-run-2"
				createdRun = run
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-child", parentRun.ProjectID, json.RawMessage(`{"from":"parent"}`), domain.TriggerWorkflow, parentRun.ID)
		if err != nil {
			t.Fatalf("TriggerSubWorkflow() error = %v", err)
		}
		if createdRun == nil {
			t.Fatal("expected child workflow run to be created")
		}
		if createdRun.ProjectID != parentRun.ProjectID {
			t.Fatalf("ProjectID = %q, want %q", createdRun.ProjectID, parentRun.ProjectID)
		}
	})
}

func TestStartSubWorkflowStep(t *testing.T) {
	t.Run("triggers sub-workflow and sets step running", func(t *testing.T) {
		stepRunningUpdated := false
		var parentRunID string
		childTriggered := false

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				switch id {
				case "wf-parent":
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
				case "wf-child":
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
				default:
					return nil, nil
				}
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				if run.WorkflowID == "wf-parent" {
					run.ID = "wr-parent"
					parentRunID = run.ID
					return nil
				}
				if run.WorkflowID == "wf-child" {
					run.ID = "wr-child"
					if run.ParentWorkflowRunID == parentRunID {
						childTriggered = true
					}
					return nil
				}
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef + "-" + sr.WorkflowRunID
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
				if strings.Contains(id, "sr-sub-") && status == domain.StepRunning {
					stepRunningUpdated = true
				}
				return nil
			},
		}

		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-child"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf-parent", "proj-1", json.RawMessage(`{"hello":"world"}`), "manual", nil)
		if err != nil {
			t.Fatalf("TriggerWorkflow() error = %v", err)
		}
		if !stepRunningUpdated {
			t.Fatal("expected sub-workflow step to be set running")
		}
		if !childTriggered {
			t.Fatal("expected child sub-workflow trigger")
		}
	})

	t.Run("fails when nesting depth exceeded", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 1,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				if id == "ancestor-run" {
					return &domain.WorkflowRun{ID: "ancestor-run", ParentWorkflowRunID: ""}, nil
				}
				return nil, nil
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "ancestor-run")
		if err == nil || !strings.Contains(err.Error(), "nesting depth") {
			t.Fatalf("expected nesting depth error, got %v", err)
		}
	})

	t.Run("fails when sub-workflow is disabled", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				if id == "wf-parent" {
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
				}
				if id == "wf-child" {
					return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: false, Version: 1}, nil
				}
				return nil, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, nil)
		if err == nil || !strings.Contains(err.Error(), "disabled") {
			t.Fatalf("expected disabled workflow error, got %v", err)
		}
	})
}

func TestGetNestingDepth(t *testing.T) {
	t.Run("depth 0 for root workflow", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 2,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, nil)
		if err != nil {
			t.Fatalf("expected depth 0 to succeed, got %v", err)
		}
	})

	t.Run("depth 1 for single parent", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 2,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				if id == "p1" {
					return &domain.WorkflowRun{ID: "p1", ParentWorkflowRunID: ""}, nil
				}
				return nil, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "p1")
		if err != nil {
			t.Fatalf("expected depth 1 to succeed, got %v", err)
		}
	})

	t.Run("depth 2 for nested chain", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:              "step-sub",
						StepRef:         "sub",
						StepType:        domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID:   "wf-child",
						MaxNestingDepth: 3,
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "p2":
					return &domain.WorkflowRun{ID: "p2", ParentWorkflowRunID: "p1"}, nil
				case "p1":
					return &domain.WorkflowRun{ID: "p1", ParentWorkflowRunID: ""}, nil
				default:
					return nil, nil
				}
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "p2")
		if err != nil {
			t.Fatalf("expected depth 2 to succeed, got %v", err)
		}
	})

	t.Run("circular reference detected", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "B":
					return &domain.WorkflowRun{ID: "B", ParentWorkflowRunID: "A"}, nil
				case "A":
					return &domain.WorkflowRun{ID: "A", ParentWorkflowRunID: "B"}, nil
				default:
					return nil, nil
				}
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "B")
		if err == nil || !strings.Contains(err.Error(), "circular") {
			t.Fatalf("expected circular reference error, got %v", err)
		}
	})

	t.Run("parent not found returns depth so far", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
				if workflowID == "wf-parent" {
					return []domain.WorkflowStep{{
						ID:            "step-sub",
						StepRef:       "sub",
						StepType:      domain.WorkflowStepTypeSubWorkflow,
						SubWorkflowID: "wf-child",
					}}, nil
				}
				return []domain.WorkflowStep{{ID: "step-child", JobID: "job-child", StepRef: "child-root"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-" + run.WorkflowID
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		mq := &mockEngineQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "jr-1"
			return nil
		}}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		_, err := engine.TriggerSubWorkflow(context.Background(), "wf-parent", "proj-1", nil, domain.TriggerWorkflow, "missing-parent")
		if err != nil {
			t.Fatalf("expected no error when parent not found, got %v", err)
		}
	})
}

func TestGetNestingDepth_Direct(t *testing.T) {
	t.Run("no parent", func(t *testing.T) {
		engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "run-a"})
		if err != nil {
			t.Fatalf("getNestingDepth() error = %v", err)
		}
		if depth != 0 {
			t.Fatalf("depth = %d, want 0", depth)
		}
	})

	t.Run("single parent", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				if id == "parent" {
					return &domain.WorkflowRun{ID: "parent"}, nil
				}
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "parent"})
		if err != nil {
			t.Fatalf("getNestingDepth() error = %v", err)
		}
		if depth != 1 {
			t.Fatalf("depth = %d, want 1", depth)
		}
	})

	t.Run("three levels deep", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "parent":
					return &domain.WorkflowRun{ID: "parent", ParentWorkflowRunID: "grandparent"}, nil
				case "grandparent":
					return &domain.WorkflowRun{ID: "grandparent"}, nil
				default:
					return nil, nil
				}
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "parent"})
		if err != nil {
			t.Fatalf("getNestingDepth() error = %v", err)
		}
		if depth != 2 {
			t.Fatalf("depth = %d, want 2", depth)
		}
	})

	t.Run("circular reference", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				switch id {
				case "run-b":
					return &domain.WorkflowRun{ID: "run-b", ParentWorkflowRunID: "run-a"}, nil
				case "run-a":
					return &domain.WorkflowRun{ID: "run-a", ParentWorkflowRunID: "run-b"}, nil
				default:
					return nil, nil
				}
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "run-a", ParentWorkflowRunID: "run-b"})
		if err == nil || !strings.Contains(err.Error(), "circular parent reference") {
			t.Fatalf("expected circular parent reference error, got %v", err)
		}
	})

	t.Run("parent not found", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, nil
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		depth, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "missing"})
		if err != nil {
			t.Fatalf("getNestingDepth() error = %v", err)
		}
		if depth != 1 {
			t.Fatalf("depth = %d, want 1", depth)
		}
	})

	t.Run("store error", func(t *testing.T) {
		ms := &mockEngineStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, errors.New("db error")
			},
		}
		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.getNestingDepth(context.Background(), &domain.WorkflowRun{ID: "child", ParentWorkflowRunID: "parent"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// propagateToParent tests — exercised indirectly through OnJobRunTerminal.

func TestPropagateToParent_ChildCompleted(t *testing.T) {
	// Flow: job run completed → step completed → fanIn (no children) →
	// checkWorkflowCompletion (all terminal) → mark child completed →
	// propagateToParent → find parent step → mark parent step completed →
	// fanIn on parent (no deps) → checkWorkflowCompletion on parent.

	parentStepCompleted := false
	childWfMarkedCompleted := false
	parentWfMarkedCompleted := false

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
			if jobRunID != "jr-child-1" {
				return nil, nil
			}
			return &domain.WorkflowStepRun{
				ID:            "sr-child-root",
				WorkflowRunID: "child-run-1",
				StepRef:       "child-root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-child-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
			if id == "sr-parent-sub" && status == domain.StepCompleted {
				parentStepCompleted = true
				// Verify output contains aggregated child outputs
				if out, ok := fields["output"]; ok {
					raw, _ := json.Marshal(out)
					if len(raw) == 0 {
						t.Error("expected non-empty output for parent step")
					}
				}
			}
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil // no deps to fan-in
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			switch id {
			case "child-run-1":
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "parent-run-1",
				}, nil
			case "parent-run-1":
				return &domain.WorkflowRun{
					ID:              "parent-run-1",
					WorkflowID:      "wf-parent",
					WorkflowVersion: 1,
					Status:          domain.WfStatusRunning,
				}, nil
			default:
				return nil, nil
			}
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			if id == "child-run-1" && to == domain.WfStatusCompleted {
				childWfMarkedCompleted = true
			}
			if id == "parent-run-1" && to == domain.WfStatusCompleted {
				parentWfMarkedCompleted = true
			}
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
			switch workflowRunID {
			case "child-run-1":
				return []domain.WorkflowStepRun{
					{ID: "sr-child-root", WorkflowRunID: "child-run-1", StepRef: "child-root", Status: domain.StepCompleted, Output: json.RawMessage(`{"result":"ok"}`)},
				}, nil
			case "parent-run-1":
				return []domain.WorkflowStepRun{
					{ID: "sr-parent-sub", WorkflowRunID: "parent-run-1", StepRef: "sub-step", Status: domain.StepCompleted},
				}, nil
			default:
				return nil, nil
			}
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
			switch workflowID {
			case "wf-child":
				return []domain.WorkflowStep{
					{ID: "step-child-root", StepRef: "child-root", JobID: "job-c1"},
				}, nil
			case "wf-parent":
				return []domain.WorkflowStep{
					{ID: "step-parent-sub", StepRef: "sub-step", StepType: domain.WorkflowStepTypeSubWorkflow, SubWorkflowID: "wf-child"},
				}, nil
			default:
				return nil, nil
			}
		},
		getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
			if workflowRunID == "parent-run-1" && stepRef == "sub-step" {
				return &domain.WorkflowStepRun{
					ID:            "sr-parent-sub",
					WorkflowRunID: "parent-run-1",
					StepRef:       "sub-step",
					Status:        domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-child-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"result":"ok"}`),
		WorkflowStepRunID: "sr-child-root",
	})
	if err != nil {
		t.Fatalf("OnJobRunTerminal() error = %v", err)
	}

	if !childWfMarkedCompleted {
		t.Fatal("expected child workflow run to be marked completed")
	}
	if !parentStepCompleted {
		t.Fatal("expected parent step run to be marked completed")
	}
	if !parentWfMarkedCompleted {
		t.Fatal("expected parent workflow run to be marked completed")
	}
}

func TestPropagateToParent_ChildFailed(t *testing.T) {
	// Flow: job run fails → step fails → handleFailedStep (fail_workflow) →
	// mark child workflow failed → cancelRemainingSteps → propagateToParent →
	// mark parent step failed → handleFailedStep on parent.

	parentStepFailed := false
	childWfMarkedFailed := false
	parentWfMarkedFailed := false

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
			if jobRunID != "jr-child-1" {
				return nil, nil
			}
			return &domain.WorkflowStepRun{
				ID:            "sr-child-root",
				WorkflowRunID: "child-run-1",
				StepRef:       "child-root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-child-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id == "sr-parent-sub" && status == domain.StepFailed {
				parentStepFailed = true
			}
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			switch id {
			case "child-run-1":
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "parent-run-1",
				}, nil
			case "parent-run-1":
				return &domain.WorkflowRun{
					ID:              "parent-run-1",
					WorkflowID:      "wf-parent",
					WorkflowVersion: 1,
					Status:          domain.WfStatusRunning,
				}, nil
			default:
				return nil, nil
			}
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			if id == "child-run-1" && to == domain.WfStatusFailed {
				childWfMarkedFailed = true
			}
			if id == "parent-run-1" && to == domain.WfStatusFailed {
				parentWfMarkedFailed = true
			}
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
			switch workflowRunID {
			case "child-run-1":
				// All step runs already terminal (the one step is failed)
				return []domain.WorkflowStepRun{
					{ID: "sr-child-root", WorkflowRunID: "child-run-1", StepRef: "child-root", Status: domain.StepFailed, Error: "job failed"},
				}, nil
			case "parent-run-1":
				return []domain.WorkflowStepRun{
					{ID: "sr-parent-sub", WorkflowRunID: "parent-run-1", StepRef: "sub-step", Status: domain.StepFailed},
				}, nil
			default:
				return nil, nil
			}
		},
		listStepsByWorkflowVerFn: func(_ context.Context, workflowID string, _ int) ([]domain.WorkflowStep, error) {
			switch workflowID {
			case "wf-child":
				return []domain.WorkflowStep{
					{ID: "step-child-root", StepRef: "child-root", JobID: "job-c1", OnFailure: domain.FailWorkflow},
				}, nil
			case "wf-parent":
				return []domain.WorkflowStep{
					{ID: "step-parent-sub", StepRef: "sub-step", StepType: domain.WorkflowStepTypeSubWorkflow, SubWorkflowID: "wf-child", OnFailure: domain.FailWorkflow},
				}, nil
			default:
				return nil, nil
			}
		},
		getStepRunByRunAndRefFn: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
			if workflowRunID == "parent-run-1" && stepRef == "sub-step" {
				return &domain.WorkflowStepRun{
					ID:            "sr-parent-sub",
					WorkflowRunID: "parent-run-1",
					StepRef:       "sub-step",
					Status:        domain.StepRunning,
				}, nil
			}
			return nil, nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-child-1",
		Status:            domain.StatusFailed,
		Error:             "job failed",
		WorkflowStepRunID: "sr-child-root",
	})
	if err != nil {
		t.Fatalf("OnJobRunTerminal() error = %v", err)
	}

	if !childWfMarkedFailed {
		t.Fatal("expected child workflow run to be marked failed")
	}
	if !parentStepFailed {
		t.Fatal("expected parent step run to be marked failed")
	}
	if !parentWfMarkedFailed {
		t.Fatal("expected parent workflow run to be marked failed")
	}
}

func TestPropagateToParent_NoParent(t *testing.T) {
	// When ParentWorkflowRunID is empty, propagateToParent is a no-op.
	// The parent's GetWorkflowRun should never be called.

	parentLookedUp := false
	getRunCalls := 0

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "child-run-1",
				StepRef:       "root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			getRunCalls++
			if id == "child-run-1" {
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "", // No parent
				}, nil
			}
			// Any other call means we tried to look up a parent
			parentLookedUp = true
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", WorkflowRunID: "child-run-1", StepRef: "root", Status: domain.StepCompleted},
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", StepRef: "root", JobID: "job-1"},
			}, nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"ok":true}`),
		WorkflowStepRunID: "sr-1",
	})
	if err != nil {
		t.Fatalf("OnJobRunTerminal() error = %v", err)
	}

	if parentLookedUp {
		t.Fatal("expected no parent lookup when ParentWorkflowRunID is empty")
	}
}

func TestPropagateToParent_ParentAlreadyTerminal(t *testing.T) {
	// When the parent workflow run is already terminal, propagation stops.
	// GetStepRunByWorkflowRunAndRef should NOT be called.

	stepRunLookedUp := false

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "child-run-1",
				StepRef:       "root",
				Status:        domain.StepRunning,
				JobRunID:      "jr-1",
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			switch id {
			case "child-run-1":
				return &domain.WorkflowRun{
					ID:                  "child-run-1",
					WorkflowID:          "wf-child",
					WorkflowVersion:     1,
					Status:              domain.WfStatusRunning,
					ParentWorkflowRunID: "parent-run-1",
				}, nil
			case "parent-run-1":
				return &domain.WorkflowRun{
					ID:              "parent-run-1",
					WorkflowID:      "wf-parent",
					WorkflowVersion: 1,
					Status:          domain.WfStatusCompleted, // Already terminal
				}, nil
			default:
				return nil, nil
			}
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", WorkflowRunID: "child-run-1", StepRef: "root", Status: domain.StepCompleted},
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{ID: "step-1", StepRef: "root", JobID: "job-1"},
			}, nil
		},
		getStepRunByRunAndRefFn: func(_ context.Context, _, _ string) (*domain.WorkflowStepRun, error) {
			stepRunLookedUp = true
			return nil, nil
		},
	}

	engine := NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default())
	cb := NewStepCallback(ms, engine, slog.Default())

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:                "jr-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(`{"ok":true}`),
		WorkflowStepRunID: "sr-1",
	})
	if err != nil {
		t.Fatalf("OnJobRunTerminal() error = %v", err)
	}

	if stepRunLookedUp {
		t.Fatal("expected GetStepRunByWorkflowRunAndRef not to be called when parent is terminal")
	}
}

func TestApplyStepOverrides(t *testing.T) {
	t.Run("no overrides returns original steps", func(t *testing.T) {
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
		}

		gotNil, err := applyStepOverrides(steps, nil)
		if err != nil {
			t.Fatalf("applyStepOverrides() with nil overrides error = %v", err)
		}
		if len(gotNil) != len(steps) {
			t.Fatalf("len(gotNil) = %d, want %d", len(gotNil), len(steps))
		}
		if gotNil[0].StepRef != "a" || gotNil[1].StepRef != "b" {
			t.Fatalf("unexpected step refs with nil overrides: %v, %v", gotNil[0].StepRef, gotNil[1].StepRef)
		}
		if len(gotNil[1].DependsOn) != 1 || gotNil[1].DependsOn[0] != "a" {
			t.Fatalf("unexpected depends_on with nil overrides: %+v", gotNil[1].DependsOn)
		}

		gotEmpty, err := applyStepOverrides(steps, []domain.StepOverride{})
		if err != nil {
			t.Fatalf("applyStepOverrides() with empty overrides error = %v", err)
		}
		if len(gotEmpty) != len(steps) {
			t.Fatalf("len(gotEmpty) = %d, want %d", len(gotEmpty), len(steps))
		}
		if gotEmpty[0].StepRef != "a" || gotEmpty[1].StepRef != "b" {
			t.Fatalf("unexpected step refs with empty overrides: %v, %v", gotEmpty[0].StepRef, gotEmpty[1].StepRef)
		}
		if len(gotEmpty[1].DependsOn) != 1 || gotEmpty[1].DependsOn[0] != "a" {
			t.Fatalf("unexpected depends_on with empty overrides: %+v", gotEmpty[1].DependsOn)
		}
	})

	t.Run("disable one step", func(t *testing.T) {
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
			{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"b"}},
		}

		got, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "b", Enabled: false}})
		if err != nil {
			t.Fatalf("applyStepOverrides() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		if got[0].StepRef != "a" || got[1].StepRef != "c" {
			t.Fatalf("unexpected filtered step refs: %v, %v", got[0].StepRef, got[1].StepRef)
		}
		if len(got[1].DependsOn) != 0 {
			t.Fatalf("expected c depends_on pruned, got %+v", got[1].DependsOn)
		}
	})

	t.Run("unknown step_ref returns error", func(t *testing.T) {
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
		}

		_, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "nonexistent", Enabled: false}})
		if err == nil || !strings.Contains(err.Error(), "unknown step_ref") {
			t.Fatalf("expected unknown step_ref error, got %v", err)
		}
	})

	t.Run("all steps disabled returns error", func(t *testing.T) {
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b"},
		}

		_, err := applyStepOverrides(steps, []domain.StepOverride{
			{StepRef: "a", Enabled: false},
			{StepRef: "b", Enabled: false},
		})
		if err == nil || !strings.Contains(err.Error(), "all steps disabled") {
			t.Fatalf("expected all steps disabled error, got %v", err)
		}
	})

	t.Run("prunes depends_on for disabled step", func(t *testing.T) {
		steps := []domain.WorkflowStep{
			{ID: "step-a", JobID: "job-a", StepRef: "a"},
			{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
			{ID: "step-c", JobID: "job-c", StepRef: "c", DependsOn: []string{"a", "b"}},
		}

		got, err := applyStepOverrides(steps, []domain.StepOverride{{StepRef: "b", Enabled: false}})
		if err != nil {
			t.Fatalf("applyStepOverrides() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		if got[1].StepRef != "c" {
			t.Fatalf("expected second step ref c, got %s", got[1].StepRef)
		}
		if len(got[1].DependsOn) != 1 || got[1].DependsOn[0] != "a" {
			t.Fatalf("expected c depends_on to be [a], got %+v", got[1].DependsOn)
		}
	})
}

func TestTriggerWorkflowWithStepOverrides(t *testing.T) {
	t.Run("overrides filter steps at trigger", func(t *testing.T) {
		createdStepRefs := make([]string, 0)
		enqueueCount := 0

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{ID: "step-a", JobID: "job-a", StepRef: "a"},
					{ID: "step-b", JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}},
				}, nil
			},
			createWorkflowRunFn: func(_ context.Context, run *domain.WorkflowRun) error {
				run.ID = "wr-override"
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			createWorkflowStepRunFn: func(_ context.Context, sr *domain.WorkflowStepRun) error {
				sr.ID = "sr-" + sr.StepRef
				createdStepRefs = append(createdStepRefs, sr.StepRef)
				return nil
			},
			updateStepRunStatusFn: func(_ context.Context, id string, _ domain.StepRunStatus, _ map[string]any) error {
				if id != "sr-a" {
					t.Fatalf("unexpected step run status update for %s", id)
				}
				return nil
			},
		}

		mq := &mockEngineQueue{
			enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				enqueueCount++
				run.ID = "jr-a"
				if run.JobID != "job-a" {
					t.Fatalf("unexpected enqueued job id: %s", run.JobID)
				}
				if run.WorkflowStepRunID != "sr-a" {
					t.Fatalf("unexpected enqueued workflow_step_run_id: %s", run.WorkflowStepRunID)
				}
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, mq, slog.Default())
		wfRun, err := engine.TriggerWorkflow(
			context.Background(),
			"wf-1",
			"proj-1",
			json.RawMessage(`{"k":"v"}`),
			"manual",
			[]domain.StepOverride{{StepRef: "b", Enabled: false}},
		)
		if err != nil {
			t.Fatalf("TriggerWorkflow() error = %v", err)
		}
		if wfRun == nil || wfRun.ID != "wr-override" || wfRun.Status != domain.WfStatusRunning {
			t.Fatalf("unexpected workflow run: %+v", wfRun)
		}
		if len(createdStepRefs) != 1 || createdStepRefs[0] != "a" {
			t.Fatalf("created step refs = %+v, want [a]", createdStepRefs)
		}
		if enqueueCount != 1 {
			t.Fatalf("enqueue count = %d, want 1", enqueueCount)
		}
	})

	t.Run("unknown override step_ref returns error", func(t *testing.T) {
		createWorkflowRunCalled := false

		ms := &mockEngineStore{
			getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "step-a", JobID: "job-a", StepRef: "a"}}, nil
			},
			createWorkflowRunFn: func(_ context.Context, _ *domain.WorkflowRun) error {
				createWorkflowRunCalled = true
				return nil
			},
		}

		engine := NewWorkflowEngine(ms, &mockEngineQueue{}, slog.Default())
		_, err := engine.TriggerWorkflow(
			context.Background(),
			"wf-1",
			"proj-1",
			nil,
			"manual",
			[]domain.StepOverride{{StepRef: "nonexistent", Enabled: false}},
		)
		if err == nil {
			t.Fatal("expected error for unknown override step_ref")
		}
		if !strings.Contains(err.Error(), "unknown step_ref") {
			t.Fatalf("expected unknown step_ref error, got %v", err)
		}
		if createWorkflowRunCalled {
			t.Fatal("expected workflow run not to be created when overrides are invalid")
		}
	})
}
