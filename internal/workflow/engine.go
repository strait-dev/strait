package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"strait/internal/domain"

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
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
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
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.TriggerWorkflow")
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

	// Inherit workflow tags onto the run.
	var runTags map[string]string
	if len(wf.Tags) > 0 {
		runTags = make(map[string]string, len(wf.Tags))
		maps.Copy(runTags, wf.Tags)
	}

	wfRun := &domain.WorkflowRun{
		WorkflowID:          workflowID,
		ProjectID:           projectID,
		Tags:                runTags,
		Status:              domain.WfStatusPending,
		TriggeredBy:         triggeredBy,
		WorkflowVersion:     wf.Version,
		WorkflowVersionID:   wf.VersionID,
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
