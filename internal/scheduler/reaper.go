package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	defaultWorkflowRetention     = 30 * 24 * time.Hour
	defaultEventTriggerRetention = 30 * 24 * time.Hour
	defaultDeleteBatchLimit      = 100
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
	DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
	OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error
	OnStepCompleted(ctx context.Context, workflowRunID string, stepRunID string)
	OnStepFailed(ctx context.Context, workflowRunID string, stepRunID string)
}

// AdvisoryLocker attempts to acquire a PostgreSQL advisory lock.
// Returns true if the lock was acquired (caller should run reaper),
// false if another instance holds it.
type AdvisoryLocker interface {
	TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error)
	ReleaseAdvisoryLock(ctx context.Context, lockID int64) error
}

// reaperAdvisoryLockID is the pg_advisory_lock key for single-leader reaper.
const reaperAdvisoryLockID int64 = 0x5374726169745265 // "StraitRe" as int64

type Reaper struct {
	store                 ReaperStore
	interval              time.Duration
	staleThreshold        time.Duration
	workflowRetention     time.Duration
	eventTriggerRetention time.Duration
	deleteBatchLimit      int
	advisoryLocker        AdvisoryLocker
	shortRetention        time.Duration
	longRetention         time.Duration
	retentionEnabled      bool
	workflowCallback      WorkflowCallback
	metrics               *telemetry.Metrics
	logger                *slog.Logger
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
		store:                 s,
		interval:              interval,
		staleThreshold:        staleThreshold,
		workflowRetention:     defaultWorkflowRetention,
		eventTriggerRetention: defaultEventTriggerRetention,
		deleteBatchLimit:      defaultDeleteBatchLimit,
		shortRetention:        shortRetention,
		longRetention:         longRetention,
		retentionEnabled:      retentionEnabled,
		workflowCallback:      workflowCallback,
		logger:                slog.Default(),
	}
}

// WithMetrics sets the telemetry metrics for the reaper.
func (r *Reaper) WithMetrics(m *telemetry.Metrics) *Reaper {
	r.metrics = m
	return r
}

// WithWorkflowRetention sets the retention period for completed workflow runs.
// Runs older than this duration are purged by the reaper.
func (r *Reaper) WithWorkflowRetention(d time.Duration) *Reaper {
	if d > 0 {
		r.workflowRetention = d
	}
	return r
}

