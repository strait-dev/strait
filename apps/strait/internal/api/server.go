package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"strait/internal/billing"
	"strait/internal/clickhouse"
	"strait/internal/compute"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/health"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/ratelimit"
	"strait/internal/store"
	"strait/internal/telemetry"
	"strait/internal/worker"

	"sync/atomic"

	"github.com/alitto/pond/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/redis/go-redis/v9"

	scalar "github.com/MarceloPetrucio/go-scalar-api-reference"
)

// globalAllowPrivateEndpoints is an atomic flag set by NewServer when
// ALLOW_PRIVATE_ENDPOINTS is true. Used by the package-level validateURL
// to skip private/loopback network checks in development.
var globalAllowPrivateEndpoints atomic.Bool

// APIStore is the subset of store operations needed by the API handlers.
// Composed of smaller, focused interfaces for each domain.
// ProjectContextSetter sets the app.current_project_id session variable for RLS policies.
type ProjectContextSetter interface {
	SetProjectContext(ctx context.Context, projectID string) error
	ClearProjectContext(ctx context.Context) error
}

//go:generate moq -stub -out mock_apistore_test.go -pkg api . APIStore
type APIStore interface {
	JobStore
	RunStore
	WorkflowStore
	DeploymentStore
	EventTriggerStore
	AuthStore
	RBACStore
	LogDrainStore
	EventSourceStore
	ProjectStore
	NotificationChannelStore
}

// ProjectStore handles project CRUD operations.
type ProjectStore interface {
	CreateProject(ctx context.Context, project *domain.Project) error
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]domain.Project, error)
	DeleteProject(ctx context.Context, id string) error
}

// JobStore handles job CRUD, groups, environments, secrets, and dependencies.
type JobStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Job, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	DeleteJob(ctx context.Context, id string) error
	BatchUpdateJobsEnabled(ctx context.Context, ids []string, enabled bool) (int64, error)
	ListJobsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Job, error)
	ListJobVersionsByJob(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobVersion, error)
	GetJobVersionByVersionID(ctx context.Context, versionID string) (*domain.JobVersion, error)
	GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	CreateJobGroup(ctx context.Context, group *domain.JobGroup) error
	GetJobGroup(ctx context.Context, id string) (*domain.JobGroup, error)
	ListJobGroups(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobGroup, error)
	UpdateJobGroup(ctx context.Context, group *domain.JobGroup) error
	DeleteJobGroup(ctx context.Context, id string) error
	ListJobsByGroup(ctx context.Context, groupID string, limit int, cursor *time.Time) ([]domain.Job, error)
	PauseJobsByGroup(ctx context.Context, groupID string) error
	ResumeJobsByGroup(ctx context.Context, groupID string) error
	GetJobGroupStats(ctx context.Context, groupID string) (*store.JobGroupStats, error)
	CreateEnvironment(ctx context.Context, env *domain.Environment) error
	GetEnvironment(ctx context.Context, id string) (*domain.Environment, error)
	ListEnvironments(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error)
	UpdateEnvironment(ctx context.Context, env *domain.Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
	GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error)
	CreateJobSecret(ctx context.Context, secret *domain.JobSecret) error
	ListJobSecrets(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error)
	DeleteJobSecret(ctx context.Context, id string) error
	CreateJobDependency(ctx context.Context, dep *domain.JobDependency) error
	ListJobDependencies(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error)
	DeleteJobDependency(ctx context.Context, id string) error
	AreJobDependenciesSatisfied(ctx context.Context, run *domain.JobRun) (bool, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	UpdateProjectDefaultRegion(ctx context.Context, projectID, defaultRegion string) error
	PauseJob(ctx context.Context, id, reason string) error
	ResumeJob(ctx context.Context, id string) error
}

