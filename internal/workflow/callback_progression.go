package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"orchestrator/internal/domain"
)

func (s *StepCallback) fanInAndStartReadyChildren(ctx context.Context, stepRun *domain.WorkflowStepRun) error {
	deps, err := s.store.IncrementStepDeps(ctx, stepRun.WorkflowRunID, stepRun.StepRef)
	if err != nil {
		return fmt.Errorf("increment step deps: %w", err)
	}
	if len(deps) == 0 {
		return nil
	}

	wfRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun.Status == domain.WfStatusPaused {
		return nil
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list steps by workflow: %w", err)
	}
	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, st := range steps {
		stepByRef[st.StepRef] = st
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}
	stepRunByID := make(map[string]domain.WorkflowStepRun, len(stepRuns))
	stepStatuses := make(map[string]domain.StepRunStatus, len(stepRuns))
	runningStepCount := 0
	for _, sr := range stepRuns {
		stepRunByID[sr.ID] = sr
		stepStatuses[sr.StepRef] = sr.Status
		if sr.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	for _, dep := range deps {
		if dep.DepsCompleted != dep.DepsRequired {
			continue
		}

		childStep, ok := stepByRef[dep.StepRef]
		if !ok {
			return fmt.Errorf("step definition not found for %s", dep.StepRef)
		}

		childStepRun, ok := stepRunByID[dep.StepRunID]
		if !ok {
			return fmt.Errorf("step run not found for %s", dep.StepRunID)
		}
		if childStepRun.Status.IsTerminal() {
			continue
		}
		if wfRun.MaxParallelSteps > 0 && runningStepCount >= wfRun.MaxParallelSteps {
			continue
		}

		allowed, err := EvaluateCondition(childStep.Condition, stepStatuses)
		if err != nil {
			return fmt.Errorf("evaluate condition for step %s: %w", childStep.StepRef, err)
		}

		if !allowed {
			now := time.Now()
			if err := s.store.UpdateStepRunStatus(ctx, childStepRun.ID, domain.StepSkipped, map[string]any{"finished_at": now}); err != nil {
				return fmt.Errorf("skip step %s: %w", childStep.StepRef, err)
			}
			stepStatuses[childStepRun.StepRef] = domain.StepSkipped
			continue
		}

		var parentOutputsPayload json.RawMessage
		if len(childStep.DependsOn) > 0 {
			outputs, err := s.store.GetStepOutputs(ctx, stepRun.WorkflowRunID, childStep.DependsOn)
			if err != nil {
				return fmt.Errorf("get step outputs for %s: %w", childStep.StepRef, err)
			}

			payload, err := json.Marshal(outputs)
			if err != nil {
				return fmt.Errorf("marshal parent outputs for %s: %w", childStep.StepRef, err)
			}
			parentOutputsPayload = payload
		}

		childRun := childStepRun
		stepDef := childStep
		if err := s.engine.startStep(ctx, &childRun, &stepDef, wfRun, parentOutputsPayload); err != nil {
			return fmt.Errorf("start child step %s: %w", childStep.StepRef, err)
		}
		stepStatuses[childStepRun.StepRef] = domain.StepRunning
		if childRun.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	return nil
}

func (s *StepCallback) checkWorkflowCompletion(ctx context.Context, workflowRunID string) error {
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}

	for _, sr := range stepRuns {
		if !sr.Status.IsTerminal() {
			return nil
		}
	}

	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun.Status.IsTerminal() {
		return nil
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}

	policyByRef := make(map[string]domain.FailurePolicy, len(steps))
	for _, step := range steps {
		policyByRef[step.StepRef] = step.OnFailure
	}

	hasFailingStep := false
	for _, sr := range stepRuns {
		if sr.Status != domain.StepFailed {
			continue
		}

		if policyByRef[sr.StepRef] != domain.Continue {
			hasFailingStep = true
			break
		}
	}

	now := time.Now()
	if hasFailingStep {
		if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusFailed, map[string]any{"finished_at": now}); err != nil {
			return fmt.Errorf("mark workflow run failed: %w", err)
		}
		wfRun.Status = domain.WfStatusFailed
		return s.propagateToParent(ctx, wfRun, stepRuns)
	}

	if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCompleted, map[string]any{"finished_at": now}); err != nil {
		return fmt.Errorf("mark workflow run completed: %w", err)
	}
	wfRun.Status = domain.WfStatusCompleted

	return s.propagateToParent(ctx, wfRun, stepRuns)
}

