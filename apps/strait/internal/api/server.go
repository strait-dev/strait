package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"strait/internal/billing"
	straitcache "strait/internal/cache"
	"strait/internal/clickhouse"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/health"
	"strait/internal/httputil"
	"strait/internal/logdrain"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/ratelimit"
	"strait/internal/store"
	"strait/internal/telemetry"
	"strait/internal/worker"
	"strait/schemas"

	"sync"
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
	WorkerStore
}

// WorkerStore handles worker read operations needed by the API.
type WorkerStore interface {
	GetWorker(ctx context.Context, workerID, projectID string) (*domain.Worker, error)
	ListWorkers(ctx context.Context, projectID, queueName string, limit, offset int) ([]domain.Worker, error)
	ListWorkerTasksByWorker(ctx context.Context, workerID, projectID string, status domain.WorkerTaskStatus, limit, offset int) ([]domain.WorkerTask, error)
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
	BatchUpdateJobsEnabled(ctx context.Context, ids []string, enabled bool, projectID string) (int64, error)
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
	GetEnvironment(ctx context.Context, id, projectID string) (*domain.Environment, error)
	ListEnvironments(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error)
	UpdateEnvironment(ctx context.Context, env *domain.Environment) error
	DeleteEnvironment(ctx context.Context, id, projectID string) error
	GetResolvedEnvironmentVariables(ctx context.Context, projectID, id string) (map[string]string, error)
	CreateJobSecret(ctx context.Context, secret *domain.JobSecret) error
	ListJobSecrets(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error)
	GetJobSecret(ctx context.Context, id, projectID string) (*domain.JobSecret, error)
	DeleteJobSecret(ctx context.Context, id, projectID string) error
	CreateJobDependency(ctx context.Context, dep *domain.JobDependency) error
	GetJobDependency(ctx context.Context, id string) (*domain.JobDependency, error)
	ListJobDependencies(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error)
	DeleteJobDependency(ctx context.Context, id string) error
	AreJobDependenciesSatisfied(ctx context.Context, run *domain.JobRun) (bool, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	UpdateProjectDefaultRegion(ctx context.Context, projectID, defaultRegion string) error
	UpdateProjectMaxKeyLifetimeDays(ctx context.Context, projectID string, days int) error
	ListAPIKeysExpiringSoon(ctx context.Context, projectID string, withinDays int) ([]domain.APIKey, error)
	PauseJob(ctx context.Context, id, reason string) error
	ResumeJob(ctx context.Context, id string) error
	UpdateJobEndpoint(ctx context.Context, jobID, projectID, endpointURL, fallbackURL, signingSecret string) error
}

// RunStore keeps the existing API store contract while grouping run-related
// operations by workflow. The smaller embedded interfaces make handler
// dependencies easier to scan without changing the method set.
type RunStore interface {
	RunLifecycleStore
	RunEventStore
	RunWebhookStore
	RunAnalyticsStore
	RunIdempotencyStore
	RunBatchStore
	RunStateStore
	RunResourceStore
	RunMemoryStore
}

// RunLifecycleStore handles run lookup, creation, transitions, replay, and DLQ
// operations.
type RunLifecycleStore interface {
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetRunStatus(ctx context.Context, id string) (domain.RunStatus, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	ListRunsByProject(
		ctx context.Context,
		projectID string,
		status *domain.RunStatus,
		metadataKey, metadataValue, triggeredBy, batchID *string,
		payloadContains json.RawMessage,
		executionMode *domain.ExecutionMode,
		errorClass *string,
		limit int,
		cursor *time.Time,
	) ([]domain.JobRun, error)
	ListRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListDeadLetterRunsFiltered(ctx context.Context, projectID string, jobID *string, masked *bool, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListChildRuns(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListRunLineage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	BulkReplayDeadLetterRuns(ctx context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error
	UpdateRunDebugMode(ctx context.Context, runID string, debugMode bool) error
	ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error)
	ReplayDeadLetterRunWithAudit(ctx context.Context, runID string, audit *domain.AuditEvent) (*domain.JobRun, error)
	UnmaskDLQRun(ctx context.Context, runID string) error
	PurgeDLQRun(ctx context.Context, runID string) error
	MarkRunReplayed(ctx context.Context, originalRunID, replayedByRunID string) error
	AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error)
	UpdateHeartbeat(ctx context.Context, id string) error
	GetDebugBundle(ctx context.Context, runID string) (*domain.DebugBundle, error)
	RescheduleRun(ctx context.Context, runID string, scheduledAt time.Time, payload json.RawMessage) error
	GetRunsByIDs(ctx context.Context, ids []string) (map[string]*domain.JobRun, error)
	BulkCancelRuns(ctx context.Context, ids []string, finishedAt time.Time, reason string) ([]store.BulkCancelResult, error)
	CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error)
	ResetRunIdempotencyKey(ctx context.Context, runID string) error
}

// RunEventStore handles event, checkpoint, and output data.
type RunEventStore interface {
	CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	ListRunCheckpoints(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error)
	UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error
	ListRunOutputs(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
}

// RunWebhookStore handles webhook subscriptions and delivery retries.
type RunWebhookStore interface {
	ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error)
	GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	RetryWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ReplayWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	CreateWebhookSubscription(ctx context.Context, sub *domain.WebhookSubscription) error
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	GetWebhookSubscription(ctx context.Context, id string) (*domain.WebhookSubscription, error)
	DeleteWebhookSubscription(ctx context.Context, id string) error
	RotateWebhookSecret(ctx context.Context, id, newSecret string, graceExpiresAt time.Time) error
	GetWebhookSubscriptionSecrets(ctx context.Context, subscriptionID string) (string, string, *time.Time, error)
}

// RunAnalyticsStore handles run, queue, cost, and approval analytics.
type RunAnalyticsStore interface {
	SumRunCostMicrousd(ctx context.Context, runID string) (int64, error)
	SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
	QueueStats(ctx context.Context) (*store.QueueStats, error)
	GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*store.PerformanceAnalytics, error)
	GetCostAnalytics(ctx context.Context, projectID string, from, to time.Time) (*store.CostAnalytics, error)
	GetCostTrends(ctx context.Context, projectID string, from, to time.Time) ([]store.CostTrendPoint, error)
	GetTopCosts(ctx context.Context, projectID string, from, to time.Time, limit int) ([]store.TopCostItem, error)
	GetApprovalStats(ctx context.Context, projectID string, from, to time.Time) (*store.ApprovalStats, error)
	GetCostOutliers(ctx context.Context, projectID string, from, to time.Time, threshold float64) ([]store.CostOutlier, error)
	AggregateCostStatsHourly(ctx context.Context, hour time.Time) error
}

// RunIdempotencyStore handles general-purpose idempotency keys that are not
// tied to a specific run row.
type RunIdempotencyStore interface {
	TryAcquireIdempotencyKey(ctx context.Context, projectID, key string, ttl time.Duration) (string, int, http.Header, []byte, error)
	CompleteIdempotencyKey(ctx context.Context, projectID, key string, responseStatus int, responseHeaders http.Header, responseBody []byte) error
	DeleteIdempotencyKey(ctx context.Context, projectID, key string) (int64, error)
}

// RunBatchStore handles batch operations, debouncing, and bulk cancellation.
type RunBatchStore interface {
	CreateBatchOperation(ctx context.Context, op *domain.BatchOperation) error
	FinalizeBatchOperation(ctx context.Context, batchID string, createdCount int) error
	GetBatchOperation(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error)
	ListBatchOperations(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.BatchOperation, error)
	BulkCancelByFilter(ctx context.Context, projectID string, f store.BulkCancelFilter, now time.Time, reason string) ([]string, error)
	UpsertDebouncePending(ctx context.Context, d *domain.DebouncePending) error
	InsertBatchBufferItem(ctx context.Context, item *domain.BatchBufferItem) error
	CountBatchBufferItems(ctx context.Context, jobID, batchKey string) (int, error)
	DrainBatchBuffer(ctx context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error)
}

// RunStateStore handles SDK run state documents.
type RunStateStore interface {
	UpsertRunState(ctx context.Context, s *domain.RunState) error
	GetRunState(ctx context.Context, runID, key string) (*domain.RunState, error)
	ListRunState(ctx context.Context, runID string) ([]domain.RunState, error)
	DeleteRunState(ctx context.Context, runID, key string) error
}

// RunResourceStore handles resource snapshots, iterations, and counters.
type RunResourceStore interface {
	CreateRunResourceSnapshot(ctx context.Context, snapshot *domain.RunResourceSnapshot) error
	ListRunResourceSnapshots(ctx context.Context, runID string, from, to *time.Time, limit int) ([]domain.RunResourceSnapshot, error)
	CountRunIterations(ctx context.Context, runID string) (int, error)
	CreateRunIteration(ctx context.Context, iter *domain.RunIteration) error
}

// RunMemoryStore handles per-job memory records used by SDK workflows.
type RunMemoryStore interface {
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
	GetEventSubscription(ctx context.Context, subID string) (*domain.EventSubscription, error)
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
	ListWorkflowStepDecisions(
		ctx context.Context,
		workflowRunID, stepRef, decisionType string,
		limit int,
		cursor *time.Time,
	) ([]domain.WorkflowStepDecision, error)
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	ListWorkflowVersions(ctx context.Context, workflowID string, limit int) ([]domain.WorkflowVersion, error)
	GetWorkflowVersionByVersionID(ctx context.Context, workflowID, versionID string) (*domain.WorkflowVersion, error)
	UpsertWorkflowPolicy(ctx context.Context, p *domain.WorkflowPolicy) error
	GetWorkflowPolicyByProject(ctx context.Context, projectID string) (*domain.WorkflowPolicy, error)
	CancelNonTerminalStepRuns(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
	CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error)
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
	GetEventTriggerByEventKeyForProject(ctx context.Context, eventKey, projectID string) (*domain.EventTrigger, error)
	UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	UpdateEventTriggerStatusFrom(
		ctx context.Context,
		id string,
		from string,
		status string,
		responsePayload json.RawMessage,
		receivedAt *time.Time,
		errMsg string,
	) error
	ListEventTriggersByProject(
		ctx context.Context,
		projectID, environmentID, status, workflowRunID, sourceType string,
		limit int,
		cursor *time.Time,
	) ([]domain.EventTrigger, error)
	ListEventTriggersByKeyPrefix(ctx context.Context, prefix string, projectID string) ([]domain.EventTrigger, error)
	CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
	ReceiveEventAndRequeueRun(ctx context.Context, triggerID string, payload json.RawMessage, receivedAt time.Time, jobRunID string) error
	SetEventTriggerSentBy(ctx context.Context, id, sentBy string) error
	GetEventTriggerStats(ctx context.Context, projectID, environmentID string) (*store.EventTriggerStats, error)
	BatchReceiveEventTriggers(ctx context.Context, triggerIDs []string, payload json.RawMessage, receivedAt time.Time, sentBy string) ([]string, error)
	DeleteEventTriggersFinishedBeforeForProject(ctx context.Context, projectID, environmentID string, before time.Time, limit int) (int64, error)
	CountEventTriggersFinishedBeforeForProject(ctx context.Context, projectID, environmentID string, before time.Time) (int64, error)
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
	CreateRotatedAPIKey(ctx context.Context, oldKeyID string, newKey *domain.APIKey, graceExpiresAt time.Time) error
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
	ListRunsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListJobsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.Job, error)
	CountCronJobsByOrg(ctx context.Context, orgID string) (int, error)
	CountEnvironmentsByProject(ctx context.Context, projectID string) (int, error)
	CountEnvironmentsByOrg(ctx context.Context, orgID string) (int, error)
	CountWebhookSubscriptionsByProject(ctx context.Context, projectID string) (int, error)
	CountWebhookSubscriptionsByOrg(ctx context.Context, orgID string) (int, error)
	CountLogDrainsByOrg(ctx context.Context, orgID string) (int, error)
	CountNotificationChannelsByProject(ctx context.Context, projectID string) (int, error)
	CreateDeviceCode(ctx context.Context, deviceCode, userCode, projectID string, scopes []string, expiresAt time.Time) error
	GetDeviceCodeByDeviceCode(ctx context.Context, deviceCode string) (*store.DeviceCodeRow, error)
	GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*store.DeviceCodeRow, error)
	ApproveDeviceCode(ctx context.Context, deviceCode, apiKeyID, rawAPIKey, projectID string, scopes []string) error
	ApproveDeviceCodeByUserCode(ctx context.Context, userCode, apiKeyID, rawAPIKey, projectID string, scopes []string) error
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
	GetResourcePolicies(ctx context.Context, projectID, resourceType, resourceID, userID string) ([]string, error)
	DeleteResourcePolicy(ctx context.Context, projectID, id string) (deletedProjectID, userID string, err error)
	ListResourcePolicies(ctx context.Context, projectID, resourceType, resourceID string, limit int, cursor *time.Time) ([]domain.ResourcePolicy, error)
	CreateTagPolicy(ctx context.Context, p *domain.TagPolicy) error
	ListTagPolicies(ctx context.Context, projectID, resourceType, userID string, limit int, cursor *time.Time) ([]domain.TagPolicy, error)
	DeleteTagPolicy(ctx context.Context, projectID, id string) (deletedProjectID, userID string, err error)
	GetTagPolicyActions(ctx context.Context, projectID, resourceType, userID string, tags map[string]string) ([]string, error)
	CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
	CreateAuditEventDeadletter(ctx context.Context, ev *domain.AuditEvent, lastErr string, retryCount int) error
	CountAuditEventsDeadletter(ctx context.Context) (int64, error)
	ListAuditEventsDeadletterByProject(ctx context.Context, projectID string, limit int, cursor string) ([]domain.AuditEvent, []string, []string, error)
	GetAuditEventDeadletter(ctx context.Context, id, projectID string) (*domain.AuditEvent, error)
	DeleteAuditEventDeadletter(ctx context.Context, id, projectID string) error
	MarkAuditDeadletterReclaimed(ctx context.Context, dlqID, newEventID string) error
	ListAuditEvents(
		ctx context.Context,
		projectID, actorID, resourceType, resourceID string,
		limit int,
		cursor, from, to *time.Time,
		ascending bool,
	) ([]domain.AuditEvent, error)
	GetAuditEvent(ctx context.Context, projectID, id string) (*domain.AuditEvent, error)
	StreamAuditEvents(ctx context.Context, projectID, actorID, resourceType string, from, to time.Time, fn func(*domain.AuditEvent) error) error
	VerifyAuditChain(ctx context.Context, projectID string) (*domain.AuditChainVerification, error)
	VerifyAuditChainIncremental(ctx context.Context, projectID string) (*domain.AuditChainVerification, error)
	GetAuditExportRowCap(ctx context.Context, projectID string) (int64, error)
	SetAuditExportRowCap(ctx context.Context, projectID string, rowCap int64) error
	GetAuditRetentionDays(ctx context.Context, projectID string) (int, bool, error)
	SetAuditRetentionDays(ctx context.Context, projectID string, days int) error
	RotateAuditSigningKey(ctx context.Context, projectID, actorID string) (int, error)

	// Data export streaming.
	StreamJobs(ctx context.Context, projectID string, fn func(*domain.Job) error) error
	StreamRuns(ctx context.Context, projectID string, from, to time.Time, fn func(*domain.JobRun) error) error
	StreamWorkflows(ctx context.Context, projectID string, fn func(*domain.Workflow) error) error
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
	TriggerWorkflow(
		ctx context.Context,
		workflowID, projectID string,
		payload json.RawMessage,
		triggeredBy string,
		stepOverrides []domain.StepOverride,
		extraTags map[string]string,
	) (*domain.WorkflowRun, error)
	RetryWorkflowRun(ctx context.Context, originalRunID string) (*domain.WorkflowRun, error)
}

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
)

