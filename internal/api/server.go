package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"orchestrator/internal/config"
	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/riandyrn/otelchi"
)

// APIStore is the subset of store operations needed by the API handlers.
type APIStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	CreateJobSecret(ctx context.Context, secret *domain.JobSecret) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string) ([]domain.Job, error)
	CreateJobGroup(ctx context.Context, group *domain.JobGroup) error
	GetJobGroup(ctx context.Context, id string) (*domain.JobGroup, error)
	ListJobGroups(ctx context.Context, projectID string) ([]domain.JobGroup, error)
	UpdateJobGroup(ctx context.Context, group *domain.JobGroup) error
	DeleteJobGroup(ctx context.Context, id string) error
	ListJobsByGroup(ctx context.Context, groupID string) ([]domain.Job, error)
	CreateEnvironment(ctx context.Context, env *domain.Environment) error
	GetEnvironment(ctx context.Context, id string) (*domain.Environment, error)
	ListEnvironments(ctx context.Context, projectID string) ([]domain.Environment, error)
	UpdateEnvironment(ctx context.Context, env *domain.Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
	GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error)
	ListJobSecrets(ctx context.Context, projectID, jobID, environment string) ([]domain.JobSecret, error)
	ListJobsByTag(ctx context.Context, projectID, tagKey, tagValue string) ([]domain.Job, error)
	CreateJobDependency(ctx context.Context, dep *domain.JobDependency) error
	ListJobDependencies(ctx context.Context, jobID string) ([]domain.JobDependency, error)
	DeleteJobDependency(ctx context.Context, id string) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	ListRunCheckpoints(ctx context.Context, runID string, limit int) ([]domain.RunCheckpoint, error)
	CreateRunUsage(ctx context.Context, usage *domain.RunUsage) error
	ListRunUsage(ctx context.Context, runID string, limit int) ([]domain.RunUsage, error)
	CreateRunToolCall(ctx context.Context, call *domain.RunToolCall) error
	ListRunToolCalls(ctx context.Context, runID string, limit int) ([]domain.RunToolCall, error)
	UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error
	ListRunOutputs(ctx context.Context, runID string) ([]domain.RunOutput, error)
	AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue *string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListDeadLetterRuns(ctx context.Context, projectID string, limit int) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error)
	UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error
	ListChildRuns(ctx context.Context, parentRunID string) ([]domain.JobRun, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error)
	ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int) ([]domain.WebhookDelivery, error)
	CreateAPIKey(ctx context.Context, key *domain.APIKey) error
	ListAPIKeysByProject(ctx context.Context, projectID string) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	ListJobVersionsByJob(ctx context.Context, jobID string) ([]domain.JobVersion, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
	UpdateHeartbeat(ctx context.Context, id string) error
	QueueStats(ctx context.Context) (*store.QueueStats, error)
	CreateWorkflow(ctx context.Context, w *domain.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	ListWorkflows(ctx context.Context, projectID string) ([]domain.Workflow, error)
	UpdateWorkflow(ctx context.Context, w *domain.Workflow) error
	CreateWorkflowVersionSnapshot(ctx context.Context, workflowID string, version int) error
	DeleteWorkflow(ctx context.Context, id string) error
	CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	DeleteStepsByWorkflow(ctx context.Context, workflowID string) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListWorkflowRuns(ctx context.Context, workflowID string, limit, offset int) ([]domain.WorkflowRun, error)
	ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int) ([]domain.WorkflowRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	CreateWorkflowRunLabels(ctx context.Context, workflowRunID string, labels map[string]string) error
	ListWorkflowRunLabels(ctx context.Context, workflowRunID string) (map[string]string, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	DeleteJobSecret(ctx context.Context, id string) error
	BatchUpdateJobsEnabled(ctx context.Context, ids []string, enabled bool) (int64, error)
	GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	GetDebugBundle(ctx context.Context, runID string) (*domain.DebugBundle, error)
	UpdateRunDebugMode(ctx context.Context, runID string, debugMode bool) error
	ListEvents(ctx context.Context, runID string) ([]domain.RunEvent, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	ListRunLineage(ctx context.Context, runID string) ([]domain.JobRun, error)
	SumRunCostMicrousd(ctx context.Context, runID string) (int64, error)
	SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error)
}

// Pinger checks service health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// WorkflowCallback is called after a run reaches a terminal state via SDK or cancel.
type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
	ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error
	ResumeWorkflowRun(ctx context.Context, workflowRunID string) error
	SkipStep(ctx context.Context, workflowRunID, stepRef, reason string) error
	ForceCompleteStep(ctx context.Context, workflowRunID, stepRef string, result json.RawMessage) error
}

type WorkflowTrigger interface {
	TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride) (*domain.WorkflowRun, error)
	RetryWorkflowRun(ctx context.Context, originalRunID string) (*domain.WorkflowRun, error)
}

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
)

