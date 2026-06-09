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
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"go.opentelemetry.io/otel"
)

func workflowApprovalID(stepRunID string) string {
	return "approval:" + stepRunID
}

func workflowApprovalEventKey(workflowRunID, stepRef string) string {
	return "approval:" + workflowRunID + ":" + stepRef
}

func workflowApprovalEventTriggerID(stepRunID string) string {
	return "evt:approval:" + stepRunID
}

func workflowCostGateApprovalID(stepRunID string) string {
	return "costgate:" + stepRunID
}

func workflowEventTriggerID(stepRunID string) string {
	return "evt:" + stepRunID
}

func workflowSleepTriggerID(stepRunID string) string {
	return "slp:" + stepRunID
}

func workflowSleepEventKey(workflowRunID, stepRef string) string {
	return "sleep:" + workflowRunID + ":" + stepRef
}

func (e *WorkflowEngine) startStep(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	step *domain.WorkflowStep,
	wfRun *domain.WorkflowRun,
	mergedPayload json.RawMessage,
) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.startStep")
	defer span.End()
	telemetry.AddSentryBreadcrumb(ctx, "workflow.step", "workflow step starting", map[string]any{
		"workflow_id":      wfRun.WorkflowID,
		"workflow_run_id":  wfRun.ID,
		"workflow_step_id": step.ID,
		"step_run_id":      stepRun.ID,
		"step_ref":         step.StepRef,
		"step_type":        string(step.StepType),
		"project_id":       wfRun.ProjectID,
		"job_id":           step.JobID,
	})

	now := time.Now()
	if step.StepType == domain.WorkflowStepTypeApproval {
		if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepWaiting, map[string]any{"started_at": now}); err != nil {
			return fmt.Errorf("set approval step waiting: %w", err)
		}
		recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepWaiting))

		approval := &domain.WorkflowStepApproval{
			ID:                workflowApprovalID(stepRun.ID),
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
		eventKey := workflowApprovalEventKey(wfRun.ID, step.StepRef)
		timeoutSecs := step.ApprovalTimeoutSecs
		if timeoutSecs <= 0 {
			timeoutSecs = domain.DefaultEventTimeoutSecs
		}
		if timeoutSecs > domain.MaxEventTimeoutSecs {
			timeoutSecs = domain.MaxEventTimeoutSecs
		}
		expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)
		trigger := &domain.EventTrigger{
			ID:                workflowApprovalEventTriggerID(stepRun.ID),
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
		telemetry.AddSentryBreadcrumb(ctx, "workflow.step", "workflow step waiting", map[string]any{
			"workflow_run_id": wfRun.ID,
			"step_run_id":     stepRun.ID,
			"step_ref":        step.StepRef,
			"step_type":       string(step.StepType),
			"project_id":      wfRun.ProjectID,
		})

		e.enqueueApprovalNotifications(ctx, wfRun.ProjectID, approval, stepRun, wfRun)
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

	// Cost gate: if a threshold is configured and the estimated cost exceeds it,
	// pause the step and request approval before proceeding.
	if step.CostGateThresholdMicrousd > 0 && step.JobID != "" {
		estimate, err := e.store.GetJobCostEstimate(ctx, step.JobID)
		if err == nil && estimate != nil && estimate.AvgCostMicrousd > step.CostGateThresholdMicrousd {
			if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepWaiting, map[string]any{"started_at": now}); err != nil {
				return fmt.Errorf("set cost gate step waiting: %w", err)
			}
			recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepWaiting))

			timeoutSecs := step.CostGateTimeoutSecs
			if timeoutSecs <= 0 {
				timeoutSecs = domain.DefaultEventTimeoutSecs
			}
			if timeoutSecs > domain.MaxEventTimeoutSecs {
				timeoutSecs = domain.MaxEventTimeoutSecs
			}
			expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)

			approval := &domain.WorkflowStepApproval{
				ID:                workflowCostGateApprovalID(stepRun.ID),
				WorkflowRunID:     wfRun.ID,
				WorkflowStepRunID: stepRun.ID,
				Approvers:         []string{},
				Status:            domain.ApprovalStatusPending,
				RequestedAt:       now,
				ExpiresAt:         &expiresAt,
			}
			if err := e.store.CreateWorkflowStepApproval(ctx, approval); err != nil {
				return fmt.Errorf("create cost gate approval: %w", err)
			}

			stepRun.Status = domain.StepWaiting
			stepRun.StartedAt = &now
			telemetry.AddSentryBreadcrumb(ctx, "workflow.step", "workflow step waiting", map[string]any{
				"workflow_run_id": wfRun.ID,
				"step_run_id":     stepRun.ID,
				"step_ref":        step.StepRef,
				"step_type":       string(step.StepType),
				"project_id":      wfRun.ProjectID,
			})

			e.enqueueApprovalNotifications(ctx, wfRun.ProjectID, approval, stepRun, wfRun)

			e.logger.Info("cost gate triggered",
				"workflow_run_id", wfRun.ID,
				"step_ref", step.StepRef,
				"job_id", step.JobID,
				"avg_cost_microusd", estimate.AvgCostMicrousd,
				"threshold_microusd", step.CostGateThresholdMicrousd,
			)

			return nil
		}
	}

	// For regular job steps: enqueue first, then mark running.
	// This avoids orphan running steps if enqueue fails.
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
	// Propagate W3C trace context from workflow run to child job runs.
	if len(wfRun.TraceContext) > 0 {
		if jobRun.Metadata == nil {
			jobRun.Metadata = make(map[string]string, 2)
		}
		if tp, ok := wfRun.TraceContext["traceparent"]; ok {
			jobRun.Metadata[domain.RunMetadataTraceParent] = tp
		}
		if ts, ok := wfRun.TraceContext["tracestate"]; ok {
			jobRun.Metadata[domain.RunMetadataTraceState] = ts
		}
	}
	if err := queue.EnqueueWithRetry(ctx, e.queue, jobRun, queue.DefaultInternalEnqueueRetryConfig()); err != nil {
		return fmt.Errorf("enqueue step job run: %w", err)
	}

	if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepRunning, map[string]any{
		"started_at": now,
		"job_run_id": jobRun.ID,
	}); err != nil {
		e.logger.Error("step run status update failed after job enqueued",
			"step_run_id", stepRun.ID,
			"job_run_id", jobRun.ID,
			"step_ref", step.StepRef,
			"error", err,
		)
		return fmt.Errorf("set step run running (job %s already enqueued): %w", jobRun.ID, err)
	}
	recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepRunning))
	stepRun.Status = domain.StepRunning
	stepRun.StartedAt = &now
	stepRun.JobRunID = jobRun.ID
	telemetry.AddSentryBreadcrumb(ctx, "workflow.step", "workflow step enqueued", map[string]any{
		"workflow_run_id": wfRun.ID,
		"step_run_id":     stepRun.ID,
		"step_ref":        step.StepRef,
		"project_id":      wfRun.ProjectID,
		"job_id":          step.JobID,
		"job_run_id":      jobRun.ID,
	})

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
	recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepRunning))
	stepRun.Status = domain.StepRunning
	stepRun.StartedAt = &now
	telemetry.AddSentryBreadcrumb(ctx, "workflow.step", "workflow subworkflow step running", map[string]any{
		"workflow_run_id":   wfRun.ID,
		"step_run_id":       stepRun.ID,
		"step_ref":          step.StepRef,
		"project_id":        wfRun.ProjectID,
		"sub_workflow_id":   step.SubWorkflowID,
		"max_nesting_depth": maxDepth,
	})

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
	telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "subworkflow triggered", map[string]any{
		"parent_workflow_run_id": wfRun.ID,
		"child_workflow_run_id":  childRun.ID,
		"step_run_id":            stepRun.ID,
		"step_ref":               step.StepRef,
		"project_id":             wfRun.ProjectID,
		"nesting_depth":          currentDepth + 1,
	})

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
	for i := range len(renderedKey) {
		if renderedKey[i] < 0x20 {
			return fmt.Errorf("event_key contains invalid control characters for step %s", step.StepRef)
		}
	}

	timeoutSecs := step.EventTimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = domain.DefaultEventTimeoutSecs
	}
	if timeoutSecs > domain.MaxEventTimeoutSecs {
		timeoutSecs = domain.MaxEventTimeoutSecs
	}
	expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)

	trigger := &domain.EventTrigger{
		ID:                workflowEventTriggerID(stepRun.ID),
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
	recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepWaiting))

	if e.onTriggerCreate != nil {
		e.onTriggerCreate(trigger)
	}

	stepRun.Status = domain.StepWaiting
	stepRun.StartedAt = &now
	telemetry.AddSentryBreadcrumb(ctx, "workflow.step", "workflow step waiting", map[string]any{
		"workflow_run_id": wfRun.ID,
		"step_run_id":     stepRun.ID,
		"step_ref":        step.StepRef,
		"step_type":       string(step.StepType),
		"project_id":      wfRun.ProjectID,
		"timeout_secs":    timeoutSecs,
		"expires_at":      expiresAt.Format(time.RFC3339),
	})

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
		durationSecs = domain.DefaultSleepDurationSecs
	}
	if durationSecs > domain.MaxSleepDurationSecs {
		return fmt.Errorf("sleep duration %d exceeds maximum %d", durationSecs, domain.MaxSleepDurationSecs)
	}
	expiresAt := now.Add(time.Duration(durationSecs) * time.Second)

	trigger := &domain.EventTrigger{
		ID:                workflowSleepTriggerID(stepRun.ID),
		EventKey:          workflowSleepEventKey(wfRun.ID, step.StepRef),
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
	recordWorkflowStepTransition(ctx, string(stepRun.Status), string(domain.StepWaiting))

	if e.onTriggerCreate != nil {
		e.onTriggerCreate(trigger)
	}

	stepRun.Status = domain.StepWaiting
	stepRun.StartedAt = &now
	telemetry.AddSentryBreadcrumb(ctx, "workflow.step", "workflow step waiting", map[string]any{
		"workflow_run_id": wfRun.ID,
		"step_run_id":     stepRun.ID,
		"step_ref":        step.StepRef,
		"step_type":       string(step.StepType),
		"project_id":      wfRun.ProjectID,
		"duration_secs":   durationSecs,
		"expires_at":      expiresAt.Format(time.RFC3339),
	})

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
	triggerKind := firstNonSpaceByte(triggerPayload)
	stepKind := firstNonSpaceByte(stepPayload)
	parentBlank := firstNonSpaceByte(parentOutputs) == 0
	triggerBlank := triggerKind == 0
	stepBlank := stepKind == 0
	if parentBlank {
		switch {
		case stepBlank:
			return cloneRaw(triggerPayload)
		case triggerBlank || triggerKind != '{' || stepKind != '{':
			return cloneRaw(stepPayload)
		}
	}

	return mergeObjectPayloads(triggerPayload, stepPayload, parentOutputs, triggerKind, stepKind, parentBlank, stepBlank)
}

