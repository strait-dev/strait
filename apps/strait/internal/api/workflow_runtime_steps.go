package api

import (
	"context"
	"fmt"

	"strait/internal/domain"
	"strait/internal/store"
)

func (s *Server) loadWorkflowRunSteps(ctx context.Context, run *domain.WorkflowRun) ([]domain.WorkflowStep, error) {
	var steps []domain.WorkflowStep
	if run.WorkflowSnapshotID != "" {
		snapshot, err := s.store.GetWorkflowSnapshot(ctx, run.WorkflowSnapshotID)
		if err != nil {
			return nil, fmt.Errorf("get workflow snapshot: %w", err)
		}
		if snapshot != nil {
			definition, err := store.ParseSnapshotDefinition(snapshot.Definition)
			if err != nil {
				return nil, fmt.Errorf("parse workflow snapshot: %w", err)
			}
			steps = definition.Steps
		}
	}
	if steps == nil {
		var err error
		steps, err = s.store.ListStepsByWorkflowVersion(ctx, run.WorkflowID, run.WorkflowVersion)
		if err != nil {
			return nil, fmt.Errorf("list workflow steps by version: %w", err)
		}
	}

	dynamicSteps, err := s.store.ListDynamicWorkflowStepsByWorkflowRun(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list dynamic workflow steps: %w", err)
	}
	if len(dynamicSteps) == 0 {
		return steps, nil
	}

	seen := make(map[string]struct{}, len(steps)+len(dynamicSteps))
	for _, step := range steps {
		seen[step.StepRef] = struct{}{}
	}
	for _, step := range dynamicSteps {
		if _, exists := seen[step.StepRef]; exists {
			return nil, fmt.Errorf("duplicate runtime step_ref %q", step.StepRef)
		}
		seen[step.StepRef] = struct{}{}
		steps = append(steps, step)
	}

	return steps, nil
}