type Server struct {
	router           chi.Router
	store            APIStore
	queue            queue.Queue
	pubsub           pubsub.Publisher
	config           *config.Config
	metricsHandler   http.Handler
	pinger           Pinger
	workflowCallback WorkflowCallback
	workflowEngine   WorkflowTrigger
}

// ServerDeps holds all dependencies required to construct a Server.
type ServerDeps struct {
	Config           *config.Config
	Store            APIStore
	Queue            queue.Queue
	PubSub           pubsub.Publisher
	MetricsHandler   http.Handler
	Pinger           Pinger
	WorkflowCallback WorkflowCallback
	WorkflowEngine   WorkflowTrigger
}

// NewServer creates a new HTTP API server with the given dependencies.
func NewServer(deps ServerDeps) *Server {
	srv := &Server{
		store:            deps.Store,
		queue:            deps.Queue,
		pubsub:           deps.PubSub,
		config:           deps.Config,
		metricsHandler:   deps.MetricsHandler,
		pinger:           deps.Pinger,
		workflowCallback: deps.WorkflowCallback,
		workflowEngine:   deps.WorkflowEngine,
	}
	srv.router = srv.routes()
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.config.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Secret", "X-Idempotency-Key", "Idempotency-Key"},
		ExposedHeaders:   []string{"Link", "X-Request-Id"},
		AllowCredentials: s.config.CORSAllowCredentials,
		MaxAge:           300,
	}))

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelchi.Middleware("orchestrator", otelchi.WithChiRoutes(r)))
	r.Use(s.requestLogger)
	r.Use(chimw.Recoverer)
	if s.config.RateLimitRequests > 0 {
		r.Use(httprate.LimitByIP(s.config.RateLimitRequests, s.config.RateLimitWindow))
	}

	triggerRateLimitRequests := s.config.TriggerRateLimitRequests
	if triggerRateLimitRequests <= 0 {
		triggerRateLimitRequests = 10
	}
	triggerRateLimitWindow := s.config.TriggerRateLimitWindow
	if triggerRateLimitWindow <= 0 {
		triggerRateLimitWindow = time.Minute
	}

	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleHealthReady)
	if s.metricsHandler != nil {
		r.Handle("/metrics", s.metricsHandler)
	}

	r.Route("/v1", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)

		r.Route("/secrets", func(r chi.Router) {
			r.Post("/", s.handleCreateSecret)
			r.Get("/", s.handleListSecrets)
			r.Delete("/{secretID}", s.handleDeleteSecret)
		})

		r.Route("/jobs", func(r chi.Router) {
			r.Post("/", s.handleCreateJob)
			r.Get("/", s.handleListJobs)
			r.Post("/batch", s.handleBatchCreateJobs)
			r.Post("/batch-enable", s.handleBatchEnableJobs)
			r.Post("/batch-disable", s.handleBatchDisableJobs)

			r.Route("/{jobID}", func(r chi.Router) {
				r.Get("/", s.handleGetJob)
				r.Patch("/", s.handleUpdateJob)
				r.Delete("/", s.handleDeleteJob)
				r.With(httprate.LimitByIP(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/trigger", s.handleTriggerJob)
				r.Post("/trigger/bulk", s.handleBulkTriggerJob)
				r.Post("/dependencies", s.handleCreateJobDependency)
				r.Get("/dependencies", s.handleListJobDependencies)
				r.Delete("/dependencies/{depID}", s.handleDeleteJobDependency)
				r.Get("/versions", s.handleListJobVersions)
				r.Post("/clone", s.handleCloneJob)
				r.Get("/health", s.handleGetJobHealth)
			})
		})

		r.Route("/job-groups", func(r chi.Router) {
			r.Post("/", s.handleCreateJobGroup)
			r.Get("/", s.handleListJobGroups)
			r.Route("/{groupID}", func(r chi.Router) {
				r.Get("/", s.handleGetJobGroup)
				r.Patch("/", s.handleUpdateJobGroup)
				r.Delete("/", s.handleDeleteJobGroup)
				r.Get("/jobs", s.handleListJobsByGroup)
			})
		})

		r.Route("/environments", func(r chi.Router) {
			r.Post("/", s.handleCreateEnvironment)
			r.Get("/", s.handleListEnvironments)
			r.Route("/{envID}", func(r chi.Router) {
				r.Get("/", s.handleGetEnvironment)
				r.Patch("/", s.handleUpdateEnvironment)
				r.Delete("/", s.handleDeleteEnvironment)
				r.Get("/variables", s.handleGetResolvedVariables)
			})
		})

		r.Route("/runs", func(r chi.Router) {
			r.Get("/", s.handleListRuns)
			r.Get("/dlq", s.handleListDeadLetterRuns)
			r.Post("/bulk-cancel", s.handleBulkCancelRuns)
			r.Route("/{runID}", func(r chi.Router) {
				r.Get("/", s.handleGetRun)
				r.Delete("/", s.handleCancelRun)
				r.Post("/replay", s.handleReplayRun)
				r.Post("/dlq-replay", s.handleReplayDeadLetterRun)
				r.Get("/stream", s.handleRunStream)
				r.Get("/children", s.handleListChildRuns)
				r.Get("/events", s.handleListRunEvents)
				r.Get("/checkpoints", s.handleListRunCheckpoints)
				r.Get("/usage", s.handleListRunUsage)
				r.Get("/tool-calls", s.handleListRunToolCalls)
				r.Get("/outputs", s.handleListRunOutputs)
				r.Get("/debug-bundle", s.handleGetDebugBundle)
				r.Post("/debug", s.handleSetDebugMode)
				r.Get("/lineage", s.handleListRunLineage)
			})
		})

		r.Get("/webhook-deliveries", s.handleListWebhookDeliveries)

		r.Route("/api-keys", func(r chi.Router) {
			r.Post("/", s.handleCreateAPIKey)
			r.Get("/", s.handleListAPIKeys)
			r.Delete("/{keyID}", s.handleRevokeAPIKey)
		})

		r.Get("/stats", s.handleStats)

		r.Route("/workflows", func(r chi.Router) {
			r.Post("/", s.handleCreateWorkflow)
			r.Get("/", s.handleListWorkflows)
			r.Route("/{workflowID}", func(r chi.Router) {
				r.Get("/", s.handleGetWorkflow)
				r.Patch("/", s.handleUpdateWorkflow)
				r.Delete("/", s.handleDeleteWorkflow)
				r.Post("/dry-run", s.handleDryRunWorkflow)
				r.Get("/graph", s.handleWorkflowGraph)
				r.Post("/trigger", s.handleTriggerWorkflow)
				r.Post("/clone", s.handleCloneWorkflow)
				r.Get("/runs", s.handleListWorkflowRuns)
			})
		})

		r.Route("/workflow-runs", func(r chi.Router) {
			r.Get("/", s.handleListWorkflowRunsByProject)
			r.Route("/{workflowRunID}", func(r chi.Router) {
				r.Get("/", s.handleGetWorkflowRun)
				r.Delete("/", s.handleCancelWorkflowRun)
				r.Post("/pause", s.handlePauseWorkflowRun)
				r.Post("/resume", s.handleResumeWorkflowRun)
				r.Get("/labels", s.handleGetWorkflowRunLabels)
				r.Get("/steps", s.handleListWorkflowStepRuns)
				r.Post("/steps/{stepRef}/approve", s.handleApproveWorkflowStep)
				r.Post("/steps/{stepRef}/skip", s.handleSkipWorkflowStep)
				r.Post("/steps/{stepRef}/force-complete", s.handleForceCompleteWorkflowStep)
				r.Post("/retry", s.handleRetryWorkflowRun)
			})
		})
	})

	r.Route("/sdk/v1", func(r chi.Router) {
		r.Use(s.runTokenAuth)
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Post("/log", s.handleSDKLog)
			r.Post("/progress", s.handleSDKProgress)
			r.Post("/annotate", s.handleSDKAnnotate)
			r.Post("/heartbeat", s.handleSDKHeartbeat)
			r.Post("/checkpoint", s.handleSDKCheckpoint)
			r.Post("/usage", s.handleSDKUsage)
			r.Post("/tool-call", s.handleSDKToolCall)
			r.Post("/output", s.handleSDKOutput)
			r.Post("/complete", s.handleSDKComplete)
			r.Post("/fail", s.handleSDKFail)
			r.Post("/spawn", s.handleSDKSpawn)
			r.Post("/continue", s.handleSDKContinue)
		})
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	// Verify database connectivity via a lightweight query
	_, err := s.store.QueueStats(r.Context())
	if err != nil {
		respondError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}

	if s.pinger != nil {
		if err := s.pinger.Ping(r.Context()); err != nil {
			respondError(w, http.StatusServiceUnavailable, "redis not ready")
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

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// validateURL checks that a URL is valid and doesn't target private networks.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("url must have a host")
	}

	// Block private/internal IPs (SSRF protection)
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("url must not point to private or loopback addresses")
		}
	}

	return nil
}
