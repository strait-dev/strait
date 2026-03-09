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
	updatedWfFields map[string]any
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

func (m *mockCompensationStore) UpdateWorkflowRunStatus(_ context.Context, _ string, _, to domain.WorkflowRunStatus, fields map[string]any) error {
	m.updatedWfStatus = to
	m.updatedWfFields = fields
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
	enqueuedRuns      []domain.JobRun
	enqueueShouldFail map[string]bool // jobID -> should fail
}

func (m *mockCompQueue) Enqueue(_ context.Context, run *domain.JobRun) error {
	if m.enqueueShouldFail != nil && m.enqueueShouldFail[run.JobID] {
		return fmt.Errorf("enqueue failed for %s", run.JobID)
	}
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

	result, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Compensated) != 0 {
		t.Errorf("expected 0 compensated, got %d", len(result.Compensated))
	}
	if result.Status != domain.CompensationNone {
		t.Errorf("expected status none, got %s", result.Status)
	}

	// Running step should be canceled
	if len(store.canceledSteps) != 1 || store.canceledSteps[0] != "sr-2" {
		t.Errorf("expected sr-2 canceled, got %v", store.canceledSteps)
	}

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

	result, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff := cmp.Diff([]string{"release-gpu"}, result.Compensated); diff != "" {
		t.Errorf("compensated mismatch (-want +got):\n%s", diff)
	}
	if result.Status != domain.CompensationCompleted {
		t.Errorf("expected completed status, got %s", result.Status)
	}

	if len(store.createdStepRuns) != 1 {
		t.Fatalf("expected 1 compensation step run, got %d", len(store.createdStepRuns))
	}
	if store.createdStepRuns[0].StepRef != "release-gpu" {
		t.Errorf("expected release-gpu step run, got %s", store.createdStepRuns[0].StepRef)
	}

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

	result, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"undo-c", "undo-b", "undo-a"}
	if diff := cmp.Diff(expected, result.Compensated); diff != "" {
		t.Errorf("compensation order mismatch (-want +got):\n%s", diff)
	}
	if result.Status != domain.CompensationCompleted {
		t.Errorf("expected completed status, got %s", result.Status)
	}

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

	result, err := engine.CompensateFailedWorkflow(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff := cmp.Diff([]string{"deprovision"}, result.Compensated); diff != "" {
		t.Errorf("compensated mismatch (-want +got):\n%s", diff)
	}
	if result.Status != domain.CompensationCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}

	if len(queue.enqueuedRuns) != 1 {
		t.Fatalf("expected 1 enqueued, got %d", len(queue.enqueuedRuns))
	}
}

