package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func newTestCallback(ms *mockCallbackStore) *StepCallback {
	return NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
}

// testWfCtx builds a wfCtx from inline data for tests that call internal methods directly.
func testWfCtx(run *domain.WorkflowRun, steps []domain.WorkflowStep) *wfCtx {
	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	stepIndex := make(map[string]int, len(steps))
	for i, st := range steps {
		stepByRef[st.StepRef] = st
		stepIndex[st.StepRef] = i
	}
	return &wfCtx{run: run, steps: steps, stepByRef: stepByRef, stepIndex: stepIndex}
}

func TestHandleFailedStep_SkipDependentsPolicy(t *testing.T) {
	t.Parallel()
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
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a", OnFailure: domain.SkipDependents},
			{StepRef: "b", DependsOn: []string{"a"}},
			{StepRef: "c"},
		},
	)
	require.NoError(t,
		cb.handleFailedStep(context.
			Background(), stepRun,
			wc))
	require.True(t, skippedIDs["sr-b"])
	require.False(t, skippedIDs["sr-c"])
}

func TestHandleFailedStep_ContinuePolicy(t *testing.T) {
	t.Parallel()
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
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a", OnFailure: domain.Continue},
		},
	)
	require.NoError(t,
		cb.handleFailedStep(context.
			Background(), stepRun,
			wc))
	require.True(t, workflowChecked)
}

func TestHandleFailedStep_DefaultPolicy(t *testing.T) {
	t.Parallel()
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
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	stepRun := &domain.WorkflowStepRun{ID: "sr-a", WorkflowRunID: "wr-1", StepRef: "a", Status: domain.StepFailed, Error: "boom"}
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "a"},
		},
	)
	require.NoError(t,
		cb.handleFailedStep(context.
			Background(), stepRun,
			wc))
	require.True(t, workflowFailed)
}

func TestCancelRemainingSteps(t *testing.T) {
	t.Parallel()
	canceledIDs := make(map[string]bool)
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	require.NoError(t,
		cb.cancelRemainingSteps(context.
			Background(),
			"wr-1"))
	require.False(t, !canceledIDs["sr-2"] || !canceledIDs["sr-3"])
	require.False(t, canceledIDs["sr-1"] || canceledIDs["sr-4"])
}

func TestCheckWorkflowCompletion_AllCompleted(t *testing.T) {
	t.Parallel()
	wfStatus := domain.WfStatusRunning
	var hookRunID string
	var hookFrom domain.WorkflowRunStatus
	var hookTo domain.WorkflowRunStatus
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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

	cb := newTestCallback(ms).WithStatusHook(func(_ context.Context, run *domain.WorkflowRun, from, to domain.WorkflowRunStatus, _ string) {
		hookRunID = run.ID
		hookFrom = from
		hookTo = to
	})
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{{StepRef: "s1"}, {StepRef: "s2"}},
	)
	require.NoError(t,
		cb.checkWorkflowCompletion(context.
			Background(), "wr-1",
			wc))
	require.Equal(t, domain.
		WfStatusCompleted,
		wfStatus,
	)
	require.False(t, hookRunID !=
		"wr-1" ||
		hookFrom !=
			domain.WfStatusRunning ||
		hookTo !=
			domain.
				WfStatusCompleted)
}

func TestCheckWorkflowCompletion_HasNonTerminal(t *testing.T) {
	t.Parallel()
	wfUpdated := false
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{{StepRef: "s1"}, {StepRef: "s2"}},
	)
	require.NoError(t,
		cb.checkWorkflowCompletion(context.
			Background(), "wr-1",
			wc))
	require.False(t, wfUpdated)
}

func TestCheckWorkflowCompletion_FailedWithContinuePolicy(t *testing.T) {
	t.Parallel()
	wfStatus := domain.WfStatusRunning
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "s1", OnFailure: domain.Continue},
			{StepRef: "s2"},
		},
	)
	require.NoError(t,
		cb.checkWorkflowCompletion(context.
			Background(), "wr-1",
			wc))
	require.Equal(t, domain.
		WfStatusCompleted,
		wfStatus,
	)
}