// ErrorResponse is the canonical error envelope returned by every API
// endpoint. The OpenAPI spec is generated from this exact shape -- see
// huma_error.go for the Huma override that wires it in.
type ErrorResponse struct {
	Error     *APIError `json:"error" doc:"Structured error payload"`
	RequestID string    `json:"request_id,omitempty" doc:"Server-assigned request identifier; useful for support and log correlation"`
}

// APIError describes a single error with a machine-readable code, a
// human-readable message, and optional supplemental detail strings.
type APIError struct {
	Code string `json:"code" enum:"bad_request,authentication_required,forbidden,not_found,conflict,validation_failed,rate_limited,enqueue_throttled,internal_error,service_unavailable" doc:"Canonical Strait error code"`

	Message string   `json:"message" doc:"Human-readable error message"`
	Details []string `json:"details,omitempty" doc:"Optional supplemental error details (e.g. per-field validation messages)"`
}

// Canonical error codes. Each maps 1:1 to a default HTTP status via
// defaultErrorCode -- see also the OpenAPI enum on APIError.Code.
const (
	ErrorCodeBadRequest             = "bad_request"
	ErrorCodeAuthenticationRequired = "authentication_required"
	ErrorCodeForbidden              = "forbidden"
	ErrorCodeNotFound               = "not_found"
	ErrorCodeConflict               = "conflict"
	ErrorCodeValidationFailed       = "validation_failed"
	ErrorCodeRateLimited            = "rate_limited"
	ErrorCodeEnqueueThrottled       = "enqueue_throttled"
	ErrorCodeInternalError          = "internal_error"
	ErrorCodeServiceUnavailable     = "service_unavailable"
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
}

