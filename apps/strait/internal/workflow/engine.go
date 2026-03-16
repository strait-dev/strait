package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	otelTrace "go.opentelemetry.io/otel/trace"
)

// DefaultMaxNestingDepth is the nesting limit when none is specified on the step.
const DefaultMaxNestingDepth = 10

// EventTriggerNotifyFunc is called after an event trigger is created, for
// metrics and webhook notification. Must be safe to call concurrently.
type EventTriggerNotifyFunc func(trigger *domain.EventTrigger)

type WorkflowEngine struct {
	store           EngineStore
	queue           EngineQueue
	logger          *slog.Logger
	maxNestingDepth int
	onTriggerCreate EventTriggerNotifyFunc
	metrics         *telemetry.Metrics
	runInTx         func(ctx context.Context, fn func(s EngineStore) error) error
}

type EngineStore interface {
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error)
	CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error
	CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error
	CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error
	CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
	GetOrCreateWorkflowSnapshot(ctx context.Context, wf *domain.Workflow, steps []domain.WorkflowStep) (*domain.WorkflowSnapshot, error)
	CopyRunState(ctx context.Context, fromRunID, toRunID string) error
}

type EngineQueue interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

// NewWorkflowEngine creates a new workflow engine for triggering and managing workflow runs.
func NewWorkflowEngine(store EngineStore, queue EngineQueue, logger *slog.Logger) *WorkflowEngine {
	if logger == nil {
		logger = slog.Default()
	}

	e := &WorkflowEngine{
		store:           store,
		queue:           queue,
		logger:          logger,
		maxNestingDepth: DefaultMaxNestingDepth,
	}
	e.runInTx = func(_ context.Context, fn func(s EngineStore) error) error {
		return fn(e.store)
	}
	return e
}

// WithOnTriggerCreate sets a callback invoked after each event trigger is created.
func (e *WorkflowEngine) WithOnTriggerCreate(fn EventTriggerNotifyFunc) *WorkflowEngine {
	e.onTriggerCreate = fn
	return e
}

func (e *WorkflowEngine) WithMetrics(m *telemetry.Metrics) *WorkflowEngine {
	e.metrics = m
	return e
}

func (e *WorkflowEngine) recordTrigger(ctx context.Context, status string) {
	if e.metrics == nil {
		return
	}
	e.metrics.WorkflowTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
}

// WithRunInTx overrides the default pass-through transaction runner.
// The provided function must call fn with a transactional EngineStore.
func (e *WorkflowEngine) WithRunInTx(fn func(ctx context.Context, fn func(s EngineStore) error) error) *WorkflowEngine {
	e.runInTx = fn
	return e
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
	extraTags map[string]string,
) (*domain.WorkflowRun, error) {
	return e.triggerWorkflowInternal(ctx, workflowID, projectID, payload, triggeredBy, "", "", stepOverrides, extraTags)
}

// TriggerSubWorkflow triggers a workflow as a child of another workflow run.
func (e *WorkflowEngine) TriggerSubWorkflow(
	ctx context.Context,
	workflowID, projectID string,
	payload json.RawMessage,
	triggeredBy string,
	parentWorkflowRunID string,
	parentStepRunID string,
) (*domain.WorkflowRun, error) {
	return e.triggerWorkflowInternal(ctx, workflowID, projectID, payload, triggeredBy, parentWorkflowRunID, parentStepRunID, nil, nil)
}

