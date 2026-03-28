package workflow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestParseDynamicWorkflowExpansion(t *testing.T) {
	t.Parallel()

	t.Run("resolves agent ids and seeds completed dependencies", func(t *testing.T) {
		t.Parallel()

		ms := &mockCallbackStore{
			getAgentFn: func(_ context.Context, id string) (*domain.Agent, error) {
				if id != "agent-worker" {
					t.Fatalf("unexpected agent lookup: %s", id)
				}
				return &domain.Agent{ID: id, ProjectID: "proj-1", JobID: "job-worker"}, nil
			},
			listStepRunStatusesByWorkflowRunFn: func(_ context.Context, workflowRunID string) (map[string]domain.StepRunStatus, error) {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
				}
				return map[string]domain.StepRunStatus{
					"research": domain.StepCompleted,
					"plan":     domain.StepRunning,
				}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		stepRun := &domain.WorkflowStepRun{ID: "sr-plan", WorkflowRunID: "wr-1", StepRef: "plan", Status: domain.StepRunning}
		wc := &wfCtx{
			run: &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"},
			steps: []domain.WorkflowStep{
				{StepRef: "plan", StepType: domain.WorkflowStepTypeJob},
				{StepRef: "research", StepType: domain.WorkflowStepTypeJob},
			},
			stepByRef: map[string]domain.WorkflowStep{
				"plan":     {StepRef: "plan", StepType: domain.WorkflowStepTypeJob},
				"research": {StepRef: "research", StepType: domain.WorkflowStepTypeJob},
			},
		}

		output := json.RawMessage(`{"dynamic_steps":[{"step_ref":"draft","agent_id":"agent-worker","depends_on":["plan","research"]}]}`)
		expansions, err := cb.parseDynamicWorkflowExpansion(context.Background(), stepRun, wc, output)
		if err != nil {
			t.Fatalf("parseDynamicWorkflowExpansion() error = %v", err)
		}
		if len(expansions) != 1 {
			t.Fatalf("len(expansions) = %d, want 1", len(expansions))
		}
		if expansions[0].Step.JobID != "job-worker" {
			t.Fatalf("expansions[0].Step.JobID = %q, want job-worker", expansions[0].Step.JobID)
		}
		if expansions[0].StepRun.DepsCompleted != 1 {
			t.Fatalf("DepsCompleted = %d, want 1", expansions[0].StepRun.DepsCompleted)
		}
		if expansions[0].StepRun.DepsRequired != 2 {
			t.Fatalf("DepsRequired = %d, want 2", expansions[0].StepRun.DepsRequired)
		}
		if expansions[0].StepRun.Status != domain.StepWaiting {
			t.Fatalf("Status = %s, want waiting", expansions[0].StepRun.Status)
		}
	})

	t.Run("rejects cycles against the existing runtime dag", func(t *testing.T) {
		t.Parallel()

		cb := NewStepCallback(&mockCallbackStore{
			listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
				return map[string]domain.StepRunStatus{}, nil
			},
		}, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())

		stepRun := &domain.WorkflowStepRun{ID: "sr-plan", WorkflowRunID: "wr-1", StepRef: "plan"}
		wc := &wfCtx{
			run: &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"},
			steps: []domain.WorkflowStep{
				{StepRef: "plan"},
				{StepRef: "final", DependsOn: []string{"plan"}},
			},
			stepByRef: map[string]domain.WorkflowStep{
				"plan":  {StepRef: "plan"},
				"final": {StepRef: "final", DependsOn: []string{"plan"}},
			},
		}

		output := json.RawMessage(`{"dynamic_steps":[{"step_ref":"draft","job_id":"job-1","depends_on":["plan-bridge"]},{"step_ref":"plan-bridge","job_id":"job-2","depends_on":["draft"]}]}`)
		_, err := cb.parseDynamicWorkflowExpansion(context.Background(), stepRun, wc, output)
		if err == nil {
			t.Fatal("parseDynamicWorkflowExpansion() error = nil, want error")
		}
	})
}

