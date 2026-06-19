package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"go.opentelemetry.io/otel"
)

type retryReadyStep struct {
	stepRun *domain.WorkflowStepRun
	step    *domain.WorkflowStep
}

func (e *WorkflowEngine) buildRetryStepRuns(
	ctx context.Context,
	store EngineStore,
	wfRun *domain.WorkflowRun,
	steps []domain.WorkflowStep,
	origStepRunByRef map[string]domain.WorkflowStepRun,
	completedRefs map[string]struct{},
	now time.Time,
) ([]retryReadyStep, error) {
	roots := make([]retryReadyStep, 0)

	for i := range steps {
		step := &steps[i]
		origSR, wasInOriginal := origStepRunByRef[step.StepRef]
		_, wasCompleted := completedRefs[step.StepRef]

		if wasInOriginal && wasCompleted {
			// Pre-complete: copy output from original run.
			finished := now
			sr := &domain.WorkflowStepRun{
				WorkflowRunID:  wfRun.ID,
				WorkflowStepID: step.ID,
				StepRef:        step.StepRef,
				Status:         domain.StepCompleted,
				DepsCompleted:  len(step.DependsOn),
				DepsRequired:   len(step.DependsOn),
				Output:         cloneRaw(origSR.Output),
				StartedAt:      &finished,
				FinishedAt:     &finished,
			}
			if err := store.CreateWorkflowStepRun(ctx, sr); err != nil {
				return nil, fmt.Errorf("create pre-completed step run %s: %w", step.StepRef, err)
			}
			continue
		}

		// Fresh step run.
		sr := &domain.WorkflowStepRun{
			WorkflowRunID:  wfRun.ID,
			WorkflowStepID: step.ID,
			StepRef:        step.StepRef,
			DepsCompleted:  0,
			DepsRequired:   len(step.DependsOn),
		}

		// Check if all deps are in the completed set.
		allDepsCompleted := true
		for _, dep := range step.DependsOn {
			if _, ok := completedRefs[dep]; !ok {
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted && len(step.DependsOn) == 0 {
			sr.Status = domain.StepPending
			roots = append(roots, retryReadyStep{stepRun: sr, step: step})
		} else if allDepsCompleted {
			sr.DepsCompleted = len(step.DependsOn)
			sr.Status = domain.StepPending
			roots = append(roots, retryReadyStep{stepRun: sr, step: step})
		} else {
			sr.Status = domain.StepWaiting
		}

		if err := store.CreateWorkflowStepRun(ctx, sr); err != nil {
			return nil, fmt.Errorf("create step run %s: %w", step.StepRef, err)
		}
	}

	return roots, nil
}

// RetryWorkflowRun creates a new workflow run that replays from the first failed step.
// Steps that completed successfully in the original run are copied as-is (pre-completed),
// while the failed step and all downstream steps are re-executed from scratch.
func (e *WorkflowEngine) RetryWorkflowRun(
	ctx context.Context,
	originalRunID string,
) (*domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.RetryWorkflowRun")
	defer span.End()
	telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "workflow retry requested", map[string]any{
		"original_workflow_run_id": originalRunID,
	})

	// 1. Fetch the original workflow run.
	origRun, err := e.store.GetWorkflowRun(ctx, originalRunID)
	if err != nil {
		return nil, fmt.Errorf("get original workflow run: %w", err)
	}
	if origRun == nil {
		return nil, fmt.Errorf("original workflow run not found: %s", originalRunID)
	}
	if !origRun.Status.IsTerminal() {
		return nil, fmt.Errorf("cannot retry workflow run %s: status is %s (must be terminal)", originalRunID, origRun.Status)
	}

	// 2. Fetch the workflow definition.
	wf, err := e.store.GetWorkflow(ctx, origRun.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	if wf == nil {
		return nil, fmt.Errorf("workflow not found: %s", origRun.WorkflowID)
	}
	if !wf.Enabled {
		return nil, fmt.Errorf("workflow is disabled: %s", origRun.WorkflowID)
	}

	// 3. Get step definitions for the original version.
	steps, err := e.listStepsByWorkflowVersion(ctx, origRun.WorkflowID, origRun.WorkflowVersion)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps: %w", err)
	}
	if err := ValidateDAG(steps); err != nil {
		return nil, fmt.Errorf("validate workflow dag: %w", err)
	}

	// 4. Get original step runs to determine which completed.
	origStepRuns, err := e.store.ListStepRunsByWorkflowRun(ctx, originalRunID, 10000, nil)
	if err != nil {
		return nil, fmt.Errorf("list original step runs: %w", err)
	}
	origStepRunByRef := make(map[string]domain.WorkflowStepRun, len(origStepRuns))
	for _, sr := range origStepRuns {
		origStepRunByRef[sr.StepRef] = sr
	}

	// Build set of completed step refs from the original run.
	completedRefs := make(map[string]struct{})
	for _, sr := range origStepRuns {
		if sr.Status == domain.StepCompleted {
			completedRefs[sr.StepRef] = struct{}{}
		}
	}

	// 5-6. Create the retry workflow run and step runs inside a transaction.
	wfRun := &domain.WorkflowRun{
		WorkflowID:       origRun.WorkflowID,
		ProjectID:        origRun.ProjectID,
		Status:           domain.WfStatusPending,
		TriggeredBy:      domain.TriggerRetry,
		WorkflowVersion:  origRun.WorkflowVersion,
		MaxParallelSteps: origRun.MaxParallelSteps,
		Payload:          cloneRaw(origRun.Payload),
		RetryOfRunID:     originalRunID,
	}
	if wf.TimeoutSecs > 0 {
		expiresAt := time.Now().Add(time.Duration(wf.TimeoutSecs) * time.Second)
		wfRun.ExpiresAt = &expiresAt
	}

	var roots []retryReadyStep
	now := time.Now()
	err = e.runInTx(ctx, func(txStore EngineStore) error {
		if err := txStore.CreateWorkflowRun(ctx, wfRun); err != nil {
			return fmt.Errorf("create retry workflow run: %w", err)
		}
		if err := txStore.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": now}); err != nil {
			return fmt.Errorf("start retry workflow run: %w", err)
		}
		var buildErr error
		roots, buildErr = e.buildRetryStepRuns(ctx, txStore, wfRun, steps, origStepRunByRef, completedRefs, now)
		if buildErr != nil {
			return buildErr
		}

		// Copy run state KV from completed job runs so downstream steps retain context.
		for ref := range completedRefs {
			origSR := origStepRunByRef[ref]
			if origSR.JobRunID != "" {
				_ = txStore.CopyRunState(ctx, origSR.JobRunID, wfRun.ID)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	wfRun.Status = domain.WfStatusRunning
	wfRun.StartedAt = &now
	telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "workflow retry started", map[string]any{
		"workflow_id":              wfRun.WorkflowID,
		"workflow_run_id":          wfRun.ID,
		"project_id":               wfRun.ProjectID,
		"original_workflow_run_id": originalRunID,
		"root_count":               len(roots),
	})

	// 7. Start ready steps (same logic as TriggerWorkflow).
	runningStarts := 0
	for _, root := range roots {
		if wfRun.MaxParallelSteps > 0 && runningStarts >= wfRun.MaxParallelSteps {
			if err := e.store.UpdateStepRunStatus(ctx, root.stepRun.ID, domain.StepWaiting, nil); err != nil {
				return nil, fmt.Errorf("set retry step waiting %s: %w", root.step.StepRef, err)
			}
			root.stepRun.Status = domain.StepWaiting
			continue
		}

		// Build parent outputs payload from pre-completed steps.
		var parentOutputsPayload json.RawMessage
		if len(root.step.DependsOn) > 0 {
			outputs, getErr := e.store.GetStepOutputs(ctx, wfRun.ID, root.step.DependsOn)
			if getErr != nil {
				return nil, fmt.Errorf("get step outputs for retry %s: %w", root.step.StepRef, getErr)
			}
			payload, marshalErr := json.Marshal(outputs)
			if marshalErr != nil {
				return nil, fmt.Errorf("marshal parent outputs for retry %s: %w", root.step.StepRef, marshalErr)
			}
			parentOutputsPayload = payload
		}

		if err := e.startStep(ctx, root.stepRun, root.step, wfRun, parentOutputsPayload); err != nil {
			return nil, fmt.Errorf("start retry step %s: %w", root.step.StepRef, err)
		}
		if root.stepRun.Status == domain.StepRunning {
			runningStarts++
		}
	}

	return wfRun, nil
}

// applyStepOverrides filters steps based on trigger-time overrides.
// Steps whose step_ref appears in overrides with enabled=false are removed.
// Remaining steps have their depends_on lists pruned of any removed refs.
// Returns an error if an override references a non-existent step_ref.
func applyStepOverrides(steps []domain.WorkflowStep, overrides []domain.StepOverride) ([]domain.WorkflowStep, error) {
	if len(overrides) == 0 {
		return steps, nil
	}
	if len(overrides) == 1 {
		override := overrides[0]
		if !override.Enabled {
			return applySingleDisabledStepOverride(steps, override.StepRef)
		}
		if !stepRefExists(steps, override.StepRef) {
			return nil, fmt.Errorf("step override references unknown step_ref %q", override.StepRef)
		}
		return steps, nil
	}

	return applyStepOverridesFiltered(steps, overrides)
}

func applyStepOverridesFiltered(steps []domain.WorkflowStep, overrides []domain.StepOverride) ([]domain.WorkflowStep, error) {
	var disabledRefs []string
	var disabledSet map[string]struct{}
	if len(overrides) <= 8 {
		for _, o := range overrides {
			if !stepRefExists(steps, o.StepRef) {
				return nil, fmt.Errorf("step override references unknown step_ref %q", o.StepRef)
			}
			if !o.Enabled {
				disabledRefs = append(disabledRefs, o.StepRef)
			}
		}
	} else {
		knownRefs := make(map[string]struct{}, len(steps))
		for _, s := range steps {
			knownRefs[s.StepRef] = struct{}{}
		}
		disabledSet = make(map[string]struct{})

		for _, o := range overrides {
			if _, ok := knownRefs[o.StepRef]; !ok {
				return nil, fmt.Errorf("step override references unknown step_ref %q", o.StepRef)
			}
			if !o.Enabled {
				disabledSet[o.StepRef] = struct{}{}
			}
		}
	}

	disabledCount := len(disabledRefs)
	if disabledSet != nil {
		disabledCount = len(disabledSet)
	}
	if disabledCount == 0 {
		return steps, nil
	}
	if disabledSet == nil && len(disabledRefs) > 2 {
		disabledSet = make(map[string]struct{}, len(disabledRefs))
		for _, ref := range disabledRefs {
			disabledSet[ref] = struct{}{}
		}
		disabledRefs = nil
	}
	if disabledSet == nil && len(disabledRefs) == 1 {
		return applySingleDisabledStepOverride(steps, disabledRefs[0])
	}

	// Filter out disabled steps and prune depends_on.
	filteredCap := max(len(steps)-disabledCount, 0)
	filtered := make([]domain.WorkflowStep, 0, filteredCap)
	for i := range steps {
		s := steps[i]
		if stepRefDisabled(disabledRefs, disabledSet, s.StepRef) {
			continue
		}

		if len(s.DependsOn) > 0 {
			removeAt := -1
			for depIdx, dep := range s.DependsOn {
				if stepRefDisabled(disabledRefs, disabledSet, dep) {
					removeAt = depIdx
					break
				}
			}
			if removeAt >= 0 {
				pruned := make([]string, 0, len(s.DependsOn)-1)
				pruned = append(pruned, s.DependsOn[:removeAt]...)
				for _, dep := range s.DependsOn[removeAt+1:] {
					if !stepRefDisabled(disabledRefs, disabledSet, dep) {
						pruned = append(pruned, dep)
					}
				}
				s.DependsOn = pruned
			}
		}

		filtered = append(filtered, s)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("all steps disabled by overrides")
	}

	return filtered, nil
}

func applySingleDisabledStepOverride(steps []domain.WorkflowStep, disabledRef string) ([]domain.WorkflowStep, error) {
	if len(steps) == 1 && steps[0].StepRef == disabledRef {
		return nil, fmt.Errorf("all steps disabled by overrides")
	}

	disabledIdx := -1
	for i := range steps {
		if steps[i].StepRef == disabledRef {
			disabledIdx = i
			break
		}
	}
	if disabledIdx < 0 {
		return nil, fmt.Errorf("step override references unknown step_ref %q", disabledRef)
	}

	filtered := make([]domain.WorkflowStep, len(steps)-1)
	copy(filtered, steps[:disabledIdx])
	copy(filtered[disabledIdx:], steps[disabledIdx+1:])

	for i := range filtered {
		s := &filtered[i]
		if len(s.DependsOn) > 0 {
			removeAt := -1
			for depIdx, dep := range s.DependsOn {
				if dep == disabledRef {
					removeAt = depIdx
					break
				}
			}
			if removeAt >= 0 {
				pruned := make([]string, 0, len(s.DependsOn)-1)
				pruned = append(pruned, s.DependsOn[:removeAt]...)
				pruned = append(pruned, s.DependsOn[removeAt+1:]...)
				s.DependsOn = pruned
			}
		}
	}

	return filtered, nil
}

func stepRefExists(steps []domain.WorkflowStep, ref string) bool {
	for i := range steps {
		if steps[i].StepRef == ref {
			return true
		}
	}
	return false
}

func stepRefDisabled(disabledRefs []string, disabledSet map[string]struct{}, ref string) bool {
	if disabledSet != nil {
		_, ok := disabledSet[ref]
		return ok
	}
	return slices.Contains(disabledRefs, ref)
}
