package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"orchestrator/internal/domain"
	storepkg "orchestrator/internal/store"
	"orchestrator/internal/worker"

	"go.opentelemetry.io/otel"
)

type StepCallback struct {
	store  CallbackStore
	engine *WorkflowEngine
	logger *slog.Logger
}

type CallbackStore interface {
	GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error)
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]storepkg.StepDepResult, error)
	IncrementStepRunAttempt(ctx context.Context, id string, newAttempt int) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
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

func (s *StepCallback) OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "workflow.OnJobRunTerminal")
	defer span.End()

	if run == nil || run.WorkflowStepRunID == "" {
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

	stepStatus, fields := mapRunStatusToStepStatus(run)

	// Apply output transformation for completed steps before persisting.
	if stepStatus == domain.StepCompleted {
		var wfRunForTransform *domain.WorkflowRun
		wfRunForTransform, err = s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
		if err != nil {
			wfRunForTransform = nil
		}
		if rawOut, ok := fields["output"].(json.RawMessage); ok && len(rawOut) > 0 {
			if transformPath := s.lookupOutputTransform(ctx, stepRun, wfRunForTransform); transformPath != "" {
				transformed, transformErr := ApplyOutputTransform(rawOut, transformPath)
				if transformErr != nil {
					s.logger.Warn("output transform failed, keeping original output",
						"step_ref", stepRun.StepRef, "transform", transformPath, "error", transformErr)
				} else {
					fields["output"] = transformed
				}
			}
		}
	}

	fields["finished_at"] = time.Now()
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, stepStatus, fields); err != nil {
		s.logger.Error("failed to update step run terminal status", "step_run_id", stepRun.ID, "status", stepStatus, "error", err)
		return fmt.Errorf("update step run terminal status: %w", err)
	}

	stepRun.Status = stepStatus
	if out, ok := fields["output"].(json.RawMessage); ok {
		stepRun.Output = out
	}
	if stepErr, ok := fields["error"].(string); ok {
		stepRun.Error = stepErr
	}

	// Check if step retry is needed before handling failure
	if stepStatus == domain.StepFailed {
		if shouldRetry, nextRetryAt, newAttempt, err := s.checkStepRetry(ctx, stepRun, run); err != nil {
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
		if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
			s.logger.Error("failed to process completed step", "step_ref", stepRun.StepRef, "error", err)
			return fmt.Errorf("process completed step %s: %w", stepRun.StepRef, err)
		}
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID)
	case domain.StepFailed:
		if err := s.handleFailedStep(ctx, stepRun); err != nil {
			s.logger.Error("failed to process failed step", "step_ref", stepRun.StepRef, "error", err)
			return fmt.Errorf("process failed step %s: %w", stepRun.StepRef, err)
		}
		return nil
	default:
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID)
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

func (s *StepCallback) checkStepRetry(ctx context.Context, stepRun *domain.WorkflowStepRun, _ *domain.JobRun) (bool, time.Time, int, error) {
	wfRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return false, time.Time{}, 0, fmt.Errorf("workflow run not found: %s", stepRun.WorkflowRunID)
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("list workflow steps: %w", err)
	}

	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, st := range steps {
		stepByRef[st.StepRef] = st
	}

	failedStep, ok := stepByRef[stepRun.StepRef]
	if !ok {
		return false, time.Time{}, 0, fmt.Errorf("step definition not found for %s", stepRun.StepRef)
	}

	// Check if retry is configured and attempts remain
	retryMaxAttempts := failedStep.RetryMaxAttempts
	if retryMaxAttempts <= 0 {
		return false, time.Time{}, 0, nil
	}

	currentAttempt := stepRun.Attempt
	if currentAttempt >= retryMaxAttempts {
		s.logger.Debug("step retry exhausted", "step_ref", stepRun.StepRef, "attempt", currentAttempt, "max_attempts", retryMaxAttempts)
		return false, time.Time{}, 0, nil
	}

	// Calculate next retry attempt and delay
	newAttempt := currentAttempt + 1
	retryBackoff := failedStep.RetryBackoff
	retryInitialDelaySecs := failedStep.RetryInitialDelaySecs
	retryMaxDelaySecs := failedStep.RetryMaxDelaySecs

	nextRetryDelay := worker.NextRetryDelayWithPolicy(
		newAttempt,
		retryBackoff,
		retryInitialDelaySecs,
		retryMaxDelaySecs,
	)
	nextRetryAt := time.Now().Add(nextRetryDelay)

	s.logger.Info("scheduling step retry", "step_ref", stepRun.StepRef, "attempt", currentAttempt, "next_attempt", newAttempt, "retry_at", nextRetryAt)

	return true, nextRetryAt, newAttempt, nil
}

