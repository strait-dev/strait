package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulate_DryRun_LinearDAG(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job", ExpectedDurationSecs: 10},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
		{StepRef: "c", StepType: "job", DependsOn: []string{"b"}, ExpectedDurationSecs: 30},
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	require.Len(t, result.ExecutionPlan,

		3)
	assert.Equal(t, "a", result.
		ExecutionPlan[0].StepRef)
	assert.Equal(t, "c", result.
		ExecutionPlan[2].StepRef)
	assert.Equal(t, 60, result.
		EstimatedDuration,
	)
	assert.Equal(t, []int{0, 1, 2}, []int{
		result.ExecutionPlan[0].ParallelGroup,
		result.ExecutionPlan[1].ParallelGroup,
		result.ExecutionPlan[2].ParallelGroup,
	})
	require.Len(t, result.DAG.Edges, 2)
	assert.Equal(t, DAGEdge{From: "a", To: "b"}, result.DAG.Edges[0])
	assert.Equal(t, DAGEdge{From: "b", To: "c"}, result.DAG.Edges[1])
}

func TestSimulate_DryRun_ParallelBranches(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job", ExpectedDurationSecs: 5},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}, ExpectedDurationSecs: 10},
		{StepRef: "c", StepType: "job", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)

	// b and c should be in the same parallel group.
	bGroup := -1
	cGroup := -1
	for _, s := range result.ExecutionPlan {
		if s.StepRef == "b" {
			bGroup = s.ParallelGroup
		}
		if s.StepRef == "c" {
			cGroup = s.ParallelGroup
		}
	}
	assert.Equal(t, cGroup,
		bGroup)
}

func TestSimulate_DryRun_ConditionalBranch(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}, Condition: json.RawMessage(`{"op":"eq","left":"status","right":"ok"}`)},
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	assert.True(t, result.
		ConditionResults["b"])
	assert.False(t, result.ExecutionPlan[1].ConditionMet ==
		nil || !*result.ExecutionPlan[1].ConditionMet)
}

func TestSimulate_CostEstimation(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job", JobID: "job-1"},
		{StepRef: "b", StepType: "job", JobID: "job-2", DependsOn: []string{"a"}},
	}
	costs := map[string]int64{
		"job-1": 1000,
		"job-2": 2500,
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	result, err := SimulateWorkflow(steps, req, costs)
	require.NoError(t, err)
	assert.EqualValues(t, 3500, result.
		EstimatedCost,
	)
}

func TestSimulate_FailureInjection_SingleStep(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
		{StepRef: "c", StepType: "job", DependsOn: []string{"b"}},
	}
	req := &SimulateRequest{
		Mode:             SimModeFailureInjection,
		FailureInjection: map[string]string{"b": "simulated failure"},
	}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	require.Len(t, result.FailurePaths,

		1)
	assert.Equal(t, "b", result.
		FailurePaths[0].StepRef)
	assert.Equal(t, "simulated failure",

		result.
			FailurePaths[0].InjectedFailure)
}

func TestSimulate_FailureInjection_WithCompensation(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job", CompensationJobID: "comp-a"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
	}
	req := &SimulateRequest{
		Mode:             SimModeFailureInjection,
		FailureInjection: map[string]string{"b": "boom"},
	}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)

	// Step a should show it would compensate.
	for _, s := range result.ExecutionPlan {
		assert.False(t, s.StepRef ==
			"a" &&
			!s.WouldCompensate,
		)
	}
}

func TestSimulate_DAGVisualizationData(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
		{StepRef: "c", StepType: "sleep", DependsOn: []string{"a"}},
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	assert.Len(t, result.DAG.
		Nodes,
		3)
	assert.Len(t, result.DAG.
		Edges,
		2)
}

func TestSimulate_EmptyPayload(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
	}
	req := &SimulateRequest{Mode: SimModeDryRun, Payload: nil}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	require.Len(t, result.ExecutionPlan,

		1)
}

func TestSimulate_NilRequest(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}}
	_, err := SimulateWorkflow(steps, nil, nil)
	assert.Error(t, err)
}

func TestSimulate_NoSteps(t *testing.T) {
	t.Parallel()
	req := &SimulateRequest{Mode: SimModeDryRun}
	_, err := SimulateWorkflow(nil, req, nil)
	assert.Error(t, err)
}

