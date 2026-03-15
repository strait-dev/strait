package api

import (
	"context"
	_ "embed"
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

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/health"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/ratelimit"
	"strait/internal/store"
	"strait/internal/telemetry"
	"strait/internal/worker"

	"github.com/alitto/pond/v2"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/redis/go-redis/v9"

	scalar "github.com/MarceloPetrucio/go-scalar-api-reference"
)

//go:embed openapi.yaml
var openapiSpec []byte

// APIStore is the subset of store operations needed by the API handlers.
// Composed of smaller, focused interfaces for each domain.
// ProjectContextSetter sets the app.current_project_id session variable for RLS policies.
type ProjectContextSetter interface {
	SetProjectContext(ctx context.Context, projectID string) error
	ClearProjectContext(ctx context.Context) error
}

type APIStore interface {
	JobStore
	RunStore
	WorkflowStore
	EventTriggerStore
	AuthStore
	RBACStore
	LogDrainStore
	EventSourceStore
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
}

// RunStore handles job runs, events, checkpoints, and related data.
type RunStore interface {
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, limit int, cursor *time.Time) ([]domain.JobRun, error)
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
	DeleteWebhookSubscription(ctx context.Context, id string) error
	QueueStats(ctx context.Context) (*store.QueueStats, error)
	GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*store.PerformanceAnalytics, error)
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
	UpsertRunState(ctx context.Context, s *domain.RunState) error
	GetRunState(ctx context.Context, runID, key string) (*domain.RunState, error)
	ListRunState(ctx context.Context, runID string) ([]domain.RunState, error)
	DeleteRunState(ctx context.Context, runID, key string) error
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
	BulkCancelWorkflowRuns(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error)
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
	RevokeAPIKey(ctx context.Context, id string) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	GetAPIKeyByID(ctx context.Context, id string) (*domain.APIKey, error)
	MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
}

// RBACStore handles role-based access control.
type RBACStore interface {
	GetUserPermissions(ctx context.Context, projectID, userID string) ([]string, error)
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
	SkipStep(ctx context.Context, workflowRunID, stepRef, reason string) error
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

type Server struct {
	router             chi.Router
	store              APIStore
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
}

// ServerDeps holds all dependencies required to construct a Server.
type ServerDeps struct {
	Config           *config.Config
	Store            APIStore
	Queue            queue.Queue
	PubSub           pubsub.Publisher
	MetricsHandler   http.Handler
	Pinger           Pinger
	HealthRegistry   *health.Registry
	WorkflowCallback WorkflowCallback
	WorkflowEngine   WorkflowTrigger
	Metrics          *telemetry.Metrics
	TxPool           store.TxBeginner // Optional: enables transactional event trigger sends.
	ActorSyncer      ActorSyncer
	PoolStatter      PoolStatter   // Optional: enables DB pool backpressure middleware.
	RedisClient      *redis.Client // Optional: enables per-project/key rate limiting.
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
	}

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

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	if s.healthRegistry != nil {
		result := s.healthRegistry.CheckAll(r.Context())
		if result.Status != health.StatusUp {
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
	dec := json.NewDecoder(io.LimitReader(r.Body, s.maxRequestBodySize))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func validateURL(rawURL string) error {
	if err := worker.ValidateEndpointURL(rawURL); err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "URL") {
			msg = "u" + msg[1:]
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
			msg = "u" + msg[1:]
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

// validateRequest validates a struct using the validator and writes an error response if invalid.
// Returns true if the struct is valid.
func (s *Server) validateRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := s.validate.Struct(v); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			messages := make([]string, 0, len(ve))
			for _, fe := range ve {
				messages = append(messages, fmt.Sprintf("%s: failed on '%s'", fe.Field(), fe.Tag()))
			}
			respondError(w, r, http.StatusBadRequest, APIError{
				Code:    ErrorCodeValidationError,
				Message: "validation failed",
				Details: messages,
			})
			return false
		}
		respondError(w, r, http.StatusBadRequest, APIError{
			Code:    ErrorCodeValidationError,
			Message: "invalid request",
		})
		return false
	}
	return true
}

func (s *Server) handleAPIReference(w http.ResponseWriter, r *http.Request) {
	htmlContent, err := scalar.ApiReferenceHTML(&scalar.Options{
		SpecContent: string(openapiSpec),
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
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Write(openapiSpec) //nolint:errcheck,gosec // best-effort response write
}
