package workflow

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/worker"
)

// stepRetryTransactioner is the subset of the store that can wrap operations in
// a single database transaction. *store.Queries satisfies this interface; the
// mock used in unit tests does not, which causes scheduleStepRetry to fall back
// to sequential (non-transactional) execution in those tests.
//
// True transactionality is verified in integration tests that use a real
// PostgreSQL connection.
type stepRetryTransactioner interface {
	WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error
}

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
	if tx, ok := s.store.(stepRetryTransactioner); ok {
		return tx.WithTx(ctx, func(txCtx context.Context, _ store.DBTX) error {
			return s.scheduleStepRetryWrites(txCtx, jobRun, stepRun, nextRetryAt, newAttempt)
		})
	}
	// Fallback for tests: the mock store does not implement stepRetryTransactioner,
	// so writes execute sequentially without a transaction wrapper.
	return s.scheduleStepRetryWrites(ctx, jobRun, stepRun, nextRetryAt, newAttempt)
}

// scheduleStepRetryWrites performs the three DB writes that record a step retry.
// It must be called inside a transaction (via scheduleStepRetry) to guarantee
// atomicity: if the process crashes between writes, the attempt counter and the
// job_retries row would be inconsistent, permanently stalling the step.
func (s *StepCallback) scheduleStepRetryWrites(ctx context.Context, jobRun *domain.JobRun, stepRun *domain.WorkflowStepRun, nextRetryAt time.Time, newAttempt int) error {
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
		if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc, true); err != nil {
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
		wfRun.FinishedAt = &now
		s.publishWorkflowRunStatus(ctx, wfRun, domain.WfStatusRunning, domain.WfStatusFailed, "step_failed")
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
	refs := dependentStepRefs(wc.steps, wc.stepIndex, failedStepRef)
	if len(refs) == 0 {
		return nil
	}
	now := time.Now()
	if _, err := s.store.SkipStepRunsByRefs(ctx, workflowRunID, refs, now); err != nil {
		return fmt.Errorf("skip step runs by refs: %w", err)
	}

	return nil
}

func dependentStepRefs(steps []domain.WorkflowStep, stepIndex map[string]int, failedStepRef string) []string {
	if len(steps) == 0 {
		return nil
	}

	if stepIndex == nil {
		stepIndex = make(map[string]int, len(steps))
		for i := range steps {
			stepIndex[steps[i].StepRef] = i
		}
	}
	failedIdx, ok := stepIndex[failedStepRef]
	if !ok {
		return dependentStepRefsByMap(steps, failedStepRef)
	}
	if refs, ok := dependentStepRefsLinearChain(steps, failedIdx); ok {
		return refs
	}
	if refs, ok := dependentStepRefsRootFanOut(steps, failedIdx); ok {
		return refs
	}

	childCounts := make([]int, len(steps))
	totalEdges := 0
	for i := range steps {
		for _, dep := range steps[i].DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			childCounts[depIdx]++
			totalEdges++
		}
	}
	if childCounts[failedIdx] == 0 {
		return nil
	}

	children := make([][]int, len(steps))
	edgeStorage := make([]int, totalEdges)
	offset := 0
	for i, count := range childCounts {
		children[i] = edgeStorage[offset : offset : offset+count]
		offset += count
	}
	for stepIdx := range steps {
		for _, dep := range steps[stepIdx].DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			children[depIdx] = append(children[depIdx], stepIdx)
		}
	}

	skipped := make([]bool, len(steps))
	queue := make([]int, 0, len(steps)-1)
	queue = append(queue, children[failedIdx]...)
	refs := make([]string, 0, len(steps)-1)
	for head := 0; head < len(queue); head++ {
		stepIdx := queue[head]
		if skipped[stepIdx] {
			continue
		}
		skipped[stepIdx] = true
		refs = append(refs, steps[stepIdx].StepRef)
		queue = append(queue, children[stepIdx]...)
	}

	return refs
}

func dependentStepRefsLinearChain(steps []domain.WorkflowStep, failedIdx int) ([]string, bool) {
	if failedIdx < 0 || failedIdx >= len(steps) {
		return nil, false
	}
	if len(steps[0].DependsOn) != 0 {
		return nil, false
	}
	for i := 1; i < len(steps); i++ {
		deps := steps[i].DependsOn
		if len(deps) != 1 || deps[0] != steps[i-1].StepRef {
			return nil, false
		}
	}
	if failedIdx == len(steps)-1 {
		return nil, true
	}

	refs := make([]string, 0, len(steps)-failedIdx-1)
	for i := failedIdx + 1; i < len(steps); i++ {
		refs = append(refs, steps[i].StepRef)
	}
	return refs, true
}

func dependentStepRefsRootFanOut(steps []domain.WorkflowStep, failedIdx int) ([]string, bool) {
	if failedIdx < 0 || failedIdx >= len(steps) {
		return nil, false
	}
	if failedIdx != 0 || len(steps[0].DependsOn) != 0 {
		return nil, false
	}
	failedStepRef := steps[failedIdx].StepRef
	for i := 1; i < len(steps); i++ {
		deps := steps[i].DependsOn
		if len(deps) != 1 || deps[0] != failedStepRef {
			return nil, false
		}
	}

	refs := make([]string, 0, len(steps)-1)
	for i := 1; i < len(steps); i++ {
		refs = append(refs, steps[i].StepRef)
	}
	return refs, true
}

func dependentStepRefsByMap(steps []domain.WorkflowStep, failedStepRef string) []string {
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

	refs := make([]string, 0, len(toSkip))
	for ref := range toSkip {
		refs = append(refs, ref)
	}
	return refs
}