func (s *StepCallback) scheduleStepRetry(ctx context.Context, jobRun *domain.JobRun, stepRun *domain.WorkflowStepRun, nextRetryAt time.Time, newAttempt int) error {
	// Increment step run attempt counter
	if err := s.store.IncrementStepRunAttempt(ctx, stepRun.ID, newAttempt); err != nil {
		return fmt.Errorf("increment step run attempt: %w", err)
	}

	// Update job run to delayed status with next retry time
	fields := map[string]any{
		"next_retry_at": nextRetryAt,
		"attempt":       newAttempt,
	}
	if err := s.store.UpdateRunStatus(ctx, jobRun.ID, jobRun.Status, domain.StatusDelayed, fields); err != nil {
		return fmt.Errorf("update job run status for retry: %w", err)
	}

	return nil
}

func (s *StepCallback) handleFailedStep(ctx context.Context, stepRun *domain.WorkflowStepRun) error {
	wfRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return fmt.Errorf("workflow run not found: %s", stepRun.WorkflowRunID)
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}

	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, st := range steps {
		stepByRef[st.StepRef] = st
	}

	failedStep, ok := stepByRef[stepRun.StepRef]
	if !ok {
		return fmt.Errorf("step definition not found for %s", stepRun.StepRef)
	}

	policy := failedStep.OnFailure
	if policy == "" {
		policy = domain.FailWorkflow
	}

	switch policy {
	case domain.FailWorkflow:
		return s.failWorkflowAndCancel(ctx, wfRun, stepRun)
	case domain.SkipDependents:
		if err := s.skipDependentSteps(ctx, stepRun.WorkflowRunID, wfRun.WorkflowID, stepRun.StepRef); err != nil {
			return fmt.Errorf("skip dependents: %w", err)
		}
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID)
	case domain.Continue:
		if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
			return fmt.Errorf("fan-in for continue policy on step %s: %w", stepRun.StepRef, err)
		}
		return s.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID)
	default:
		return s.failWorkflowAndCancel(ctx, wfRun, stepRun)
	}
}

// failWorkflowAndCancel marks the workflow as failed, cancels remaining steps, and propagates to parent.
func (s *StepCallback) failWorkflowAndCancel(ctx context.Context, wfRun *domain.WorkflowRun, stepRun *domain.WorkflowStepRun) error {
	if wfRun.Status == domain.WfStatusRunning {
		now := time.Now()
		if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{"error": stepRun.Error, "finished_at": now}); err != nil {
			return fmt.Errorf("mark workflow failed: %w", err)
		}
		wfRun.Status = domain.WfStatusFailed
	}
	if err := s.cancelRemainingSteps(ctx, stepRun.WorkflowRunID); err != nil {
		return fmt.Errorf("cancel remaining steps: %w", err)
	}
	return s.propagateToParent(ctx, wfRun, nil)
}