func TestCheckWorkflowCompletion_FailedWithoutContinue(t *testing.T) {
	t.Parallel()
	wfStatus := domain.WfStatusRunning
	ms := &mockCallbackStore{
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
		[]domain.WorkflowStep{
			{StepRef: "s1", OnFailure: domain.FailWorkflow},
			{StepRef: "s2"},
		},
	)
	require.NoError(t,
		cb.checkWorkflowCompletion(context.
			Background(), "wr-1",
			wc))
	require.Equal(t, domain.
		WfStatusFailed,
		wfStatus,
	)
}

func TestHasBlockingFailedStep(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "continue", OnFailure: domain.Continue},
		{StepRef: "continue-too", OnFailure: domain.Continue},
		{StepRef: "fail", OnFailure: domain.FailWorkflow},
		{StepRef: "default"},
	}
	tests := []struct {
		name           string
		failedStepRefs []string
		want           bool
	}{
		{name: "no failed refs", failedStepRefs: nil, want: false},
		{name: "single continue failure", failedStepRefs: []string{"continue"}, want: false},
		{name: "single explicit failure", failedStepRefs: []string{"fail"}, want: true},
		{name: "single default failure", failedStepRefs: []string{"default"}, want: true},
		{name: "unknown failure blocks", failedStepRefs: []string{"missing"}, want: true},
		{name: "mixed continue and failure blocks", failedStepRefs: []string{"continue", "fail"}, want: true},
		{name: "all continue failures do not block", failedStepRefs: []string{"continue", "continue-too"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasBlockingFailedStep(steps, tt.failedStepRefs)
			require.Equal(t, tt.
				want, got)
		})
	}
}

func BenchmarkHasBlockingFailedStep(b *testing.B) {
	steps := make([]domain.WorkflowStep, 100)
	for i := range steps {
		steps[i] = domain.WorkflowStep{
			StepRef:   fmt.Sprintf("step-%03d", i),
			OnFailure: domain.FailWorkflow,
		}
	}
	steps[90].OnFailure = domain.Continue

	b.Run("no_failed_refs", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if hasBlockingFailedStep(steps, nil) {
				b.Fatal("expected no blocking failure")
			}
		}
	})
	b.Run("single_continue_failure", func(b *testing.B) {
		failedRefs := []string{"step-090"}
		b.ReportAllocs()
		for b.Loop() {
			if hasBlockingFailedStep(steps, failedRefs) {
				b.Fatal("expected continue failure to be ignored")
			}
		}
	})
	b.Run("single_blocking_failure", func(b *testing.B) {
		failedRefs := []string{"step-099"}
		b.ReportAllocs()
		for b.Loop() {
			if !hasBlockingFailedStep(steps, failedRefs) {
				b.Fatal("expected blocking failure")
			}
		}
	})
	b.Run("multiple_continue_failures", func(b *testing.B) {
		for i := range steps {
			steps[i].OnFailure = domain.Continue
		}
		failedRefs := []string{"step-010", "step-050", "step-090"}
		b.ReportAllocs()
		for b.Loop() {
			if hasBlockingFailedStep(steps, failedRefs) {
				b.Fatal("expected no blocking failure")
			}
		}
	})
}

