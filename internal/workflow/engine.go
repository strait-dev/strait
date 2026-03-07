package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"orchestrator/internal/domain"

	"go.opentelemetry.io/otel"
)

// DefaultMaxNestingDepth is the nesting limit when none is specified on the step.
const DefaultMaxNestingDepth = 10

type WorkflowEngine struct {
	store           EngineStore
	queue           EngineQueue
	logger          *slog.Logger
	maxNestingDepth int
}

type EngineStore interface {
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error)
	CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error
	CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error
	CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
}

type EngineQueue interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

// NewWorkflowEngine creates a new workflow engine for triggering and managing workflow runs.
func NewWorkflowEngine(store EngineStore, queue EngineQueue, logger *slog.Logger) *WorkflowEngine {
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkflowEngine{
		store:           store,
		queue:           queue,
		logger:          logger,
		maxNestingDepth: DefaultMaxNestingDepth,
	}
}

// WithMaxNestingDepth overrides the default sub-workflow nesting depth limit.
func (e *WorkflowEngine) WithMaxNestingDepth(n int) *WorkflowEngine {
	if n > 0 {
		e.maxNestingDepth = n
	}
	return e
}

func (e *WorkflowEngine) TriggerWorkflow(
	ctx context.Context,
	workflowID, projectID string,
	payload json.RawMessage,
	triggeredBy string,
	stepOverrides []domain.StepOverride,
) (*domain.WorkflowRun, error) {
	return e.triggerWorkflowInternal(ctx, workflowID, projectID, payload, triggeredBy, "", stepOverrides)
}

// TriggerSubWorkflow triggers a workflow as a child of another workflow run.
func (e *WorkflowEngine) TriggerSubWorkflow(
	ctx context.Context,
	workflowID, projectID string,
	payload json.RawMessage,
	triggeredBy string,
	parentWorkflowRunID string,
) (*domain.WorkflowRun, error) {
	return e.triggerWorkflowInternal(ctx, workflowID, projectID, payload, triggeredBy, parentWorkflowRunID, nil)
}

func (e *WorkflowEngine) triggerWorkflowInternal(
	ctx context.Context,
	workflowID, projectID string,
	payload json.RawMessage,
	triggeredBy string,
	parentWorkflowRunID string,
	stepOverrides []domain.StepOverride,
) (*domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "workflow.TriggerWorkflow")
	defer span.End()

	wf, err := e.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	if wf == nil {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if !wf.Enabled {
		return nil, fmt.Errorf("workflow is disabled: %s", workflowID)
	}
	if projectID == "" {
		projectID = wf.ProjectID
	}
	if wf.ProjectID != "" && projectID != wf.ProjectID {
		return nil, fmt.Errorf("workflow %s does not belong to project %s", workflowID, projectID)
	}

	steps, err := e.store.ListStepsByWorkflowVersion(ctx, workflowID, wf.Version)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps by version: %w", err)
	}

	// Apply step overrides to filter steps at trigger time.
	if len(stepOverrides) > 0 {
		steps, err = applyStepOverrides(steps, stepOverrides)
		if err != nil {
			return nil, fmt.Errorf("apply step overrides: %w", err)
		}
	}

	if err := ValidateDAG(steps); err != nil {
		return nil, fmt.Errorf("validate workflow dag: %w", err)
	}

	if wf.MaxConcurrentRuns > 0 {
		const maxConcurrencyRetries = 120
		for i := range maxConcurrencyRetries {
			running, countErr := e.store.CountRunningWorkflowRuns(ctx, workflowID)
			if countErr != nil {
				return nil, fmt.Errorf("count running workflow runs: %w", countErr)
			}
			if running < wf.MaxConcurrentRuns {
				break
			}
			if i == maxConcurrencyRetries-1 {
				return nil, fmt.Errorf("workflow %s: max concurrent runs (%d) reached, timed out waiting for slot", workflowID, wf.MaxConcurrentRuns)
			}

			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("wait for workflow concurrency slot: %w", ctx.Err())
			case <-time.After(250 * time.Millisecond):
			}
		}
	}

	if triggeredBy == "" {
		triggeredBy = domain.TriggerManual
	}

	wfRun := &domain.WorkflowRun{
		WorkflowID:          workflowID,
		ProjectID:           projectID,
		Status:              domain.WfStatusPending,
		TriggeredBy:         triggeredBy,
		WorkflowVersion:     wf.Version,
		MaxParallelSteps:    wf.MaxParallelSteps,
		Payload:             payload,
		ParentWorkflowRunID: parentWorkflowRunID,
	}
	if wf.TimeoutSecs > 0 {
		expiresAt := time.Now().Add(time.Duration(wf.TimeoutSecs) * time.Second)
		wfRun.ExpiresAt = &expiresAt
	}
	if err := e.store.CreateWorkflowRun(ctx, wfRun); err != nil {
		return nil, fmt.Errorf("create workflow run: %w", err)
	}

	now := time.Now()
	if err := e.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": now}); err != nil {
		return nil, fmt.Errorf("start workflow run: %w", err)
	}
	wfRun.Status = domain.WfStatusRunning
	wfRun.StartedAt = &now

	type rootToStart struct {
		stepRun *domain.WorkflowStepRun
		step    *domain.WorkflowStep
	}
	roots := make([]rootToStart, 0)

	for i := range steps {
		step := &steps[i]
		stepRun := &domain.WorkflowStepRun{
			WorkflowRunID:  wfRun.ID,
			WorkflowStepID: step.ID,
			StepRef:        step.StepRef,
			DepsCompleted:  0,
			DepsRequired:   len(step.DependsOn),
		}

		if len(step.DependsOn) == 0 {
			stepRun.Status = domain.StepPending
			stepRun.DepsRequired = 0
			roots = append(roots, rootToStart{stepRun: stepRun, step: step})
		} else {
			stepRun.Status = domain.StepWaiting
		}

		if err := e.store.CreateWorkflowStepRun(ctx, stepRun); err != nil {
			return nil, fmt.Errorf("create step run %s: %w", step.StepRef, err)
		}
	}

	runningStarts := 0
	for _, root := range roots {
		if wfRun.MaxParallelSteps > 0 && runningStarts >= wfRun.MaxParallelSteps {
			if err := e.store.UpdateStepRunStatus(ctx, root.stepRun.ID, domain.StepWaiting, nil); err != nil {
				return nil, fmt.Errorf("set root step waiting %s: %w", root.step.StepRef, err)
			}
			root.stepRun.Status = domain.StepWaiting
			continue
		}
		if err := e.startStep(ctx, root.stepRun, root.step, wfRun, nil); err != nil {
			return nil, fmt.Errorf("start root step %s: %w", root.step.StepRef, err)
		}
		if root.stepRun.Status == domain.StepRunning {
			runningStarts++
		}
	}

	return wfRun, nil
}

