package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"slices"
	"time"

	"strait/internal/domain"

	"github.com/samber/lo"
	otelattr "go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

func (s *StepCallback) fanInAndStartReadyChildren(ctx context.Context, stepRun *domain.WorkflowStepRun, wc *wfCtx) error {
	lockID := advisoryXactLockIDForStepRun(stepRun.ID)
	if err := s.store.AdvisoryXactLock(ctx, lockID); err != nil {
		return fmt.Errorf("advisory xact lock for step %s: %w", stepRun.StepRef, err)
	}

	if _, err := s.store.IncrementStepDeps(ctx, stepRun.WorkflowRunID, stepRun.StepRef); err != nil {
		return fmt.Errorf("increment step deps: %w", err)
	}

	// Re-read workflow run status after acquiring the lock to prevent a race
	// where pause commits between our initial cache load and lock acquisition.
	freshRun, freshErr := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if freshErr != nil {
		return fmt.Errorf("re-read workflow run status: %w", freshErr)
	}
	if freshRun.Status == domain.WfStatusPaused || freshRun.Status.IsTerminal() {
		return nil
	}

	stepStatuses, err := s.store.ListStepRunStatusesByWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("list step run statuses by workflow run: %w", err)
	}
	runningStepRuns, err := s.store.ListRunningStepRunsByWorkflowRun(ctx, stepRun.WorkflowRunID, 10000)
	if err != nil {
		return fmt.Errorf("list running step runs by workflow run: %w", err)
	}
	runnableStepRuns, err := s.store.ListRunnableStepRunsByWorkflowRun(ctx, stepRun.WorkflowRunID, 10000)
	if err != nil {
		return fmt.Errorf("list runnable step runs by workflow run: %w", err)
	}

	return s.scheduleRunnableSteps(ctx, wc.run, wc.steps, stepStatuses, runningStepRuns, runnableStepRuns)
}

func (s *StepCallback) scheduleRunnableSteps(
	ctx context.Context,
	wfRun *domain.WorkflowRun,
	steps []domain.WorkflowStep,
	stepStatuses map[string]domain.StepRunStatus,
	runningStepRuns []domain.WorkflowStepRun,
	runnableStepRuns []domain.WorkflowStepRun,
) error {
	stepByRef := lo.KeyBy(steps, func(st domain.WorkflowStep) string { return st.StepRef })
	runningStepCount := len(runningStepRuns)
	runningByConcurrencyKey := make(map[string]int)
	runningByResourceClass := make(map[string]int)
	for _, sr := range runningStepRuns {
		if sr.Status != domain.StepRunning {
			continue
		}
		if st, ok := stepByRef[sr.StepRef]; ok {
			if st.ConcurrencyKey != "" {
				runningByConcurrencyKey[st.ConcurrencyKey]++
			}
			runningByResourceClass[effectiveResourceClass(st.ResourceClass)]++
		}
	}

	for _, sr := range runnableStepRuns {
		if sr.Status.IsTerminal() || sr.Status == domain.StepRunning {
			continue
		}
		if sr.DepsCompleted != sr.DepsRequired {
			continue
		}

		stepDef, ok := stepByRef[sr.StepRef]
		if !ok {
			return fmt.Errorf("step definition not found for %s", sr.StepRef)
		}
		if wfRun.MaxParallelSteps > 0 && runningStepCount >= wfRun.MaxParallelSteps {
			s.recordDecision(ctx, &sr, "scheduler", "wait", "blocked by max_parallel_steps", nil)
			continue
		}
		if stepDef.ConcurrencyKey != "" && runningByConcurrencyKey[stepDef.ConcurrencyKey] > 0 {
			s.recordDecision(ctx, &sr, "concurrency", "wait", "blocked by concurrency_key", nil)
			continue
		}
		if !hasResourceClassCapacity(runningByResourceClass, stepDef.ResourceClass) {
			s.recordDecision(ctx, &sr, "resource", "wait", "blocked by resource_class quota", nil)
			continue
		}

		allowed, err := EvaluateCondition(stepDef.Condition, stepStatuses)
		if err != nil {
			return fmt.Errorf("evaluate condition for step %s: %w", stepDef.StepRef, err)
		}
		if !allowed {
			now := time.Now()
			s.recordDecision(ctx, &sr, "condition", "skip", "condition evaluated to false", stepDef.Condition)
			if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepSkipped, map[string]any{"finished_at": now}); err != nil {
				return fmt.Errorf("skip step %s: %w", stepDef.StepRef, err)
			}
			stepStatuses[sr.StepRef] = domain.StepSkipped
			continue
		}

		var parentOutputsPayload json.RawMessage
		if len(stepDef.DependsOn) > 0 {
			outputs, err := s.store.GetStepOutputs(ctx, sr.WorkflowRunID, stepDef.DependsOn)
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
			return fmt.Errorf("start runnable step %s: %w", stepDef.StepRef, err)
		}
		stepStatuses[sr.StepRef] = srCopy.Status
		if srCopy.Status == domain.StepRunning {
			s.recordStepWaitDuration(ctx, wfRun, stepCopy, sr)
			runningStepCount++
			if stepCopy.ConcurrencyKey != "" {
				runningByConcurrencyKey[stepCopy.ConcurrencyKey]++
			}
			runningByResourceClass[effectiveResourceClass(stepCopy.ResourceClass)]++
		}
	}

	return nil
}

