package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/google/go-cmp/cmp"
)

// mockCompensationStore implements CompensationStore for testing.
type mockCompensationStore struct {
	workflowRun *domain.WorkflowRun
	workflow    *domain.Workflow
	steps       []domain.WorkflowStep
	stepRuns    []domain.WorkflowStepRun

	createdStepRuns []domain.WorkflowStepRun
	canceledSteps   []string
	canceledRuns    []string
	updatedWfStatus domain.WorkflowRunStatus
}

func (m *mockCompensationStore) GetWorkflowRun(_ context.Context, id string) (*domain.WorkflowRun, error) {
	if m.workflowRun != nil && m.workflowRun.ID == id {
		return m.workflowRun, nil
	}
	return nil, nil
}

func (m *mockCompensationStore) GetWorkflow(_ context.Context, id string) (*domain.Workflow, error) {
	if m.workflow != nil && m.workflow.ID == id {
		return m.workflow, nil
	}
	return nil, nil
}

func (m *mockCompensationStore) ListStepsByWorkflowVersion(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
	return m.steps, nil
}

func (m *mockCompensationStore) ListStepRunsByWorkflowRun(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
	return m.stepRuns, nil
}

func (m *mockCompensationStore) UpdateWorkflowRunStatus(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
	m.updatedWfStatus = to
	return nil
}

func (m *mockCompensationStore) UpdateStepRunStatus(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
	if status == domain.StepCanceled {
		m.canceledSteps = append(m.canceledSteps, id)
	}
	return nil
}

func (m *mockCompensationStore) UpdateRunStatus(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
	m.canceledRuns = append(m.canceledRuns, id)
	return nil
}

func (m *mockCompensationStore) CreateWorkflowStepRun(_ context.Context, sr *domain.WorkflowStepRun) error {
	sr.ID = fmt.Sprintf("comp-sr-%d", len(m.createdStepRuns)+1)
	m.createdStepRuns = append(m.createdStepRuns, *sr)
	return nil
}

// mockCompQueue implements EngineQueue for testing.
type mockCompQueue struct {
	enqueuedRuns []domain.JobRun
}

func (m *mockCompQueue) Enqueue(_ context.Context, run *domain.JobRun) error {
	run.ID = fmt.Sprintf("comp-run-%d", len(m.enqueuedRuns)+1)
	m.enqueuedRuns = append(m.enqueuedRuns, *run)
	return nil
}

func TestCancelWorkflowRun_NoCompensation(t *testing.T) {
	t.Parallel()
	now := time.Now()
	store := &mockCompensationStore{
		workflowRun: &domain.WorkflowRun{
			ID:              "wfr-1",
			WorkflowID:      "wf-1",
			ProjectID:       "proj-1",
			Status:          domain.WfStatusRunning,
			WorkflowVersion: 1,
		},
		steps: []domain.WorkflowStep{
			{ID: "s1", WorkflowID: "wf-1", StepRef: "step-a", JobID: "job-a"},
			{ID: "s2", WorkflowID: "wf-1", StepRef: "step-b", JobID: "job-b", DependsOn: []string{"step-a"}},
		},
		stepRuns: []domain.WorkflowStepRun{
			{ID: "sr-1", WorkflowRunID: "wfr-1", StepRef: "step-a", Status: domain.StepCompleted, FinishedAt: &now},
			{ID: "sr-2", WorkflowRunID: "wfr-1", StepRef: "step-b", Status: domain.StepRunning, JobRunID: "run-b"},
		},
	}

	queue := &mockCompQueue{}
	engine := NewCompensationEngine(store, queue, nil)

	compensated, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No compensation steps defined, so none should be triggered
	if len(compensated) != 0 {
		t.Errorf("expected 0 compensated, got %d", len(compensated))
	}

	// Running step should be canceled
	if len(store.canceledSteps) != 1 || store.canceledSteps[0] != "sr-2" {
		t.Errorf("expected sr-2 canceled, got %v", store.canceledSteps)
	}

	// Workflow should be marked canceled
	if store.updatedWfStatus != domain.WfStatusCanceled {
		t.Errorf("expected canceled status, got %s", store.updatedWfStatus)
	}
}

