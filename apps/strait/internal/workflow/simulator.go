package workflow

import (
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"
)

// SimulationMode defines the type of simulation.
type SimulationMode string

const (
	SimModeDryRun           SimulationMode = "dry_run"
	SimModeSandbox          SimulationMode = "sandbox"
	SimModeFailureInjection SimulationMode = "failure_injection"
)

// SimulateRequest is the input for a workflow simulation.
type SimulateRequest struct {
	WorkflowID       string            `json:"workflow_id"`
	Payload          json.RawMessage   `json:"payload,omitempty"`
	Mode             SimulationMode    `json:"mode"`
	FailureInjection map[string]string `json:"failure_injection,omitempty"` // step_ref -> error message
}

// SimulationResult is the output of a workflow simulation.
type SimulationResult struct {
	ExecutionPlan     []SimulatedStep `json:"execution_plan"`
	DAG               SimulationDAG   `json:"dag"`
	EstimatedDuration int             `json:"estimated_duration_secs"`
	EstimatedCost     int64           `json:"estimated_cost_microusd"`
	ConditionResults  map[string]bool `json:"condition_results,omitempty"`
	FailurePaths      []SimulatedStep `json:"failure_paths,omitempty"`
	Mode              SimulationMode  `json:"mode"`
}

// SimulatedStep represents a step in the simulation execution plan.
type SimulatedStep struct {
	StepRef           string          `json:"step_ref"`
	StepType          string          `json:"step_type"`
	Order             int             `json:"order"`
	ParallelGroup     int             `json:"parallel_group"`
	DependsOn         []string        `json:"depends_on,omitempty"`
	EstimatedDuration int             `json:"estimated_duration_secs"`
	EstimatedCost     int64           `json:"estimated_cost_microusd,omitempty"`
	ConditionMet      *bool           `json:"condition_met,omitempty"`
	InjectedFailure   string          `json:"injected_failure,omitempty"`
	WouldCompensate   bool            `json:"would_compensate,omitempty"`
	CompensationJobID string          `json:"compensation_job_id,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
}

// SimulationDAG contains the DAG structure for visualization.
type SimulationDAG struct {
	Nodes []DAGNode `json:"nodes"`
	Edges []DAGEdge `json:"edges"`
}

// DAGNode represents a node in the DAG visualization.
type DAGNode struct {
	ID       string `json:"id"`
	StepRef  string `json:"step_ref"`
	StepType string `json:"step_type"`
	Group    int    `json:"parallel_group"`
}

// DAGEdge represents a dependency edge in the DAG.
type DAGEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// SimulateWorkflow performs a dry-run simulation of a workflow.
func SimulateWorkflow(
	steps []domain.WorkflowStep,
	req *SimulateRequest,
	costEstimates map[string]int64,
) (*SimulationResult, error) {
	if req == nil {
		return nil, fmt.Errorf("simulation request is nil")
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow has no steps")
	}

	// Build topological order.
	order := buildTopologicalOrder(steps)
	stepByRef := make(map[string]*domain.WorkflowStep, len(steps))
	for i := range steps {
		stepByRef[steps[i].StepRef] = &steps[i]
	}

	// Assign parallel groups.
	groups := assignParallelGroups(steps, order)

	// Build execution plan.
	plan := make([]SimulatedStep, 0, len(order))
	failurePaths := make([]SimulatedStep, 0)
	conditionResults := make(map[string]bool)

	for i, ref := range order {
		step := stepByRef[ref]
		if step == nil {
			continue
		}

		simStep := SimulatedStep{
			StepRef:           ref,
			StepType:          string(step.StepType),
			Order:             i + 1,
			ParallelGroup:     groups[ref],
			DependsOn:         step.DependsOn,
			EstimatedDuration: step.ExpectedDurationSecs,
		}

		if simStep.StepType == "" {
			simStep.StepType = "job"
		}

		// Cost estimate.
		if step.JobID != "" && costEstimates != nil {
			simStep.EstimatedCost = costEstimates[step.JobID]
		}

		// Condition evaluation (simplified: mark as present).
		if len(step.Condition) > 0 {
			met := true // dry-run assumes conditions pass.
			simStep.ConditionMet = &met
			conditionResults[ref] = met
		}

		// Compensation info.
		if step.CompensationJobID != "" {
			simStep.WouldCompensate = true
			simStep.CompensationJobID = step.CompensationJobID
		}

		// Failure injection.
		if req.FailureInjection != nil {
			if errMsg, injected := req.FailureInjection[ref]; injected {
				simStep.InjectedFailure = errMsg
				failurePaths = append(failurePaths, simStep)
			}
		}

		plan = append(plan, simStep)
	}

	// Calculate totals.
	totalDuration := 0
	var totalCost int64
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if expectedTime := CalculateExpectedCompletion(steps, epoch); expectedTime != nil {
		totalDuration = int(expectedTime.Sub(epoch).Seconds())
	}
	for _, s := range plan {
		totalCost += s.EstimatedCost
	}

	// Build DAG.
	dag := buildSimulationDAG(steps, groups)

	return &SimulationResult{
		ExecutionPlan:     plan,
		DAG:               dag,
		EstimatedDuration: totalDuration,
		EstimatedCost:     totalCost,
		ConditionResults:  conditionResults,
		FailurePaths:      failurePaths,
		Mode:              req.Mode,
	}, nil
}

func assignParallelGroups(steps []domain.WorkflowStep, order []string) map[string]int {
	groups := make(map[string]int, len(steps))
	depDepth := make(map[string]int, len(steps))

	stepByRef := make(map[string]*domain.WorkflowStep, len(steps))
	for i := range steps {
		stepByRef[steps[i].StepRef] = &steps[i]
	}

	for _, ref := range order {
		step := stepByRef[ref]
		if step == nil {
			continue
		}
		maxParentDepth := -1
		for _, dep := range step.DependsOn {
			if d, ok := depDepth[dep]; ok && d > maxParentDepth {
				maxParentDepth = d
			}
		}
		depth := maxParentDepth + 1
		depDepth[ref] = depth
		groups[ref] = depth
	}

	return groups
}

func buildSimulationDAG(steps []domain.WorkflowStep, groups map[string]int) SimulationDAG {
	dag := SimulationDAG{
		Nodes: make([]DAGNode, len(steps)),
		Edges: make([]DAGEdge, 0),
	}

	for i, s := range steps {
		stepType := string(s.StepType)
		if stepType == "" {
			stepType = "job"
		}
		dag.Nodes[i] = DAGNode{
			ID:       s.StepRef,
			StepRef:  s.StepRef,
			StepType: stepType,
			Group:    groups[s.StepRef],
		}
		for _, dep := range s.DependsOn {
			dag.Edges = append(dag.Edges, DAGEdge{From: dep, To: s.StepRef})
		}
	}

	return dag
}

// ValidateSimulateRequest checks that a simulation request is valid.
func ValidateSimulateRequest(req *SimulateRequest, steps []domain.WorkflowStep) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	validModes := map[SimulationMode]bool{
		SimModeDryRun:           true,
		SimModeSandbox:          true,
		SimModeFailureInjection: true,
	}
	if !validModes[req.Mode] {
		return fmt.Errorf("invalid simulation mode: %s", req.Mode)
	}

	// Validate failure injection step refs exist.
	if req.FailureInjection != nil {
		stepRefs := make(map[string]bool, len(steps))
		for _, s := range steps {
			stepRefs[s.StepRef] = true
		}
		for ref := range req.FailureInjection {
			if !stepRefs[ref] {
				return fmt.Errorf("failure injection references unknown step: %s", ref)
			}
		}
	}

	return nil
}
