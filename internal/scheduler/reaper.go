package scheduler

import (
	"context"
	"log/slog"
	"time"

	"orchestrator/internal/domain"

	"go.opentelemetry.io/otel"
)

// ReaperStore is the subset of store operations needed by Reaper.
type ReaperStore interface {
	ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error)
	ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	ListTimedOutWorkflowRuns(ctx context.Context) ([]domain.WorkflowRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error)
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
}

type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
}

type Reaper struct {
	store            ReaperStore
	interval         time.Duration
	staleThreshold   time.Duration
	workflowCallback WorkflowCallback
	logger           *slog.Logger
}

func NewReaper(s ReaperStore, interval, staleThreshold time.Duration, workflowCallback WorkflowCallback) *Reaper {
	return &Reaper{
		store:            s,
		interval:         interval,
		staleThreshold:   staleThreshold,
		workflowCallback: workflowCallback,
		logger:           slog.Default(),
	}
}

func (r *Reaper) notifyWorkflowCallback(ctx context.Context, run *domain.JobRun) {
	if r.workflowCallback == nil {
		return
	}

	if err := r.workflowCallback.OnJobRunTerminal(ctx, run); err != nil {
		r.logger.Error("workflow callback failed", "run_id", run.ID, "error", err)
	}
}

func (r *Reaper) Run(ctx context.Context) {
	slog.Info("reaper started", "interval", r.interval, "stale_threshold", r.staleThreshold)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reaper stopping")
			return
		case <-ticker.C:
			r.reapStaleDequeued(ctx)
			r.reapStale(ctx)
			r.reapExpired(ctx)
			r.reapTimedOutWorkflows(ctx)
			r.reapExpiredApprovals(ctx)
		}
	}
}

func (r *Reaper) reapTimedOutWorkflows(ctx context.Context) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "reaper.ReapTimedOutWorkflows")
	defer span.End()

	runs, err := r.store.ListTimedOutWorkflowRuns(ctx)
	if err != nil {
		slog.Error("failed to list timed out workflow runs", "error", err)
		return
	}

	for _, wfRun := range runs {
		if err := r.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusFailed, map[string]any{
			"finished_at": time.Now(),
			"error":       "workflow timed out",
		}); err != nil {
			slog.Error("failed to fail timed out workflow run", "workflow_run_id", wfRun.ID, "error", err)
			continue
		}

		stepRuns, listErr := r.store.ListStepRunsByWorkflowRun(ctx, wfRun.ID)
		if listErr != nil {
			slog.Error("failed to list workflow step runs for timed out workflow", "workflow_run_id", wfRun.ID, "error", listErr)
			continue
		}

		now := time.Now()
		for _, stepRun := range stepRuns {
			if !stepRun.Status.IsTerminal() {
				if err := r.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepCanceled, map[string]any{
					"finished_at": now,
					"error":       "workflow timed out",
				}); err != nil {
					slog.Error("failed to cancel timed out workflow step run", "step_run_id", stepRun.ID, "error", err)
				}
			}

			if stepRun.JobRunID == "" {
				continue
			}

			jobRun, getErr := r.store.GetRun(ctx, stepRun.JobRunID)
			if getErr != nil {
				slog.Error("failed to get job run for timed out workflow", "job_run_id", stepRun.JobRunID, "error", getErr)
				continue
			}
			if jobRun == nil || jobRun.Status.IsTerminal() {
				continue
			}

			if err := r.store.UpdateRunStatus(ctx, jobRun.ID, jobRun.Status, domain.StatusCanceled, map[string]any{
				"finished_at": now,
				"error":       "workflow timed out",
			}); err != nil {
				slog.Error("failed to cancel job run for timed out workflow", "job_run_id", jobRun.ID, "error", err)
			}
		}
	}
}

func (r *Reaper) reapExpiredApprovals(ctx context.Context) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "reaper.ReapExpiredApprovals")
	defer span.End()

	approvals, err := r.store.ListExpiredWorkflowStepApprovals(ctx)
	if err != nil {
		slog.Error("failed to list expired approvals", "error", err)
		return
	}

	now := time.Now()
	for _, approval := range approvals {
		if err := r.store.UpdateWorkflowStepApproval(ctx, approval.ID, "timed_out", "", nil, "approval timed out"); err != nil {
			slog.Error("failed to mark approval timed out", "approval_id", approval.ID, "error", err)
			continue
		}

		if err := r.store.UpdateStepRunStatus(ctx, approval.WorkflowStepRunID, domain.StepFailed, map[string]any{
			"finished_at": now,
			"error":       "approval timed out",
		}); err != nil {
			slog.Error("failed to mark approval step failed", "approval_id", approval.ID, "error", err)
		}

		if err := r.store.UpdateWorkflowRunStatus(ctx, approval.WorkflowRunID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{
			"finished_at": now,
			"error":       "approval timed out",
		}); err != nil {
			slog.Error("failed to fail workflow on approval timeout", "approval_id", approval.ID, "error", err)
		}
	}
}

func (r *Reaper) reapStale(ctx context.Context) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "reaper.ReapStale")
	defer span.End()

	runs, err := r.store.ListStaleRuns(ctx, r.staleThreshold)
	if err != nil {
		slog.Error("failed to list stale runs", "error", err)
		return
	}

	for _, run := range runs {
		err := r.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCrashed, map[string]any{
			"finished_at": time.Now(),
			"error":       "heartbeat lost",
		})
		if err != nil {
			slog.Error("failed to crash stale run", "run_id", run.ID, "job_id", run.JobID, "error", err)
			continue
		}
		run.Status = domain.StatusCrashed
		r.notifyWorkflowCallback(ctx, &run)

		slog.Warn("stale run marked crashed", "run_id", run.ID, "job_id", run.JobID)
	}
}

func (r *Reaper) reapExpired(ctx context.Context) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "reaper.ReapExpired")
	defer span.End()

	runs, err := r.store.ListExpiredRuns(ctx)
	if err != nil {
		slog.Error("failed to list expired runs", "error", err)
		return
	}

	for _, run := range runs {
		err := r.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusExpired, map[string]any{
			"finished_at": time.Now(),
			"error":       "run expired",
		})
		if err != nil {
			slog.Error("failed to expire run", "run_id", run.ID, "job_id", run.JobID, "from_status", run.Status, "error", err)
			continue
		}
		run.Status = domain.StatusExpired
		r.notifyWorkflowCallback(ctx, &run)

		slog.Warn("run marked expired", "run_id", run.ID, "job_id", run.JobID, "from_status", run.Status)
	}
}

func (r *Reaper) reapStaleDequeued(ctx context.Context) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "reaper.ReapStaleDequeued")
	defer span.End()

	runs, err := r.store.ListStaleDequeued(ctx, r.staleThreshold)
	if err != nil {
		slog.Error("failed to list stale dequeued runs", "error", err)
		return
	}

	for _, run := range runs {
		err := r.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, map[string]any{
			"started_at": nil,
		})
		if err != nil {
			slog.Error("failed to re-queue stale dequeued run", "run_id", run.ID, "job_id", run.JobID, "error", err)
			continue
		}

		slog.Warn("stale dequeued run re-queued", "run_id", run.ID, "job_id", run.JobID)
	}
}