func BenchmarkAggregateChildStepOutputs(b *testing.B) {
	stepRuns := make([]domain.WorkflowStepRun, 1000)
	for i := range stepRuns {
		ref := fmt.Sprintf("step-%04d", i)
		stepRuns[i] = domain.WorkflowStepRun{
			ID:      "sr-" + ref,
			StepRef: ref,
		}
		if i%2 == 0 {
			stepRuns[i].Output = json.RawMessage(`{"ok":true}`)
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		output := aggregateChildStepOutputs(stepRuns)
		if len(output) == 0 {
			b.Fatal("expected aggregated output")
		}
	}
}

func TestAggregateChildStepOutputs(t *testing.T) {
	t.Parallel()
	output := aggregateChildStepOutputs([]domain.WorkflowStepRun{
		{StepRef: `step-"a"`, Output: json.RawMessage(`{"a":1}`)},
		{StepRef: "empty"},
		{StepRef: "step-b", Output: json.RawMessage(`{"b":2}`)},
	})
	require.NotEmpty(t,
		output)

	var parsed map[string]json.RawMessage
	require.NoError(t,
		json.Unmarshal(output,
			&parsed,
		))
	require.Equal(t, `{"a":1}`,
		string(
			parsed[`step-"a"`]))

	if _, ok := parsed["empty"]; ok {
		require.Fail(t,

			"empty output should not be included")
	}
	require.Equal(t, `{"b":2}`,
		string(
			parsed["step-b"]),
	)
}

func TestAggregateChildStepOutputs_NoOutputs(t *testing.T) {
	t.Parallel()
	output := aggregateChildStepOutputs([]domain.WorkflowStepRun{
		{StepRef: "a"},
		{StepRef: "b"},
	})
	require.Nil(t, output)
}

func TestSkipDependentSteps_TransitiveSkip(t *testing.T) {
	t.Parallel()
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
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
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
	wc := testWfCtx(
		&domain.WorkflowRun{WorkflowVersion: 1},
		[]domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b", DependsOn: []string{"a"}},
			{StepRef: "c", DependsOn: []string{"b"}},
			{StepRef: "d"},
		},
	)
	require.NoError(t,
		cb.skipDependentSteps(context.
			Background(), "wr-1",
			wc,
			"a"))
	require.True(t, skippedIDs["sr-b"])
	require.True(t, skippedIDs["sr-c"])
	require.False(t, skippedIDs["sr-d"])
}

func TestDependentStepRefs_OrderedChain(t *testing.T) {
	t.Parallel()
	steps := benchmarkSkipDependentChain(1000)

	got := dependentStepRefs(steps, nil, "step-0500")
	require.Len(t, got,
		499)
	require.Equal(t, "step-0501",
		got[0])
	require.Equal(t, "step-0999",
		got[len(got)-1],
	)
}

func TestDependentStepRefs_RootFanOut(t *testing.T) {
	t.Parallel()
	steps := benchmarkSkipDependentFanOut(1000)

	got := dependentStepRefs(steps, nil, "root")
	require.Len(t, got,
		999)
	require.Equal(t, "step-0001",
		got[0])
	require.Equal(t, "step-0999",
		got[len(got)-1],
	)
}

func TestDependentStepRefs_UnorderedDAGFallsBack(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "child", DependsOn: []string{"root"}},
		{StepRef: "root"},
	}

	got := dependentStepRefs(steps, nil, "root")
	want := []string{"child"}
	require.True(t, slices.
		Equal(got, want))
}