func mergeObjectPayloads(
	triggerPayload,
	stepPayload,
	parentOutputs json.RawMessage,
	triggerKind,
	stepKind byte,
	parentBlank,
	stepBlank bool,
) json.RawMessage {
	if triggerKind == '{' && stepKind == '{' {
		if out, ok := mergeJSONObjectPayloads(triggerPayload, stepPayload, parentOutputs, !parentBlank); ok {
			return out
		}
	}

	triggerObj, triggerIsObject := decodeJSONObject(triggerPayload)
	stepObj, stepIsObject := decodeJSONObject(stepPayload)

	if !triggerIsObject || !stepIsObject {
		if !stepBlank {
			return cloneRaw(stepPayload)
		}
		return cloneRaw(triggerPayload)
	}

	merged := make(map[string]any, len(triggerObj)+len(stepObj)+1)
	maps.Copy(merged, triggerObj)
	maps.Copy(merged, stepObj)

	if !parentBlank {
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
		if !stepBlank {
			return cloneRaw(stepPayload)
		}
		return cloneRaw(triggerPayload)
	}

	return out
}

type jsonObjectField struct {
	key string
	raw []byte
}

func mergeJSONObjectPayloads(triggerPayload, stepPayload, parentOutputs json.RawMessage, includeParent bool) (json.RawMessage, bool) {
	triggerFields, triggerHasDuplicates, ok := splitTopLevelJSONObjectFields(triggerPayload)
	if !ok {
		return nil, false
	}
	stepFields, stepHasDuplicates, ok := splitTopLevelJSONObjectFields(stepPayload)
	if !ok {
		return nil, false
	}

	var parentValue []byte
	if includeParent {
		parentValue = bytes.TrimSpace(parentOutputs)
		if !json.Valid(parentValue) {
			return nil, false
		}
	}

	var lastTriggerField map[string]int
	if triggerHasDuplicates {
		lastTriggerField = lastJSONFieldIndexes(triggerFields)
	}
	lastStepField := lastJSONFieldIndexes(stepFields)

	outCap := len(triggerPayload) + len(stepPayload) + len(parentValue) + len(`,"parent_outputs":`)
	out := make([]byte, 0, outCap)
	out = append(out, '{')
	hasField := false
	appendField := func(raw []byte) {
		if hasField {
			out = append(out, ',')
		}
		out = append(out, raw...)
		hasField = true
	}

	for i, field := range triggerFields {
		if triggerHasDuplicates && lastTriggerField[field.key] != i {
			continue
		}
		if _, overwritten := lastStepField[field.key]; overwritten {
			continue
		}
		if includeParent && field.key == "parent_outputs" {
			continue
		}
		appendField(field.raw)
	}
	for i, field := range stepFields {
		if stepHasDuplicates && lastStepField[field.key] != i {
			continue
		}
		if includeParent && field.key == "parent_outputs" {
			continue
		}
		appendField(field.raw)
	}
	if includeParent {
		if hasField {
			out = append(out, ',')
		}
		out = append(out, `"parent_outputs":`...)
		out = append(out, parentValue...)
	}
	out = append(out, '}')

	return out, true
}