func (s *StepCallback) fanInAndStartReadyChildren(ctx context.Context, stepRun *domain.WorkflowStepRun) error {
	deps, err := s.store.IncrementStepDeps(ctx, stepRun.WorkflowRunID, stepRun.StepRef)
	if err != nil {
		return fmt.Errorf("increment step deps: %w", err)
	}
	if len(deps) == 0 {
		return nil
	}

	wfRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun.Status == domain.WfStatusPaused {
		return nil
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list steps by workflow: %w", err)
	}
	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, st := range steps {
		stepByRef[st.StepRef] = st
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}
	stepRunByID := make(map[string]domain.WorkflowStepRun, len(stepRuns))
	stepStatuses := make(map[string]domain.StepRunStatus, len(stepRuns))
	runningStepCount := 0
	for _, sr := range stepRuns {
		stepRunByID[sr.ID] = sr
		stepStatuses[sr.StepRef] = sr.Status
		if sr.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	for _, dep := range deps {
		if dep.DepsCompleted != dep.DepsRequired {
			continue
		}

		childStep, ok := stepByRef[dep.StepRef]
		if !ok {
			return fmt.Errorf("step definition not found for %s", dep.StepRef)
		}

		childStepRun, ok := stepRunByID[dep.StepRunID]
		if !ok {
			return fmt.Errorf("step run not found for %s", dep.StepRunID)
		}
		if childStepRun.Status.IsTerminal() {
			continue
		}
		if wfRun.MaxParallelSteps > 0 && runningStepCount >= wfRun.MaxParallelSteps {
			continue
		}

		allowed, err := EvaluateCondition(childStep.Condition, stepStatuses)
		if err != nil {
			return fmt.Errorf("evaluate condition for step %s: %w", childStep.StepRef, err)
		}

		if !allowed {
			now := time.Now()
			if err := s.store.UpdateStepRunStatus(ctx, childStepRun.ID, domain.StepSkipped, map[string]any{"finished_at": now}); err != nil {
				return fmt.Errorf("skip step %s: %w", childStep.StepRef, err)
			}
			stepStatuses[childStepRun.StepRef] = domain.StepSkipped
			continue
		}

		var parentOutputsPayload json.RawMessage
		if len(childStep.DependsOn) > 0 {
			outputs, err := s.store.GetStepOutputs(ctx, stepRun.WorkflowRunID, childStep.DependsOn)
			if err != nil {
				return fmt.Errorf("get step outputs for %s: %w", childStep.StepRef, err)
			}

			payload, err := json.Marshal(outputs)
			if err != nil {
				return fmt.Errorf("marshal parent outputs for %s: %w", childStep.StepRef, err)
			}
			parentOutputsPayload = payload
		}

		childRun := childStepRun
		stepDef := childStep
		if err := s.engine.startStep(ctx, &childRun, &stepDef, wfRun, parentOutputsPayload); err != nil {
			return fmt.Errorf("start child step %s: %w", childStep.StepRef, err)
		}
		stepStatuses[childStepRun.StepRef] = domain.StepRunning
		if childRun.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	return nil
}

func (s *StepCallback) checkWorkflowCompletion(ctx context.Context, workflowRunID string) error {
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}

	for _, sr := range stepRuns {
		if !sr.Status.IsTerminal() {
			return nil
		}
	}

	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun.Status.IsTerminal() {
		return nil
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}

	policyByRef := make(map[string]domain.FailurePolicy, len(steps))
	for _, step := range steps {
		policyByRef[step.StepRef] = step.OnFailure
	}

	hasFailingStep := false
	for _, sr := range stepRuns {
		if sr.Status != domain.StepFailed {
			continue
		}

		if policyByRef[sr.StepRef] != domain.Continue {
			hasFailingStep = true
			break
		}
	}

	now := time.Now()
	if hasFailingStep {
		if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusFailed, map[string]any{"finished_at": now}); err != nil {
			return fmt.Errorf("mark workflow run failed: %w", err)
		}
		wfRun.Status = domain.WfStatusFailed
		return s.propagateToParent(ctx, wfRun, stepRuns)
	}

	if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusCompleted, map[string]any{"finished_at": now}); err != nil {
		return fmt.Errorf("mark workflow run completed: %w", err)
	}
	wfRun.Status = domain.WfStatusCompleted

	return s.propagateToParent(ctx, wfRun, stepRuns)
}

