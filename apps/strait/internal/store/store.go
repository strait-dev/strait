package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrJobSlugConflict              = errors.New("job slug conflict")
	ErrJobNotFound                  = errors.New("job not found")
	ErrJobGroupNotFound             = errors.New("job group not found")
	ErrWebhookSubscriptionNotFound  = errors.New("webhook subscription not found")
	ErrEnvironmentNotFound          = errors.New("environment not found")
	ErrJobSecretNotFound            = errors.New("job secret not found")
	ErrRunNotFound                  = errors.New("run not found")
	ErrRunConflict                  = errors.New("run status update conflict")
	ErrWorkflowNotFound             = errors.New("workflow not found")
	ErrWorkflowStepNotFound         = errors.New("workflow step not found")
	ErrWorkflowRunNotFound          = errors.New("workflow run not found")
	ErrWorkflowStepRunNotFound      = errors.New("workflow step run not found")
	ErrEventKeyConflict             = errors.New("event key conflict")
	ErrWorkflowVersionNotFound      = errors.New("workflow version not found")
	ErrDeploymentVersionNotFound    = errors.New("deployment version not found")
	ErrNotificationChannelNotFound  = errors.New("notification channel not found")
	ErrJobMemoryPerKeyLimitExceeded = errors.New("job memory per-key limit exceeded")
	ErrJobMemoryPerJobLimitExceeded = errors.New("job memory per-job limit exceeded")
)

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type JobStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Job, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	DeleteJob(ctx context.Context, id string) error
	ListCronJobs(ctx context.Context) ([]domain.Job, error)
	GetProjectQuota(ctx context.Context, projectID string) (*ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
	PauseJob(ctx context.Context, id, reason string) error
	ResumeJob(ctx context.Context, id string) error
}

type JobGroupStore interface {
	CreateJobGroup(ctx context.Context, group *domain.JobGroup) error
	GetJobGroup(ctx context.Context, id string) (*domain.JobGroup, error)
	ListJobGroups(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobGroup, error)
	UpdateJobGroup(ctx context.Context, group *domain.JobGroup) error
	DeleteJobGroup(ctx context.Context, id string) error
	ListJobsByGroup(ctx context.Context, groupID string, limit int, cursor *time.Time) ([]domain.Job, error)
	PauseJobsByGroup(ctx context.Context, groupID string) error
	ResumeJobsByGroup(ctx context.Context, groupID string) error
	GetJobGroupStats(ctx context.Context, groupID string) (*JobGroupStats, error)
}

type JobGroupStats struct {
	GroupID   string         `json:"group_id"`
	RunCounts map[string]int `json:"run_counts"`
}

type EnvironmentStore interface {
	CreateEnvironment(ctx context.Context, env *domain.Environment) error
	GetEnvironment(ctx context.Context, id string) (*domain.Environment, error)
	ListEnvironments(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error)
	UpdateEnvironment(ctx context.Context, env *domain.Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
	GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error)
	CreateStandardEnvironments(ctx context.Context, projectID string) error
}

type JobSecretStore interface {
	CreateJobSecret(ctx context.Context, secret *domain.JobSecret) error
	GetJobSecret(ctx context.Context, id string) (*domain.JobSecret, error)
	ListJobSecrets(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error)
	DeleteJobSecret(ctx context.Context, id string) error
	ListJobSecretsByJob(ctx context.Context, jobID, environment string) ([]domain.JobSecret, error)
}

