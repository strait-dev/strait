package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"orchestrator/internal/domain"
	storepkg "orchestrator/internal/store"

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
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
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
