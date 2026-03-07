package scheduler

import (
	"context"
	"log/slog"
	"time"

	"orchestrator/internal/domain"

	"go.opentelemetry.io/otel"
)

const (
	defaultWorkflowRetention = 30 * 24 * time.Hour
	defaultDeleteBatchLimit  = 100
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
	DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
}

type Reaper struct {
	store             ReaperStore
	interval          time.Duration
	staleThreshold    time.Duration
	workflowRetention time.Duration
	deleteBatchLimit  int
	shortRetention    time.Duration
	longRetention     time.Duration
	retentionEnabled  bool
	workflowCallback  WorkflowCallback
	logger            *slog.Logger
}

// NewReaper creates a new stale and expired run reaper.
func NewReaper(s ReaperStore, interval, staleThreshold, shortRetention, longRetention time.Duration, retentionEnabled bool, workflowCallback WorkflowCallback) *Reaper {
	if shortRetention <= 0 {
		shortRetention = 30 * 24 * time.Hour
	}
	if longRetention <= 0 {
		longRetention = 90 * 24 * time.Hour
	}
	return &Reaper{
		store:             s,
		interval:          interval,
		staleThreshold:    staleThreshold,
		workflowRetention: defaultWorkflowRetention,
		deleteBatchLimit:  defaultDeleteBatchLimit,
		shortRetention:    shortRetention,
		longRetention:     longRetention,
		retentionEnabled:  retentionEnabled,
		workflowCallback:  workflowCallback,
		logger:            slog.Default(),
	}
}

// WithWorkflowRetention sets the retention period for completed workflow runs.
// Runs older than this duration are purged by the reaper.
func (r *Reaper) WithWorkflowRetention(d time.Duration) *Reaper {
	if d > 0 {
		r.workflowRetention = d
	}
	return r
}

// WithDeleteBatchSize sets the number of workflow runs to delete per reaper cycle.
func (r *Reaper) WithDeleteBatchSize(n int) *Reaper {
	if n > 0 {
		r.deleteBatchLimit = n
	}
	return r
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
	r.logger.Info("reaper configured", "interval", r.interval, "stale_threshold", r.staleThreshold)
	loop := NewMaintenanceLoop("reaper", r.interval, r.logger, func(loopCtx context.Context) {
		r.reapStaleDequeued(loopCtx)
		r.reapStale(loopCtx)
		r.reapExpired(loopCtx)
		r.reapTimedOutWorkflows(loopCtx)
		r.reapExpiredApprovals(loopCtx)
		r.reapOldWorkflowRuns(loopCtx)
		if r.retentionEnabled {
			r.reapTerminalRetention(loopCtx)
		}
	})
	loop.Run(ctx)
}

func (r *Reaper) reapOldWorkflowRuns(ctx context.Context) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "reaper.ReapOldWorkflowRuns")
	defer span.End()

	if r.workflowRetention <= 0 {
		return
	}

	before := time.Now().Add(-r.workflowRetention)
	count, err := r.store.DeleteWorkflowRunsFinishedBefore(ctx, before, r.deleteBatchLimit)
	if err != nil {
		slog.Error("failed to delete old workflow runs", "error", err)
		return
	}
	if count > 0 {
		slog.Info("deleted old workflow runs", "count", count, "before", before.UTC())
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
		if err := r.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusTimedOut, map[string]any{
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
			if errPaused := r.store.UpdateWorkflowRunStatus(ctx, approval.WorkflowRunID, domain.WfStatusPaused, domain.WfStatusFailed, map[string]any{
				"finished_at": now,
				"error":       "approval timed out",
			}); errPaused != nil {
				slog.Error("failed to fail workflow on approval timeout", "approval_id", approval.ID, "error", errPaused)
			}
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

func (r *Reaper) reapTerminalRetention(ctx context.Context) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "reaper.ReapTerminalRetention")
	defer span.End()

	deleted, err := r.store.DeleteTerminalRunsPastRetention(ctx, r.shortRetention, r.longRetention)
	if err != nil {
		slog.Error("failed to delete retained terminal runs", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("deleted terminal runs past retention", "count", deleted)
	}
}
