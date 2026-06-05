package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// prefetchStepOutputs tests.

// TestPrefetchStepOutputs_BatchesAllDeps verifies that a single
// GetStepOutputs call is made with the deduplicated union of all
// DependsOn refs across runnable step runs.
// Covers: callback_progression.go prefetchStepOutputs batching logic.
func TestPrefetchStepOutputs_BatchesAllDeps(t *testing.T) {
	t.Parallel()

	var capturedRefs []string
	callCount := 0

	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, refs []string) (map[string]json.RawMessage, error) {
			callCount++
			capturedRefs = refs
			out := make(map[string]json.RawMessage, len(refs))
			for _, r := range refs {
				out[r] = json.RawMessage(fmt.Sprintf(`{"ref":"%s"}`, r))
			}
			return out, nil
		},
	}
	cb := newTestCallback(ms)

	stepByRef := map[string]domain.WorkflowStep{
		"step-a": {StepRef: "step-a", DependsOn: []string{"root"}},
		"step-b": {StepRef: "step-b", DependsOn: []string{"root", "step-x"}},
		"step-c": {StepRef: "step-c", DependsOn: []string{"root", "step-y"}},
	}
	runnableStepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "step-a", Status: domain.StepPending, DepsCompleted: 1, DepsRequired: 1},
		{ID: "sr-b", StepRef: "step-b", Status: domain.StepPending, DepsCompleted: 2, DepsRequired: 2},
		{ID: "sr-c", StepRef: "step-c", Status: domain.StepPending, DepsCompleted: 2, DepsRequired: 2},
	}

	outputs, err := cb.prefetchStepOutputs(context.Background(), "wr-1", runnableStepRuns, stepByRef)
	require.NoError(t,
		err)
	require.EqualValues(t, 1,
		callCount)

	// Verify the unique dep refs: root, step-x, step-y (order may vary).
	sort.Strings(capturedRefs)
	want := []string{"root", "step-x", "step-y"}
	require.Len(t, capturedRefs,
		len(want))

	for i := range want {
		require.Equal(t, want[i], capturedRefs[i])

	}
	require.Len(t, outputs,
		3)

	// Verify outputs contain all 3 keys.

}

// TestPrefetchStepOutputs_NoDeps_ReturnsNil verifies that when all
// runnable steps have no DependsOn, GetStepOutputs is never called
// and nil is returned.
// Covers: callback_progression.go L646-648 (len(allDeps) == 0 early return).
func TestPrefetchStepOutputs_NoDeps_ReturnsNil(t *testing.T) {
	t.Parallel()

	called := false
	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			called = true
			return nil, nil
		},
	}
	cb := newTestCallback(ms)

	stepByRef := map[string]domain.WorkflowStep{
		"step-a": {StepRef: "step-a", DependsOn: nil},
		"step-b": {StepRef: "step-b", DependsOn: []string{}},
	}
	runnableStepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "step-a", Status: domain.StepPending},
		{ID: "sr-b", StepRef: "step-b", Status: domain.StepPending},
	}

	outputs, err := cb.prefetchStepOutputs(context.Background(), "wr-1", runnableStepRuns, stepByRef)
	require.NoError(t,
		err)
	require.Nil(t, outputs)
	require.False(t, called)

}

