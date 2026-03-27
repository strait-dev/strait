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

	stepMap := make(map[string]*domain.WorkflowStep, len(steps))
	for i := range steps {
		stepMap[steps[i].StepRef] = &steps[i]
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
		DataFlow:      make([]DataFlowEdge, 0),
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
			// Calculate data size from output of parent step.
			for _, parentSR := range stepRuns {
				if parentSR.StepRef == dep && len(parentSR.Output) > 0 {
					edge.DataSize = len(parentSR.Output)
					break
				}
			}
			view.DataFlow = append(view.DataFlow, edge)
		}
	}

	return view, nil
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

	stepsAMap := make(map[string]*domain.WorkflowStepRun, len(stepsA))
	for i := range stepsA {
		stepsAMap[stepsA[i].StepRef] = &stepsA[i]
	}
	stepsBMap := make(map[string]*domain.WorkflowStepRun, len(stepsB))
	for i := range stepsB {
		stepsBMap[stepsB[i].StepRef] = &stepsB[i]
	}

	// All refs from both runs.
	allRefs := make(map[string]bool)
	for _, s := range stepsA {
		allRefs[s.StepRef] = true
	}
	for _, s := range stepsB {
		allRefs[s.StepRef] = true
	}

	for ref := range allRefs {
		a, inA := stepsAMap[ref]
		b, inB := stepsBMap[ref]

		sc := StepComparison{StepRef: ref}

		if inA && !inB {
			sc.OnlyInA = true
			sc.DurationA = stepDurationMS(a)
			comp.StepDiffs = append(comp.StepDiffs, sc)
			continue
		}
		if !inA && inB {
			sc.OnlyInB = true
			sc.DurationB = stepDurationMS(b)
			comp.StepDiffs = append(comp.StepDiffs, sc)
			continue
		}

		if string(a.Status) != string(b.Status) {
			sc.StatusDiff = &StringDiff{A: string(a.Status), B: string(b.Status)}
		}
		sc.DurationA = stepDurationMS(a)
		sc.DurationB = stepDurationMS(b)

		if sc.StatusDiff != nil || sc.DurationA != sc.DurationB {
			comp.StepDiffs = append(comp.StepDiffs, sc)
		}
	}

	return comp
}

func stepDurationMS(sr *domain.WorkflowStepRun) int64 {
	if sr == nil || sr.StartedAt == nil || sr.FinishedAt == nil {
		return 0
	}
	return sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
}
