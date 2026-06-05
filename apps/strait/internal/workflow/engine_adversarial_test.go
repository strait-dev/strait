package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// newTestEngine creates a WorkflowEngine with the given mock store and queue.
func newTestEngine(store *mockEngineStore, queue *mockEngineQueue) *WorkflowEngine {
	return NewWorkflowEngine(store, queue, slog.Default())
}

// minimalWorkflowStore returns a mockEngineStore that provides the minimum viable
// responses for TriggerWorkflow to succeed with a single root job step.
func minimalWorkflowStore(wfID, projectID string, steps []domain.WorkflowStep) *mockEngineStore {
	return &mockEngineStore{
		getWorkflowFn: func(_ context.Context, id string) (*domain.Workflow, error) {
			if id != wfID {
				return nil, fmt.Errorf("unexpected workflow id: %s", id)
			}
			return &domain.Workflow{
				ID:        wfID,
				ProjectID: projectID,
				Enabled:   true,
				Version:   1,
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return steps, nil
		},
	}
}

// singleJobStep returns a minimal one-step workflow definition.
func singleJobStep() []domain.WorkflowStep {
	return []domain.WorkflowStep{
		{
			ID:         "s1",
			WorkflowID: "wf-1",
			StepRef:    "build",
			JobID:      "j-1",
			StepType:   domain.WorkflowStepTypeJob,
			OnFailure:  domain.FailWorkflow,
		},
	}
}

// TestWorkflowNesting_AtMaxDepth verifies that triggering a sub-workflow step
// at exactly DefaultMaxNestingDepth-1 current depth succeeds.
func TestWorkflowNesting_AtMaxDepth(t *testing.T) {
	t.Parallel()

	// Build a parent chain of depth DefaultMaxNestingDepth - 1 so the next
	// sub-workflow trigger is at the limit but not over it.
	depth := DefaultMaxNestingDepth - 1
	runs := make(map[string]*domain.WorkflowRun, depth+1)
	for i := 0; i <= depth; i++ {
		id := fmt.Sprintf("wr-%d", i)
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("wr-%d", i-1)
		}
		runs[id] = &domain.WorkflowRun{
			ID:                  id,
			WorkflowID:          "wf-child",
			ProjectID:           "proj-1",
			ParentWorkflowRunID: parent,
			Status:              domain.WfStatusRunning,
		}
	}

	leafID := fmt.Sprintf("wr-%d", depth)

	ms := minimalWorkflowStore("wf-child", "proj-1", singleJobStep())
	ms.getWorkflowRunFn = func(_ context.Context, id string) (*domain.WorkflowRun, error) {
		if r, ok := runs[id]; ok {
			return r, nil
		}
		return nil, fmt.Errorf("run not found: %s", id)
	}

	engine := newTestEngine(ms, &mockEngineQueue{})

	// getNestingDepth should return depth, which equals DefaultMaxNestingDepth - 1.
	got, err := engine.getNestingDepth(context.Background(), runs[leafID])
	require.NoError(t,
		err)
	require.Equal(t, depth,
		got)

	// Since depth < maxNestingDepth, a sub-workflow start should not be rejected
	// by the depth check itself. We only verify the depth calculation here.
}

// TestWorkflowNesting_OverMaxDepth verifies that nesting depth at or above the
// configured maximum causes startSubWorkflowStep to return an error.
func TestWorkflowNesting_OverMaxDepth(t *testing.T) {
	t.Parallel()

	maxDepth := 3

	// Build a chain that already has maxDepth levels.
	runs := make(map[string]*domain.WorkflowRun, maxDepth+1)
	for i := 0; i <= maxDepth; i++ {
		id := fmt.Sprintf("wr-%d", i)
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("wr-%d", i-1)
		}
		runs[id] = &domain.WorkflowRun{
			ID:                  id,
			WorkflowID:          "wf-child",
			ProjectID:           "proj-1",
			ParentWorkflowRunID: parent,
			Status:              domain.WfStatusRunning,
		}
	}

	leafID := fmt.Sprintf("wr-%d", maxDepth)

	ms := minimalWorkflowStore("wf-child", "proj-1", singleJobStep())
	ms.getWorkflowRunFn = func(_ context.Context, id string) (*domain.WorkflowRun, error) {
		if r, ok := runs[id]; ok {
			return r, nil
		}
		return nil, fmt.Errorf("run not found: %s", id)
	}

	engine := newTestEngine(ms, &mockEngineQueue{})
	engine.WithMaxNestingDepth(maxDepth)

	step := &domain.WorkflowStep{
		StepRef:       "sub",
		StepType:      domain.WorkflowStepTypeSubWorkflow,
		SubWorkflowID: "wf-child",
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "sub"}

	err := engine.startSubWorkflowStep(
		context.Background(), stepRun, step, runs[leafID], nil, time.Now(),
	)
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "nesting depth")
}