type Server struct {
	router                     chi.Router
	store                      APIStore
	outboxAdminStore           outboxAdminStore
	analyticsStore             AnalyticsStore
	queue                      queue.Queue
	pubsub                     pubsub.Publisher
	config                     *config.Config
	metrics                    *telemetry.Metrics
	metricsHandler             http.Handler
	pinger                     Pinger
	healthRegistry             *health.Registry
	workflowCallback           WorkflowCallback
	workflowEngine             WorkflowTrigger
	txPool                     store.TxBeginner
	actorSyncer                ActorSyncer
	validate                   *validator.Validate
	maxRequestBodySize         int64
	poolStatter                PoolStatter
	poolBackpressure           *poolBackpressureSampler
	permCache                  *permissionCache
	quotaCache                 *quotaCache
	apiKeyCache                *apiKeyCache
	jobDependencyCache         *jobDependencyCache
	cacheBus                   *straitcache.Bus
	workerJobBarrier           *straitcache.Tier[string, struct{}]
	runStatusReadModel         *straitcache.ReadModel[*domain.JobRun]
	workflowRunStatusReadModel *straitcache.ReadModel[*domain.WorkflowRun]
	oidcVerifier               *oidcVerifier
	bgPool                     pond.Pool // bounded pool for fire-and-forget background tasks (API key touch, actor sync)
	runInTx                    func(ctx context.Context, fn func(s APIStore) error) error
	rateLimiter                *ratelimit.RedisRateLimiter
	authLimiter                *ratelimit.AuthLimiter
	encryptor                  Encryptor
	stripeWebhook              http.Handler
	billingEnforcer            BillingEnforcer
	usageService               UsageService
	chExporter                 *clickhouse.Exporter
	edition                    domain.Edition
	version                    string
	startedAt                  time.Time
	cdcWebhookReceiver         http.Handler
	cachedOpenAPISpec          []byte
	cachedOpenAPISpecGzip      []byte

	// trustedProxies is the parsed CIDR list of reverse proxies whose
	// X-Forwarded-For header is trusted for client IP attribution. Empty
	// means XFF is ignored entirely (fail-safe default).
	trustedProxies []net.IPNet

	// profilingAllowedCIDRs is an optional allowlist for /debug/pprof.
	// Empty means any authenticated caller may access pprof.
	profilingAllowedCIDRs []net.IPNet

	// SSE connection limiters to prevent goroutine/connection exhaustion.
	sseGlobalConns  atomic.Int64
	sseProjectConns sync.Map // map[string]*atomic.Int64

	// Async audit event drain for hot-path handlers (job trigger, bulk trigger).
	// A single goroutine reads from auditAsyncCh and writes events sequentially
	// via store.CreateAuditEvent, preserving HMAC chain ordering.
	//
	// drainCtx is a long-lived context that scopes every per-event DB call the
	// drainer issues. stopAuditAsyncDrain closes the channel, waits for the
	// drainer to finish what is queued, then calls drainCancel to terminate any
	// straggling DB call still in flight — bounding total shutdown time even
	// when the per-event 10s timeout would otherwise dominate.
	auditAsyncCh         chan *domain.AuditEvent
	auditAsyncDone       chan struct{}
	auditAsyncMu         sync.RWMutex
	auditAsyncStopOnce   sync.Once
	auditAsyncStopped    bool
	auditAsyncBufferSize int // 0 means use the package-level constant default
	drainCtx             context.Context
	drainCancel          context.CancelFunc

	// Optional SIEM forwarder. When non-nil, every successfully persisted
	// audit event is also enqueued for async batched forwarding to the
	// configured external SIEM endpoint.
	siemDrain *logdrain.AuditSIEMDrain
}

