package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
	storepkg "strait/internal/store"
	"strait/internal/telemetry"

	"go.opentelemetry.io/otel"
)

type StepCallback struct {
	store      CallbackStore
	engine     *WorkflowEngine
	logger     *slog.Logger
	metrics    *telemetry.Metrics
	chExporter *clickhouse.Exporter
	statusHook WorkflowRunStatusHook
	stepsCache *workflowStepsVersionCache
}

// WorkflowRunStatusHook observes workflow run transitions completed by the
// callback path. It must be best-effort and must not block workflow progression.
type WorkflowRunStatusHook func(ctx context.Context, run *domain.WorkflowRun, from, to domain.WorkflowRunStatus, reason string)

type CallbackStore interface {
	GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error)
	GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	UpdateStepRunStatusFrom(ctx context.Context, id string, from, to domain.StepRunStatus, fields map[string]any) error
	IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]storepkg.StepDepResult, error)
	IncrementStepDepsIncludingFailed(ctx context.Context, workflowRunID string, completedStepRef string) ([]storepkg.StepDepResult, error)
	IncrementStepRunAttempt(ctx context.Context, id string, newAttempt int) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	ListRunnableStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error)
	ListRunningStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error)
	ListStepRunStatusesByWorkflowRun(ctx context.Context, workflowRunID string) (map[string]domain.StepRunStatus, error)
	CountNonTerminalStepRuns(ctx context.Context, workflowRunID string) (int, error)
	ListFailedStepRunRefs(ctx context.Context, workflowRunID string) ([]string, error)
	CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	SkipStepRunsByRefs(ctx context.Context, workflowRunID string, refs []string, finishedAt time.Time) (int64, error)
	GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	ScheduleRetry(ctx context.Context, runID string, at time.Time, attempt int) error
	ListDependentsByDependencyJob(ctx context.Context, dependsOnJobID string) ([]domain.JobDependency, error)
	ListWaitingRunsByJobIDs(ctx context.Context, jobIDs []string, limit int) ([]domain.JobRun, error)
	AreJobDependenciesSatisfied(ctx context.Context, run *domain.JobRun) (bool, error)
	GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
	GetEventTriggerByStepRunID(ctx context.Context, stepRunID string) (*domain.EventTrigger, error)
	GetEventTriggerByEventKeyForProject(ctx context.Context, eventKey, projectID string) (*domain.EventTrigger, error)
	UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	AdvisoryXactLock(ctx context.Context, lockID int64) error
	CreateWorkflowStepDecision(ctx context.Context, d *domain.WorkflowStepDecision) error
	GetWorkflowSnapshot(ctx context.Context, projectID, id string) (*domain.WorkflowSnapshot, error)
	RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error)
}

type compensationCallbackStore interface {
	MarkCompensationRunTerminalByJobRunID(ctx context.Context, jobRunID string, status string, output json.RawMessage, errMsg string, finishedAt time.Time) (*domain.CompensationRun, error)
	CountIncompleteCompensationRuns(ctx context.Context, workflowRunID string) (int, error)
}

type pausedRunQueueRequeuer interface {
	RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error)
}

type progressionEventCreator interface {
	CreateWorkflowProgressionEvent(ctx context.Context, workflowRunID, stepRunID, stepRef, status string) error
}

// NewStepCallback creates a new step callback handler for workflow progression.
func NewStepCallback(store CallbackStore, engine *WorkflowEngine, logger *slog.Logger) *StepCallback {
	if logger == nil {
		logger = slog.Default()
	}

	return &StepCallback{
		store:  store,
		engine: engine,
		logger: logger,
	}
}

// wfCtx caches immutable workflow data for a single callback invocation.
// Step definitions for a given (workflowID, version) never change after creation,
// so fetching them once per entry point is safe.
type wfCtx struct {
	run       *domain.WorkflowRun
	steps     []domain.WorkflowStep
	stepByRef map[string]domain.WorkflowStep
	stepIndex map[string]int
}

