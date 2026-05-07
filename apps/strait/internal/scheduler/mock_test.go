package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
)

var _ queue.Queue = (*mockQueue)(nil)

// mockPollerStore implements PollerStore for testing.
type mockPollerStore struct {
	activateDueRunsFn func(ctx context.Context, limit int) (int64, error)
}

func (m *mockPollerStore) ActivateDueRuns(ctx context.Context, limit int) (int64, error) {
	if m.activateDueRunsFn != nil {
		return m.activateDueRunsFn(ctx, limit)
	}
	return 0, nil
}

// mockReaperStore implements ReaperStore for testing.
type mockReaperStore struct {
	listStaleRunsFn                           func(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	listExpiredRunsFn                         func(ctx context.Context) ([]domain.JobRun, error)
	listStaleDequeuedFn                       func(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	listTimedOutWfRunsFn                      func(ctx context.Context) ([]domain.WorkflowRun, error)
	listStalledWorkflowRunsFn                 func(ctx context.Context, threshold time.Duration) ([]domain.WorkflowRun, error)
	listStepRunsByWfRunFn                     func(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	updateWorkflowRunStatusFn                 func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	updateStepRunStatusFn                     func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	getRunFn                                  func(ctx context.Context, id string) (*domain.JobRun, error)
	listExpiredApprovalsFn                    func(ctx context.Context) ([]domain.WorkflowStepApproval, error)
	getStepRunByRunAndRefFn                   func(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	updateWorkflowApprovalFn                  func(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	updateRunStatusFn                         func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	deleteOldWorkflowRunsFn                   func(ctx context.Context, before time.Time, limit int) (int64, error)
	deleteRetentionFn                         func(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error)
	listExpiredEventTriggersFn                func(ctx context.Context) ([]domain.EventTrigger, error)
	updateEventTriggerStatusFn                func(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	listReceivedEventTriggersWithStaleStepsFn func(ctx context.Context) ([]domain.EventTrigger, error)
	deleteEventTriggersFinishedBeforeFn       func(ctx context.Context, before time.Time, limit int) (int64, error)
	cancelNonTerminalStepRunsFn               func(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	cancelJobRunsByWorkflowRunFn              func(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
}

type mockCronStore struct {
	listCronJobsFn              func(ctx context.Context) ([]domain.Job, error)
	listCronWorkflowsFn         func(ctx context.Context) ([]domain.Workflow, error)
	countRunningWfRunsFn        func(ctx context.Context, workflowID string) (int, error)
	countActiveRunsForJobFn     func(ctx context.Context, jobID string) (int, error)
	cancelActiveRunsForJobFn    func(ctx context.Context, jobID string, reason string) ([]store.CanceledRun, error)
	cancelChildRunsByParentIDFn func(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error)
}

func (m *mockCronStore) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	if m.listCronJobsFn != nil {
		return m.listCronJobsFn(ctx)
	}
	return nil, nil
}

func (m *mockCronStore) ListCronWorkflows(ctx context.Context) ([]domain.Workflow, error) {
	if m.listCronWorkflowsFn != nil {
		return m.listCronWorkflowsFn(ctx)
	}
	return nil, nil
}

func (m *mockCronStore) CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error) {
	if m.countRunningWfRunsFn != nil {
		return m.countRunningWfRunsFn(ctx, workflowID)
	}
	return 0, nil
}

func (m *mockCronStore) CountActiveRunsForJob(ctx context.Context, jobID string) (int, error) {
	if m.countActiveRunsForJobFn != nil {
		return m.countActiveRunsForJobFn(ctx, jobID)
	}
	return 0, nil
}

func (m *mockCronStore) CancelActiveRunsForJob(ctx context.Context, jobID string, reason string) ([]store.CanceledRun, error) {
	if m.cancelActiveRunsForJobFn != nil {
		return m.cancelActiveRunsForJobFn(ctx, jobID, reason)
	}
	return nil, nil
}

func (m *mockCronStore) CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error) {
	if m.cancelChildRunsByParentIDFn != nil {
		return m.cancelChildRunsByParentIDFn(ctx, parentIDs, finishedAt, reason)
	}
	return 0, nil
}

type mockQueue struct {
	enqueueFn           func(ctx context.Context, run *domain.JobRun) error
	dequeueFn           func(ctx context.Context) (*domain.JobRun, error)
	dequeueNFn          func(ctx context.Context, n int) ([]domain.JobRun, error)
	dequeueNByProjectFn func(ctx context.Context, n int, projectID string) ([]domain.JobRun, error)
}

func (m *mockQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, run)
	}
	return nil
}

func (m *mockQueue) EnqueueInTx(ctx context.Context, _ store.DBTX, run *domain.JobRun) error {
	return m.Enqueue(ctx, run)
}

func (m *mockQueue) EnqueueBatch(_ context.Context, runs []*domain.JobRun) (int64, error) {
	return int64(len(runs)), nil
}

func (m *mockQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	if m.dequeueFn != nil {
		return m.dequeueFn(ctx)
	}
	return nil, nil
}

func (m *mockQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	if m.dequeueNFn != nil {
		return m.dequeueNFn(ctx, n)
	}
	return nil, nil
}

func (m *mockQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
	return m.DequeueN(ctx, n)
}

func (m *mockQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	if m.dequeueNByProjectFn != nil {
		return m.dequeueNByProjectFn(ctx, n, projectID)
	}
	return nil, nil
}

func (m *mockReaperStore) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	if m.listStaleRunsFn != nil {
		return m.listStaleRunsFn(ctx, threshold)
	}
	return nil, nil
}

func (m *mockReaperStore) ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error) {
	if m.listExpiredRunsFn != nil {
		return m.listExpiredRunsFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	if m.listStaleDequeuedFn != nil {
		return m.listStaleDequeuedFn(ctx, threshold)
	}
	return nil, nil
}

func (m *mockReaperStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockReaperStore) ListTimedOutWorkflowRuns(ctx context.Context) ([]domain.WorkflowRun, error) {
	if m.listTimedOutWfRunsFn != nil {
		return m.listTimedOutWfRunsFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) ListStalledWorkflowRuns(ctx context.Context, threshold time.Duration) ([]domain.WorkflowRun, error) {
	if m.listStalledWorkflowRunsFn != nil {
		return m.listStalledWorkflowRunsFn(ctx, threshold)
	}
	return nil, nil
}

func (m *mockReaperStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByWfRunFn != nil {
		return m.listStepRunsByWfRunFn(ctx, workflowRunID, limit, cursor)
	}
	return nil, nil
}

func (m *mockReaperStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	if m.updateWorkflowRunStatusFn != nil {
		return m.updateWorkflowRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockReaperStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, status, fields)
	}
	return nil
}

func (m *mockReaperStore) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	if m.getRunFn != nil {
		return m.getRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockReaperStore) ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error) {
	if m.listExpiredApprovalsFn != nil {
		return m.listExpiredApprovalsFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
	if m.getStepRunByRunAndRefFn != nil {
		return m.getStepRunByRunAndRefFn(ctx, workflowRunID, stepRef)
	}
	return nil, nil
}

func (m *mockReaperStore) UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
	if m.updateWorkflowApprovalFn != nil {
		return m.updateWorkflowApprovalFn(ctx, id, status, approvedBy, approvedAt, errMsg)
	}
	return nil
}

func (m *mockReaperStore) DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if m.deleteOldWorkflowRunsFn != nil {
		return m.deleteOldWorkflowRunsFn(ctx, before, limit)
	}
	return 0, nil
}

