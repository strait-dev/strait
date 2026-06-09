package workflow

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	stepsCache      *workflowStepsVersionCache
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
	GetJobCostEstimate(ctx context.Context, jobID string) (*domain.JobCostEstimate, error)
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

type EngineQueue interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

type workflowCanaryStore interface {
	GetActiveCanaryDeployment(ctx context.Context, workflowID string) (*domain.CanaryDeployment, error)
	GetWorkflowVersion(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error)
}

type projectExecutionStateStore interface {
	IsProjectRunnable(ctx context.Context, projectID string) (bool, error)
}

type bootstrapStore interface {
	CreateWorkflowRunBootstrap(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error
}

type rootStepStart struct {
	stepRun *domain.WorkflowStepRun
	step    *domain.WorkflowStep
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

func (e *WorkflowEngine) WithDefinitionCaches(cfg WorkflowDefinitionCacheConfig) *WorkflowEngine {
	e.stepsCache = newWorkflowStepsVersionCache(cfg)
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

func (e *WorkflowEngine) listStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	key := workflowStepsVersionKey{WorkflowID: workflowID, Version: version}
	loader := func(loadCtx context.Context, loadKey workflowStepsVersionKey) ([]domain.WorkflowStep, error) {
		steps, err := e.store.ListStepsByWorkflowVersion(loadCtx, loadKey.WorkflowID, loadKey.Version)
		if err != nil {
			return nil, err
		}
		return domain.CloneWorkflowSteps(steps), nil
	}
	if e.stepsCache == nil {
		return loader(ctx, key)
	}
	return e.stepsCache.Load(ctx, key, loader)
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
	telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "workflow trigger requested", map[string]any{
		"workflow_id":            workflowID,
		"project_id":             projectID,
		"triggered_by":           triggeredBy,
		"parent_workflow_run_id": parentWorkflowRunID,
		"parent_step_run_id":     parentStepRunID,
	})
	triggerStatus := "success"
	defer func() {
		e.recordTrigger(ctx, triggerStatus)
	}()

	wf, projectID, err := e.loadRunnableWorkflow(ctx, workflowID, projectID)
	if err != nil {
		triggerStatus = "error"
		return nil, err
	}

	prepared, err := e.prepareWorkflowTrigger(ctx, wf, workflowID, stepOverrides)
	if err != nil {
		triggerStatus = "error"
		return nil, err
	}

	wfRun, roots, err := e.createRunnableWorkflowRun(
		ctx,
		wf,
		prepared,
		workflowID,
		projectID,
		payload,
		triggeredBy,
		parentWorkflowRunID,
		parentStepRunID,
		extraTags,
	)
	if err != nil {
		triggerStatus = "error"
		return nil, err
	}

	if err := e.startRootWorkflowSteps(ctx, wfRun, roots); err != nil {
		triggerStatus = "error"
		return nil, err
	}

	return wfRun, nil
}

type preparedWorkflowTrigger struct {
	steps    []domain.WorkflowStep
	snapshot *domain.WorkflowSnapshot
}

func (e *WorkflowEngine) prepareWorkflowTrigger(
	ctx context.Context,
	wf *domain.Workflow,
	workflowID string,
	stepOverrides []domain.StepOverride,
) (preparedWorkflowTrigger, error) {
	steps, err := e.listStepsByWorkflowVersion(ctx, workflowID, wf.Version)
	if err != nil {
		return preparedWorkflowTrigger{}, fmt.Errorf("list workflow steps by version: %w", err)
	}
	if len(stepOverrides) > 0 {
		steps, err = applyStepOverrides(steps, stepOverrides)
		if err != nil {
			return preparedWorkflowTrigger{}, fmt.Errorf("apply step overrides: %w", err)
		}
	}
	if err := ValidateDAG(steps); err != nil {
		return preparedWorkflowTrigger{}, fmt.Errorf("validate workflow dag: %w", err)
	}
	if err := e.enforceWorkflowConcurrency(ctx, workflowID, wf.MaxConcurrentRuns); err != nil {
		return preparedWorkflowTrigger{}, err
	}

	snapshot, err := e.store.GetOrCreateWorkflowSnapshot(ctx, wf, steps)
	if err != nil {
		return preparedWorkflowTrigger{}, fmt.Errorf("create workflow snapshot: %w", err)
	}
	return preparedWorkflowTrigger{steps: steps, snapshot: snapshot}, nil
}

func (e *WorkflowEngine) enforceWorkflowConcurrency(ctx context.Context, workflowID string, maxConcurrentRuns int) error {
	if maxConcurrentRuns <= 0 {
		return nil
	}
	running, err := e.store.CountRunningWorkflowRuns(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("count running workflow runs: %w", err)
	}
	if running >= maxConcurrentRuns {
		return fmt.Errorf("workflow %s: max concurrent runs (%d) reached", workflowID, maxConcurrentRuns)
	}
	return nil
}

func (e *WorkflowEngine) createRunnableWorkflowRun(
	ctx context.Context,
	wf *domain.Workflow,
	prepared preparedWorkflowTrigger,
	workflowID string,
	projectID string,
	payload json.RawMessage,
	triggeredBy string,
	parentWorkflowRunID string,
	parentStepRunID string,
	extraTags map[string]string,
) (*domain.WorkflowRun, []rootStepStart, error) {
	if triggeredBy == "" {
		triggeredBy = domain.TriggerManual
	}
	wfRun := newWorkflowRun(
		ctx,
		wf,
		workflowID,
		projectID,
		payload,
		triggeredBy,
		parentWorkflowRunID,
		parentStepRunID,
		prepared.snapshot,
		extraTags,
	)
	now := time.Now()
	stepRuns := initialWorkflowStepRuns(wfRun.ID, prepared.steps)
	if err := e.bootstrapWorkflowRun(ctx, wfRun, stepRuns, now); err != nil {
		return nil, nil, err
	}

	roots := rootWorkflowSteps(prepared.steps, stepRuns)
	wfRun.Status = domain.WfStatusRunning
	wfRun.StartedAt = &now
	recordWorkflowActiveRunDelta(ctx, wfRun.ProjectID, 1)
	telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "workflow run started", map[string]any{
		"workflow_id":     wfRun.WorkflowID,
		"workflow_run_id": wfRun.ID,
		"project_id":      wfRun.ProjectID,
		"version":         wfRun.WorkflowVersion,
		"step_count":      len(stepRuns),
		"root_count":      len(roots),
	})
	return wfRun, roots, nil
}