func (s *StepCallback) loadWfCtx(ctx context.Context, workflowRunID string) (*wfCtx, error) {
	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run not found: %s", workflowRunID)
	}

	steps, err := s.loadStepDefinitions(ctx, wfRun)
	if err != nil {
		return nil, err
	}

	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	stepIndex := make(map[string]int, len(steps))
	for i, st := range steps {
		stepByRef[st.StepRef] = st
		stepIndex[st.StepRef] = i
	}
	return &wfCtx{run: wfRun, steps: steps, stepByRef: stepByRef, stepIndex: stepIndex}, nil
}

// loadStepDefinitions reads step definitions from the snapshot when available,
// falling back to the live workflow_version_steps table for pre-snapshot runs.
func (s *StepCallback) loadStepDefinitions(ctx context.Context, wfRun *domain.WorkflowRun) ([]domain.WorkflowStep, error) {
	if wfRun.WorkflowSnapshotID != "" {
		snapshot, err := s.store.GetWorkflowSnapshot(ctx, wfRun.ProjectID, wfRun.WorkflowSnapshotID)
		if err != nil {
			s.logger.Warn("failed to load snapshot, falling back to live table",
				"workflow_run_id", wfRun.ID, "snapshot_id", wfRun.WorkflowSnapshotID, "error", err)
		} else if snapshot != nil {
			def, parseErr := storepkg.ParseSnapshotDefinition(snapshot.Definition)
			if parseErr != nil {
				s.logger.Warn("failed to parse snapshot, falling back to live table",
					"workflow_run_id", wfRun.ID, "snapshot_id", wfRun.WorkflowSnapshotID, "error", parseErr)
			} else {
				return def.Steps, nil
			}
		}
	}

	// Fallback: read from live workflow_version_steps table.
	steps, err := s.listStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return nil, fmt.Errorf("list steps by workflow version: %w", err)
	}
	return steps, nil
}

func (s *StepCallback) WithDefinitionCaches(cfg WorkflowDefinitionCacheConfig) *StepCallback {
	s.stepsCache = newWorkflowStepsVersionCache(cfg)
	return s
}

func (s *StepCallback) listStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	key := workflowStepsVersionKey{WorkflowID: workflowID, Version: version}
	loader := func(loadCtx context.Context, loadKey workflowStepsVersionKey) ([]domain.WorkflowStep, error) {
		steps, err := s.store.ListStepsByWorkflowVersion(loadCtx, loadKey.WorkflowID, loadKey.Version)
		if err != nil {
			return nil, err
		}
		return domain.CloneWorkflowSteps(steps), nil
	}
	if s.stepsCache == nil {
		return loader(ctx, key)
	}
	return s.stepsCache.Load(ctx, key, loader)
}

func (s *StepCallback) WithMetrics(m *telemetry.Metrics) *StepCallback {
	s.metrics = m
	return s
}

// WithChExporter attaches the ClickHouse exporter for step analytics.
func (s *StepCallback) WithChExporter(e *clickhouse.Exporter) *StepCallback {
	s.chExporter = e
	return s
}

// WithStatusHook attaches a best-effort observer for workflow run transitions
// produced by callback-driven progression.
func (s *StepCallback) WithStatusHook(hook WorkflowRunStatusHook) *StepCallback {
	s.statusHook = hook
	return s
}

func (s *StepCallback) publishWorkflowRunStatus(ctx context.Context, run *domain.WorkflowRun, from, to domain.WorkflowRunStatus, reason string) {
	if s.statusHook == nil || run == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in workflow status hook",
				"workflow_run_id", run.ID,
				"from", from,
				"to", to,
				"panic", r)
		}
	}()
	s.statusHook(ctx, run, from, to, reason)
}

func approvalAuditActor(actor string) (string, string) {
	if actor == "" {
		return "system", "system"
	}
	if actor == "system" {
		return actor, "system"
	}
	if strings.HasPrefix(actor, "system:") {
		return actor, "system"
	}
	if strings.HasPrefix(actor, "apikey:") {
		return actor, "api_key"
	}
	return actor, "user"
}