// TestPrefetchStepOutputs_SkipsTerminalAndRunning verifies that
// terminal (completed/failed/skipped/canceled) and running step runs
// are excluded from dependency collection.
// Covers: callback_progression.go L637-639 (IsTerminal / StepRunning guard).
func TestPrefetchStepOutputs_SkipsTerminalAndRunning(t *testing.T) {
	t.Parallel()

	var capturedRefs []string
	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, refs []string) (map[string]json.RawMessage, error) {
			capturedRefs = refs
			out := make(map[string]json.RawMessage, len(refs))
			for _, r := range refs {
				out[r] = json.RawMessage(`{}`)
			}
			return out, nil
		},
	}
	cb := newTestCallback(ms)

	stepByRef := map[string]domain.WorkflowStep{
		"completed-step": {StepRef: "completed-step", DependsOn: []string{"should-not-appear-1"}},
		"failed-step":    {StepRef: "failed-step", DependsOn: []string{"should-not-appear-2"}},
		"running-step":   {StepRef: "running-step", DependsOn: []string{"should-not-appear-3"}},
		"pending-step":   {StepRef: "pending-step", DependsOn: []string{"only-this-dep"}},
	}
	runnableStepRuns := []domain.WorkflowStepRun{
		{ID: "sr-1", StepRef: "completed-step", Status: domain.StepCompleted},
		{ID: "sr-2", StepRef: "failed-step", Status: domain.StepFailed},
		{ID: "sr-3", StepRef: "running-step", Status: domain.StepRunning},
		{ID: "sr-4", StepRef: "pending-step", Status: domain.StepPending},
	}

	outputs, err := cb.prefetchStepOutputs(context.Background(), "wr-1", runnableStepRuns, stepByRef)
	require.NoError(t,
		err)
	require.False(t, len(capturedRefs) !=
		1 || capturedRefs[0] != "only-this-dep",
	)
	require.Len(t, outputs,
		1)

}

// TestPrefetchStepOutputs_Error_PropagatedCorrectly verifies that a
// store error from GetStepOutputs is wrapped and propagated.
// Covers: callback_progression.go L654-656 (error wrapping).
func TestPrefetchStepOutputs_Error_PropagatedCorrectly(t *testing.T) {
	t.Parallel()

	storeErr := errors.New("db connection lost")
	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			return nil, storeErr
		},
	}
	cb := newTestCallback(ms)

	stepByRef := map[string]domain.WorkflowStep{
		"step-a": {StepRef: "step-a", DependsOn: []string{"root"}},
	}
	runnableStepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "step-a", Status: domain.StepPending},
	}

	_, err := cb.prefetchStepOutputs(context.Background(), "wr-1", runnableStepRuns, stepByRef)
	require.Error(t, err)
	require.True(t, errors.Is(err, storeErr))

}

// TestPrefetchStepOutputs_EmptySlice verifies that an empty runnable
// step run slice returns nil, nil without calling the store.
// Covers: callback_progression.go L646-648 (len(allDeps) == 0).
func TestPrefetchStepOutputs_EmptySlice(t *testing.T) {
	t.Parallel()

	called := false
	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			called = true
			return nil, nil
		},
	}
	cb := newTestCallback(ms)

	outputs, err := cb.prefetchStepOutputs(context.Background(), "wr-1", nil, nil)
	require.NoError(t,
		err)
	require.Nil(t, outputs)
	require.False(t, called)

}

// TestPrefetchStepOutputs_AllTerminal verifies that when every step
// run is terminal or running, no store call is made.
// Covers: callback_progression.go L637-639 + L646-648.
func TestPrefetchStepOutputs_AllTerminal(t *testing.T) {
	t.Parallel()

	called := false
	ms := &mockCallbackStore{
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			called = true
			return nil, nil
		},
	}
	cb := newTestCallback(ms)

	stepByRef := map[string]domain.WorkflowStep{
		"a": {StepRef: "a", DependsOn: []string{"x"}},
		"b": {StepRef: "b", DependsOn: []string{"y"}},
	}
	runs := []domain.WorkflowStepRun{
		{ID: "sr-1", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-2", StepRef: "b", Status: domain.StepRunning},
	}

	outputs, err := cb.prefetchStepOutputs(context.Background(), "wr-1", runs, stepByRef)
	require.NoError(t,
		err)
	require.Nil(t, outputs)
	require.False(t, called)

}

// scheduleRunnableSteps — condition evaluates to false → step skipped.