// propagateToParent propagates the terminal status of a child workflow run
// back to the parent step run that spawned it via sub_workflow.
func (s *StepCallback) propagateToParent(ctx context.Context, childRun *domain.WorkflowRun, childStepRuns []domain.WorkflowStepRun) error {
	if childRun.ParentWorkflowRunID == "" {
		return nil
	}
	if !childRun.Status.IsTerminal() {
		return nil
	}

	parentRun, err := s.store.GetWorkflowRun(ctx, childRun.ParentWorkflowRunID)
	if err != nil {
		return fmt.Errorf("get parent workflow run: %w", err)
	}
	if parentRun == nil {
		s.logger.Warn("parent workflow run not found for sub-workflow propagation",
			"child_run_id", childRun.ID, "parent_run_id", childRun.ParentWorkflowRunID)
		return nil
	}
	if parentRun.Status.IsTerminal() {
		return nil
	}

	// Get parent's step definitions to find which step is a sub_workflow that matches this child.
	parentSteps, err := s.store.ListStepsByWorkflowVersion(ctx, parentRun.WorkflowID, parentRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list parent workflow steps: %w", err)
	}

	// Find the sub_workflow step whose SubWorkflowID matches the child's workflow ID.
	var matchingStepRef string
	for _, step := range parentSteps {
		if step.StepType == domain.WorkflowStepTypeSubWorkflow && step.SubWorkflowID == childRun.WorkflowID {
			matchingStepRef = step.StepRef
			break
		}
	}
	if matchingStepRef == "" {
		s.logger.Warn("no matching sub_workflow step found in parent",
			"child_run_id", childRun.ID, "child_workflow_id", childRun.WorkflowID,
			"parent_run_id", parentRun.ID)
		return nil
	}

	// Find the parent step run for this sub_workflow step.
	parentStepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, parentRun.ID, matchingStepRef)
	if err != nil {
		return fmt.Errorf("get parent step run for sub-workflow: %w", err)
	}
	if parentStepRun == nil || parentStepRun.Status.IsTerminal() {
		return nil
	}

	now := time.Now()
	switch childRun.Status {
	case domain.WfStatusCompleted:
		// Aggregate child step outputs as the parent step's output.
		var outputPayload json.RawMessage
		if len(childStepRuns) > 0 {
			outputs := make(map[string]json.RawMessage, len(childStepRuns))
			for _, sr := range childStepRuns {
				if len(sr.Output) > 0 {
					outputs[sr.StepRef] = sr.Output
				}
			}
			if len(outputs) > 0 {
				if raw, marshalErr := json.Marshal(outputs); marshalErr == nil {
					outputPayload = raw
				}
			}
		}

		fields := map[string]any{"finished_at": now}
		if len(outputPayload) > 0 {
			fields["output"] = outputPayload
		}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepCompleted, fields); err != nil {
			return fmt.Errorf("complete parent step run for sub-workflow: %w", err)
		}
		parentStepRun.Status = domain.StepCompleted
		if len(outputPayload) > 0 {
			parentStepRun.Output = outputPayload
		}

		if err := s.fanInAndStartReadyChildren(ctx, parentStepRun); err != nil {
			return fmt.Errorf("fan-in after sub-workflow completion for step %s: %w", matchingStepRef, err)
		}
		return s.checkWorkflowCompletion(ctx, parentRun.ID)

	case domain.WfStatusFailed:
		fields := map[string]any{"finished_at": now, "error": childRun.Error}
		if childRun.Error == "" {
			fields["error"] = fmt.Sprintf("sub-workflow %s failed", childRun.WorkflowID)
		}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepFailed, fields); err != nil {
			return fmt.Errorf("fail parent step run for sub-workflow: %w", err)
		}
		parentStepRun.Status = domain.StepFailed
		parentStepRun.Error = fields["error"].(string)

		return s.handleFailedStep(ctx, parentStepRun)

	default:
		// Canceled or timed_out child workflows fail the parent step.
		errMsg := fmt.Sprintf("sub-workflow %s ended with status %s", childRun.WorkflowID, childRun.Status)
		fields := map[string]any{"finished_at": now, "error": errMsg}
		if err := s.store.UpdateStepRunStatus(ctx, parentStepRun.ID, domain.StepFailed, fields); err != nil {
			return fmt.Errorf("fail parent step run for sub-workflow: %w", err)
		}
		parentStepRun.Status = domain.StepFailed
		parentStepRun.Error = errMsg

		return s.handleFailedStep(ctx, parentStepRun)
	}
}

