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
	"net/url"
	"strings"
	"time"

	"strait/internal/clickhouse"
	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/google/uuid"
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
	DeleteRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error)
	DeleteWorkflowRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error)
	DeleteAuditEventsBefore(ctx context.Context, projectID string, cutoff time.Time) (int64, error)
	DeleteAuditEventsBeforeExcluding(ctx context.Context, cutoff time.Time, excludeProjectIDs []string) (int64, error)
	ListAuditRetentionOverrides(ctx context.Context) ([]store.AuditRetentionOverride, error)
	ListAuditEventsDeadletter(ctx context.Context, limit int) ([]domain.AuditEvent, []string, error)
	ListAuditEventsDeadletterWithAttempts(ctx context.Context, limit int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error)
	IncrementAuditDeadletterAttempt(ctx context.Context, id string) error
	MarkAuditDeadletterReclaimed(ctx context.Context, dlqID, newEventID string) error
	ReplayAuditEventDeadletter(ctx context.Context, id, projectID, newEventID string) (*domain.AuditEvent, bool, error)
	DeleteAuditDeadletterOlderThan(ctx context.Context, cutoff time.Time) (map[string]int64, error)
	CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
	DeleteAuditEventDeadletter(ctx context.Context, id, projectID string) error
	ArchiveTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration, batchSize int) (int64, error)
	DeleteHistoryRunsPastRetention(ctx context.Context, cutoff time.Time, limit int) (int64, error)
	ArchiveConsumedOutboxBatch(ctx context.Context, olderThan time.Duration, batchSize int) (int64, error)
	DeleteOutboxHistoryPastRetention(ctx context.Context, cutoff time.Time, limit int) (int64, error)
	PurgeQuarantinedOutboxOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error)
	GetRunFromHistory(ctx context.Context, id string) (*domain.JobRun, error)
}

type staleRunRetryStore interface {
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	ScheduleRetry(ctx context.Context, runID string, at time.Time, attempt int) error
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
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	CreateAPIKey(ctx context.Context, key *domain.APIKey) error
	CreateRotatedAPIKey(ctx context.Context, oldKeyID string, newKey *domain.APIKey, graceExpiresAt time.Time) error
	MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	RevokeAPIKey(ctx context.Context, id string) error
	DisableAPIKeyAutoRotation(ctx context.Context, id string) error
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
	ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error
}

// CostGateDefaultActionStore is an optional interface for looking up cost gate default actions.
type CostGateDefaultActionStore interface {
	GetCostGateDefaultAction(ctx context.Context, stepRunID string) (string, error)
}

// ApprovalNotifierStore is an optional interface for sending approval lifecycle notifications.
type ApprovalNotifierStore interface {
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
}

// ApprovalNotifierBulkChannelStore optionally bulk-lists channels for approval notifications.
type ApprovalNotifierBulkChannelStore interface {
	ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error)
}

// AdvisoryLocker attempts to acquire a PostgreSQL advisory lock.
// Returns true if the lock was acquired (caller should run reaper),
// false if another instance holds it.
type AdvisoryLocker interface {
	TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error)
	ReleaseAdvisoryLock(ctx context.Context, lockID int64) error
}

type AdvisoryLockRunner interface {
	RunWithAdvisoryLock(ctx context.Context, lockID int64, fn func(context.Context) error) (bool, error)
}

// reaperAdvisoryLockID is the pg_advisory_lock key for single-leader reaper.
const reaperAdvisoryLockID int64 = 0x5374726169745265 // "StraitRe" as int64

// ApprovalReminderStore is an optional interface for querying approvals past their reminder point.
type ApprovalReminderStore interface {
	ListApprovalsPastReminderPoint(ctx context.Context) ([]domain.WorkflowStepApproval, error)
}

// OrgRetentionResolver resolves the retention period for an organization based on its plan.
type OrgRetentionResolver interface {
	ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error)
	GetOrgRetentionDays(ctx context.Context, orgID string) (int, error)
}

type Reaper struct {
	store                      ReaperStore
	interval                   time.Duration
	staleThreshold             time.Duration
	workflowRetention          time.Duration
	eventTriggerRetention      time.Duration
	stalledThreshold           time.Duration
	deleteBatchLimit           int
	advisoryLocker             AdvisoryLocker
	shortRetention             time.Duration
	longRetention              time.Duration
	retentionEnabled           bool
	workflowCallback           WorkflowCallback
	chExporter                 *clickhouse.Exporter
	metrics                    *telemetry.Metrics
	logger                     *slog.Logger
	stalledAction              string
	dlqAlertCooldown           map[string]time.Time
	queueAlertCooldown         map[string]time.Time
	reminderSent               map[string]time.Time
	orgRetention               OrgRetentionResolver
	auditRetentionDefaultDays  int
	auditDLQReclaimBatch       int
	auditDLQMaxAgeDays         int
	auditDLQMaxReclaimAttempts int
	archiveEnabled             bool
	allowPrivateEndpoints      bool
	rotationWebhookClient      *http.Client
	rotationSecretDecryptor    SecretDecryptor
}

