package workflow

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/worker"
)

func (s *StepCallback) checkStepRetry(_ context.Context, stepRun *domain.WorkflowStepRun, _ *domain.JobRun, wc *wfCtx) (bool, time.Time, int, error) {
	failedStep, ok := wc.stepByRef[stepRun.StepRef]
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
		"attempt": newAttempt,
	}
	if err := s.store.ScheduleRetry(ctx, jobRun.ID, nextRetryAt, newAttempt); err != nil {
		return fmt.Errorf("schedule step retry: %w", err)
	}
	if err := s.store.UpdateRunStatus(ctx, jobRun.ID, jobRun.Status, domain.StatusDelayed, fields); err != nil {
		return fmt.Errorf("update job run status for retry: %w", err)
	}

	return nil
}

func (s *StepCallback) handleFailedStep(ctx context.Context, stepRun *domain.WorkflowStepRun, wc *wfCtx) error {
	failedStep, ok := wc.stepByRef[stepRun.StepRef]
	if !ok {
		return fmt.Errorf("step definition not found for %s", stepRun.StepRef)
	}

	policy := failedStep.OnFailure
	if policy == "" {
		policy = domain.FailWorkflow
	}

	switch policy {
	case domain.FailWorkflow:
		return s.failWorkflowAndCancel(ctx, wc.run, stepRun)
	case domain.SkipDependents:
		if err := s.skipDependentSteps(ctx, stepRun.WorkflowRunID, wc, stepRun.StepRef); err != nil {
			return fmt.Errorf("skip dependents: %w", err)
		}
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID, wc)
	case domain.Continue:
		if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc); err != nil {
			return fmt.Errorf("fan-in for continue policy on step %s: %w", stepRun.StepRef, err)
		}
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID, wc)
	default:
		return s.failWorkflowAndCancel(ctx, wc.run, stepRun)
	}
}

// failWorkflowAndCancel marks the workflow as failed, cancels remaining steps, and propagates to parent.
func (s *StepCallback) failWorkflowAndCancel(ctx context.Context, wfRun *domain.WorkflowRun, stepRun *domain.WorkflowStepRun) error {
	if wfRun.Status == domain.WfStatusRunning {
		now := time.Now()
		if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{"error": stepRun.Error, "finished_at": now}); err != nil {
			return fmt.Errorf("mark workflow failed: %w", err)
		}
		recordWorkflowActiveRunDelta(ctx, wfRun.ProjectID, -1)
		wfRun.Status = domain.WfStatusFailed
	}
	if err := s.cancelRemainingSteps(ctx, stepRun.WorkflowRunID); err != nil {
		return fmt.Errorf("cancel remaining steps: %w", err)
	}
	return s.propagateToParent(ctx, wfRun, nil)
}

func (s *StepCallback) cancelRemainingSteps(ctx context.Context, workflowRunID string) error {
	now := time.Now()
	if _, err := s.store.CancelNonTerminalStepRuns(ctx, workflowRunID, now, ""); err != nil {
		return fmt.Errorf("cancel non-terminal step runs: %w", err)
	}
	return nil
}

func (s *StepCallback) skipDependentSteps(ctx context.Context, workflowRunID string, wc *wfCtx, failedStepRef string) error {
	dependents := make(map[string][]string, len(wc.steps))
	for _, step := range wc.steps {
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

	refs := make([]string, 0, len(toSkip))
	for ref := range toSkip {
		refs = append(refs, ref)
	}

	now := time.Now()
	if _, err := s.store.SkipStepRunsByRefs(ctx, workflowRunID, refs, now); err != nil {
		return fmt.Errorf("skip step runs by refs: %w", err)
	}

	return nil
}