func advisoryXactLockIDForStepRun(stepRunID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(stepRunID))
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

func (s *StepCallback) recordStepWaitDuration(ctx context.Context, wfRun *domain.WorkflowRun, step domain.WorkflowStep, stepRun domain.WorkflowStepRun) {
	if s.metrics == nil {
		return
	}
	if stepRun.CreatedAt.IsZero() {
		return
	}
	wait := time.Since(stepRun.CreatedAt).Seconds()
	if wait < 0 {
		wait = 0
	}
	attrs := otelmetric.WithAttributes(
		otelattr.String("workflow_id", wfRun.WorkflowID),
		otelattr.String("workflow_run_id", wfRun.ID),
		otelattr.String("step_ref", step.StepRef),
	)
	s.metrics.WorkflowStepWaitDuration.Record(ctx, wait, attrs)
}

func (s *StepCallback) checkWorkflowCompletion(ctx context.Context, workflowRunID string, wc *wfCtx) error {
	nonTerminalCount, err := s.store.CountNonTerminalStepRuns(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("count non-terminal step runs: %w", err)
	}
	if nonTerminalCount > 0 {
		return nil
	}

	// Re-fetch the workflow run for fresh terminal status check (concurrent completions can race).
	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun.Status.IsTerminal() {
		return nil
	}
	// Update wc.run so downstream (propagateToParent) sees the latest state.
	wc.run = wfRun

	policyByRef := lo.Associate(wc.steps, func(step domain.WorkflowStep) (string, domain.FailurePolicy) {
		return step.StepRef, step.OnFailure
	})

	failedStepRefs, err := s.store.ListFailedStepRunRefs(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list failed step refs: %w", err)
	}

	hasFailingStep := false
	for _, stepRef := range failedStepRefs {
		if policyByRef[stepRef] != domain.Continue {
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
		if wfRun.ParentWorkflowRunID != "" {
			stepRuns, listErr := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
			if listErr != nil {
				return fmt.Errorf("list step runs: %w", listErr)
			}
			return s.propagateToParent(ctx, wfRun, stepRuns)
		}
		return nil
	}

	if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCompleted, map[string]any{"finished_at": now}); err != nil {
		return fmt.Errorf("mark workflow run completed: %w", err)
	}
	wfRun.Status = domain.WfStatusCompleted
	if wfRun.ParentWorkflowRunID != "" {
		stepRuns, listErr := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
		if listErr != nil {
			return fmt.Errorf("list step runs: %w", listErr)
		}
		return s.propagateToParent(ctx, wfRun, stepRuns)
	}
	return nil
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

	var parentStepRun *domain.WorkflowStepRun
	if childRun.ParentStepRunID != "" {
		parentStepRun, err = s.store.GetWorkflowStepRun(ctx, childRun.ParentStepRunID)
		if err != nil {
			return fmt.Errorf("get parent step run by id for sub-workflow: %w", err)
		}
	}

	if parentStepRun == nil {
		// Backward-compatible fallback for runs created before parent_step_run_id existed.
		parentSteps, listErr := s.loadStepDefinitions(ctx, parentRun)
		if listErr != nil {
			return fmt.Errorf("load parent step definitions: %w", listErr)
		}

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

		parentStepRun, err = s.store.GetStepRunByWorkflowRunAndRef(ctx, parentRun.ID, matchingStepRef)
		if err != nil {
			return fmt.Errorf("get parent step run for sub-workflow fallback: %w", err)
		}
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
			outputs := lo.Associate(
				lo.Filter(childStepRuns, func(sr domain.WorkflowStepRun, _ int) bool { return len(sr.Output) > 0 }),
				func(sr domain.WorkflowStepRun) (string, json.RawMessage) { return sr.StepRef, sr.Output },
			)
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

		parentWc, wcErr := s.loadWfCtx(ctx, parentRun.ID)
		if wcErr != nil {
			return fmt.Errorf("load parent workflow context: %w", wcErr)
		}
		if err := s.fanInAndStartReadyChildren(ctx, parentStepRun, parentWc); err != nil {
			return fmt.Errorf("fan-in after sub-workflow completion for step %s: %w", parentStepRun.StepRef, err)
		}
		return s.checkWorkflowCompletion(ctx, parentRun.ID, parentWc)

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

		parentWc, wcErr := s.loadWfCtx(ctx, parentRun.ID)
		if wcErr != nil {
			return fmt.Errorf("load parent workflow context: %w", wcErr)
		}
		return s.handleFailedStep(ctx, parentStepRun, parentWc)

	default:
		// Canceled or timed_out child workflows fail the parent step.
		errMsg := fmt.Sprintf("sub-workflow %s ended with status %s", childRun.WorkflowID, childRun.Status)
		fields := map[string]any{"finished_at": now, "error": errMsg}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepFailed, fields); err != nil {
			return fmt.Errorf("fail parent step run for sub-workflow: %w", err)
		}
		parentStepRun.Status = domain.StepFailed
		parentStepRun.Error = errMsg

		parentWc, wcErr := s.loadWfCtx(ctx, parentRun.ID)
		if wcErr != nil {
			return fmt.Errorf("load parent workflow context: %w", wcErr)
		}
		return s.handleFailedStep(ctx, parentStepRun, parentWc)
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

	// Empty approvers list means any authenticated user can approve (e.g. cost gates).
	if len(approval.Approvers) > 0 && !slices.Contains(approval.Approvers, approver) {
		return fmt.Errorf("approver %s is not allowed for step %s", approver, stepRef)
	}

	now := time.Now()
	if err := s.store.UpdateWorkflowStepApproval(ctx, approval.ID, domain.ApprovalStatusApproved, approver, &now, ""); err != nil {
		return fmt.Errorf("update approval: %w", err)
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepCompleted, map[string]any{"finished_at": now}); err != nil {
		return fmt.Errorf("complete approval step run: %w", err)
	}

	// Sync parallel event trigger (if exists) — non-fatal.
	if trigger, getErr := s.store.GetEventTriggerByStepRunID(ctx, stepRun.ID); getErr == nil && trigger != nil && trigger.Status == domain.EventTriggerStatusWaiting {
		if syncErr := s.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, nil, &now, ""); syncErr != nil {
			s.logger.Warn("failed to sync event trigger for approval (non-fatal)", "step_run_id", stepRun.ID, "error", syncErr)
		}
	}

	stepRun.Status = domain.StepCompleted
	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		return fmt.Errorf("load workflow context: %w", wcErr)
	}

	if s.engine != nil {
		s.engine.enqueueApprovalNotification(ctx, wc.run.ProjectID,
			domain.NotificationEventApprovalCompleted, map[string]any{
				"approval_id":     approval.ID,
				"workflow_run_id": wc.run.ID,
				"workflow_id":     wc.run.WorkflowID,
				"step_ref":        stepRun.StepRef,
				"approved_by":     approver,
				"approved_at":     now,
			})
	}

	if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc); err != nil {
		return fmt.Errorf("fan-in after approval for step %s: %w", stepRef, err)
	}

	return s.checkWorkflowCompletion(ctx, workflowRunID, wc)
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
	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		return fmt.Errorf("load workflow context: %w", wcErr)
	}

	// Transition any pending approval to rejected so the reaper does not
	// pick it up later as timed_out.
	if approval, aprErr := s.store.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID); aprErr == nil && approval != nil && approval.Status == domain.ApprovalStatusPending {
		if updErr := s.store.UpdateWorkflowStepApproval(ctx, approval.ID, domain.ApprovalStatusRejected, "", nil, reason); updErr != nil {
			s.logger.Warn("failed to reject approval on skip (non-fatal)", "approval_id", approval.ID, "error", updErr)
		} else if s.engine != nil {
			s.engine.enqueueApprovalNotification(ctx, wc.run.ProjectID,
				domain.NotificationEventApprovalRejected, map[string]any{
					"approval_id":     approval.ID,
					"workflow_run_id": wc.run.ID,
					"workflow_id":     wc.run.WorkflowID,
					"step_ref":        stepRun.StepRef,
					"rejected_by":     "skip",
					"reason":          reason,
				})
		}
	}

	if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc); err != nil {
		return fmt.Errorf("fan-in after skip: %w", err)
	}
	return s.checkWorkflowCompletion(ctx, workflowRunID, wc)
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
	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		return fmt.Errorf("load workflow context: %w", wcErr)
	}

	if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc); err != nil {
		return fmt.Errorf("fan-in after force-complete: %w", err)
	}
	return s.checkWorkflowCompletion(ctx, workflowRunID, wc)
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

	// Re-enqueue job runs that were paused (containers stopped).
	requeueCount, requeueErr := s.store.RequeuePausedJobRuns(ctx, workflowRunID)
	if requeueErr != nil {
		return fmt.Errorf("requeue paused job runs: %w", requeueErr)
	}
	if requeueCount > 0 {
		s.logger.Info("requeued paused job runs on resume",
			"workflow_run_id", workflowRunID, "count", requeueCount)
	}

	steps, err := s.loadStepDefinitions(ctx, wfRun)
	if err != nil {
		return fmt.Errorf("load step definitions: %w", err)
	}

	stepStatuses, err := s.store.ListStepRunStatusesByWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list step run statuses by workflow run: %w", err)
	}
	runningStepRuns, err := s.store.ListRunningStepRunsByWorkflowRun(ctx, workflowRunID, 10000)
	if err != nil {
		return fmt.Errorf("list running step runs by workflow run: %w", err)
	}
	runnableStepRuns, err := s.store.ListRunnableStepRunsByWorkflowRun(ctx, workflowRunID, 10000)
	if err != nil {
		return fmt.Errorf("list runnable step runs by workflow run: %w", err)
	}

	if err := s.scheduleRunnableSteps(ctx, wfRun, steps, stepStatuses, runningStepRuns, runnableStepRuns); err != nil {
		return fmt.Errorf("schedule runnable steps: %w", err)
	}

	return nil
}

func effectiveResourceClass(v string) string {
	if v == "" {
		return "small"
	}
	return v
}

func hasResourceClassCapacity(running map[string]int, class string) bool {
	limits := map[string]int{"small": 50, "medium": 20, "large": 5}
	resolved := effectiveResourceClass(class)
	limit, ok := limits[resolved]
	if !ok {
		limit = limits["small"]
		resolved = "small"
	}
	return running[resolved] < limit
}
