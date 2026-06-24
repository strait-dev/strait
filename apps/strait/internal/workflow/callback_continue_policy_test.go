package workflow

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	storepkg "strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleFailedStep_Continue_CallsIncrementStepDepsIncludingFailed(t *testing.T) {
	t.Parallel()
	var includingFailedCalled bool
	ms := &mockCallbackStore{
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]storepkg.StepDepResult, error) {
			t.Fatal("IncrementStepDeps must not be called for OnFailure Continue")
			return nil, nil
		},
		incrementStepDepsIncludingFailedFn: func(_ context.Context, _, _ string) ([]storepkg.StepDepResult, error) {
			includingFailedCalled = true
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a", OnFailure: domain.Continue},
			}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a", OnFailure: domain.Continue},
		},
	)
	require.NoError(t, cb.handleFailedStep(context.Background(), stepRun, wc))
	assert.True(t, includingFailedCalled)
}

func TestHandleFailedStep_FailWorkflow_DoesNotCallIncrementStepDepsIncludingFailed(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{
		incrementStepDepsIncludingFailedFn: func(_ context.Context, _, _ string) ([]storepkg.StepDepResult, error) {
			t.Fatal("IncrementStepDepsIncludingFailed must not be called for OnFailure FailWorkflow")
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a", OnFailure: domain.FailWorkflow},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepFailed, Error: "boom"}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a", OnFailure: domain.FailWorkflow},
		},
	)
	require.NoError(t, cb.handleFailedStep(context.Background(), stepRun, wc))
}

func TestDependentStepReachableAfterContinuePolicy(t *testing.T) {
	t.Parallel()
	var capturedRunID, capturedStepRef string
	ms := &mockCallbackStore{
		incrementStepDepsIncludingFailedFn: func(_ context.Context, runID, stepRef string) ([]storepkg.StepDepResult, error) {
			capturedRunID = runID
			capturedStepRef = stepRef
			return []storepkg.StepDepResult{
				{StepRunID: "sr-b", StepRef: "b", DepsCompleted: 1, DepsRequired: 1},
			}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _, _ string) ([]storepkg.StepDepResult, error) {
			t.Fatal("IncrementStepDeps must not be called for OnFailure Continue")
			return nil, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a", OnFailure: domain.Continue},
				{StepRef: "b", DependsOn: []string{"a"}},
			}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-a", StepRef: "a", Status: domain.StepFailed},
				{ID: "sr-b", StepRef: "b", Status: domain.StepWaiting, DepsCompleted: 0, DepsRequired: 1},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepFailed}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a", OnFailure: domain.Continue},
			{StepRef: "b", DependsOn: []string{"a"}},
		},
	)
	require.NoError(t, cb.handleFailedStep(context.Background(), stepRun, wc))
	assert.Equal(t, "wr-1", capturedRunID)
	assert.Equal(t, "a", capturedStepRef)
}