func (m *mockReaperStore) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	if m.deleteRetentionFn != nil {
		return m.deleteRetentionFn(ctx, shortRetention, longRetention)
	}
	return 0, nil
}

func (m *mockReaperStore) DeleteRunsByOrgOlderThan(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) DeleteWorkflowRunsByOrgOlderThan(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) ListExpiredEventTriggers(ctx context.Context) ([]domain.EventTrigger, error) {
	if m.listExpiredEventTriggersFn != nil {
		return m.listExpiredEventTriggersFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error {
	if m.updateEventTriggerStatusFn != nil {
		return m.updateEventTriggerStatusFn(ctx, id, status, responsePayload, receivedAt, errMsg)
	}
	return nil
}

func (m *mockReaperStore) CancelEventTriggersByWorkflowRun(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) ListReceivedEventTriggersWithStaleSteps(ctx context.Context) ([]domain.EventTrigger, error) {
	if m.listReceivedEventTriggersWithStaleStepsFn != nil {
		return m.listReceivedEventTriggersWithStaleStepsFn(ctx)
	}
	return nil, nil
}

func (m *mockReaperStore) DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if m.deleteEventTriggersFinishedBeforeFn != nil {
		return m.deleteEventTriggersFinishedBeforeFn(ctx, before, limit)
	}
	return 0, nil
}

func (m *mockReaperStore) CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	if m.cancelNonTerminalStepRunsFn != nil {
		return m.cancelNonTerminalStepRunsFn(ctx, workflowRunID, finishedAt, reason)
	}
	return 0, nil
}