// TestWorkflowNesting_ZeroDepth verifies that WithMaxNestingDepth(0) is ignored
// and the engine falls back to DefaultMaxNestingDepth.
func TestWorkflowNesting_ZeroDepth(t *testing.T) {
	t.Parallel()

	engine := newTestEngine(&mockEngineStore{}, &mockEngineQueue{})
	engine.WithMaxNestingDepth(0)
	require.Equal(t, DefaultMaxNestingDepth,

		engine.
			maxNestingDepth,
	)
}

// TestWorkflowNesting_NegativeDepth verifies that WithMaxNestingDepth(-1) is
// ignored and the engine falls back to DefaultMaxNestingDepth.
func TestWorkflowNesting_NegativeDepth(t *testing.T) {
	t.Parallel()

	engine := newTestEngine(&mockEngineStore{}, &mockEngineQueue{})
	engine.WithMaxNestingDepth(-1)
	require.Equal(t, DefaultMaxNestingDepth,

		engine.
			maxNestingDepth,
	)
}

// TestWorkflowTrigger_HugePayload verifies that TriggerWorkflow accepts a
// large payload without panicking or silently truncating.
func TestWorkflowTrigger_HugePayload(t *testing.T) {
	t.Parallel()

	// 1 MB payload.
	bigValue := strings.Repeat("x", 1<<20)
	payload, err := json.Marshal(map[string]string{"data": bigValue})
	require.NoError(t,
		err)

	var capturedPayload json.RawMessage
	ms := minimalWorkflowStore("wf-1", "proj-1", singleJobStep())
	q := &mockEngineQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedPayload = run.Payload
			return nil
		},
	}

	engine := newTestEngine(ms, q)
	run, trigErr := engine.TriggerWorkflow(
		context.Background(), "wf-1", "proj-1", payload, "manual", nil, nil,
	)
	require.NoError(t, trigErr)
	require.NotNil(t,
		run)
	require.GreaterOrEqual(t, len(capturedPayload), 1<<20)
}

// TestWorkflowTrigger_NullPayload verifies that TriggerWorkflow handles a nil
// payload without errors.
func TestWorkflowTrigger_NullPayload(t *testing.T) {
	t.Parallel()

	ms := minimalWorkflowStore("wf-1", "proj-1", singleJobStep())
	engine := newTestEngine(ms, &mockEngineQueue{})

	run, err := engine.TriggerWorkflow(
		context.Background(), "wf-1", "proj-1", nil, "manual", nil, nil,
	)
	require.NoError(t,
		err)
	require.NotNil(t,
		run)
}

// TestWorkflowTrigger_EmptyStepOverrides verifies that passing an empty
// overrides slice does not alter step execution.
func TestWorkflowTrigger_EmptyStepOverrides(t *testing.T) {
	t.Parallel()

	steps := singleJobStep()
	ms := minimalWorkflowStore("wf-1", "proj-1", steps)
	enqueued := 0
	q := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued++
			return nil
		},
	}

	engine := newTestEngine(ms, q)
	run, err := engine.TriggerWorkflow(
		context.Background(), "wf-1", "proj-1",
		json.RawMessage(`{}`), "manual",
		[]domain.StepOverride{}, nil,
	)
	require.NoError(t,
		err)
	require.NotNil(t,
		run)
	require.Equal(t, 1,
		enqueued)
}