// SecretDecryptor decrypts at-rest secrets such as rotation webhook signing
// keys. The reaper uses it to sign outbound rotation webhooks.
type SecretDecryptor interface {
	Decrypt(ciphertext []byte) ([]byte, error)
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
func NewReaper(
	s ReaperStore,
	interval time.Duration,
	staleThreshold time.Duration,
	shortRetention time.Duration,
	longRetention time.Duration,
	retentionEnabled bool,
	workflowCallback WorkflowCallback,
) *Reaper {
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
		stalledAction:         "reconcile",
		dlqAlertCooldown:      make(map[string]time.Time),
		queueAlertCooldown:    make(map[string]time.Time),
		reminderSent:          make(map[string]time.Time),
	}
}

// WithMetrics sets the telemetry metrics for the reaper.
func (r *Reaper) WithMetrics(m *telemetry.Metrics) *Reaper {
	r.metrics = m
	return r
}

// WithRotationSecretDecryptor sets the decryptor used to recover at-rest
// rotation webhook signing secrets. When unset, rotation webhooks fail closed
// because the new API key can only be delivered over a signed request.
func (r *Reaper) WithRotationSecretDecryptor(d SecretDecryptor) *Reaper {
	r.rotationSecretDecryptor = d
	return r
}

// WithAllowPrivateEndpoints allows scheduler-originated webhooks to target
// private addresses. It is intended for explicitly configured self-hosted
// deployments and tests; production defaults to the SSRF-safe external policy.
func (r *Reaper) WithAllowPrivateEndpoints(allow bool) *Reaper {
	r.allowPrivateEndpoints = allow
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

// WithOrgRetention enables per-org plan-based retention cleanup.
// When set, the reaper periodically queries each org's plan retention and
// prunes runs older than the plan allows.
func (r *Reaper) WithOrgRetention(resolver OrgRetentionResolver) *Reaper {
	r.orgRetention = resolver
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
			r.stalledAction = "reconcile"
		} else {
			r.stalledAction = action
		}
	default:
		r.logger.Warn("invalid stalled action, using reconcile", "action", action)
		r.stalledAction = "reconcile"
	}
	return r
}

// WithAdvisoryLocker enables distributed single-leader reaping using pg_try_advisory_lock.
func (r *Reaper) WithAdvisoryLocker(locker AdvisoryLocker) *Reaper {
	r.advisoryLocker = locker
	return r
}

// WithChExporter attaches the ClickHouse exporter for event trigger timeout analytics.
func (r *Reaper) WithChExporter(e *clickhouse.Exporter) *Reaper {
	r.chExporter = e
	return r
}

func (r *Reaper) WithArchiveEnabled(enabled bool) *Reaper {
	r.archiveEnabled = enabled
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
	r.runMaintenanceCycle(ctx)
}

