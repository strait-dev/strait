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
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string) ([]domain.Job, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	ListChildRuns(ctx context.Context, parentRunID string) ([]domain.JobRun, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error)
	ListWebhookDeliveries(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error)
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
	DeleteWorkflow(ctx context.Context, id string) error
	CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	DeleteStepsByWorkflow(ctx context.Context, workflowID string) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListWorkflowRuns(ctx context.Context, workflowID string, limit, offset int) ([]domain.WorkflowRun, error)
	ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int) ([]domain.WorkflowRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
}

// Pinger checks service health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// WorkflowCallback is called after a run reaches a terminal state via SDK or cancel.
type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
}

type WorkflowTrigger interface {
	TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string) (*domain.WorkflowRun, error)
}

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

func NewServer(cfg *config.Config, s APIStore, q queue.Queue, pub pubsub.Publisher, metricsHandler http.Handler, pinger Pinger, wfCallback WorkflowCallback, workflowEngine WorkflowTrigger) *Server {
	srv := &Server{
		store:            s,
		queue:            q,
		pubsub:           pub,
		config:           cfg,
		metricsHandler:   metricsHandler,
		pinger:           pinger,
		workflowCallback: wfCallback,
		workflowEngine:   workflowEngine,
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

	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleHealthReady)
	if s.metricsHandler != nil {
		r.Handle("/metrics", s.metricsHandler)
	}

	r.Route("/v1", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)

		r.Route("/jobs", func(r chi.Router) {
			r.Post("/", s.handleCreateJob)
			r.Get("/", s.handleListJobs)

			r.Route("/{jobID}", func(r chi.Router) {
				r.Get("/", s.handleGetJob)
				r.Patch("/", s.handleUpdateJob)
				r.Delete("/", s.handleDeleteJob)
				r.With(httprate.LimitByIP(10, time.Minute)).Post("/trigger", s.handleTriggerJob)
				r.Post("/trigger/bulk", s.handleBulkTriggerJob)
				r.Get("/versions", s.handleListJobVersions)
			})
		})

		r.Route("/runs", func(r chi.Router) {
			r.Get("/", s.handleListRuns)
			r.Post("/bulk-cancel", s.handleBulkCancelRuns)
			r.Route("/{runID}", func(r chi.Router) {
				r.Get("/", s.handleGetRun)
				r.Delete("/", s.handleCancelRun)
				r.Get("/stream", s.handleRunStream)
				r.Get("/children", s.handleListChildRuns)
				r.Get("/events", s.handleListRunEvents)
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
				r.Post("/trigger", s.handleTriggerWorkflow)
				r.Get("/runs", s.handleListWorkflowRuns)
			})
		})

		r.Route("/workflow-runs", func(r chi.Router) {
			r.Get("/", s.handleListWorkflowRunsByProject)
			r.Route("/{workflowRunID}", func(r chi.Router) {
				r.Get("/", s.handleGetWorkflowRun)
				r.Delete("/", s.handleCancelWorkflowRun)
				r.Get("/steps", s.handleListWorkflowStepRuns)
			})
		})
	})

	r.Route("/sdk/v1", func(r chi.Router) {
		r.Use(s.runTokenAuth)
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Post("/log", s.handleSDKLog)
			r.Post("/heartbeat", s.handleSDKHeartbeat)
			r.Post("/complete", s.handleSDKComplete)
			r.Post("/fail", s.handleSDKFail)
			r.Post("/spawn", s.handleSDKSpawn)
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
		return fmt.Errorf("URL must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	// Block private/internal IPs (SSRF protection)
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL must not point to private or loopback addresses")
		}
	}

	return nil
}