func (e *WorkflowEngine) startStep(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	step *domain.WorkflowStep,
	wfRun *domain.WorkflowRun,
	mergedPayload json.RawMessage,
) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "workflow.startStep")
	defer span.End()

	now := time.Now()
	if step.StepType == domain.WorkflowStepTypeApproval {
		if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepWaiting, map[string]any{"started_at": now}); err != nil {
			return fmt.Errorf("set approval step waiting: %w", err)
		}

		approval := &domain.WorkflowStepApproval{
			ID:                fmt.Sprintf("approval:%s", stepRun.ID),
			WorkflowRunID:     wfRun.ID,
			WorkflowStepRunID: stepRun.ID,
			Approvers:         slices.Clone(step.ApprovalApprovers),
			Status:            domain.ApprovalStatusPending,
			RequestedAt:       now,
		}
		if step.ApprovalTimeoutSecs > 0 {
			expiresAt := now.Add(time.Duration(step.ApprovalTimeoutSecs) * time.Second)
			approval.ExpiresAt = &expiresAt
		}
		if err := e.store.CreateWorkflowStepApproval(ctx, approval); err != nil {
			return fmt.Errorf("create workflow step approval: %w", err)
		}
		stepRun.Status = domain.StepWaiting
		stepRun.StartedAt = &now
		return nil
	}

	if step.StepType == domain.WorkflowStepTypeSubWorkflow {
		return e.startSubWorkflowStep(ctx, stepRun, step, wfRun, mergedPayload, now)
	}

	if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepRunning, map[string]any{"started_at": now}); err != nil {
		return fmt.Errorf("set step run running: %w", err)
	}
	stepRun.Status = domain.StepRunning
	stepRun.StartedAt = &now

	renderedStepPayload := renderTemplateVars(step.Payload, wfRun.Payload)
	payload := mergePayloads(wfRun.Payload, renderedStepPayload, mergedPayload)
	jobRun := &domain.JobRun{
		JobID:               step.JobID,
		ProjectID:           wfRun.ProjectID,
		Payload:             payload,
		TriggeredBy:         domain.TriggerWorkflow,
		WorkflowStepRunID:   stepRun.ID,
		TimeoutSecsOverride: step.TimeoutSecsOverride,
	}
	if err := e.queue.Enqueue(ctx, jobRun); err != nil {
		return fmt.Errorf("enqueue step job run: %w", err)
	}

	if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepRunning, map[string]any{"job_run_id": jobRun.ID}); err != nil {
		return fmt.Errorf("attach job run to step run: %w", err)
	}
	stepRun.JobRunID = jobRun.ID

	return nil
}

