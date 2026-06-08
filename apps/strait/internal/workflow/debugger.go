package workflow

import (
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"
)

// DebugView represents the full debug data for a workflow run.
type DebugView struct {
	WorkflowRunID string          `json:"workflow_run_id"`
	WorkflowID    string          `json:"workflow_id"`
	Status        string          `json:"status"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	TotalDuration int64           `json:"total_duration_ms"`
	TotalCost     int64           `json:"total_cost_microusd"`
	Steps         []DebugStep     `json:"steps"`
	DataFlow      []DataFlowEdge  `json:"data_flow"`
	Error         string          `json:"error,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// DebugStep represents a single step in the debug timeline.
type DebugStep struct {
	StepRef    string          `json:"step_ref"`
	StepRunID  string          `json:"step_run_id"`
	StepType   string          `json:"step_type"`
	Status     string          `json:"status"`
	JobRunID   string          `json:"job_run_id,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Duration   int64           `json:"duration_ms"`
	Cost       int64           `json:"cost_microusd,omitempty"`
	Attempt    int             `json:"attempt"`
	DependsOn  []string        `json:"depends_on,omitempty"`
}

// DataFlowEdge represents data flowing between steps.
type DataFlowEdge struct {
	FromStepRef string `json:"from_step_ref"`
	ToStepRef   string `json:"to_step_ref"`
	DataSize    int    `json:"data_size_bytes,omitempty"`
}

// RunComparison represents a diff between two workflow runs.
type RunComparison struct {
	RunA       string           `json:"run_a"`
	RunB       string           `json:"run_b"`
	StatusDiff *StringDiff      `json:"status_diff,omitempty"`
	StepDiffs  []StepComparison `json:"step_diffs"`
}

// StringDiff represents a difference in a string value.
type StringDiff struct {
	A string `json:"a"`
	B string `json:"b"`
}

// StepComparison represents the diff for a single step between two runs.
type StepComparison struct {
	StepRef    string      `json:"step_ref"`
	StatusDiff *StringDiff `json:"status_diff,omitempty"`
	DurationA  int64       `json:"duration_ms_a"`
	DurationB  int64       `json:"duration_ms_b"`
	OnlyInA    bool        `json:"only_in_a,omitempty"`
	OnlyInB    bool        `json:"only_in_b,omitempty"`
}

// BuildDebugView assembles a debug view from workflow run data and step runs.
func BuildDebugView(
	wfRun *domain.WorkflowRun,
	steps []domain.WorkflowStep,
	stepRuns []domain.WorkflowStepRun,
	stepCosts map[string]int64,
) (*DebugView, error) {
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run is nil")
	}

	if workflowDebugStepsAligned(steps, stepRuns) {
		return buildAlignedDebugView(wfRun, steps, stepRuns, stepCosts), nil
	}

	stepMap := make(map[string]*domain.WorkflowStep, len(steps))
	dataFlowCap := 0
	for i := range steps {
		stepMap[steps[i].StepRef] = &steps[i]
		dataFlowCap += len(steps[i].DependsOn)
	}

	view := &DebugView{
		WorkflowRunID: wfRun.ID,
		WorkflowID:    wfRun.WorkflowID,
		Status:        string(wfRun.Status),
		StartedAt:     wfRun.StartedAt,
		FinishedAt:    wfRun.FinishedAt,
		Error:         wfRun.Error,
		Payload:       wfRun.Payload,
		Steps:         make([]DebugStep, 0, len(stepRuns)),
		DataFlow:      make([]DataFlowEdge, 0, dataFlowCap),
	}

	if wfRun.StartedAt != nil && wfRun.FinishedAt != nil {
		view.TotalDuration = wfRun.FinishedAt.Sub(*wfRun.StartedAt).Milliseconds()
	}

	for _, sr := range stepRuns {
		step := stepMap[sr.StepRef]

		ds := DebugStep{
			StepRef:    sr.StepRef,
			StepRunID:  sr.ID,
			Status:     string(sr.Status),
			JobRunID:   sr.JobRunID,
			Output:     sr.Output,
			Error:      sr.Error,
			StartedAt:  sr.StartedAt,
			FinishedAt: sr.FinishedAt,
			Attempt:    sr.Attempt,
		}

		if step != nil {
			ds.StepType = string(step.StepType)
			ds.DependsOn = step.DependsOn
			if ds.StepType == "" {
				ds.StepType = "job"
			}
		}

		if sr.StartedAt != nil && sr.FinishedAt != nil {
			ds.Duration = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
		}

		if stepCosts != nil {
			ds.Cost = stepCosts[sr.ID]
		}
		view.TotalCost += ds.Cost

		view.Steps = append(view.Steps, ds)
	}

	var outputSizeByRef map[string]int
	if dataFlowCap > 0 {
		for _, sr := range stepRuns {
			if len(sr.Output) > 0 {
				if outputSizeByRef == nil {
					outputSizeByRef = make(map[string]int, len(stepRuns))
				}
				outputSizeByRef[sr.StepRef] = len(sr.Output)
			}
		}
	}

	// Build data flow edges from step dependencies.
	for _, sr := range stepRuns {
		step := stepMap[sr.StepRef]
		if step == nil {
			continue
		}
		for _, dep := range step.DependsOn {
			edge := DataFlowEdge{
				FromStepRef: dep,
				ToStepRef:   sr.StepRef,
			}
			edge.DataSize = outputSizeByRef[dep]
			view.DataFlow = append(view.DataFlow, edge)
		}
	}

	return view, nil
}

func workflowDebugStepsAligned(steps []domain.WorkflowStep, stepRuns []domain.WorkflowStepRun) bool {
	if len(steps) != len(stepRuns) {
		return false
	}
	for i := range steps {
		if steps[i].StepRef != stepRuns[i].StepRef {
			return false
		}
	}
	return true
}

func buildAlignedDebugView(
	wfRun *domain.WorkflowRun,
	steps []domain.WorkflowStep,
	stepRuns []domain.WorkflowStepRun,
	stepCosts map[string]int64,
) *DebugView {
	dataFlowCap := 0
	for i := range steps {
		dataFlowCap += len(steps[i].DependsOn)
	}

	view := &DebugView{
		WorkflowRunID: wfRun.ID,
		WorkflowID:    wfRun.WorkflowID,
		Status:        string(wfRun.Status),
		StartedAt:     wfRun.StartedAt,
		FinishedAt:    wfRun.FinishedAt,
		Error:         wfRun.Error,
		Payload:       wfRun.Payload,
		Steps:         make([]DebugStep, 0, len(stepRuns)),
		DataFlow:      make([]DataFlowEdge, 0, dataFlowCap),
	}

	if wfRun.StartedAt != nil && wfRun.FinishedAt != nil {
		view.TotalDuration = wfRun.FinishedAt.Sub(*wfRun.StartedAt).Milliseconds()
	}

	linearChain := stepsFormOrderedLinearChainByRef(steps)
	var outputSizeByRef map[string]int
	if dataFlowCap > 0 && !linearChain {
		for i := range stepRuns {
			if len(stepRuns[i].Output) > 0 {
				if outputSizeByRef == nil {
					outputSizeByRef = make(map[string]int, len(stepRuns))
				}
				outputSizeByRef[stepRuns[i].StepRef] = len(stepRuns[i].Output)
			}
		}
	}

	for i := range stepRuns {
		sr := &stepRuns[i]
		step := &steps[i]
		stepType := string(step.StepType)
		if stepType == "" {
			stepType = "job"
		}

		ds := DebugStep{
			StepRef:    sr.StepRef,
			StepRunID:  sr.ID,
			StepType:   stepType,
			Status:     string(sr.Status),
			JobRunID:   sr.JobRunID,
			Output:     sr.Output,
			Error:      sr.Error,
			StartedAt:  sr.StartedAt,
			FinishedAt: sr.FinishedAt,
			Attempt:    sr.Attempt,
			DependsOn:  step.DependsOn,
		}

		if sr.StartedAt != nil && sr.FinishedAt != nil {
			ds.Duration = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
		}

		if stepCosts != nil {
			ds.Cost = stepCosts[sr.ID]
		}
		view.TotalCost += ds.Cost
		view.Steps = append(view.Steps, ds)

		if linearChain {
			for _, dep := range step.DependsOn {
				edge := DataFlowEdge{
					FromStepRef: dep,
					ToStepRef:   step.StepRef,
				}
				if i > 0 {
					edge.DataSize = len(stepRuns[i-1].Output)
				}
				view.DataFlow = append(view.DataFlow, edge)
			}
		}
	}

	if linearChain {
		return view
	}

	for i := range steps {
		for _, dep := range steps[i].DependsOn {
			edge := DataFlowEdge{
				FromStepRef: dep,
				ToStepRef:   steps[i].StepRef,
			}
			edge.DataSize = outputSizeByRef[dep]
			view.DataFlow = append(view.DataFlow, edge)
		}
	}

	return view
}

// CompareRuns creates a diff between two workflow runs.
func CompareRuns(
	runA *domain.WorkflowRun, stepsA []domain.WorkflowStepRun,
	runB *domain.WorkflowRun, stepsB []domain.WorkflowStepRun,
) *RunComparison {
	comp := &RunComparison{
		RunA:      runA.ID,
		RunB:      runB.ID,
		StepDiffs: make([]StepComparison, 0),
	}

	if runA.Status != runB.Status {
		comp.StatusDiff = &StringDiff{A: string(runA.Status), B: string(runB.Status)}
	}

	if len(stepsA) == len(stepsB) {
		sameOrder := true
		for i := range stepsA {
			if stepsA[i].StepRef != stepsB[i].StepRef {
				sameOrder = false
				break
			}
		}
		if sameOrder {
			for i := range stepsA {
				appendStepComparisonDiff(comp, &stepsA[i], &stepsB[i])
			}
			return comp
		}
	}

	stepsBMap := make(map[string]*domain.WorkflowStepRun, len(stepsB))
	for i := range stepsB {
		stepsBMap[stepsB[i].StepRef] = &stepsB[i]
	}

	matchedB := 0
	for i := range stepsA {
		a := &stepsA[i]
		b, inB := stepsBMap[a.StepRef]

		sc := StepComparison{StepRef: a.StepRef}
		if !inB {
			sc.OnlyInA = true
			sc.DurationA = stepDurationMS(a)
			comp.StepDiffs = append(comp.StepDiffs, sc)
			continue
		}
		matchedB++

		appendStepComparisonDiff(comp, a, b)
	}
	if matchedB == len(stepsB) {
		return comp
	}

	stepsAMap := make(map[string]struct{}, len(stepsA))
	for i := range stepsA {
		stepsAMap[stepsA[i].StepRef] = struct{}{}
	}
	for i := range stepsB {
		b := &stepsB[i]
		if _, ok := stepsAMap[b.StepRef]; ok {
			continue
		}
		comp.StepDiffs = append(comp.StepDiffs, StepComparison{
			StepRef:   b.StepRef,
			OnlyInB:   true,
			DurationB: stepDurationMS(b),
		})
	}

	return comp
}

func appendStepComparisonDiff(comp *RunComparison, a, b *domain.WorkflowStepRun) {
	sc := StepComparison{
		StepRef:   a.StepRef,
		DurationA: stepDurationMS(a),
		DurationB: stepDurationMS(b),
	}
	if string(a.Status) != string(b.Status) {
		sc.StatusDiff = &StringDiff{A: string(a.Status), B: string(b.Status)}
	}
	if sc.StatusDiff != nil || sc.DurationA != sc.DurationB {
		comp.StepDiffs = append(comp.StepDiffs, sc)
	}
}

func stepDurationMS(sr *domain.WorkflowStepRun) int64 {
	if sr == nil || sr.StartedAt == nil || sr.FinishedAt == nil {
		return 0
	}
	return sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
}