type RunStore interface {
	CreateRun(ctx context.Context, run *domain.JobRun) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, executionMode *domain.ExecutionMode, errorClass *string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListDeadLetterRunsFiltered(ctx context.Context, projectID string, jobID *string, masked *bool, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListFinishedRunsSince(ctx context.Context, projectID string, since time.Time, sinceRunID string, limit int) ([]domain.JobRun, error)
	BulkReplayDeadLetterRuns(ctx context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateRunStatusReturningOld(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) (domain.RunStatus, error)
	ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error)
	ReplayDeadLetterRunWithAudit(ctx context.Context, runID string, audit *domain.AuditEvent) (*domain.JobRun, error)
	UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error
	UpdateHeartbeat(ctx context.Context, id string) error
	BatchUpdateHeartbeat(ctx context.Context, ids []string) error
	ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	ListDueRuns(ctx context.Context) ([]domain.JobRun, error)
	ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error)
	ListChildRuns(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error)
	FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	ListRunCheckpoints(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error)
	CreateRunUsage(ctx context.Context, usage *domain.RunUsage) error
	ListRunUsage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error)
	CreateRunToolCall(ctx context.Context, call *domain.RunToolCall) error
	ListRunToolCalls(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error)
	UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error
	ListRunOutputs(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error)
	AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error)
	GetEndpointCircuitState(ctx context.Context, endpointURL string) (*domain.EndpointCircuitState, error)
	CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	RecordEndpointCircuitFailure(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error
	RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error
	GetEndpointHealthScore(ctx context.Context, endpointURL string) (*domain.EndpointHealthScore, error)
	UpsertEndpointHealthScore(ctx context.Context, score *domain.EndpointHealthScore) error
	GetDebugBundle(ctx context.Context, runID string) (*domain.DebugBundle, error)
	UpdateRunDebugMode(ctx context.Context, runID string, debugMode bool) error
	ListRunLineage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	SumRunCostMicrousd(ctx context.Context, runID string) (int64, error)
	SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error)
	GetRunsByIDs(ctx context.Context, ids []string) (map[string]*domain.JobRun, error)
	BulkCancelRuns(ctx context.Context, ids []string, finishedAt time.Time, reason string) ([]BulkCancelResult, error)
	CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error)
	ResetRunIdempotencyKey(ctx context.Context, runID string) error
	RescheduleRun(ctx context.Context, runID string, scheduledAt time.Time, payload json.RawMessage) error
	BulkCancelByFilter(ctx context.Context, projectID string, f BulkCancelFilter, now time.Time, reason string) ([]string, error)
	CreateRunResourceSnapshot(ctx context.Context, snapshot *domain.RunResourceSnapshot) error
	ListRunResourceSnapshots(ctx context.Context, runID string, from, to *time.Time, limit int) ([]domain.RunResourceSnapshot, error)
	SumRunTotalTokens(ctx context.Context, runID string) (int64, error)
	CountRunToolCalls(ctx context.Context, runID string) (int, error)
	CountRunIterations(ctx context.Context, runID string) (int, error)
	CreateRunIteration(ctx context.Context, iter *domain.RunIteration) error
}

type ProjectQuota struct {
	ProjectID                     string
	MaxQueuedRuns                 int
	MaxExecutingRuns              int
	MaxJobs                       int
	Timezone                      string
	MaxCostPerRunMicrousd         int64
	MaxDailyCostMicrousd          int64
	ComputeDailyCostLimitMicrousd int64
	MaxActiveEventTriggers        int // 0 = unlimited
	RateLimitRequests             int
	RateLimitWindowSecs           int
	DefaultRegion                 string
	PlanTier                      string
	MaxTokensPerRun               int64
	MaxToolCallsPerRun            int
	MaxIterationsPerRun           int
	MaxMemoryPerKeyBytes          int
	MaxMemoryPerJobBytes          int
	MaxKeyLifetimeDays            int
}