func lastJSONFieldIndexes(fields []jsonObjectField) map[string]int {
	indexes := make(map[string]int, len(fields))
	for i := range fields {
		indexes[fields[i].key] = i
	}
	return indexes
}

func splitTopLevelJSONObjectFields(payload json.RawMessage) ([]jsonObjectField, bool, bool) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' || !json.Valid(trimmed) {
		return nil, false, false
	}
	if len(trimmed) == 2 {
		return nil, false, true
	}

	fields := make([]jsonObjectField, 0, 8)
	hasDuplicates := false
	i := 1
	for {
		i = skipJSONSpaces(trimmed, i)
		if i >= len(trimmed)-1 {
			return fields, hasDuplicates, true
		}

		fieldStart := i
		if trimmed[i] != '"' {
			return nil, false, false
		}
		keyStart := i
		keyEnd, escapedKey, ok := scanJSONString(trimmed, i)
		if !ok {
			return nil, false, false
		}
		var key string
		if escapedKey {
			var err error
			key, err = strconv.Unquote(string(trimmed[keyStart:keyEnd]))
			if err != nil {
				return nil, false, false
			}
		} else {
			key = string(trimmed[keyStart+1 : keyEnd-1])
		}
		i = skipJSONSpaces(trimmed, keyEnd)
		if i >= len(trimmed) || trimmed[i] != ':' {
			return nil, false, false
		}
		i++

		valueEnd, delimiter, ok := scanJSONObjectValue(trimmed, i)
		if !ok {
			return nil, false, false
		}
		fieldEnd := trimJSONRightSpaces(trimmed, fieldStart, valueEnd)
		for i := range fields {
			if fields[i].key == key {
				hasDuplicates = true
				break
			}
		}
		fields = append(fields, jsonObjectField{
			key: key,
			raw: bytes.TrimSpace(trimmed[fieldStart:fieldEnd]),
		})

		if delimiter == '}' {
			return fields, hasDuplicates, true
		}
		i = valueEnd + 1
	}
}