func (s *StepCallback) emitApprovalAuditEvent(
	ctx context.Context,
	wfRun *domain.WorkflowRun,
	stepRun *domain.WorkflowStepRun,
	approval *domain.WorkflowStepApproval,
	actor,
	action,
	decision,
	reason string,
) {
	if wfRun == nil || stepRun == nil || approval == nil {
		return
	}

	details := map[string]any{
		"workflow_run_id": wfRun.ID,
		"workflow_id":     wfRun.WorkflowID,
		"step_ref":        stepRun.StepRef,
		"step_run_id":     stepRun.ID,
		"decision":        decision,
	}
	if reason != "" {
		details["reason"] = reason
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		s.logger.Warn("failed to marshal approval audit event details",
			"approval_id", approval.ID,
			"action", action,
			"error", err)
		return
	}

	actorID, actorType := approvalAuditActor(actor)
	if err := s.store.CreateAuditEvent(ctx, &domain.AuditEvent{
		ProjectID:    wfRun.ProjectID,
		ActorID:      actorID,
		ActorType:    actorType,
		Action:       action,
		ResourceType: "workflow_step_approval",
		ResourceID:   approval.ID,
		Details:      detailsJSON,
	}); err != nil {
		s.logger.Warn("failed to create approval audit event",
			"approval_id", approval.ID,
			"action", action,
			"error", err)
	}
}

func (s *StepCallback) OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.OnJobRunTerminal")
	defer span.End()

	if run == nil {
		return nil
	}
	defer s.tryReleaseDependencyRuns(ctx, run)

	if handled, err := s.handleCompensationJobTerminal(ctx, run); handled || err != nil {
		return err
	}

	if run.WorkflowStepRunID == "" {
		return nil
	}

	stepRun, err := s.store.GetStepRunByJobRunID(ctx, run.ID)
	if err != nil {
		s.logger.Error("failed to get step run by job run", "job_run_id", run.ID, "error", err)
		return fmt.Errorf("get step run by job run id: %w", err)
	}
	if stepRun == nil || stepRun.Status.IsTerminal() {
		return nil
	}

	wc, wcErr := s.loadWfCtx(ctx, stepRun.WorkflowRunID)
	if wcErr != nil {
		s.logger.Error("failed to load workflow context", "workflow_run_id", stepRun.WorkflowRunID, "error", wcErr)
		return fmt.Errorf("load workflow context: %w", wcErr)
	}

	stepStatus, fields := mapRunStatusToStepStatus(run)

	// Apply output transformation for completed steps before persisting.
	if stepStatus == domain.StepCompleted {
		if rawOut, ok := fields["output"].(json.RawMessage); ok && len(rawOut) > 0 {
			if step, ok := wc.stepByRef[stepRun.StepRef]; ok && step.OutputTransform != "" {
				transformed, transformErr := ApplyOutputTransform(rawOut, step.OutputTransform)
				if transformErr != nil {
					s.logger.Warn("output transform failed, keeping original output",
						"step_ref", stepRun.StepRef, "transform", step.OutputTransform, "error", transformErr)
				} else {
					fields["output"] = transformed
				}
			}
		}
	}

	finishedAt := time.Now()
	fields["finished_at"] = finishedAt
	previousStatus := stepRun.Status
	if err := s.store.UpdateStepRunStatusFrom(ctx, stepRun.ID, stepRun.Status, stepStatus, fields); err != nil {
		s.logger.Error("failed to update step run terminal status", "step_run_id", stepRun.ID, "from", stepRun.Status, "status", stepStatus, "error", err)
		return fmt.Errorf("update step run terminal status: %w", err)
	}
	recordWorkflowStepTransition(ctx, string(previousStatus), string(stepStatus))
	recordWorkflowStepDuration(ctx, workflowStepKind(wc, stepRun), workflowStepOutcome(stepStatus), stepRun.StartedAt, finishedAt)

	stepRun.Status = stepStatus
	if out, ok := fields["output"].(json.RawMessage); ok {
		stepRun.Output = out
	}
	if stepErr, ok := fields["error"].(string); ok {
		stepRun.Error = stepErr
	}

	// Enqueue step analytics to ClickHouse.
	s.enqueueStepAnalytics(stepRun, wc)

	// Check if step retry is needed before handling failure
	if stepStatus == domain.StepFailed {
		if shouldRetry, nextRetryAt, newAttempt, err := s.checkStepRetry(ctx, stepRun, run, wc); err != nil {
			s.logger.Error("failed to check step retry", "step_run_id", stepRun.ID, "error", err)
			return fmt.Errorf("check step retry: %w", err)
		} else if shouldRetry {
			// Schedule retry for the job run
			if err := s.scheduleStepRetry(ctx, run, stepRun, nextRetryAt, newAttempt); err != nil {
				s.logger.Error("failed to schedule step retry", "step_run_id", stepRun.ID, "error", err)
				return fmt.Errorf("schedule step retry: %w", err)
			}
			return nil
		}
	}

	switch stepStatus {
	case domain.StepCompleted:
		// Auto-emit event if step has event_emit_key configured.
		s.tryEmitEvent(ctx, stepRun, wc)

		creator, ok := s.store.(progressionEventCreator)
		if !ok {
			return fmt.Errorf("workflow progression event store not available")
		}
		if err := creator.CreateWorkflowProgressionEvent(ctx, stepRun.WorkflowRunID, stepRun.ID, stepRun.StepRef, string(stepStatus)); err != nil {
			return fmt.Errorf("create workflow progression event: %w", err)
		}
		return nil
	case domain.StepFailed:
		if err := s.handleFailedStep(ctx, stepRun, wc); err != nil {
			s.logger.Error("failed to process failed step", "step_ref", stepRun.StepRef, "error", err)
			return fmt.Errorf("process failed step %s: %w", stepRun.StepRef, err)
		}
		return nil
	default:
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID, wc)
	}
}

