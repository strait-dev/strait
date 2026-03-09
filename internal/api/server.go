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
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"

	scalar "github.com/MarceloPetrucio/go-scalar-api-reference"
)

//go:embed openapi.yaml
var openapiSpec []byte

// APIStore is the subset of store operations needed by the API handlers.
type APIStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	CreateJobSecret(ctx context.Context, secret *domain.JobSecret) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Job, error)
	CreateJobGroup(ctx context.Context, group *domain.JobGroup) error
	GetJobGroup(ctx context.Context, id string) (*domain.JobGroup, error)
	ListJobGroups(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobGroup, error)
	UpdateJobGroup(ctx context.Context, group *domain.JobGroup) error
	DeleteJobGroup(ctx context.Context, id string) error
	ListJobsByGroup(ctx context.Context, groupID string, limit int, cursor *time.Time) ([]domain.Job, error)
	CreateEnvironment(ctx context.Context, env *domain.Environment) error
	GetEnvironment(ctx context.Context, id string) (*domain.Environment, error)
	ListEnvironments(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error)
	UpdateEnvironment(ctx context.Context, env *domain.Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
	GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error)
	ListJobSecrets(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error)
	ListJobsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Job, error)
	CreateJobDependency(ctx context.Context, dep *domain.JobDependency) error
	ListJobDependencies(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobDependency, error)
	DeleteJobDependency(ctx context.Context, id string) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
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
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue *string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error)
	UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error
	ListChildRuns(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error)
	CreateAPIKey(ctx context.Context, key *domain.APIKey) error
	ListAPIKeysByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	ListJobVersionsByJob(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobVersion, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
	UpdateHeartbeat(ctx context.Context, id string) error
	QueueStats(ctx context.Context) (*store.QueueStats, error)
	CreateWorkflow(ctx context.Context, w *domain.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	ListWorkflows(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Workflow, error)
	UpdateWorkflow(ctx context.Context, w *domain.Workflow) error
	CreateWorkflowVersionSnapshot(ctx context.Context, workflowID string, version int) error
	DeleteWorkflow(ctx context.Context, id string) error
	CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	DeleteStepsByWorkflow(ctx context.Context, workflowID string) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListWorkflowRuns(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error)
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
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	ListRunLineage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.JobRun, error)
	SumRunCostMicrousd(ctx context.Context, runID string) (int64, error)
	SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error)
	CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error
	GetEventTriggerByEventKey(ctx context.Context, eventKey string) (*domain.EventTrigger, error)
	UpdateEventTriggerStatus(ctx context.Context, id string, status string, responsePayload json.RawMessage, receivedAt *time.Time, errMsg string) error
	ListEventTriggersByProject(ctx context.Context, projectID string, status string, limit int, cursor *time.Time) ([]domain.EventTrigger, error)
	CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
}

// Pinger checks service health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// WorkflowCallback is called after a run reaches a terminal state via SDK or cancel.
type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
	OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error
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

// ErrorResponse is the standard error response returned by all API endpoints.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type Server struct {
	router             chi.Router
	store              APIStore
	queue              queue.Queue
	pubsub             pubsub.Publisher
	config             *config.Config
	metricsHandler     http.Handler
	pinger             Pinger
	healthRegistry     *health.Registry
	workflowCallback   WorkflowCallback
	workflowEngine     WorkflowTrigger
	validate           *validator.Validate
	maxRequestBodySize int64
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
}

// NewServer creates a new HTTP API server with the given dependencies.
func NewServer(deps ServerDeps) *Server {
	maxBody := deps.Config.MaxRequestBodySize
	if maxBody <= 0 {
		maxBody = 1 << 20 // 1MB default
	}
	srv := &Server{
		store:              deps.Store,
		queue:              deps.Queue,
		pubsub:             deps.PubSub,
		config:             deps.Config,
		metricsHandler:     deps.MetricsHandler,
		pinger:             deps.Pinger,
		healthRegistry:     deps.HealthRegistry,
		workflowCallback:   deps.WorkflowCallback,
		workflowEngine:     deps.WorkflowEngine,
		validate:           validator.New(validator.WithRequiredStructEnabled()),
		maxRequestBodySize: maxBody,
	}
	srv.router = srv.routes()
	return srv
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

func respondError(w http.ResponseWriter, r *http.Request, status int, message string) {
	var requestID string
	if r != nil {
		requestID = chimw.GetReqID(r.Context())
	}
	respondJSON(w, status, ErrorResponse{
		Error:     message,
		RequestID: requestID,
	})
}

func (s *Server) decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, s.maxRequestBodySize))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// validateURL checks that a URL is valid and doesn't target private networks.
// It performs DNS resolution to prevent DNS rebinding attacks and blocks
// known dangerous hostnames and non-standard ports.
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

	host := u.Hostname()

	// Block known dangerous hostnames
	blockedHosts := []string{"localhost", "metadata.google.internal", "169.254.169.254"}
	for _, blocked := range blockedHosts {
		if strings.EqualFold(host, blocked) {
			return fmt.Errorf("url must not point to internal services")
		}
	}

	// Check IP directly or resolve hostname to verify all resolved IPs
	ip := net.ParseIP(host)
	if ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("url must not point to private or loopback addresses")
		}
	} else {
		ips, err := net.LookupIP(host)
		if err == nil {
			if slices.ContainsFunc(ips, isPrivateIP) {
				return fmt.Errorf("url must not point to private or loopback addresses")
			}
		}
		// DNS resolution failure is not an error — the hostname may not be resolvable yet
		// (e.g., user is setting up their webhook endpoint). Only block confirmed private IPs.
	}

	// Block non-standard ports that might bypass firewalls
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		portNum, err := strconv.Atoi(port)
		if err != nil || portNum < 1 || portNum > 65535 {
			return fmt.Errorf("invalid port number")
		}
		allowedPorts := map[int]bool{80: true, 443: true, 8080: true, 8443: true, 3000: true, 4000: true, 5000: true, 9000: true}
		if !allowedPorts[portNum] {
			return fmt.Errorf("port %d is not allowed for webhooks", portNum)
		}
	}

	return nil
}

// isPrivateIP returns true if the IP is loopback, private, link-local, or unspecified.
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
			respondError(w, r, http.StatusBadRequest, "validation failed: "+strings.Join(messages, ", "))
			return false
		}
		respondError(w, r, http.StatusBadRequest, "invalid request")
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