// acquireSSEConn attempts to reserve an SSE connection slot for the given project.
// Returns false if either the global or per-project limit would be exceeded.
func (s *Server) acquireSSEConn(projectID string) bool {
	maxGlobal := s.config.SSEMaxConns
	if maxGlobal <= 0 {
		maxGlobal = 5000
	}
	maxProject := s.config.SSEMaxConnsPerProject
	if maxProject <= 0 {
		maxProject = 100
	}

	if !atomicIncrementBelow(&s.sseGlobalConns, maxGlobal) {
		return false
	}

	counter := s.projectSSECounter(projectID)
	if !atomicIncrementBelow(counter, maxProject) {
		s.sseGlobalConns.Add(-1)
		return false
	}
	return true
}

// releaseSSEConn releases an SSE connection slot for the given project.
func (s *Server) releaseSSEConn(projectID string) {
	s.sseGlobalConns.Add(-1)
	counter := s.projectSSECounter(projectID)
	counter.Add(-1)
}

// projectSSECounter returns the per-project SSE connection counter,
// creating one if it does not exist.
func (s *Server) projectSSECounter(projectID string) *atomic.Int64 {
	val, _ := s.sseProjectConns.LoadOrStore(projectID, &atomic.Int64{})
	return val.(*atomic.Int64)
}

