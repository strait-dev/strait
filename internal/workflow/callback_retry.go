package workflow

import (
	"context"
	"fmt"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/worker"
)

func (s *StepCallback) checkStepRetry(ctx context.Context, stepRun *domain.WorkflowStepRun, _ *domain.JobRun) (bool, time.Time, int, error) {
	wfRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return false, time.Time{}, 0, fmt.Errorf("workflow run not found: %s", stepRun.WorkflowRunID)
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("list workflow steps: %w", err)
	}

	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, st := range steps {
		stepByRef[st.StepRef] = st
	}

	failedStep, ok := stepByRef[stepRun.StepRef]
	if !ok {
		return false, time.Time{}, 0, fmt.Errorf("step definition not found for %s", stepRun.StepRef)
	}

	retryMaxAttempts := failedStep.RetryMaxAttempts
	if retryMaxAttempts <= 0 {
		return false, time.Time{}, 0, nil
	}

	currentAttempt := stepRun.Attempt
	if currentAttempt >= retryMaxAttempts {
		s.logger.Debug("step retry exhausted", "step_ref", stepRun.StepRef, "attempt", currentAttempt, "max_attempts", retryMaxAttempts)
		return false, time.Time{}, 0, nil
	}

	newAttempt := currentAttempt + 1
	retryBackoff := failedStep.RetryBackoff
	retryInitialDelaySecs := failedStep.RetryInitialDelaySecs
	retryMaxDelaySecs := failedStep.RetryMaxDelaySecs

	nextRetryDelay := worker.NextRetryDelayWithPolicy(
		newAttempt,
		retryBackoff,
		retryInitialDelaySecs,
		retryMaxDelaySecs,
	)
	nextRetryAt := time.Now().Add(nextRetryDelay)

	s.logger.Info("scheduling step retry", "step_ref", stepRun.StepRef, "attempt", currentAttempt, "next_attempt", newAttempt, "retry_at", nextRetryAt)

	return true, nextRetryAt, newAttempt, nil
}

func (s *StepCallback) scheduleStepRetry(ctx context.Context, jobRun *domain.JobRun, stepRun *domain.WorkflowStepRun, nextRetryAt time.Time, newAttempt int) error {
	if err := s.store.IncrementStepRunAttempt(ctx, stepRun.ID, newAttempt); err != nil {
		return fmt.Errorf("increment step run attempt: %w", err)
	}

	fields := map[string]any{
		"next_retry_at": nextRetryAt,
		"attempt":       newAttempt,
	}
	if err := s.store.UpdateRunStatus(ctx, jobRun.ID, jobRun.Status, domain.StatusDelayed, fields); err != nil {
		return fmt.Errorf("update job run status for retry: %w", err)
	}

	return nil
}

func (s *StepCallback) handleFailedStep(ctx context.Context, stepRun *domain.WorkflowStepRun) error {
	wfRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return fmt.Errorf("workflow run not found: %s", stepRun.WorkflowRunID)
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}

	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, st := range steps {
		stepByRef[st.StepRef] = st
	}

	failedStep, ok := stepByRef[stepRun.StepRef]
	if !ok {
		return fmt.Errorf("step definition not found for %s", stepRun.StepRef)
	}

	policy := failedStep.OnFailure
	if policy == "" {
		policy = domain.FailWorkflow
	}

	switch policy {
	case domain.FailWorkflow:
		return s.failWorkflowAndCancel(ctx, wfRun, stepRun)
	case domain.SkipDependents:
		if err := s.skipDependentSteps(ctx, stepRun.WorkflowRunID, wfRun.WorkflowID, stepRun.StepRef); err != nil {
			return fmt.Errorf("skip dependents: %w", err)
		}
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID)
	case domain.Continue:
		if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
			return fmt.Errorf("fan-in for continue policy on step %s: %w", stepRun.StepRef, err)
		}
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID)
	default:
		return s.failWorkflowAndCancel(ctx, wfRun, stepRun)
	}
}

// failWorkflowAndCancel marks the workflow as failed, cancels remaining steps, and propagates to parent.
func (s *StepCallback) failWorkflowAndCancel(ctx context.Context, wfRun *domain.WorkflowRun, stepRun *domain.WorkflowStepRun) error {
	if wfRun.Status == domain.WfStatusRunning {
		now := time.Now()
		if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{"error": stepRun.Error, "finished_at": now}); err != nil {
			return fmt.Errorf("mark workflow failed: %w", err)
		}
		wfRun.Status = domain.WfStatusFailed
	}
	if err := s.cancelRemainingSteps(ctx, stepRun.WorkflowRunID); err != nil {
		return fmt.Errorf("cancel remaining steps: %w", err)
	}
	return s.propagateToParent(ctx, wfRun, nil)
}

func (s *StepCallback) cancelRemainingSteps(ctx context.Context, workflowRunID string) error {
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}

	now := time.Now()
	for _, sr := range stepRuns {
		if sr.Status.IsTerminal() {
			continue
		}
		if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepCanceled, map[string]any{"finished_at": now}); err != nil {
			return fmt.Errorf("cancel step run %s: %w", sr.ID, err)
		}
	}

	return nil
}

func (s *StepCallback) skipDependentSteps(ctx context.Context, workflowRunID, workflowID, failedStepRef string) error {
	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, workflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}

	dependents := make(map[string][]string, len(steps))
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			dependents[dep] = append(dependents[dep], step.StepRef)
		}
	}

	toSkip := make(map[string]struct{})
	queue := append([]string(nil), dependents[failedStepRef]...)
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]

		if _, seen := toSkip[ref]; seen {
			continue
		}
		toSkip[ref] = struct{}{}
		queue = append(queue, dependents[ref]...)
	}

	if len(toSkip) == 0 {
		return nil
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}

	now := time.Now()
	for _, sr := range stepRuns {
		if _, ok := toSkip[sr.StepRef]; !ok {
			continue
		}
		if sr.Status.IsTerminal() {
			continue
		}

		if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepSkipped, map[string]any{"finished_at": now}); err != nil {
			return fmt.Errorf("skip step run %s: %w", sr.ID, err)
		}
	}

	return nil
}