func (r *Reaper) Run(ctx context.Context) {
	r.logger.Info("reaper configured", "interval", r.interval, "stale_threshold", r.staleThreshold)
	loop := NewMaintenanceLoop("reaper", r.interval, r.logger, func(loopCtx context.Context) {
		runCycle := func(ctx context.Context) error {
			r.runMaintenanceCycle(ctx)
			if r.retentionEnabled {
				r.reapTerminalRetention(ctx)
				r.reapPerOrgRetention(ctx)
			}
			return nil
		}

		if r.advisoryLocker != nil {
			if runner, ok := r.advisoryLocker.(AdvisoryLockRunner); ok {
				acquired, err := runner.RunWithAdvisoryLock(loopCtx, reaperAdvisoryLockID, runCycle)
				if err != nil {
					r.logger.Error("advisory locked reaper cycle failed", "error", err)
					return
				}
				if !acquired {
					r.logger.Debug("reaper advisory lock held by another instance, skipping cycle")
				}
				return
			}

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

		_ = runCycle(loopCtx)
	})
	loop.Run(ctx)
}

func (r *Reaper) runMaintenanceCycle(ctx context.Context) {
	r.reapStaleDequeued(ctx)
	r.reapStale(ctx)
	r.reapExpired(ctx)
	r.reapTimedOutWorkflows(ctx)
	r.reapExpiredApprovals(ctx)
	r.reapApprovalReminders(ctx)
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
	r.reapAuditEvents(ctx)
	r.reclaimAuditDeadletter(ctx)
	r.reapDeadletter(ctx)
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
		// Cost gate approvals with default_action=approve should auto-approve on timeout.
		if strings.HasPrefix(approval.ID, "costgate:") {
			if r.handleCostGateTimeout(ctx, &approval) {
				continue
			}
		}

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

		r.sendApprovalNotification(ctx, approval.WorkflowRunID, domain.NotificationEventApprovalExpired, map[string]any{
			"approval_id":     approval.ID,
			"workflow_run_id": approval.WorkflowRunID,
			"step_run_id":     approval.WorkflowStepRunID,
			"expired_at":      now,
		})

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

func (r *Reaper) reapApprovalReminders(ctx context.Context) {
	rs, ok := r.store.(ApprovalReminderStore)
	if !ok {
		return
	}
	ns, hasNotifier := r.store.(ApprovalNotifierStore)
	if !hasNotifier {
		return
	}

	// Clean stale entries from reminderSent once the approval has expired.
	now := time.Now()
	for id, expiresAt := range r.reminderSent {
		if now.After(expiresAt) {
			delete(r.reminderSent, id)
		}
	}

	approvals, err := rs.ListApprovalsPastReminderPoint(ctx)
	if err != nil {
		slog.Warn("failed to list approvals past reminder point", "error", err)
		return
	}
	if len(approvals) == 0 {
		return
	}

	workflowRuns := make(map[string]*domain.WorkflowRun)
	channelsByProject := make(map[string][]domain.NotificationChannel)
	projectIDs := make([]string, 0, len(approvals))
	seenProjectIDs := make(map[string]struct{})
	for _, approval := range approvals {
		if _, sent := r.reminderSent[approval.ID]; sent {
			continue
		}

		wfRun, ok := workflowRuns[approval.WorkflowRunID]
		if !ok {
			var wfErr error
			wfRun, wfErr = ns.GetWorkflowRun(ctx, approval.WorkflowRunID)
			if wfErr != nil {
				slog.Warn("failed to get workflow run for approval reminder", "workflow_run_id", approval.WorkflowRunID, "error", wfErr)
				continue
			}
			if wfRun == nil {
				slog.Warn("failed to get workflow run for approval reminder", "workflow_run_id", approval.WorkflowRunID)
				continue
			}
			workflowRuns[approval.WorkflowRunID] = wfRun
		}

		if _, seen := seenProjectIDs[wfRun.ProjectID]; !seen {
			seenProjectIDs[wfRun.ProjectID] = struct{}{}
			projectIDs = append(projectIDs, wfRun.ProjectID)
		}
	}

	if bulkStore, ok := r.store.(ApprovalNotifierBulkChannelStore); ok && len(projectIDs) > 0 {
		bulkChannels, chBulkErr := bulkStore.ListEnabledNotificationChannelsByProjectIDs(ctx, projectIDs)
		if chBulkErr != nil {
			slog.Warn("failed to bulk-list notification channels for approval reminders", "error", chBulkErr)
		} else {
			channelsByProject = bulkChannels
		}
	}

	for _, approval := range approvals {
		if _, sent := r.reminderSent[approval.ID]; sent {
			continue
		}
		wfRun := workflowRuns[approval.WorkflowRunID]
		if wfRun == nil {
			continue
		}

		var timeRemainingSecs float64
		if approval.ExpiresAt != nil {
			timeRemainingSecs = time.Until(*approval.ExpiresAt).Seconds()
		}

		channels, ok := channelsByProject[wfRun.ProjectID]
		if !ok {
			var chErr error
			channels, chErr = ns.ListEnabledNotificationChannels(ctx, wfRun.ProjectID)
			if chErr != nil {
				slog.Warn("failed to list notification channels for approval reminder", "error", chErr)
				continue
			}
			channelsByProject[wfRun.ProjectID] = channels
		}

		payload, marshalErr := json.Marshal(struct {
			ApprovalID        string  `json:"approval_id"`
			WorkflowRunID     string  `json:"workflow_run_id"`
			WorkflowID        string  `json:"workflow_id"`
			StepRunID         string  `json:"step_run_id"`
			TimeRemainingSecs float64 `json:"time_remaining_secs"`
		}{
			ApprovalID:        approval.ID,
			WorkflowRunID:     approval.WorkflowRunID,
			WorkflowID:        wfRun.WorkflowID,
			StepRunID:         approval.WorkflowStepRunID,
			TimeRemainingSecs: timeRemainingSecs,
		})
		if marshalErr != nil {
			continue
		}

		for _, ch := range channels {
			d := &domain.NotificationDelivery{
				ChannelID:   ch.ID,
				ProjectID:   wfRun.ProjectID,
				EventType:   domain.NotificationEventApprovalReminder,
				Payload:     payload,
				Status:      "pending",
				MaxAttempts: 3,
			}
			if err := ns.CreateNotificationDelivery(ctx, d); err != nil {
				slog.Warn("failed to create approval reminder delivery", "channel_id", ch.ID, "error", err)
			}
		}

		if approval.ExpiresAt != nil {
			r.reminderSent[approval.ID] = *approval.ExpiresAt
		} else {
			r.reminderSent[approval.ID] = now.Add(time.Hour)
		}
	}
}

// sendApprovalNotification sends an approval lifecycle notification through configured channels.
// It type-asserts the store to ApprovalNotifierStore; if the store does not implement it, this is a no-op.
func (r *Reaper) sendApprovalNotification(ctx context.Context, workflowRunID, eventType string, payload map[string]any) {
	ns, ok := r.store.(ApprovalNotifierStore)
	if !ok {
		return
	}

	wfRun, err := ns.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		slog.Warn("failed to get workflow run for approval notification", "workflow_run_id", workflowRunID, "error", err)
		return
	}
	if wfRun == nil {
		slog.Warn("failed to get workflow run for approval notification", "workflow_run_id", workflowRunID)
		return
	}

	channels, err := ns.ListEnabledNotificationChannels(ctx, wfRun.ProjectID)
	if err != nil {
		slog.Warn("failed to list notification channels for approval notification", "project_id", wfRun.ProjectID, "error", err)
		return
	}

	payload["workflow_id"] = wfRun.WorkflowID
	payloadBytes, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		slog.Warn("failed to marshal approval notification payload", "error", marshalErr)
		return
	}

	for _, ch := range channels {
		d := &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   wfRun.ProjectID,
			EventType:   eventType,
			Payload:     payloadBytes,
			Status:      "pending",
			MaxAttempts: 3,
		}
		if err := ns.CreateNotificationDelivery(ctx, d); err != nil {
			slog.Warn("failed to create approval notification delivery",
				"channel_id", ch.ID, "event_type", eventType, "error", err)
		}
	}
}