func (e *WorkflowEngine) loadRunnableWorkflow(ctx context.Context, workflowID, projectID string) (*domain.Workflow, string, error) {
	wf, err := e.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, "", fmt.Errorf("get workflow: %w", err)
	}
	if wf == nil {
		return nil, "", fmt.Errorf("workflow not found: %s", workflowID)
	}
	if !wf.Enabled {
		return nil, "", fmt.Errorf("workflow is disabled: %s", workflowID)
	}

	if projectID == "" {
		projectID = wf.ProjectID
	}
	if wf.ProjectID != "" && projectID != wf.ProjectID {
		return nil, "", fmt.Errorf("workflow %s does not belong to project %s", workflowID, projectID)
	}
	if projectChecker, ok := e.store.(projectExecutionStateStore); ok {
		runnable, err := projectChecker.IsProjectRunnable(ctx, projectID)
		if err != nil {
			return nil, "", fmt.Errorf("check workflow project execution state: %w", err)
		}
		if !runnable {
			return nil, "", fmt.Errorf("project %s is not active for workflow execution", projectID)
		}
	}
	if err := e.applyCanaryRouting(ctx, wf); err != nil {
		return nil, "", err
	}
	return wf, projectID, nil
}

func newWorkflowRun(
	ctx context.Context,
	wf *domain.Workflow,
	workflowID string,
	projectID string,
	payload json.RawMessage,
	triggeredBy string,
	parentWorkflowRunID string,
	parentStepRunID string,
	snapshot *domain.WorkflowSnapshot,
	extraTags map[string]string,
) *domain.WorkflowRun {
	wfRun := &domain.WorkflowRun{
		ID:                  uuid.Must(uuid.NewV7()).String(),
		WorkflowID:          workflowID,
		ProjectID:           projectID,
		Tags:                workflowRunTags(wf.Tags, extraTags),
		Status:              domain.WfStatusPending,
		TriggeredBy:         triggeredBy,
		WorkflowVersion:     wf.Version,
		WorkflowVersionID:   wf.VersionID,
		WorkflowSnapshotID:  workflowSnapshotID(snapshot),
		MaxParallelSteps:    wf.MaxParallelSteps,
		Payload:             payload,
		ParentWorkflowRunID: parentWorkflowRunID,
		ParentStepRunID:     parentStepRunID,
		TraceContext:        workflowTraceContext(ctx),
	}
	if wf.TimeoutSecs > 0 {
		expiresAt := time.Now().Add(time.Duration(wf.TimeoutSecs) * time.Second)
		wfRun.ExpiresAt = &expiresAt
	}
	return wfRun
}