func (s *StepCallback) handleCompensationJobTerminal(ctx context.Context, run *domain.JobRun) (bool, error) {
	if !isCompensationJobRun(run) {
		return false, nil
	}
	compStore, ok := s.store.(compensationCallbackStore)
	if !ok {
		return true, fmt.Errorf("workflow compensation store unavailable")
	}

	status := domain.CompensationCompleted
	errMsg := run.Error
	if run.Status != domain.StatusCompleted {
		status = domain.CompensationFailed
		if errMsg == "" {
			errMsg = fmt.Sprintf("compensation job ended with status %s", run.Status)
		}
	}

	finishedAt := time.Now()
	compRun, err := compStore.MarkCompensationRunTerminalByJobRunID(ctx, run.ID, status, run.Result, errMsg, finishedAt)
	if err != nil {
		return true, fmt.Errorf("mark compensation run terminal: %w", err)
	}

	if status == domain.CompensationFailed {
		if err := s.store.UpdateWorkflowRunStatus(ctx, compRun.WorkflowRunID, domain.WfStatusCompensating, domain.WfStatusCompensationFailed, map[string]any{
			"finished_at": finishedAt,
			"error":       errMsg,
		}); err != nil {
			return true, fmt.Errorf("mark workflow compensation failed: %w", err)
		}
		return true, nil
	}

	remaining, err := compStore.CountIncompleteCompensationRuns(ctx, compRun.WorkflowRunID)
	if err != nil {
		return true, fmt.Errorf("count incomplete compensation runs: %w", err)
	}
	if remaining == 0 {
		if err := s.store.UpdateWorkflowRunStatus(ctx, compRun.WorkflowRunID, domain.WfStatusCompensating, domain.WfStatusCompensated, map[string]any{
			"finished_at": finishedAt,
		}); err != nil {
			return true, fmt.Errorf("mark workflow compensated: %w", err)
		}
	}

	return true, nil
}

func isCompensationJobRun(run *domain.JobRun) bool {
	if run == nil {
		return false
	}
	if run.Metadata == nil {
		return false
	}
	return run.Metadata[domain.RunMetadataCompensationRunID] != ""
}