func BenchmarkSkipDependentSteps(b *testing.B) {
	benchmarks := []struct {
		name          string
		steps         []domain.WorkflowStep
		failedStepRef string
		wantSkipped   int
	}{
		{
			name:          "chain100",
			steps:         benchmarkSkipDependentChain(100),
			failedStepRef: "step-0000",
			wantSkipped:   99,
		},
		{
			name:          "chain1000",
			steps:         benchmarkSkipDependentChain(1000),
			failedStepRef: "step-0000",
			wantSkipped:   999,
		},
		{
			name:          "fanout1000",
			steps:         benchmarkSkipDependentFanOut(1000),
			failedStepRef: "root",
			wantSkipped:   999,
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			ms := &mockCallbackStore{
				skipStepRunsByRefsFn: func(_ context.Context, _ string, refs []string, _ time.Time) (int64, error) {
					if len(refs) != bm.wantSkipped {
						b.Fatalf("skipped refs = %d, want %d", len(refs), bm.wantSkipped)
					}
					return int64(len(refs)), nil
				},
			}
			cb := newTestCallback(ms)
			wc := testWfCtx(
				&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning},
				bm.steps,
			)
			ctx := context.Background()

			b.ReportAllocs()
			for b.Loop() {
				if err := cb.skipDependentSteps(ctx, "wr-1", wc, bm.failedStepRef); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkSkipDependentChain(n int) []domain.WorkflowStep {
	steps := make([]domain.WorkflowStep, n)
	for i := range steps {
		ref := fmt.Sprintf("step-%04d", i)
		steps[i] = domain.WorkflowStep{StepRef: ref}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	return steps
}

func benchmarkSkipDependentFanOut(n int) []domain.WorkflowStep {
	steps := make([]domain.WorkflowStep, n)
	steps[0] = domain.WorkflowStep{StepRef: "root"}
	for i := 1; i < n; i++ {
		steps[i] = domain.WorkflowStep{
			StepRef:   fmt.Sprintf("step-%04d", i),
			DependsOn: []string{"root"},
		}
	}
	return steps
}

func TestEmitEventIfConfigured_ResolvesWaitingTrigger(t *testing.T) {
	t.Parallel()

	var resolvedTriggerID string
	var resolvedPayload json.RawMessage
	var targetStepCompleted bool

	ms := &mockCallbackStore{
		getWorkflowRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			if id == "wr-1" {
				return &domain.WorkflowRun{
					ID:              "wr-1",
					WorkflowID:      "wf-1",
					ProjectID:       "proj-1",
					WorkflowVersion: 1,
					Status:          domain.WfStatusRunning,
					Payload:         json.RawMessage(`{"env":"prod"}`),
				}, nil
			}
			return nil, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "emitter", EventEmitKey: "chain:{{env}}:done"},
				{StepRef: "waiter", StepType: domain.WorkflowStepTypeWaitForEvent, EventKey: "chain:prod:done"},
			}, nil
		},
		getEventTriggerByEventKeyFn: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			if key == "chain:prod:done" {
				return &domain.EventTrigger{
					ID:                "evt-waiter",
					EventKey:          "chain:prod:done",
					SourceType:        domain.EventSourceWorkflowStep,
					WorkflowRunID:     "wr-1",
					WorkflowStepRunID: "sr-waiter",
					Status:            domain.EventTriggerStatusWaiting,
					ProjectID:         "proj-1",
				}, nil
			}
			return nil, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, id string, status string, payload json.RawMessage, _ *time.Time, _ string) error {
			if status == domain.EventTriggerStatusReceived {
				resolvedTriggerID = id
				resolvedPayload = payload
			}
			return nil
		},
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			if id == "sr-waiter" {
				return &domain.WorkflowStepRun{ID: "sr-waiter", StepRef: "waiter", WorkflowRunID: "wr-1", Status: domain.StepWaiting}, nil
			}
			return nil, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-waiter", StepRef: "waiter", WorkflowRunID: "wr-1", Status: domain.StepWaiting},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id == "sr-waiter" && status == domain.StepCompleted {
				targetStepCompleted = true
			}
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	emitterStepRun := &domain.WorkflowStepRun{
		ID:            "sr-emitter",
		StepRef:       "emitter",
		WorkflowRunID: "wr-1",
		Status:        domain.StepCompleted,
		Output:        json.RawMessage(`{"data":"result"}`),
	}

	// Call tryEmitEvent which should resolve the waiting trigger AND resume the step.
	wc := testWfCtx(
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", WorkflowVersion: 1, Status: domain.WfStatusRunning, Payload: json.RawMessage(`{"env":"prod"}`)},
		[]domain.WorkflowStep{
			{StepRef: "emitter", EventEmitKey: "chain:{{env}}:done"},
			{StepRef: "waiter", StepType: domain.WorkflowStepTypeWaitForEvent, EventKey: "chain:prod:done"},
		},
	)
	cb.tryEmitEvent(context.Background(), emitterStepRun, wc)
	require.Equal(t, "evt-waiter",
		resolvedTriggerID,
	)
	require.JSONEq(t, `{"data":"result"}`,

		string(resolvedPayload))
	require.True(t, targetStepCompleted)
}

func TestOnJobRunTerminal_UpdateStepStatusError(t *testing.T) {
	t.Parallel()
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: "wr-1", StepRef: "s1", Status: domain.StepRunning}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			return errors.New("store error")
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1"}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "s1"}}, nil
		},
	}

	cb := newTestCallback(ms)
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-1", WorkflowStepRunID: "sr-1", Status: domain.StatusCompleted})
	require.Error(t, err)
}

