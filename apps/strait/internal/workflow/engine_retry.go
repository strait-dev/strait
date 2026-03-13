package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"

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
	steps, err := e.store.ListStepsByWorkflowVersion(ctx, origRun.WorkflowID, origRun.WorkflowVersion)
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
	if err := e.runInTx(ctx, func(txStore EngineStore) error {
		if err := txStore.CreateWorkflowRun(ctx, wfRun); err != nil {
			return fmt.Errorf("create retry workflow run: %w", err)
		}
		if err := txStore.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": now}); err != nil {
			return fmt.Errorf("start retry workflow run: %w", err)
		}
		var buildErr error
		roots, buildErr = e.buildRetryStepRuns(ctx, txStore, wfRun, steps, origStepRunByRef, completedRefs, now)
		return buildErr
	}); err != nil {
		return nil, err
	}
	wfRun.Status = domain.WfStatusRunning
	wfRun.StartedAt = &now

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
	// Build set of disabled step refs.
	disabled := make(map[string]struct{})
	knownRefs := make(map[string]struct{}, len(steps))
	for _, s := range steps {
		knownRefs[s.StepRef] = struct{}{}
	}

	for _, o := range overrides {
		if _, ok := knownRefs[o.StepRef]; !ok {
			return nil, fmt.Errorf("step override references unknown step_ref %q", o.StepRef)
		}
		if !o.Enabled {
			disabled[o.StepRef] = struct{}{}
		}
	}

	if len(disabled) == 0 {
		return steps, nil
	}

	// Filter out disabled steps and prune depends_on.
	filtered := make([]domain.WorkflowStep, 0, len(steps))
	for _, s := range steps {
		if _, skip := disabled[s.StepRef]; skip {
			continue
		}

		// Prune disabled refs from depends_on.
		if len(s.DependsOn) > 0 {
			pruned := make([]string, 0, len(s.DependsOn))
			for _, dep := range s.DependsOn {
				if _, skip := disabled[dep]; !skip {
					pruned = append(pruned, dep)
				}
			}
			s.DependsOn = pruned
		}

		filtered = append(filtered, s)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("all steps disabled by overrides")
	}

	return filtered, nil
}
