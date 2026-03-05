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

type WorkflowEngine struct {
	store  EngineStore
	queue  EngineQueue
	logger *slog.Logger
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
}

type EngineQueue interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

func NewWorkflowEngine(store EngineStore, queue EngineQueue, logger *slog.Logger) *WorkflowEngine {
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkflowEngine{
		store:  store,
		queue:  queue,
		logger: logger,
	}
}

func (e *WorkflowEngine) TriggerWorkflow(
	ctx context.Context,
	workflowID, projectID string,
	payload json.RawMessage,
	triggeredBy string,
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
	if err := ValidateDAG(steps); err != nil {
		return nil, fmt.Errorf("validate workflow dag: %w", err)
	}

	if wf.MaxConcurrentRuns > 0 {
		for {
			running, countErr := e.store.CountRunningWorkflowRuns(ctx, workflowID)
			if countErr != nil {
				return nil, fmt.Errorf("count running workflow runs: %w", countErr)
			}
			if running < wf.MaxConcurrentRuns {
				break
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
		WorkflowID:      workflowID,
		ProjectID:       projectID,
		Status:          domain.WfStatusPending,
		TriggeredBy:     triggeredBy,
		WorkflowVersion: wf.Version,
		Payload:         payload,
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

	for _, root := range roots {
		if err := e.startStep(ctx, root.stepRun, root.step, wfRun, nil); err != nil {
			return nil, fmt.Errorf("start root step %s: %w", root.step.StepRef, err)
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
			Status:            "pending",
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

	if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepRunning, map[string]any{"started_at": now}); err != nil {
		return fmt.Errorf("set step run running: %w", err)
	}
	stepRun.Status = domain.StepRunning
	stepRun.StartedAt = &now

	payload := mergePayloads(wfRun.Payload, step.Payload, mergedPayload)
	jobRun := &domain.JobRun{
		JobID:             step.JobID,
		ProjectID:         wfRun.ProjectID,
		Payload:           payload,
		TriggeredBy:       domain.TriggerWorkflow,
		WorkflowStepRunID: stepRun.ID,
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