// RunStore handles job runs, events, checkpoints, and related data.
type RunStore interface {
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, executionMode *domain.ExecutionMode, errorClass *string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListChildRuns(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListRunLineage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	BulkReplayDeadLetterRuns(ctx context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error
	UpdateRunDebugMode(ctx context.Context, runID string, debugMode bool) error
	ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error)
	AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error)
	UpdateHeartbeat(ctx context.Context, id string) error
	GetDebugBundle(ctx context.Context, runID string) (*domain.DebugBundle, error)
	CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	ListRunCheckpoints(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error)
	CreateRunUsage(ctx context.Context, usage *domain.RunUsage) error
	ListRunUsage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error)
	CreateRunToolCall(ctx context.Context, call *domain.RunToolCall) error
	ListRunToolCalls(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error)
	UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error
	ListRunOutputs(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error)
	SumRunCostMicrousd(ctx context.Context, runID string) (int64, error)
	SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
	GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	RetryWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	CreateWebhookSubscription(ctx context.Context, sub *domain.WebhookSubscription) error
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	GetWebhookSubscription(ctx context.Context, id string) (*domain.WebhookSubscription, error)
	DeleteWebhookSubscription(ctx context.Context, id string) error
	QueueStats(ctx context.Context) (*store.QueueStats, error)
	GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*store.PerformanceAnalytics, error)
	GetCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.CostAnalytics, error)
	GetCostTrends(ctx context.Context, projectID string, from, to time.Time) ([]store.CostTrendPoint, error)
	GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopCostItem, error)
	GetComputeCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.ComputeCostAnalytics, error)
	GetApprovalStats(ctx context.Context, projectID string, from, to time.Time) (*store.ApprovalStats, error)
	GetCostOutliers(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]store.CostOutlier, error)
	AggregateCostStatsHourly(ctx context.Context, hour time.Time) error
	GetRunsByIDs(ctx context.Context, ids []string) (map[string]*domain.JobRun, error)
	BulkCancelRuns(ctx context.Context, ids []string, finishedAt time.Time, reason string) ([]store.BulkCancelResult, error)
	CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error)
	ResetRunIdempotencyKey(ctx context.Context, runID string) error
	RescheduleRun(ctx context.Context, runID string, scheduledAt time.Time, payload json.RawMessage) error
	CreateBatchOperation(ctx context.Context, op *domain.BatchOperation) error
	FinalizeBatchOperation(ctx context.Context, batchID string, createdCount int) error
	GetBatchOperation(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error)
	ListBatchOperations(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.BatchOperation, error)
	BulkCancelByFilter(ctx context.Context, projectID string, f store.BulkCancelFilter, now time.Time, reason string) ([]string, error)
	UpsertDebouncePending(ctx context.Context, d *domain.DebouncePending) error
	InsertBatchBufferItem(ctx context.Context, item *domain.BatchBufferItem) error
	CountBatchBufferItems(ctx context.Context, jobID, batchKey string) (int, error)
	DrainBatchBuffer(ctx context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error)
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ReplayWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	UpsertRunState(ctx context.Context, s *domain.RunState) error
	GetRunState(ctx context.Context, runID, key string) (*domain.RunState, error)
	ListRunState(ctx context.Context, runID string) ([]domain.RunState, error)
	DeleteRunState(ctx context.Context, runID, key string) error
	CreateRunResourceSnapshot(ctx context.Context, snapshot *domain.RunResourceSnapshot) error
	ListRunResourceSnapshots(ctx context.Context, runID string, from, to *time.Time, limit int) ([]domain.RunResourceSnapshot, error)
	SumRunTotalTokens(ctx context.Context, runID string) (int64, error)
	CountRunToolCalls(ctx context.Context, runID string) (int, error)
	CountRunIterations(ctx context.Context, runID string) (int, error)
	CreateRunIteration(ctx context.Context, iter *domain.RunIteration) error
	UpsertJobMemory(ctx context.Context, mem *domain.JobMemory) error
	UpsertJobMemoryWithQuota(ctx context.Context, mem *domain.JobMemory, maxPerKey, maxPerJob int) error
	GetJobMemory(ctx context.Context, jobID, key string) (*domain.JobMemory, error)
	ListJobMemory(ctx context.Context, jobID string) ([]domain.JobMemory, error)
	DeleteJobMemory(ctx context.Context, jobID, key string) error
	SumJobMemorySizeBytes(ctx context.Context, jobID string) (int, error)
}

type LogDrainStore interface {
	CreateLogDrain(ctx context.Context, drain *domain.LogDrain) error
	GetLogDrain(ctx context.Context, drainID, projectID string) (*domain.LogDrain, error)
	ListLogDrains(ctx context.Context, projectID string) ([]domain.LogDrain, error)
	UpdateLogDrain(ctx context.Context, drainID, projectID string, patch map[string]any) error
	DeleteLogDrain(ctx context.Context, drainID, projectID string) error
}

// EventSourceStore handles event source and subscription operations.
type EventSourceStore interface {
	CreateEventSource(ctx context.Context, src *domain.EventSource) error
	GetEventSource(ctx context.Context, sourceID, projectID string) (*domain.EventSource, error)
	GetEventSourceByName(ctx context.Context, projectID, name string) (*domain.EventSource, error)
	ListEventSources(ctx context.Context, projectID string) ([]domain.EventSource, error)
	UpdateEventSource(ctx context.Context, sourceID, projectID string, patch map[string]any) error
	DeleteEventSource(ctx context.Context, sourceID, projectID string) error
	CreateEventSubscription(ctx context.Context, sub *domain.EventSubscription) error
	ListEventSubscriptionsBySource(ctx context.Context, sourceID string) ([]domain.EventSubscription, error)
	DeleteEventSubscription(ctx context.Context, subID string) error
}