func TestOnStepCompleted_AdvancesWorkflow(t *testing.T) {
	t.Parallel()

	var incrementedRef string

	ms := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			if id == "sr-1" {
				return &domain.WorkflowStepRun{ID: "sr-1", StepRef: "step-a", WorkflowRunID: "wr-1", Status: domain.StepCompleted}, nil
			}
			return nil, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "step-a", WorkflowRunID: "wr-1", Status: domain.StepCompleted},
				{ID: "sr-2", StepRef: "step-b", WorkflowRunID: "wr-1", Status: domain.StepWaiting},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "step-a"},
				{StepRef: "step-b", DependsOn: []string{"step-a"}},
			}, nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, completedRef string) ([]store.StepDepResult, error) {
			incrementedRef = completedRef
			return []store.StepDepResult{}, nil
		},
	}

	cb := newTestCallback(ms)
	cb.OnStepCompleted(context.Background(), "wr-1", "sr-1")
	require.Equal(t, "step-a",
		incrementedRef,
	)
}

func TestFanInAndStartReadyChildren_AcquiresAdvisoryXactLock(t *testing.T) {
	t.Parallel()

	var lockCalled bool

	ms := &mockCallbackStore{
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return []store.StepDepResult{{
				StepRunID:     "sr-child",
				StepRef:       "step-b",
				DepsCompleted: 1,
				DepsRequired:  1,
			}}, nil
		},
		advisoryXactLockFn: func(_ context.Context, lockID int64) error {
			lockCalled = lockID != 0
			return nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1, Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "step-b"}}, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{{ID: "sr-child", StepRef: "step-b", Status: domain.StepCompleted}}, nil
		},
	}

	cb := newTestCallback(ms)
	err := cb.fanInAndStartReadyChildren(context.Background(), &domain.WorkflowStepRun{WorkflowRunID: "wr-1", StepRef: "step-a"}, &wfCtx{
		run:   &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusRunning},
		steps: []domain.WorkflowStep{{StepRef: "step-a"}, {StepRef: "step-b"}},
	})
	require.NoError(t,
		err)
	require.True(t, lockCalled)
}

func TestOnStepCompleted_StepNotFound(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-other", StepRef: "step-b", WorkflowRunID: "wr-1", Status: domain.StepCompleted},
			}, nil
		},
	}

	cb := newTestCallback(ms)
	// Should return cleanly without panic when step ID doesn't match.
	cb.OnStepCompleted(context.Background(), "wr-1", "sr-nonexistent")
}

func TestOnStepFailed_RespectsOnFailureContinue(t *testing.T) {
	t.Parallel()

	var workflowFailed bool

	ms := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			if id == "sr-1" {
				return &domain.WorkflowStepRun{ID: "sr-1", StepRef: "step-a", WorkflowRunID: "wr-1", Status: domain.StepFailed}, nil
			}
			return nil, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-1", StepRef: "step-a", WorkflowRunID: "wr-1", Status: domain.StepFailed},
				{ID: "sr-2", StepRef: "step-b", WorkflowRunID: "wr-1", Status: domain.StepCompleted},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "step-a", OnFailure: domain.Continue},
				{StepRef: "step-b"},
			}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, to domain.WorkflowRunStatus, _ map[string]any) error {
			if to == domain.WfStatusFailed {
				workflowFailed = true
			}
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return []store.StepDepResult{}, nil
		},
	}

	cb := newTestCallback(ms)
	cb.OnStepFailed(context.Background(), "wr-1", "sr-1")
	require.False(t, workflowFailed)
}

func TestOnStepFailed_StepNotFound(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listStepRunsByWorkflowRun: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-other", StepRef: "step-b", WorkflowRunID: "wr-1", Status: domain.StepRunning},
			}, nil
		},
	}

	cb := newTestCallback(ms)
	// Should return cleanly without panic when step ID doesn't match.
	cb.OnStepFailed(context.Background(), "wr-1", "sr-nonexistent")
}

