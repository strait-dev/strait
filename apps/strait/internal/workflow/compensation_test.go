package workflow

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for BuildCompensationPlan.

func TestCompensation_ReverseOrder(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
		{StepRef: "b", DependsOn: []string{"a"}, CompensationJobID: "comp-b"},
		{StepRef: "c", DependsOn: []string{"b"}, CompensationJobID: "comp-c"},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"a":1}`)},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted, Output: json.RawMessage(`{"b":2}`)},
		{ID: "sr-c", StepRef: "c", Status: domain.StepFailed},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Steps,
		2)
	assert.Equal(t, "b", plan.
		Steps[0].StepRef)
	assert.Equal(t, "a", plan.
		Steps[1].StepRef)

	// b should be compensated before a (reverse order).

}

func TestCompensation_OnlyCompletedSteps(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
		{StepRef: "b", DependsOn: []string{"a"}, CompensationJobID: "comp-b"},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepFailed},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Steps,
		1)
	assert.Equal(t, "a", plan.
		Steps[0].StepRef)

}

func BenchmarkBuildCompensationPlan_Chain100(b *testing.B) {
	steps := make([]domain.WorkflowStep, 100)
	stepRuns := make([]domain.WorkflowStepRun, 100)
	for i := range steps {
		ref := fmt.Sprintf("step-%03d", i)
		steps[i] = domain.WorkflowStep{
			StepRef:           ref,
			CompensationJobID: "comp-" + ref,
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
		stepRuns[i] = domain.WorkflowStepRun{
			ID:      "sr-" + ref,
			StepRef: ref,
			Status:  domain.StepCompleted,
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		plan, err := BuildCompensationPlan("wfr-bench", steps, stepRuns)
		if err != nil {
			b.Fatal(err)
		}
		if len(plan.Steps) != len(steps) {
			b.Fatalf("plan length = %d, want %d", len(plan.Steps), len(steps))
		}
	}
}

func BenchmarkBuildCompensationPlan_Chain1000(b *testing.B) {
	steps := make([]domain.WorkflowStep, 1000)
	stepRuns := make([]domain.WorkflowStepRun, 1000)
	for i := range steps {
		ref := fmt.Sprintf("step-%04d", i)
		steps[i] = domain.WorkflowStep{
			StepRef:           ref,
			CompensationJobID: "comp-" + ref,
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
		stepRuns[i] = domain.WorkflowStepRun{
			ID:      "sr-" + ref,
			StepRef: ref,
			Status:  domain.StepCompleted,
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		plan, err := BuildCompensationPlan("wfr-bench", steps, stepRuns)
		if err != nil {
			b.Fatal(err)
		}
		if len(plan.Steps) != len(steps) {
			b.Fatalf("plan length = %d, want %d", len(plan.Steps), len(steps))
		}
	}
}

func TestCompensation_SkipsStepsWithoutCompensation(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
		{StepRef: "b", DependsOn: []string{"a"}}, // no compensation
		{StepRef: "c", DependsOn: []string{"b"}, CompensationJobID: "comp-c"},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted},
		{ID: "sr-c", StepRef: "c", Status: domain.StepFailed},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Steps,
		1)
	assert.Equal(t, "a", plan.
		Steps[0].StepRef)

	// Only a has compensation (b has none, c failed).

}

func TestCompensation_PassesOriginalOutput(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
	}
	output := json.RawMessage(`{"charge_id":"ch_123","amount":4999}`)
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, Output: output},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, string(output),
		string(plan.
			Steps[0].
			OriginalOutput,
		))

}

func TestCompensation_DiamondDAG(t *testing.T) {
	t.Parallel()
	// A->(B,C)->D. D fails, compensate C, B, A in reverse order.
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
		{StepRef: "b", DependsOn: []string{"a"}, CompensationJobID: "comp-b"},
		{StepRef: "c", DependsOn: []string{"a"}, CompensationJobID: "comp-c"},
		{StepRef: "d", DependsOn: []string{"b", "c"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted},
		{ID: "sr-c", StepRef: "c", Status: domain.StepCompleted},
		{ID: "sr-d", StepRef: "d", Status: domain.StepFailed},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Steps,
		3)

	// b and c should come before a. Since b and c are at same topo level,
	// order between them is deterministic (alphabetical for same-level).
	refs := make([]string, len(plan.Steps))
	for i, s := range plan.Steps {
		refs[i] = s.StepRef
	}
	assert.Equal(t, "a", refs[len(refs)-1])

	// Last should be a (root).

}

func TestCompensation_UnorderedDefinitionsUseTopologicalFallback(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{StepRef: "c", DependsOn: []string{"b"}, CompensationJobID: "comp-c"},
		{StepRef: "a", CompensationJobID: "comp-a"},
		{StepRef: "b", DependsOn: []string{"a"}, CompensationJobID: "comp-b"},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted},
		{ID: "sr-c", StepRef: "c", Status: domain.StepCompleted},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)

	refs := make([]string, len(plan.Steps))
	for i, step := range plan.Steps {
		refs[i] = step.StepRef
	}
	want := []string{"c", "b", "a"}
	for i := range want {
		require.Equal(t, want[i],
			refs[i])

	}
}

func TestCompensation_NoCompensationNeeded(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a"}, // no compensation_job_id
		{StepRef: "b", DependsOn: []string{"a"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepFailed},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	assert.Nil(t, plan)

}

func TestCompensation_EmptySteps(t *testing.T) {
	t.Parallel()
	plan, err := BuildCompensationPlan("wfr-1", nil, nil)
	require.NoError(t, err)
	assert.Nil(t, plan)

}

func TestCompensation_TimeoutPropagated(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a", CompensationTimeoutSecs: 60},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	assert.EqualValues(t, 60, plan.
		Steps[0].TimeoutSecs,
	)

}

// FSM transition tests.

func TestFSM_FailedToCompensating(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusFailed, domain.WfStatusCompensating)
	assert.NoError(t, err)

}

func TestFSM_CompensatingToCompensated(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusCompensating, domain.WfStatusCompensated)
	assert.NoError(t, err)

}

func TestFSM_CompensatingToCompensationFailed(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusCompensating, domain.WfStatusCompensationFailed)
	assert.NoError(t, err)

}

func TestFSM_CompensatingToCompleted(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusCompensating, domain.WfStatusCompleted)
	assert.Error(t, err)

}

func TestFSM_CompletedToCompensating(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusCompleted, domain.WfStatusCompensating)
	assert.Error(t, err)

}

func TestFSM_RunningToCompensated(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusRunning, domain.WfStatusCompensated)
	assert.Error(t, err)

}

func TestFSM_TimedOutToCompensating(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusTimedOut, domain.WfStatusCompensating)
	assert.NoError(t, err)

}

func TestFSM_CompensatingToCanceled(t *testing.T) {
	t.Parallel()
	err := domain.ValidateWorkflowTransition(domain.WfStatusCompensating, domain.WfStatusCanceled)
	assert.NoError(t, err)

}

func TestFSM_CompensatedIsTerminal(t *testing.T) {
	t.Parallel()
	assert.True(t, domain.WfStatusCompensated.
		IsTerminal())

}

func TestFSM_CompensationFailedIsTerminal(t *testing.T) {
	t.Parallel()
	assert.True(t, domain.WfStatusCompensationFailed.
		IsTerminal())

}

// ValidateCompensationRequest tests.

func TestValidateCompensation_FailedRun(t *testing.T) {
	t.Parallel()
	run := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusFailed}
	assert.NoError(t, ValidateCompensationRequest(run))

}

func TestValidateCompensation_TimedOutRun(t *testing.T) {
	t.Parallel()
	run := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusTimedOut}
	assert.NoError(t, ValidateCompensationRequest(run))

}

func TestValidateCompensation_RunningRun(t *testing.T) {
	t.Parallel()
	run := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusRunning}
	err := ValidateCompensationRequest(run)
	assert.Error(t, err)

}

func TestValidateCompensation_AlreadyCompensating(t *testing.T) {
	t.Parallel()
	run := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusCompensating}
	err := ValidateCompensationRequest(run)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(),
		"already compensating",
	))

}

func TestValidateCompensation_NilRun(t *testing.T) {
	t.Parallel()
	err := ValidateCompensationRequest(nil)
	assert.Error(t, err)

}

// Fuzz tests.

func FuzzCompensation_ReverseTopologicalOrder(f *testing.F) {
	f.Add(uint8(3), "a,b,c", ",a,b")
	f.Add(uint8(5), "a,b,c,d,e", ",a,a,b,c")
	f.Add(uint8(1), "x", "")

	f.Fuzz(func(t *testing.T, numSteps uint8, refsCSV, depsCSV string) {
		if numSteps == 0 || numSteps > 20 {
			return
		}
		refs := strings.Split(refsCSV, ",")
		deps := strings.Split(depsCSV, ",")
		if len(refs) < int(numSteps) {
			return
		}

		steps := make([]domain.WorkflowStep, numSteps)
		stepRuns := make([]domain.WorkflowStepRun, numSteps)
		for i := range steps {
			steps[i].StepRef = refs[i]
			steps[i].CompensationJobID = "comp-" + refs[i]
			if i < len(deps) && deps[i] != "" {
				steps[i].DependsOn = []string{deps[i]}
			}
			stepRuns[i] = domain.WorkflowStepRun{
				ID:      "sr-" + refs[i],
				StepRef: refs[i],
				Status:  domain.StepCompleted,
			}
		}

		// Must never panic.
		_, _ = BuildCompensationPlan("wfr-fuzz", steps, stepRuns)
	})
}

func FuzzCompensation_StatusTransitions(f *testing.F) {
	f.Add("failed", "compensating")
	f.Add("compensating", "compensated")
	f.Add("compensating", "compensation_failed")
	f.Add("running", "compensated")
	f.Add("completed", "compensating")

	f.Fuzz(func(t *testing.T, from, to string) {
		// Must never panic.
		_ = domain.ValidateWorkflowTransition(
			domain.WorkflowRunStatus(from),
			domain.WorkflowRunStatus(to),
		)
	})
}

// Adversarial tests.

func TestCompensation_NilStepOutput(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, Output: nil},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Nil(t, plan.
		Steps[0].OriginalOutput,
	)

}

func TestCompensation_HugeOutput(t *testing.T) {
	t.Parallel()
	largeOutput, _ := json.Marshal(map[string]string{
		"data": strings.Repeat("x", 5*1024*1024),
	})
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, Output: largeOutput},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.GreaterOrEqual(t,
		len(plan.
			Steps[0].OriginalOutput,
		), 5*
			1024*1024,
	)

}

func TestCompensation_ManySteps(t *testing.T) {
	t.Parallel()
	n := 100
	steps := make([]domain.WorkflowStep, n)
	stepRuns := make([]domain.WorkflowStepRun, n)

	for i := range n {
		ref := fmt.Sprintf("step-%03d", i)
		steps[i] = domain.WorkflowStep{
			StepRef:           ref,
			CompensationJobID: "comp-" + ref,
		}
		if i > 0 {
			steps[i].DependsOn = []string{fmt.Sprintf("step-%03d", i-1)}
		}
		stepRuns[i] = domain.WorkflowStepRun{
			ID:      "sr-" + ref,
			StepRef: ref,
			Status:  domain.StepCompleted,
		}
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Steps,
		n)
	assert.Equal(t, "step-099",
		plan.
			Steps[0].StepRef,
	)
	assert.Equal(t, "step-000",
		plan.
			Steps[n-1].
			StepRef)

	// First compensated should be step-099 (last in chain).

	// Last should be step-000.

}

func TestCompensation_CompensationOfCompensation(t *testing.T) {
	t.Parallel()
	// A compensation job itself could have compensation config.
	// BuildCompensationPlan should not recurse -- it only looks at the original workflow steps.
	steps := []domain.WorkflowStep{
		{StepRef: "a", CompensationJobID: "comp-a"},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
	}

	plan, err := BuildCompensationPlan("wfr-1", steps, stepRuns)
	require.NoError(t, err)
	require.Len(t, plan.Steps,
		1)

	// Plan should be simple, not recursive.

}

func TestBuildTopologicalOrder_Deterministic(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "c"},
		{StepRef: "a"},
		{StepRef: "b"},
	}

	// Run multiple times, should always be same order.
	for range 10 {
		// Shuffle input.
		rand.Shuffle(len(steps), func(i, j int) {
			steps[i], steps[j] = steps[j], steps[i]
		})

		order := buildTopologicalOrder(steps)
		sorted := make([]string, len(order))
		copy(sorted, order)
		sort.Strings(sorted)

		for i, ref := range order {
			if ref != sorted[i] {
				assert.Failf(t, "test failure",

					"non-deterministic order: got %v", order)
				break
			}
		}
	}
}

func TestBuildTopologicalOrder_DuplicateDependencyRefs(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a"},
		{StepRef: "b", DependsOn: []string{"a", "a"}},
	}

	order := buildTopologicalOrder(steps)
	require.Equal(t, "a,b",
		strings.Join(order,
			","))

}