func TestCancelWorkflowRun_WithCompensation(t *testing.T) {
	t.Parallel()
	now := time.Now()
	earlier := now.Add(-time.Minute)
	store := &mockCompensationStore{
		workflowRun: &domain.WorkflowRun{
			ID:              "wfr-1",
			WorkflowID:      "wf-1",
			ProjectID:       "proj-1",
			Status:          domain.WfStatusRunning,
			WorkflowVersion: 1,
		},
		steps: []domain.WorkflowStep{
			{ID: "s1", WorkflowID: "wf-1", StepRef: "allocate-gpu", JobID: "job-allocate", CompensateStepRef: "release-gpu"},
			{ID: "s2", WorkflowID: "wf-1", StepRef: "run-inference", JobID: "job-inference", DependsOn: []string{"allocate-gpu"}},
			{ID: "s3", WorkflowID: "wf-1", StepRef: "release-gpu", JobID: "job-release"},
		},
		stepRuns: []domain.WorkflowStepRun{
			{ID: "sr-1", WorkflowRunID: "wfr-1", StepRef: "allocate-gpu", Status: domain.StepCompleted,
				Output: json.RawMessage(`{"gpu_id": "gpu-42"}`), FinishedAt: &earlier},
			{ID: "sr-2", WorkflowRunID: "wfr-1", StepRef: "run-inference", Status: domain.StepRunning, JobRunID: "run-infer"},
		},
	}

	queue := &mockCompQueue{}
	engine := NewCompensationEngine(store, queue, nil)

	compensated, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have compensated the allocate-gpu step
	if diff := cmp.Diff([]string{"release-gpu"}, compensated); diff != "" {
		t.Errorf("compensated mismatch (-want +got):\n%s", diff)
	}

	// Should have created a compensation step run
	if len(store.createdStepRuns) != 1 {
		t.Fatalf("expected 1 compensation step run, got %d", len(store.createdStepRuns))
	}
	if store.createdStepRuns[0].StepRef != "release-gpu" {
		t.Errorf("expected release-gpu step run, got %s", store.createdStepRuns[0].StepRef)
	}

	// Should have enqueued a compensation job with the original output as payload
	if len(queue.enqueuedRuns) != 1 {
		t.Fatalf("expected 1 enqueued compensation job, got %d", len(queue.enqueuedRuns))
	}
	if queue.enqueuedRuns[0].JobID != "job-release" {
		t.Errorf("expected job-release, got %s", queue.enqueuedRuns[0].JobID)
	}
	if string(queue.enqueuedRuns[0].Payload) != `{"gpu_id": "gpu-42"}` {
		t.Errorf("expected gpu payload, got %s", queue.enqueuedRuns[0].Payload)
	}
	if queue.enqueuedRuns[0].TriggeredBy != "compensation" {
		t.Errorf("expected compensation trigger, got %s", queue.enqueuedRuns[0].TriggeredBy)
	}
}

