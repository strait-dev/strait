package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"strait/internal/domain"
	storepkg "strait/internal/store"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
	otelattr "go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

var stepQueueDuration otelmetric.Float64Histogram

// init registers the workflow step queue metric as a package-level instrument.
// The metric is emitted from hot callback paths, so callers should not pay a
// per-callback lookup cost.
func init() {
	meter := otel.Meter("strait/workflow")
	stepQueueDuration, _ = meter.Float64Histogram(
		"strait_workflow_step_queue_seconds",
		otelmetric.WithDescription("Time between step run creation and scheduling"),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 5, 10, 30, 60, 300),
	)
}

func (s *StepCallback) fanInAndStartReadyChildren(ctx context.Context, stepRun *domain.WorkflowStepRun, wc *wfCtx, failedCountsAsResolved bool) error {
	lockID := advisoryXactLockIDForStepRun(stepRun.ID)
	if err := s.store.AdvisoryXactLock(ctx, lockID); err != nil {
		return fmt.Errorf("advisory xact lock for step %s: %w", stepRun.StepRef, err)
	}

	var stepDepsErr error
	if failedCountsAsResolved {
		_, stepDepsErr = s.store.IncrementStepDepsIncludingFailed(ctx, stepRun.WorkflowRunID, stepRun.StepRef)
	} else {
		_, stepDepsErr = s.store.IncrementStepDeps(ctx, stepRun.WorkflowRunID, stepRun.StepRef)
	}
	if stepDepsErr != nil {
		return fmt.Errorf("increment step deps: %w", stepDepsErr)
	}

	// Re-read workflow run status after acquiring the lock to prevent a race
	// where pause commits between our initial cache load and lock acquisition.
	freshRun, freshErr := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if freshErr != nil {
		return fmt.Errorf("re-read workflow run status: %w", freshErr)
	}
	if workflowProgressionShouldStop(freshRun.Status) {
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

func (s *StepCallback) fanInBatchAndStartReadyChildren(ctx context.Context, workflowRunID string, completedStepRefs []string, wc *wfCtx) error {
	if len(completedStepRefs) == 0 {
		return nil
	}
	lockID := advisoryXactLockIDForStepRun("workflow:" + workflowRunID)
	if err := s.store.AdvisoryXactLock(ctx, lockID); err != nil {
		return fmt.Errorf("advisory xact lock for workflow %s: %w", workflowRunID, err)
	}

	if batchStore, ok := s.store.(interface {
		IncrementStepDepsBatch(context.Context, string, []string) ([]storepkg.StepDepResult, error)
	}); ok {
		if _, err := batchStore.IncrementStepDepsBatch(ctx, workflowRunID, completedStepRefs); err != nil {
			return fmt.Errorf("increment step deps batch: %w", err)
		}
	} else {
		for _, completedRef := range completedStepRefs {
			if _, err := s.store.IncrementStepDeps(ctx, workflowRunID, completedRef); err != nil {
				return fmt.Errorf("increment step deps: %w", err)
			}
		}
	}

	freshRun, freshErr := s.store.GetWorkflowRun(ctx, workflowRunID)
	if freshErr != nil {
		return fmt.Errorf("re-read workflow run status: %w", freshErr)
	}
	if workflowProgressionShouldStop(freshRun.Status) {
		return nil
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

	return s.scheduleRunnableSteps(ctx, freshRun, wc.steps, stepStatuses, runningStepRuns, runnableStepRuns)
}

func workflowProgressionShouldStop(status domain.WorkflowRunStatus) bool {
	return status == domain.WfStatusPaused || status.IsTerminal()
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
	prefetchedOutputs, err := s.prefetchStepOutputs(ctx, wfRun.ID, runnableStepRuns, stepByRef)
	if err != nil {
		return err
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
			recordWorkflowStepTransition(ctx, string(sr.Status), string(domain.StepSkipped))
			recordWorkflowStepDuration(ctx, string(stepDef.StepType), workflowStepOutcome(domain.StepSkipped), sr.StartedAt, now)
			stepStatuses[sr.StepRef] = domain.StepSkipped
			continue
		}

		var parentOutputsPayload json.RawMessage
		if len(stepDef.DependsOn) > 0 && prefetchedOutputs != nil {
			stepOutputs := make(map[string]json.RawMessage, len(stepDef.DependsOn))
			for _, dep := range stepDef.DependsOn {
				if out, ok := prefetchedOutputs[dep]; ok {
					stepOutputs[dep] = out
				}
			}
			payload, err := json.Marshal(stepOutputs)
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
			recordStepQueueDuration(ctx, stepDef.StepRef, sr.CreatedAt)
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
	nonTerminalCount, failedStepRefs, err := s.workflowCompletionCounts(ctx, workflowRunID)
	if err != nil {
		return err
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

	now := time.Now()
	if hasBlockingFailedStep(wc.steps, failedStepRefs) {
		fromStatus := wfRun.Status
		if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusFailed, map[string]any{"finished_at": now}); err != nil {
			return fmt.Errorf("mark workflow run failed: %w", err)
		}
		recordWorkflowActiveRunDelta(ctx, wfRun.ProjectID, -1)
		wfRun.Status = domain.WfStatusFailed
		wfRun.FinishedAt = &now
		s.publishWorkflowRunStatus(ctx, wfRun, fromStatus, domain.WfStatusFailed, "workflow_completion")
		if wfRun.ParentWorkflowRunID != "" {
			stepRuns, listErr := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
			if listErr != nil {
				return fmt.Errorf("list step runs: %w", listErr)
			}
			return s.propagateToParent(ctx, wfRun, stepRuns)
		}
		return nil
	}

	fromStatus := wfRun.Status
	if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCompleted, map[string]any{"finished_at": now}); err != nil {
		return fmt.Errorf("mark workflow run completed: %w", err)
	}
	recordWorkflowActiveRunDelta(ctx, wfRun.ProjectID, -1)
	wfRun.Status = domain.WfStatusCompleted
	wfRun.FinishedAt = &now
	s.publishWorkflowRunStatus(ctx, wfRun, fromStatus, domain.WfStatusCompleted, "workflow_completion")
	if wfRun.ParentWorkflowRunID != "" {
		stepRuns, listErr := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID, 10000, nil)
		if listErr != nil {
			return fmt.Errorf("list step runs: %w", listErr)
		}
		return s.propagateToParent(ctx, wfRun, stepRuns)
	}
	return nil
}

func (s *StepCallback) workflowCompletionCounts(ctx context.Context, workflowRunID string) (int, []string, error) {
	if summaryStore, ok := s.store.(interface {
		GetWorkflowStepCompletionSummary(context.Context, string) (storepkg.WorkflowStepCompletionSummary, error)
	}); ok {
		summary, err := summaryStore.GetWorkflowStepCompletionSummary(ctx, workflowRunID)
		if err != nil {
			return 0, nil, fmt.Errorf("get workflow step completion summary: %w", err)
		}
		return summary.NonTerminalCount, summary.FailedStepRefs, nil
	}
	nonTerminalCount, err := s.store.CountNonTerminalStepRuns(ctx, workflowRunID)
	if err != nil {
		return 0, nil, fmt.Errorf("count non-terminal step runs: %w", err)
	}
	if nonTerminalCount > 0 {
		return nonTerminalCount, nil, nil
	}
	failedStepRefs, err := s.store.ListFailedStepRunRefs(ctx, workflowRunID)
	if err != nil {
		return 0, nil, fmt.Errorf("list failed step refs: %w", err)
	}
	return nonTerminalCount, failedStepRefs, nil
}

func hasBlockingFailedStep(steps []domain.WorkflowStep, failedStepRefs []string) bool {
	switch len(failedStepRefs) {
	case 0:
		return false
	case 1:
		return failurePolicyForStepRef(steps, failedStepRefs[0]) != domain.Continue
	}

	failedRefs := make(map[string]struct{}, len(failedStepRefs))
	for _, stepRef := range failedStepRefs {
		failedRefs[stepRef] = struct{}{}
	}
	for _, step := range steps {
		if _, failed := failedRefs[step.StepRef]; failed {
			if step.OnFailure != domain.Continue {
				return true
			}
			delete(failedRefs, step.StepRef)
		}
	}
	return len(failedRefs) > 0
}

func failurePolicyForStepRef(steps []domain.WorkflowStep, stepRef string) domain.FailurePolicy {
	for _, step := range steps {
		if step.StepRef == stepRef {
			return step.OnFailure
		}
	}
	return ""
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
		outputPayload := aggregateChildStepOutputs(childStepRuns)

		fields := map[string]any{"finished_at": now}
		if len(outputPayload) > 0 {
			fields["output"] = outputPayload
		}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepCompleted, fields); err != nil {
			return fmt.Errorf("complete parent step run for sub-workflow: %w", err)
		}
		recordSubWorkflowStepTerminal(ctx, parentStepRun, domain.StepCompleted, now)
		parentStepRun.Status = domain.StepCompleted
		if len(outputPayload) > 0 {
			parentStepRun.Output = outputPayload
		}

		parentWc, wcErr := s.loadWfCtx(ctx, parentRun.ID)
		if wcErr != nil {
			return fmt.Errorf("load parent workflow context: %w", wcErr)
		}
		if err := s.fanInAndStartReadyChildren(ctx, parentStepRun, parentWc, false); err != nil {
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
		recordSubWorkflowStepTerminal(ctx, parentStepRun, domain.StepFailed, now)
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
		recordSubWorkflowStepTerminal(ctx, parentStepRun, domain.StepFailed, now)
		parentStepRun.Status = domain.StepFailed
		parentStepRun.Error = errMsg

		parentWc, wcErr := s.loadWfCtx(ctx, parentRun.ID)
		if wcErr != nil {
			return fmt.Errorf("load parent workflow context: %w", wcErr)
		}
		return s.handleFailedStep(ctx, parentStepRun, parentWc)
	}
}

func aggregateChildStepOutputs(childStepRuns []domain.WorkflowStepRun) json.RawMessage {
	if len(childStepRuns) == 0 {
		return nil
	}
	outputCount := 0
	size := 2
	for i := range childStepRuns {
		if len(childStepRuns[i].Output) == 0 {
			continue
		}
		if outputCount > 0 {
			size++
		}
		size += len(childStepRuns[i].StepRef) + len(childStepRuns[i].Output) + 3
		outputCount++
	}
	if outputCount == 0 {
		return nil
	}

	outputPayload := make([]byte, 0, size)
	outputPayload = append(outputPayload, '{')
	wroteOutput := false
	for i := range childStepRuns {
		if len(childStepRuns[i].Output) == 0 {
			continue
		}
		if wroteOutput {
			outputPayload = append(outputPayload, ',')
		}
		outputPayload = appendJSONKey(outputPayload, childStepRuns[i].StepRef)
		outputPayload = append(outputPayload, ':')
		outputPayload = append(outputPayload, childStepRuns[i].Output...)
		wroteOutput = true
	}
	outputPayload = append(outputPayload, '}')
	return outputPayload
}

func appendJSONKey(dst []byte, key string) []byte {
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c < 0x20 || c == '\\' || c == '"' || c >= utf8.RuneSelf {
			return strconv.AppendQuote(dst, key)
		}
	}
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"')
	return dst
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
	recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepCompleted))
	recordWorkflowDurableWait(ctx, stepRun.StartedAt, now)

	// Sync parallel event trigger (if exists) — non-fatal.
	trigger, getErr := s.store.GetEventTriggerByStepRunID(ctx, stepRun.ID)
	triggerWaiting := getErr == nil && trigger != nil && trigger.Status == domain.EventTriggerStatusWaiting
	if triggerWaiting {
		if syncErr := s.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, nil, &now, ""); syncErr != nil {
			s.logger.Warn("failed to sync event trigger for approval (non-fatal)", "step_run_id", stepRun.ID, "error", syncErr)
		}
	}

	stepRun.Status = domain.StepCompleted
	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		return fmt.Errorf("load workflow context: %w", wcErr)
	}
	recordWorkflowStepDuration(ctx, workflowStepKind(wc, stepRun), workflowStepOutcome(domain.StepCompleted), stepRun.StartedAt, now)

	s.emitApprovalAuditEvent(ctx, wc.run, stepRun, approval, approver, "workflow.step.approved", "approved", "")

	if s.engine != nil {
		s.engine.enqueueApprovalNotification(ctx, wc.run.ProjectID,
			domain.NotificationEventApprovalCompleted, map[string]any{
				"approval_id":     approval.ID,
				"decision":        "approved",
				"workflow_run_id": wc.run.ID,
				"workflow_id":     wc.run.WorkflowID,
				"step_ref":        stepRun.StepRef,
				"approved_by":     approver,
				"approved_at":     now,
			})
	}

	if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc, false); err != nil {
		return fmt.Errorf("fan-in after approval for step %s: %w", stepRef, err)
	}

	return s.checkWorkflowCompletion(ctx, workflowRunID, wc)
}