// OnEventReceived handles progression when an external event is received for a workflow step.
func (s *StepCallback) OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.OnEventReceived")
	defer span.End()

	if trigger == nil {
		return nil
	}
	isWorkflowStepEvent := trigger.SourceType == domain.EventSourceWorkflowStep
	hasStepRunID := trigger.WorkflowStepRunID != ""
	if !isWorkflowStepEvent || !hasStepRunID {
		return nil
	}

	targetStepRun, err := s.store.GetWorkflowStepRun(ctx, trigger.WorkflowStepRunID)
	if err != nil {
		return fmt.Errorf("get step run for event trigger: %w", err)
	}
	if targetStepRun == nil {
		return nil
	}

	// When handleSendEvent wraps the trigger + step update in runInTx,
	// the step is already completed before this callback runs. Skip the
	// status update but still drive fan-in and workflow completion.
	if !targetStepRun.Status.IsTerminal() {
		now := time.Now()
		fields := map[string]any{
			"finished_at": now,
		}
		if len(trigger.ResponsePayload) > 0 {
			fields["output"] = trigger.ResponsePayload
		}
		previousStatus := targetStepRun.Status
		if err := s.store.UpdateStepRunStatusFrom(ctx, targetStepRun.ID, targetStepRun.Status, domain.StepCompleted, fields); err != nil {
			return fmt.Errorf("update step run status for event trigger: %w", err)
		}
		recordWorkflowStepTransition(ctx, string(previousStatus), string(domain.StepCompleted))
		recordWorkflowDurableWait(ctx, targetStepRun.StartedAt, now)
		targetStepRun.Status = domain.StepCompleted
		if len(trigger.ResponsePayload) > 0 {
			targetStepRun.Output = trigger.ResponsePayload
		}
	} else if targetStepRun.Status != domain.StepCompleted {
		// Step is terminal but not completed (e.g., failed, canceled). No progression.
		return nil
	}

	wc, wcErr := s.loadWfCtx(ctx, targetStepRun.WorkflowRunID)
	if wcErr != nil {
		return fmt.Errorf("load workflow context: %w", wcErr)
	}
	recordWorkflowStepDuration(ctx, workflowStepKind(wc, targetStepRun), workflowStepOutcome(targetStepRun.Status), targetStepRun.StartedAt, time.Now())

	// Auto-emit event if step has event_emit_key configured.
	s.tryEmitEvent(ctx, targetStepRun, wc)

	// Fan-in and start ready children.
	if err := s.fanInAndStartReadyChildren(ctx, targetStepRun, wc, false); err != nil {
		s.logger.Error("failed to process event-completed step", "step_ref", targetStepRun.StepRef, "error", err)
		return fmt.Errorf("process event-completed step %s: %w", targetStepRun.StepRef, err)
	}

	return s.checkWorkflowCompletion(ctx, targetStepRun.WorkflowRunID, wc)
}

// OnStepCompleted handles workflow progression after a step is externally completed
// (e.g., by the reaper for sleep triggers). The step must already be in a terminal state.
func (s *StepCallback) OnStepCompleted(ctx context.Context, workflowRunID string, stepRunID string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.OnStepCompleted")
	defer span.End()

	target, err := s.store.GetWorkflowStepRun(ctx, stepRunID)
	if err != nil {
		s.logger.Error("OnStepCompleted: failed to get step run", "workflow_run_id", workflowRunID, "step_run_id", stepRunID, "error", err)
		return
	}
	if target == nil {
		return
	}

	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		s.logger.Error("OnStepCompleted: failed to load workflow context", "workflow_run_id", workflowRunID, "error", wcErr)
		return
	}

	s.tryEmitEvent(ctx, target, wc)

	if err := s.fanInAndStartReadyChildren(ctx, target, wc, false); err != nil {
		s.logger.Error("OnStepCompleted: failed to advance workflow", "step_ref", target.StepRef, "error", err)
		return
	}

	if err := s.checkWorkflowCompletion(ctx, workflowRunID, wc); err != nil {
		s.logger.Error("OnStepCompleted: failed to check workflow completion", "workflow_run_id", workflowRunID, "error", err)
	}
}

// OnStepFailed handles workflow progression after a step fails, delegating to
// handleFailedStep which respects the step's on_failure policy.
func (s *StepCallback) OnStepFailed(ctx context.Context, workflowRunID string, stepRunID string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.OnStepFailed")
	defer span.End()

	target, err := s.store.GetWorkflowStepRun(ctx, stepRunID)
	if err != nil {
		s.logger.Error("OnStepFailed: failed to get step run", "workflow_run_id", workflowRunID, "step_run_id", stepRunID, "error", err)
		return
	}
	if target == nil {
		s.logger.Warn("OnStepFailed: step run not found", "step_run_id", stepRunID)
		return
	}

	wc, wcErr := s.loadWfCtx(ctx, workflowRunID)
	if wcErr != nil {
		s.logger.Error("OnStepFailed: failed to load workflow context", "error", wcErr)
		return
	}

	if err := s.handleFailedStep(ctx, target, wc); err != nil {
		s.logger.Error("OnStepFailed: failed to handle step failure", "step_ref", target.StepRef, "error", err)
	}
}

