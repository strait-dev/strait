package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"strait/internal/domain"

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
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error)
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
	ListExpiredEventTriggers(ctx context.Context) ([]domain.EventTrigger, error)
	UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
	ListReceivedEventTriggersWithStaleSteps(ctx context.Context) ([]domain.EventTrigger, error)
}

type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
	OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error
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

// ReapOnce runs all reaper passes exactly once. Exported for integration tests.
func (r *Reaper) ReapOnce(ctx context.Context) {
	r.reapStaleDequeued(ctx)
	r.reapStale(ctx)
	r.reapExpired(ctx)
	r.reapTimedOutWorkflows(ctx)
	r.reapExpiredApprovals(ctx)
	r.reapExpiredEventTriggers(ctx)
	r.reapInconsistentEventTriggers(ctx)
	r.reapOldWorkflowRuns(ctx)
}

func (r *Reaper) Run(ctx context.Context) {
	r.logger.Info("reaper configured", "interval", r.interval, "stale_threshold", r.staleThreshold)
	loop := NewMaintenanceLoop("reaper", r.interval, r.logger, func(loopCtx context.Context) {
		r.reapStaleDequeued(loopCtx)
		r.reapStale(loopCtx)
		r.reapExpired(loopCtx)
		r.reapTimedOutWorkflows(loopCtx)
		r.reapExpiredApprovals(loopCtx)
		r.reapExpiredEventTriggers(loopCtx)
		r.reapInconsistentEventTriggers(loopCtx)
		r.reapOldWorkflowRuns(loopCtx)
		if r.retentionEnabled {
			r.reapTerminalRetention(loopCtx)
		}
	})
	loop.Run(ctx)
}

func (r *Reaper) reapOldWorkflowRuns(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapOldWorkflowRuns")
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
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapTimedOutWorkflows")
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

		stepRuns, listErr := r.store.ListStepRunsByWorkflowRun(ctx, wfRun.ID, 10000, nil)
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

		// Cancel any pending event triggers for this workflow.
		if _, cancelErr := r.store.CancelEventTriggersByWorkflowRun(ctx, wfRun.ID); cancelErr != nil {
			slog.Error("failed to cancel event triggers for timed out workflow", "workflow_run_id", wfRun.ID, "error", cancelErr)
		}
	}
}

func (r *Reaper) reapExpiredApprovals(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapExpiredApprovals")
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

func (r *Reaper) reapExpiredEventTriggers(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapExpiredEventTriggers")
	defer span.End()

	triggers, err := r.store.ListExpiredEventTriggers(ctx)
	if err != nil {
		slog.Error("failed to list expired event triggers", "error", err)
		return
	}

	now := time.Now()
	for _, trigger := range triggers {
		if err := r.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusTimedOut, nil, nil, "event trigger timed out"); err != nil {
			slog.Error("failed to mark event trigger timed out", "trigger_id", trigger.ID, "error", err)
			continue
		}

		switch trigger.SourceType {
		case domain.EventSourceWorkflowStep:
			if trigger.WorkflowStepRunID == "" {
				continue
			}
			if err := r.store.UpdateStepRunStatus(ctx, trigger.WorkflowStepRunID, domain.StepFailed, map[string]any{
				"finished_at": now,
				"error":       "event trigger timed out",
			}); err != nil {
				slog.Error("failed to mark event trigger step failed", "trigger_id", trigger.ID, "step_run_id", trigger.WorkflowStepRunID, "error", err)
			}

			if trigger.WorkflowRunID != "" {
				if err := r.store.UpdateWorkflowRunStatus(ctx, trigger.WorkflowRunID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{
					"finished_at": now,
					"error":       "event trigger timed out",
				}); err != nil {
					// Also try from paused state, like approval reaper.
					if errPaused := r.store.UpdateWorkflowRunStatus(ctx, trigger.WorkflowRunID, domain.WfStatusPaused, domain.WfStatusFailed, map[string]any{
						"finished_at": now,
						"error":       "event trigger timed out",
					}); errPaused != nil {
						slog.Error("failed to fail workflow on event trigger timeout", "trigger_id", trigger.ID, "workflow_run_id", trigger.WorkflowRunID, "error", errPaused)
					}
				}
			}

		case domain.EventSourceJobRun:
			if trigger.JobRunID == "" {
				continue
			}
			jobRun, getErr := r.store.GetRun(ctx, trigger.JobRunID)
			if getErr != nil {
				slog.Error("failed to get job run for event trigger timeout", "trigger_id", trigger.ID, "job_run_id", trigger.JobRunID, "error", getErr)
				continue
			}
			if jobRun == nil || jobRun.Status.IsTerminal() {
				continue
			}
			if err := r.store.UpdateRunStatus(ctx, jobRun.ID, jobRun.Status, domain.StatusTimedOut, map[string]any{
				"finished_at": now,
				"error":       "event trigger timed out",
			}); err != nil {
				slog.Error("failed to timeout job run for event trigger", "trigger_id", trigger.ID, "job_run_id", trigger.JobRunID, "error", err)
			}
		}
	}
}

// reapInconsistentEventTriggers finds triggers marked 'received' whose associated
// step run or job run is still 'waiting'. This happens when the process crashes
// between the trigger status update and the step/run completion. The 30-second
// grace period in the query prevents interfering with in-flight operations.
func (r *Reaper) reapInconsistentEventTriggers(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapInconsistentEventTriggers")
	defer span.End()

	triggers, err := r.store.ListReceivedEventTriggersWithStaleSteps(ctx)
	if err != nil {
		slog.Error("failed to list inconsistent event triggers", "error", err)
		return
	}

	for _, trigger := range triggers {
		switch trigger.SourceType {
		case domain.EventSourceWorkflowStep:
			if r.workflowCallback != nil {
				if err := r.workflowCallback.OnEventReceived(ctx, &trigger); err != nil {
					slog.Error("failed to reconcile event trigger step completion", "trigger_id", trigger.ID, "error", err)
				} else {
					slog.Info("reconciled inconsistent event trigger", "trigger_id", trigger.ID, "source_type", trigger.SourceType)
				}
			}

		case domain.EventSourceJobRun:
			if trigger.JobRunID == "" {
				continue
			}
			if err := r.store.UpdateRunStatus(ctx, trigger.JobRunID, domain.StatusWaiting, domain.StatusQueued, map[string]any{
				"checkpoint_data": trigger.ResponsePayload,
			}); err != nil {
				slog.Error("failed to reconcile event trigger job run", "trigger_id", trigger.ID, "job_run_id", trigger.JobRunID, "error", err)
			} else {
				slog.Info("reconciled inconsistent event trigger", "trigger_id", trigger.ID, "source_type", trigger.SourceType)
			}
		}
	}
}

func (r *Reaper) reapStale(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapStale")
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
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapExpired")
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
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapStaleDequeued")
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
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapTerminalRetention")
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
