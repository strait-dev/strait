package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"
)

// mockAPIStore implements APIStore for testing.
type mockAPIStore struct {
	createJobFn                   func(ctx context.Context, job *domain.Job) error
	createJobSecretFn             func(ctx context.Context, secret *domain.JobSecret) error
	getJobFn                      func(ctx context.Context, id string) (*domain.Job, error)
	getJobBySlugFn                func(ctx context.Context, projectID, slug string) (*domain.Job, error)
	listJobsFn                    func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Job, error)
	createJobGroupFn              func(ctx context.Context, group *domain.JobGroup) error
	getJobGroupFn                 func(ctx context.Context, id string) (*domain.JobGroup, error)
	listJobGroupsFn               func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobGroup, error)
	updateJobGroupFn              func(ctx context.Context, group *domain.JobGroup) error
	deleteJobFn                   func(ctx context.Context, id string) error
	deleteJobGroupFn              func(ctx context.Context, id string) error
	listJobsByGroupFn             func(ctx context.Context, groupID string, limit int, cursor *time.Time) ([]domain.Job, error)
	createEnvironmentFn           func(ctx context.Context, env *domain.Environment) error
	getEnvironmentFn              func(ctx context.Context, id string) (*domain.Environment, error)
	listEnvironmentsFn            func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error)
	updateEnvironmentFn           func(ctx context.Context, env *domain.Environment) error
	deleteEnvironmentFn           func(ctx context.Context, id string) error
	getResolvedEnvVarsFn          func(ctx context.Context, id string) (map[string]string, error)
	listJobSecretsFn              func(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error)
	listJobsByTagFn               func(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Job, error)
	createJobDependencyFn         func(ctx context.Context, dep *domain.JobDependency) error
	listJobDependenciesFn         func(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error)
	deleteJobDependencyFn         func(ctx context.Context, id string) error
	updateJobFn                   func(ctx context.Context, job *domain.Job) error
	getRunFn                      func(ctx context.Context, id string) (*domain.JobRun, error)
	getRunByIdempotencyKeyFn      func(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	findRecentRunByPayloadFn      func(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	countRunsForJobSinceFn        func(ctx context.Context, jobID string, since time.Time) (int, error)
	createRunCheckpointFn         func(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	listRunCheckpointsFn          func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error)
	createRunUsageFn              func(ctx context.Context, usage *domain.RunUsage) error
	listRunUsageFn                func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error)
	createRunToolCallFn           func(ctx context.Context, call *domain.RunToolCall) error
	listRunToolCallsFn            func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error)
	upsertRunOutputFn             func(ctx context.Context, output *domain.RunOutput) error
	listRunOutputsFn              func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error)
	areAllDescendantsTerminalFn   func(ctx context.Context, parentRunID string) (bool, error)
	getProjectQuotaFn             func(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	countProjectQueuedRunsFn      func(ctx context.Context, projectID string) (int, error)
	countProjectActiveRunsFn      func(ctx context.Context, projectID string) (int, error)
	listRunsByProjectFn           func(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue *string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	listDeadLetterRunsFn          func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	updateRunStatusFn             func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	replayDeadLetterRunFn         func(ctx context.Context, runID string) (*domain.JobRun, error)
	updateRunMetadataFn           func(ctx context.Context, id string, annotations map[string]string) error
	listChildRunsFn               func(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	insertEventFn                 func(ctx context.Context, event *domain.RunEvent) error
	listEventsByRunFilteredFn     func(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	listWebhookDeliveriesFn       func(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error)
	createAPIKeyFn                func(ctx context.Context, key *domain.APIKey) error
	listAPIKeysByProjectFn        func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error)
	revokeAPIKeyFn                func(ctx context.Context, id string) error
	listJobVersionsByJobFn        func(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobVersion, error)
	getAPIKeyByHashFn             func(ctx context.Context, keyHash string) (*domain.APIKey, error)
	getAPIKeyByIDFn               func(ctx context.Context, id string) (*domain.APIKey, error)
	markAPIKeyRotatedFn           func(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	touchAPIKeyLastUsedFn         func(ctx context.Context, id string) error
	updateHeartbeatFn             func(ctx context.Context, id string) error
	queueStatsFn                  func(ctx context.Context) (*store.QueueStats, error)
	createWorkflowFn              func(ctx context.Context, w *domain.Workflow) error
	getWorkflowFn                 func(ctx context.Context, id string) (*domain.Workflow, error)
	getWorkflowBySlugFn           func(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	listWorkflowsFn               func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Workflow, error)
	updateWorkflowFn              func(ctx context.Context, w *domain.Workflow) error
	createWorkflowSnapshotFn      func(ctx context.Context, workflowID string, version int) error
	deleteWorkflowFn              func(ctx context.Context, id string) error
	createWorkflowStepFn          func(ctx context.Context, step *domain.WorkflowStep) error
	listStepsByWorkflowFn         func(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	listStepsByWorkflowVerFn      func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	deleteStepsByWorkflowFn       func(ctx context.Context, workflowID string) error
	getWorkflowRunFn              func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listWorkflowRunsFn            func(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	listWorkflowRunsByProjFn      func(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	createWorkflowRunLabelsFn     func(ctx context.Context, workflowRunID string, labels map[string]string) error
	listWorkflowRunLabelsFn       func(ctx context.Context, workflowRunID string) (map[string]string, error)
	listStepRunsByRunFn           func(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	updateWorkflowRunStatusFn     func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	updateStepRunStatusFn         func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	getStepRunByRunAndRefFn       func(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	getStepApprovalByStepRunFn    func(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	updateStepApprovalFn          func(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	deleteJobSecretFn             func(ctx context.Context, id string) error
	batchUpdateJobsEnabledFn      func(ctx context.Context, ids []string, enabled bool) (int64, error)
	getJobHealthStatsFn           func(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	getDebugBundleFn              func(ctx context.Context, runID string) (*domain.DebugBundle, error)
	updateRunDebugModeFn          func(ctx context.Context, runID string, debugMode bool) error
	listEventsFn                  func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	createRunFn                   func(ctx context.Context, run *domain.JobRun) error
	listRunLineageFn              func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	sumRunCostMicrousdFn          func(ctx context.Context, runID string) (int64, error)
	sumProjectDailyCostMicrousdFn func(ctx context.Context, projectID string, timezone string) (int64, error)
	getUserPermissionsFn          func(ctx context.Context, projectID, userID string) ([]string, error)
	createProjectRoleFn           func(ctx context.Context, role *domain.ProjectRole) error
	getProjectRoleFn              func(ctx context.Context, id string) (*domain.ProjectRole, error)
	updateProjectRoleFn           func(ctx context.Context, role *domain.ProjectRole) error
	listProjectRolesFn            func(ctx context.Context, projectID string) ([]domain.ProjectRole, error)
	deleteProjectRoleFn           func(ctx context.Context, id string) error
	assignMemberRoleFn            func(ctx context.Context, m *domain.ProjectMemberRole) error
	listProjectMembersFn          func(ctx context.Context, projectID string) ([]domain.ProjectMemberRole, error)
	removeMemberRoleFn            func(ctx context.Context, projectID, userID string) error
}

func (m *mockAPIStore) CreateJob(ctx context.Context, job *domain.Job) error {
	if m.createJobFn != nil {
		return m.createJobFn(ctx, job)
	}
	return nil
}

func (m *mockAPIStore) CreateJobSecret(ctx context.Context, secret *domain.JobSecret) error {
	if m.createJobSecretFn != nil {
		return m.createJobSecretFn(ctx, secret)
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

func (m *mockAPIStore) ListJobs(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Job, error) {
	if m.listJobsFn != nil {
		return m.listJobsFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateJobGroup(ctx context.Context, group *domain.JobGroup) error {
	if m.createJobGroupFn != nil {
		return m.createJobGroupFn(ctx, group)
	}
	return nil
}

func (m *mockAPIStore) GetJobGroup(ctx context.Context, id string) (*domain.JobGroup, error) {
	if m.getJobGroupFn != nil {
		return m.getJobGroupFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) ListJobGroups(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobGroup, error) {
	if m.listJobGroupsFn != nil {
		return m.listJobGroupsFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateJobGroup(ctx context.Context, group *domain.JobGroup) error {
	if m.updateJobGroupFn != nil {
		return m.updateJobGroupFn(ctx, group)
	}
	return nil
}

func (m *mockAPIStore) DeleteJob(_ context.Context, id string) error {
	if m.deleteJobFn != nil {
		return m.deleteJobFn(context.Background(), id)
	}
	return nil
}

func (m *mockAPIStore) DeleteJobGroup(ctx context.Context, id string) error {
	if m.deleteJobGroupFn != nil {
		return m.deleteJobGroupFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) ListJobsByGroup(ctx context.Context, groupID string, limit int, cursor *time.Time) ([]domain.Job, error) {
	if m.listJobsByGroupFn != nil {
		return m.listJobsByGroupFn(ctx, groupID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateEnvironment(ctx context.Context, env *domain.Environment) error {
	if m.createEnvironmentFn != nil {
		return m.createEnvironmentFn(ctx, env)
	}
	return nil
}

func (m *mockAPIStore) GetEnvironment(ctx context.Context, id string) (*domain.Environment, error) {
	if m.getEnvironmentFn != nil {
		return m.getEnvironmentFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) ListEnvironments(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error) {
	if m.listEnvironmentsFn != nil {
		return m.listEnvironmentsFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateEnvironment(ctx context.Context, env *domain.Environment) error {
	if m.updateEnvironmentFn != nil {
		return m.updateEnvironmentFn(ctx, env)
	}
	return nil
}

func (m *mockAPIStore) DeleteEnvironment(ctx context.Context, id string) error {
	if m.deleteEnvironmentFn != nil {
		return m.deleteEnvironmentFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error) {
	if m.getResolvedEnvVarsFn != nil {
		return m.getResolvedEnvVarsFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) ListJobSecrets(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error) {
	if m.listJobSecretsFn != nil {
		return m.listJobSecretsFn(ctx, projectID, jobID, environment, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) ListJobsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Job, error) {
	if m.listJobsByTagFn != nil {
		return m.listJobsByTagFn(ctx, projectID, tagKey, tagValue, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateJobDependency(ctx context.Context, dep *domain.JobDependency) error {
	if m.createJobDependencyFn != nil {
		return m.createJobDependencyFn(ctx, dep)
	}
	return nil
}

func (m *mockAPIStore) ListJobDependencies(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error) {
	if m.listJobDependenciesFn != nil {
		return m.listJobDependenciesFn(ctx, jobID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) DeleteJobDependency(ctx context.Context, id string) error {
	if m.deleteJobDependencyFn != nil {
		return m.deleteJobDependencyFn(ctx, id)
	}
	return nil
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

func (m *mockAPIStore) ListRunCheckpoints(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error) {
	if m.listRunCheckpointsFn != nil {
		return m.listRunCheckpointsFn(ctx, runID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateRunUsage(ctx context.Context, usage *domain.RunUsage) error {
	if m.createRunUsageFn != nil {
		return m.createRunUsageFn(ctx, usage)
	}
	return nil
}

func (m *mockAPIStore) ListRunUsage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error) {
	if m.listRunUsageFn != nil {
		return m.listRunUsageFn(ctx, runID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateRunToolCall(ctx context.Context, call *domain.RunToolCall) error {
	if m.createRunToolCallFn != nil {
		return m.createRunToolCallFn(ctx, call)
	}
	return nil
}

func (m *mockAPIStore) ListRunToolCalls(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error) {
	if m.listRunToolCallsFn != nil {
		return m.listRunToolCallsFn(ctx, runID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error {
	if m.upsertRunOutputFn != nil {
		return m.upsertRunOutputFn(ctx, output)
	}
	return nil
}

func (m *mockAPIStore) ListRunOutputs(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error) {
	if m.listRunOutputsFn != nil {
		return m.listRunOutputsFn(ctx, runID, limit, cursor)
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

func (m *mockAPIStore) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	if m.listRunsByProjectFn != nil {
		return m.listRunsByProjectFn(ctx, projectID, status, metadataKey, metadataValue, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) ListRunsByTag(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
	return nil, nil
}

func (m *mockAPIStore) ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	if m.listDeadLetterRunsFn != nil {
		return m.listDeadLetterRunsFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockAPIStore) ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error) {
	if m.replayDeadLetterRunFn != nil {
		return m.replayDeadLetterRunFn(ctx, runID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error {
	if m.updateRunMetadataFn != nil {
		return m.updateRunMetadataFn(ctx, id, annotations)
	}
	return nil
}

func (m *mockAPIStore) ListChildRuns(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	if m.listChildRunsFn != nil {
		return m.listChildRunsFn(ctx, parentRunID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) InsertEvent(ctx context.Context, event *domain.RunEvent) error {
	if m.insertEventFn != nil {
		return m.insertEventFn(ctx, event)
	}
	return nil
}

func (m *mockAPIStore) ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error) {
	if m.listEventsByRunFilteredFn != nil {
		return m.listEventsByRunFilteredFn(ctx, runID, level, eventType, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error) {
	if m.listWebhookDeliveriesFn != nil {
		return m.listWebhookDeliveriesFn(ctx, projectID, status, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	if m.createAPIKeyFn != nil {
		return m.createAPIKeyFn(ctx, key)
	}
	return nil
}

func (m *mockAPIStore) ListAPIKeysByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error) {
	if m.listAPIKeysByProjectFn != nil {
		return m.listAPIKeysByProjectFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) RevokeAPIKey(ctx context.Context, id string) error {
	if m.revokeAPIKeyFn != nil {
		return m.revokeAPIKeyFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) ListJobVersionsByJob(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobVersion, error) {
	if m.listJobVersionsByJobFn != nil {
		return m.listJobVersionsByJobFn(ctx, jobID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) GetJobVersionByVersionID(_ context.Context, _ string) (*domain.JobVersion, error) {
	return nil, store.ErrJobNotFound
}

func (m *mockAPIStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	if m.getAPIKeyByHashFn != nil {
		return m.getAPIKeyByHashFn(ctx, keyHash)
	}
	return nil, fmt.Errorf("api key not found")
}

func (m *mockAPIStore) GetAPIKeyByID(ctx context.Context, id string) (*domain.APIKey, error) {
	if m.getAPIKeyByIDFn != nil {
		return m.getAPIKeyByIDFn(ctx, id)
	}
	return nil, fmt.Errorf("api key not found")
}

func (m *mockAPIStore) MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
	if m.markAPIKeyRotatedFn != nil {
		return m.markAPIKeyRotatedFn(ctx, oldKeyID, newKeyID, graceExpiresAt)
	}
	return nil
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

func (m *mockAPIStore) ListWorkflows(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Workflow, error) {
	if m.listWorkflowsFn != nil {
		return m.listWorkflowsFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWorkflowsByTag(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.Workflow, error) {
	return nil, nil
}

func (m *mockAPIStore) UpdateWorkflow(ctx context.Context, w *domain.Workflow) error {
	if m.updateWorkflowFn != nil {
		return m.updateWorkflowFn(ctx, w)
	}
	return nil
}

func (m *mockAPIStore) CreateWorkflowVersionSnapshot(ctx context.Context, workflowID string, version int) error {
	if m.createWorkflowSnapshotFn != nil {
		return m.createWorkflowSnapshotFn(ctx, workflowID, version)
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

func (m *mockAPIStore) ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	if m.listStepsByWorkflowVerFn != nil {
		return m.listStepsByWorkflowVerFn(ctx, workflowID, version)
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

func (m *mockAPIStore) ListWorkflowRuns(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	if m.listWorkflowRunsFn != nil {
		return m.listWorkflowRunsFn(ctx, workflowID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	if m.listWorkflowRunsByProjFn != nil {
		return m.listWorkflowRunsByProjFn(ctx, projectID, status, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWorkflowRunsByTag(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.WorkflowRun, error) {
	return nil, nil
}

func (m *mockAPIStore) CreateWorkflowRunLabels(ctx context.Context, workflowRunID string, labels map[string]string) error {
	if m.createWorkflowRunLabelsFn != nil {
		return m.createWorkflowRunLabelsFn(ctx, workflowRunID, labels)
	}
	return nil
}

func (m *mockAPIStore) ListWorkflowRunLabels(ctx context.Context, workflowRunID string) (map[string]string, error) {
	if m.listWorkflowRunLabelsFn != nil {
		return m.listWorkflowRunLabelsFn(ctx, workflowRunID)
	}
	return map[string]string{}, nil
}

func (m *mockAPIStore) ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
	if m.listStepRunsByRunFn != nil {
		return m.listStepRunsByRunFn(ctx, workflowRunID, limit, cursor)
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

func (m *mockAPIStore) GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
	if m.getStepRunByRunAndRefFn != nil {
		return m.getStepRunByRunAndRefFn(ctx, workflowRunID, stepRef)
	}
	return nil, nil
}

func (m *mockAPIStore) GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error) {
	if m.getStepApprovalByStepRunFn != nil {
		return m.getStepApprovalByStepRunFn(ctx, stepRunID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error {
	if m.updateStepApprovalFn != nil {
		return m.updateStepApprovalFn(ctx, id, status, approvedBy, approvedAt, errMsg)
	}
	return nil
}

func (m *mockAPIStore) ListWorkflowVersions(ctx context.Context, workflowID string, limit int) ([]domain.WorkflowVersion, error) {
	return nil, nil
}

func (m *mockAPIStore) GetWorkflowVersionByVersionID(ctx context.Context, workflowID, versionID string) (*domain.WorkflowVersion, error) {
	return nil, store.ErrWorkflowVersionNotFound
}

func (m *mockAPIStore) DeleteJobSecret(ctx context.Context, id string) error {
	if m.deleteJobSecretFn != nil {
		return m.deleteJobSecretFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) BatchUpdateJobsEnabled(ctx context.Context, ids []string, enabled bool) (int64, error) {
	if m.batchUpdateJobsEnabledFn != nil {
		return m.batchUpdateJobsEnabledFn(ctx, ids, enabled)
	}
	return 0, nil
}

func (m *mockAPIStore) GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error) {
	if m.getJobHealthStatsFn != nil {
		return m.getJobHealthStatsFn(ctx, jobID, since)
	}
	return &store.JobHealthStats{}, nil
}

func (m *mockAPIStore) GetDebugBundle(ctx context.Context, runID string) (*domain.DebugBundle, error) {
	if m.getDebugBundleFn != nil {
		return m.getDebugBundleFn(ctx, runID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateRunDebugMode(ctx context.Context, runID string, debugMode bool) error {
	if m.updateRunDebugModeFn != nil {
		return m.updateRunDebugModeFn(ctx, runID, debugMode)
	}
	return nil
}

func (m *mockAPIStore) ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error) {
	if m.listEventsFn != nil {
		return m.listEventsFn(ctx, runID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateRun(ctx context.Context, run *domain.JobRun) error {
	if m.createRunFn != nil {
		return m.createRunFn(ctx, run)
	}
	return nil
}

func (m *mockAPIStore) ListRunLineage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	if m.listRunLineageFn != nil {
		return m.listRunLineageFn(ctx, runID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) SumRunCostMicrousd(ctx context.Context, runID string) (int64, error) {
	if m.sumRunCostMicrousdFn != nil {
		return m.sumRunCostMicrousdFn(ctx, runID)
	}
	return 0, nil
}

func (m *mockAPIStore) SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error) {
	if m.sumProjectDailyCostMicrousdFn != nil {
		return m.sumProjectDailyCostMicrousdFn(ctx, projectID, timezone)
	}
	return 0, nil
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

// RBAC mock methods.
func (m *mockAPIStore) GetUserPermissions(ctx context.Context, projectID, userID string) ([]string, error) {
	if m.getUserPermissionsFn != nil {
		return m.getUserPermissionsFn(ctx, projectID, userID)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateProjectRole(ctx context.Context, role *domain.ProjectRole) error {
	if m.createProjectRoleFn != nil {
		return m.createProjectRoleFn(ctx, role)
	}
	return nil
}

func (m *mockAPIStore) GetProjectRole(ctx context.Context, id string) (*domain.ProjectRole, error) {
	if m.getProjectRoleFn != nil {
		return m.getProjectRoleFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateProjectRole(ctx context.Context, role *domain.ProjectRole) error {
	if m.updateProjectRoleFn != nil {
		return m.updateProjectRoleFn(ctx, role)
	}
	return nil
}

func (m *mockAPIStore) ListProjectRoles(ctx context.Context, projectID string, _ int, _ *time.Time) ([]domain.ProjectRole, error) {
	if m.listProjectRolesFn != nil {
		return m.listProjectRolesFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) DeleteProjectRole(ctx context.Context, id string) error {
	if m.deleteProjectRoleFn != nil {
		return m.deleteProjectRoleFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) AssignMemberRole(ctx context.Context, m2 *domain.ProjectMemberRole) error {
	if m.assignMemberRoleFn != nil {
		return m.assignMemberRoleFn(ctx, m2)
	}
	return nil
}

func (m *mockAPIStore) GetMemberRole(_ context.Context, _, _ string) (*domain.ProjectMemberRole, error) {
	return nil, nil
}

func (m *mockAPIStore) RemoveMemberRole(ctx context.Context, projectID, userID string) error {
	if m.removeMemberRoleFn != nil {
		return m.removeMemberRoleFn(ctx, projectID, userID)
	}
	return nil
}

func (m *mockAPIStore) ListProjectMembers(ctx context.Context, projectID string, _ int, _ *time.Time) ([]domain.ProjectMemberRole, error) {
	if m.listProjectMembersFn != nil {
		return m.listProjectMembersFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) SeedProjectSystemRoles(_ context.Context, _ string) error {
	return nil
}

func (m *mockAPIStore) CreateResourcePolicy(_ context.Context, _ *domain.ResourcePolicy) error {
	return nil
}

func (m *mockAPIStore) GetResourcePolicies(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockAPIStore) DeleteResourcePolicy(_ context.Context, _ string) (string, string, error) {
	return "", "", nil
}

func (m *mockAPIStore) ListResourcePolicies(_ context.Context, _, _ string, _ int, _ *time.Time) ([]domain.ResourcePolicy, error) {
	return nil, nil
}

func (m *mockAPIStore) CreateAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	return nil
}

func (m *mockAPIStore) ListAuditEvents(_ context.Context, _, _, _, _ string, _ int, _ *time.Time) ([]domain.AuditEvent, error) {
	return nil, nil
}