func workflowRunTags(workflowTags, extraTags map[string]string) map[string]string {
	if len(workflowTags) == 0 && len(extraTags) == 0 {
		return nil
	}
	runTags := make(map[string]string, len(workflowTags)+len(extraTags))
	maps.Copy(runTags, workflowTags)
	maps.Copy(runTags, extraTags)
	return runTags
}

func workflowTraceContext(ctx context.Context) map[string]string {
	spanCtx := otelTrace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}

	traceCtx := map[string]string{
		"traceparent": workflowTraceparent(spanCtx),
	}
	if spanCtx.TraceState().Len() > 0 {
		traceState := spanCtx.TraceState().String()
		if len(traceState) <= 512 {
			traceCtx["tracestate"] = traceState
		}
	}
	return traceCtx
}

func workflowTraceparent(spanCtx otelTrace.SpanContext) string {
	const hexChars = "0123456789abcdef"

	traceID := spanCtx.TraceID()
	spanID := spanCtx.SpanID()
	flags := byte(spanCtx.TraceFlags())

	var out [55]byte
	out[0] = '0'
	out[1] = '0'
	out[2] = '-'
	hex.Encode(out[3:35], traceID[:])
	out[35] = '-'
	hex.Encode(out[36:52], spanID[:])
	out[52] = '-'
	out[53] = hexChars[flags>>4]
	out[54] = hexChars[flags&0x0f]
	return string(out[:])
}

func workflowSnapshotID(snapshot *domain.WorkflowSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return snapshot.ID
}