func TestOnJobRunTerminal_ReleasesWaitingDependencyRuns(t *testing.T) {
	t.Parallel()

	queuedRunID := ""
	ms := &mockCallbackStore{
		listDependentsByDependencyJobFn: func(_ context.Context, dependsOnJobID string) ([]domain.JobDependency, error) {
			require.Equal(t, "job-upstream",
				dependsOnJobID,
			)

			return []domain.JobDependency{{JobID: "job-downstream", DependsOnJobID: dependsOnJobID, Condition: "completed"}}, nil
		},
		listWaitingRunsByJobIDsFn: func(_ context.Context, jobIDs []string, _ int) ([]domain.JobRun, error) {
			require.False(t, len(jobIDs) != 1 ||
				jobIDs[0] != "job-downstream",
			)

			return []domain.JobRun{{ID: "run-waiting", JobID: "job-downstream", Status: domain.StatusWaiting}}, nil
		},
		areJobDependenciesSatisfiedFn: func(_ context.Context, run *domain.JobRun) (bool, error) {
			require.Equal(t, "run-waiting",
				run.
					ID)

			return true, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			require.False(t, from !=
				domain.StatusWaiting ||
				to !=
					domain.StatusQueued,
			)

			queuedRunID = id
			return nil
		},
	}

	cb := newTestCallback(ms)
	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{ID: "run-upstream", JobID: "job-upstream", Status: domain.StatusCompleted})
	require.NoError(t,
		err)
	require.Equal(t, "run-waiting",
		queuedRunID,
	)
}

// emitEventIfConfigured: nil step is a no-op.
func TestEmitEventIfConfigured_NilStep(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)

	// Should not panic.
	cb.emitEventIfConfigured(context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1"},
		nil,
		&domain.WorkflowRun{ID: "wr-1"},
	)
}

// emitEventIfConfigured: empty emit key is a no-op.
func TestEmitEventIfConfigured_EmptyEmitKey(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{}
	cb := newTestCallback(ms)

	cb.emitEventIfConfigured(context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1"},
		&domain.WorkflowStep{StepRef: "step-1", EventEmitKey: ""},
		&domain.WorkflowRun{ID: "wr-1"},
	)
}

// emitEventIfConfigured: trigger not found is a no-op.
func TestEmitEventIfConfigured_TriggerNotFound(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, nil
		},
	}
	cb := newTestCallback(ms)

	cb.emitEventIfConfigured(context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1"},
		&domain.WorkflowStep{StepRef: "step-1", EventEmitKey: "emit:test"},
		&domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"},
	)
}

// emitEventIfConfigured: trigger already received is a no-op.
func TestEmitEventIfConfigured_TriggerAlreadyReceived(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:       "evt-1",
				EventKey: "emit:test",
				Status:   domain.EventTriggerStatusReceived,
			}, nil
		},
	}
	cb := newTestCallback(ms)

	cb.emitEventIfConfigured(context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1"},
		&domain.WorkflowStep{StepRef: "step-1", EventEmitKey: "emit:test"},
		&domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"},
	)
}

// emitEventIfConfigured: store error on get trigger logs and returns.
func TestEmitEventIfConfigured_GetTriggerError(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return nil, errors.New("db error")
		},
	}
	cb := newTestCallback(ms)

	cb.emitEventIfConfigured(context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1"},
		&domain.WorkflowStep{StepRef: "step-1", EventEmitKey: "emit:test"},
		&domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"},
	)
}

// emitEventIfConfigured: store error on update trigger logs and returns.
func TestEmitEventIfConfigured_UpdateTriggerError(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-1",
				EventKey:   "emit:test",
				Status:     domain.EventTriggerStatusWaiting,
				SourceType: domain.EventSourceWorkflowStep,
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return errors.New("update failed")
		},
	}
	cb := newTestCallback(ms)

	cb.emitEventIfConfigured(context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1", Output: json.RawMessage(`{"ok":true}`)},
		&domain.WorkflowStep{StepRef: "step-1", EventEmitKey: "emit:test"},
		&domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"},
	)
}

// emitEventIfConfigured: job run source re-queues the run.
func TestEmitEventIfConfigured_JobRunSource(t *testing.T) {
	t.Parallel()

	var requeuedRunID string
	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-jr",
				EventKey:   "emit:job",
				Status:     domain.EventTriggerStatusWaiting,
				SourceType: domain.EventSourceJobRun,
				JobRunID:   "run-99",
			}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, _ domain.RunStatus, _ map[string]any) error {
			requeuedRunID = id
			return nil
		},
	}
	cb := newTestCallback(ms)

	cb.emitEventIfConfigured(context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1", Output: json.RawMessage(`{"result":"done"}`)},
		&domain.WorkflowStep{StepRef: "step-1", EventEmitKey: "emit:job"},
		&domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1"},
	)
	require.Equal(t, "run-99",
		requeuedRunID,
	)
}
