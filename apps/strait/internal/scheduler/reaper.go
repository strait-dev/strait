package scheduler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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
	CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	ListReceivedEventTriggersWithStaleSteps(ctx context.Context) ([]domain.EventTrigger, error)
	DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

// DLQMonitorStore is an optional interface for DLQ depth monitoring.
type DLQMonitorStore interface {
	ListDLQDepthByJob(ctx context.Context) ([]DLQJobDepth, error)
}

// ReconciliationStore is an optional interface for orphaned step run and stuck webhook reconciliation.
type ReconciliationStore interface {
	ListOrphanedStepRuns(ctx context.Context) ([]store.OrphanedStepRun, error)
	ResetStuckWebhookDeliveries(ctx context.Context) (int64, error)
}

// QueueDepthMonitorStore is an optional interface for queue depth monitoring.
type QueueDepthMonitorStore interface {
	ListQueueDepthByJob(ctx context.Context) ([]store.QueueJobDepth, error)
}

// AutoRotateAPIKeysStore is an optional interface for automatic API key rotation.
type AutoRotateAPIKeysStore interface {
	ListAPIKeysDueRotation(ctx context.Context) ([]domain.APIKey, error)
	CreateAPIKey(ctx context.Context, key *domain.APIKey) error
	MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
}

// DLQJobDepth represents the dead-letter queue depth for a single job.
type DLQJobDepth struct {
	JobID             string
	WebhookURL        string
	DLQCount          int
	DLQAlertThreshold int
}

type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
	OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error
	OnStepCompleted(ctx context.Context, workflowRunID string, stepRunID string)
	OnStepFailed(ctx context.Context, workflowRunID string, stepRunID string)
	ResumeWorkflowRun(ctx context.Context, workflowRunID string) error
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

// MachineDestroyer can stop and destroy container machines (used for orphan cleanup).
type MachineDestroyer interface {
	Stop(ctx context.Context, machineID string) error
	Destroy(ctx context.Context, machineID string) error
}

type Reaper struct {
	store                 ReaperStore
	interval              time.Duration
	staleThreshold        time.Duration
	workflowRetention     time.Duration
	eventTriggerRetention time.Duration
	stalledThreshold      time.Duration
	deleteBatchLimit      int
	advisoryLocker        AdvisoryLocker
	shortRetention        time.Duration
	longRetention         time.Duration
	retentionEnabled      bool
	workflowCallback      WorkflowCallback
	machineDestroyer      MachineDestroyer
	metrics               *telemetry.Metrics
	logger                *slog.Logger
	stalledAction         string
	dlqAlertCooldown      map[string]time.Time
	queueAlertCooldown    map[string]time.Time
}

func (r *Reaper) recordOperation(ctx context.Context, operation, status string) {
	if r.metrics == nil {
		return
	}
	r.metrics.ReaperOperations.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("status", status),
	))
}