func (m *mockReaperStore) CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	if m.cancelJobRunsByWorkflowRunFn != nil {
		return m.cancelJobRunsByWorkflowRunFn(ctx, workflowRunID, finishedAt, reason)
	}
	return 0, nil
}

// Audit reaper interface stubs for mockReaperStore.
func (m *mockReaperStore) DeleteAuditEventsBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockReaperStore) DeleteAuditEventsBeforeExcluding(_ context.Context, _ time.Time, _ []string) (int64, error) {
	return 0, nil
}
func (m *mockReaperStore) ListAuditRetentionOverrides(_ context.Context) ([]store.AuditRetentionOverride, error) {
	return nil, nil
}
func (m *mockReaperStore) ListAuditEventsDeadletter(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
	return nil, nil, nil
}
func (m *mockReaperStore) CreateAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	return nil
}
func (m *mockReaperStore) DeleteAuditEventDeadletter(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockReaperStore) ListAuditEventsDeadletterWithAttempts(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
	return nil, nil, nil, nil
}
func (m *mockReaperStore) IncrementAuditDeadletterAttempt(_ context.Context, _ string) error {
	return nil
}
func (m *mockReaperStore) MarkAuditDeadletterReclaimed(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockReaperStore) DeleteAuditDeadletterOlderThan(_ context.Context, _ time.Time) (map[string]int64, error) {
	return nil, nil
}

func (m *mockReaperStore) ArchiveTerminalRunsPastRetention(_ context.Context, _, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) DeleteHistoryRunsPastRetention(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) ArchiveConsumedOutboxBatch(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) DeleteOutboxHistoryPastRetention(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) PurgeQuarantinedOutboxOlderThan(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *mockReaperStore) GetRunFromHistory(_ context.Context, _ string) (*domain.JobRun, error) {
	return nil, nil
}

// mockWorkflowCallback implements WorkflowCallback for testing.
type mockWorkflowCallback struct {
	onJobRunTerminalFn func(ctx context.Context, run *domain.JobRun) error
	onEventReceivedFn  func(ctx context.Context, trigger *domain.EventTrigger) error
	onStepCompletedFn  func(ctx context.Context, workflowRunID string, stepRunID string)
	onStepFailedFn     func(ctx context.Context, workflowRunID string, stepRunID string)
	resumeWorkflowFn   func(ctx context.Context, workflowRunID string) error
	approveStepFn      func(ctx context.Context, workflowRunID, stepRef, approver string) error
}

func (m *mockWorkflowCallback) OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error {
	if m.onJobRunTerminalFn != nil {
		return m.onJobRunTerminalFn(ctx, run)
	}
	return nil
}

func (m *mockWorkflowCallback) OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error {
	if m.onEventReceivedFn != nil {
		return m.onEventReceivedFn(ctx, trigger)
	}
	return nil
}

func (m *mockWorkflowCallback) OnStepCompleted(ctx context.Context, workflowRunID string, stepRunID string) {
	if m.onStepCompletedFn != nil {
		m.onStepCompletedFn(ctx, workflowRunID, stepRunID)
	}
}

func (m *mockWorkflowCallback) OnStepFailed(ctx context.Context, workflowRunID string, stepRunID string) {
	if m.onStepFailedFn != nil {
		m.onStepFailedFn(ctx, workflowRunID, stepRunID)
	}
}

func (m *mockWorkflowCallback) ResumeWorkflowRun(ctx context.Context, workflowRunID string) error {
	if m.resumeWorkflowFn != nil {
		return m.resumeWorkflowFn(ctx, workflowRunID)
	}
	return nil
}

func (m *mockWorkflowCallback) ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error {
	if m.approveStepFn != nil {
		return m.approveStepFn(ctx, workflowRunID, stepRef, approver)
	}
	return nil
}