// handleCostGateTimeout checks if a cost gate approval should be auto-approved on timeout.
// Returns true if the approval was handled (auto-approved), false to fall through to default fail behavior.
func (r *Reaper) handleCostGateTimeout(ctx context.Context, approval *domain.WorkflowStepApproval) bool {
	if r.workflowCallback == nil {
		return false
	}

	cgStore, ok := r.store.(CostGateDefaultActionStore)
	if !ok {
		return false
	}

	action, err := cgStore.GetCostGateDefaultAction(ctx, approval.WorkflowStepRunID)
	if err != nil {
		slog.Warn("failed to look up cost gate default action", "approval_id", approval.ID, "error", err)
		return false
	}
	if action != "approve" {
		return false
	}

	// Look up step run to get the step ref needed by ApproveStep.
	type stepRunGetter interface {
		GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	}
	srGetter, ok := r.store.(stepRunGetter)
	if !ok {
		return false
	}

	stepRun, srErr := srGetter.GetWorkflowStepRun(ctx, approval.WorkflowStepRunID)
	if srErr != nil || stepRun == nil {
		slog.Warn("failed to get step run for cost gate auto-approve", "approval_id", approval.ID, "error", srErr)
		return false
	}

	if err := r.workflowCallback.ApproveStep(ctx, approval.WorkflowRunID, stepRun.StepRef, "system:cost-gate-timeout"); err != nil {
		slog.Error("failed to auto-approve cost gate on timeout", "approval_id", approval.ID, "error", err)
		return false
	}

	slog.Info("cost gate auto-approved on timeout", "approval_id", approval.ID, "workflow_run_id", approval.WorkflowRunID, "step_ref", stepRun.StepRef)
	return true
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

		if r.chExporter != nil {
			waitDurationMs := uint64(max(now.Sub(trigger.RequestedAt).Milliseconds(), 0))
			r.chExporter.Enqueue(clickhouse.EventTriggerEventRecord{
				TriggerID:      trigger.ID,
				EventKey:       trigger.EventKey,
				ProjectID:      trigger.ProjectID,
				SourceType:     trigger.SourceType,
				Status:         domain.EventTriggerStatusTimedOut,
				TimeoutSecs:    uint32(max(trigger.TimeoutSecs, 0)), //nolint:gosec // timeout is always non-negative
				WaitDurationMs: waitDurationMs,
				CreatedAt:      trigger.RequestedAt,
			})
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
			fields := map[string]any{
				"finished_at": now,
				"error":       "failed by stalled workflow recovery policy",
			}
			if err := r.store.UpdateWorkflowRunStatus(ctx, run.ID, run.Status, domain.WfStatusFailed, fields); err != nil {
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
		if r.retryStaleRun(ctx, &run) {
			continue
		}
		r.crashStaleRun(ctx, &run)
	}
}

func (r *Reaper) retryStaleRun(ctx context.Context, run *domain.JobRun) bool {
	retryStore, ok := r.store.(staleRunRetryStore)
	if !ok {
		return false
	}

	job, err := retryStore.GetJob(ctx, run.JobID)
	if err != nil {
		slog.Warn("failed to load job for stale run retry decision", "run_id", run.ID, "job_id", run.JobID, "error", err)
		return false
	}
	if job == nil {
		return false
	}

	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if run.Attempt >= maxAttempts {
		return false
	}

	nextAttempt := run.Attempt + 1
	retryAt := nextStaleRunRetryAt(run.Attempt)
	if err := retryStore.ScheduleRetry(ctx, run.ID, retryAt, nextAttempt); err != nil {
		slog.Error("failed to schedule stale run retry", "run_id", run.ID, "job_id", run.JobID, "error", err)
		return false
	}

	fields := map[string]any{
		"attempt":      nextAttempt,
		"started_at":   nil,
		"finished_at":  nil,
		"heartbeat_at": nil,
		"error":        "heartbeat lost; retry scheduled",
		"error_class":  "transient",
	}
	if err := r.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
		slog.Error("failed to requeue stale run", "run_id", run.ID, "job_id", run.JobID, "error", err)
		return false
	}

	run.Attempt = nextAttempt
	run.Status = domain.StatusQueued
	slog.Warn("stale run requeued after heartbeat loss", "run_id", run.ID, "job_id", run.JobID, "attempt", nextAttempt, "next_retry_at", retryAt)
	return true
}

