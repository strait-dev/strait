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
	createJobFn                          func(ctx context.Context, job *domain.Job) error
	createJobSecretFn                    func(ctx context.Context, secret *domain.JobSecret) error
	getJobFn                             func(ctx context.Context, id string) (*domain.Job, error)
	getJobBySlugFn                       func(ctx context.Context, projectID, slug string) (*domain.Job, error)
	listJobsFn                           func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Job, error)
	createJobGroupFn                     func(ctx context.Context, group *domain.JobGroup) error
	getJobGroupFn                        func(ctx context.Context, id string) (*domain.JobGroup, error)
	listJobGroupsFn                      func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobGroup, error)
	updateJobGroupFn                     func(ctx context.Context, group *domain.JobGroup) error
	deleteJobFn                          func(ctx context.Context, id string) error
	deleteJobGroupFn                     func(ctx context.Context, id string) error
	listJobsByGroupFn                    func(ctx context.Context, groupID string, limit int, cursor *time.Time) ([]domain.Job, error)
	pauseJobsByGroupFn                   func(ctx context.Context, groupID string) error
	resumeJobsByGroupFn                  func(ctx context.Context, groupID string) error
	getJobGroupStatsFn                   func(ctx context.Context, groupID string) (*store.JobGroupStats, error)
	createEnvironmentFn                  func(ctx context.Context, env *domain.Environment) error
	getEnvironmentFn                     func(ctx context.Context, id string) (*domain.Environment, error)
	listEnvironmentsFn                   func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error)
	updateEnvironmentFn                  func(ctx context.Context, env *domain.Environment) error
	deleteEnvironmentFn                  func(ctx context.Context, id string) error
	getResolvedEnvVarsFn                 func(ctx context.Context, id string) (map[string]string, error)
	listJobSecretsFn                     func(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error)
	listJobsByTagFn                      func(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Job, error)
	createJobDependencyFn                func(ctx context.Context, dep *domain.JobDependency) error
	listJobDependenciesFn                func(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error)
	deleteJobDependencyFn                func(ctx context.Context, id string) error
	areJobDependenciesSatisfiedFn        func(ctx context.Context, run *domain.JobRun) (bool, error)
	updateJobFn                          func(ctx context.Context, job *domain.Job) error
	getRunFn                             func(ctx context.Context, id string) (*domain.JobRun, error)
	getRunByIdempotencyKeyFn             func(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	findRecentRunByPayloadFn             func(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	countRunsForJobSinceFn               func(ctx context.Context, jobID string, since time.Time) (int, error)
	createRunCheckpointFn                func(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	listRunCheckpointsFn                 func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error)
	createRunUsageFn                     func(ctx context.Context, usage *domain.RunUsage) error
	listRunUsageFn                       func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error)
	createRunToolCallFn                  func(ctx context.Context, call *domain.RunToolCall) error
	listRunToolCallsFn                   func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error)
	upsertRunOutputFn                    func(ctx context.Context, output *domain.RunOutput) error
	listRunOutputsFn                     func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error)
	areAllDescendantsTerminalFn          func(ctx context.Context, parentRunID string) (bool, error)
	getProjectQuotaFn                    func(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	countProjectQueuedRunsFn             func(ctx context.Context, projectID string) (int, error)
	countProjectActiveRunsFn             func(ctx context.Context, projectID string) (int, error)
	listRunsByProjectFn                  func(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, executionMode *domain.ExecutionMode, errorClass *string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	listDeadLetterRunsFn                 func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	bulkReplayDeadLetterRunsFn           func(ctx context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error)
	updateRunStatusFn                    func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	replayDeadLetterRunFn                func(ctx context.Context, runID string) (*domain.JobRun, error)
	updateRunMetadataFn                  func(ctx context.Context, id string, annotations map[string]string) error
	resetRunIdempotencyKeyFn             func(ctx context.Context, runID string) error
	listChildRunsFn                      func(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	insertEventFn                        func(ctx context.Context, event *domain.RunEvent) error
	createRunResourceSnapshotFn          func(ctx context.Context, s *domain.RunResourceSnapshot) error
	listRunResourceSnapshotsFn           func(ctx context.Context, runID string, from, to *time.Time, limit int) ([]domain.RunResourceSnapshot, error)
	getApprovalStatsFn                   func(ctx context.Context, projectID string, from, to time.Time) (*store.ApprovalStats, error)
	getCostOutliersFn                    func(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]store.CostOutlier, error)
	streamAuditEventsFn                  func(ctx context.Context, projectID, actorID, resourceType string, from, to time.Time, fn func(*domain.AuditEvent) error) error
	listEventsByRunFilteredFn            func(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	listWebhookDeliveriesFn              func(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error)
	createWebhookSubscriptionFn          func(ctx context.Context, sub *domain.WebhookSubscription) error
	listWebhookSubscriptionsFn           func(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	deleteWebhookSubscriptionFn          func(ctx context.Context, id string) error
	createAPIKeyFn                       func(ctx context.Context, key *domain.APIKey) error
	listAPIKeysByProjectFn               func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error)
	revokeAPIKeyFn                       func(ctx context.Context, id string) error
	listJobVersionsByJobFn               func(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobVersion, error)
	getAPIKeyByHashFn                    func(ctx context.Context, keyHash string) (*domain.APIKey, error)
	getAPIKeyByIDFn                      func(ctx context.Context, id string) (*domain.APIKey, error)
	markAPIKeyRotatedFn                  func(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	touchAPIKeyLastUsedFn                func(ctx context.Context, id string) error
	updateHeartbeatFn                    func(ctx context.Context, id string) error
	batchUpdateHeartbeatFn               func(ctx context.Context, ids []string) error
	queueStatsFn                         func(ctx context.Context) (*store.QueueStats, error)
	getPerformanceAnalyticsFn            func(ctx context.Context, projectID string, periodHours int) (*store.PerformanceAnalytics, error)
	getCostAnalyticsFn                   func(ctx context.Context, projectID string, from, to time.Time) (*store.CostAnalytics, error)
	getCostTrendsFn                      func(ctx context.Context, projectID string, from, to time.Time) ([]store.CostTrendPoint, error)
	getTopCostsFn                        func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopCostItem, error)
	getComputeCostAnalyticsFn            func(ctx context.Context, projectID string, from, to time.Time) (*store.ComputeCostAnalytics, error)
	aggregateCostStatsHourlyFn           func(ctx context.Context, hour time.Time) error
	getRunTimelineFn                     func(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.RunTimelineBucket, error)
	getRunDurationDistributionFn         func(ctx context.Context, projectID string, from, to time.Time) ([]store.RunDurationBucket, error)
	getRunFailureReasonsFn               func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.RunFailureReason, error)
	getRunSummaryFn                      func(ctx context.Context, projectID string, from, to time.Time) (*store.RunSummary, error)
	getRunsByTriggerFn                   func(ctx context.Context, projectID string, from, to time.Time) ([]store.RunsByTrigger, error)
	getJobHistoryFn                      func(ctx context.Context, projectID, jobID string, from, to time.Time, bucket string) ([]store.JobHistoryBucket, error)
	getJobComparisonFn                   func(ctx context.Context, projectID string, jobIDs []string, from, to time.Time) ([]store.JobComparison, error)
	getJobReliabilityFn                  func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobReliability, error)
	getRunsByVersionFn                   func(ctx context.Context, projectID, jobID string, from, to time.Time) ([]store.RunsByVersion, error)
	getJobCostRankingFn                  func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobCostRanking, error)
	getTopFailingJobsFn                  func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingJob, error)
	getTagSummaryFn                      func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagSummary, error)
	getTopFailingTagsFn                  func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingTag, error)
	getTagCostFn                         func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagCost, error)
	getWorkflowStepDurationsFn           func(ctx context.Context, projectID, workflowID string, from, to time.Time) ([]store.StepDuration, error)
	getWorkflowCompletionRatesFn         func(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WorkflowCompletionBucket, error)
	getWorkflowSummaryFn                 func(ctx context.Context, projectID string, from, to time.Time) (*store.WorkflowSummary, error)
	getWebhookDeliveryStatsFn            func(ctx context.Context, projectID string, from, to time.Time) ([]store.WebhookEndpointStats, error)
	getWebhookEndpointHealthFn           func(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WebhookHealthBucket, error)
	getTopFailingWebhooksFn              func(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingEndpoint, error)
	getEventVolumeFn                     func(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.EventVolumeBucket, error)
	getEventLatencyFn                    func(ctx context.Context, projectID string, from, to time.Time) (*store.EventLatencyStats, error)
	getCostForecastFn                    func(ctx context.Context, projectID string, from, to time.Time) (*store.CostForecast, error)
	getCostByTriggerFn                   func(ctx context.Context, projectID string, from, to time.Time) ([]store.CostByTrigger, error)
	getCostByMachineFn                   func(ctx context.Context, projectID string, from, to time.Time) ([]store.CostByMachine, error)
	createWorkflowFn                     func(ctx context.Context, w *domain.Workflow) error
	getWorkflowFn                        func(ctx context.Context, id string) (*domain.Workflow, error)
	getWorkflowBySlugFn                  func(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	listWorkflowsFn                      func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Workflow, error)
	updateWorkflowFn                     func(ctx context.Context, w *domain.Workflow) error
	createWorkflowSnapshotFn             func(ctx context.Context, workflowID string, version int) error
	deleteWorkflowFn                     func(ctx context.Context, id string) error
	createWorkflowStepFn                 func(ctx context.Context, step *domain.WorkflowStep) error
	listStepsByWorkflowFn                func(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	listStepsByWorkflowVerFn             func(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	deleteStepsByWorkflowFn              func(ctx context.Context, workflowID string) error
	getWorkflowRunFn                     func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listWorkflowRunsFn                   func(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	listWorkflowRunsByProjFn             func(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	listWorkflowsByTagFn                 func(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Workflow, error)
	listWorkflowRunsByTagFn              func(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	createWorkflowRunLabelsFn            func(ctx context.Context, workflowRunID string, labels map[string]string) error
	listWorkflowRunLabelsFn              func(ctx context.Context, workflowRunID string) (map[string]string, error)
	listStepRunsByRunFn                  func(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	listWorkflowStepDecisionsFn          func(ctx context.Context, workflowRunID, stepRef, decisionType string, limit int, cursor *time.Time) ([]domain.WorkflowStepDecision, error)
	upsertWorkflowPolicyFn               func(ctx context.Context, p *domain.WorkflowPolicy) error
	getWorkflowPolicyByProjectFn         func(ctx context.Context, projectID string) (*domain.WorkflowPolicy, error)
	listWorkflowVersionsFn               func(ctx context.Context, workflowID string, limit int) ([]domain.WorkflowVersion, error)
	getWorkflowVersionByVersionIDFn      func(ctx context.Context, workflowID, versionID string) (*domain.WorkflowVersion, error)
	createDeploymentVersionFn            func(ctx context.Context, deployment *domain.DeploymentVersion) error
	getDeploymentVersionFn               func(ctx context.Context, deploymentID, projectID string) (*domain.DeploymentVersion, error)
	listDeploymentVersionsFn             func(ctx context.Context, projectID, environment string, limit int, cursor *time.Time) ([]domain.DeploymentVersion, error)
	finalizeDeploymentVersionFn          func(ctx context.Context, deploymentID, projectID, updatedBy string) (*domain.DeploymentVersion, error)
	promoteDeploymentVersionFn           func(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error)
	rollbackDeploymentVersionFn          func(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error)
	updateWorkflowRunStatusFn            func(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	updateStepRunStatusFn                func(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	getStepRunByRunAndRefFn              func(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	getStepApprovalByStepRunFn           func(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	updateStepApprovalFn                 func(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	deleteJobSecretFn                    func(ctx context.Context, id string) error
	batchUpdateJobsEnabledFn             func(ctx context.Context, ids []string, enabled bool) (int64, error)
	getJobHealthStatsFn                  func(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	getDebugBundleFn                     func(ctx context.Context, runID string) (*domain.DebugBundle, error)
	updateRunDebugModeFn                 func(ctx context.Context, runID string, debugMode bool) error
	listEventsFn                         func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	createRunFn                          func(ctx context.Context, run *domain.JobRun) error
	listRunLineageFn                     func(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	sumRunCostMicrousdFn                 func(ctx context.Context, runID string) (int64, error)
	sumProjectDailyCostMicrousdFn        func(ctx context.Context, projectID string, timezone string) (int64, error)
	getUserPermissionsFn                 func(ctx context.Context, projectID, userID string) ([]string, error)
	createProjectRoleFn                  func(ctx context.Context, role *domain.ProjectRole) error
	getProjectRoleFn                     func(ctx context.Context, id string) (*domain.ProjectRole, error)
	updateProjectRoleFn                  func(ctx context.Context, role *domain.ProjectRole) error
	listProjectRolesFn                   func(ctx context.Context, projectID string) ([]domain.ProjectRole, error)
	deleteProjectRoleFn                  func(ctx context.Context, id string) error
	assignMemberRoleFn                   func(ctx context.Context, m *domain.ProjectMemberRole) error
	getMemberRoleFn                      func(ctx context.Context, projectID, userID string) (*domain.ProjectMemberRole, error)
	listProjectMembersFn                 func(ctx context.Context, projectID string) ([]domain.ProjectMemberRole, error)
	removeMemberRoleFn                   func(ctx context.Context, projectID, userID string) error
	createResourcePolicyFn               func(ctx context.Context, p *domain.ResourcePolicy) error
	deleteResourcePolicyFn               func(ctx context.Context, id string) (string, string, error)
	createTagPolicyFn                    func(ctx context.Context, p *domain.TagPolicy) error
	deleteTagPolicyFn                    func(ctx context.Context, id string) (string, string, error)
	createAuditEventFn                   func(ctx context.Context, ev *domain.AuditEvent) error
	createEventTriggerFn                 func(ctx context.Context, trigger *domain.EventTrigger) error
	getEventTriggerByEventKeyFn          func(ctx context.Context, key string) (*domain.EventTrigger, error)
	updateEventTriggerStatusFn           func(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	listEventTriggersByProjectFn         func(ctx context.Context, projectID, status, workflowRunID, sourceType string, limit int, cursor *time.Time) ([]domain.EventTrigger, error)
	listEventTriggersByKeyPrefixFn       func(ctx context.Context, prefix string, projectID string) ([]domain.EventTrigger, error)
	cancelEventTriggersByWorkflowRunFn   func(ctx context.Context, workflowRunID string) (int64, error)
	receiveEventAndRequeueRunFn          func(ctx context.Context, triggerID string, payload json.RawMessage, receivedAt time.Time, jobRunID string) error
	setEventTriggerSentByFn              func(ctx context.Context, id, sentBy string) error
	getEventTriggerStatsFn               func(ctx context.Context, projectID string) (*store.EventTriggerStats, error)
	batchReceiveEventTriggersFn          func(ctx context.Context, ids []string, payload json.RawMessage, receivedAt time.Time, sentBy string) ([]string, error)
	deleteEventTriggersFinishedBeforeFn  func(ctx context.Context, before time.Time, limit int) (int64, error)
	countEventTriggersFinishedBeforeFn   func(ctx context.Context, before time.Time) (int64, error)
	countActiveEventTriggersByProjectFn  func(ctx context.Context, projectID string) (int, error)
	getWebhookDeliveryFn                 func(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	retryWebhookDeliveryFn               func(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	updateWebhookDeliveryFn              func(ctx context.Context, d *domain.WebhookDelivery) error
	cancelNonTerminalStepRunsFn          func(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	cancelJobRunsByWorkflowRunFn         func(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	getRunsByIDsFn                       func(ctx context.Context, ids []string) (map[string]*domain.JobRun, error)
	bulkCancelRunsFn                     func(ctx context.Context, ids []string, finishedAt time.Time, reason string) ([]store.BulkCancelResult, error)
	cancelChildRunsByParentIDsFn         func(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error)
	rescheduleRunFn                      func(ctx context.Context, runID string, scheduledAt time.Time, payload json.RawMessage) error
	createBatchOperationFn               func(ctx context.Context, op *domain.BatchOperation) error
	finalizeBatchOperationFn             func(ctx context.Context, batchID string, createdCount int) error
	getBatchOperationFn                  func(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error)
	listBatchOperationsFn                func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.BatchOperation, error)
	bulkCancelByFilterFn                 func(ctx context.Context, projectID string, f store.BulkCancelFilter, now time.Time, reason string) ([]string, error)
	bulkCancelWorkflowRunsFn             func(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error)
	createLogDrainFn                     func(ctx context.Context, drain *domain.LogDrain) error
	getLogDrainFn                        func(ctx context.Context, drainID, projectID string) (*domain.LogDrain, error)
	listLogDrainsFn                      func(ctx context.Context, projectID string) ([]domain.LogDrain, error)
	updateLogDrainFn                     func(ctx context.Context, drainID, projectID string, patch map[string]any) error
	deleteLogDrainFn                     func(ctx context.Context, drainID, projectID string) error
	createEventSourceFn                  func(ctx context.Context, src *domain.EventSource) error
	getEventSourceFn                     func(ctx context.Context, sourceID, projectID string) (*domain.EventSource, error)
	getEventSourceByNameFn               func(ctx context.Context, projectID, name string) (*domain.EventSource, error)
	listEventSourcesFn                   func(ctx context.Context, projectID string) ([]domain.EventSource, error)
	updateEventSourceFn                  func(ctx context.Context, sourceID, projectID string, patch map[string]any) error
	deleteEventSourceFn                  func(ctx context.Context, sourceID, projectID string) error
	createEventSubscriptionFn            func(ctx context.Context, sub *domain.EventSubscription) error
	listEventSubscriptionsBySourceFn     func(ctx context.Context, sourceID string) ([]domain.EventSubscription, error)
	deleteEventSubscriptionFn            func(ctx context.Context, subID string) error
	upsertRunStateFn                     func(ctx context.Context, s *domain.RunState) error
	getRunStateFn                        func(ctx context.Context, runID, key string) (*domain.RunState, error)
	listRunStateFn                       func(ctx context.Context, runID string) ([]domain.RunState, error)
	deleteRunStateFn                     func(ctx context.Context, runID, key string) error
	upsertJobMemoryFn                    func(ctx context.Context, mem *domain.JobMemory) error
	upsertJobMemoryWithQuotaFn           func(ctx context.Context, mem *domain.JobMemory, maxPerKey, maxPerJob int) error
	getJobMemoryFn                       func(ctx context.Context, jobID, key string) (*domain.JobMemory, error)
	listJobMemoryFn                      func(ctx context.Context, jobID string) ([]domain.JobMemory, error)
	deleteJobMemoryFn                    func(ctx context.Context, jobID, key string) error
	sumJobMemorySizeBytesFn              func(ctx context.Context, jobID string) (int, error)
	replayWebhookDeliveryFn              func(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	createWebhookDeliveryFn              func(ctx context.Context, d *domain.WebhookDelivery) error
	listManagedMachineIDsByWorkflowRunFn func(ctx context.Context, workflowRunID string) ([]string, error)
	markJobRunsPausedByWorkflowRunFn     func(ctx context.Context, workflowRunID string) (int64, error)
	requeuePausedJobRunsFn               func(ctx context.Context, workflowRunID string) (int64, error)
	updateProjectDefaultRegionFn         func(ctx context.Context, projectID, defaultRegion string) error
	createNotificationChannelFn          func(ctx context.Context, ch *domain.NotificationChannel) error
	getNotificationChannelFn             func(ctx context.Context, id, projectID string) (*domain.NotificationChannel, error)
	listNotificationChannelsFn           func(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	updateNotificationChannelFn          func(ctx context.Context, ch *domain.NotificationChannel) error
	deleteNotificationChannelFn          func(ctx context.Context, id, projectID string) error
	createNotificationDeliveryFn         func(ctx context.Context, d *domain.NotificationDelivery) error
	listNotificationDeliveriesFn         func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error)
	createDeviceCodeFn                   func(ctx context.Context, deviceCode, userCode, projectID string, scopes []string, expiresAt time.Time) error
	getDeviceCodeByDeviceCodeFn          func(ctx context.Context, deviceCode string) (*store.DeviceCodeRow, error)
	approveDeviceCodeFn                  func(ctx context.Context, deviceCode, apiKeyID, rawAPIKey string) error
	exchangeDeviceCodeFn                 func(ctx context.Context, deviceCode string) (string, error)
	cleanupExpiredDeviceCodesFn          func(ctx context.Context) (int64, error)
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

func (m *mockAPIStore) PauseJobsByGroup(ctx context.Context, groupID string) error {
	if m.pauseJobsByGroupFn != nil {
		return m.pauseJobsByGroupFn(ctx, groupID)
	}
	return nil
}

func (m *mockAPIStore) ResumeJobsByGroup(ctx context.Context, groupID string) error {
	if m.resumeJobsByGroupFn != nil {
		return m.resumeJobsByGroupFn(ctx, groupID)
	}
	return nil
}

func (m *mockAPIStore) GetJobGroupStats(ctx context.Context, groupID string) (*store.JobGroupStats, error) {
	if m.getJobGroupStatsFn != nil {
		return m.getJobGroupStatsFn(ctx, groupID)
	}
	return &store.JobGroupStats{GroupID: groupID, RunCounts: map[string]int{}}, nil
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

func (m *mockAPIStore) AreJobDependenciesSatisfied(ctx context.Context, run *domain.JobRun) (bool, error) {
	if m.areJobDependenciesSatisfiedFn != nil {
		return m.areJobDependenciesSatisfiedFn(ctx, run)
	}
	return true, nil
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

func (m *mockAPIStore) CreateRunResourceSnapshot(ctx context.Context, s *domain.RunResourceSnapshot) error {
	if m.createRunResourceSnapshotFn != nil {
		return m.createRunResourceSnapshotFn(ctx, s)
	}
	return nil
}

func (m *mockAPIStore) ListRunResourceSnapshots(ctx context.Context, runID string, from, to *time.Time, limit int) ([]domain.RunResourceSnapshot, error) {
	if m.listRunResourceSnapshotsFn != nil {
		return m.listRunResourceSnapshotsFn(ctx, runID, from, to, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) SumRunTotalTokens(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockAPIStore) CountRunToolCalls(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockAPIStore) CountRunIterations(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockAPIStore) CreateRunIteration(_ context.Context, _ *domain.RunIteration) error {
	return nil
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

func (m *mockAPIStore) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, executionMode *domain.ExecutionMode, errorClass *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	if m.listRunsByProjectFn != nil {
		return m.listRunsByProjectFn(ctx, projectID, status, metadataKey, metadataValue, triggeredBy, batchID, payloadContains, executionMode, errorClass, limit, cursor)
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

func (m *mockAPIStore) BulkReplayDeadLetterRuns(ctx context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
	if m.bulkReplayDeadLetterRunsFn != nil {
		return m.bulkReplayDeadLetterRunsFn(ctx, runIDs, projectID, limit)
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

func (m *mockAPIStore) ResetRunIdempotencyKey(ctx context.Context, runID string) error {
	if m.resetRunIdempotencyKeyFn != nil {
		return m.resetRunIdempotencyKeyFn(ctx, runID)
	}
	return nil
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

func (m *mockAPIStore) CreateWebhookSubscription(ctx context.Context, sub *domain.WebhookSubscription) error {
	if m.createWebhookSubscriptionFn != nil {
		return m.createWebhookSubscriptionFn(ctx, sub)
	}
	return nil
}

func (m *mockAPIStore) ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error) {
	if m.listWebhookSubscriptionsFn != nil {
		return m.listWebhookSubscriptionsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) DeleteWebhookSubscription(ctx context.Context, id string) error {
	if m.deleteWebhookSubscriptionFn != nil {
		return m.deleteWebhookSubscriptionFn(ctx, id)
	}
	return nil
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

func (m *mockAPIStore) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	if m.batchUpdateHeartbeatFn != nil {
		return m.batchUpdateHeartbeatFn(ctx, ids)
	}
	return nil
}

func (m *mockAPIStore) QueueStats(ctx context.Context) (*store.QueueStats, error) {
	if m.queueStatsFn != nil {
		return m.queueStatsFn(ctx)
	}
	return &store.QueueStats{}, nil
}

func (m *mockAPIStore) GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*store.PerformanceAnalytics, error) {
	if m.getPerformanceAnalyticsFn != nil {
		return m.getPerformanceAnalyticsFn(ctx, projectID, periodHours)
	}
	return &store.PerformanceAnalytics{}, nil
}

func (m *mockAPIStore) GetCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.CostAnalytics, error) {
	if m.getCostAnalyticsFn != nil {
		return m.getCostAnalyticsFn(ctx, projectID, from, to)
	}
	return &store.CostAnalytics{}, nil
}

func (m *mockAPIStore) GetCostTrends(ctx context.Context, projectID string, from, to time.Time) ([]store.CostTrendPoint, error) {
	if m.getCostTrendsFn != nil {
		return m.getCostTrendsFn(ctx, projectID, from, to)
	}
	return nil, nil
}

func (m *mockAPIStore) GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopCostItem, error) {
	if m.getTopCostsFn != nil {
		return m.getTopCostsFn(ctx, projectID, from, to, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) GetComputeCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.ComputeCostAnalytics, error) {
	if m.getComputeCostAnalyticsFn != nil {
		return m.getComputeCostAnalyticsFn(ctx, projectID, from, to)
	}
	return &store.ComputeCostAnalytics{}, nil
}

func (m *mockAPIStore) GetApprovalStats(ctx context.Context, projectID string, from, to time.Time) (*store.ApprovalStats, error) {
	if m.getApprovalStatsFn != nil {
		return m.getApprovalStatsFn(ctx, projectID, from, to)
	}
	return &store.ApprovalStats{}, nil
}

func (m *mockAPIStore) GetCostOutliers(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]store.CostOutlier, error) {
	if m.getCostOutliersFn != nil {
		return m.getCostOutliersFn(ctx, projectID, from, to, threshold)
	}
	return nil, nil
}

func (m *mockAPIStore) AggregateCostStatsHourly(ctx context.Context, hour time.Time) error {
	if m.aggregateCostStatsHourlyFn != nil {
		return m.aggregateCostStatsHourlyFn(ctx, hour)
	}
	return nil
}

func (m *mockAPIStore) GetRunTimeline(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.RunTimelineBucket, error) {
	if m.getRunTimelineFn != nil {
		return m.getRunTimelineFn(ctx, projectID, from, to, bucket)
	}
	return []store.RunTimelineBucket{}, nil
}

func (m *mockAPIStore) GetRunDurationDistribution(ctx context.Context, projectID string, from, to time.Time) ([]store.RunDurationBucket, error) {
	if m.getRunDurationDistributionFn != nil {
		return m.getRunDurationDistributionFn(ctx, projectID, from, to)
	}
	return []store.RunDurationBucket{}, nil
}

func (m *mockAPIStore) GetRunFailureReasons(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.RunFailureReason, error) {
	if m.getRunFailureReasonsFn != nil {
		return m.getRunFailureReasonsFn(ctx, projectID, from, to, limit)
	}
	return []store.RunFailureReason{}, nil
}

func (m *mockAPIStore) GetRunSummary(ctx context.Context, projectID string, from, to time.Time) (*store.RunSummary, error) {
	if m.getRunSummaryFn != nil {
		return m.getRunSummaryFn(ctx, projectID, from, to)
	}
	return &store.RunSummary{}, nil
}

func (m *mockAPIStore) GetRunsByTrigger(ctx context.Context, projectID string, from, to time.Time) ([]store.RunsByTrigger, error) {
	if m.getRunsByTriggerFn != nil {
		return m.getRunsByTriggerFn(ctx, projectID, from, to)
	}
	return []store.RunsByTrigger{}, nil
}

func (m *mockAPIStore) GetJobHistory(ctx context.Context, projectID, jobID string, from, to time.Time, bucket string) ([]store.JobHistoryBucket, error) {
	if m.getJobHistoryFn != nil {
		return m.getJobHistoryFn(ctx, projectID, jobID, from, to, bucket)
	}
	return []store.JobHistoryBucket{}, nil
}

func (m *mockAPIStore) GetJobComparison(ctx context.Context, projectID string, jobIDs []string, from, to time.Time) ([]store.JobComparison, error) {
	if m.getJobComparisonFn != nil {
		return m.getJobComparisonFn(ctx, projectID, jobIDs, from, to)
	}
	return []store.JobComparison{}, nil
}

func (m *mockAPIStore) GetJobReliability(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobReliability, error) {
	if m.getJobReliabilityFn != nil {
		return m.getJobReliabilityFn(ctx, projectID, from, to, limit)
	}
	return []store.JobReliability{}, nil
}

func (m *mockAPIStore) GetRunsByVersion(ctx context.Context, projectID, jobID string, from, to time.Time) ([]store.RunsByVersion, error) {
	if m.getRunsByVersionFn != nil {
		return m.getRunsByVersionFn(ctx, projectID, jobID, from, to)
	}
	return []store.RunsByVersion{}, nil
}

func (m *mockAPIStore) GetJobCostRanking(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobCostRanking, error) {
	if m.getJobCostRankingFn != nil {
		return m.getJobCostRankingFn(ctx, projectID, from, to, limit)
	}
	return []store.JobCostRanking{}, nil
}

func (m *mockAPIStore) GetTopFailingJobs(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingJob, error) {
	if m.getTopFailingJobsFn != nil {
		return m.getTopFailingJobsFn(ctx, projectID, from, to, limit)
	}
	return []store.TopFailingJob{}, nil
}

func (m *mockAPIStore) GetTagSummary(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagSummary, error) {
	if m.getTagSummaryFn != nil {
		return m.getTagSummaryFn(ctx, projectID, from, to, limit)
	}
	return []store.TagSummary{}, nil
}

func (m *mockAPIStore) GetTopFailingTags(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingTag, error) {
	if m.getTopFailingTagsFn != nil {
		return m.getTopFailingTagsFn(ctx, projectID, from, to, limit)
	}
	return []store.TopFailingTag{}, nil
}

func (m *mockAPIStore) GetTagCost(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagCost, error) {
	if m.getTagCostFn != nil {
		return m.getTagCostFn(ctx, projectID, from, to, limit)
	}
	return []store.TagCost{}, nil
}

func (m *mockAPIStore) GetWorkflowStepDurations(ctx context.Context, projectID, workflowID string, from, to time.Time) ([]store.StepDuration, error) {
	if m.getWorkflowStepDurationsFn != nil {
		return m.getWorkflowStepDurationsFn(ctx, projectID, workflowID, from, to)
	}
	return []store.StepDuration{}, nil
}

func (m *mockAPIStore) GetWorkflowCompletionRates(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WorkflowCompletionBucket, error) {
	if m.getWorkflowCompletionRatesFn != nil {
		return m.getWorkflowCompletionRatesFn(ctx, projectID, from, to, bucket)
	}
	return []store.WorkflowCompletionBucket{}, nil
}

func (m *mockAPIStore) GetWorkflowSummary(ctx context.Context, projectID string, from, to time.Time) (*store.WorkflowSummary, error) {
	if m.getWorkflowSummaryFn != nil {
		return m.getWorkflowSummaryFn(ctx, projectID, from, to)
	}
	return &store.WorkflowSummary{}, nil
}

func (m *mockAPIStore) GetWebhookDeliveryStats(ctx context.Context, projectID string, from, to time.Time) ([]store.WebhookEndpointStats, error) {
	if m.getWebhookDeliveryStatsFn != nil {
		return m.getWebhookDeliveryStatsFn(ctx, projectID, from, to)
	}
	return []store.WebhookEndpointStats{}, nil
}

func (m *mockAPIStore) GetWebhookEndpointHealth(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WebhookHealthBucket, error) {
	if m.getWebhookEndpointHealthFn != nil {
		return m.getWebhookEndpointHealthFn(ctx, projectID, from, to, bucket)
	}
	return []store.WebhookHealthBucket{}, nil
}

func (m *mockAPIStore) GetTopFailingWebhooks(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingEndpoint, error) {
	if m.getTopFailingWebhooksFn != nil {
		return m.getTopFailingWebhooksFn(ctx, projectID, from, to, limit)
	}
	return []store.TopFailingEndpoint{}, nil
}

func (m *mockAPIStore) GetEventVolume(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.EventVolumeBucket, error) {
	if m.getEventVolumeFn != nil {
		return m.getEventVolumeFn(ctx, projectID, from, to, bucket)
	}
	return []store.EventVolumeBucket{}, nil
}

func (m *mockAPIStore) GetEventLatency(ctx context.Context, projectID string, from, to time.Time) (*store.EventLatencyStats, error) {
	if m.getEventLatencyFn != nil {
		return m.getEventLatencyFn(ctx, projectID, from, to)
	}
	return &store.EventLatencyStats{}, nil
}

func (m *mockAPIStore) GetCostForecast(ctx context.Context, projectID string, from, to time.Time) (*store.CostForecast, error) {
	if m.getCostForecastFn != nil {
		return m.getCostForecastFn(ctx, projectID, from, to)
	}
	return &store.CostForecast{}, nil
}

func (m *mockAPIStore) GetCostByTrigger(ctx context.Context, projectID string, from, to time.Time) ([]store.CostByTrigger, error) {
	if m.getCostByTriggerFn != nil {
		return m.getCostByTriggerFn(ctx, projectID, from, to)
	}
	return []store.CostByTrigger{}, nil
}

func (m *mockAPIStore) GetCostByMachine(ctx context.Context, projectID string, from, to time.Time) ([]store.CostByMachine, error) {
	if m.getCostByMachineFn != nil {
		return m.getCostByMachineFn(ctx, projectID, from, to)
	}
	return []store.CostByMachine{}, nil
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

func (m *mockAPIStore) ListWorkflowsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Workflow, error) {
	if m.listWorkflowsByTagFn != nil {
		return m.listWorkflowsByTagFn(ctx, projectID, tagKey, tagValue, limit, cursor)
	}
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

func (m *mockAPIStore) ListWorkflowRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	if m.listWorkflowRunsByTagFn != nil {
		return m.listWorkflowRunsByTagFn(ctx, projectID, tagKey, tagValue, limit, cursor)
	}
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

func (m *mockAPIStore) ListWorkflowStepDecisions(ctx context.Context, workflowRunID, stepRef, decisionType string, limit int, cursor *time.Time) ([]domain.WorkflowStepDecision, error) {
	if m.listWorkflowStepDecisionsFn != nil {
		return m.listWorkflowStepDecisionsFn(ctx, workflowRunID, stepRef, decisionType, limit, cursor)
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
	if m.listWorkflowVersionsFn != nil {
		return m.listWorkflowVersionsFn(ctx, workflowID, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) GetWorkflowVersionByVersionID(ctx context.Context, workflowID, versionID string) (*domain.WorkflowVersion, error) {
	if m.getWorkflowVersionByVersionIDFn != nil {
		return m.getWorkflowVersionByVersionIDFn(ctx, workflowID, versionID)
	}
	return nil, store.ErrWorkflowVersionNotFound
}

func (m *mockAPIStore) UpsertWorkflowPolicy(ctx context.Context, p *domain.WorkflowPolicy) error {
	if m.upsertWorkflowPolicyFn != nil {
		return m.upsertWorkflowPolicyFn(ctx, p)
	}
	return nil
}

func (m *mockAPIStore) GetWorkflowPolicyByProject(ctx context.Context, projectID string) (*domain.WorkflowPolicy, error) {
	if m.getWorkflowPolicyByProjectFn != nil {
		return m.getWorkflowPolicyByProjectFn(ctx, projectID)
	}
	return nil, nil
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
	enqueueBatchFn      func(ctx context.Context, runs []*domain.JobRun) (int64, error)
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

func (m *mockQueue) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	if m.enqueueBatchFn != nil {
		return m.enqueueBatchFn(ctx, runs)
	}
	// Fall back to individual enqueue for backwards-compatible tests.
	for _, run := range runs {
		if m.enqueueFn != nil {
			if err := m.enqueueFn(ctx, run); err != nil {
				return 0, err
			}
		}
	}
	return int64(len(runs)), nil
}

func (m *mockQueue) DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error) {
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

func (m *mockPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, msg := range messages {
		if err := m.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
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

func (m *mockAPIStore) GetMemberRole(ctx context.Context, projectID, userID string) (*domain.ProjectMemberRole, error) {
	if m.getMemberRoleFn != nil {
		return m.getMemberRoleFn(ctx, projectID, userID)
	}
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

func (m *mockAPIStore) CreateResourcePolicy(ctx context.Context, p *domain.ResourcePolicy) error {
	if m.createResourcePolicyFn != nil {
		return m.createResourcePolicyFn(ctx, p)
	}
	return nil
}

func (m *mockAPIStore) GetResourcePolicies(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockAPIStore) DeleteResourcePolicy(ctx context.Context, id string) (string, string, error) {
	if m.deleteResourcePolicyFn != nil {
		return m.deleteResourcePolicyFn(ctx, id)
	}
	return "", "", nil
}

func (m *mockAPIStore) ListResourcePolicies(_ context.Context, _, _ string, _ int, _ *time.Time) ([]domain.ResourcePolicy, error) {
	return nil, nil
}

func (m *mockAPIStore) CreateTagPolicy(ctx context.Context, p *domain.TagPolicy) error {
	if m.createTagPolicyFn != nil {
		return m.createTagPolicyFn(ctx, p)
	}
	return nil
}

func (m *mockAPIStore) ListTagPolicies(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.TagPolicy, error) {
	return nil, nil
}

func (m *mockAPIStore) DeleteTagPolicy(ctx context.Context, id string) (string, string, error) {
	if m.deleteTagPolicyFn != nil {
		return m.deleteTagPolicyFn(ctx, id)
	}
	return "", "", store.ErrTagPolicyNotFound
}

func (m *mockAPIStore) GetTagPolicyActions(_ context.Context, _, _, _ string, _ map[string]string) ([]string, error) {
	return nil, nil
}

func (m *mockAPIStore) CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error {
	if m.createAuditEventFn != nil {
		return m.createAuditEventFn(ctx, ev)
	}
	return nil
}

func (m *mockAPIStore) ListAuditEvents(_ context.Context, _, _, _, _ string, _ int, _, _, _ *time.Time, _ bool) ([]domain.AuditEvent, error) {
	return nil, nil
}

func (m *mockAPIStore) StreamAuditEvents(ctx context.Context, projectID, actorID, resourceType string, from, to time.Time, fn func(*domain.AuditEvent) error) error {
	if m.streamAuditEventsFn != nil {
		return m.streamAuditEventsFn(ctx, projectID, actorID, resourceType, from, to, fn)
	}
	return nil
}

// Event trigger mock methods.

func (m *mockAPIStore) CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error {
	if m.createEventTriggerFn != nil {
		return m.createEventTriggerFn(ctx, trigger)
	}
	return nil
}

func (m *mockAPIStore) GetEventTriggerByEventKey(ctx context.Context, key string) (*domain.EventTrigger, error) {
	if m.getEventTriggerByEventKeyFn != nil {
		return m.getEventTriggerByEventKeyFn(ctx, key)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error {
	if m.updateEventTriggerStatusFn != nil {
		return m.updateEventTriggerStatusFn(ctx, id, status, responsePayload, receivedAt, errMsg)
	}
	return nil
}

func (m *mockAPIStore) ListEventTriggersByProject(ctx context.Context, projectID, status, workflowRunID, sourceType string, limit int, cursor *time.Time) ([]domain.EventTrigger, error) {
	if m.listEventTriggersByProjectFn != nil {
		return m.listEventTriggersByProjectFn(ctx, projectID, status, workflowRunID, sourceType, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) ListEventTriggersByKeyPrefix(ctx context.Context, prefix string, projectID string) ([]domain.EventTrigger, error) {
	if m.listEventTriggersByKeyPrefixFn != nil {
		return m.listEventTriggersByKeyPrefixFn(ctx, prefix, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error) {
	if m.cancelEventTriggersByWorkflowRunFn != nil {
		return m.cancelEventTriggersByWorkflowRunFn(ctx, workflowRunID)
	}
	return 0, nil
}

func (m *mockAPIStore) ReceiveEventAndRequeueRun(ctx context.Context, triggerID string, payload json.RawMessage, receivedAt time.Time, jobRunID string) error {
	if m.receiveEventAndRequeueRunFn != nil {
		return m.receiveEventAndRequeueRunFn(ctx, triggerID, payload, receivedAt, jobRunID)
	}
	return nil
}

func (m *mockAPIStore) SetEventTriggerSentBy(ctx context.Context, id, sentBy string) error {
	if m.setEventTriggerSentByFn != nil {
		return m.setEventTriggerSentByFn(ctx, id, sentBy)
	}
	return nil
}

func (m *mockAPIStore) GetEventTriggerStats(ctx context.Context, projectID string) (*store.EventTriggerStats, error) {
	if m.getEventTriggerStatsFn != nil {
		return m.getEventTriggerStatsFn(ctx, projectID)
	}
	return &store.EventTriggerStats{}, nil
}

func (m *mockAPIStore) BatchReceiveEventTriggers(ctx context.Context, ids []string, payload json.RawMessage, receivedAt time.Time, sentBy string) ([]string, error) {
	if m.batchReceiveEventTriggersFn != nil {
		return m.batchReceiveEventTriggersFn(ctx, ids, payload, receivedAt, sentBy)
	}
	return ids, nil
}

func (m *mockAPIStore) DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if m.deleteEventTriggersFinishedBeforeFn != nil {
		return m.deleteEventTriggersFinishedBeforeFn(ctx, before, limit)
	}
	return 0, nil
}

func (m *mockAPIStore) CountEventTriggersFinishedBefore(ctx context.Context, before time.Time) (int64, error) {
	if m.countEventTriggersFinishedBeforeFn != nil {
		return m.countEventTriggersFinishedBeforeFn(ctx, before)
	}
	return 0, nil
}

func (m *mockAPIStore) CountActiveEventTriggersByProject(ctx context.Context, projectID string) (int, error) {
	if m.countActiveEventTriggersByProjectFn != nil {
		return m.countActiveEventTriggersByProjectFn(ctx, projectID)
	}
	return 0, nil
}

func (m *mockAPIStore) GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	if m.getWebhookDeliveryFn != nil {
		return m.getWebhookDeliveryFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) RetryWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	if m.retryWebhookDeliveryFn != nil {
		return m.retryWebhookDeliveryFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	if m.updateWebhookDeliveryFn != nil {
		return m.updateWebhookDeliveryFn(ctx, d)
	}
	return nil
}

func (m *mockAPIStore) CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	if m.cancelNonTerminalStepRunsFn != nil {
		return m.cancelNonTerminalStepRunsFn(ctx, workflowRunID, finishedAt, reason)
	}
	return 0, nil
}

func (m *mockAPIStore) CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	if m.cancelJobRunsByWorkflowRunFn != nil {
		return m.cancelJobRunsByWorkflowRunFn(ctx, workflowRunID, finishedAt, reason)
	}
	return 0, nil
}

func (m *mockAPIStore) GetRunsByIDs(ctx context.Context, ids []string) (map[string]*domain.JobRun, error) {
	if m.getRunsByIDsFn != nil {
		return m.getRunsByIDsFn(ctx, ids)
	}
	return nil, nil
}

func (m *mockAPIStore) BulkCancelRuns(ctx context.Context, ids []string, finishedAt time.Time, reason string) ([]store.BulkCancelResult, error) {
	if m.bulkCancelRunsFn != nil {
		return m.bulkCancelRunsFn(ctx, ids, finishedAt, reason)
	}
	return nil, nil
}

func (m *mockAPIStore) CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error) {
	if m.cancelChildRunsByParentIDsFn != nil {
		return m.cancelChildRunsByParentIDsFn(ctx, parentIDs, finishedAt, reason)
	}
	return 0, nil
}

func (m *mockAPIStore) CreateBatchOperation(ctx context.Context, op *domain.BatchOperation) error {
	if m.createBatchOperationFn != nil {
		return m.createBatchOperationFn(ctx, op)
	}
	return nil
}

func (m *mockAPIStore) FinalizeBatchOperation(ctx context.Context, batchID string, createdCount int) error {
	if m.finalizeBatchOperationFn != nil {
		return m.finalizeBatchOperationFn(ctx, batchID, createdCount)
	}
	return nil
}

func (m *mockAPIStore) GetBatchOperation(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error) {
	if m.getBatchOperationFn != nil {
		return m.getBatchOperationFn(ctx, batchID, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) ListBatchOperations(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.BatchOperation, error) {
	if m.listBatchOperationsFn != nil {
		return m.listBatchOperationsFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) RescheduleRun(ctx context.Context, runID string, scheduledAt time.Time, payload json.RawMessage) error {
	if m.rescheduleRunFn != nil {
		return m.rescheduleRunFn(ctx, runID, scheduledAt, payload)
	}
	return nil
}

func (m *mockAPIStore) BulkCancelByFilter(ctx context.Context, projectID string, f store.BulkCancelFilter, now time.Time, reason string) ([]string, error) {
	if m.bulkCancelByFilterFn != nil {
		return m.bulkCancelByFilterFn(ctx, projectID, f, now, reason)
	}
	return nil, nil
}

func (m *mockAPIStore) BulkCancelWorkflowRuns(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error) {
	if m.bulkCancelWorkflowRunsFn != nil {
		return m.bulkCancelWorkflowRunsFn(ctx, projectID, ids, now)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateLogDrain(ctx context.Context, drain *domain.LogDrain) error {
	if m.createLogDrainFn != nil {
		return m.createLogDrainFn(ctx, drain)
	}
	return nil
}

func (m *mockAPIStore) GetLogDrain(ctx context.Context, drainID, projectID string) (*domain.LogDrain, error) {
	if m.getLogDrainFn != nil {
		return m.getLogDrainFn(ctx, drainID, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) ListLogDrains(ctx context.Context, projectID string) ([]domain.LogDrain, error) {
	if m.listLogDrainsFn != nil {
		return m.listLogDrainsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateLogDrain(ctx context.Context, drainID, projectID string, patch map[string]any) error {
	if m.updateLogDrainFn != nil {
		return m.updateLogDrainFn(ctx, drainID, projectID, patch)
	}
	return nil
}

func (m *mockAPIStore) DeleteLogDrain(ctx context.Context, drainID, projectID string) error {
	if m.deleteLogDrainFn != nil {
		return m.deleteLogDrainFn(ctx, drainID, projectID)
	}
	return nil
}

func (m *mockAPIStore) CreateEventSource(ctx context.Context, src *domain.EventSource) error {
	if m.createEventSourceFn != nil {
		return m.createEventSourceFn(ctx, src)
	}
	return nil
}

func (m *mockAPIStore) GetEventSource(ctx context.Context, sourceID, projectID string) (*domain.EventSource, error) {
	if m.getEventSourceFn != nil {
		return m.getEventSourceFn(ctx, sourceID, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) GetEventSourceByName(ctx context.Context, projectID, name string) (*domain.EventSource, error) {
	if m.getEventSourceByNameFn != nil {
		return m.getEventSourceByNameFn(ctx, projectID, name)
	}
	return nil, nil
}

func (m *mockAPIStore) ListEventSources(ctx context.Context, projectID string) ([]domain.EventSource, error) {
	if m.listEventSourcesFn != nil {
		return m.listEventSourcesFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateEventSource(ctx context.Context, sourceID, projectID string, patch map[string]any) error {
	if m.updateEventSourceFn != nil {
		return m.updateEventSourceFn(ctx, sourceID, projectID, patch)
	}
	return nil
}

func (m *mockAPIStore) DeleteEventSource(ctx context.Context, sourceID, projectID string) error {
	if m.deleteEventSourceFn != nil {
		return m.deleteEventSourceFn(ctx, sourceID, projectID)
	}
	return nil
}

func (m *mockAPIStore) CreateEventSubscription(ctx context.Context, sub *domain.EventSubscription) error {
	if m.createEventSubscriptionFn != nil {
		return m.createEventSubscriptionFn(ctx, sub)
	}
	return nil
}

func (m *mockAPIStore) ListEventSubscriptionsBySource(ctx context.Context, sourceID string) ([]domain.EventSubscription, error) {
	if m.listEventSubscriptionsBySourceFn != nil {
		return m.listEventSubscriptionsBySourceFn(ctx, sourceID)
	}
	return nil, nil
}

func (m *mockAPIStore) DeleteEventSubscription(ctx context.Context, subID string) error {
	if m.deleteEventSubscriptionFn != nil {
		return m.deleteEventSubscriptionFn(ctx, subID)
	}
	return nil
}

func (m *mockAPIStore) CreateDeploymentVersion(ctx context.Context, deployment *domain.DeploymentVersion) error {
	if m.createDeploymentVersionFn != nil {
		return m.createDeploymentVersionFn(ctx, deployment)
	}
	return nil
}

func (m *mockAPIStore) GetDeploymentVersion(ctx context.Context, deploymentID, projectID string) (*domain.DeploymentVersion, error) {
	if m.getDeploymentVersionFn != nil {
		return m.getDeploymentVersionFn(ctx, deploymentID, projectID)
	}
	return nil, store.ErrDeploymentVersionNotFound
}

func (m *mockAPIStore) ListDeploymentVersions(ctx context.Context, projectID, environment string, limit int, cursor *time.Time) ([]domain.DeploymentVersion, error) {
	if m.listDeploymentVersionsFn != nil {
		return m.listDeploymentVersionsFn(ctx, projectID, environment, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) FinalizeDeploymentVersion(ctx context.Context, deploymentID, projectID, updatedBy string) (*domain.DeploymentVersion, error) {
	if m.finalizeDeploymentVersionFn != nil {
		return m.finalizeDeploymentVersionFn(ctx, deploymentID, projectID, updatedBy)
	}
	return nil, store.ErrDeploymentVersionNotFound
}

func (m *mockAPIStore) PromoteDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error) {
	if m.promoteDeploymentVersionFn != nil {
		return m.promoteDeploymentVersionFn(ctx, deploymentID, projectID, environment, updatedBy)
	}
	return nil, store.ErrDeploymentVersionNotFound
}

func (m *mockAPIStore) RollbackDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error) {
	if m.rollbackDeploymentVersionFn != nil {
		return m.rollbackDeploymentVersionFn(ctx, deploymentID, projectID, environment, updatedBy)
	}
	return nil, store.ErrDeploymentVersionNotFound
}

func (m *mockAPIStore) CountBatchBufferItems(ctx context.Context, jobID, batchKey string) (int, error) {
	return 0, nil
}

func (m *mockAPIStore) CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	if m.createWebhookDeliveryFn != nil {
		return m.createWebhookDeliveryFn(ctx, d)
	}
	return nil
}

func (m *mockAPIStore) ReplayWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	if m.replayWebhookDeliveryFn != nil {
		return m.replayWebhookDeliveryFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) UpsertDebouncePending(ctx context.Context, d *domain.DebouncePending) error {
	return nil
}

func (m *mockAPIStore) InsertBatchBufferItem(ctx context.Context, item *domain.BatchBufferItem) error {
	return nil
}

func (m *mockAPIStore) DrainBatchBuffer(ctx context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error) {
	return nil, nil
}

func (m *mockAPIStore) UpsertRunState(ctx context.Context, s *domain.RunState) error {
	if m.upsertRunStateFn != nil {
		return m.upsertRunStateFn(ctx, s)
	}
	return nil
}

func (m *mockAPIStore) GetRunState(ctx context.Context, runID, key string) (*domain.RunState, error) {
	if m.getRunStateFn != nil {
		return m.getRunStateFn(ctx, runID, key)
	}
	return nil, nil
}

func (m *mockAPIStore) ListRunState(ctx context.Context, runID string) ([]domain.RunState, error) {
	if m.listRunStateFn != nil {
		return m.listRunStateFn(ctx, runID)
	}
	return nil, nil
}

func (m *mockAPIStore) DeleteRunState(ctx context.Context, runID, key string) error {
	if m.deleteRunStateFn != nil {
		return m.deleteRunStateFn(ctx, runID, key)
	}
	return nil
}

func (m *mockAPIStore) ListManagedMachineIDsByWorkflowRun(ctx context.Context, workflowRunID string) ([]string, error) {
	if m.listManagedMachineIDsByWorkflowRunFn != nil {
		return m.listManagedMachineIDsByWorkflowRunFn(ctx, workflowRunID)
	}
	return nil, nil
}

func (m *mockAPIStore) MarkJobRunsPausedByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error) {
	if m.markJobRunsPausedByWorkflowRunFn != nil {
		return m.markJobRunsPausedByWorkflowRunFn(ctx, workflowRunID)
	}
	return 0, nil
}

func (m *mockAPIStore) RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error) {
	if m.requeuePausedJobRunsFn != nil {
		return m.requeuePausedJobRunsFn(ctx, workflowRunID)
	}
	return 0, nil
}

func (m *mockAPIStore) UpdateProjectDefaultRegion(ctx context.Context, projectID, defaultRegion string) error {
	if m.updateProjectDefaultRegionFn != nil {
		return m.updateProjectDefaultRegionFn(ctx, projectID, defaultRegion)
	}
	return nil
}

func (m *mockAPIStore) UpsertJobMemory(ctx context.Context, mem *domain.JobMemory) error {
	if m.upsertJobMemoryFn != nil {
		return m.upsertJobMemoryFn(ctx, mem)
	}
	return nil
}

func (m *mockAPIStore) UpsertJobMemoryWithQuota(ctx context.Context, mem *domain.JobMemory, maxPerKey, maxPerJob int) error {
	if m.upsertJobMemoryWithQuotaFn != nil {
		return m.upsertJobMemoryWithQuotaFn(ctx, mem, maxPerKey, maxPerJob)
	}
	return nil
}

func (m *mockAPIStore) GetJobMemory(ctx context.Context, jobID, key string) (*domain.JobMemory, error) {
	if m.getJobMemoryFn != nil {
		return m.getJobMemoryFn(ctx, jobID, key)
	}
	return nil, nil
}

func (m *mockAPIStore) ListJobMemory(ctx context.Context, jobID string) ([]domain.JobMemory, error) {
	if m.listJobMemoryFn != nil {
		return m.listJobMemoryFn(ctx, jobID)
	}
	return nil, nil
}

func (m *mockAPIStore) DeleteJobMemory(ctx context.Context, jobID, key string) error {
	if m.deleteJobMemoryFn != nil {
		return m.deleteJobMemoryFn(ctx, jobID, key)
	}
	return nil
}

func (m *mockAPIStore) SumJobMemorySizeBytes(ctx context.Context, jobID string) (int, error) {
	if m.sumJobMemorySizeBytesFn != nil {
		return m.sumJobMemorySizeBytesFn(ctx, jobID)
	}
	return 0, nil
}

func (m *mockAPIStore) CreateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error {
	if m.createNotificationChannelFn != nil {
		return m.createNotificationChannelFn(ctx, ch)
	}
	return nil
}

func (m *mockAPIStore) GetNotificationChannel(ctx context.Context, id, projectID string) (*domain.NotificationChannel, error) {
	if m.getNotificationChannelFn != nil {
		return m.getNotificationChannelFn(ctx, id, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) ListNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	if m.listNotificationChannelsFn != nil {
		return m.listNotificationChannelsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error {
	if m.updateNotificationChannelFn != nil {
		return m.updateNotificationChannelFn(ctx, ch)
	}
	return nil
}

func (m *mockAPIStore) DeleteNotificationChannel(ctx context.Context, id, projectID string) error {
	if m.deleteNotificationChannelFn != nil {
		return m.deleteNotificationChannelFn(ctx, id, projectID)
	}
	return nil
}

func (m *mockAPIStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	if m.createNotificationDeliveryFn != nil {
		return m.createNotificationDeliveryFn(ctx, d)
	}
	return nil
}

func (m *mockAPIStore) ListNotificationDeliveries(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error) {
	if m.listNotificationDeliveriesFn != nil {
		return m.listNotificationDeliveriesFn(ctx, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) CreateDeviceCode(ctx context.Context, deviceCode, userCode, projectID string, scopes []string, expiresAt time.Time) error {
	if m.createDeviceCodeFn != nil {
		return m.createDeviceCodeFn(ctx, deviceCode, userCode, projectID, scopes, expiresAt)
	}
	return nil
}

func (m *mockAPIStore) GetDeviceCodeByDeviceCode(ctx context.Context, deviceCode string) (*store.DeviceCodeRow, error) {
	if m.getDeviceCodeByDeviceCodeFn != nil {
		return m.getDeviceCodeByDeviceCodeFn(ctx, deviceCode)
	}
	return nil, store.ErrDeviceCodeNotFound
}

func (m *mockAPIStore) ApproveDeviceCode(ctx context.Context, deviceCode, apiKeyID, rawAPIKey string) error {
	if m.approveDeviceCodeFn != nil {
		return m.approveDeviceCodeFn(ctx, deviceCode, apiKeyID, rawAPIKey)
	}
	return nil
}

func (m *mockAPIStore) ExchangeDeviceCode(ctx context.Context, deviceCode string) (string, error) {
	if m.exchangeDeviceCodeFn != nil {
		return m.exchangeDeviceCodeFn(ctx, deviceCode)
	}
	return "", nil
}

func (m *mockAPIStore) CleanupExpiredDeviceCodes(ctx context.Context) (int64, error) {
	if m.cleanupExpiredDeviceCodesFn != nil {
		return m.cleanupExpiredDeviceCodesFn(ctx)
	}
	return 0, nil
}
