package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"strait/internal/domain"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ExecutionPlan) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.ExecutionPlan))
	}
	if result.ExecutionPlan[0].StepRef != "a" {
		t.Errorf("first step = %q, want a", result.ExecutionPlan[0].StepRef)
	}
	if result.ExecutionPlan[2].StepRef != "c" {
		t.Errorf("last step = %q, want c", result.ExecutionPlan[2].StepRef)
	}
	if result.EstimatedDuration != 60 {
		t.Errorf("estimated duration = %d, want 60", result.EstimatedDuration)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
	if bGroup != cGroup {
		t.Errorf("b (group %d) and c (group %d) should be in the same parallel group", bGroup, cGroup)
	}
}

func TestSimulate_DryRun_ConditionalBranch(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}, Condition: json.RawMessage(`{"op":"eq","left":"status","right":"ok"}`)},
	}
	req := &SimulateRequest{Mode: SimModeDryRun}

	result, err := SimulateWorkflow(steps, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ConditionResults["b"] != true {
		t.Error("dry run should assume conditions pass")
	}
	if result.ExecutionPlan[1].ConditionMet == nil || !*result.ExecutionPlan[1].ConditionMet {
		t.Error("condition_met should be true for dry run")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EstimatedCost != 3500 {
		t.Errorf("total cost = %d, want 3500", result.EstimatedCost)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.FailurePaths) != 1 {
		t.Fatalf("expected 1 failure path, got %d", len(result.FailurePaths))
	}
	if result.FailurePaths[0].StepRef != "b" {
		t.Errorf("failure step = %q, want b", result.FailurePaths[0].StepRef)
	}
	if result.FailurePaths[0].InjectedFailure != "simulated failure" {
		t.Errorf("injected failure = %q, want 'simulated failure'", result.FailurePaths[0].InjectedFailure)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Step a should show it would compensate.
	for _, s := range result.ExecutionPlan {
		if s.StepRef == "a" && !s.WouldCompensate {
			t.Error("step a should show would_compensate=true")
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.DAG.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(result.DAG.Nodes))
	}
	if len(result.DAG.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(result.DAG.Edges))
	}
}

func TestSimulate_EmptyPayload(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
	}
	req := &SimulateRequest{Mode: SimModeDryRun, Payload: nil}

	result, err := SimulateWorkflow(steps, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ExecutionPlan) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.ExecutionPlan))
	}
}

func TestSimulate_NilRequest(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}}
	_, err := SimulateWorkflow(steps, nil, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
}

func TestSimulate_NoSteps(t *testing.T) {
	t.Parallel()
	req := &SimulateRequest{Mode: SimModeDryRun}
	_, err := SimulateWorkflow(nil, req, nil)
	if err == nil {
		t.Error("expected error for no steps")
	}
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

// ValidateSimulateRequest tests.

func TestValidateSimulateRequest_Valid(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}}
	req := &SimulateRequest{Mode: SimModeDryRun}
	if err := ValidateSimulateRequest(req, steps); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateSimulateRequest_InvalidMode(t *testing.T) {
	t.Parallel()
	req := &SimulateRequest{Mode: "invalid"}
	err := ValidateSimulateRequest(req, nil)
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestValidateSimulateRequest_InvalidFailureInjection(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a"}}
	req := &SimulateRequest{
		Mode:             SimModeFailureInjection,
		FailureInjection: map[string]string{"nonexistent": "boom"},
	}
	err := ValidateSimulateRequest(req, steps)
	if err == nil {
		t.Error("expected error for unknown step ref in failure injection")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention unknown step, got: %v", err)
	}
}

func TestValidateSimulateRequest_NilRequest(t *testing.T) {
	t.Parallel()
	err := ValidateSimulateRequest(nil, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ExecutionPlan) != 100 {
		t.Errorf("expected 100 steps, got %d", len(result.ExecutionPlan))
	}
}

func TestSimulate_5MBPayload(t *testing.T) {
	t.Parallel()
	largePayload, _ := json.Marshal(map[string]string{"data": strings.Repeat("x", 5*1024*1024)})
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	req := &SimulateRequest{Mode: SimModeDryRun, Payload: largePayload}

	result, err := SimulateWorkflow(steps, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ExecutionPlan) != 1 {
		t.Errorf("expected 1 step, got %d", len(result.ExecutionPlan))
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.FailurePaths) != 2 {
		t.Errorf("expected 2 failure paths, got %d", len(result.FailurePaths))
	}
}

func TestSimulate_ModePreservedInResult(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	for _, mode := range []SimulationMode{SimModeDryRun, SimModeSandbox, SimModeFailureInjection} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			req := &SimulateRequest{Mode: mode}
			result, err := SimulateWorkflow(steps, req, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Mode != mode {
				t.Errorf("mode = %q, want %q", result.Mode, mode)
			}
		})
	}
}
