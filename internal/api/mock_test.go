package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/store"
)

// mockAPIStore implements APIStore for testing.
type mockAPIStore struct {
	createJobFn                 func(ctx context.Context, job *domain.Job) error
	getJobFn                    func(ctx context.Context, id string) (*domain.Job, error)
	getJobBySlugFn              func(ctx context.Context, projectID, slug string) (*domain.Job, error)
	listJobsFn                  func(ctx context.Context, projectID string) ([]domain.Job, error)
	listJobsByTagFn             func(ctx context.Context, projectID, tagKey, tagValue string) ([]domain.Job, error)
	updateJobFn                 func(ctx context.Context, job *domain.Job) error
	getRunFn                    func(ctx context.Context, id string) (*domain.JobRun, error)
	getRunByIdempotencyKeyFn    func(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	findRecentRunByPayloadFn    func(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	countRunsForJobSinceFn      func(ctx context.Context, jobID string, since time.Time) (int, error)
	createRunCheckpointFn       func(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	listRunCheckpointsFn        func(ctx context.Context, runID string, limit int) ([]domain.RunCheckpoint, error)
	createRunUsageFn            func(ctx context.Context, usage *domain.RunUsage) error
	listRunUsageFn              func(ctx context.Context, runID string, limit int) ([]domain.RunUsage, error)
	createRunToolCallFn         func(ctx context.Context, call *domain.RunToolCall) error
	listRunToolCallsFn          func(ctx context.Context, runID string, limit int) ([]domain.RunToolCall, error)
	upsertRunOutputFn           func(ctx context.Context, output *domain.RunOutput) error
	listRunOutputsFn            func(ctx context.Context, runID string) ([]domain.RunOutput, error)
	areAllDescendantsTerminalFn func(ctx context.Context, parentRunID string) (bool, error)
	getProjectQuotaFn           func(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	countProjectQueuedRunsFn    func(ctx context.Context, projectID string) (int, error)
	countProjectActiveRunsFn    func(ctx context.Context, projectID string) (int, error)
	listRunsByProjectFn         func(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error)
	updateRunStatusFn           func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	listChildRunsFn             func(ctx context.Context, parentRunID string) ([]domain.JobRun, error)
	insertEventFn               func(ctx context.Context, event *domain.RunEvent) error
	listEventsByRunFilteredFn   func(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error)
	listWebhookDeliveriesFn     func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error)
	createAPIKeyFn              func(ctx context.Context, key *domain.APIKey) error
	listAPIKeysByProjectFn      func(ctx context.Context, projectID string) ([]domain.APIKey, error)
	revokeAPIKeyFn              func(ctx context.Context, id string) error
	listJobVersionsByJobFn      func(ctx context.Context, jobID string) ([]domain.JobVersion, error)
	getAPIKeyByHashFn           func(ctx context.Context, keyHash string) (*domain.APIKey, error)
	touchAPIKeyLastUsedFn       func(ctx context.Context, id string) error
	updateHeartbeatFn           func(ctx context.Context, id string) error
	queueStatsFn                func(ctx context.Context) (*store.QueueStats, error)
	createWorkflowFn            func(ctx context.Context, w *domain.Workflow) error
	getWorkflowFn               func(ctx context.Context, id string) (*domain.Workflow, error)
	getWorkflowBySlugFn         func(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	listWorkflowsFn             func(ctx context.Context, projectID string) ([]domain.Workflow, error)
	updateWorkflowFn            func(ctx context.Context, w *domain.Workflow) error
	deleteWorkflowFn            func(ctx context.Context, id string) error
	createWorkflowStepFn        func(ctx context.Context, step *domain.WorkflowStep) error
	listStepsByWorkflowFn       func(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	deleteStepsByWorkflowFn     func(ctx context.Context, workflowID string) error
	getWorkflowRunFn            func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listWorkflowRunsFn          func(ctx context.Context, workflowID string, limit, offset int) ([]domain.WorkflowRun, error)
	listWorkflowRunsByProjFn    func(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int) ([]domain.WorkflowRun, error)
	listStepRunsByRunFn         func(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	updateWorkflowRunStatusFn   func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	updateStepRunStatusFn       func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
}

func (m *mockAPIStore) CreateJob(ctx context.Context, job *domain.Job) error {
	if m.createJobFn != nil {
		return m.createJobFn(ctx, job)
	}
	return nil
}

func (m *mockAPIStore) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	if m.getJobFn != nil {
		return m.getJobFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error) {
	if m.getJobBySlugFn != nil {
		return m.getJobBySlugFn(ctx, projectID, slug)
	}
	return nil, nil
}

func (m *mockAPIStore) ListJobs(ctx context.Context, projectID string) ([]domain.Job, error) {
	if m.listJobsFn != nil {
		return m.listJobsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) ListJobsByTag(ctx context.Context, projectID, tagKey, tagValue string) ([]domain.Job, error) {
	if m.listJobsByTagFn != nil {
		return m.listJobsByTagFn(ctx, projectID, tagKey, tagValue)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateJob(ctx context.Context, job *domain.Job) error {
	if m.updateJobFn != nil {
		return m.updateJobFn(ctx, job)
	}
	return nil
}

func (m *mockAPIStore) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	if m.getRunFn != nil {
		return m.getRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error) {
	if m.getRunByIdempotencyKeyFn != nil {
		return m.getRunByIdempotencyKeyFn(ctx, jobID, idempotencyKey)
	}
	return nil, nil
}

func (m *mockAPIStore) FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error) {
	if m.findRecentRunByPayloadFn != nil {
		return m.findRecentRunByPayloadFn(ctx, jobID, payload, since)
	}
	return nil, nil
}

func (m *mockAPIStore) CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error) {
	if m.countRunsForJobSinceFn != nil {
		return m.countRunsForJobSinceFn(ctx, jobID, since)
	}
	return 0, nil
}

func (m *mockAPIStore) CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error {
	if m.createRunCheckpointFn != nil {
		return m.createRunCheckpointFn(ctx, checkpoint)
	}
	return nil
}

func (m *mockAPIStore) ListRunCheckpoints(ctx context.Context, runID string, limit int) ([]domain.RunCheckpoint, error) {
	if m.listRunCheckpointsFn != nil {
		return m.listRunCheckpointsFn(ctx, runID, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateRunUsage(ctx context.Context, usage *domain.RunUsage) error {
	if m.createRunUsageFn != nil {
		return m.createRunUsageFn(ctx, usage)
	}
	return nil
}

func (m *mockAPIStore) ListRunUsage(ctx context.Context, runID string, limit int) ([]domain.RunUsage, error) {
	if m.listRunUsageFn != nil {
		return m.listRunUsageFn(ctx, runID, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateRunToolCall(ctx context.Context, call *domain.RunToolCall) error {
	if m.createRunToolCallFn != nil {
		return m.createRunToolCallFn(ctx, call)
	}
	return nil
}

func (m *mockAPIStore) ListRunToolCalls(ctx context.Context, runID string, limit int) ([]domain.RunToolCall, error) {
	if m.listRunToolCallsFn != nil {
		return m.listRunToolCallsFn(ctx, runID, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error {
	if m.upsertRunOutputFn != nil {
		return m.upsertRunOutputFn(ctx, output)
	}
	return nil
}

func (m *mockAPIStore) ListRunOutputs(ctx context.Context, runID string) ([]domain.RunOutput, error) {
	if m.listRunOutputsFn != nil {
		return m.listRunOutputsFn(ctx, runID)
	}
	return nil, nil
}

func (m *mockAPIStore) AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error) {
	if m.areAllDescendantsTerminalFn != nil {
		return m.areAllDescendantsTerminalFn(ctx, parentRunID)
	}
	return true, nil
}

func (m *mockAPIStore) GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error) {
	if m.getProjectQuotaFn != nil {
		return m.getProjectQuotaFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error) {
	if m.countProjectQueuedRunsFn != nil {
		return m.countProjectQueuedRunsFn(ctx, projectID)
	}
	return 0, nil
}

func (m *mockAPIStore) CountProjectActiveRuns(ctx context.Context, projectID string) (int, error) {
	if m.countProjectActiveRunsFn != nil {
		return m.countProjectActiveRunsFn(ctx, projectID)
	}
	return 0, nil
}

func (m *mockAPIStore) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	if m.listRunsByProjectFn != nil {
		return m.listRunsByProjectFn(ctx, projectID, status, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockAPIStore) ListChildRuns(ctx context.Context, parentRunID string) ([]domain.JobRun, error) {
	if m.listChildRunsFn != nil {
		return m.listChildRunsFn(ctx, parentRunID)
	}
	return nil, nil
}

func (m *mockAPIStore) InsertEvent(ctx context.Context, event *domain.RunEvent) error {
	if m.insertEventFn != nil {
		return m.insertEventFn(ctx, event)
	}
	return nil
}

func (m *mockAPIStore) ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error) {
	if m.listEventsByRunFilteredFn != nil {
		return m.listEventsByRunFilteredFn(ctx, runID, level, eventType)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWebhookDeliveries(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
	if m.listWebhookDeliveriesFn != nil {
		return m.listWebhookDeliveriesFn(ctx, status, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	if m.createAPIKeyFn != nil {
		return m.createAPIKeyFn(ctx, key)
	}
	return nil
}

func (m *mockAPIStore) ListAPIKeysByProject(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	if m.listAPIKeysByProjectFn != nil {
		return m.listAPIKeysByProjectFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) RevokeAPIKey(ctx context.Context, id string) error {
	if m.revokeAPIKeyFn != nil {
		return m.revokeAPIKeyFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) ListJobVersionsByJob(ctx context.Context, jobID string) ([]domain.JobVersion, error) {
	if m.listJobVersionsByJobFn != nil {
		return m.listJobVersionsByJobFn(ctx, jobID)
	}
	return nil, nil
}

func (m *mockAPIStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	if m.getAPIKeyByHashFn != nil {
		return m.getAPIKeyByHashFn(ctx, keyHash)
	}
	return nil, fmt.Errorf("api key not found")
}

func (m *mockAPIStore) TouchAPIKeyLastUsed(ctx context.Context, id string) error {
	if m.touchAPIKeyLastUsedFn != nil {
		return m.touchAPIKeyLastUsedFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) UpdateHeartbeat(ctx context.Context, id string) error {
	if m.updateHeartbeatFn != nil {
		return m.updateHeartbeatFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) QueueStats(ctx context.Context) (*store.QueueStats, error) {
	if m.queueStatsFn != nil {
		return m.queueStatsFn(ctx)
	}
	return &store.QueueStats{}, nil
}

func (m *mockAPIStore) CreateWorkflow(ctx context.Context, w *domain.Workflow) error {
	if m.createWorkflowFn != nil {
		return m.createWorkflowFn(ctx, w)
	}
	return nil
}

func (m *mockAPIStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	if m.getWorkflowFn != nil {
		return m.getWorkflowFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error) {
	if m.getWorkflowBySlugFn != nil {
		return m.getWorkflowBySlugFn(ctx, projectID, slug)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWorkflows(ctx context.Context, projectID string) ([]domain.Workflow, error) {
	if m.listWorkflowsFn != nil {
		return m.listWorkflowsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateWorkflow(ctx context.Context, w *domain.Workflow) error {
	if m.updateWorkflowFn != nil {
		return m.updateWorkflowFn(ctx, w)
	}
	return nil
}

func (m *mockAPIStore) DeleteWorkflow(ctx context.Context, id string) error {
	if m.deleteWorkflowFn != nil {
		return m.deleteWorkflowFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error {
	if m.createWorkflowStepFn != nil {
		return m.createWorkflowStepFn(ctx, step)
	}
	return nil
}

func (m *mockAPIStore) ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error) {
	if m.listStepsByWorkflowFn != nil {
		return m.listStepsByWorkflowFn(ctx, workflowID)
	}
	return nil, nil
}

func (m *mockAPIStore) DeleteStepsByWorkflow(ctx context.Context, workflowID string) error {
	if m.deleteStepsByWorkflowFn != nil {
		return m.deleteStepsByWorkflowFn(ctx, workflowID)
	}
	return nil
}

func (m *mockAPIStore) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getWorkflowRunFn != nil {
		return m.getWorkflowRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWorkflowRuns(ctx context.Context, workflowID string, limit, offset int) ([]domain.WorkflowRun, error) {
	if m.listWorkflowRunsFn != nil {
		return m.listWorkflowRunsFn(ctx, workflowID, limit, offset)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int) ([]domain.WorkflowRun, error) {
	if m.listWorkflowRunsByProjFn != nil {
		return m.listWorkflowRunsByProjFn(ctx, projectID, status, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByRunFn != nil {
		return m.listStepRunsByRunFn(ctx, workflowRunID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	if m.updateWorkflowRunStatusFn != nil {
		return m.updateWorkflowRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockAPIStore) UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if m.updateStepRunStatusFn != nil {
		return m.updateStepRunStatusFn(ctx, id, status, fields)
	}
	return nil
}

// mockQueue implements queue.Queue for testing.
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

func (m *mockQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	if m.dequeueNByProjectFn != nil {
		return m.dequeueNByProjectFn(ctx, n, projectID)
	}
	return nil, nil
}

// mockPublisher implements pubsub.Publisher for testing.
type mockPublisher struct {
	publishFn   func(ctx context.Context, channel string, data []byte) error
	subscribeFn func(ctx context.Context, channel string) (*pubsub.Subscription, error)
	closeFn     func() error
}

func (m *mockPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	if m.publishFn != nil {
		return m.publishFn(ctx, channel, data)
	}
	return nil
}

func (m *mockPublisher) Subscribe(ctx context.Context, channel string) (*pubsub.Subscription, error) {
	if m.subscribeFn != nil {
		return m.subscribeFn(ctx, channel)
	}
	return nil, nil
}

func (m *mockPublisher) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error {
	return m.err
}