// propagateToParent propagates the terminal status of a child workflow run
// back to the parent step run that spawned it via sub_workflow.
func (s *StepCallback) propagateToParent(ctx context.Context, childRun *domain.WorkflowRun, childStepRuns []domain.WorkflowStepRun) error {
	if childRun.ParentWorkflowRunID == "" {
		return nil
	}
	if !childRun.Status.IsTerminal() {
		return nil
	}

	parentRun, err := s.store.GetWorkflowRun(ctx, childRun.ParentWorkflowRunID)
	if err != nil {
		return fmt.Errorf("get parent workflow run: %w", err)
	}
	if parentRun == nil {
		s.logger.Warn("parent workflow run not found for sub-workflow propagation",
			"child_run_id", childRun.ID, "parent_run_id", childRun.ParentWorkflowRunID)
		return nil
	}
	if parentRun.Status.IsTerminal() {
		return nil
	}

	// Get parent's step definitions to find which step is a sub_workflow that matches this child.
	parentSteps, err := s.store.ListStepsByWorkflowVersion(ctx, parentRun.WorkflowID, parentRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list parent workflow steps: %w", err)
	}

	// Find the sub_workflow step whose SubWorkflowID matches the child's workflow ID.
	var matchingStepRef string
	for _, step := range parentSteps {
		if step.StepType == domain.WorkflowStepTypeSubWorkflow && step.SubWorkflowID == childRun.WorkflowID {
			matchingStepRef = step.StepRef
			break
		}
	}
	if matchingStepRef == "" {
		s.logger.Warn("no matching sub_workflow step found in parent",
			"child_run_id", childRun.ID, "child_workflow_id", childRun.WorkflowID,
			"parent_run_id", parentRun.ID)
		return nil
	}

	// Find the parent step run for this sub_workflow step.
	parentStepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, parentRun.ID, matchingStepRef)
	if err != nil {
		return fmt.Errorf("get parent step run for sub-workflow: %w", err)
	}
	if parentStepRun == nil || parentStepRun.Status.IsTerminal() {
		return nil
	}

	now := time.Now()
	switch childRun.Status {
	case domain.WfStatusCompleted:
		// Aggregate child step outputs as the parent step's output.
		var outputPayload json.RawMessage
		if len(childStepRuns) > 0 {
			outputs := make(map[string]json.RawMessage, len(childStepRuns))
			for _, sr := range childStepRuns {
				if len(sr.Output) > 0 {
					outputs[sr.StepRef] = sr.Output
				}
			}
			if len(outputs) > 0 {
				if raw, marshalErr := json.Marshal(outputs); marshalErr == nil {
					outputPayload = raw
				}
			}
		}

		fields := map[string]any{"finished_at": now}
		if len(outputPayload) > 0 {
			fields["output"] = outputPayload
		}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepCompleted, fields); err != nil {
			return fmt.Errorf("complete parent step run for sub-workflow: %w", err)
		}
		parentStepRun.Status = domain.StepCompleted
		if len(outputPayload) > 0 {
			parentStepRun.Output = outputPayload
		}

		if err := s.fanInAndStartReadyChildren(ctx, parentStepRun); err != nil {
			return fmt.Errorf("fan-in after sub-workflow completion for step %s: %w", matchingStepRef, err)
		}
		return s.checkWorkflowCompletion(ctx, parentRun.ID)

	case domain.WfStatusFailed:
		fields := map[string]any{"finished_at": now, "error": childRun.Error}
		if childRun.Error == "" {
			fields["error"] = fmt.Sprintf("sub-workflow %s failed", childRun.WorkflowID)
		}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepFailed, fields); err != nil {
			return fmt.Errorf("fail parent step run for sub-workflow: %w", err)
		}
		parentStepRun.Status = domain.StepFailed
		parentStepRun.Error = fields["error"].(string)

		return s.handleFailedStep(ctx, parentStepRun)

	default:
		// Canceled or timed_out child workflows fail the parent step.
		errMsg := fmt.Sprintf("sub-workflow %s ended with status %s", childRun.WorkflowID, childRun.Status)
		fields := map[string]any{"finished_at": now, "error": errMsg}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepFailed, fields); err != nil {
			return fmt.Errorf("fail parent step run for sub-workflow: %w", err)
		}
		parentStepRun.Status = domain.StepFailed
		parentStepRun.Error = errMsg

		return s.handleFailedStep(ctx, parentStepRun)
	}
}