func (s *StepCallback) cancelRemainingSteps(ctx context.Context, workflowRunID string) error {
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}

	now := time.Now()
	for _, sr := range stepRuns {
		if sr.Status.IsTerminal() {
			continue
		}
		if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepCanceled, map[string]any{"finished_at": now}); err != nil {
			return fmt.Errorf("cancel step run %s: %w", sr.ID, err)
		}
	}

	return nil
}

func (s *StepCallback) skipDependentSteps(ctx context.Context, workflowRunID, workflowID, failedStepRef string) error {
	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, workflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}

	dependents := make(map[string][]string, len(steps))
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			dependents[dep] = append(dependents[dep], step.StepRef)
		}
	}

	toSkip := make(map[string]struct{})
	queue := append([]string(nil), dependents[failedStepRef]...)
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]

		if _, seen := toSkip[ref]; seen {
			continue
		}
		toSkip[ref] = struct{}{}
		queue = append(queue, dependents[ref]...)
	}

	if len(toSkip) == 0 {
		return nil
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}

	now := time.Now()
	for _, sr := range stepRuns {
		if _, ok := toSkip[sr.StepRef]; !ok {
			continue
		}
		if sr.Status.IsTerminal() {
			continue
		}

		if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepSkipped, map[string]any{"finished_at": now}); err != nil {
			return fmt.Errorf("skip step run %s: %w", sr.ID, err)
		}
	}

	return nil
}

func (s *StepCallback) ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error {
	if approver == "" {
		return fmt.Errorf("approver is required")
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
	if err != nil {
		return fmt.Errorf("get step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("step run not found for %s", stepRef)
	}
	if stepRun.Status.IsTerminal() {
		return fmt.Errorf("step %s is already in terminal state", stepRef)
	}

	approval, err := s.store.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	if err != nil {
		return fmt.Errorf("get workflow step approval: %w", err)
	}
	if approval == nil {
		return fmt.Errorf("approval not found for step %s", stepRef)
	}
	if approval.Status != domain.ApprovalStatusPending {
		return fmt.Errorf("approval for step %s is already %s", stepRef, approval.Status)
	}

	if !slices.Contains(approval.Approvers, approver) {
		return fmt.Errorf("approver %s is not allowed for step %s", approver, stepRef)
	}

	now := time.Now()
	if err := s.store.UpdateWorkflowStepApproval(ctx, approval.ID, domain.ApprovalStatusApproved, approver, &now, ""); err != nil {
		return fmt.Errorf("update approval: %w", err)
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepCompleted, map[string]any{"finished_at": now}); err != nil {
		return fmt.Errorf("complete approval step run: %w", err)
	}

	stepRun.Status = domain.StepCompleted
	if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
		return fmt.Errorf("fan-in after approval for step %s: %w", stepRef, err)
	}

	return s.checkWorkflowCompletion(ctx, workflowRunID)
}

func (s *StepCallback) SkipStep(ctx context.Context, workflowRunID, stepRef, reason string) error {
	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
	if err != nil {
		return fmt.Errorf("get step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("step run not found for %s", stepRef)
	}
	if stepRun.Status != domain.StepPending && stepRun.Status != domain.StepWaiting {
		return fmt.Errorf("cannot skip step in %s status", stepRun.Status)
	}

	now := time.Now()
	fields := map[string]any{"finished_at": now}
	if reason != "" {
		fields["error"] = reason
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepSkipped, fields); err != nil {
		return fmt.Errorf("skip step: %w", err)
	}
	stepRun.Status = domain.StepSkipped

	if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
		return fmt.Errorf("fan-in after skip: %w", err)
	}
	return s.checkWorkflowCompletion(ctx, workflowRunID)
}

func (s *StepCallback) ForceCompleteStep(ctx context.Context, workflowRunID, stepRef string, result json.RawMessage) error {
	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, workflowRunID, stepRef)
	if err != nil {
		return fmt.Errorf("get step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("step run not found for %s", stepRef)
	}
	if stepRun.Status != domain.StepPending && stepRun.Status != domain.StepWaiting {
		return fmt.Errorf("cannot force-complete step in %s status", stepRun.Status)
	}

	now := time.Now()
	fields := map[string]any{"finished_at": now}
	if len(result) > 0 {
		fields["output"] = result
	}
	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepCompleted, fields); err != nil {
		return fmt.Errorf("force-complete step: %w", err)
	}
	stepRun.Status = domain.StepCompleted

	if err := s.fanInAndStartReadyChildren(ctx, stepRun); err != nil {
		return fmt.Errorf("fan-in after force-complete: %w", err)
	}
	return s.checkWorkflowCompletion(ctx, workflowRunID)
}

