package workflow

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"
)

func newTestCallback(ms *mockCallbackStore) *StepCallback {
	return NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
}

func TestHandleFailedStep_SkipDependentsPolicy(t *testing.T) {
	skippedIDs := make(map[string]bool)
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a", OnFailure: domain.SkipDependents},
				{StepRef: "b", DependsOn: []string{"a"}},
				{StepRef: "c"},
			}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
				{ID: "sr-b", StepRef: "b", Status: domain.StepWaiting},
				{ID: "sr-c", StepRef: "c", Status: domain.StepCompleted},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if status == domain.StepSkipped {
				skippedIDs[id] = true
			}
			return nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepFailed}
	if err := cb.handleFailedStep(context.Background(), stepRun); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skippedIDs["sr-b"] {
		t.Fatal("expected sr-b to be skipped")
	}
	if skippedIDs["sr-c"] {
		t.Fatal("sr-c should not be skipped (not a dependent)")
	}
}

func TestHandleFailedStep_ContinuePolicy(t *testing.T) {
	workflowChecked := false
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a", OnFailure: domain.Continue},
			}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			workflowChecked = true
			return []domain.WorkflowStepRun{
				{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepFailed}
	if err := cb.handleFailedStep(context.Background(), stepRun); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !workflowChecked {
		t.Fatal("expected checkWorkflowCompletion to be called")
	}
}

func TestHandleFailedStep_DefaultPolicy(t *testing.T) {
	workflowFailed := false
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a"}, // No OnFailure set → defaults to fail_workflow.
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			if to == domain.WfStatusFailed {
				workflowFailed = true
			}
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepFailed, Error: "boom"}
	if err := cb.handleFailedStep(context.Background(), stepRun); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !workflowFailed {
		t.Fatal("expected workflow to fail with default policy")
	}
}

func TestCancelRemainingSteps(t *testing.T) {
	canceledIDs := make(map[string]bool)
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted},
				{ID: "sr-2", StepRef: "s2", Status: domain.StepWaiting},
				{ID: "sr-3", StepRef: "s3", Status: domain.StepPending},
				{ID: "sr-4", StepRef: "s4", Status: domain.StepFailed},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if status == domain.StepCanceled {
				canceledIDs[id] = true
			}
			return nil
		},
	}

	cb := newTestCallback(ms)
	if err := cb.cancelRemainingSteps(context.Background(), "wr-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canceledIDs["sr-2"] || !canceledIDs["sr-3"] {
		t.Fatalf("expected sr-2 and sr-3 to be canceled, got %v", canceledIDs)
	}
	if canceledIDs["sr-1"] || canceledIDs["sr-4"] {
		t.Fatal("terminal steps should not be canceled")
	}
}

func TestCheckWorkflowCompletion_AllCompleted(t *testing.T) {
	wfStatus := domain.WfStatusRunning
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted},
				{ID: "sr-2", StepRef: "s2", Status: domain.StepCompleted},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "s1"},
				{StepRef: "s2"},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			wfStatus = to
			return nil
		},
	}

	cb := newTestCallback(ms)
	if err := cb.checkWorkflowCompletion(context.Background(), "wr-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wfStatus != domain.WfStatusCompleted {
		t.Fatalf("expected workflow completed, got %s", wfStatus)
	}
}

func TestCheckWorkflowCompletion_HasNonTerminal(t *testing.T) {
	wfUpdated := false
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "s1", Status: domain.StepCompleted},
				{ID: "sr-2", StepRef: "s2", Status: domain.StepRunning},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			wfUpdated = true
			return nil
		},
	}

	cb := newTestCallback(ms)
	if err := cb.checkWorkflowCompletion(context.Background(), "wr-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wfUpdated {
		t.Fatal("workflow should not be updated when steps are still running")
	}
}

func TestCheckWorkflowCompletion_FailedWithContinuePolicy(t *testing.T) {
	wfStatus := domain.WfStatusRunning
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "s1", Status: domain.StepFailed},
				{ID: "sr-2", StepRef: "s2", Status: domain.StepCompleted},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "s1", OnFailure: domain.Continue},
				{StepRef: "s2"},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			wfStatus = to
			return nil
		},
	}

	cb := newTestCallback(ms)
	if err := cb.checkWorkflowCompletion(context.Background(), "wr-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wfStatus != domain.WfStatusCompleted {
		t.Fatalf("expected workflow completed (continue policy), got %s", wfStatus)
	}
}

func TestCheckWorkflowCompletion_FailedWithoutContinue(t *testing.T) {
	wfStatus := domain.WfStatusRunning
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "s1", Status: domain.StepFailed},
				{ID: "sr-2", StepRef: "s2", Status: domain.StepCompleted},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "s1", OnFailure: domain.FailWorkflow},
				{StepRef: "s2"},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			wfStatus = to
			return nil
		},
	}

	cb := newTestCallback(ms)
	if err := cb.checkWorkflowCompletion(context.Background(), "wr-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wfStatus != domain.WfStatusFailed {
		t.Fatalf("expected workflow failed, got %s", wfStatus)
	}
}

func TestSkipDependentSteps_TransitiveSkip(t *testing.T) {
	skippedIDs := make(map[string]bool)
	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{WorkflowVersion: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "b", DependsOn: []string{"a"}},
				{StepRef: "c", DependsOn: []string{"b"}},
				{StepRef: "d"}, // Independent step.
			}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
				{ID: "sr-b", StepRef: "b", Status: domain.StepWaiting},
				{ID: "sr-c", StepRef: "c", Status: domain.StepPending},
				{ID: "sr-d", StepRef: "d", Status: domain.StepRunning},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if status == domain.StepSkipped {
				skippedIDs[id] = true
			}
			return nil
		},
	}

	cb := newTestCallback(ms)
	if err := cb.skipDependentSteps(context.Background(), "wr-1", "wf-1", "a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skippedIDs["sr-b"] {
		t.Fatal("expected sr-b (direct dependent) to be skipped")
	}
	if !skippedIDs["sr-c"] {
		t.Fatal("expected sr-c (transitive dependent) to be skipped")
	}
	if skippedIDs["sr-d"] {
		t.Fatal("sr-d should not be skipped (independent)")
	}
}

func TestOnJobRunTerminal_UpdateStepStatusError(t *testing.T) {
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return errors.New("store error")
		},
	}

	cb := newTestCallback(ms)
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	if err == nil {
		t.Fatal("expected error from update step run status")
	}
}