func mapRunStatusToStepStatus(run *domain.JobRun) (domain.StepRunStatus, map[string]any) {
	fields := make(map[string]any)

	switch run.Status {
	case domain.StatusCompleted:
		if len(run.Result) > 0 {
			fields["output"] = run.Result
		}
		return domain.StepCompleted, fields
	case domain.StatusCanceled:
		return domain.StepCanceled, fields
	case domain.StatusFailed, domain.StatusDeadLetter, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed, domain.StatusExpired:
		if run.Error != "" {
			fields["error"] = run.Error
		} else {
			fields["error"] = fmt.Sprintf("job run ended with status %s", run.Status)
		}
		return domain.StepFailed, fields
	default:
		if run.Error != "" {
			fields["error"] = run.Error
		} else {
			fields["error"] = fmt.Sprintf("job run ended with unexpected status %s", run.Status)
		}
		return domain.StepFailed, fields
	}
}

// tryEmitEvent uses the cached step definitions to check if a completed step
// has event_emit_key configured, and auto-emits an event if so. Non-fatal on failure.
func (s *StepCallback) tryEmitEvent(ctx context.Context, stepRun *domain.WorkflowStepRun, wc *wfCtx) {
	step, ok := wc.stepByRef[stepRun.StepRef]
	if !ok {
		return
	}
	s.emitEventIfConfigured(ctx, stepRun, &step, wc.run)
}

// emitEventIfConfigured checks if a step has event_emit_key set and if so,
// auto-resolves a matching waiting trigger with the step's output.
// The event_emit_key is template-rendered using the workflow run payload.
func (s *StepCallback) emitEventIfConfigured(ctx context.Context, stepRun *domain.WorkflowStepRun, step *domain.WorkflowStep, wfRun *domain.WorkflowRun) {
	if step == nil || step.EventEmitKey == "" {
		return
	}

	emitKey := renderStringTemplate(step.EventEmitKey, wfRun.Payload)
	if emitKey == "" {
		return
	}
	if wfRun == nil || wfRun.ProjectID == "" {
		s.logger.Error("emit event: missing workflow project scope", "event_key", emitKey)
		return
	}

	trigger, err := s.store.GetEventTriggerByEventKeyForProject(ctx, emitKey, wfRun.ProjectID)
	if err != nil {
		s.logger.Error("emit event: failed to get trigger", "event_key", emitKey, "error", err)
		return
	}
	if trigger == nil || trigger.Status != domain.EventTriggerStatusWaiting {
		return
	}

	now := time.Now()
	if err := s.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, stepRun.Output, &now, ""); err != nil {
		s.logger.Error("emit event: failed to resolve trigger", "event_key", emitKey, "trigger_id", trigger.ID, "error", err)
		return
	}

	// Update the in-memory trigger so OnEventReceived sees the correct state.
	trigger.Status = domain.EventTriggerStatusReceived
	trigger.ResponsePayload = stepRun.Output
	trigger.ReceivedAt = &now

	// Resume the waiting step/run — without this, the target step stays
	// in 'waiting' until the reconciliation reaper picks it up.
	switch trigger.SourceType {
	case domain.EventSourceWorkflowStep:
		if err := s.OnEventReceived(ctx, trigger); err != nil {
			s.logger.Error("emit event: failed to resume waiting step", "event_key", emitKey, "trigger_id", trigger.ID, "error", err)
		}
	case domain.EventSourceJobRun:
		if trigger.JobRunID != "" {
			if err := s.store.UpdateRunStatus(ctx, trigger.JobRunID, domain.StatusWaiting, domain.StatusQueued, nil); err != nil {
				s.logger.Error("emit event: failed to re-queue job run", "event_key", emitKey, "job_run_id", trigger.JobRunID, "error", err)
			}
		}
	}

	s.logger.Info("auto-emitted event on step completion", "step_ref", step.StepRef, "event_key", emitKey, "trigger_id", trigger.ID)
}