func BenchmarkSimulateWorkflow_Chain100(b *testing.B) {
	steps := make([]domain.WorkflowStep, 100)
	for i := range steps {
		steps[i] = domain.WorkflowStep{
			StepRef:              fmt.Sprintf("step-%03d", i),
			StepType:             domain.WorkflowStepTypeJob,
			ExpectedDurationSecs: 1,
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	b.ReportAllocs()
	for b.Loop() {
		result, err := SimulateWorkflow(steps, req, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.ExecutionPlan) != len(steps) {
			b.Fatalf("execution plan length = %d, want %d", len(result.ExecutionPlan), len(steps))
		}
	}
}

func BenchmarkSimulateWorkflow_Chain1000(b *testing.B) {
	steps := make([]domain.WorkflowStep, 1000)
	for i := range steps {
		steps[i] = domain.WorkflowStep{
			StepRef:              fmt.Sprintf("step-%04d", i),
			StepType:             domain.WorkflowStepTypeJob,
			ExpectedDurationSecs: 1,
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	b.ReportAllocs()
	for b.Loop() {
		result, err := SimulateWorkflow(steps, req, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.ExecutionPlan) != len(steps) {
			b.Fatalf("execution plan length = %d, want %d", len(result.ExecutionPlan), len(steps))
		}
		if result.EstimatedDuration != len(steps) {
			b.Fatalf("estimated duration = %d, want %d", result.EstimatedDuration, len(steps))
		}
	}
}

// ValidateSimulateRequest tests.

func TestValidateSimulateRequest_Valid(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}}
	req := &SimulateRequest{Mode: SimModeDryRun}
	assert.NoError(t, ValidateSimulateRequest(
		req, steps),
	)
}

func TestValidateSimulateRequest_InvalidMode(t *testing.T) {
	t.Parallel()
	req := &SimulateRequest{Mode: "invalid"}
	err := ValidateSimulateRequest(req, nil)
	assert.Error(t, err)
}

func TestValidateSimulateRequest_InvalidFailureInjection(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}}
	req := &SimulateRequest{
		Mode:             SimModeFailureInjection,
		FailureInjection: map[string]string{"nonexistent": "boom"},
	}
	err := ValidateSimulateRequest(req, steps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestValidateSimulateRequest_ValidFailureInjection(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b"}}
	req := &SimulateRequest{
		Mode: SimModeFailureInjection,
		FailureInjection: map[string]string{
			"a": "boom",
			"b": "again",
		},
	}
	require.NoError(t, ValidateSimulateRequest(req, steps))
}

func TestValidateSimulateRequest_SingleInvalidFailureInjection(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}}
	req := &SimulateRequest{
		Mode:             SimModeFailureInjection,
		FailureInjection: map[string]string{"missing": "boom"},
	}
	err := ValidateSimulateRequest(req, steps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestValidateSimulateRequest_NilRequest(t *testing.T) {
	t.Parallel()
	err := ValidateSimulateRequest(nil, nil)
	assert.Error(t, err)
}

func BenchmarkValidateSimulateRequest(b *testing.B) {
	steps := make([]domain.WorkflowStep, 100)
	for i := range steps {
		steps[i] = domain.WorkflowStep{StepRef: fmt.Sprintf("step-%03d", i)}
	}

	b.Run("no_failure_injection", func(b *testing.B) {
		req := &SimulateRequest{Mode: SimModeDryRun}
		b.ReportAllocs()
		for b.Loop() {
			if err := ValidateSimulateRequest(req, steps); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("single_failure_injection", func(b *testing.B) {
		req := &SimulateRequest{
			Mode:             SimModeFailureInjection,
			FailureInjection: map[string]string{"step-099": "boom"},
		}
		b.ReportAllocs()
		for b.Loop() {
			if err := ValidateSimulateRequest(req, steps); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Fuzz tests.

func FuzzSimulate_RandomPayloads(f *testing.F) {
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, payload []byte) {
		steps := []domain.WorkflowStep{
			{StepRef: "a", StepType: "job"},
			{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
		}
		req := &SimulateRequest{
			Mode:    SimModeDryRun,
			Payload: payload,
		}
		// Must never panic.
		_, _ = SimulateWorkflow(steps, req, nil)
	})
}

func FuzzSimulate_RandomFailureInjection(f *testing.F) {
	f.Add("a", "error")
	f.Add("b", "")
	f.Add("nonexistent", "boom")

	f.Fuzz(func(t *testing.T, stepRef, errMsg string) {
		steps := []domain.WorkflowStep{
			{StepRef: "a", StepType: "job"},
			{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
		}
		req := &SimulateRequest{
			Mode:             SimModeFailureInjection,
			FailureInjection: map[string]string{stepRef: errMsg},
		}
		// Must never panic.
		_, _ = SimulateWorkflow(steps, req, nil)
	})
}

// Adversarial tests.

func TestSimulate_100StepDAG(t *testing.T) {
	t.Parallel()
	steps := make([]domain.WorkflowStep, 100)
	for i := range 100 {
		steps[i] = domain.WorkflowStep{
			StepRef:              fmt.Sprintf("step-%03d", i),
			StepType:             "job",
			ExpectedDurationSecs: 1,
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	assert.Len(t, result.ExecutionPlan,

		100)
}

func TestSimulate_5MBPayload(t *testing.T) {
	t.Parallel()
	largePayload, _ := json.Marshal(map[string]string{"data": strings.Repeat("x", 5*1024*1024)})
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	req := &SimulateRequest{Mode: SimModeDryRun, Payload: largePayload}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	assert.Len(t, result.ExecutionPlan,

		1)
}

func TestSimulate_FailureInjectionMultipleSteps(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
		{StepRef: "c", StepType: "job", DependsOn: []string{"a"}},
		{StepRef: "d", StepType: "job", DependsOn: []string{"b", "c"}},
	}
	req := &SimulateRequest{
		Mode: SimModeFailureInjection,
		FailureInjection: map[string]string{
			"b": "error 1",
			"d": "error 2",
		},
	}

	result, err := SimulateWorkflow(steps, req, nil)
	require.NoError(t, err)
	assert.Len(t, result.FailurePaths,

		2)
}

func TestSimulate_ModePreservedInResult(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	for _, mode := range []SimulationMode{SimModeDryRun, SimModeSandbox, SimModeFailureInjection} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			req := &SimulateRequest{Mode: mode}
			result, err := SimulateWorkflow(steps, req, nil)
			require.NoError(t, err)
			assert.Equal(t, mode, result.
				Mode,
			)
		})
	}
}