func atomicIncrementBelow(counter *atomic.Int64, limit int64) bool {
	for {
		current := counter.Load()
		if current >= limit {
			return false
		}
		if counter.CompareAndSwap(current, current+1) {
			return true
		}
	}
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
	CheckMaxDispatchPriority(ctx context.Context, projectID string, requestedPriority int) error
	GetProjectOrgID(ctx context.Context, projectID string) (string, error)
	GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error)
	GetOrgPlanLimits(ctx context.Context, orgID string) (billing.OrgPlanLimits, error)
	GetMonthlyRunCount(ctx context.Context, orgID string) (int64, error)
	EnsureOrgSubscription(ctx context.Context, orgID string) error
	DispatchBilling(ctx context.Context, orgID string, planTier domain.PlanTier, eventType string, detail map[string]any)
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
	SetOverageEnabled(ctx context.Context, orgID string, enabled bool) error
	PreviewDowngrade(ctx context.Context, orgID string, targetTier domain.PlanTier) (*billing.DowngradeImpact, error)
	DetectAnomalies(ctx context.Context, orgID string) ([]billing.AnomalyAlert, error)
	GetProjectBudget(ctx context.Context, projectID string) (*billing.ProjectBudgetResponse, error)
	SetProjectBudget(ctx context.Context, projectID string, budgetMicro int64, action string) error
	GetAnomalyConfig(ctx context.Context, orgID string) (*billing.AnomalyConfigResponse, error)
	SetAnomalyConfig(ctx context.Context, orgID string, warning, critical float64) error
	GetEmailPreferences(ctx context.Context, orgID string) (*billing.EmailPreferencesResponse, error)
	UpdateEmailPreferences(ctx context.Context, orgID string, enabled bool) error
}

// ServerDeps holds all dependencies required to construct a Server.
type ServerDeps struct {
	Config               *config.Config
	Store                APIStore
	AnalyticsStore       AnalyticsStore // Optional: ClickHouse-backed analytics queries.
	Queue                queue.Queue
	PubSub               pubsub.Publisher
	MetricsHandler       http.Handler
	Pinger               Pinger
	HealthRegistry       *health.Registry
	WorkflowCallback     WorkflowCallback
	WorkflowEngine       WorkflowTrigger
	Metrics              *telemetry.Metrics
	TxPool               store.TxBeginner // Optional: enables transactional event trigger sends.
	ActorSyncer          ActorSyncer
	PoolStatter          PoolStatter              // Optional: enables DB pool backpressure middleware.
	RedisClient          *redis.Client            // Required in production startup; enables rate limiting.
	CacheBus             *straitcache.Bus         // Optional: enables cross-replica cache update/invalidation publishing.
	CacheRegistry        *straitcache.Registry    // Optional: registers API cache namespaces for cachebus fanout.
	Encryptor            Encryptor                // Optional: enables event source signature encryption.
	StripeWebhook        http.Handler             // Optional: Stripe billing webhook handler.
	BillingEnforcer      BillingEnforcer          // Optional: enables billing limit checks on project create.
	UsageService         UsageService             // Optional: enables usage endpoint.
	CHExporter           *clickhouse.Exporter     // Optional: enables ClickHouse analytics export from API handlers.
	Edition              domain.Edition           // Edition controls feature gating (community vs cloud).
	Version              string                   // Build version (injected via ldflags).
	CDCWebhookReceiver   http.Handler             // Required in production startup; handles CDC webhook push delivery.
	AuditAsyncBufferSize int                      // Optional: overrides the audit async channel capacity (default 4096, minimum 256).
	SIEMDrain            *logdrain.AuditSIEMDrain // Optional: forwards successfully persisted audit events to an external SIEM endpoint.
}

// PoolStatter provides connection pool statistics for backpressure.
type PoolStatter interface {
	AcquiredConns() int32
	MaxConns() int32
	EmptyAcquireCount() int64
	EmptyAcquireWaitTime() time.Duration
}

// PoolBackpressureStats is a single consistent pool-stat snapshot used by the
// admission-control hot path.
type PoolBackpressureStats struct {
	AcquiredConns        int32
	MaxConns             int32
	EmptyAcquireCount    int64
	EmptyAcquireWaitTime time.Duration
}