// WorkflowStore handles workflows, steps, runs, and approvals.
type WorkflowStore interface {
	CreateWorkflow(ctx context.Context, w *domain.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	ListWorkflows(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Workflow, error)
	ListWorkflowsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Workflow, error)
	UpdateWorkflow(ctx context.Context, w *domain.Workflow) error
	DeleteWorkflow(ctx context.Context, id string) error
	CreateWorkflowVersionSnapshot(ctx context.Context, workflowID string, version int) error
	CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	DeleteStepsByWorkflow(ctx context.Context, workflowID string) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListWorkflowRuns(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	ListWorkflowRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	CreateWorkflowRunLabels(ctx context.Context, workflowRunID string, labels map[string]string) error
	ListWorkflowRunLabels(ctx context.Context, workflowRunID string) (map[string]string, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	ListWorkflowStepDecisions(ctx context.Context, workflowRunID, stepRef, decisionType string, limit int, cursor *time.Time) ([]domain.WorkflowStepDecision, error)
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	ListWorkflowVersions(ctx context.Context, workflowID string, limit int) ([]domain.WorkflowVersion, error)
	GetWorkflowVersionByVersionID(ctx context.Context, workflowID, versionID string) (*domain.WorkflowVersion, error)
	UpsertWorkflowPolicy(ctx context.Context, p *domain.WorkflowPolicy) error
	GetWorkflowPolicyByProject(ctx context.Context, projectID string) (*domain.WorkflowPolicy, error)
	CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	ListManagedMachineIDsByWorkflowRun(ctx context.Context, workflowRunID string) ([]string, error)
	BulkCancelWorkflowRuns(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error)
	MarkJobRunsPausedByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
	RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error)
	CountActiveWorkflowRunsByVersion(ctx context.Context, workflowID, versionID string) (int, error)
	CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error)
	ListActiveWorkflowVersions(ctx context.Context, workflowID string) ([]store.ActiveVersion, error)

	// Canary deployments.
	CreateCanaryDeployment(ctx context.Context, canary *domain.CanaryDeployment) error
	GetActiveCanaryDeployment(ctx context.Context, workflowID string) (*domain.CanaryDeployment, error)
	UpdateCanaryDeploymentTraffic(ctx context.Context, workflowID string, trafficPct int) error
	CompleteCanaryDeployment(ctx context.Context, workflowID, status string) error
}

// DeploymentStore handles deployment version lifecycle operations.
type DeploymentStore interface {
	CreateDeploymentVersion(ctx context.Context, deployment *domain.DeploymentVersion) error
	GetDeploymentVersion(ctx context.Context, deploymentID, projectID string) (*domain.DeploymentVersion, error)
	ListDeploymentVersions(ctx context.Context, projectID, environment string, limit int, cursor *time.Time) ([]domain.DeploymentVersion, error)
	FinalizeDeploymentVersion(ctx context.Context, deploymentID, projectID, updatedBy string) (*domain.DeploymentVersion, error)
	PromoteDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error)
	RollbackDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error)
}

// EventTriggerStore handles event trigger operations.
type EventTriggerStore interface {
	CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error
	GetEventTriggerByEventKey(ctx context.Context, eventKey string) (*domain.EventTrigger, error)
	UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	ListEventTriggersByProject(ctx context.Context, projectID, status, workflowRunID, sourceType string, limit int, cursor *time.Time) ([]domain.EventTrigger, error)
	ListEventTriggersByKeyPrefix(ctx context.Context, prefix string, projectID string) ([]domain.EventTrigger, error)
	CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
	ReceiveEventAndRequeueRun(ctx context.Context, triggerID string, payload json.RawMessage, receivedAt time.Time, jobRunID string) error
	SetEventTriggerSentBy(ctx context.Context, id, sentBy string) error
	GetEventTriggerStats(ctx context.Context, projectID string) (*store.EventTriggerStats, error)
	BatchReceiveEventTriggers(ctx context.Context, triggerIDs []string, payload json.RawMessage, receivedAt time.Time, sentBy string) ([]string, error)
	DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
	CountEventTriggersFinishedBefore(ctx context.Context, before time.Time) (int64, error)
	CountActiveEventTriggersByProject(ctx context.Context, projectID string) (int, error)
}