// TestScheduleRunnableSteps_ConditionFalse_StepSkipped verifies that
// when a step's condition evaluates to false, the step run is marked
// StepSkipped and is NOT started via the engine.
// Covers: callback_progression.go L110-118 (condition=false → skip path).
func TestScheduleRunnableSteps_ConditionFalse_StepSkipped(t *testing.T) {
	t.Parallel()

	var skippedID string
	var skippedStatus domain.StepRunStatus
	ms := &mockCallbackStore{
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			skippedID = id
			skippedStatus = status
			return nil
		},
		// GetStepOutputs returns empty (no deps to fetch).
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			return map[string]json.RawMessage{}, nil
		},
		// recordDecision needs createWorkflowStepDecisionFn to not panic.
		createWorkflowStepDecisionFn: func(_ context.Context, _ *domain.WorkflowStepDecision) error {
			return nil
		},
	}
	cb := newTestCallback(ms)

	// Build a condition that evaluates to false: step_status where
	// dep-x has status "completed" but stepStatuses says "failed".
	cond := json.RawMessage(`{"type":"step_status","step_ref":"dep-x","status":"completed"}`)

	steps := []domain.WorkflowStep{
		{StepRef: "guarded", DependsOn: []string{"dep-x"}, Condition: cond, OnFailure: domain.FailWorkflow},
	}
	stepStatuses := map[string]domain.StepRunStatus{
		"dep-x": domain.StepFailed, // Condition wants "completed" — will evaluate to false.
	}
	wfRun := &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}

	runnableStepRuns := []domain.WorkflowStepRun{
		{
			ID:            "sr-guarded",
			WorkflowRunID: "wr-1",
			StepRef:       "guarded",
			Status:        domain.StepPending,
			DepsCompleted: 1,
			DepsRequired:  1,
		},
	}

	err := cb.scheduleRunnableSteps(
		context.Background(),
		wfRun,
		steps,
		stepStatuses,
		nil, // no running steps
		runnableStepRuns,
	)
	require.NoError(t,
		err)
	require.Equal(t, "sr-guarded",
		skippedID,
	)
	require.Equal(t, domain.
		StepSkipped,
		skippedStatus,
	)
	require.Equal(t, domain.
		StepSkipped,
		stepStatuses["guarded"])

}

// Cost gate tests (engine_steps.go L99-119).

// NOTE: The mockEngineStore has GetJobCostEstimate hardcoded to return (nil, nil)
// with no configurable hook. To properly test the cost gate trigger path
// (engine_steps.go L97-135), the following changes would be needed:
//
// 1. Add `getJobCostEstimateFn func(ctx context.Context, jobID string) (*domain.JobCostEstimate, error)`
//    to mockEngineStore.
// 2. Update GetJobCostEstimate method to dispatch to the hook.
//
// The test would then:
//   - Set step.CostGateThresholdMicrousd = 1000, step.JobID = "job-1"
//   - Mock GetJobCostEstimate to return &domain.JobCostEstimate{AvgCostMicrousd: 2000}
//   - Verify UpdateStepRunStatus is called with StepWaiting
//   - Verify CreateWorkflowStepApproval is called with ID prefix "costgate:"
//   - Verify stepRun.Status is set to StepWaiting after startStep returns
//
// The existing tests TestStepExecution_CostGateZeroThreshold and
// TestStepExecution_CostGateNegativeThreshold in engine_adversarial_test.go
// only cover the NOT-triggered paths.

// TestCostGate_BelowThreshold_Proceeds verifies that when the cost estimate
// is below the threshold, the step proceeds normally (not gated).
// This works with the existing hardcoded mock that returns nil estimate.
// Covers: engine_steps.go L97-99 (condition short-circuits).
func TestCostGate_BelowThreshold_Proceeds(t *testing.T) {
	t.Parallel()

	enqueued := false
	es := &mockEngineStore{
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return nil
		},
		getStepOutputsFn: func(_ context.Context, _ string, _ []string) (map[string]json.RawMessage, error) {
			return nil, nil
		},
	}
	eq := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	engine := NewWorkflowEngine(es, eq, slog.Default())

	step := &domain.WorkflowStep{
		StepRef:                   "deploy",
		StepType:                  domain.WorkflowStepTypeJob,
		JobID:                     "job-1",
		CostGateThresholdMicrousd: 5000, // Threshold set, but mock returns nil estimate.
		OnFailure:                 domain.FailWorkflow,
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "deploy"}
	wfRun := &domain.WorkflowRun{
		ID:         "wr-1",
		WorkflowID: "wf-1",
		Status:     domain.WfStatusRunning,
	}
	require.NoError(t,
		engine.startStep(context.Background(), stepRun,
			step,
			wfRun, nil,
		),
	)
	require.True(t, enqueued)

}