func TestStepCallback_OnJobRunTerminal_DynamicExpansion(t *testing.T) {
	t.Parallel()

	t.Run("invalid dynamic steps fail the workflow step", func(t *testing.T) {
		t.Parallel()

		var updatedStatuses []domain.StepRunStatus
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-plan", WorkflowRunID: "wr-1", StepRef: "plan", Status: domain.StepRunning}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "plan", OnFailure: domain.FailWorkflow}}, nil
			},
			listDynamicWorkflowStepsByRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
				return nil, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, fields map[string]any) error {
				updatedStatuses = append(updatedStatuses, status)
				if status == domain.StepFailed {
					errMsg, _ := fields["error"].(string)
					if !strings.Contains(errMsg, "unknown step") {
						t.Fatalf("unexpected error message: %s", errMsg)
					}
				}
				return nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
				if to != domain.WfStatusFailed {
					t.Fatalf("workflow status = %s, want failed", to)
				}
				return nil
			},
			listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return []domain.WorkflowStepRun{{ID: "sr-plan", StepRef: "plan", Status: domain.StepFailed}}, nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
			ID:                "run-1",
			WorkflowStepRunID: "sr-plan",
			Status:            domain.StatusCompleted,
			Result:            json.RawMessage(`{"dynamic_steps":[{"step_ref":"draft","job_id":"job-1","depends_on":["missing"]}]}`),
		})
		if err != nil {
			t.Fatalf("OnJobRunTerminal() error = %v", err)
		}
		if len(updatedStatuses) == 0 || updatedStatuses[0] != domain.StepFailed {
			t.Fatalf("step statuses = %v, want first status failed", updatedStatuses)
		}
	})

	t.Run("persists dynamic expansions before fan-in", func(t *testing.T) {
		t.Parallel()

		persisted := make([]store.DynamicWorkflowExpansion, 0)
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-plan", WorkflowRunID: "wr-1", StepRef: "plan", Status: domain.StepRunning}, nil
			},
			getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "plan", OnFailure: domain.FailWorkflow}}, nil
			},
			listDynamicWorkflowStepsByRunFn: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
				return nil, nil
			},
			listStepRunStatusesByWorkflowRunFn: func(_ context.Context, _ string) (map[string]domain.StepRunStatus, error) {
				return map[string]domain.StepRunStatus{"plan": domain.StepRunning}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
				if status != domain.StepCompleted {
					t.Fatalf("status = %s, want completed", status)
				}
				return nil
			},
			createWorkflowDynamicExpansionFn: func(_ context.Context, workflowRunID, parentStepRunID string, expansions []store.DynamicWorkflowExpansion) error {
				if workflowRunID != "wr-1" || parentStepRunID != "sr-plan" {
					t.Fatalf("unexpected expansion target: %s %s", workflowRunID, parentStepRunID)
				}
				persisted = append(persisted, expansions...)
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			listRunningStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
			listRunnableStepRunsByWorkflowRunFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStepRun, error) {
				return nil, nil
			},
			countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
				return nil, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
				if to != domain.WfStatusCompleted {
					t.Fatalf("workflow status = %s, want completed", to)
				}
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
			ID:                "run-1",
			WorkflowStepRunID: "sr-plan",
			Status:            domain.StatusCompleted,
			Result:            json.RawMessage(`{"dynamic_steps":[{"step_ref":"draft","job_id":"job-draft","depends_on":["plan"]}]}`),
		})
		if err != nil {
			t.Fatalf("OnJobRunTerminal() error = %v", err)
		}
		if len(persisted) != 1 {
			t.Fatalf("len(persisted) = %d, want 1", len(persisted))
		}
		if persisted[0].Step.StepRef != "draft" {
			t.Fatalf("persisted step_ref = %q, want draft", persisted[0].Step.StepRef)
		}
	})
}

func FuzzParseDynamicWorkflowStepRequests(f *testing.F) {
	f.Add(`{"dynamic_steps":[{"step_ref":"draft","job_id":"job-1","depends_on":["plan"]}]}`)
	f.Add(`{"dynamic_steps":"nope"}`)
	f.Add(`[]`)

	f.Fuzz(func(t *testing.T, input string) {
		t.Parallel()
		_, _, _ = parseDynamicWorkflowStepRequests(json.RawMessage(input))
	})
}