func TestCancelWorkflowRun_MultipleCompensation_ReverseOrder(t *testing.T) {
	t.Parallel()
	t1 := time.Now().Add(-3 * time.Minute)
	t2 := time.Now().Add(-2 * time.Minute)
	t3 := time.Now().Add(-1 * time.Minute)

	store := &mockCompensationStore{
		workflowRun: &domain.WorkflowRun{
			ID:              "wfr-1",
			WorkflowID:      "wf-1",
			ProjectID:       "proj-1",
			Status:          domain.WfStatusRunning,
			WorkflowVersion: 1,
		},
		steps: []domain.WorkflowStep{
			{ID: "s1", StepRef: "step-a", JobID: "job-a", CompensateStepRef: "undo-a"},
			{ID: "s2", StepRef: "step-b", JobID: "job-b", CompensateStepRef: "undo-b", DependsOn: []string{"step-a"}},
			{ID: "s3", StepRef: "step-c", JobID: "job-c", CompensateStepRef: "undo-c", DependsOn: []string{"step-b"}},
			{ID: "s4", StepRef: "step-d", JobID: "job-d", DependsOn: []string{"step-c"}},
			{ID: "s5", StepRef: "undo-a", JobID: "job-undo-a"},
			{ID: "s6", StepRef: "undo-b", JobID: "job-undo-b"},
			{ID: "s7", StepRef: "undo-c", JobID: "job-undo-c"},
		},
		stepRuns: []domain.WorkflowStepRun{
			{ID: "sr-1", WorkflowRunID: "wfr-1", StepRef: "step-a", Status: domain.StepCompleted, FinishedAt: &t1},
			{ID: "sr-2", WorkflowRunID: "wfr-1", StepRef: "step-b", Status: domain.StepCompleted, FinishedAt: &t2},
			{ID: "sr-3", WorkflowRunID: "wfr-1", StepRef: "step-c", Status: domain.StepCompleted, FinishedAt: &t3},
			{ID: "sr-4", WorkflowRunID: "wfr-1", StepRef: "step-d", Status: domain.StepRunning, JobRunID: "run-d"},
		},
	}

	queue := &mockCompQueue{}
	engine := NewCompensationEngine(store, queue, nil)

	compensated, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should compensate in reverse order: c, b, a (most recently completed first)
	expected := []string{"undo-c", "undo-b", "undo-a"}
	if diff := cmp.Diff(expected, compensated); diff != "" {
		t.Errorf("compensation order mismatch (-want +got):\n%s", diff)
	}

	// Should have enqueued 3 compensation jobs
	if len(queue.enqueuedRuns) != 3 {
		t.Fatalf("expected 3 enqueued, got %d", len(queue.enqueuedRuns))
	}
}

func TestCancelWorkflowRun_AlreadyTerminal(t *testing.T) {
	t.Parallel()
	store := &mockCompensationStore{
		workflowRun: &domain.WorkflowRun{
			ID:     "wfr-1",
			Status: domain.WfStatusCompleted,
		},
	}

	queue := &mockCompQueue{}
	engine := NewCompensationEngine(store, queue, nil)

	_, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err == nil {
		t.Fatal("expected error for already terminal workflow")
	}
}

func TestCancelWorkflowRun_NotFound(t *testing.T) {
	t.Parallel()
	store := &mockCompensationStore{}
	queue := &mockCompQueue{}
	engine := NewCompensationEngine(store, queue, nil)

	_, err := engine.CancelWorkflowRun(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workflow run")
	}
}

func TestCompensateFailedWorkflow(t *testing.T) {
	t.Parallel()
	now := time.Now()
	store := &mockCompensationStore{
		workflowRun: &domain.WorkflowRun{
			ID:              "wfr-1",
			WorkflowID:      "wf-1",
			ProjectID:       "proj-1",
			Status:          domain.WfStatusFailed,
			WorkflowVersion: 1,
		},
		steps: []domain.WorkflowStep{
			{ID: "s1", StepRef: "provision", JobID: "job-provision", CompensateStepRef: "deprovision"},
			{ID: "s2", StepRef: "deploy", JobID: "job-deploy", DependsOn: []string{"provision"}},
			{ID: "s3", StepRef: "deprovision", JobID: "job-deprovision"},
		},
		stepRuns: []domain.WorkflowStepRun{
			{ID: "sr-1", WorkflowRunID: "wfr-1", StepRef: "provision", Status: domain.StepCompleted, FinishedAt: &now},
			{ID: "sr-2", WorkflowRunID: "wfr-1", StepRef: "deploy", Status: domain.StepFailed},
		},
	}

	queue := &mockCompQueue{}
	engine := NewCompensationEngine(store, queue, nil)

	compensated, err := engine.CompensateFailedWorkflow(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff := cmp.Diff([]string{"deprovision"}, compensated); diff != "" {
		t.Errorf("compensated mismatch (-want +got):\n%s", diff)
	}

	// Only the completed step with compensation should be triggered (not the failed step)
	if len(queue.enqueuedRuns) != 1 {
		t.Fatalf("expected 1 enqueued, got %d", len(queue.enqueuedRuns))
	}
}
