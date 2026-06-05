package workflow

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadStepDefinitions_WithSnapshot(t *testing.T) {
	t.Parallel()

	snapshotSteps := []domain.WorkflowStep{
		{ID: "snap-s1", StepRef: "build", JobID: "j1", OnFailure: domain.FailWorkflow, StepType: domain.WorkflowStepTypeJob},
		{ID: "snap-s2", StepRef: "deploy", JobID: "j2", DependsOn: []string{"build"}, OnFailure: domain.Continue, StepType: domain.WorkflowStepTypeJob},
	}
	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{ID: "wf-1"},
		Steps:    snapshotSteps,
	}
	defJSON, _ := json.Marshal(def)

	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:                 id,
				WorkflowID:         "wf-1",
				WorkflowVersion:    1,
				WorkflowSnapshotID: "snap-123",
				Status:             domain.WfStatusRunning,
			}, nil
		},
	}

	// Override GetWorkflowSnapshot on the store mock to return our snapshot.
	origGetSnapshot := ms.GetWorkflowSnapshot
	_ = origGetSnapshot
	// We need a custom mock — extend the callback store.
	snapshotStore := &snapshotMockCallbackStore{
		mockCallbackStore: ms,
		getWorkflowSnapshotFn: func(_ context.Context, id string) (*domain.WorkflowSnapshot, error) {
			require.Equal(t, "snap-123",
				id)

			return &domain.WorkflowSnapshot{
				ID:         "snap-123",
				WorkflowID: "wf-1",
				Definition: defJSON,
			}, nil
		},
	}

	cb := NewStepCallback(snapshotStore, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	wfRun := &domain.WorkflowRun{
		ID:                 "wr-1",
		WorkflowID:         "wf-1",
		WorkflowVersion:    1,
		WorkflowSnapshotID: "snap-123",
	}

	steps, err := cb.loadStepDefinitions(context.Background(), wfRun)
	require.NoError(t,
		err)
	require.Len(t, steps,
		2)
	assert.False(t, steps[0].StepRef !=
		"build" ||
		steps[1].StepRef !=
			"deploy")

}

func TestLoadStepDefinitions_WithoutSnapshot_FallsBackToLiveTable(t *testing.T) {
	t.Parallel()

	liveSteps := []domain.WorkflowStep{
		{ID: "live-s1", StepRef: "test", JobID: "j1", OnFailure: domain.FailWorkflow, StepType: domain.WorkflowStepTypeJob},
	}

	ms := &mockCallbackStore{
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return liveSteps, nil
		},
	}

	cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	wfRun := &domain.WorkflowRun{
		ID:                 "wr-2",
		WorkflowID:         "wf-1",
		WorkflowVersion:    1,
		WorkflowSnapshotID: "", // No snapshot.
	}

	steps, err := cb.loadStepDefinitions(context.Background(), wfRun)
	require.NoError(t,
		err)
	require.Len(t, steps,
		1)
	assert.Equal(t, "test",
		steps[0].StepRef,
	)

}

func TestLoadStepDefinitions_SnapshotNotFound_FallsBackToLiveTable(t *testing.T) {
	t.Parallel()

	liveSteps := []domain.WorkflowStep{
		{ID: "live-s1", StepRef: "fallback", StepType: domain.WorkflowStepTypeJob},
	}

	snapshotStore := &snapshotMockCallbackStore{
		mockCallbackStore: &mockCallbackStore{
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return liveSteps, nil
			},
		},
		getWorkflowSnapshotFn: func(_ context.Context, _ string) (*domain.WorkflowSnapshot, error) {
			return nil, nil // Not found.
		},
	}

	cb := NewStepCallback(snapshotStore, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	wfRun := &domain.WorkflowRun{
		ID:                 "wr-3",
		WorkflowID:         "wf-1",
		WorkflowVersion:    1,
		WorkflowSnapshotID: "snap-missing",
	}

	steps, err := cb.loadStepDefinitions(context.Background(), wfRun)
	require.NoError(t,
		err)
	assert.False(t, len(steps) != 1 ||
		steps[0].StepRef !=
			"fallback",
	)

}