// PoolBackpressureSnapshotter lets real pool adapters expose all backpressure
// fields from one Stat() call. PoolStatter remains for lightweight tests and
// adapters that do not have a native snapshot API.
type PoolBackpressureSnapshotter interface {
	BackpressureStats() PoolBackpressureStats
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
	statusModels := newStatusReadModels(deps.RedisClient, statusReadModelTTL(deps.Config))
	cacheDeps := apiCacheDeps{
		Redis:    deps.RedisClient,
		Bus:      deps.CacheBus,
		Registry: deps.CacheRegistry,
	}

	srv := &Server{
		store:              deps.Store,
		outboxAdminStore:   resolveOutboxAdminStore(deps.Store),
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
		permCache:          newPermissionCache(permCacheTTL(deps.Config), cacheDeps),
		quotaCache: newQuotaCache(quotaCacheTTL(deps.Config), func(ctx context.Context, projectID string) (*store.ProjectQuota, error) {
			return deps.Store.GetProjectQuota(ctx, projectID)
		}, cacheDeps),
		apiKeyCache:                newAPIKeyCache(apiKeyCacheTTL(deps.Config), cacheDeps),
		jobDependencyCache:         newJobDependencyCache(jobDepsCacheTTL(deps.Config), cacheDeps),
		cacheBus:                   deps.CacheBus,
		workerJobBarrier:           newWorkerJobBarrier(workerJobBarrierTTL(deps.Config), deps.RedisClient),
		runStatusReadModel:         statusModels.run,
		workflowRunStatusReadModel: statusModels.workflowRun,
		oidcVerifier:               verifier,
		bgPool:                     pond.NewPool(4),
		rateLimiter:                ratelimit.NewRedisRateLimiter(deps.RedisClient, deps.RedisClient != nil),
		authLimiter:                ratelimit.NewAuthLimiter(deps.RedisClient, deps.RedisClient != nil),
		encryptor:                  deps.Encryptor,
		stripeWebhook:              deps.StripeWebhook,
		billingEnforcer:            deps.BillingEnforcer,
		usageService:               deps.UsageService,
		chExporter:                 deps.CHExporter,
		edition:                    deps.Edition,
		version:                    deps.Version,
		startedAt:                  time.Now(),
		cdcWebhookReceiver:         deps.CDCWebhookReceiver,
		siemDrain:                  deps.SIEMDrain,
	}
	if srv.siemDrain != nil && deps.Metrics != nil {
		srv.siemDrain.SetDroppedCounter(deps.Metrics.AuditSIEMDropped)
		srv.siemDrain.SetMetrics(
			deps.Metrics.AuditSIEMForwarded,
			deps.Metrics.AuditSIEMFailed,
			deps.Metrics.AuditSIEMCircuitOpen,
			deps.Metrics.AuditSIEMBatchSize,
		)
	}

	globalAllowPrivateEndpoints.Store(deps.Config != nil && deps.Config.AllowPrivateEndpoints)

	if deps.Config != nil {
		srv.trustedProxies = parseTrustedProxies(deps.Config.TrustedProxies)
		if len(deps.Config.TrustedProxies) > 0 && len(srv.trustedProxies) == 0 {
			slog.Warn("TRUSTED_PROXIES configured but no valid CIDR/IP entries parsed; X-Forwarded-For will be ignored")
		}
		srv.profilingAllowedCIDRs = parseTrustedProxies(deps.Config.ProfilingAllowedCIDRs)
		if len(deps.Config.ProfilingAllowedCIDRs) > 0 && len(srv.profilingAllowedCIDRs) == 0 {
			slog.Warn("STRAIT_PROFILING_ALLOWED_CIDRS configured but no valid CIDR/IP entries parsed; pprof CIDR allowlist disabled")
		}
	}

	if deps.TxPool != nil {
		if configuredStore, ok := deps.Store.(*store.Queries); ok {
			srv.runInTx = func(ctx context.Context, fn func(s APIStore) error) error {
				return configuredStore.WithTxQueries(ctx, func(q *store.Queries) error {
					return fn(q)
				})
			}
		} else {
			srv.runInTx = func(ctx context.Context, fn func(s APIStore) error) error {
				return store.WithTx(ctx, deps.TxPool, func(q *store.Queries) error {
					return fn(q)
				})
			}
		}
	} else {
		srv.runInTx = func(_ context.Context, fn func(s APIStore) error) error {
			return fn(srv.store)
		}
	}

	srv.auditAsyncBufferSize = deps.AuditAsyncBufferSize
	if srv.auditAsyncBufferSize < 256 {
		srv.auditAsyncBufferSize = auditAsyncBufferSize // fallback to constant default
	}

	if srv.poolStatter != nil {
		srv.poolBackpressure = newPoolBackpressureSampler(srv.poolStatter, 0, 0)
		srv.poolBackpressure.Start()
	}

	srv.router = srv.routes()
	srv.startAuditAsyncDrain()
	if srv.siemDrain != nil {
		srv.siemDrain.Start(context.Background())
	}
	return srv
}