// AuthStore handles API keys and authentication.
type AuthStore interface {
	CreateAPIKey(ctx context.Context, key *domain.APIKey) error
	ListAPIKeysByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error)
	ListAPIKeysByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	GetAPIKeyByID(ctx context.Context, id string) (*domain.APIKey, error)
	MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
	ListRunsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListJobsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.Job, error)
	CountCronJobsByOrg(ctx context.Context, orgID string) (int, error)
	CountEnvironmentsByProject(ctx context.Context, projectID string) (int, error)
	CountWebhookSubscriptionsByProject(ctx context.Context, projectID string) (int, error)
	CreateDeviceCode(ctx context.Context, deviceCode, userCode, projectID string, scopes []string, expiresAt time.Time) error
	GetDeviceCodeByDeviceCode(ctx context.Context, deviceCode string) (*store.DeviceCodeRow, error)
	ApproveDeviceCode(ctx context.Context, deviceCode, apiKeyID, rawAPIKey string) error
	ExchangeDeviceCode(ctx context.Context, deviceCode string) (string, error)
	CleanupExpiredDeviceCodes(ctx context.Context) (int64, error)
}

// RBACStore handles role-based access control.
type RBACStore interface {
	GetUserPermissions(ctx context.Context, projectID, userID string) ([]string, error)
	UserHasProjectAccess(ctx context.Context, userID, projectID string) (bool, error)
	CreateProjectRole(ctx context.Context, role *domain.ProjectRole) error
	GetProjectRole(ctx context.Context, id string) (*domain.ProjectRole, error)
	UpdateProjectRole(ctx context.Context, role *domain.ProjectRole) error
	ListProjectRoles(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.ProjectRole, error)
	DeleteProjectRole(ctx context.Context, id string) error
	AssignMemberRole(ctx context.Context, m *domain.ProjectMemberRole) error
	GetMemberRole(ctx context.Context, projectID, userID string) (*domain.ProjectMemberRole, error)
	RemoveMemberRole(ctx context.Context, projectID, userID string) error
	ListProjectMembers(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.ProjectMemberRole, error)
	SeedProjectSystemRoles(ctx context.Context, projectID string) error
	CreateResourcePolicy(ctx context.Context, p *domain.ResourcePolicy) error
	GetResourcePolicies(ctx context.Context, resourceType, resourceID, userID string) ([]string, error)
	DeleteResourcePolicy(ctx context.Context, id string) (projectID, userID string, err error)
	ListResourcePolicies(ctx context.Context, resourceType, resourceID string, limit int, cursor *time.Time) ([]domain.ResourcePolicy, error)
	CreateTagPolicy(ctx context.Context, p *domain.TagPolicy) error
	ListTagPolicies(ctx context.Context, projectID, resourceType, userID string, limit int, cursor *time.Time) ([]domain.TagPolicy, error)
	DeleteTagPolicy(ctx context.Context, id string) (projectID, userID string, err error)
	GetTagPolicyActions(ctx context.Context, projectID, resourceType, userID string, tags map[string]string) ([]string, error)
	CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
	ListAuditEvents(ctx context.Context, projectID, actorID, resourceType, resourceID string, limit int, cursor, from, to *time.Time, ascending bool) ([]domain.AuditEvent, error)
	StreamAuditEvents(ctx context.Context, projectID, actorID, resourceType string, from, to time.Time, fn func(*domain.AuditEvent) error) error
}

// NotificationChannelStore handles notification channel and delivery operations.
type NotificationChannelStore interface {
	CreateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error
	GetNotificationChannel(ctx context.Context, id, projectID string) (*domain.NotificationChannel, error)
	ListNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error
	DeleteNotificationChannel(ctx context.Context, id, projectID string) error
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
	ListNotificationDeliveries(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error)
}

// ActorSyncer lazily persists actor profile information from request headers.
type ActorSyncer interface {
	UpsertKnownActor(ctx context.Context, id, email, name string) error
}

// Pinger checks service health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// WorkflowCallback is called after a run reaches a terminal state via SDK or cancel.
type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
	OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error
	OnStepFailed(ctx context.Context, workflowRunID string, stepRunID string)
	ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error
	ResumeWorkflowRun(ctx context.Context, workflowRunID string) error
	SkipStep(ctx context.Context, workflowRunID, stepRef, reason, actor string) error
	ForceCompleteStep(ctx context.Context, workflowRunID, stepRef string, result json.RawMessage) error
}

type WorkflowTrigger interface {
	TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error)
	RetryWorkflowRun(ctx context.Context, originalRunID string) (*domain.WorkflowRun, error)
}

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
)