func (s *StepCallback) recordDecision(ctx context.Context, stepRun *domain.WorkflowStepRun, decisionType, decision, explanation string, details json.RawMessage) {
	if stepRun == nil || stepRun.WorkflowRunID == "" {
		return
	}
	if err := s.store.CreateWorkflowStepDecision(ctx, &domain.WorkflowStepDecision{
		WorkflowRunID: stepRun.WorkflowRunID,
		StepRunID:     stepRun.ID,
		StepRef:       stepRun.StepRef,
		DecisionType:  decisionType,
		Decision:      decision,
		Explanation:   explanation,
		Details:       details,
	}); err != nil {
		slog.Warn("failed to record step decision", "step_run_id", stepRun.ID, "error", err)
	}
}

// enqueueStepAnalytics sends a WorkflowStepAnalyticsRecord to ClickHouse.
func (s *StepCallback) enqueueStepAnalytics(stepRun *domain.WorkflowStepRun, wc *wfCtx) {
	if s.chExporter == nil {
		return
	}
	if stepRun == nil {
		return
	}
	if wc == nil {
		return
	}
	if wc.run == nil {
		return
	}
	var durationMs uint64
	if stepRun.StartedAt != nil {
		finishedAt := time.Now()
		if stepRun.FinishedAt != nil {
			finishedAt = *stepRun.FinishedAt
		}
		durationMs = uint64(max(finishedAt.Sub(*stepRun.StartedAt).Milliseconds(), 0))
	}
	s.chExporter.Enqueue(clickhouse.WorkflowStepAnalyticsRecord{
		StepRunID:     stepRun.ID,
		WorkflowRunID: stepRun.WorkflowRunID,
		WorkflowID:    wc.run.WorkflowID,
		ProjectID:     wc.run.ProjectID,
		StepRef:       stepRun.StepRef,
		Status:        string(stepRun.Status),
		DurationMs:    durationMs,
		Attempt:       uint8(min(stepRun.Attempt, 255)), //nolint:gosec // clamped to uint8 range
		Error:         stepRun.Error,
		CreatedAt:     stepRun.CreatedAt,
		StartedAt:     stepRun.StartedAt,
		FinishedAt:    stepRun.FinishedAt,
	})
}

func (s *StepCallback) tryReleaseDependencyRuns(ctx context.Context, run *domain.JobRun) {
	if run == nil || !run.Status.IsTerminal() {
		return
	}

	deps, err := s.store.ListDependentsByDependencyJob(ctx, run.JobID)
	if err != nil {
		s.logger.Error("dependency release: list dependents failed", "job_id", run.JobID, "error", err)
		return
	}
	if len(deps) == 0 {
		return
	}

	dependentJobIDs := make([]string, 0, len(deps))
	seen := make(map[string]struct{}, len(deps))
	for _, dep := range deps {
		if _, ok := seen[dep.JobID]; ok {
			continue
		}
		seen[dep.JobID] = struct{}{}
		dependentJobIDs = append(dependentJobIDs, dep.JobID)
	}

	waitingRuns, err := s.store.ListWaitingRunsByJobIDs(ctx, dependentJobIDs, 1000)
	if err != nil {
		s.logger.Error("dependency release: list waiting runs failed", "job_ids", dependentJobIDs, "error", err)
		return
	}

	for i := range waitingRuns {
		candidate := &waitingRuns[i]
		satisfied, checkErr := s.store.AreJobDependenciesSatisfied(ctx, candidate)
		if checkErr != nil {
			s.logger.Error("dependency release: dependency check failed", "run_id", candidate.ID, "error", checkErr)
			continue
		}
		if !satisfied {
			continue
		}
		if err := s.store.UpdateRunStatus(ctx, candidate.ID, domain.StatusWaiting, domain.StatusQueued, nil); err != nil {
			s.logger.Error("dependency release: failed to queue waiting run", "run_id", candidate.ID, "error", err)
		}
	}
}