func (s *StepCallback) ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error {
	if approver == "" {
		return fmt.Errorf("approver is required")
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
	if err != nil {
		return fmt.Errorf("get step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("step run not found for %s", stepRef)
	}
	if stepRun.Status.IsTerminal() {
		return fmt.Errorf("step %s is already in terminal state", stepRef)
	}

	approval, err := s.store.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	if err != nil {
		return fmt.Errorf("get workflow step approval: %w", err)
	}
	if approval == nil {
		return fmt.Errorf("approval not found for step %s", stepRef)
	}
	if approval.Status != domain.ApprovalStatusPending {
		return fmt.Errorf("approval for step %s is already %s", stepRef, approval.Status)
	}

	if !slices.Contains(approval.Approvers, approver) {
		return fmt.Errorf("approver %s is not allowed for step %s", approver, stepRef)
	}

	now := time.Now()
	if err := s.store.UpdateWorkflowStepApproval(ctx, approval.ID, domain.ApprovalStatusApproved, approver, &now, ""); err != nil {
		return fmt.Errorf("update approval: %w", err)
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepCompleted, map[string]any{"finished_at": now}); err != nil {
		return fmt.Errorf("complete approval step run: %w", err)
	}

	stepRun.Status = domain.StepCompleted
	if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
		return fmt.Errorf("fan-in after approval for step %s: %w", stepRef, err)
	}

	return s.checkWorkflowCompletion(ctx, workflowRunID)
}

func (s *StepCallback) SkipStep(ctx context.Context, workflowRunID, stepRef, reason string) error {
	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
	if err != nil {
		return fmt.Errorf("get step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("step run not found for %s", stepRef)
	}
	if stepRun.Status != domain.StepPending && stepRun.Status != domain.StepWaiting {
		return fmt.Errorf("cannot skip step in %s status", stepRun.Status)
	}

	now := time.Now()
	fields := map[string]any{"finished_at": now}
	if reason != "" {
		fields["error"] = reason
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepSkipped, fields); err != nil {
		return fmt.Errorf("skip step: %w", err)
	}
	stepRun.Status = domain.StepSkipped

	if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
		return fmt.Errorf("fan-in after skip: %w", err)
	}
	return s.checkWorkflowCompletion(ctx, workflowRunID)
}

func (s *StepCallback) ForceCompleteStep(ctx context.Context, workflowRunID, stepRef string, result json.RawMessage) error {
	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
	if err != nil {
		return fmt.Errorf("get step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("step run not found for %s", stepRef)
	}
	if stepRun.Status != domain.StepPending && stepRun.Status != domain.StepWaiting {
		return fmt.Errorf("cannot force-complete step in %s status", stepRun.Status)
	}

	now := time.Now()
	fields := map[string]any{"finished_at": now}
	if len(result) > 0 {
		fields["output"] = result
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepCompleted, fields); err != nil {
		return fmt.Errorf("force-complete step: %w", err)
	}
	stepRun.Status = domain.StepCompleted

	if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
		return fmt.Errorf("fan-in after force-complete: %w", err)
	}
	return s.checkWorkflowCompletion(ctx, workflowRunID)
}

func (s *StepCallback) ResumeWorkflowRun(ctx context.Context, workflowRunID string) error {
	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return fmt.Errorf("workflow run not found: %s", workflowRunID)
	}
	if wfRun.Status != domain.WfStatusPaused {
		return fmt.Errorf("workflow run %s is not paused", workflowRunID)
	}

	if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPaused, domain.WfStatusRunning, nil); err != nil {
		return fmt.Errorf("resume workflow run: %w", err)
	}

	wfRun.Status = domain.WfStatusRunning

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}
	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, step := range steps {
		stepByRef[step.StepRef] = step
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}
	stepStatuses := make(map[string]domain.StepRunStatus, len(stepRuns))
	runningStepCount := 0
	for _, sr := range stepRuns {
		stepStatuses[sr.StepRef] = sr.Status
		if sr.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	for _, sr := range stepRuns {
		if sr.Status.IsTerminal() || sr.Status == domain.StepRunning {
			continue
		}
		if sr.DepsCompleted != sr.DepsRequired {
			continue
		}
		if wfRun.MaxParallelSteps > 0 && runningStepCount >= wfRun.MaxParallelSteps {
			continue
		}

		stepDef, ok := stepByRef[sr.StepRef]
		if !ok {
			return fmt.Errorf("step definition not found for %s", sr.StepRef)
		}

		allowed, condErr := EvaluateCondition(stepDef.Condition, stepStatuses)
		if condErr != nil {
			return fmt.Errorf("evaluate condition for step %s: %w", stepDef.StepRef, condErr)
		}
		if !allowed {
			now := time.Now()
			if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepSkipped, map[string]any{"finished_at": now}); err != nil {
				return fmt.Errorf("skip step %s: %w", stepDef.StepRef, err)
			}
			stepStatuses[sr.StepRef] = domain.StepSkipped
			continue
		}

		var parentOutputsPayload json.RawMessage
		if len(stepDef.DependsOn) > 0 {
			outputs, err := s.store.GetStepOutputs(ctx, workflowRunID, stepDef.DependsOn)
			if err != nil {
				return fmt.Errorf("get step outputs for %s: %w", stepDef.StepRef, err)
			}
			payload, err := json.Marshal(outputs)
			if err != nil {
				return fmt.Errorf("marshal parent outputs for %s: %w", stepDef.StepRef, err)
			}
			parentOutputsPayload = payload
		}

		srCopy := sr
		stepCopy := stepDef
		if err := s.engine.startStep(ctx, &srCopy, &stepCopy, wfRun, parentOutputsPayload); err != nil {
			return fmt.Errorf("start resumed step %s: %w", stepDef.StepRef, err)
		}
		stepStatuses[sr.StepRef] = srCopy.Status
		if srCopy.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	return nil
}

// lookupOutputTransform finds the output_transform path for a step.
// Returns empty string if none is configured or on lookup error.
func (s *StepCallback) lookupOutputTransform(ctx context.Context, stepRun *domain.WorkflowStepRun, wfRun *domain.WorkflowRun) string {
	if wfRun == nil {
		loadedRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
		if err != nil || loadedRun == nil {
			return ""
		}
		wfRun = loadedRun
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return ""
	}

	for _, st := range steps {
		if st.StepRef == stepRun.StepRef {
			return st.OutputTransform
		}
	}

	return ""
}