// WithEventTriggerRetention sets the retention period for completed event triggers.
func (r *Reaper) WithEventTriggerRetention(d time.Duration) *Reaper {
	if d > 0 {
		r.eventTriggerRetention = d
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

// WithAdvisoryLocker enables distributed single-leader reaping using pg_try_advisory_lock.
func (r *Reaper) WithAdvisoryLocker(locker AdvisoryLocker) *Reaper {
	r.advisoryLocker = locker
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
	r.reapOldEventTriggers(ctx)
}

func (r *Reaper) Run(ctx context.Context) {
	r.logger.Info("reaper configured", "interval", r.interval, "stale_threshold", r.staleThreshold)
	loop := NewMaintenanceLoop("reaper", r.interval, r.logger, func(loopCtx context.Context) {
		if r.advisoryLocker != nil {
			acquired, err := r.advisoryLocker.TryAdvisoryLock(loopCtx, reaperAdvisoryLockID)
			if err != nil {
				r.logger.Error("advisory lock check failed, skipping cycle", "error", err)
				return
			}
			if !acquired {
				r.logger.Debug("reaper advisory lock held by another instance, skipping cycle")
				return
			}
			defer func() {
				if err := r.advisoryLocker.ReleaseAdvisoryLock(loopCtx, reaperAdvisoryLockID); err != nil {
					r.logger.Warn("failed to release advisory lock", "error", err)
				}
			}()
		}

		r.reapStaleDequeued(loopCtx)
		r.reapStale(loopCtx)
		r.reapExpired(loopCtx)
		r.reapTimedOutWorkflows(loopCtx)
		r.reapExpiredApprovals(loopCtx)
		r.reapExpiredEventTriggers(loopCtx)
		r.reapInconsistentEventTriggers(loopCtx)
		r.reapOldWorkflowRuns(loopCtx)
		r.reapOldEventTriggers(loopCtx)
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

// completeSleepTrigger handles expired sleep triggers by completing the step
// rather than failing it — the sleep duration has elapsed successfully.
func (r *Reaper) completeSleepTrigger(ctx context.Context, trigger *domain.EventTrigger, now time.Time) {
	// Build sleep output metadata for downstream steps.
	sleptSecs := now.Sub(trigger.RequestedAt).Seconds()
	sleepOutput := json.RawMessage(fmt.Sprintf(
		`{"slept_for_secs":%.1f,"completed_at":%q}`,
		sleptSecs, now.UTC().Format(time.RFC3339),
	))

	receivedAt := now
	if err := r.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, sleepOutput, &receivedAt, ""); err != nil {
		slog.Error("failed to complete sleep trigger", "trigger_id", trigger.ID, "error", err)
		return
	}

	if trigger.WorkflowStepRunID != "" {
		if err := r.store.UpdateStepRunStatus(ctx, trigger.WorkflowStepRunID, domain.StepCompleted, map[string]any{
			"finished_at": now,
			"output":      sleepOutput,
		}); err != nil {
			slog.Error("failed to complete sleep step", "trigger_id", trigger.ID, "step_run_id", trigger.WorkflowStepRunID, "error", err)
			return
		}

		if trigger.WorkflowRunID != "" && r.workflowCallback != nil {
			r.workflowCallback.OnStepCompleted(ctx, trigger.WorkflowRunID, trigger.WorkflowStepRunID)
		}
	}

	if r.metrics != nil {
		attrs := metric.WithAttributes(
			attribute.String("source_type", trigger.SourceType),
			attribute.String("project_id", trigger.ProjectID),
			attribute.String("trigger_type", trigger.TriggerType),
		)
		waitDuration := now.Sub(trigger.RequestedAt).Seconds()
		r.metrics.EventTriggerWaitDuration.Record(ctx, waitDuration, attrs)
	}

	slog.Info("sleep trigger completed", "trigger_id", trigger.ID, "step_run_id", trigger.WorkflowStepRunID)
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

		// Delegate to the workflow callback, which respects on_failure policy
		// (continue, skip_dependents, fail_workflow). Falls back to directly
		// failing the workflow run when the callback is nil.
		if r.workflowCallback != nil {
			r.workflowCallback.OnStepFailed(ctx, approval.WorkflowRunID, approval.WorkflowStepRunID)
		} else {
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
		// Sleep triggers: expiry means completion (success), not timeout.
		if trigger.TriggerType == domain.TriggerTypeSleep {
			r.completeSleepTrigger(ctx, &trigger, now)
			continue
		}

		if err := r.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusTimedOut, nil, nil, "event trigger timed out"); err != nil {
			slog.Error("failed to mark event trigger timed out", "trigger_id", trigger.ID, "error", err)
			continue
		}

		if r.metrics != nil {
			attrs := metric.WithAttributes(
				attribute.String("source_type", trigger.SourceType),
				attribute.String("project_id", trigger.ProjectID),
			)
			r.metrics.EventTriggersTimedOut.Add(ctx, 1, attrs)
			waitDuration := now.Sub(trigger.RequestedAt).Seconds()
			r.metrics.EventTriggerWaitDuration.Record(ctx, waitDuration, attrs)
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

			// Delegate to the workflow callback, which respects on_failure policy
			// (continue, skip_dependents, fail_workflow). Falls back to directly
			// failing the workflow run when the callback is nil.
			if trigger.WorkflowRunID != "" {
				if r.workflowCallback != nil {
					r.workflowCallback.OnStepFailed(ctx, trigger.WorkflowRunID, trigger.WorkflowStepRunID)
				} else {
					if err := r.store.UpdateWorkflowRunStatus(ctx, trigger.WorkflowRunID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{
						"finished_at": now,
						"error":       "event trigger timed out",
					}); err != nil {
						if errPaused := r.store.UpdateWorkflowRunStatus(ctx, trigger.WorkflowRunID, domain.WfStatusPaused, domain.WfStatusFailed, map[string]any{
							"finished_at": now,
							"error":       "event trigger timed out",
						}); errPaused != nil {
							slog.Error("failed to fail workflow on event trigger timeout", "trigger_id", trigger.ID, "workflow_run_id", trigger.WorkflowRunID, "error", errPaused)
						}
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
				// Sleep triggers use OnStepCompleted; event triggers use OnEventReceived.
				if trigger.TriggerType == domain.TriggerTypeSleep {
					r.workflowCallback.OnStepCompleted(ctx, trigger.WorkflowRunID, trigger.WorkflowStepRunID)
				} else if err := r.workflowCallback.OnEventReceived(ctx, &trigger); err != nil {
					slog.Error("failed to reconcile event trigger step completion", "trigger_id", trigger.ID, "error", err)
					continue
				}
				slog.Info("reconciled inconsistent event trigger", "trigger_id", trigger.ID, "source_type", trigger.SourceType, "trigger_type", trigger.TriggerType)
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

func (r *Reaper) reapOldEventTriggers(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapOldEventTriggers")
	defer span.End()

	if r.eventTriggerRetention <= 0 {
		return
	}

	before := time.Now().Add(-r.eventTriggerRetention)
	count, err := r.store.DeleteEventTriggersFinishedBefore(ctx, before, r.deleteBatchLimit)
	if err != nil {
		slog.Error("failed to delete old event triggers", "error", err)
		return
	}
	if count > 0 {
		slog.Info("deleted old event triggers", "count", count, "before", before.UTC())
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