func skipJSONSpaces(in []byte, i int) int {
	for i < len(in) {
		switch in[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func trimJSONRightSpaces(in []byte, start, end int) int {
	for end > start {
		switch in[end-1] {
		case ' ', '\n', '\r', '\t':
			end--
		default:
			return end
		}
	}
	return end
}

func scanJSONString(in []byte, start int) (int, bool, bool) {
	escaped := false
	hasEscapedByte := false
	for i := start + 1; i < len(in); i++ {
		switch {
		case escaped:
			escaped = false
		case in[i] == '\\':
			hasEscapedByte = true
			escaped = true
		case in[i] == '"':
			return i + 1, hasEscapedByte, true
		}
	}
	return 0, false, false
}

func scanJSONObjectValue(in []byte, start int) (end int, delimiter byte, ok bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(in); i++ {
		c := in[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			if depth == 0 {
				if c != '}' {
					return 0, 0, false
				}
				return i, c, true
			}
			depth--
		case ',':
			if depth == 0 {
				return i, c, true
			}
		}
	}
	return 0, 0, false
}

func isBlankRaw(in json.RawMessage) bool {
	return firstNonSpaceByte(in) == 0
}

func firstNonSpaceByte(in json.RawMessage) byte {
	for _, b := range in {
		switch b {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return b
		}
	}
	return 0
}

func decodeJSONObject(payload json.RawMessage) (map[string]any, bool) {
	if isBlankRaw(payload) {
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

// enqueueApprovalNotification creates notification deliveries for all enabled
// notification channels in the project for a given approval event.
func (e *WorkflowEngine) enqueueApprovalNotification(ctx context.Context, projectID, eventType string, payload map[string]any) {
	channels, err := e.store.ListEnabledNotificationChannels(ctx, projectID)
	if err != nil {
		e.logger.Warn("failed to list notification channels for approval", "project_id", projectID, "error", err)
		return
	}
	if len(channels) == 0 {
		return
	}

	payloadBytes, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		e.logger.Warn("failed to marshal approval notification payload", "error", marshalErr)
		return
	}

	for _, ch := range channels {
		d := &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   projectID,
			EventType:   eventType,
			Payload:     payloadBytes,
			Status:      "pending",
			MaxAttempts: 3,
		}
		if err := e.store.CreateNotificationDelivery(ctx, d); err != nil {
			e.logger.Warn("failed to create notification delivery",
				"channel_id", ch.ID, "event_type", eventType, "error", err)
		}
	}
}

// enqueueApprovalNotifications creates notification deliveries for all enabled
// notification channels in the project when an approval is requested.
func (e *WorkflowEngine) enqueueApprovalNotifications(
	ctx context.Context,
	projectID string,
	approval *domain.WorkflowStepApproval,
	stepRun *domain.WorkflowStepRun,
	wfRun *domain.WorkflowRun,
) {
	e.enqueueApprovalNotification(ctx, projectID, domain.NotificationEventApprovalRequested, map[string]any{
		"approval_id":     approval.ID,
		"workflow_run_id": wfRun.ID,
		"workflow_id":     wfRun.WorkflowID,
		"step_ref":        stepRun.StepRef,
		"approvers":       approval.Approvers,
		"requested_at":    approval.RequestedAt,
		"expires_at":      approval.ExpiresAt,
	})
}