// TestWorkflowTrigger_InvalidStepOverride verifies that referencing a
// non-existent step_ref in overrides produces an error.
func TestWorkflowTrigger_InvalidStepOverride(t *testing.T) {
	t.Parallel()

	ms := minimalWorkflowStore("wf-1", "proj-1", singleJobStep())
	engine := newTestEngine(ms, &mockEngineQueue{})

	_, err := engine.TriggerWorkflow(
		context.Background(), "wf-1", "proj-1",
		json.RawMessage(`{}`), "manual",
		[]domain.StepOverride{
			{StepRef: "nonexistent", Enabled: false},
		},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "unknown step_ref",
	)
}

// TestStepExecution_UnknownStepType verifies that startStep for an unrecognized
// step type falls through to the job enqueue path rather than panicking.
func TestStepExecution_UnknownStepType(t *testing.T) {
	t.Parallel()

	enqueued := false
	ms := &mockEngineStore{}
	q := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}

	engine := newTestEngine(ms, q)

	step := &domain.WorkflowStep{
		ID:        "s1",
		StepRef:   "mystery",
		StepType:  domain.WorkflowStepType("totally_unknown"),
		JobID:     "j-1",
		OnFailure: domain.FailWorkflow,
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "mystery"}
	wfRun := &domain.WorkflowRun{
		ID:         "wr-1",
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
		Status:     domain.WfStatusRunning,
	}

	err := engine.startStep(context.Background(), stepRun, step, wfRun, nil)
	require.NoError(t,
		err)
	require.True(t, enqueued)
}

// TestStepExecution_CostGateZeroThreshold verifies that a cost gate with
// threshold=0 does not trigger the approval gate (the condition is > 0).
func TestStepExecution_CostGateZeroThreshold(t *testing.T) {
	t.Parallel()

	enqueued := false
	ms := &mockEngineStore{}
	q := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}

	engine := newTestEngine(ms, q)

	step := &domain.WorkflowStep{
		ID:                        "s1",
		StepRef:                   "deploy",
		StepType:                  domain.WorkflowStepTypeJob,
		JobID:                     "j-1",
		OnFailure:                 domain.FailWorkflow,
		CostGateThresholdMicrousd: 0, // Zero threshold: should not trigger gate.
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "deploy"}
	wfRun := &domain.WorkflowRun{
		ID:         "wr-1",
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
		Status:     domain.WfStatusRunning,
	}

	err := engine.startStep(context.Background(), stepRun, step, wfRun, nil)
	require.NoError(t,
		err)
	require.True(t, enqueued)
}

// TestStepExecution_CostGateNegativeThreshold verifies that a negative cost
// gate threshold does not trigger the approval gate.
func TestStepExecution_CostGateNegativeThreshold(t *testing.T) {
	t.Parallel()

	enqueued := false
	ms := &mockEngineStore{}
	q := &mockEngineQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}

	engine := newTestEngine(ms, q)

	step := &domain.WorkflowStep{
		ID:                        "s1",
		StepRef:                   "deploy",
		StepType:                  domain.WorkflowStepTypeJob,
		JobID:                     "j-1",
		OnFailure:                 domain.FailWorkflow,
		CostGateThresholdMicrousd: -100, // Negative threshold: should not trigger gate.
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1", StepRef: "deploy"}
	wfRun := &domain.WorkflowRun{
		ID:         "wr-1",
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
		Status:     domain.WfStatusRunning,
	}

	err := engine.startStep(context.Background(), stepRun, step, wfRun, nil)
	require.NoError(t,
		err)
	require.True(t, enqueued)
}

// TestSnapshotEnforcement_StaleVersion verifies that GetOrCreateWorkflowSnapshot
// is called during trigger and its result is stored in the workflow run.
func TestSnapshotEnforcement_StaleVersion(t *testing.T) {
	t.Parallel()

	snapshotCalled := false
	ms := minimalWorkflowStore("wf-1", "proj-1", singleJobStep())

	// Replace the default GetOrCreateWorkflowSnapshot with one that tracks calls.
	origStore := ms
	snapshotStore := &snapshotTrackingEngineStore{
		mockEngineStore: origStore,
		getOrCreateWorkflowSnapshotFn: func(_ context.Context, wf *domain.Workflow, _ []domain.WorkflowStep) (*domain.WorkflowSnapshot, error) {
			snapshotCalled = true
			return &domain.WorkflowSnapshot{
				ID:         "snap-v1",
				WorkflowID: wf.ID,
				Version:    wf.Version,
			}, nil
		},
	}

	engine := NewWorkflowEngine(snapshotStore, &mockEngineQueue{}, slog.Default())

	run, err := engine.TriggerWorkflow(
		context.Background(), "wf-1", "proj-1",
		json.RawMessage(`{}`), "manual", nil, nil,
	)
	require.NoError(t,
		err)
	require.True(t, snapshotCalled)
	require.Equal(t, "snap-v1",
		run.WorkflowSnapshotID,
	)
}