func initialWorkflowStepRuns(workflowRunID string, steps []domain.WorkflowStep) []domain.WorkflowStepRun {
	stepRuns := make([]domain.WorkflowStepRun, 0, len(steps))
	for i := range steps {
		step := &steps[i]
		sr := domain.WorkflowStepRun{
			ID:             uuid.Must(uuid.NewV7()).String(),
			WorkflowRunID:  workflowRunID,
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
	return stepRuns
}

func (e *WorkflowEngine) bootstrapWorkflowRun(
	ctx context.Context,
	wfRun *domain.WorkflowRun,
	stepRuns []domain.WorkflowStepRun,
	now time.Time,
) error {
	if bs, ok := e.store.(bootstrapStore); ok {
		if err := bs.CreateWorkflowRunBootstrap(ctx, wfRun, stepRuns, now); err != nil {
			return fmt.Errorf("create workflow bootstrap: %w", err)
		}
		return nil
	}
	if err := e.store.CreateWorkflowRun(ctx, wfRun); err != nil {
		return fmt.Errorf("create workflow run: %w", err)
	}
	if err := e.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": now}); err != nil {
		return fmt.Errorf("start workflow run: %w", err)
	}
	for i := range stepRuns {
		if err := e.store.CreateWorkflowStepRun(ctx, &stepRuns[i]); err != nil {
			return fmt.Errorf("create step run %s: %w", stepRuns[i].StepRef, err)
		}
	}
	return nil
}

func rootWorkflowSteps(steps []domain.WorkflowStep, stepRuns []domain.WorkflowStepRun) []rootStepStart {
	roots := make([]rootStepStart, 0)
	for i := range steps {
		if len(steps[i].DependsOn) == 0 {
			roots = append(roots, rootStepStart{stepRun: &stepRuns[i], step: &steps[i]})
		}
	}
	return roots
}

func (e *WorkflowEngine) startRootWorkflowSteps(
	ctx context.Context,
	wfRun *domain.WorkflowRun,
	roots []rootStepStart,
) error {
	runningStarts := 0
	runningByConcurrencyKey := make(map[string]int)
	for _, root := range roots {
		if wfRun.MaxParallelSteps > 0 && runningStarts >= wfRun.MaxParallelSteps {
			if err := e.store.UpdateStepRunStatus(ctx, root.stepRun.ID, domain.StepWaiting, nil); err != nil {
				return fmt.Errorf("set root step waiting %s: %w", root.step.StepRef, err)
			}
			root.stepRun.Status = domain.StepWaiting
			continue
		}
		if root.step.ConcurrencyKey != "" && runningByConcurrencyKey[root.step.ConcurrencyKey] > 0 {
			if err := e.store.UpdateStepRunStatus(ctx, root.stepRun.ID, domain.StepWaiting, nil); err != nil {
				return fmt.Errorf("set root step waiting by concurrency key %s: %w", root.step.StepRef, err)
			}
			root.stepRun.Status = domain.StepWaiting
			continue
		}
		if err := e.startStep(ctx, root.stepRun, root.step, wfRun, nil); err != nil {
			return fmt.Errorf("start root step %s: %w", root.step.StepRef, err)
		}
		if root.stepRun.Status == domain.StepRunning {
			runningStarts++
			if root.step.ConcurrencyKey != "" {
				runningByConcurrencyKey[root.step.ConcurrencyKey]++
			}
		}
	}
	return nil
}

func (e *WorkflowEngine) applyCanaryRouting(ctx context.Context, wf *domain.Workflow) error {
	canaryStore, ok := e.store.(workflowCanaryStore)
	if !ok {
		return nil
	}
	canary, err := canaryStore.GetActiveCanaryDeployment(ctx, wf.ID)
	if err != nil {
		if errors.Is(err, domain.ErrCanaryNotFound) {
			return nil
		}
		return fmt.Errorf("load active workflow canary: %w", err)
	}
	if canary == nil || canary.ProjectID != wf.ProjectID {
		return nil
	}
	resolved := NewCanaryRouter().ResolveVersion(&CanaryDeployment{
		WorkflowID:    canary.WorkflowID,
		ProjectID:     canary.ProjectID,
		SourceVersion: canary.SourceVersion,
		TargetVersion: canary.TargetVersion,
		TrafficPct:    canary.TrafficPct,
		Status:        CanaryStatus(canary.Status),
	})
	if resolved == 0 || resolved == wf.Version {
		return nil
	}
	version, err := canaryStore.GetWorkflowVersion(ctx, wf.ID, resolved)
	if err != nil {
		return fmt.Errorf("load canary workflow version %d: %w", resolved, err)
	}
	applyWorkflowVersion(wf, version)
	return nil
}

func applyWorkflowVersion(wf *domain.Workflow, version *domain.WorkflowVersion) {
	wf.Version = version.Version
	wf.VersionID = version.VersionID
	wf.Name = version.Name
	wf.Slug = version.Slug
	wf.Description = version.Description
	wf.Enabled = version.Enabled
	wf.TimeoutSecs = version.TimeoutSecs
	wf.MaxConcurrentRuns = version.MaxConcurrentRuns
	wf.MaxParallelSteps = version.MaxParallelSteps
	wf.Cron = version.Cron
	wf.CronTimezone = version.CronTimezone
	wf.SkipIfRunning = version.SkipIfRunning
}