func (s *Server) dbBackpressure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.shouldApplyDBBackpressure() {
			w.Header().Set("Retry-After", "1")
			respondError(w, r, http.StatusTooManyRequests, APIError{
				Code:    ErrorCodeRateLimited,
				Message: "database admission control throttled",
				Details: []string{
					"retry_after_seconds=1",
				},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// shouldApplyDBBackpressure decides whether to 429 an incoming request because
// the connection pool can't currently serve it. Two independent signals:
//
//  1. Snapshot pool occupancy: if >90% of max connections are checked out
//     right now, shed. This is a per-request read (cheap, deterministic).
//  2. Acquire-wait pressure: published by poolBackpressureSampler from a
//     background goroutine. Decoupled from the request path so concurrent
//     callers all observe the same verdict — the earlier delta-since-last-call
//     calculation done in-band was racy under load (all but one concurrent
//     request would see a near-zero delta against the just-updated baseline
//     and admit).
func (s *Server) shouldApplyDBBackpressure() bool {
	stats := poolBackpressureStats(s.poolStatter)
	if stats.MaxConns > 0 && stats.AcquiredConns > int32(float64(stats.MaxConns)*0.9) {
		return true
	}
	if s.poolBackpressure != nil && s.poolBackpressure.Shedding() {
		return true
	}
	return false
}

func poolBackpressureStats(ps PoolStatter) PoolBackpressureStats {
	if ps == nil {
		return PoolBackpressureStats{}
	}
	if snapshotter, ok := ps.(PoolBackpressureSnapshotter); ok {
		return snapshotter.BackpressureStats()
	}
	return PoolBackpressureStats{
		AcquiredConns:        ps.AcquiredConns(),
		MaxConns:             ps.MaxConns(),
		EmptyAcquireCount:    ps.EmptyAcquireCount(),
		EmptyAcquireWaitTime: ps.EmptyAcquireWaitTime(),
	}
}

func permCacheTTL(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.PermissionCacheTTL > 0 {
		return cfg.PermissionCacheTTL
	}
	return 30 * time.Second
}

func quotaCacheTTL(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.ProjectQuotaCacheTTL > 0 {
		return cfg.ProjectQuotaCacheTTL
	}
	return 60 * time.Second
}

func apiKeyCacheTTL(cfg *config.Config) time.Duration {
	if cfg != nil {
		return cfg.APIKeyCacheTTL
	}
	return time.Minute
}

func jobDepsCacheTTL(cfg *config.Config) time.Duration {
	if cfg != nil {
		return cfg.JobDepsCacheTTL
	}
	return 5 * time.Minute
}

func statusReadModelTTL(cfg *config.Config) time.Duration {
	if cfg != nil {
		return cfg.StatusReadModelTTL
	}
	return 5 * time.Minute
}

// Close releases resources held by the server (e.g. background goroutines).
// Call this when shutting down.
//
// Audit shutdown ordering is load-bearing: the primary async drainer must
// stop FIRST so it stops feeding new events into siemDrain. We then call
// FlushNow synchronously to push whatever the drainer just enqueued and
// finally Stop the SIEM drain. Without the explicit FlushNow, late-drained
// events would have to wait for the SIEM ticker to fire (default 10s) but
// Stop's budget is only 5s — they would be dropped on the floor.
func (s *Server) Close() {
	if s.permCache != nil {
		s.permCache.Stop()
	}
	if s.quotaCache != nil {
		s.quotaCache.Stop()
	}
	if s.apiKeyCache != nil {
		s.apiKeyCache.Stop()
	}
	if s.jobDependencyCache != nil {
		s.jobDependencyCache.Stop()
	}
	if s.workerJobBarrier != nil {
		s.workerJobBarrier.Stop()
	}
	if s.poolBackpressure != nil {
		s.poolBackpressure.Stop()
	}
	if s.bgPool != nil {
		s.bgPool.StopAndWait()
	}
	s.stopAuditAsyncDrain()
	if s.siemDrain != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.siemDrain.FlushNow(flushCtx); err != nil {
			slog.Warn("audit SIEM drain final flush failed", "error", err)
		}
		flushCancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.siemDrain.Stop(stopCtx)
		stopCancel()
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// RFC 8288 Link headers for agent discovery.
	w.Header().Set(
		"Link",
		`</reference/openapi.json>; rel="service-desc", </reference>; rel="service-doc", </.well-known/oauth-protected-resource>; rel="oauth-protected-resource"`,
	)

	resp := map[string]any{
		"status":    "ok",
		"version":   s.version,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Detailed subsystem checks are only exposed to authenticated internal callers.
	// The public endpoint returns a minimal status to prevent infrastructure fingerprinting.
	secret := r.Header.Get("X-Internal-Secret")
	isInternal := secret != "" && subtle.ConstantTimeCompare([]byte(secret), []byte(s.config.InternalSecret)) == 1

	if s.healthRegistry != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		result := s.healthRegistry.CheckAll(ctx)

		if result.Status != health.StatusUp {
			resp["status"] = string(result.Status)
		}

		if isInternal {
			resp["edition"] = string(s.edition)
			resp["uptime_seconds"] = int(time.Since(s.startedAt).Seconds())
			subsystems := make(map[string]string)
			for _, c := range result.Components {
				subsystems[c.Name] = string(c.Status)
			}
			resp["subsystems"] = subsystems
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	// Detailed subsystem inventory (db, redis, clickhouse, ...) and per-
	// component status is only exposed to authenticated internal callers.
	// The public endpoint returns a minimal {status} body so an
	// unauthenticated probe cannot fingerprint our infrastructure or
	// learn which dependency is currently degraded.
	secret := r.Header.Get("X-Internal-Secret")
	isInternal := secret != "" && s.config != nil && s.config.InternalSecret != "" &&
		subtle.ConstantTimeCompare([]byte(secret), []byte(s.config.InternalSecret)) == 1

	if s.healthRegistry != nil {
		result := s.healthRegistry.CheckAll(r.Context())
		if result.Status == health.StatusDown {
			if isInternal {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				if err := json.NewEncoder(w).Encode(result); err != nil {
					slog.Warn("failed to encode health check response", "error", err)
				}
				return
			}
			respondJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		if isInternal {
			respondJSON(w, http.StatusOK, result)
			return
		}
		respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}

	_, err := s.store.QueueStats(r.Context())
	if err != nil {
		if isInternal {
			respondError(w, r, http.StatusServiceUnavailable, "database not ready")
			return
		}
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}

	if s.pinger != nil {
		if err := s.pinger.Ping(r.Context()); err != nil {
			if isInternal {
				respondError(w, r, http.StatusServiceUnavailable, "redis not ready")
				return
			}
			respondJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
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

	respondJSON(w, status, ErrorResponse{
		Error:     normalizeAPIError(status, errInput),
		RequestID: requestID,
	})
}

// normalizeAPIError coerces any caller-supplied error input into the canonical
// *APIError shape. Bare strings/errors are wrapped using the default code for
// the response status so the wire format is identical for every endpoint.
func normalizeAPIError(status int, errInput any) *APIError {
	switch v := errInput.(type) {
	case APIError:
		if v.Code == "" {
			v.Code = defaultErrorCode(status)
		}
		if v.Message == "" {
			v.Message = http.StatusText(status)
		}
		return &v
	case *APIError:
		if v == nil {
			return &APIError{Code: defaultErrorCode(status), Message: http.StatusText(status)}
		}
		apiErr := *v
		if apiErr.Code == "" {
			apiErr.Code = defaultErrorCode(status)
		}
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(status)
		}
		return &apiErr
	case error:
		if v == nil {
			return &APIError{Code: defaultErrorCode(status), Message: http.StatusText(status)}
		}
		return &APIError{Code: defaultErrorCode(status), Message: v.Error()}
	case string:
		if v == "" {
			return &APIError{Code: defaultErrorCode(status), Message: http.StatusText(status)}
		}
		return &APIError{Code: defaultErrorCode(status), Message: v}
	default:
		return &APIError{Code: defaultErrorCode(status), Message: http.StatusText(status)}
	}
}

func defaultErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return ErrorCodeBadRequest
	case http.StatusUnauthorized:
		return ErrorCodeAuthenticationRequired
	case http.StatusForbidden:
		return ErrorCodeForbidden
	case http.StatusNotFound:
		return ErrorCodeNotFound
	case http.StatusConflict:
		return ErrorCodeConflict
	case http.StatusUnprocessableEntity:
		return ErrorCodeValidationFailed
	case http.StatusTooManyRequests:
		return ErrorCodeRateLimited
	case http.StatusServiceUnavailable:
		return ErrorCodeServiceUnavailable
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
	return validateURLWithAllowPrivate(rawURL, globalAllowPrivateEndpoints.Load())
}

func (s *Server) validateURL(rawURL string) error {
	return validateURLWithAllowPrivate(rawURL, s.config != nil && s.config.AllowPrivateEndpoints)
}

// validateEndpointURL validates a job endpoint (or fallback endpoint) URL using
// the same private/port/SSRF rules as validateURL and, when ENDPOINT_REQUIRE_TLS
// is enabled, additionally requires an https scheme. Job dispatch injects the
// job's decrypted secrets (X-Secret-*) and the run-token JWT (X-Run-Token), so
// operators can mandate TLS for these endpoints the same way WEBHOOK_REQUIRE_TLS
// does for webhook delivery. The knob defaults off, preserving the historical
// http-permitting behavior for self-host/dev topologies.
func (s *Server) validateEndpointURL(rawURL string) error {
	if err := s.validateURL(rawURL); err != nil {
		return err
	}
	if s.config != nil && s.config.EndpointRequireTLS {
		u, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}
		if !strings.EqualFold(u.Scheme, "https") {
			return errors.New("url must use https when ENDPOINT_REQUIRE_TLS is enabled")
		}
	}
	return nil
}

func validateURLWithAllowPrivate(rawURL string, allowPrivate bool) error {
	if err := worker.ValidateEndpointURL(rawURL, worker.WithAllowPrivateEndpoints(allowPrivate)); err != nil {
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
	if allowPrivate {
		return nil
	}

	// Use the shared SSRF validator for comprehensive private/loopback/
	// link-local/CGNAT IP checks, including DNS resolution of hostnames.
	if err := httputil.ValidateExternalURL(rawURL); err != nil {
		return fmt.Errorf("url rejected: %w", err)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
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
	allowPrivate := globalAllowPrivateEndpoints.Load()
	if err := worker.ValidateEndpointURL(
		rawURL,
		worker.WithRequireTLS(requireTLS),
		worker.WithAllowPrivateEndpoints(allowPrivate),
	); err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "URL") {
			msg = "url" + msg[3:]
		}
		return errors.New(msg)
	}

	if !allowPrivate {
		if err := httputil.ValidateExternalURL(rawURL); err != nil {
			return fmt.Errorf("url rejected: %w", err)
		}
	}

	return nil
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

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Vary", "Accept-Encoding")
	if acceptsGzip(r.Header.Get("Accept-Encoding")) && len(s.cachedOpenAPISpecGzip) > 0 {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(s.cachedOpenAPISpecGzip)
		return
	}
	_, _ = w.Write(s.cachedOpenAPISpec)
}

func acceptsGzip(acceptEncoding string) bool {
	for part := range strings.SplitSeq(acceptEncoding, ",") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]), "gzip") {
			return true
		}
	}
	return false
}

func (s *Server) handleStraitJSONSchema(w http.ResponseWriter, _ *http.Request) {
	// Serve the embedded strait.json schema file. This is the authoritative
	// schema for all SDK project configuration files. Clients (IDEs, SDK CI)
	// fetch this at most once per day.
	w.Header().Set("Content-Type", "application/schema+json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(schemas.StraitJSON)
}

// handleOAuthProtectedResource serves RFC 9728 OAuth Protected Resource
// Metadata so API clients can discover how to authenticate with the API.
func (s *Server) handleOAuthProtectedResource(w http.ResponseWriter, _ *http.Request) {
	if !s.config.OIDCEnabled || s.config.OIDCIssuer == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	scopes := []string{
		"openid", "profile", "email", "offline_access",
		"jobs:read", "jobs:write", "jobs:trigger",
		"runs:read", "runs:write",
		"workflows:read", "workflows:write", "workflows:trigger",
		"secrets:read", "secrets:write",
		"stats:read",
		"webhooks:read", "webhooks:write",
		"projects:read", "projects:write", "projects:manage",
	}

	meta := map[string]any{
		"resource":              s.config.ExternalAPIURL,
		"authorization_servers": []string{s.config.OIDCIssuer},
		"scopes_supported":      scopes,
	}

	w.Header().Set("Cache-Control", "public, max-age=3600")
	respondJSON(w, http.StatusOK, meta)
}