// startSubWorkflowStep triggers a child workflow for a sub_workflow step.
func (e *WorkflowEngine) startSubWorkflowStep(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	step *domain.WorkflowStep,
	wfRun *domain.WorkflowRun,
	mergedPayload json.RawMessage,
	now time.Time,
) error {
	maxDepth := step.MaxNestingDepth
	if maxDepth <= 0 {
		maxDepth = e.maxNestingDepth
	}
	currentDepth, err := e.getNestingDepth(ctx, wfRun)
	if err != nil {
		return fmt.Errorf("get nesting depth: %w", err)
	}
	if currentDepth >= maxDepth {
		return fmt.Errorf("sub-workflow nesting depth %d exceeds max allowed %d for step %s", currentDepth, maxDepth, step.StepRef)
	}

	if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepRunning, map[string]any{"started_at": now}); err != nil {
		return fmt.Errorf("set sub-workflow step running: %w", err)
	}
	stepRun.Status = domain.StepRunning
	stepRun.StartedAt = &now

	renderedStepPayload := renderTemplateVars(step.Payload, wfRun.Payload)
	payload := mergePayloads(wfRun.Payload, renderedStepPayload, mergedPayload)

	childRun, err := e.TriggerSubWorkflow(ctx, step.SubWorkflowID, wfRun.ProjectID, payload, domain.TriggerWorkflow, wfRun.ID)
	if err != nil {
		return fmt.Errorf("trigger sub-workflow for step %s: %w", step.StepRef, err)
	}

	e.logger.Info("sub-workflow triggered",
		"parent_workflow_run_id", wfRun.ID,
		"child_workflow_run_id", childRun.ID,
		"step_ref", step.StepRef,
		"sub_workflow_id", step.SubWorkflowID,
		"nesting_depth", currentDepth+1,
	)

	return nil
}

// getNestingDepth calculates how deeply nested a workflow run is by walking up the parent chain.
func (e *WorkflowEngine) getNestingDepth(ctx context.Context, wfRun *domain.WorkflowRun) (int, error) {
	depth := 0
	current := wfRun
	seen := make(map[string]struct{})

	for current.ParentWorkflowRunID != "" {
		if _, ok := seen[current.ParentWorkflowRunID]; ok {
			return depth, fmt.Errorf("circular parent reference detected at %s", current.ParentWorkflowRunID)
		}
		seen[current.ID] = struct{}{}
		depth++

		parent, err := e.store.GetWorkflowRun(ctx, current.ParentWorkflowRunID)
		if err != nil {
			return depth, fmt.Errorf("get parent workflow run %s: %w", current.ParentWorkflowRunID, err)
		}
		if parent == nil {
			break
		}
		current = parent
	}

	return depth, nil
}

func mergePayloads(triggerPayload, stepPayload, parentOutputs json.RawMessage) json.RawMessage {
	triggerObj, triggerIsObject := decodeJSONObject(triggerPayload)
	stepObj, stepIsObject := decodeJSONObject(stepPayload)

	if !triggerIsObject || !stepIsObject {
		if len(bytes.TrimSpace(stepPayload)) > 0 {
			return cloneRaw(stepPayload)
		}
		return cloneRaw(triggerPayload)
	}

	merged := make(map[string]any, len(triggerObj)+len(stepObj)+1)
	maps.Copy(merged, triggerObj)
	maps.Copy(merged, stepObj)

	if len(bytes.TrimSpace(parentOutputs)) > 0 {
		var parentValue any
		if err := json.Unmarshal(parentOutputs, &parentValue); err == nil {
			merged["parent_outputs"] = parentValue
		} else {
			merged["parent_outputs"] = parentOutputs
		}
	}

	out, err := json.Marshal(merged)
	if err != nil {
		slog.Warn("mergePayloads: failed to marshal merged payload, falling back", "error", err)
		if len(bytes.TrimSpace(stepPayload)) > 0 {
			return cloneRaw(stepPayload)
		}
		return cloneRaw(triggerPayload)
	}

	return out
}

func decodeJSONObject(payload json.RawMessage) (map[string]any, bool) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return map[string]any{}, true
	}

	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, false
	}
	if obj == nil {
		return nil, false
	}

	return obj, true
}

func cloneRaw(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}

type retryReadyStep struct {
	stepRun *domain.WorkflowStepRun
	step    *domain.WorkflowStep
}

func (e *WorkflowEngine) buildRetryStepRuns(
	ctx context.Context,
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
			if err := e.store.CreateWorkflowStepRun(ctx, sr); err != nil {
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

		if err := e.store.CreateWorkflowStepRun(ctx, sr); err != nil {
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "workflow.RetryWorkflowRun")
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
	origStepRuns, err := e.store.ListStepRunsByWorkflowRun(ctx, originalRunID)
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

	// 5. Create the retry workflow run.
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
	if err := e.store.CreateWorkflowRun(ctx, wfRun); err != nil {
		return nil, fmt.Errorf("create retry workflow run: %w", err)
	}

	now := time.Now()
	if err := e.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": now}); err != nil {
		return nil, fmt.Errorf("start retry workflow run: %w", err)
	}
	wfRun.Status = domain.WfStatusRunning
	wfRun.StartedAt = &now

	// 6. Create step runs. Completed steps are pre-populated; others start fresh.
	roots, err := e.buildRetryStepRuns(ctx, wfRun, steps, origStepRunByRef, completedRefs, now)
	if err != nil {
		return nil, err
	}

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