func (r *Reaper) recordDeleted(ctx context.Context, recordType string, count int64) {
	if r.metrics == nil || count <= 0 {
		return
	}
	r.metrics.ReaperRecordsDeleted.Add(ctx, count, metric.WithAttributes(attribute.String("type", recordType)))
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
		stalledThreshold:      15 * time.Minute,
		deleteBatchLimit:      defaultDeleteBatchLimit,
		shortRetention:        shortRetention,
		longRetention:         longRetention,
		retentionEnabled:      retentionEnabled,
		workflowCallback:      workflowCallback,
		logger:                slog.Default(),
		stalledAction:         "log_only",
		dlqAlertCooldown:      make(map[string]time.Time),
		queueAlertCooldown:    make(map[string]time.Time),
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

func (r *Reaper) WithStalledThreshold(d time.Duration) *Reaper {
	if d > 0 {
		r.stalledThreshold = d
	}
	return r
}

func (r *Reaper) WithStalledAction(action string) *Reaper {
	switch action {
	case "", "log_only", "reconcile", "fail_workflow":
		if action == "" {
			r.stalledAction = "log_only"
		} else {
			r.stalledAction = action
		}
	default:
		r.logger.Warn("invalid stalled action, using log_only", "action", action)
		r.stalledAction = "log_only"
	}
	return r
}

// WithAdvisoryLocker enables distributed single-leader reaping using pg_try_advisory_lock.
func (r *Reaper) WithAdvisoryLocker(locker AdvisoryLocker) *Reaper {
	r.advisoryLocker = locker
	return r
}

// WithMachineDestroyer sets the container runtime for orphaned machine cleanup.
func (r *Reaper) WithMachineDestroyer(d MachineDestroyer) *Reaper {
	r.machineDestroyer = d
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
	r.reapStalledWorkflows(ctx)
	r.reapOldWorkflowRuns(ctx)
	r.reapOldEventTriggers(ctx)
	r.monitorDLQDepth(ctx)
	r.reapOrphanedStepRuns(ctx)
	r.reapStuckWebhookDeliveries(ctx)
	r.monitorQueueDepth(ctx)
	r.autoRotateAPIKeys(ctx)
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
		r.reapStalledWorkflows(loopCtx)
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
	const operation = "reap_old_workflow_runs"

	if r.workflowRetention <= 0 {
		return
	}

	before := time.Now().Add(-r.workflowRetention)
	count, err := r.store.DeleteWorkflowRunsFinishedBefore(ctx, before, r.deleteBatchLimit)
	if err != nil {
		slog.Error("failed to delete old workflow runs", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "workflow_runs", count)
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
		now := time.Now()
		if err := r.store.UpdateWorkflowRunStatus(ctx, wfRun.ID, wfRun.Status, domain.WfStatusTimedOut, map[string]any{
			"finished_at": now,
			"error":       "workflow timed out",
		}); err != nil {
			slog.Error("failed to fail timed out workflow run", "workflow_run_id", wfRun.ID, "error", err)
			continue
		}

		if _, err := r.store.CancelNonTerminalStepRuns(ctx, wfRun.ID, now, "workflow timed out"); err != nil {
			slog.Error("failed to cancel step runs for timed out workflow", "workflow_run_id", wfRun.ID, "error", err)
		}

		if _, err := r.store.CancelJobRunsByWorkflowRun(ctx, wfRun.ID, now, "workflow timed out"); err != nil {
			slog.Error("failed to cancel job runs for timed out workflow", "workflow_run_id", wfRun.ID, "error", err)
		}

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
	const operation = "reap_expired_event_triggers"

	triggers, err := r.store.ListExpiredEventTriggers(ctx)
	if err != nil {
		slog.Error("failed to list expired event triggers", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")

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

func (r *Reaper) reapStalledWorkflows(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapStalledWorkflows")
	defer span.End()

	type stalledLister interface {
		ListStalledWorkflowRuns(ctx context.Context, threshold time.Duration) ([]domain.WorkflowRun, error)
	}
	lister, ok := r.store.(stalledLister)
	if !ok {
		return
	}

	runs, err := lister.ListStalledWorkflowRuns(ctx, r.stalledThreshold)
	if err != nil {
		slog.Error("failed to list stalled workflow runs", "error", err)
		return
	}
	for _, run := range runs {
		slog.Warn("detected stalled workflow run", "workflow_run_id", run.ID, "workflow_id", run.WorkflowID, "started_at", run.StartedAt, "action", r.stalledAction)
		if r.metrics != nil {
			r.metrics.WorkflowStalledRuns.Add(ctx, 1)
		}
		switch r.stalledAction {
		case "fail_workflow":
			now := time.Now()
			if err := r.store.UpdateWorkflowRunStatus(ctx, run.ID, run.Status, domain.WfStatusFailed, map[string]any{"finished_at": now, "error": "failed by stalled workflow recovery policy"}); err != nil {
				slog.Error("failed to fail stalled workflow run", "workflow_run_id", run.ID, "error", err)
			}
		case "reconcile":
			if r.workflowCallback == nil {
				slog.Warn("stalled workflow reconcile requested without callback", "workflow_run_id", run.ID)
				continue
			}
			if err := r.workflowCallback.ResumeWorkflowRun(ctx, run.ID); err != nil {
				slog.Error("failed to reconcile stalled workflow run", "workflow_run_id", run.ID, "error", err)
			} else {
				slog.Info("reconciled stalled workflow run", "workflow_run_id", run.ID)
			}
		}
	}
}

func (r *Reaper) reapOldEventTriggers(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapOldEventTriggers")
	defer span.End()
	const operation = "reap_old_event_triggers"

	if r.eventTriggerRetention <= 0 {
		return
	}

	before := time.Now().Add(-r.eventTriggerRetention)
	count, err := r.store.DeleteEventTriggersFinishedBefore(ctx, before, r.deleteBatchLimit)
	if err != nil {
		slog.Error("failed to delete old event triggers", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "event_triggers", count)
	if count > 0 {
		slog.Info("deleted old event triggers", "count", count, "before", before.UTC())
	}
}

func (r *Reaper) reapStale(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapStale")
	defer span.End()
	const operation = "reap_stale"

	runs, err := r.store.ListStaleRuns(ctx, r.staleThreshold)
	if err != nil {
		slog.Error("failed to list stale runs", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")

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

		// Clean up orphaned machines for managed runs.
		if r.machineDestroyer != nil && run.ExecutionMode == domain.ExecutionModeManaged && run.MachineID != "" {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 15*time.Second)
			if stopErr := r.machineDestroyer.Stop(cleanCtx, run.MachineID); stopErr != nil {
				r.logger.Warn("failed to stop orphaned machine, attempting destroy",
					"run_id", run.ID, "machine_id", run.MachineID, "error", stopErr)
			}
			if destroyErr := r.machineDestroyer.Destroy(cleanCtx, run.MachineID); destroyErr != nil {
				r.logger.Warn("failed to destroy orphaned machine",
					"run_id", run.ID, "machine_id", run.MachineID, "error", destroyErr)
			} else {
				r.logger.Info("destroyed orphaned machine", "run_id", run.ID, "machine_id", run.MachineID)
			}
			cleanCancel()
		}

		r.notifyWorkflowCallback(ctx, &run)

		slog.Warn("stale run marked crashed", "run_id", run.ID, "job_id", run.JobID)
	}
}

func (r *Reaper) reapExpired(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapExpired")
	defer span.End()
	const operation = "reap_expired"

	runs, err := r.store.ListExpiredRuns(ctx)
	if err != nil {
		slog.Error("failed to list expired runs", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")

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
	const operation = "reap_stale_dequeued"

	runs, err := r.store.ListStaleDequeued(ctx, r.staleThreshold)
	if err != nil {
		slog.Error("failed to list stale dequeued runs", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")

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
	const operation = "reap_terminal_retention"

	deleted, err := r.store.DeleteTerminalRunsPastRetention(ctx, r.shortRetention, r.longRetention)
	if err != nil {
		slog.Error("failed to delete retained terminal runs", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "terminal_runs", deleted)
	if deleted > 0 {
		slog.Info("deleted terminal runs past retention", "count", deleted)
	}
}

func (r *Reaper) monitorDLQDepth(ctx context.Context) {
	dlqStore, ok := r.store.(DLQMonitorStore)
	if !ok {
		return
	}

	depths, err := dlqStore.ListDLQDepthByJob(ctx)
	if err != nil {
		r.logger.Error("failed to query DLQ depth", "error", err)
		return
	}

	now := time.Now()
	for _, d := range depths {
		if r.metrics != nil {
			r.metrics.DLQDepth.Record(ctx, int64(d.DLQCount), metric.WithAttributes(
				attribute.String("job_id", d.JobID),
			))
		}

		// Check cooldown — skip if alerted within the last hour.
		if lastAlert, exists := r.dlqAlertCooldown[d.JobID]; exists && now.Sub(lastAlert) < time.Hour {
			continue
		}

		r.logger.Warn("DLQ threshold exceeded",
			"job_id", d.JobID,
			"dlq_count", d.DLQCount,
			"threshold", d.DLQAlertThreshold,
		)
		r.dlqAlertCooldown[d.JobID] = now
	}
}

func (r *Reaper) reapOrphanedStepRuns(ctx context.Context) {
	reconciler, ok := r.store.(ReconciliationStore)
	if !ok {
		return
	}

	orphans, err := reconciler.ListOrphanedStepRuns(ctx)
	if err != nil {
		r.logger.Error("failed to list orphaned step runs", "error", err)
		return
	}

	for _, o := range orphans {
		r.logger.Warn("reconciling orphaned step run",
			"step_run_id", o.StepRunID,
			"workflow_run_id", o.WorkflowRunID,
			"job_run_id", o.JobRunID,
			"job_status", o.JobStatus,
		)

		if o.JobStatus == domain.StatusCompleted {
			if r.workflowCallback != nil {
				r.workflowCallback.OnStepCompleted(ctx, o.WorkflowRunID, o.StepRunID)
			}
		} else {
			if r.workflowCallback != nil {
				r.workflowCallback.OnStepFailed(ctx, o.WorkflowRunID, o.StepRunID)
			}
		}
	}
}

func (r *Reaper) reapStuckWebhookDeliveries(ctx context.Context) {
	reconciler, ok := r.store.(ReconciliationStore)
	if !ok {
		return
	}

	count, err := reconciler.ResetStuckWebhookDeliveries(ctx)
	if err != nil {
		r.logger.Error("failed to reset stuck webhook deliveries", "error", err)
		return
	}

	if count > 0 {
		r.logger.Info("reset stuck webhook deliveries", "count", count)
	}
}

func (r *Reaper) autoRotateAPIKeys(ctx context.Context) {
	rotateStore, ok := r.store.(AutoRotateAPIKeysStore)
	if !ok {
		return
	}

	keys, err := rotateStore.ListAPIKeysDueRotation(ctx)
	if err != nil {
		r.logger.Error("failed to list api keys due rotation", "error", err)
		return
	}

	for _, oldKey := range keys {
		// Generate new key material.
		rawBytes := make([]byte, 32)
		if _, err := rand.Read(rawBytes); err != nil {
			r.logger.Error("failed to generate random key for rotation", "key_id", oldKey.ID, "error", err)
			continue
		}
		rawKey := "strait_" + hex.EncodeToString(rawBytes)
		keyHash := sha256.Sum256([]byte(rawKey))

		newKey := &domain.APIKey{
			ProjectID:            oldKey.ProjectID,
			Name:                 oldKey.Name + " (auto-rotated)",
			KeyHash:              hex.EncodeToString(keyHash[:]),
			KeyPrefix:            rawKey[:12],
			Scopes:               oldKey.Scopes,
			ExpiresAt:            oldKey.ExpiresAt,
			EnvironmentID:        oldKey.EnvironmentID,
			RotationIntervalDays: oldKey.RotationIntervalDays,
			RotationWebhookURL:   oldKey.RotationWebhookURL,
		}
		// Set next_rotation_at for the new key.
		if oldKey.RotationIntervalDays != nil && *oldKey.RotationIntervalDays > 0 {
			nextRotation := time.Now().Add(time.Duration(*oldKey.RotationIntervalDays) * 24 * time.Hour)
			newKey.NextRotationAt = &nextRotation
		}

		if err := rotateStore.CreateAPIKey(ctx, newKey); err != nil {
			r.logger.Error("failed to create rotated api key", "key_id", oldKey.ID, "error", err)
			continue
		}

		graceExpiresAt := time.Now().Add(24 * time.Hour) // 24h grace period
		if err := rotateStore.MarkAPIKeyRotated(ctx, oldKey.ID, newKey.ID, graceExpiresAt); err != nil {
			r.logger.Error("failed to mark old key as rotated", "key_id", oldKey.ID, "new_key_id", newKey.ID, "error", err)
			continue
		}

		r.logger.Info("auto-rotated api key", "old_key_id", oldKey.ID, "new_key_id", newKey.ID)

		// Record audit event.
		details, _ := json.Marshal(map[string]any{
			"old_key_id":       oldKey.ID,
			"new_key_id":       newKey.ID,
			"grace_expires_at": graceExpiresAt,
		})
		_ = rotateStore.CreateAuditEvent(ctx, &domain.AuditEvent{
			ProjectID:    oldKey.ProjectID,
			ActorID:      "system",
			ActorType:    "system",
			Action:       "api_key.auto_rotated",
			ResourceType: "api_key",
			ResourceID:   oldKey.ID,
			Details:      details,
		})

		// Notify rotation webhook if configured.
		if oldKey.RotationWebhookURL != "" {
			r.notifyRotationWebhook(ctx, oldKey.RotationWebhookURL, oldKey.ID, newKey.ID, newKey.KeyPrefix, oldKey.ProjectID)
		}
	}
}

func (r *Reaper) notifyRotationWebhook(ctx context.Context, webhookURL, oldKeyID, newKeyID, newKeyPrefix, projectID string) {
	payload, _ := json.Marshal(map[string]any{
		"event":          "api_key.auto_rotated",
		"old_key_id":     oldKeyID,
		"new_key_id":     newKeyID,
		"new_key_prefix": newKeyPrefix,
		"project_id":     projectID,
		"rotated_at":     time.Now().UTC(),
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		r.logger.Error("failed to create rotation webhook request", "url", webhookURL, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Strait-Event", "api_key.auto_rotated")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		r.logger.Warn("rotation webhook notification failed", "url", webhookURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		r.logger.Warn("rotation webhook returned non-success", "url", webhookURL, "status", resp.StatusCode)
	}
}

func (r *Reaper) monitorQueueDepth(ctx context.Context) {
	qdStore, ok := r.store.(QueueDepthMonitorStore)
	if !ok {
		return
	}

	depths, err := qdStore.ListQueueDepthByJob(ctx)
	if err != nil {
		r.logger.Error("failed to query queue depth", "error", err)
		return
	}

	now := time.Now()
	for _, d := range depths {
		if r.metrics != nil {
			r.metrics.QueueDepthPerJob.Record(ctx, int64(d.QueuedCount), metric.WithAttributes(
				attribute.String("job_id", d.JobID),
			))
		}

		if lastAlert, exists := r.queueAlertCooldown[d.JobID]; exists && now.Sub(lastAlert) < time.Hour {
			continue
		}

		r.logger.Warn("queue depth threshold exceeded",
			"job_id", d.JobID,
			"queued_count", d.QueuedCount,
			"threshold", d.QueueDepthAlertThreshold,
		)
		r.queueAlertCooldown[d.JobID] = now
	}
}