// JobHealthStats contains aggregated health metrics for a job.
type JobHealthStats struct {
	TotalRuns       int     `json:"total_runs"`
	CompletedRuns   int     `json:"completed_runs"`
	FailedRuns      int     `json:"failed_runs"`
	TimedOutRuns    int     `json:"timed_out_runs"`
	CrashedRuns     int     `json:"crashed_runs"`
	CanceledRuns    int     `json:"canceled_runs"`
	ExpiredRuns     int     `json:"expired_runs"`
	SuccessRate     float64 `json:"success_rate"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	P95DurationSecs float64 `json:"p95_duration_secs"`
	P99DurationSecs float64 `json:"p99_duration_secs"`
	HealthScore     float64 `json:"health_score"`
}

type EventStore interface {
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	ListEventsAsc(ctx context.Context, runID string, limit int, afterTime *time.Time, afterID string) ([]domain.RunEvent, error)
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
}

type WebhookDeliveryStore interface {
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	EnqueueRunWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun, maxAttempts int) (*domain.WebhookDelivery, error)
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error)
	GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	RetryWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	ListPendingWebhookRetries(ctx context.Context) ([]domain.WebhookDelivery, error)
	ListPendingRunWebhookDeliveries(ctx context.Context) ([]domain.WebhookDelivery, error)
	DeleteOldWebhookDeliveries(ctx context.Context, before time.Time, limit int) (int, error)
}

type WebhookSubscriptionStore interface {
	CreateWebhookSubscription(ctx context.Context, sub *domain.WebhookSubscription) error
	GetWebhookSubscription(ctx context.Context, id string) (*domain.WebhookSubscription, error)
	ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	DeleteWebhookSubscription(ctx context.Context, id string) error
	RotateWebhookSecret(ctx context.Context, id, newSecret string, graceExpiresAt time.Time) error
	GetWebhookSubscriptionSecrets(ctx context.Context, subscriptionID string) (string, string, *time.Time, error)
}

type APIKeyStore interface {
	CreateAPIKey(ctx context.Context, key *domain.APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	ListAPIKeysByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
}

type JobVersionStore interface {
	CreateJobVersion(ctx context.Context, v *domain.JobVersion) error
	ListJobVersionsByJob(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobVersion, error)
	GetJobVersion(ctx context.Context, jobID string, version int) (*domain.JobVersion, error)
}

type WorkflowStore interface {
	CreateWorkflow(ctx context.Context, w *domain.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	ListWorkflows(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Workflow, error)
	ListCronWorkflows(ctx context.Context) ([]domain.Workflow, error)
	UpdateWorkflow(ctx context.Context, w *domain.Workflow) error
	CreateWorkflowVersionSnapshot(ctx context.Context, workflowID string, version int) error
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error)
	DeleteWorkflow(ctx context.Context, id string) error
}

type WorkflowStepStore interface {
	CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	GetWorkflowStep(ctx context.Context, id string) (*domain.WorkflowStep, error)
	DeleteStepsByWorkflow(ctx context.Context, workflowID string) error
}

// StepDepResult is returned by IncrementStepDeps for each step whose deps_completed was incremented.
type StepDepResult struct {
	StepRunID     string
	StepRef       string
	DepsCompleted int
	DepsRequired  int
	JobID         string
	Condition     json.RawMessage
	Payload       json.RawMessage
	WorkflowRunID string
}

type WorkflowRunStore interface {
	CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListWorkflowRuns(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	CreateWorkflowRunLabels(ctx context.Context, workflowRunID string, labels map[string]string) error
	ListWorkflowRunLabels(ctx context.Context, workflowRunID string) (map[string]string, error)
	DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	ListTimedOutWorkflowRuns(ctx context.Context) ([]domain.WorkflowRun, error)
	GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error)
	BulkCancelWorkflowRuns(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error)
}

type WorkflowStepRunStore interface {
	CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error
	GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
	ListRunnableStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error)
	ListRunningStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int) ([]domain.WorkflowStepRun, error)
	ListStepRunStatusesByWorkflowRun(ctx context.Context, workflowRunID string) (map[string]domain.StepRunStatus, error)
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]StepDepResult, error)
	GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error)
	IncrementStepRunAttempt(ctx context.Context, id string, newAttempt int) error
	CreateWorkflowStepDecision(ctx context.Context, d *domain.WorkflowStepDecision) error
	ListWorkflowStepDecisions(ctx context.Context, workflowRunID, stepRef, decisionType string, limit int, cursor *time.Time) ([]domain.WorkflowStepDecision, error)
}

type EventTriggerStore interface {
	CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error
	GetEventTriggerByEventKey(ctx context.Context, eventKey string) (*domain.EventTrigger, error)
	GetEventTriggerByStepRunID(ctx context.Context, stepRunID string) (*domain.EventTrigger, error)
	GetEventTriggerByJobRunID(ctx context.Context, jobRunID string) (*domain.EventTrigger, error)
	UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	ListExpiredEventTriggers(ctx context.Context) ([]domain.EventTrigger, error)
	ListEventTriggersByProject(ctx context.Context, projectID, status, workflowRunID, sourceType string, limit int, cursor *time.Time) ([]domain.EventTrigger, error)
	CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
	CancelEventTriggerByJobRun(ctx context.Context, jobRunID string) error
	ListReceivedEventTriggersWithStaleSteps(ctx context.Context) ([]domain.EventTrigger, error)
	DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
	ReceiveEventAndRequeueRun(ctx context.Context, triggerID string, payload json.RawMessage, receivedAt time.Time, jobRunID string) error
	SetEventTriggerSentBy(ctx context.Context, id, sentBy string) error
	BatchReceiveEventTriggers(ctx context.Context, triggerIDs []string, payload json.RawMessage, receivedAt time.Time, sentBy string) ([]string, error)
}

type BatchOperationStore interface {
	CreateBatchOperation(ctx context.Context, op *domain.BatchOperation) error
	FinalizeBatchOperation(ctx context.Context, batchID string, createdCount int) error
	GetBatchOperation(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error)
	ListBatchOperations(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.BatchOperation, error)
}

type DeploymentStore interface {
	CreateDeploymentVersion(ctx context.Context, deployment *domain.DeploymentVersion) error
	GetDeploymentVersion(ctx context.Context, deploymentID, projectID string) (*domain.DeploymentVersion, error)
	ListDeploymentVersions(ctx context.Context, projectID, environment string, limit int, cursor *time.Time) ([]domain.DeploymentVersion, error)
	FinalizeDeploymentVersion(ctx context.Context, deploymentID, projectID, updatedBy string) (*domain.DeploymentVersion, error)
	PromoteDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error)
	RollbackDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error)
}

type LogDrainStore interface {
	CreateLogDrain(ctx context.Context, drain *domain.LogDrain) error
	GetLogDrain(ctx context.Context, drainID, projectID string) (*domain.LogDrain, error)
	ListLogDrains(ctx context.Context, projectID string) ([]domain.LogDrain, error)
	ListEnabledLogDrains(ctx context.Context) ([]domain.LogDrain, error)
	UpdateLogDrain(ctx context.Context, drainID, projectID string, patch map[string]any) error
	DeleteLogDrain(ctx context.Context, drainID, projectID string) error
}

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

// JobMemoryStore defines operations for job-level persistent memory.
type JobMemoryStore interface {
	UpsertJobMemory(ctx context.Context, mem *domain.JobMemory) error
	UpsertJobMemoryWithQuota(ctx context.Context, mem *domain.JobMemory, maxPerKey, maxPerJob int) error
	GetJobMemory(ctx context.Context, jobID, key string) (*domain.JobMemory, error)
	ListJobMemory(ctx context.Context, jobID string) ([]domain.JobMemory, error)
	DeleteJobMemory(ctx context.Context, jobID, key string) error
	SumJobMemorySizeBytes(ctx context.Context, jobID string) (int, error)
	DeleteExpiredJobMemory(ctx context.Context) (int64, error)
}

// CostEstimateStore defines operations for job cost estimates.
type CostEstimateStore interface {
	GetJobCostEstimate(ctx context.Context, jobID string) (*domain.JobCostEstimate, error)
	UpsertJobCostEstimate(ctx context.Context, jobID string) error
	ListActiveJobIDs(ctx context.Context) ([]string, error)
}

// NotificationStore handles notification channel and delivery operations.
type NotificationStore interface {
	CreateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error
	GetNotificationChannel(ctx context.Context, id, projectID string) (*domain.NotificationChannel, error)
	ListNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, ch *domain.NotificationChannel) error
	DeleteNotificationChannel(ctx context.Context, id, projectID string) error
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
	ClaimPendingNotificationDeliveries(ctx context.Context, limit int, leaseDuration time.Duration) ([]domain.NotificationDelivery, error)
	UpdateClaimedNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) (bool, error)
	ListNotificationDeliveries(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.NotificationDelivery, error)
}

type Store interface {
	JobStore
	JobGroupStore
	EnvironmentStore
	JobSecretStore
	RunStore
	EventStore
	WebhookDeliveryStore
	WebhookSubscriptionStore
	APIKeyStore
	JobVersionStore
	WorkflowStore
	WorkflowStepStore
	WorkflowRunStore
	WorkflowStepRunStore
	WorkflowSnapshotStore
	EventTriggerStore
	BatchOperationStore
	DeploymentStore
	LogDrainStore
	EventSourceStore
	CostEstimateStore
	JobMemoryStore
	NotificationStore
	QueueStats(ctx context.Context) (*QueueStats, error)
}

type Queries struct {
	db                  DBTX
	secretEncryptionKey string
	auditSigningKey     []byte
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}

func (q *Queries) SetSecretEncryptionKey(secretEncryptionKey string) {
	q.secretEncryptionKey = secretEncryptionKey
}

func (q *Queries) SetAuditSigningKey(key []byte) {
	q.auditSigningKey = key
}

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

func WithTx(ctx context.Context, db TxBeginner, fn func(q *Queries) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			slog.Warn("failed to rollback transaction", "error", rbErr)
		}
	}()

	if err := fn(New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true

	return nil
}

// TryAdvisoryLock attempts to acquire a PostgreSQL session-level advisory lock.
// Returns true if the lock was acquired, false if held by another session.
func (q *Queries) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	var acquired bool
	err := q.db.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired)
	if err != nil {
		return false, fmt.Errorf("pg_try_advisory_lock: %w", err)
	}
	return acquired, nil
}

// ReleaseAdvisoryLock releases a PostgreSQL session-level advisory lock.
func (q *Queries) ReleaseAdvisoryLock(ctx context.Context, lockID int64) error {
	_, err := q.db.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockID)
	if err != nil {
		return fmt.Errorf("pg_advisory_unlock: %w", err)
	}
	return nil
}

// SetProjectContext sets the app.current_project_id session variable for RLS policies.
func (q *Queries) SetProjectContext(ctx context.Context, projectID string) error {
	_, err := q.db.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", projectID)
	if err != nil {
		return fmt.Errorf("set project context: %w", err)
	}
	return nil
}

// ClearProjectContext resets the app.current_project_id session variable.
func (q *Queries) ClearProjectContext(ctx context.Context) error {
	_, err := q.db.Exec(ctx, "SELECT set_config('app.current_project_id', '', true)")
	if err != nil {
		return fmt.Errorf("clear project context: %w", err)
	}
	return nil
}

func (q *Queries) AdvisoryXactLock(ctx context.Context, lockID int64) error {
	_, err := q.db.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockID)
	if err != nil {
		return fmt.Errorf("pg_advisory_xact_lock: %w", err)
	}
	return nil
}