func (r *Reaper) crashStaleRun(ctx context.Context, run *domain.JobRun) {
	err := r.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCrashed, map[string]any{
		"finished_at": time.Now(),
		"error":       "heartbeat lost",
	})
	if err != nil {
		slog.Error("failed to crash stale run", "run_id", run.ID, "job_id", run.JobID, "error", err)
		return
	}
	run.Status = domain.StatusCrashed

	r.notifyWorkflowCallback(ctx, run)

	slog.Warn("stale run marked crashed", "run_id", run.ID, "job_id", run.JobID)
}

func nextStaleRunRetryAt(attempt int) time.Time {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second
	for range attempt - 1 {
		if delay >= time.Hour/2 {
			delay = time.Hour
			break
		}
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	return time.Now().Add(delay)
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

	if r.archiveEnabled {
		r.archiveTerminalRuns(ctx)
		r.archiveConsumedOutbox(ctx)
		r.purgeStaleQuarantinedOutbox(ctx)
		r.reapHistoryRetention(ctx)
		r.reapOutboxHistoryRetention(ctx)
		return
	}

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

func (r *Reaper) archiveTerminalRuns(ctx context.Context) {
	const operation = "archive_terminal_runs"
	batchSize := r.deleteBatchLimit
	if batchSize <= 0 {
		batchSize = 100
	}

	archived, err := r.store.ArchiveTerminalRunsPastRetention(ctx, r.shortRetention, r.longRetention, batchSize)
	if err != nil {
		slog.Error("failed to archive terminal runs", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "archived_runs", archived)
	if archived > 0 {
		slog.Info("archived terminal runs to history", "count", archived)
	}
}

func (r *Reaper) reapHistoryRetention(ctx context.Context) {
	const operation = "reap_history_retention"
	batchSize := r.deleteBatchLimit
	if batchSize <= 0 {
		batchSize = 100
	}

	cutoff := time.Now().Add(-r.longRetention)
	deleted, err := r.store.DeleteHistoryRunsPastRetention(ctx, cutoff, batchSize)
	if err != nil {
		slog.Error("failed to delete history runs past retention", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "history_runs", deleted)
	if deleted > 0 {
		slog.Info("deleted history runs past retention", "count", deleted)
	}
}

func (r *Reaper) archiveConsumedOutbox(ctx context.Context) {
	const operation = "archive_consumed_outbox"
	batchSize := r.deleteBatchLimit
	if batchSize <= 0 {
		batchSize = 100
	}

	archived, err := r.store.ArchiveConsumedOutboxBatch(ctx, r.shortRetention, batchSize)
	if err != nil {
		slog.Error("failed to archive consumed outbox rows", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "archived_outbox", archived)
	if archived > 0 {
		slog.Info("archived consumed outbox rows to history", "count", archived)
	}
}

func (r *Reaper) reapOutboxHistoryRetention(ctx context.Context) {
	const operation = "reap_outbox_history_retention"
	batchSize := r.deleteBatchLimit
	if batchSize <= 0 {
		batchSize = 100
	}

	cutoff := time.Now().Add(-r.longRetention)
	deleted, err := r.store.DeleteOutboxHistoryPastRetention(ctx, cutoff, batchSize)
	if err != nil {
		slog.Error("failed to delete outbox history past retention", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "outbox_history", deleted)
	if deleted > 0 {
		slog.Info("deleted outbox history past retention", "count", deleted)
	}
}

func (r *Reaper) purgeStaleQuarantinedOutbox(ctx context.Context) {
	const operation = "purge_stale_quarantined_outbox"
	batchSize := r.deleteBatchLimit
	if batchSize <= 0 {
		batchSize = 100
	}

	cutoff := time.Now().Add(-r.longRetention)
	deleted, err := r.store.PurgeQuarantinedOutboxOlderThan(ctx, cutoff, batchSize)
	if err != nil {
		slog.Error("failed to purge stale quarantined outbox rows", "error", err)
		r.recordOperation(ctx, operation, "error")
		return
	}
	r.recordOperation(ctx, operation, "success")
	r.recordDeleted(ctx, "quarantined_outbox", deleted)
	if deleted > 0 {
		slog.Info("purged stale quarantined outbox rows", "count", deleted)
	}
}

func (r *Reaper) reapPerOrgRetention(ctx context.Context) {
	if r.orgRetention == nil {
		return
	}

	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapPerOrgRetention")
	defer span.End()

	orgIDs, err := r.orgRetention.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		r.logger.Warn("failed to list org IDs for retention sweep", "error", err)
		return
	}

	var totalDeleted int64
	for _, orgID := range orgIDs {
		days, retErr := r.orgRetention.GetOrgRetentionDays(ctx, orgID)
		if retErr != nil || days <= 0 {
			continue
		}

		retention := time.Duration(days) * 24 * time.Hour
		deleted, delErr := r.store.DeleteRunsByOrgOlderThan(ctx, orgID, retention)
		if delErr != nil {
			r.logger.Warn("failed to delete retained runs for org",
				"org_id", orgID, "retention_days", days, "error", delErr)
			continue
		}
		totalDeleted += deleted

		wfDeleted, wfErr := r.store.DeleteWorkflowRunsByOrgOlderThan(ctx, orgID, retention)
		if wfErr != nil {
			r.logger.Warn("failed to delete retained workflow runs for org",
				"org_id", orgID, "retention_days", days, "error", wfErr)
			continue
		}
		totalDeleted += wfDeleted
	}

	if totalDeleted > 0 {
		r.logger.Info("per-org retention sweep completed", "total_deleted", totalDeleted, "orgs_checked", len(orgIDs))
		r.recordDeleted(ctx, "per_org_retention", totalDeleted)
	}
}

// alertCooldownPruneTTL is the maximum age of an entry retained in the
// DLQ / queue-depth alert cooldown maps before it is dropped on the next
// monitoring pass.
const alertCooldownPruneTTL = 24 * time.Hour

// pruneAlertCooldowns removes entries from the DLQ and queue-depth alert
// cooldown maps once they are older than alertCooldownPruneTTL. Without
// this, a long-lived reaper accumulates one entry per ever-seen job, since
// job IDs are never removed even when the job is deleted. Called at the
// start of each monitoring pass so the maps stay bounded by
// recently-active job IDs.
func (r *Reaper) pruneAlertCooldowns(now time.Time) {
	for k, t := range r.dlqAlertCooldown {
		if now.Sub(t) > alertCooldownPruneTTL {
			delete(r.dlqAlertCooldown, k)
		}
	}
	for k, t := range r.queueAlertCooldown {
		if now.Sub(t) > alertCooldownPruneTTL {
			delete(r.queueAlertCooldown, k)
		}
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
	r.pruneAlertCooldowns(now)
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
		if oldKey.RotationWebhookURL == "" {
			r.logger.Warn("skipping api key auto-rotation without rotation webhook", "key_id", oldKey.ID, "project_id", oldKey.ProjectID)
			if err := rotateStore.DisableAPIKeyAutoRotation(ctx, oldKey.ID); err != nil {
				r.logger.Error("failed to disable api key auto-rotation without webhook", "key_id", oldKey.ID, "project_id", oldKey.ProjectID, "error", err)
			}
			continue
		}
		if _, err := r.rotationWebhookSigningSecret(oldKey.RotationWebhookSecret, oldKey.ID, oldKey.ProjectID); err != nil {
			r.logger.Warn(
				"skipping api key auto-rotation without a usable rotation webhook signing secret",
				"key_id", oldKey.ID,
				"project_id", oldKey.ProjectID,
				"error", err,
			)
			continue
		}
		if err := validateRotationWebhookURL(oldKey.RotationWebhookURL, r.allowPrivateEndpoints); err != nil {
			r.logger.Warn("skipping api key auto-rotation with invalid rotation webhook URL",
				"key_id", oldKey.ID,
				"project_id", oldKey.ProjectID,
				"url", httputil.RedactURLForLog(oldKey.RotationWebhookURL),
				"error", err,
			)
			continue
		}

		expiresAt, err := autoRotatedAPIKeyExpiry(ctx, rotateStore, oldKey)
		if err != nil {
			r.logger.Warn("skipping api key auto-rotation that violates lifetime policy", "key_id", oldKey.ID, "project_id", oldKey.ProjectID, "error", err)
			continue
		}

		// Generate new key material.
		rawBytes := make([]byte, 32)
		if _, err := rand.Read(rawBytes); err != nil {
			r.logger.Error("failed to generate random key for rotation", "key_id", oldKey.ID, "error", err)
			continue
		}
		rawKey := "strait_" + hex.EncodeToString(rawBytes)
		keyHash := sha256.Sum256([]byte(rawKey))

		newKey := &domain.APIKey{
			ID:                    uuid.Must(uuid.NewV7()).String(),
			ProjectID:             oldKey.ProjectID,
			OrgID:                 oldKey.OrgID,
			Name:                  oldKey.Name + " (auto-rotated)",
			KeyHash:               hex.EncodeToString(keyHash[:]),
			KeyPrefix:             rawKey[:domain.APIKeyPrefixLen],
			Scopes:                oldKey.Scopes,
			ExpiresAt:             expiresAt,
			EnvironmentID:         oldKey.EnvironmentID,
			RotationIntervalDays:  oldKey.RotationIntervalDays,
			RotationWebhookURL:    oldKey.RotationWebhookURL,
			RotationWebhookSecret: oldKey.RotationWebhookSecret,
		}
		// Set next_rotation_at for the new key.
		if oldKey.RotationIntervalDays != nil && *oldKey.RotationIntervalDays > 0 {
			nextRotation := time.Now().Add(time.Duration(*oldKey.RotationIntervalDays) * 24 * time.Hour)
			newKey.NextRotationAt = &nextRotation
		}

		graceExpiresAt := time.Now().Add(24 * time.Hour) // 24h grace period
		if err := rotateStore.CreateAPIKey(ctx, newKey); err != nil {
			r.logger.Error("failed to create auto-rotated api key before webhook delivery", "key_id", oldKey.ID, "new_key_id", newKey.ID, "error", err)
			continue
		}

		if err := r.notifyRotationWebhook(
			ctx,
			oldKey.RotationWebhookURL,
			oldKey.RotationWebhookSecret,
			oldKey.ID,
			newKey.ID,
			rawKey,
			newKey.KeyPrefix,
			oldKey.ProjectID,
		); err != nil {
			r.logger.Warn(
				"rotation webhook notification failed; revoking undelivered new key and keeping old key active",
				"key_id", oldKey.ID,
				"new_key_id", newKey.ID,
				"error", err,
			)
			if revokeErr := rotateStore.RevokeAPIKey(ctx, newKey.ID); revokeErr != nil {
				r.logger.Error("failed to revoke undelivered auto-rotated api key", "key_id", oldKey.ID, "new_key_id", newKey.ID, "error", revokeErr)
			}
			continue
		}

		if err := rotateStore.MarkAPIKeyRotated(ctx, oldKey.ID, newKey.ID, graceExpiresAt); err != nil {
			r.logger.Error("failed to mark old api key rotated; revoking delivered replacement", "key_id", oldKey.ID, "new_key_id", newKey.ID, "error", err)
			if revokeErr := rotateStore.RevokeAPIKey(ctx, newKey.ID); revokeErr != nil {
				r.logger.Error("failed to revoke auto-rotated api key after mark failure", "key_id", oldKey.ID, "new_key_id", newKey.ID, "error", revokeErr)
			}
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
	}
}

func autoRotatedAPIKeyExpiry(ctx context.Context, rotateStore AutoRotateAPIKeysStore, oldKey domain.APIKey) (*time.Time, error) {
	quota, err := rotateStore.GetProjectQuota(ctx, oldKey.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("load project quota: %w", err)
	}
	maxLifetimeDays := 0
	if quota != nil {
		maxLifetimeDays = quota.MaxKeyLifetimeDays
	}
	return domain.ApplyAPIKeyLifetimePolicy(time.Now(), oldKey.ExpiresAt, maxLifetimeDays)
}

type apiKeyRotationWebhook struct {
	URL             string
	EncryptedSecret []byte
	OldKeyID        string
	NewKeyID        string
	NewKey          string
	NewKeyPrefix    string
	ProjectID       string
}

func (r *Reaper) notifyRotationWebhook(
	ctx context.Context,
	webhookURL string,
	encryptedSecret []byte,
	oldKeyID string,
	newKeyID string,
	newKey string,
	newKeyPrefix string,
	projectID string,
) error {
	return r.notifyRotationWebhookRequest(ctx, apiKeyRotationWebhook{
		URL:             webhookURL,
		EncryptedSecret: encryptedSecret,
		OldKeyID:        oldKeyID,
		NewKeyID:        newKeyID,
		NewKey:          newKey,
		NewKeyPrefix:    newKeyPrefix,
		ProjectID:       projectID,
	})
}

func (r *Reaper) notifyRotationWebhookRequest(ctx context.Context, webhook apiKeyRotationWebhook) error {
	logURL := httputil.RedactURLForLog(webhook.URL)
	if err := validateRotationWebhookURL(webhook.URL, r.allowPrivateEndpoints); err != nil {
		r.logger.Warn("rotation webhook URL blocked", "url", logURL, "error", err)
		return err
	}

	payload, _ := json.Marshal(map[string]any{
		"event":          "api_key.auto_rotated",
		"old_key_id":     webhook.OldKeyID,
		"new_key_id":     webhook.NewKeyID,
		"new_key":        webhook.NewKey,
		"new_key_prefix": webhook.NewKeyPrefix,
		"project_id":     webhook.ProjectID,
		"rotated_at":     time.Now().UTC(),
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(payload))
	if err != nil {
		r.logger.Error("failed to create rotation webhook request", "url", logURL, "error", err)
		return fmt.Errorf("create rotation webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Strait-Event", "api_key.auto_rotated")

	signingSecret, err := r.rotationWebhookSigningSecret(webhook.EncryptedSecret, webhook.OldKeyID, webhook.ProjectID)
	if err != nil {
		return err
	}
	deliveryID := uuid.Must(uuid.NewV7()).String()
	timestamp := time.Now().UTC().Format(time.RFC3339)
	straitcrypto.SignWebhookRequest(req, signingSecret, payload, deliveryID, timestamp)

	client := r.rotationWebhookClient
	if client == nil {
		client = &http.Client{
			Timeout:   10 * time.Second,
			Transport: httputil.NewExternalTransport(r.allowPrivateEndpoints),
		}
	}
	requestClient := *client
	requestClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := requestClient.Do(req)
	if err != nil {
		safeErr := httputil.SanitizeHTTPClientError(err)
		r.logger.Warn("rotation webhook notification failed", "url", logURL, "error", safeErr)
		return fmt.Errorf("send rotation webhook: %s", safeErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		r.logger.Warn("rotation webhook returned non-success", "url", logURL, "status", resp.StatusCode)
		return fmt.Errorf("rotation webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (r *Reaper) rotationWebhookSigningSecret(encryptedSecret []byte, oldKeyID, projectID string) ([]byte, error) {
	switch {
	case len(encryptedSecret) == 0:
		return nil, fmt.Errorf("api key %s in project %s has no rotation webhook signing secret", oldKeyID, projectID)
	case r.rotationSecretDecryptor == nil:
		return nil, fmt.Errorf("rotation webhook signing secret decryptor is not configured for api key %s in project %s", oldKeyID, projectID)
	}
	plaintext, err := r.rotationSecretDecryptor.Decrypt(encryptedSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt rotation webhook signing secret for api key %s in project %s: %w", oldKeyID, projectID, err)
	}
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("rotation webhook signing secret is empty for api key %s in project %s", oldKeyID, projectID)
	}
	return canonicalRotationWebhookSecret(plaintext), nil
}

func canonicalRotationWebhookSecret(plaintext []byte) []byte {
	if bytes.HasPrefix(plaintext, []byte("whsec_")) {
		return plaintext
	}
	out := make([]byte, len("whsec_")+hex.EncodedLen(len(plaintext)))
	copy(out, "whsec_")
	hex.Encode(out[len("whsec_"):], plaintext)
	return out
}

func validateRotationWebhookURL(rawURL string, allowPrivateEndpoints bool) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse rotation webhook url: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("rotation webhook must use https")
	}
	if err := httputil.ValidateExternalURL(rawURL); err != nil && !allowPrivateEndpoints {
		return fmt.Errorf("ssrf guard rejected rotation webhook url: %w", err)
	}
	return nil
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
	r.pruneAlertCooldowns(now)
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