func TestLoadStepDefinitions_SnapshotPreservesAllFields(t *testing.T) {
	t.Parallel()

	step := domain.WorkflowStep{
		ID:                    "ws-1",
		WorkflowID:            "wf-1",
		JobID:                 "j-1",
		StepRef:               "step-a",
		DependsOn:             []string{"step-0"},
		Condition:             json.RawMessage(`{"op":"eq"}`),
		OnFailure:             domain.SkipDependents,
		Payload:               json.RawMessage(`{"x":1}`),
		StepType:              domain.WorkflowStepTypeJob,
		RetryMaxAttempts:      5,
		RetryBackoff:          domain.RetryBackoffExponential,
		RetryInitialDelaySecs: 10,
		RetryMaxDelaySecs:     300,
		TimeoutSecsOverride:   120,
		OutputTransform:       "$.data",
		ConcurrencyKey:        "ck",
		ResourceClass:         "large",
		EventKey:              "ek",
		EventEmitKey:          "eek",
	}

	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{ID: "wf-1"},
		Steps:    []domain.WorkflowStep{step},
	}
	defJSON, _ := json.Marshal(def)

	snapshotStore := &snapshotMockCallbackStore{
		mockCallbackStore: &mockCallbackStore{},
		getWorkflowSnapshotFn: func(_ context.Context, _ string) (*domain.WorkflowSnapshot, error) {
			return &domain.WorkflowSnapshot{ID: "snap-1", Definition: defJSON}, nil
		},
	}

	cb := NewStepCallback(snapshotStore, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	wfRun := &domain.WorkflowRun{
		ID:                 "wr-x",
		WorkflowID:         "wf-1",
		WorkflowSnapshotID: "snap-1",
	}

	steps, err := cb.loadStepDefinitions(context.Background(), wfRun)
	require.NoError(t,
		err)

	got := steps[0]
	assert.EqualValues(t, 5,
		got.RetryMaxAttempts,
	)
	assert.EqualValues(t, 120,
		got.TimeoutSecsOverride,
	)
	assert.Equal(t, `{"op":"eq"}`,
		string(got.Condition))
	assert.Equal(t, `{"x":1}`,
		string(got.
			Payload))
	assert.Equal(t, domain.
		SkipDependents,
		got.OnFailure,
	)
	assert.Equal(t, "$.data",
		got.OutputTransform,
	)
	assert.Equal(t, "ck",
		got.ConcurrencyKey,
	)

}

func TestLoadWfCtx_UsesSnapshotSteps(t *testing.T) {
	t.Parallel()

	snapshotSteps := []domain.WorkflowStep{
		{StepRef: "snap-step", OnFailure: domain.Continue, StepType: domain.WorkflowStepTypeJob},
	}
	def := domain.WorkflowSnapshotDefinition{Steps: snapshotSteps}
	defJSON, _ := json.Marshal(def)

	snapshotStore := &snapshotMockCallbackStore{
		mockCallbackStore: &mockCallbackStore{
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID:                 "wr-1",
					WorkflowID:         "wf-1",
					WorkflowSnapshotID: "snap-1",
					Status:             domain.WfStatusRunning,
				}, nil
			},
			// This should NOT be called when snapshot is available.
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				require.Fail(t,

					"ListStepsByWorkflowVersion should not be called when snapshot is available")
				return nil, nil
			},
		},
		getWorkflowSnapshotFn: func(_ context.Context, _ string) (*domain.WorkflowSnapshot, error) {
			return &domain.WorkflowSnapshot{ID: "snap-1", Definition: defJSON}, nil
		},
	}

	cb := NewStepCallback(snapshotStore, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
	wc, err := cb.loadWfCtx(context.Background(), "wr-1")
	require.NoError(t,
		err)
	assert.False(t, len(wc.steps) != 1 ||
		wc.steps[0].StepRef !=
			"snap-step")

	if _, ok := wc.stepByRef["snap-step"]; !ok {
		assert.Fail(t,

			"stepByRef missing snap-step")
	}
}

// snapshotMockCallbackStore wraps mockCallbackStore and overrides GetWorkflowSnapshot.
type snapshotMockCallbackStore struct {
	*mockCallbackStore
	getWorkflowSnapshotFn func(ctx context.Context, id string) (*domain.WorkflowSnapshot, error)
}

func (m *snapshotMockCallbackStore) GetWorkflowSnapshot(ctx context.Context, id string) (*domain.WorkflowSnapshot, error) {
	if m.getWorkflowSnapshotFn != nil {
		return m.getWorkflowSnapshotFn(ctx, id)
	}
	return nil, nil
}

// Ensure snapshotMockCallbackStore satisfies CallbackStore.
var _ CallbackStore = (*snapshotMockCallbackStore)(nil)

// Suppress unused import warnings.
var (
	_ = store.ErrRunNotFound
	_ = slog.Default
)
