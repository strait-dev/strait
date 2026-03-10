package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
)

func (e *WorkflowEngine) startStep(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	step *domain.WorkflowStep,
	wfRun *domain.WorkflowRun,
	mergedPayload json.RawMessage,
) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.startStep")
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

		// Create a parallel event trigger for unified tracking.
		eventKey := fmt.Sprintf("approval:%s:%s", wfRun.ID, step.StepRef)
		timeoutSecs := step.ApprovalTimeoutSecs
		if timeoutSecs <= 0 {
			timeoutSecs = domain.DefaultEventTimeoutSecs
		}
		expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)
		trigger := &domain.EventTrigger{
			ID:                fmt.Sprintf("evt:approval:%s", stepRun.ID),
			EventKey:          eventKey,
			ProjectID:         wfRun.ProjectID,
			SourceType:        domain.EventSourceWorkflowStep,
			WorkflowRunID:     wfRun.ID,
			WorkflowStepRunID: stepRun.ID,
			Status:            domain.EventTriggerStatusWaiting,
			TimeoutSecs:       timeoutSecs,
			RequestedAt:       now,
			ExpiresAt:         expiresAt,
		}
		if err := e.store.CreateEventTrigger(ctx, trigger); err != nil {
			e.logger.Warn("failed to create event trigger for approval step (non-fatal)", "step_ref", step.StepRef, "error", err)
		}

		stepRun.Status = domain.StepWaiting
		stepRun.StartedAt = &now
		return nil
	}

	if step.StepType == domain.WorkflowStepTypeWaitForEvent {
		return e.startWaitForEventStep(ctx, stepRun, step, wfRun, now)
	}

	if step.StepType == domain.WorkflowStepTypeSleep {
		return e.startSleepStep(ctx, stepRun, step, wfRun, now)
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
		Tags:                wfRun.Tags,
		Payload:             payload,
		TriggeredBy:         domain.TriggerWorkflow,
		WorkflowStepRunID:   stepRun.ID,
		TimeoutSecsOverride: step.TimeoutSecsOverride,
		CreatedBy:           "system:workflow",
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

	childRun, err := e.TriggerSubWorkflow(ctx, step.SubWorkflowID, wfRun.ProjectID, payload, domain.TriggerWorkflow, wfRun.ID, stepRun.ID)
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

// startWaitForEventStep pauses execution until an external event is received.
// No goroutine is held — the wait is a database row.
func (e *WorkflowEngine) startWaitForEventStep(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	step *domain.WorkflowStep,
	wfRun *domain.WorkflowRun,
	now time.Time,
) error {
	// Render event key template against workflow payload BEFORE any status changes,
	// so we fail fast without leaving the step in a stuck waiting state.
	renderedKey := renderStringTemplate(step.EventKey, wfRun.Payload)
	if renderedKey == "" {
		return fmt.Errorf("event_key is empty for step %s", step.StepRef)
	}
	if len(renderedKey) > 512 {
		return fmt.Errorf("event_key exceeds 512 characters for step %s", step.StepRef)
	}
	for i := 0; i < len(renderedKey); i++ {
		if renderedKey[i] < 0x20 {
			return fmt.Errorf("event_key contains invalid control characters for step %s", step.StepRef)
		}
	}

	timeoutSecs := step.EventTimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = domain.DefaultEventTimeoutSecs
	}
	expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)

	trigger := &domain.EventTrigger{
		ID:                fmt.Sprintf("evt:%s", stepRun.ID),
		EventKey:          renderedKey,
		ProjectID:         wfRun.ProjectID,
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Status:            domain.EventTriggerStatusWaiting,
		TimeoutSecs:       timeoutSecs,
		RequestedAt:       now,
		ExpiresAt:         expiresAt,
		NotifyURL:         step.EventNotifyURL,
	}

	// Create the trigger BEFORE updating step status. If trigger creation fails
	// (e.g., key conflict), the step stays in its current status rather than
	// being stuck in 'waiting' with no trigger row.
	if err := e.store.CreateEventTrigger(ctx, trigger); err != nil {
		if errors.Is(err, store.ErrEventKeyConflict) {
			return fmt.Errorf("event key %q already in use — use a unique key pattern like {workflow_id}:{run_id}:{step_ref}: %w", renderedKey, err)
		}
		return fmt.Errorf("create event trigger for step %s: %w", step.StepRef, err)
	}

	if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepWaiting, map[string]any{"started_at": now}); err != nil {
		return fmt.Errorf("set wait_for_event step waiting: %w", err)
	}

	if e.onTriggerCreate != nil {
		e.onTriggerCreate(trigger)
	}

	stepRun.Status = domain.StepWaiting
	stepRun.StartedAt = &now

	e.logger.Info("wait_for_event step started",
		"workflow_run_id", wfRun.ID,
		"step_ref", step.StepRef,
		"event_key", renderedKey,
		"expires_at", expiresAt,
	)

	return nil
}

// startSleepStep creates a "sleep" event trigger that the reaper will complete
// when the expiry time is reached. No goroutine is held.
func (e *WorkflowEngine) startSleepStep(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	step *domain.WorkflowStep,
	wfRun *domain.WorkflowRun,
	now time.Time,
) error {
	durationSecs := step.SleepDurationSecs
	if durationSecs <= 0 {
		durationSecs = 60 // default 1 minute
	}
	expiresAt := now.Add(time.Duration(durationSecs) * time.Second)

	trigger := &domain.EventTrigger{
		ID:                fmt.Sprintf("slp:%s", stepRun.ID),
		EventKey:          fmt.Sprintf("sleep:%s:%s", wfRun.ID, step.StepRef),
		ProjectID:         wfRun.ProjectID,
		SourceType:        domain.EventSourceWorkflowStep,
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Status:            domain.EventTriggerStatusWaiting,
		TriggerType:       domain.TriggerTypeSleep,
		TimeoutSecs:       durationSecs,
		RequestedAt:       now,
		ExpiresAt:         expiresAt,
	}

	// Create the trigger BEFORE updating step status. If trigger creation fails,
	// the step stays in its current status rather than being stuck in 'waiting'
	// with no trigger row.
	if err := e.store.CreateEventTrigger(ctx, trigger); err != nil {
		return fmt.Errorf("create sleep trigger for step %s: %w", step.StepRef, err)
	}

	if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepWaiting, map[string]any{"started_at": now}); err != nil {
		return fmt.Errorf("set sleep step waiting: %w", err)
	}

	if e.onTriggerCreate != nil {
		e.onTriggerCreate(trigger)
	}

	stepRun.Status = domain.StepWaiting
	stepRun.StartedAt = &now

	e.logger.Info("sleep step started",
		"workflow_run_id", wfRun.ID,
		"step_ref", step.StepRef,
		"duration_secs", durationSecs,
		"expires_at", expiresAt,
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