func (s *StepCallback) SkipStep(ctx context.Context, workflowRunID, stepRef, reason, actor string) error {
	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
	if err != nil {
		return fmt.Errorf("get step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("step run not found for %s", stepRef)
	}
	if !isPendingOrWaitingStepStatus(stepRun.Status) {
		return fmt.Errorf("cannot skip step in %s status", stepRun.Status)
	}

	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		return fmt.Errorf("load workflow context: %w", wcErr)
	}

	now := time.Now()

	// Reject any pending approval before marking the step as skipped so
	// both writes must succeed atomically — if the approval rejection
	// fails the step stays in its current state and the caller can retry.
	approval, aprErr := s.store.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	if aprErr != nil {
		return fmt.Errorf("get workflow step approval: %w", aprErr)
	}
	if approval != nil && approval.Status == domain.ApprovalStatusPending {
		rejectedBy := actor
		if rejectedBy == "" {
			rejectedBy = "skip"
		}
		if updErr := s.store.UpdateWorkflowStepApproval(ctx, approval.ID, domain.ApprovalStatusRejected, rejectedBy, &now, reason); updErr != nil {
			return fmt.Errorf("reject approval on skip: %w", updErr)
		}
		s.emitApprovalAuditEvent(ctx, wc.run, stepRun, approval, actor, "workflow.step.rejected", "rejected", reason)
		if s.engine != nil {
			s.engine.enqueueApprovalNotification(ctx, wc.run.ProjectID,
				domain.NotificationEventApprovalCompleted, map[string]any{
					"approval_id":     approval.ID,
					"decision":        "rejected",
					"workflow_run_id": wc.run.ID,
					"workflow_id":     wc.run.WorkflowID,
					"step_ref":        stepRun.StepRef,
					"rejected_by":     rejectedBy,
					"rejected_at":     now,
					"reason":          reason,
				})
		}
	}

	fields := map[string]any{"finished_at": now}
	if reason != "" {
		fields["error"] = reason
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepSkipped, fields); err != nil {
		return fmt.Errorf("skip step: %w", err)
	}
	recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepSkipped))
	recordWorkflowStepDuration(ctx, workflowStepKind(wc, stepRun), workflowStepOutcome(domain.StepSkipped), stepRun.StartedAt, now)
	stepRun.Status = domain.StepSkipped

	if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc, false); err != nil {
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
	if !isPendingOrWaitingStepStatus(stepRun.Status) {
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
	recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepCompleted))
	stepRun.Status = domain.StepCompleted
	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		return fmt.Errorf("load workflow context: %w", wcErr)
	}
	recordWorkflowStepDuration(ctx, workflowStepKind(wc, stepRun), workflowStepOutcome(domain.StepCompleted), stepRun.StartedAt, now)

	if err := s.fanInAndStartReadyChildren(ctx, stepRun, wc, false); err != nil {
		return fmt.Errorf("fan-in after force-complete: %w", err)
	}
	return s.checkWorkflowCompletion(ctx, workflowRunID, wc)
}

