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
		wfRun, err := engine.TriggerWorkflow(context.Background(), "wf-1", "proj-1", json.RawMessage(`{"k":"v"}`), "manual")
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
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "")
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
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-1", nil, "")
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
		_, err := engine.TriggerWorkflow(context.Background(), "wf", "proj-b", nil, "")
		if err == nil || !strings.Contains(err.Error(), "does not belong") {
			t.Fatalf("expected project mismatch error, got %v", err)
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