func TestCancelWorkflowRun_PartialCompensation(t *testing.T) {
	t.Parallel()
	t1 := time.Now().Add(-2 * time.Minute)
	t2 := time.Now().Add(-1 * time.Minute)

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
			{ID: "s2", StepRef: "step-b", JobID: "job-b", CompensateStepRef: "undo-b"},
			{ID: "s3", StepRef: "undo-a", JobID: "job-undo-a"},
			{ID: "s4", StepRef: "undo-b", JobID: "job-undo-b"},
		},
		stepRuns: []domain.WorkflowStepRun{
			{ID: "sr-1", WorkflowRunID: "wfr-1", StepRef: "step-a", Status: domain.StepCompleted, FinishedAt: &t1},
			{ID: "sr-2", WorkflowRunID: "wfr-1", StepRef: "step-b", Status: domain.StepCompleted, FinishedAt: &t2},
		},
	}

	// Make one job fail to enqueue
	queue := &mockCompQueue{
		enqueueShouldFail: map[string]bool{"job-undo-b": true},
	}
	engine := NewCompensationEngine(store, queue, nil)

	result, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// undo-b should be in reverse order first but will fail, undo-a should succeed
	if result.Status != domain.CompensationPartial {
		t.Errorf("expected partial status, got %s", result.Status)
	}
	if len(result.Compensated) != 1 {
		t.Errorf("expected 1 compensated, got %d", len(result.Compensated))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestCancelWorkflowRun_AllCompensationFails(t *testing.T) {
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
			{ID: "s1", StepRef: "step-a", JobID: "job-a", CompensateStepRef: "undo-a"},
			{ID: "s2", StepRef: "undo-a", JobID: "job-undo-a"},
		},
		stepRuns: []domain.WorkflowStepRun{
			{ID: "sr-1", WorkflowRunID: "wfr-1", StepRef: "step-a", Status: domain.StepCompleted, FinishedAt: &now},
		},
	}

	queue := &mockCompQueue{
		enqueueShouldFail: map[string]bool{"job-undo-a": true},
	}
	engine := NewCompensationEngine(store, queue, nil)

	result, err := engine.CancelWorkflowRun(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.CompensationFailed {
		t.Errorf("expected failed status, got %s", result.Status)
	}
	if len(result.Compensated) != 0 {
		t.Errorf("expected 0 compensated, got %d", len(result.Compensated))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestRetryFailedCompensation(t *testing.T) {
	t.Parallel()

	store := &mockCompensationStore{
		workflowRun: &domain.WorkflowRun{
			ID:                 "wfr-1",
			WorkflowID:         "wf-1",
			ProjectID:          "proj-1",
			Status:             domain.WfStatusCanceled,
			WorkflowVersion:    1,
			CompensationStatus: domain.CompensationPartial,
		},
		steps: []domain.WorkflowStep{
			{ID: "s1", StepRef: "step-a", JobID: "job-a", CompensateStepRef: "undo-a"},
			{ID: "s2", StepRef: "undo-a", JobID: "job-undo-a"},
		},
		stepRuns: []domain.WorkflowStepRun{
			// The original step
			{ID: "sr-1", WorkflowRunID: "wfr-1", StepRef: "step-a", Status: domain.StepCompleted},
			// The failed compensation step run
			{ID: "comp-sr-1", WorkflowRunID: "wfr-1", StepRef: "undo-a", Status: domain.StepFailed, Attempt: 1},
		},
	}

	queue := &mockCompQueue{}
	engine := NewCompensationEngine(store, queue, nil)

	result, err := engine.RetryFailedCompensation(context.Background(), "wfr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.CompensationCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	if diff := cmp.Diff([]string{"undo-a"}, result.Compensated); diff != "" {
		t.Errorf("compensated mismatch (-want +got):\n%s", diff)
	}
	if len(queue.enqueuedRuns) != 1 {
		t.Fatalf("expected 1 re-enqueued, got %d", len(queue.enqueuedRuns))
	}
	if queue.enqueuedRuns[0].TriggeredBy != "compensation_retry" {
		t.Errorf("expected compensation_retry trigger, got %s", queue.enqueuedRuns[0].TriggeredBy)
	}
}

func TestRetryFailedCompensation_NotRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.CompensationStatus
	}{
		{"none", domain.CompensationNone},
		{"completed", domain.CompensationCompleted},
		{"running", domain.CompensationRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockCompensationStore{
				workflowRun: &domain.WorkflowRun{
					ID:                 "wfr-1",
					Status:             domain.WfStatusCanceled,
					CompensationStatus: tt.status,
				},
			}
			queue := &mockCompQueue{}
			engine := NewCompensationEngine(store, queue, nil)

			_, err := engine.RetryFailedCompensation(context.Background(), "wfr-1")
			if err == nil {
				t.Fatalf("expected error for compensation status %s", tt.status)
			}
		})
	}
}

func TestCompensationResult_StatusFields(t *testing.T) {
	t.Parallel()
	// Verify the compensation status constants are well-defined
	statuses := []domain.CompensationStatus{
		domain.CompensationNone,
		domain.CompensationPending,
		domain.CompensationRunning,
		domain.CompensationCompleted,
		domain.CompensationPartial,
		domain.CompensationFailed,
	}

	seen := make(map[domain.CompensationStatus]bool)
	for _, s := range statuses {
		if s == "" {
			t.Errorf("compensation status should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate compensation status: %s", s)
		}
		seen[s] = true
	}
}
