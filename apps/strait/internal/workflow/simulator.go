package workflow

import (
	"encoding/json"
	"fmt"

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

	if stepsFormOrderedLinearChainByRef(steps) {
		return simulateOrderedLinearWorkflow(steps, req, costEstimates), nil
	}

	// Build topological order.
	stepIndex := buildStepIndex(steps)
	order := buildTopologicalOrderIndexesWithStepIndex(steps, stepIndex)

	// Assign parallel groups and calculate critical-path duration in one
	// dependency pass over the topological order.
	groups, totalDuration := calculateSimulationTimings(steps, order, stepIndex)

	// Build execution plan.
	plan := make([]SimulatedStep, 0, len(order))
	var failurePaths []SimulatedStep
	var conditionResults map[string]bool
	var totalCost int64

	for i, stepIdx := range order {
		step := &steps[stepIdx]

		simStep := SimulatedStep{
			StepRef:           step.StepRef,
			StepType:          string(step.StepType),
			Order:             i + 1,
			ParallelGroup:     groups[stepIdx],
			DependsOn:         step.DependsOn,
			EstimatedDuration: step.ExpectedDurationSecs,
		}

		if simStep.StepType == "" {
			simStep.StepType = "job"
		}

		// Cost estimate.
		if step.JobID != "" && costEstimates != nil {
			simStep.EstimatedCost = costEstimates[step.JobID]
			totalCost += simStep.EstimatedCost
		}

		// Condition evaluation (simplified: mark as present).
		if len(step.Condition) > 0 {
			met := true // dry-run assumes conditions pass.
			simStep.ConditionMet = &met
			if conditionResults == nil {
				conditionResults = make(map[string]bool)
			}
			conditionResults[step.StepRef] = met
		}

		// Compensation info.
		if step.CompensationJobID != "" {
			simStep.WouldCompensate = true
			simStep.CompensationJobID = step.CompensationJobID
		}

		// Failure injection.
		if req.FailureInjection != nil {
			if errMsg, injected := req.FailureInjection[step.StepRef]; injected {
				simStep.InjectedFailure = errMsg
				failurePaths = append(failurePaths, simStep)
			}
		}

		plan = append(plan, simStep)
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

func simulateOrderedLinearWorkflow(
	steps []domain.WorkflowStep,
	req *SimulateRequest,
	costEstimates map[string]int64,
) *SimulationResult {
	plan := make([]SimulatedStep, len(steps))
	nodes := make([]DAGNode, len(steps))
	edges := make([]DAGEdge, 0, max(0, len(steps)-1))

	var failurePaths []SimulatedStep
	var conditionResults map[string]bool
	var totalCost int64
	totalDuration := 0

	for i := range steps {
		step := &steps[i]
		stepType := string(step.StepType)
		if stepType == "" {
			stepType = "job"
		}

		simStep := SimulatedStep{
			StepRef:           step.StepRef,
			StepType:          stepType,
			Order:             i + 1,
			ParallelGroup:     i,
			DependsOn:         step.DependsOn,
			EstimatedDuration: step.ExpectedDurationSecs,
		}

		if step.JobID != "" && costEstimates != nil {
			simStep.EstimatedCost = costEstimates[step.JobID]
			totalCost += simStep.EstimatedCost
		}

		if len(step.Condition) > 0 {
			met := true
			simStep.ConditionMet = &met
			if conditionResults == nil {
				conditionResults = make(map[string]bool)
			}
			conditionResults[step.StepRef] = met
		}

		if step.CompensationJobID != "" {
			simStep.WouldCompensate = true
			simStep.CompensationJobID = step.CompensationJobID
		}

		if req.FailureInjection != nil {
			if errMsg, injected := req.FailureInjection[step.StepRef]; injected {
				simStep.InjectedFailure = errMsg
				failurePaths = append(failurePaths, simStep)
			}
		}

		plan[i] = simStep
		nodes[i] = DAGNode{
			ID:       step.StepRef,
			StepRef:  step.StepRef,
			StepType: stepType,
			Group:    i,
		}
		for _, dep := range step.DependsOn {
			edges = append(edges, DAGEdge{From: dep, To: step.StepRef})
		}
		totalDuration += step.ExpectedDurationSecs
	}

	return &SimulationResult{
		ExecutionPlan:     plan,
		DAG:               SimulationDAG{Nodes: nodes, Edges: edges},
		EstimatedDuration: totalDuration,
		EstimatedCost:     totalCost,
		ConditionResults:  conditionResults,
		FailurePaths:      failurePaths,
		Mode:              req.Mode,
	}
}

func calculateSimulationTimings(
	steps []domain.WorkflowStep,
	order []int,
	stepIndex map[string]int,
) ([]int, int) {
	groups := make([]int, len(steps))
	durationByStep := make([]int, len(steps))
	maxDuration := 0
	for _, stepIdx := range order {
		step := &steps[stepIdx]
		maxParentDepth := -1
		parentDuration := 0
		for _, dep := range step.DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			if groups[depIdx] > maxParentDepth {
				maxParentDepth = groups[depIdx]
			}
			if durationByStep[depIdx] > parentDuration {
				parentDuration = durationByStep[depIdx]
			}
		}

		groups[stepIdx] = maxParentDepth + 1
		durationByStep[stepIdx] = parentDuration + step.ExpectedDurationSecs
		if durationByStep[stepIdx] > maxDuration {
			maxDuration = durationByStep[stepIdx]
		}
	}

	return groups, maxDuration
}

func buildSimulationDAG(steps []domain.WorkflowStep, groups []int) SimulationDAG {
	edgeCount := 0
	for _, s := range steps {
		edgeCount += len(s.DependsOn)
	}
	dag := SimulationDAG{
		Nodes: make([]DAGNode, len(steps)),
		Edges: make([]DAGEdge, 0, edgeCount),
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
			Group:    groups[i],
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

	switch req.Mode {
	case SimModeDryRun, SimModeSandbox, SimModeFailureInjection:
	default:
		return fmt.Errorf("invalid simulation mode: %s", req.Mode)
	}

	// Validate failure injection step refs exist.
	if len(req.FailureInjection) == 0 {
		return nil
	}
	if len(req.FailureInjection) == 1 {
		for ref := range req.FailureInjection {
			for _, s := range steps {
				if s.StepRef == ref {
					return nil
				}
			}
			return fmt.Errorf("failure injection references unknown step: %s", ref)
		}
	}

	stepRefs := make(map[string]struct{}, len(steps))
	for _, s := range steps {
		stepRefs[s.StepRef] = struct{}{}
	}
	for ref := range req.FailureInjection {
		if _, ok := stepRefs[ref]; !ok {
			return fmt.Errorf("failure injection references unknown step: %s", ref)
		}
	}

	return nil
}