// ErrorResponse is the standard error response returned by all API endpoints.
type ErrorResponse struct {
	Error     any    `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

type APIError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

const (
	ErrorCodeValidationError = "validation_error"
	ErrorCodeNotFound        = "not_found"
	ErrorCodeConflict        = "conflict"
	ErrorCodeRateLimited     = "rate_limited"
	ErrorCodeInternalError   = "internal_error"
	ErrorCodeUnauthorized    = "unauthorized"
	ErrorCodeForbidden       = "forbidden"
)

// AnalyticsStore is the subset of analytics query methods that can be backed
// by either Postgres (store.Queries) or ClickHouse (clickhouse.AnalyticsStore).
//
//go:generate moq -stub -out mock_analyticsstore_test.go -pkg api . AnalyticsStore
type AnalyticsStore interface {
	GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*store.PerformanceAnalytics, error)
	GetCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.CostAnalytics, error)
	GetCostTrends(ctx context.Context, projectID string, from, to time.Time) ([]store.CostTrendPoint, error)
	GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopCostItem, error)
	GetComputeCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.ComputeCostAnalytics, error)
	GetCostOutliers(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]store.CostOutlier, error)
	GetApprovalStats(ctx context.Context, projectID string, from, to time.Time) (*store.ApprovalStats, error)

	// Run analytics
	GetRunTimeline(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.RunTimelineBucket, error)
	GetRunDurationDistribution(ctx context.Context, projectID string, from, to time.Time) ([]store.RunDurationBucket, error)
	GetRunFailureReasons(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.RunFailureReason, error)
	GetRunSummary(ctx context.Context, projectID string, from, to time.Time) (*store.RunSummary, error)
	GetRunsByTrigger(ctx context.Context, projectID string, from, to time.Time) ([]store.RunsByTrigger, error)

	// Job analytics
	GetJobHistory(ctx context.Context, projectID, jobID string, from, to time.Time, bucket string) ([]store.JobHistoryBucket, error)
	GetJobComparison(ctx context.Context, projectID string, jobIDs []string, from, to time.Time) ([]store.JobComparison, error)
	GetJobReliability(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobReliability, error)
	GetRunsByVersion(ctx context.Context, projectID, jobID string, from, to time.Time) ([]store.RunsByVersion, error)
	GetJobCostRanking(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.JobCostRanking, error)
	GetTopFailingJobs(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingJob, error)

	// Tag analytics
	GetTagSummary(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagSummary, error)
	GetTopFailingTags(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingTag, error)
	GetTagCost(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TagCost, error)

	// Workflow analytics
	GetWorkflowStepDurations(ctx context.Context, projectID, workflowID string, from, to time.Time) ([]store.StepDuration, error)
	GetWorkflowCompletionRates(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WorkflowCompletionBucket, error)
	GetWorkflowSummary(ctx context.Context, projectID string, from, to time.Time) (*store.WorkflowSummary, error)

	// Webhook analytics
	GetWebhookDeliveryStats(ctx context.Context, projectID string, from, to time.Time) ([]store.WebhookEndpointStats, error)
	GetWebhookEndpointHealth(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.WebhookHealthBucket, error)
	GetTopFailingWebhooks(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopFailingEndpoint, error)

	// Event analytics
	GetEventVolume(ctx context.Context, projectID string, from, to time.Time, bucket string) ([]store.EventVolumeBucket, error)
	GetEventLatency(ctx context.Context, projectID string, from, to time.Time) (*store.EventLatencyStats, error)

	// Cost analytics (new)
	GetCostForecast(ctx context.Context, projectID string, from, to time.Time) (*store.CostForecast, error)
	GetCostByTrigger(ctx context.Context, projectID string, from, to time.Time) ([]store.CostByTrigger, error)
	GetCostByMachine(ctx context.Context, projectID string, from, to time.Time) ([]store.CostByMachine, error)
}

type Server struct {
	router             chi.Router
	store              APIStore
	analyticsStore     AnalyticsStore
	queue              queue.Queue
	pubsub             pubsub.Publisher
	config             *config.Config
	metrics            *telemetry.Metrics
	metricsHandler     http.Handler
	pinger             Pinger
	healthRegistry     *health.Registry
	workflowCallback   WorkflowCallback
	workflowEngine     WorkflowTrigger
	txPool             store.TxBeginner
	actorSyncer        ActorSyncer
	validate           *validator.Validate
	maxRequestBodySize int64
	poolStatter        PoolStatter
	permCache          *permissionCache
	oidcVerifier       *oidcVerifier
	bgPool             pond.Pool // bounded pool for fire-and-forget background tasks (API key touch, actor sync)
	runInTx            func(ctx context.Context, fn func(s APIStore) error) error
	rateLimiter        *ratelimit.RedisRateLimiter
	encryptor          Encryptor
	containerRuntime   compute.ContainerRuntime
	polarWebhook       http.Handler
	billingEnforcer    BillingEnforcer
	usageService       UsageService
	referralService    ReferralService
	chExporter         *clickhouse.Exporter
	edition            domain.Edition
	version            string
	startedAt          time.Time
	cdcWebhookReceiver http.Handler
	cachedOpenAPISpec  []byte
}

// analytics returns the ClickHouse analytics store when available, falling back to Postgres.
func (s *Server) analytics() AnalyticsStore {
	if s.analyticsStore != nil {
		return s.analyticsStore
	}
	if as, ok := s.store.(AnalyticsStore); ok {
		return as
	}
	return nil
}

// requireAnalytics returns the analytics store or an error suitable for huma
// handlers when no analytics backend is configured.
func (s *Server) requireAnalytics() (AnalyticsStore, error) {
	a := s.analytics()
	if a == nil {
		return nil, huma.Error503ServiceUnavailable("analytics store unavailable")
	}
	return a, nil
}

// Encryptor encrypts and decrypts byte slices (used for event source signature secrets).
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// BillingEnforcer checks org-level billing limits.
type BillingEnforcer interface {
	CheckProjectLimit(ctx context.Context, orgID string) error
	CheckMemberLimit(ctx context.Context, orgID string) error
	CheckOrgCreationLimit(ctx context.Context, userID string, planTier domain.PlanTier) error
	CheckProjectBudgetLimit(ctx context.Context, projectID string) error
	GetProjectOrgID(ctx context.Context, projectID string) (string, error)
	GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error)
	GetOrgPlanLimits(ctx context.Context, orgID string) (billing.OrgPlanLimits, error)
	EnsureOrgSubscription(ctx context.Context, orgID string) error
}

// UsageService provides org usage data for the billing dashboard.
type UsageService interface {
	GetCurrentUsage(ctx context.Context, orgID string) (*billing.CurrentUsageResponse, error)
	GetUsageHistory(ctx context.Context, orgID string, from, to time.Time) ([]billing.UsageHistoryEntry, error)
	GetUsageForecast(ctx context.Context, orgID string) (*billing.UsageForecastResponse, error)
	GetProjectCosts(ctx context.Context, orgID string, from, to time.Time) ([]billing.ProjectCostEntry, error)
	ExportUsageCSV(ctx context.Context, orgID string, from, to time.Time) ([]byte, error)
	ExportUsagePDF(ctx context.Context, orgID string, from, to time.Time) ([]byte, error)
	GetSpendingLimit(ctx context.Context, orgID string) (*billing.SpendingLimitResponse, error)
	SetSpendingLimit(ctx context.Context, orgID string, limitMicrousd int64, action string) error
	PreviewDowngrade(ctx context.Context, orgID string, targetTier domain.PlanTier) (*billing.DowngradeImpact, error)
	DetectAnomalies(ctx context.Context, orgID string) ([]billing.AnomalyAlert, error)
	GetProjectBudget(ctx context.Context, projectID string) (*billing.ProjectBudgetResponse, error)
	SetProjectBudget(ctx context.Context, projectID string, budgetMicro int64, action string) error
	GetAnomalyConfig(ctx context.Context, orgID string) (*billing.AnomalyConfigResponse, error)
	SetAnomalyConfig(ctx context.Context, orgID string, warning, critical float64) error
	GetEmailPreferences(ctx context.Context, orgID string) (*billing.EmailPreferencesResponse, error)
	UpdateEmailPreferences(ctx context.Context, orgID string, enabled bool) error
}

// ReferralService handles referral code management.
type ReferralService interface {
	GenerateCode(ctx context.Context, orgID string) (*billing.Referral, error)
	ActivateReferral(ctx context.Context, code, referredOrgID, referredEmail string) (*billing.Referral, error)
	ListReferrals(ctx context.Context, orgID string) ([]billing.Referral, error)
	AutoActivateReferral(ctx context.Context, orgID string) error
}

// ServerDeps holds all dependencies required to construct a Server.
type ServerDeps struct {
	Config             *config.Config
	Store              APIStore
	AnalyticsStore     AnalyticsStore // Optional: ClickHouse-backed analytics queries.
	Queue              queue.Queue
	PubSub             pubsub.Publisher
	MetricsHandler     http.Handler
	Pinger             Pinger
	HealthRegistry     *health.Registry
	WorkflowCallback   WorkflowCallback
	WorkflowEngine     WorkflowTrigger
	Metrics            *telemetry.Metrics
	TxPool             store.TxBeginner // Optional: enables transactional event trigger sends.
	ActorSyncer        ActorSyncer
	PoolStatter        PoolStatter              // Optional: enables DB pool backpressure middleware.
	RedisClient        *redis.Client            // Optional: enables per-project/key rate limiting.
	Encryptor          Encryptor                // Optional: enables event source signature encryption.
	ContainerRuntime   compute.ContainerRuntime // Optional: enables managed container stop on cancel.
	PolarWebhook       http.Handler             // Optional: Polar billing webhook handler.
	BillingEnforcer    BillingEnforcer          // Optional: enables billing limit checks on project create.
	UsageService       UsageService             // Optional: enables usage endpoint.
	ReferralService    ReferralService          // Optional: enables referral endpoints.
	CHExporter         *clickhouse.Exporter     // Optional: enables ClickHouse analytics export from API handlers.
	Edition            domain.Edition           // Edition controls feature gating (community vs cloud).
	Version            string                   // Build version (injected via ldflags).
	CDCWebhookReceiver http.Handler             // Optional: enables CDC webhook push endpoint.
}

// PoolStatter provides connection pool statistics for backpressure.
type PoolStatter interface {
	AcquiredConns() int32
	MaxConns() int32
}

// NewServer creates a new HTTP API server with the given dependencies.
func NewServer(deps ServerDeps) *Server {
	maxBody := deps.Config.MaxRequestBodySize
	if maxBody <= 0 {
		maxBody = 1 << 20 // 1MB default
	}

	verifier, err := newOIDCVerifier(deps.Config)
	if err != nil {
		slog.Warn("failed to initialize OIDC verifier; disabling OIDC auth", "error", err)
		verifier = &oidcVerifier{enabled: false}
	}

	srv := &Server{
		store:              deps.Store,
		analyticsStore:     deps.AnalyticsStore,
		queue:              deps.Queue,
		pubsub:             deps.PubSub,
		config:             deps.Config,
		metrics:            deps.Metrics,
		metricsHandler:     deps.MetricsHandler,
		pinger:             deps.Pinger,
		healthRegistry:     deps.HealthRegistry,
		workflowCallback:   deps.WorkflowCallback,
		workflowEngine:     deps.WorkflowEngine,
		txPool:             deps.TxPool,
		actorSyncer:        deps.ActorSyncer,
		validate:           validator.New(validator.WithRequiredStructEnabled()),
		maxRequestBodySize: maxBody,
		poolStatter:        deps.PoolStatter,
		permCache:          newPermissionCache(permCacheTTL(deps.Config)),
		oidcVerifier:       verifier,
		bgPool:             pond.NewPool(4),
		rateLimiter:        ratelimit.NewRedisRateLimiter(deps.RedisClient, deps.RedisClient != nil),
		encryptor:          deps.Encryptor,
		containerRuntime:   deps.ContainerRuntime,
		polarWebhook:       deps.PolarWebhook,
		billingEnforcer:    deps.BillingEnforcer,
		usageService:       deps.UsageService,
		referralService:    deps.ReferralService,
		chExporter:         deps.CHExporter,
		edition:            deps.Edition,
		version:            deps.Version,
		startedAt:          time.Now(),
		cdcWebhookReceiver: deps.CDCWebhookReceiver,
	}

	globalAllowPrivateEndpoints.Store(deps.Config != nil && deps.Config.AllowPrivateEndpoints)

	if deps.TxPool != nil {
		srv.runInTx = func(ctx context.Context, fn func(s APIStore) error) error {
			return store.WithTx(ctx, deps.TxPool, func(q *store.Queries) error {
				return fn(q)
			})
		}
	} else {
		srv.runInTx = func(_ context.Context, fn func(s APIStore) error) error {
			return fn(srv.store)
		}
	}

	srv.router = srv.routes()
	return srv
}

func (s *Server) dbBackpressure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acquired := s.poolStatter.AcquiredConns()
		maxConns := s.poolStatter.MaxConns()
		if maxConns > 0 && acquired > int32(float64(maxConns)*0.9) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func permCacheTTL(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.PermissionCacheTTL > 0 {
		return cfg.PermissionCacheTTL
	}
	return 30 * time.Second
}

// Close releases resources held by the server (e.g. background goroutines).
// Call this when shutting down.
func (s *Server) Close() {
	if s.permCache != nil {
		s.permCache.Stop()
	}
	if s.bgPool != nil {
		s.bgPool.StopAndWait()
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":         "ok",
		"edition":        string(s.edition),
		"version":        s.version,
		"uptime_seconds": int(time.Since(s.startedAt).Seconds()),
	}

	if s.healthRegistry != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		result := s.healthRegistry.CheckAll(ctx)

		subsystems := make(map[string]string)
		for _, c := range result.Components {
			subsystems[c.Name] = string(c.Status)
		}
		resp["subsystems"] = subsystems

		if result.Status != health.StatusUp {
			resp["status"] = string(result.Status)
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	if s.healthRegistry != nil {
		result := s.healthRegistry.CheckAll(r.Context())
		if result.Status == health.StatusDown {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			if err := json.NewEncoder(w).Encode(result); err != nil {
				slog.Warn("failed to encode health check response", "error", err)
			}
			return
		}
		respondJSON(w, http.StatusOK, result)
		return
	}

	_, err := s.store.QueueStats(r.Context())
	if err != nil {
		respondError(w, r, http.StatusServiceUnavailable, "database not ready")
		return
	}

	if s.pinger != nil {
		if err := s.pinger.Ping(r.Context()); err != nil {
			respondError(w, r, http.StatusServiceUnavailable, "redis not ready")
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}

func respondError(w http.ResponseWriter, r *http.Request, status int, errInput any) {
	var requestID string
	if r != nil {
		requestID = chimw.GetReqID(r.Context())
	}

	errorBody := normalizeAPIError(status, errInput)
	respondJSON(w, status, ErrorResponse{
		Error:     errorBody,
		RequestID: requestID,
	})
}

func normalizeAPIError(status int, errInput any) any {
	switch v := errInput.(type) {
	case APIError:
		if v.Code == "" {
			v.Code = defaultErrorCode(status)
		}
		if v.Message == "" {
			v.Message = http.StatusText(status)
		}
		return v
	case *APIError:
		if v == nil {
			return http.StatusText(status)
		}
		apiErr := *v
		if apiErr.Code == "" {
			apiErr.Code = defaultErrorCode(status)
		}
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(status)
		}
		return apiErr
	case error:
		if v == nil {
			return http.StatusText(status)
		}
		return v.Error()
	case string:
		if v == "" {
			return http.StatusText(status)
		}
		return v
	default:
		return http.StatusText(status)
	}
}

func defaultErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return ErrorCodeValidationError
	case http.StatusUnauthorized:
		return ErrorCodeUnauthorized
	case http.StatusForbidden:
		return ErrorCodeForbidden
	case http.StatusNotFound:
		return ErrorCodeNotFound
	case http.StatusConflict:
		return ErrorCodeConflict
	case http.StatusTooManyRequests:
		return ErrorCodeRateLimited
	default:
		return ErrorCodeInternalError
	}
}

func (s *Server) decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(&nullByteStrippingReader{r: io.LimitReader(r.Body, s.maxRequestBodySize)})
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func validateURL(rawURL string) error {
	if err := worker.ValidateEndpointURL(rawURL); err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "URL") {
			msg = "url" + msg[3:]
		}
		return errors.New(msg)
	}

	// ALLOW_PRIVATE_ENDPOINTS is checked at startup via config; the flag is
	// stored on the Server struct. For the package-level validateURL (called
	// from handlers that have no server reference), we skip the network
	// checks only when the global was set by the last NewServer call.
	// This is safe because in production there is exactly one Server instance.
	if globalAllowPrivateEndpoints.Load() {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := u.Hostname()
	blockedHosts := []string{"localhost", "metadata.google.internal", "169.254.169.254"}
	for _, blocked := range blockedHosts {
		if strings.EqualFold(host, blocked) {
			return fmt.Errorf("url must not point to internal services")
		}
	}

	ip := net.ParseIP(host)
	if ip == nil {
		ips, lookupErr := net.LookupIP(host)
		if lookupErr == nil && slices.ContainsFunc(ips, isPrivateIP) {
			return fmt.Errorf("url must not point to private or loopback addresses")
		}
	}

	if port := u.Port(); port != "" && port != "80" && port != "443" {
		portNum, convErr := strconv.Atoi(port)
		if convErr != nil || portNum < 1 || portNum > 65535 {
			return fmt.Errorf("invalid port number")
		}

		allowedPorts := map[int]bool{80: true, 443: true, 8080: true, 8443: true, 3000: true, 4000: true, 5000: true, 9000: true}
		if !allowedPorts[portNum] {
			return fmt.Errorf("port %d is not allowed for webhooks", portNum)
		}
	}

	return nil
}

func validateURLWithTLS(rawURL string, requireTLS bool) error {
	if err := worker.ValidateEndpointURLWithTLS(rawURL, requireTLS); err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "URL") {
			msg = "url" + msg[3:]
		}
		return errors.New(msg)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := u.Hostname()
	blockedHosts := []string{"localhost", "metadata.google.internal", "169.254.169.254"}
	for _, blocked := range blockedHosts {
		if strings.EqualFold(host, blocked) {
			return fmt.Errorf("url must not point to internal services")
		}
	}

	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func (s *Server) handleAPIReference(w http.ResponseWriter, r *http.Request) {
	// Serve Scalar API reference using the cached OpenAPI spec.
	htmlContent, err := scalar.ApiReferenceHTML(&scalar.Options{
		SpecURL:     "/reference/openapi.json",
		SpecContent: "{}",
		CustomOptions: scalar.CustomOptions{
			PageTitle: "Strait API",
		},
		DarkMode: true,
	})
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to generate API reference")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlContent)
}

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	// Serve the cached OpenAPI spec as JSON.
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(s.cachedOpenAPISpec)
}