func isPendingOrWaitingStepStatus(status domain.StepRunStatus) bool {
	switch status {
	case domain.StepPending, domain.StepWaiting:
		return true
	default:
		return false
	}
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
	requeueCount, requeueErr := s.requeuePausedJobRuns(ctx, workflowRunID)
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

func (s *StepCallback) requeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error) {
	if s.engine != nil {
		if requeuer, ok := s.engine.queue.(pausedRunQueueRequeuer); ok {
			return requeuer.RequeuePausedJobRuns(ctx, workflowRunID)
		}
	}
	return s.store.RequeuePausedJobRuns(ctx, workflowRunID)
}

func effectiveResourceClass(v string) string {
	if v == "" {
		return "small"
	}
	return v
}

func hasResourceClassCapacity(running map[string]int, class string) bool {
	resolved := effectiveResourceClass(class)
	limit := 50
	switch resolved {
	case "medium":
		limit = 20
	case "large":
		limit = 5
	case "small":
	default:
		resolved = "small"
	}
	return running[resolved] < limit
}

// prefetchStepOutputs batches all dependency output fetches into one query.
func (s *StepCallback) prefetchStepOutputs(
	ctx context.Context,
	workflowRunID string,
	runnableStepRuns []domain.WorkflowStepRun,
	stepByRef map[string]domain.WorkflowStep,
) (map[string]json.RawMessage, error) {
	allDeps := make(map[string]struct{})
	for _, sr := range runnableStepRuns {
		if sr.Status.IsTerminal() || sr.Status == domain.StepRunning {
			continue
		}
		if stepDef, ok := stepByRef[sr.StepRef]; ok {
			for _, dep := range stepDef.DependsOn {
				allDeps[dep] = struct{}{}
			}
		}
	}
	if len(allDeps) == 0 {
		return nil, nil
	}
	depRefs := make([]string, 0, len(allDeps))
	for dep := range allDeps {
		depRefs = append(depRefs, dep)
	}
	outputs, err := s.store.GetStepOutputs(ctx, workflowRunID, depRefs)
	if err != nil {
		return nil, fmt.Errorf("prefetch step outputs: %w", err)
	}
	return outputs, nil
}

func recordStepQueueDuration(ctx context.Context, stepRef string, createdAt time.Time) {
	if stepQueueDuration == nil || createdAt.IsZero() {
		return
	}
	if qd := time.Since(createdAt).Seconds(); qd > 0 {
		stepQueueDuration.Record(ctx, qd, otelmetric.WithAttributes(
			otelattr.String("step_ref", stepRef),
		))
	}
}