func (e *WorkflowEngine) triggerWorkflowInternal(
	ctx context.Context,
	workflowID, projectID string,
	payload json.RawMessage,
	triggeredBy string,
	parentWorkflowRunID string,
	parentStepRunID string,
	stepOverrides []domain.StepOverride,
	extraTags map[string]string,
) (*domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.TriggerWorkflow")
	defer span.End()
	triggerStatus := "success"
	defer func() {
		e.recordTrigger(ctx, triggerStatus)
	}()

	wf, err := e.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		triggerStatus = "error"
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	if wf == nil {
		triggerStatus = "error"
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if !wf.Enabled {
		triggerStatus = "error"
		return nil, fmt.Errorf("workflow is disabled: %s", workflowID)
	}
	if projectID == "" {
		projectID = wf.ProjectID
	}
	if wf.ProjectID != "" && projectID != wf.ProjectID {
		triggerStatus = "error"
		return nil, fmt.Errorf("workflow %s does not belong to project %s", workflowID, projectID)
	}

	steps, err := e.store.ListStepsByWorkflowVersion(ctx, workflowID, wf.Version)
	if err != nil {
		triggerStatus = "error"
		return nil, fmt.Errorf("list workflow steps by version: %w", err)
	}

	// Apply step overrides to filter steps at trigger time.
	if len(stepOverrides) > 0 {
		steps, err = applyStepOverrides(steps, stepOverrides)
		if err != nil {
			triggerStatus = "error"
			return nil, fmt.Errorf("apply step overrides: %w", err)
		}
	}

	if err := ValidateDAG(steps); err != nil {
		triggerStatus = "error"
		return nil, fmt.Errorf("validate workflow dag: %w", err)
	}

	// Create an immutable snapshot of the workflow definition (metadata + steps)
	// so that in-flight runs are immune to live workflow_steps changes.
	// Snapshot failure is fatal — without it the run would silently read live
	// definitions, breaking the immutability contract.
	snapshot, snapshotErr := e.store.GetOrCreateWorkflowSnapshot(ctx, wf, steps)
	if snapshotErr != nil {
		triggerStatus = "error"
		return nil, fmt.Errorf("create workflow snapshot: %w", snapshotErr)
	}

	if wf.MaxConcurrentRuns > 0 {
		running, countErr := e.store.CountRunningWorkflowRuns(ctx, workflowID)
		if countErr != nil {
			triggerStatus = "error"
			return nil, fmt.Errorf("count running workflow runs: %w", countErr)
		}
		if running >= wf.MaxConcurrentRuns {
			triggerStatus = "error"
			return nil, fmt.Errorf("workflow %s: max concurrent runs (%d) reached", workflowID, wf.MaxConcurrentRuns)
		}
	}

	if triggeredBy == "" {
		triggeredBy = domain.TriggerManual
	}

	// Inherit workflow tags onto the run, then overlay any trigger-time tags.
	var runTags map[string]string
	if len(wf.Tags) > 0 || len(extraTags) > 0 {
		runTags = make(map[string]string, len(wf.Tags)+len(extraTags))
		maps.Copy(runTags, wf.Tags)
		maps.Copy(runTags, extraTags)
	}

	// Capture W3C trace context from the current OTel span for propagation.
	var traceCtx map[string]string
	spanCtx := otelTrace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		traceCtx = map[string]string{
			"traceparent": fmt.Sprintf("00-%s-%s-%s", spanCtx.TraceID(), spanCtx.SpanID(), spanCtx.TraceFlags()),
		}
		if spanCtx.TraceState().Len() > 0 {
			ts := spanCtx.TraceState().String()
			if len(ts) <= 512 {
				traceCtx["tracestate"] = ts
			}
		}
	}

	var snapshotID string
	if snapshot != nil {
		snapshotID = snapshot.ID
	}

	wfRun := &domain.WorkflowRun{
		WorkflowID:          workflowID,
		ProjectID:           projectID,
		Tags:                runTags,
		Status:              domain.WfStatusPending,
		TriggeredBy:         triggeredBy,
		WorkflowVersion:     wf.Version,
		WorkflowVersionID:   wf.VersionID,
		WorkflowSnapshotID:  snapshotID,
		MaxParallelSteps:    wf.MaxParallelSteps,
		Payload:             payload,
		ParentWorkflowRunID: parentWorkflowRunID,
		ParentStepRunID:     parentStepRunID,
		TraceContext:        traceCtx,
	}
	if wfRun.ID == "" {
		wfRun.ID = uuid.Must(uuid.NewV7()).String()
	}
	if wf.TimeoutSecs > 0 {
		expiresAt := time.Now().Add(time.Duration(wf.TimeoutSecs) * time.Second)
		wfRun.ExpiresAt = &expiresAt
	}
	now := time.Now()

	type rootToStart struct {
		stepRun *domain.WorkflowStepRun
		step    *domain.WorkflowStep
	}
	roots := make([]rootToStart, 0)
	stepRuns := make([]domain.WorkflowStepRun, 0, len(steps))
	for i := range steps {
		step := &steps[i]
		sr := domain.WorkflowStepRun{
			ID:             uuid.Must(uuid.NewV7()).String(),
			WorkflowRunID:  wfRun.ID,
			WorkflowStepID: step.ID,
			StepRef:        step.StepRef,
			DepsCompleted:  0,
			DepsRequired:   len(step.DependsOn),
		}
		if len(step.DependsOn) == 0 {
			sr.Status = domain.StepPending
			sr.DepsRequired = 0
		} else {
			sr.Status = domain.StepWaiting
		}
		stepRuns = append(stepRuns, sr)
	}

	type bootstrapStore interface {
		CreateWorkflowRunBootstrap(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error
	}
	if bs, ok := e.store.(bootstrapStore); ok {
		if err := bs.CreateWorkflowRunBootstrap(ctx, wfRun, stepRuns, now); err != nil {
			triggerStatus = "error"
			return nil, fmt.Errorf("create workflow bootstrap: %w", err)
		}
	} else {
		if err := e.store.CreateWorkflowRun(ctx, wfRun); err != nil {
			triggerStatus = "error"
			return nil, fmt.Errorf("create workflow run: %w", err)
		}
		if err := e.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": now}); err != nil {
			triggerStatus = "error"
			return nil, fmt.Errorf("start workflow run: %w", err)
		}
		for i := range stepRuns {
			if err := e.store.CreateWorkflowStepRun(ctx, &stepRuns[i]); err != nil {
				triggerStatus = "error"
				return nil, fmt.Errorf("create step run %s: %w", stepRuns[i].StepRef, err)
			}
		}
	}
	wfRun.Status = domain.WfStatusRunning
	wfRun.StartedAt = &now

	for i := range steps {
		step := &steps[i]
		sr := &stepRuns[i]
		if len(step.DependsOn) == 0 {
			roots = append(roots, rootToStart{stepRun: sr, step: step})
		}
	}

	runningStarts := 0
	runningByConcurrencyKey := make(map[string]int)
	for _, root := range roots {
		if wfRun.MaxParallelSteps > 0 && runningStarts >= wfRun.MaxParallelSteps {
			if err := e.store.UpdateStepRunStatus(ctx, root.stepRun.ID, domain.StepWaiting, nil); err != nil {
				triggerStatus = "error"
				return nil, fmt.Errorf("set root step waiting %s: %w", root.step.StepRef, err)
			}
			root.stepRun.Status = domain.StepWaiting
			continue
		}
		if root.step.ConcurrencyKey != "" && runningByConcurrencyKey[root.step.ConcurrencyKey] > 0 {
			if err := e.store.UpdateStepRunStatus(ctx, root.stepRun.ID, domain.StepWaiting, nil); err != nil {
				triggerStatus = "error"
				return nil, fmt.Errorf("set root step waiting by concurrency key %s: %w", root.step.StepRef, err)
			}
			root.stepRun.Status = domain.StepWaiting
			continue
		}
		if err := e.startStep(ctx, root.stepRun, root.step, wfRun, nil); err != nil {
			triggerStatus = "error"
			return nil, fmt.Errorf("start root step %s: %w", root.step.StepRef, err)
		}
		if root.stepRun.Status == domain.StepRunning {
			runningStarts++
			if root.step.ConcurrencyKey != "" {
				runningByConcurrencyKey[root.step.ConcurrencyKey]++
			}
		}
	}

	return wfRun, nil
}