// TestSnapshotEnforcement_ConcurrentUpdate verifies that if
// GetOrCreateWorkflowSnapshot returns an error (simulating a concurrent
// modification), TriggerWorkflow propagates the error.
func TestSnapshotEnforcement_ConcurrentUpdate(t *testing.T) {
	t.Parallel()

	ms := minimalWorkflowStore("wf-1", "proj-1", singleJobStep())

	snapshotStore := &snapshotTrackingEngineStore{
		mockEngineStore: ms,
		getOrCreateWorkflowSnapshotFn: func(_ context.Context, _ *domain.Workflow, _ []domain.WorkflowStep) (*domain.WorkflowSnapshot, error) {
			return nil, fmt.Errorf("concurrent modification: version changed")
		},
	}

	engine := NewWorkflowEngine(snapshotStore, &mockEngineQueue{}, slog.Default())

	_, err := engine.TriggerWorkflow(
		context.Background(), "wf-1", "proj-1",
		json.RawMessage(`{}`), "manual", nil, nil,
	)
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "create workflow snapshot")
}

// FuzzWorkflowPayload verifies that TriggerWorkflow does not panic on
// arbitrary payload bytes.
func FuzzWorkflowPayload(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(`{"deeply":{"nested":{"key":"value"}}}`))
	f.Add([]byte{0x00, 0xff, 0xfe})
	f.Add([]byte(``))

	ms := minimalWorkflowStore("wf-1", "proj-1", singleJobStep())
	engine := newTestEngine(ms, &mockEngineQueue{})

	f.Fuzz(func(t *testing.T, payload []byte) {
		// We only care that TriggerWorkflow does not panic.
		// Errors are acceptable for malformed payloads.
		_, _ = engine.TriggerWorkflow(
			context.Background(), "wf-1", "proj-1",
			json.RawMessage(payload), "manual", nil, nil,
		)
	})
}

// FuzzStepOverrides verifies that TriggerWorkflow does not panic on arbitrary
// step override step_ref values.
func FuzzStepOverrides(f *testing.F) {
	f.Add("build", true)
	f.Add("", false)
	f.Add("nonexistent", false)
	f.Add(strings.Repeat("a", 1000), true)
	f.Add("step\x00with\x00nulls", false)

	ms := minimalWorkflowStore("wf-1", "proj-1", singleJobStep())
	engine := newTestEngine(ms, &mockEngineQueue{})

	f.Fuzz(func(t *testing.T, stepRef string, enabled bool) {
		// We only care that TriggerWorkflow does not panic.
		_, _ = engine.TriggerWorkflow(
			context.Background(), "wf-1", "proj-1",
			json.RawMessage(`{}`), "manual",
			[]domain.StepOverride{{StepRef: stepRef, Enabled: enabled}},
			nil,
		)
	})
}

// snapshotTrackingEngineStore wraps mockEngineStore to override
// GetOrCreateWorkflowSnapshot with custom behavior.
type snapshotTrackingEngineStore struct {
	*mockEngineStore
	getOrCreateWorkflowSnapshotFn func(ctx context.Context, wf *domain.Workflow, steps []domain.WorkflowStep) (*domain.WorkflowSnapshot, error)
}

// GetOrCreateWorkflowSnapshot delegates to the custom function if set.
func (s *snapshotTrackingEngineStore) GetOrCreateWorkflowSnapshot(ctx context.Context, wf *domain.Workflow, steps []domain.WorkflowStep) (*domain.WorkflowSnapshot, error) {
	if s.getOrCreateWorkflowSnapshotFn != nil {
		return s.getOrCreateWorkflowSnapshotFn(ctx, wf, steps)
	}
	return &domain.WorkflowSnapshot{ID: "snap-default"}, nil
}

// Ensure snapshotTrackingEngineStore satisfies EngineStore.
var _ EngineStore = (*snapshotTrackingEngineStore)(nil)