func (s *StepCallback) ResumeWorkflowRun(ctx context.Context, workflowRunID string) error {
	wfRun, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	if wfRun == nil {
		return fmt.Errorf("workflow run not found: %s", workflowRunID)
	}
	if wfRun.Status != domain.WfStatusPaused {
		return fmt.Errorf("workflow run %s is not paused", workflowRunID)
	}

	if err := s.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPaused, domain.WfStatusRunning, nil); err != nil {
		return fmt.Errorf("resume workflow run: %w", err)
	}

	wfRun.Status = domain.WfStatusRunning

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list workflow steps: %w", err)
	}
	stepByRef := make(map[string]domain.WorkflowStep, len(steps))
	for _, step := range steps {
		stepByRef[step.StepRef] = step
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("list step runs by workflow run: %w", err)
	}
	stepStatuses := make(map[string]domain.StepRunStatus, len(stepRuns))
	runningStepCount := 0
	for _, sr := range stepRuns {
		stepStatuses[sr.StepRef] = sr.Status
		if sr.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	for _, sr := range stepRuns {
		if sr.Status.IsTerminal() || sr.Status == domain.StepRunning {
			continue
		}
		if sr.DepsCompleted != sr.DepsRequired {
			continue
		}
		if wfRun.MaxParallelSteps > 0 && runningStepCount >= wfRun.MaxParallelSteps {
			continue
		}

		stepDef, ok := stepByRef[sr.StepRef]
		if !ok {
			return fmt.Errorf("step definition not found for %s", sr.StepRef)
		}

		allowed, condErr := EvaluateCondition(stepDef.Condition, stepStatuses)
		if condErr != nil {
			return fmt.Errorf("evaluate condition for step %s: %w", stepDef.StepRef, condErr)
		}
		if !allowed {
			now := time.Now()
			if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepSkipped, map[string]any{"finished_at": now}); err != nil {
				return fmt.Errorf("skip step %s: %w", stepDef.StepRef, err)
			}
			stepStatuses[sr.StepRef] = domain.StepSkipped
			continue
		}

		var parentOutputsPayload json.RawMessage
		if len(stepDef.DependsOn) > 0 {
			outputs, err := s.store.GetStepOutputs(ctx, workflowRunID, stepDef.DependsOn)
			if err != nil {
				return fmt.Errorf("get step outputs for %s: %w", stepDef.StepRef, err)
			}
			payload, err := json.Marshal(outputs)
			if err != nil {
				return fmt.Errorf("marshal parent outputs for %s: %w", stepDef.StepRef, err)
			}
			parentOutputsPayload = payload
		}

		srCopy := sr
		stepCopy := stepDef
		if err := s.engine.startStep(ctx, &srCopy, &stepCopy, wfRun, parentOutputsPayload); err != nil {
			return fmt.Errorf("start resumed step %s: %w", stepDef.StepRef, err)
		}
		stepStatuses[sr.StepRef] = srCopy.Status
		if srCopy.Status == domain.StepRunning {
			runningStepCount++
		}
	}

	return nil
}

// lookupOutputTransform finds the output_transform path for a step.
// Returns empty string if none is configured or on lookup error.
func (s *StepCallback) lookupOutputTransform(ctx context.Context, stepRun *domain.WorkflowStepRun, wfRun *domain.WorkflowRun) string {
	if wfRun == nil {
		loadedRun, err := s.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
		if err != nil || loadedRun == nil {
			return ""
		}
		wfRun = loadedRun
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
	if err != nil {
		return ""
	}

	for _, st := range steps {
		if st.StepRef == stepRun.StepRef {
			return st.OutputTransform
		}
	}

	return ""
}
