package agents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/alitto/pond/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

const (
	backingJobSlugPrefix     = "__agent__"
	backingJobEndpoint       = "https://agents.local.invalid/dispatch"
	defaultMaxConcurrentRuns = 10
)

// cgnatBlock is the Carrier-Grade NAT range (RFC 6598) not covered by net.IP.IsPrivate().
var cgnatBlock = &net.IPNet{IP: net.ParseIP("100.64.0.0"), Mask: net.CIDRMask(10, 32)}

var (
	ErrNotDeployed         = errors.New("agent is not deployed")
	ErrConcurrencyExceeded = errors.New("agent has too many concurrent runs")
	ErrAgentQuotaExceeded  = errors.New("agent quota exceeded for this project")
	ErrRunQuotaExceeded    = errors.New("monthly agent run quota exceeded")
)

// DirectRunResult contains the data needed for client-side dispatch.
type DirectRunResult struct {
	RunID     string                  `json:"run_id"`
	WorkerURL string                  `json:"worker_url"`
	Token     string                  `json:"token"`
	Envelope  RuntimeDispatchEnvelope `json:"envelope"`
}

type ReplayAgentRunRequest struct {
	ProjectID       string
	AgentID         string
	OriginalRunID   string
	ConfigOverrides map[string]any
	FromCheckpoint  int
	Actor           string
}

type Service interface {
	CreateAgent(ctx context.Context, req CreateAgentRequest) (*domain.Agent, error)
	GetAgent(ctx context.Context, projectID, agentID string) (*domain.Agent, error)
	ListAgents(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Agent, error)
	UpdateAgent(ctx context.Context, req UpdateAgentRequest) (*domain.Agent, error)
	DeleteAgent(ctx context.Context, projectID, agentID string) error
	// DeployAgent deploys the agent without pinning to a specific
	// environment. Preserved for backwards compatibility with callers that
	// predate environment binding; new code should use DeployAgentToEnv.
	DeployAgent(ctx context.Context, projectID, agentID, actor string) (*domain.AgentDeployment, error)
	// DeployAgentToEnv deploys the agent to a specific platform environment.
	// Multiple concurrent deployments are allowed, one per environment,
	// so dev/staging/prod promotion flows work without interfering.
	DeployAgentToEnv(ctx context.Context, projectID, agentID, environmentID, actor string) (*domain.AgentDeployment, error)
	RunAgent(ctx context.Context, req RunAgentRequest) (*domain.JobRun, error)
	PrepareDirectRun(ctx context.Context, req RunAgentRequest) (*DirectRunResult, error)
	ListAgentRuns(ctx context.Context, projectID, agentID string, limit, offset int) ([]domain.JobRun, error)
	ReplayAgentRun(ctx context.Context, req ReplayAgentRunRequest) (*domain.JobRun, error)
	Close()
}

type agentStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	DeleteJob(ctx context.Context, id string) error
	CreateAgent(ctx context.Context, agent *domain.Agent) error
	GetAgent(ctx context.Context, id string) (*domain.Agent, error)
	ListAgents(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Agent, error)
	UpdateAgent(ctx context.Context, agent *domain.Agent) error
	DeleteAgent(ctx context.Context, id string) error
	NextAgentDeploymentVersion(ctx context.Context, agentID string) (int, error)
	CreateAgentDeployment(ctx context.Context, deployment *domain.AgentDeployment) error
	GetLatestAgentDeployment(ctx context.Context, agentID string) (*domain.AgentDeployment, error)
	GetLatestAgentDeploymentByEnvironment(ctx context.Context, agentID, environmentID string) (*domain.AgentDeployment, error)
	ListAgentDeployments(ctx context.Context, agentID string, limit int, cursor *time.Time) ([]domain.AgentDeployment, error)
	UpdateAgentDeployment(ctx context.Context, id string, patch map[string]any) error
	ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error)
	ListRunsByJobAndAgentDeployment(ctx context.Context, jobID, agentDeploymentID string, limit, offset int) ([]domain.JobRun, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	AdvisoryXactLock(ctx context.Context, lockID int64) error
	GetAgentDeploymentByID(ctx context.Context, deploymentID string) (*domain.AgentDeployment, error)
	GetActiveAgentCanary(ctx context.Context, agentID string) (*domain.AgentCanaryDeployment, error)
	GetActiveAgentCanaryWithTarget(ctx context.Context, agentID string) (*domain.AgentCanaryDeployment, *domain.AgentDeployment, error)
	GetEnvironment(ctx context.Context, envID string) (*domain.Environment, error)
	ListProjectSecretsByEnv(ctx context.Context, projectID, environmentID string) ([]domain.ProjectSecret, error)
}

type Provider interface {
	Name() string
	Deploy(ctx context.Context, agent *domain.Agent, deployment *domain.AgentDeployment) (json.RawMessage, error)
	Undeploy(ctx context.Context, agent *domain.Agent, deployment *domain.AgentDeployment) error
	Run(ctx context.Context, agent *domain.Agent, deployment *domain.AgentDeployment, run *domain.JobRun) (json.RawMessage, error)
}

type LocalStubProvider struct{}

func (LocalStubProvider) Name() string {
	return ProviderNameLocalStub
}

func (LocalStubProvider) Deploy(_ context.Context, agent *domain.Agent, deployment *domain.AgentDeployment) (json.RawMessage, error) {
	return mustJSON(map[string]any{
		"provider":       ProviderNameLocalStub,
		"agent_id":       agent.ID,
		"deployment_id":  deployment.ID,
		"deployment_ver": deployment.Version,
	}), nil
}

func (LocalStubProvider) Undeploy(context.Context, *domain.Agent, *domain.AgentDeployment) error {
	return nil
}

func (LocalStubProvider) Run(_ context.Context, agent *domain.Agent, deployment *domain.AgentDeployment, run *domain.JobRun) (json.RawMessage, error) {
	var payload any
	if len(run.Payload) > 0 {
		if err := json.Unmarshal(run.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode run payload: %w", err)
		}
	}

	if m, ok := payload.(map[string]any); ok {
		if raw, exists := m["_stub_error"]; exists {
			return nil, fmt.Errorf("stub provider error: %v", raw)
		}
	}

	return mustJSON(map[string]any{
		"agent_id":           agent.ID,
		"agent_slug":         agent.Slug,
		"deployment_id":      deployment.ID,
		"deployment_version": deployment.Version,
		"provider":           ProviderNameLocalStub,
		"received_payload":   payload,
	}), nil
}

// QuotaChecker provides project quota information for agent enforcement.
type QuotaChecker interface {
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	CountProjectRunsSince(ctx context.Context, projectID string, since time.Time) (int, error)
}

// AgentBillingEnforcer checks agent-specific billing limits before dispatch.
type AgentBillingEnforcer interface {
	CheckAgentSpendingLimit(ctx context.Context, projectID string) error
	GetAgentPlanForProject(ctx context.Context, projectID string) (string, error)
}

// WebhookDeliveryStore creates durable webhook delivery records that the
// existing DeliveryWorker processes with exponential backoff.
type WebhookDeliveryStore interface {
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
}

type localService struct {
	store             agentStore
	txb               store.TxBeginner
	p                 Provider
	runtime           RuntimeRunner
	callbacks         RuntimeCallbackClient
	dispatchHTTP      *http.Client
	internalSecret    string
	now               func() time.Time
	apiBaseURL        string
	jwtSigningKey     string
	dispatchPool      pond.Pool
	maxConcurrentRuns int
	quotaChecker      QuotaChecker
	billingEnforcer   AgentBillingEnforcer
	canaryRouter      *AgentCanaryRouter
	webhookStore      WebhookDeliveryStore
}

type Option func(*localService)

type CreateAgentRequest struct {
	ProjectID       string
	Name            string
	Slug            string
	Description     string
	Model           string
	ModelFallbacks  []string
	Config          json.RawMessage
	ProviderSecrets map[string]string
	Cron            string
	CronTimezone    string
	Actor           string
}

type UpdateAgentRequest struct {
	ProjectID       string
	AgentID         string
	Name            string
	Slug            string
	Description     string
	Model           string
	ModelFallbacks  []string
	Config          json.RawMessage
	ProviderSecrets map[string]string
	Cron            string
	CronTimezone    string
	Actor           string
}

type RunAgentRequest struct {
	ProjectID string
	AgentID   string
	Payload   json.RawMessage
	Actor     string
	// EnvironmentID, when set, targets a specific environment's deployment
	// for this run. If empty, the service falls back to the most recent
	// deployment across all environments (legacy behavior for agents that
	// have no environment_id yet).
	EnvironmentID string
}

func NewService(q *store.Queries, txb store.TxBeginner, opts ...Option) Service {
	svc := &localService{
		store:        q,
		txb:          txb,
		p:            LocalStubProvider{},
		now:          time.Now,
		dispatchPool: pond.NewPool(4),
		apiBaseURL:   "http://127.0.0.1:8080",
		dispatchHTTP: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	if svc.runtime == nil {
		svc.runtime = NewCommandRuntimeRunner(CommandRuntimeOptions{})
	}
	if svc.callbacks == nil {
		svc.callbacks = NewHTTPCallbackClient(svc.apiBaseURL, &http.Client{Timeout: 15 * time.Second})
	}
	if svc.maxConcurrentRuns <= 0 {
		svc.maxConcurrentRuns = defaultMaxConcurrentRuns
	}
	return svc
}

func WithProvider(p Provider) Option {
	return func(s *localService) {
		if p != nil {
			s.p = p
		}
	}
}

func WithRuntimeRunner(r RuntimeRunner) Option {
	return func(s *localService) {
		if r != nil {
			s.runtime = r
		}
	}
}

func WithCallbackClient(c RuntimeCallbackClient) Option {
	return func(s *localService) {
		if c != nil {
			s.callbacks = c
		}
	}
}

func WithClock(now func() time.Time) Option {
	return func(s *localService) {
		if now != nil {
			s.now = now
		}
	}
}

func WithAPIBaseURL(baseURL string) Option {
	return func(s *localService) {
		if baseURL != "" {
			s.apiBaseURL = baseURL
		}
	}
}

func WithJWTSigningKey(key string) Option {
	return func(s *localService) {
		s.jwtSigningKey = key
	}
}

func WithDispatchPool(pool pond.Pool) Option {
	return func(s *localService) {
		if pool != nil {
			s.dispatchPool = pool
		}
	}
}

func WithInternalSecret(secret string) Option {
	return func(s *localService) {
		s.internalSecret = secret
	}
}

func WithDispatchHTTPClient(client *http.Client) Option {
	return func(s *localService) {
		if client != nil {
			s.dispatchHTTP = client
		}
	}
}

func WithQuotaChecker(qc QuotaChecker) Option {
	return func(s *localService) {
		if qc != nil {
			s.quotaChecker = qc
		}
	}
}

func WithMaxConcurrentRuns(n int) Option {
	return func(s *localService) {
		if n > 0 {
			s.maxConcurrentRuns = n
		}
	}
}

func WithWebhookStore(ws WebhookDeliveryStore) Option {
	return func(s *localService) {
		if ws != nil {
			s.webhookStore = ws
		}
	}
}

func WithCanaryRouter(cr *AgentCanaryRouter) Option {
	return func(s *localService) {
		if cr != nil {
			s.canaryRouter = cr
		}
	}
}

func WithBillingEnforcer(be AgentBillingEnforcer) Option {
	return func(s *localService) {
		if be != nil {
			s.billingEnforcer = be
		}
	}
}

func (s *localService) Close() {
	if s.dispatchPool != nil {
		s.dispatchPool.StopAndWait()
	}
}

func (s *localService) CreateAgent(ctx context.Context, req CreateAgentRequest) (*domain.Agent, error) {
	if err := validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check agent quota from project quotas table.
	if s.quotaChecker != nil {
		quota, qErr := s.quotaChecker.GetProjectQuota(ctx, req.ProjectID)
		if qErr == nil && quota != nil && quota.MaxAgents > 0 {
			existing, listErr := s.store.ListAgents(ctx, req.ProjectID, quota.MaxAgents+1, nil)
			if listErr == nil && len(existing) >= quota.MaxAgents {
				return nil, ErrAgentQuotaExceeded
			}
		}
	}

	// Check agent definition limit from billing plan.
	if s.billingEnforcer != nil {
		agentTier, tierErr := s.billingEnforcer.GetAgentPlanForProject(ctx, req.ProjectID)
		if tierErr != nil {
			slog.Warn("agent plan lookup failed, defaulting to free", "project_id", req.ProjectID, "error", tierErr)
		}
		limits := billing.GetAgentPlanLimits(domain.PlanTier(agentTier))
		if limits.MaxAgentDefinitions > 0 { // -1 = unlimited
			existing, listErr := s.store.ListAgents(ctx, req.ProjectID, limits.MaxAgentDefinitions+1, nil)
			if listErr == nil && len(existing) >= limits.MaxAgentDefinitions {
				return nil, ErrAgentQuotaExceeded
			}
		}
	}

	var created *domain.Agent
	err := store.WithTx(ctx, s.txb, func(txQ *store.Queries) error {
		backingJob := buildBackingJob(req)
		if err := txQ.CreateJob(ctx, backingJob); err != nil {
			if errors.Is(err, store.ErrJobSlugConflict) {
				return store.ErrAgentSlugConflict
			}
			return err
		}

		var encryptedSecrets string
		if len(req.ProviderSecrets) > 0 {
			var encErr error
			encryptedSecrets, encErr = txQ.EncryptAgentProviderSecrets(req.ProviderSecrets)
			if encErr != nil {
				return fmt.Errorf("encrypt provider secrets: %w", encErr)
			}
		}

		agent := &domain.Agent{
			ID:                       uuid.Must(uuid.NewV7()).String(),
			ProjectID:                req.ProjectID,
			JobID:                    backingJob.ID,
			Name:                     req.Name,
			Slug:                     req.Slug,
			Description:              req.Description,
			Model:                    req.Model,
			ModelFallbacks:           req.ModelFallbacks,
			Config:                   normalizedConfig(req.Config),
			ProviderSecretsEncrypted: encryptedSecrets,
			CreatedBy:                req.Actor,
			UpdatedBy:                req.Actor,
		}
		if err := txQ.CreateAgent(ctx, agent); err != nil {
			return err
		}
		created = agent
		return nil
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *localService) GetAgent(ctx context.Context, projectID, agentID string) (*domain.Agent, error) {
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if agent.ProjectID != projectID {
		return nil, store.ErrAgentNotFound
	}
	return agent, nil
}

func (s *localService) ListAgents(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Agent, error) {
	return s.store.ListAgents(ctx, projectID, limit, cursor)
}

func (s *localService) UpdateAgent(ctx context.Context, req UpdateAgentRequest) (*domain.Agent, error) {
	if err := validateUpdateRequest(req); err != nil {
		return nil, err
	}

	agent, err := s.GetAgent(ctx, req.ProjectID, req.AgentID)
	if err != nil {
		return nil, err
	}
	job, err := s.store.GetJob(ctx, agent.JobID)
	if err != nil {
		return nil, err
	}

	updated := *agent
	updated.Name = req.Name
	updated.Slug = req.Slug
	updated.Description = req.Description
	updated.Model = req.Model
	updated.Config = normalizedConfig(req.Config)
	updated.UpdatedBy = req.Actor

	err = store.WithTx(ctx, s.txb, func(txQ *store.Queries) error {
		job.Name = "[internal] agent " + updated.Name
		job.Slug = backingJobSlug(updated.Slug)
		job.Description = "Internal backing job for agent " + updated.Slug
		job.Tags = map[string]string{
			"strait_internal": "agent",
			"agent_slug":      updated.Slug,
		}
		job.Cron = req.Cron
		job.Timezone = req.CronTimezone
		job.Enabled = req.Cron != ""
		job.UpdatedBy = req.Actor
		if err := txQ.UpdateJob(ctx, job); err != nil {
			if errors.Is(err, store.ErrJobSlugConflict) {
				return store.ErrAgentSlugConflict
			}
			return err
		}
		return txQ.UpdateAgent(ctx, &updated)
	})
	if err != nil {
		return nil, err
	}

	return &updated, nil
}

func (s *localService) DeleteAgent(ctx context.Context, projectID, agentID string) error {
	agent, err := s.GetAgent(ctx, projectID, agentID)
	if err != nil {
		return err
	}

	// Cancel any active runs before deleting the agent.
	runs, err := s.store.ListRunsByJob(ctx, agent.JobID, 500, 0)
	if err != nil {
		return fmt.Errorf("list agent runs for deletion: %w", err)
	}
	now := s.now().UTC()
	for _, run := range runs {
		if run.Status.IsTerminal() {
			continue
		}
		if cancelErr := s.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusCanceled, map[string]any{
			"finished_at": now,
		}); cancelErr != nil {
			_ = s.store.InsertEvent(ctx, &domain.RunEvent{
				RunID:   run.ID,
				Type:    domain.EventError,
				Level:   "warn",
				Message: fmt.Sprintf("failed to cancel run during agent deletion: %v", cancelErr),
			})
		}
	}

	// Best-effort undeploy of all deployments. Log failures but continue
	// so that a single provider error does not block agent deletion.
	deployments, err := s.store.ListAgentDeployments(ctx, agent.ID, 100, nil)
	if err != nil {
		return err
	}
	for _, deployment := range deployments {
		deploymentCopy := deployment
		if undeployErr := s.p.Undeploy(ctx, agent, &deploymentCopy); undeployErr != nil {
			_ = s.store.InsertEvent(ctx, &domain.RunEvent{
				RunID:   agent.ID,
				Type:    domain.EventError,
				Level:   "warn",
				Message: fmt.Sprintf("best-effort undeploy failed for deployment %s: %v", deploymentCopy.ID, undeployErr),
			})
		}
	}

	return store.WithTx(ctx, s.txb, func(txQ *store.Queries) error {
		if err := txQ.DeleteAgent(ctx, agent.ID); err != nil {
			return err
		}
		return txQ.DeleteJob(ctx, agent.JobID)
	})
}

func (s *localService) DeployAgent(ctx context.Context, projectID, agentID, actor string) (*domain.AgentDeployment, error) {
	return s.DeployAgentToEnv(ctx, projectID, agentID, "", actor)
}

func (s *localService) DeployAgentToEnv(ctx context.Context, projectID, agentID, environmentID, actor string) (*domain.AgentDeployment, error) {
	agent, err := s.GetAgent(ctx, projectID, agentID)
	if err != nil {
		return nil, err
	}

	var deployment *domain.AgentDeployment
	err = store.WithTx(ctx, s.txb, func(txQ *store.Queries) error {
		if err := txQ.AdvisoryXactLock(ctx, advisoryLockID(agent.ID)); err != nil {
			return fmt.Errorf("lock agent deployment: %w", err)
		}

		// Validate the environment belongs to the agent's project so a
		// caller can't cross-project a deploy by passing someone else's
		// environment_id.
		if environmentID != "" {
			env, envErr := txQ.GetEnvironment(ctx, environmentID)
			if envErr != nil {
				return fmt.Errorf("resolve deployment environment: %w", envErr)
			}
			if env.ProjectID != agent.ProjectID {
				return fmt.Errorf("environment %s does not belong to project %s", environmentID, agent.ProjectID)
			}
		}

		version, err := txQ.NextAgentDeploymentVersion(ctx, agent.ID)
		if err != nil {
			return err
		}

		deployment = &domain.AgentDeployment{
			ID:             uuid.Must(uuid.NewV7()).String(),
			AgentID:        agent.ID,
			EnvironmentID:  environmentID,
			Version:        version,
			Status:         domain.AgentDeploymentStatusPending,
			Provider:       s.p.Name(),
			ConfigSnapshot: agent.Config,
			CreatedBy:      actor,
		}
		if err := txQ.CreateAgentDeployment(ctx, deployment); err != nil {
			return err
		}

		providerMeta, err := s.p.Deploy(ctx, agent, deployment)
		if err != nil {
			_ = txQ.UpdateAgentDeployment(ctx, deployment.ID, map[string]any{
				"status": domain.AgentDeploymentStatusFailed,
			})
			return err
		}
		now := s.now().UTC()
		deployment.Status = domain.AgentDeploymentStatusDeployed
		deployment.ProviderMetadata = providerMeta
		deployment.DeployedAt = &now
		return txQ.UpdateAgentDeployment(ctx, deployment.ID, map[string]any{
			"status":            string(domain.AgentDeploymentStatusDeployed),
			"provider_metadata": providerMeta,
			"deployed_at":       now,
		})
	})
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

func (s *localService) RunAgent(ctx context.Context, req RunAgentRequest) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "agents.RunAgent")
	defer span.End()

	if err := validateRunRequest(req); err != nil {
		return nil, err
	}

	// Check monthly run quota.
	if s.quotaChecker != nil {
		quota, qErr := s.quotaChecker.GetProjectQuota(ctx, req.ProjectID)
		if qErr == nil && quota != nil && quota.MaxAgentRunsPerMonth > 0 {
			monthStart := beginningOfMonth(s.now())
			count, countErr := s.quotaChecker.CountProjectRunsSince(ctx, req.ProjectID, monthStart)
			if countErr == nil && count >= quota.MaxAgentRunsPerMonth {
				return nil, ErrRunQuotaExceeded
			}
		}
	}

	// Resolve the per-request billing snapshot once. This collapses the
	// two separate billing-enforcer queries (spending-limit check and
	// plan-tier lookup) into a single pass so RunAgent hits
	// org_subscriptions once instead of twice. Also applied in
	// PrepareDirectRun so code-paths stay symmetric.
	snapshot, err := s.loadAgentEnforcement(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}

	agent, err := s.GetAgent(ctx, req.ProjectID, req.AgentID)
	if err != nil {
		return nil, err
	}
	job, err := s.store.GetJob(ctx, agent.JobID)
	if err != nil {
		return nil, err
	}

	// Resolve the target deployment. If the caller specified an environment,
	// pick the latest deployment pinned to that environment. Otherwise fall
	// back to the most recent deployment across all envs (legacy behavior
	// for agents without environment bindings).
	var deployment *domain.AgentDeployment
	if req.EnvironmentID != "" {
		deployment, err = s.store.GetLatestAgentDeploymentByEnvironment(ctx, agent.ID, req.EnvironmentID)
	} else {
		deployment, err = s.store.GetLatestAgentDeployment(ctx, agent.ID)
	}
	if err != nil {
		return nil, ErrNotDeployed
	}
	if deployment.Status != domain.AgentDeploymentStatusDeployed {
		return nil, ErrNotDeployed
	}

	// Route via canary if active (probabilistic traffic splitting). Canary
	// targets must share the primary's environment so prod traffic never
	// gets routed into a dev deployment by mistake.
	if altDeploy := s.resolveCanaryTarget(ctx, agent.ID, deployment); altDeploy != nil {
		deployment = altDeploy
	}

	// Enforce per-deployment concurrency limit using the snapshot's plan
	// limits. Per-deployment so dev and prod deployments of the same
	// agent don't starve each other's budgets.
	maxConcurrent := s.maxConcurrentRuns
	if snapshot.limits.MaxAgentConcurrentRuns > 0 {
		maxConcurrent = snapshot.limits.MaxAgentConcurrentRuns
	}
	existingRuns, err := s.listRunsForConcurrency(ctx, agent.JobID, deployment.ID, maxConcurrent+1)
	if err != nil {
		return nil, fmt.Errorf("check agent concurrency: %w", err)
	}
	activeCount := 0
	for _, r := range existingRuns {
		if !r.Status.IsTerminal() {
			activeCount++
		}
	}
	if activeCount >= maxConcurrent {
		return nil, ErrConcurrencyExceeded
	}

	run := &domain.JobRun{
		ID:                uuid.Must(uuid.NewV7()).String(),
		JobID:             agent.JobID,
		ProjectID:         agent.ProjectID,
		Status:            domain.StatusQueued,
		Attempt:           1,
		Payload:           req.Payload,
		TriggeredBy:       domain.TriggerManual,
		JobVersion:        job.Version,
		JobVersionID:      job.VersionID,
		ExecutionMode:     job.ExecutionMode,
		CreatedBy:         req.Actor,
		AgentDeploymentID: deployment.ID,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   run.ID,
		Type:    domain.EventStateChange,
		Level:   "info",
		Message: "agent run queued",
		Data: mustJSON(map[string]any{
			"agent_id":       agent.ID,
			"deployment_id":  deployment.ID,
			"deployment_ver": deployment.Version,
		}),
	})

	if s.dispatchPool != nil {
		agentCopy := *agent
		jobCopy := *job
		deploymentCopy := *deployment
		runID := run.ID
		s.dispatchPool.Submit(func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("agent dispatch panic recovered", "run_id", runID, "panic", r)
					s.markRuntimeSystemFailed(context.Background(), runID, fmt.Sprintf("dispatch panic: %v", r))
				}
			}()
			s.dispatchRun(context.Background(), &agentCopy, &jobCopy, &deploymentCopy, runID)
		})
	}
	return run, nil
}

func (s *localService) PrepareDirectRun(ctx context.Context, req RunAgentRequest) (*DirectRunResult, error) {
	if err := validateRunRequest(req); err != nil {
		return nil, err
	}

	agent, err := s.GetAgent(ctx, req.ProjectID, req.AgentID)
	if err != nil {
		return nil, err
	}
	job, err := s.store.GetJob(ctx, agent.JobID)
	if err != nil {
		return nil, err
	}
	deployment, err := s.store.GetLatestAgentDeployment(ctx, agent.ID)
	if err != nil {
		return nil, ErrNotDeployed
	}
	if deployment.Status != domain.AgentDeploymentStatusDeployed {
		return nil, ErrNotDeployed
	}

	// Create the run but do NOT dispatch it.
	run := &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         agent.JobID,
		ProjectID:     agent.ProjectID,
		Status:        domain.StatusQueued,
		Attempt:       1,
		Payload:       req.Payload,
		TriggeredBy:   domain.TriggerManual,
		JobVersion:    job.Version,
		JobVersionID:  job.VersionID,
		ExecutionMode: job.ExecutionMode,
		CreatedBy:     req.Actor,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	envelope, token, err := s.buildRuntimeEnvelope(ctx, agent, job, deployment, run)
	if err != nil {
		return nil, fmt.Errorf("build runtime envelope: %w", err)
	}

	// Determine the worker URL from deployment metadata.
	workerURL := s.apiBaseURL + "/v1/agents/" + agent.ID + "/run"
	if deployment.Provider == ProviderNameCloudflare {
		metadata, parseErr := ParseCloudflareDeploymentMetadata(deployment.ProviderMetadata)
		if parseErr == nil {
			workerURL = metadata.DispatchWorkerURL
		}
	}

	return &DirectRunResult{
		RunID:     run.ID,
		WorkerURL: workerURL,
		Token:     token,
		Envelope:  envelope,
	}, nil
}

func (s *localService) ListAgentRuns(ctx context.Context, projectID, agentID string, limit, offset int) ([]domain.JobRun, error) {
	agent, err := s.GetAgent(ctx, projectID, agentID)
	if err != nil {
		return nil, err
	}
	return s.store.ListRunsByJob(ctx, agent.JobID, limit, offset)
}

// listRunsForConcurrency returns recent runs for the agent's backing job,
// scoped to a specific deployment when one is given. A backing job may host
// multiple deployments (one per environment), and concurrency must be
// enforced per deployment so dev and prod don't starve each other.
func (s *localService) listRunsForConcurrency(ctx context.Context, jobID, deploymentID string, limit int) ([]domain.JobRun, error) {
	if deploymentID != "" {
		return s.store.ListRunsByJobAndAgentDeployment(ctx, jobID, deploymentID, limit, 0)
	}
	return s.store.ListRunsByJob(ctx, jobID, limit, 0)
}

// agentEnforcementSnapshot holds the per-request billing state for a
// RunAgent/PrepareDirectRun/ReplayAgentRun call. Resolving it once at
// the top of the flow keeps the billing enforcer from being queried
// twice (CheckAgentSpendingLimit + GetAgentPlanForProject) per request
// and gives every downstream concurrency/limit check a consistent view.
type agentEnforcementSnapshot struct {
	tier   domain.PlanTier
	limits billing.OrgPlanLimits
}

// loadAgentEnforcement resolves the billing snapshot for a project. If
// the billing enforcer is absent (test contexts), returns an empty
// snapshot that lets existing tests run unchanged. Spending-limit
// failures are returned unwrapped so RunAgent can translate them to
// ErrSpendingLimitExceeded (or similar) at the handler layer.
func (s *localService) loadAgentEnforcement(ctx context.Context, projectID string) (*agentEnforcementSnapshot, error) {
	snapshot := &agentEnforcementSnapshot{
		tier:   domain.AgentPlanFree,
		limits: billing.GetAgentPlanLimits(domain.AgentPlanFree),
	}
	if s.billingEnforcer == nil {
		return snapshot, nil
	}
	if err := s.billingEnforcer.CheckAgentSpendingLimit(ctx, projectID); err != nil {
		return nil, err
	}
	tier, tierErr := s.billingEnforcer.GetAgentPlanForProject(ctx, projectID)
	if tierErr != nil {
		slog.Warn("agent plan lookup failed, defaulting to free", "project_id", projectID, "error", tierErr)
		return snapshot, nil
	}
	if tier == "" {
		tier = string(domain.AgentPlanFree)
	}
	snapshot.tier = domain.PlanTier(tier)
	snapshot.limits = billing.GetAgentPlanLimits(snapshot.tier)
	return snapshot, nil
}

// resolveCanaryTarget returns a canary deployment to swap in for the
// primary, or nil if no swap should occur. The canary target must share
// the primary's environment — a mismatch is logged and skipped so prod
// traffic never crosses into a dev deployment by mistake.
//
// Uses the single-roundtrip GetActiveAgentCanaryWithTarget to avoid the
// per-RunAgent N+1 from the original implementation (one query for the
// canary row, then a separate query for its target deployment).
func (s *localService) resolveCanaryTarget(ctx context.Context, agentID string, primary *domain.AgentDeployment) *domain.AgentDeployment {
	if s.canaryRouter == nil {
		return nil
	}
	canary, target, err := s.store.GetActiveAgentCanaryWithTarget(ctx, agentID)
	if err != nil {
		slog.Warn("failed to get active canary", "agent_id", agentID, "error", err)
		return nil
	}
	if canary == nil || target == nil {
		return nil
	}
	targetID := s.canaryRouter.Route(canary)
	if targetID == "" || targetID == primary.ID {
		return nil
	}
	// The router may pick the source deployment instead of the target.
	// If it picks the target we already have it; otherwise fall back to
	// a single lookup (rare path).
	if targetID != target.ID {
		altDeploy, altErr := s.store.GetAgentDeploymentByID(ctx, targetID)
		if altErr != nil || altDeploy == nil {
			return nil
		}
		target = altDeploy
	}
	if target.EnvironmentID != primary.EnvironmentID {
		slog.Warn("canary target skipped: env mismatch",
			"agent_id", agentID,
			"primary_env", primary.EnvironmentID,
			"canary_env", target.EnvironmentID)
		return nil
	}
	return target
}

func (s *localService) ReplayAgentRun(ctx context.Context, req ReplayAgentRunRequest) (*domain.JobRun, error) {
	agent, err := s.GetAgent(ctx, req.ProjectID, req.AgentID)
	if err != nil {
		return nil, err
	}

	originalRun, err := s.store.GetRun(ctx, req.OriginalRunID)
	if err != nil {
		return nil, fmt.Errorf("get original run: %w", err)
	}
	if !originalRun.Status.IsTerminal() {
		return nil, fmt.Errorf("can only replay terminal runs (current: %s)", originalRun.Status)
	}

	// Carry forward the original run's deployment so a prod failure
	// replays into prod, not dev. Fall back to the latest deployment if
	// the original run predates per-deployment stamping.
	var deployment *domain.AgentDeployment
	if originalRun.AgentDeploymentID != "" {
		deployment, err = s.store.GetAgentDeploymentByID(ctx, originalRun.AgentDeploymentID)
	} else {
		deployment, err = s.store.GetLatestAgentDeployment(ctx, agent.ID)
	}
	if err != nil {
		return nil, ErrNotDeployed
	}
	if deployment.Status != domain.AgentDeploymentStatusDeployed {
		return nil, ErrNotDeployed
	}

	// Determine payload: use checkpoint state if requested, otherwise original payload.
	payload := originalRun.Payload
	if req.FromCheckpoint > 0 {
		cp, cpErr := s.store.GetLatestCheckpoint(ctx, req.OriginalRunID)
		if cpErr != nil || cp == nil {
			return nil, fmt.Errorf("checkpoint not found for run %s", req.OriginalRunID)
		}
		payload = cp.State
	}

	// Apply config overrides if provided. Filter through allowlist to prevent
	// injection of webhook_url, webhook_secret, sandbox, or provider_secrets.
	agentForRun := *agent
	if len(req.ConfigOverrides) > 0 {
		safeOverrides := FilterAllowedReplayKeys(req.ConfigOverrides)
		var existingCfg map[string]any
		if len(agent.Config) > 0 {
			_ = json.Unmarshal(agent.Config, &existingCfg)
		}
		if existingCfg == nil {
			existingCfg = make(map[string]any)
		}
		maps.Copy(existingCfg, safeOverrides)
		merged, _ := json.Marshal(existingCfg)
		agentForRun.Config = merged

		// If model override is specified, apply at agent level too.
		if m, ok := safeOverrides["model"].(string); ok && m != "" {
			agentForRun.Model = m
		}
	}

	job, err := s.store.GetJob(ctx, agent.JobID)
	if err != nil {
		return nil, err
	}

	run := &domain.JobRun{
		ID:                uuid.Must(uuid.NewV7()).String(),
		JobID:             agent.JobID,
		ProjectID:         agent.ProjectID,
		Status:            domain.StatusQueued,
		Attempt:           1,
		Payload:           payload,
		TriggeredBy:       "replay",
		JobVersion:        job.Version,
		JobVersionID:      job.VersionID,
		ExecutionMode:     job.ExecutionMode,
		CreatedBy:         req.Actor,
		AgentDeploymentID: deployment.ID,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   run.ID,
		Type:    domain.EventStateChange,
		Level:   "info",
		Message: "agent run replayed",
		Data: mustJSON(map[string]any{
			"original_run_id": req.OriginalRunID,
			"from_checkpoint": req.FromCheckpoint,
			"has_overrides":   len(req.ConfigOverrides) > 0,
		}),
	})

	if s.dispatchPool != nil {
		agentCopy := agentForRun
		jobCopy := *job
		deploymentCopy := *deployment
		replayRunID := run.ID
		s.dispatchPool.Submit(func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("agent replay dispatch panic recovered", "run_id", replayRunID, "panic", r)
					s.markRuntimeSystemFailed(context.Background(), replayRunID, fmt.Sprintf("dispatch panic: %v", r))
				}
			}()
			s.dispatchRun(context.Background(), &agentCopy, &jobCopy, &deploymentCopy, replayRunID)
		})
	}

	return run, nil
}

func buildBackingJob(req CreateAgentRequest) *domain.Job {
	maxAttempts := 1
	timeoutSecs := 300
	if len(req.Config) > 0 {
		var cfg map[string]any
		if err := json.Unmarshal(req.Config, &cfg); err == nil {
			if v, ok := cfg["max_attempts"].(float64); ok && v >= 1 && v <= 10 {
				maxAttempts = int(v)
			}
			if v, ok := cfg["timeout_secs"].(float64); ok && v >= 1 && v <= 3600 {
				timeoutSecs = int(v)
			}
		}
	}

	job := &domain.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   req.ProjectID,
		Name:        "[internal] agent " + req.Name,
		Slug:        backingJobSlug(req.Slug),
		Description: "Internal backing job for agent " + req.Slug,
		PayloadSchema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":true
		}`),
		Tags: map[string]string{
			"strait_internal": "agent",
			"agent_slug":      req.Slug,
		},
		EndpointURL:   backingJobEndpoint,
		MaxAttempts:   maxAttempts,
		TimeoutSecs:   timeoutSecs,
		Enabled:       false,
		ExecutionMode: domain.ExecutionModeHTTP,
		CreatedBy:     req.Actor,
		UpdatedBy:     req.Actor,
	}
	if req.Cron != "" {
		job.Cron = req.Cron
		job.Timezone = req.CronTimezone
		job.Enabled = true
	}
	return job
}

func backingJobSlug(agentSlug string) string {
	return backingJobSlugPrefix + agentSlug
}

func normalizedConfig(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func beginningOfMonth(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func advisoryLockID(value string) int64 {
	sum := sha256.Sum256([]byte(value))
	return int64(binary.BigEndian.Uint64(sum[:8]) & ((1 << 63) - 1))
}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

func (s *localService) dispatchRun(ctx context.Context, agent *domain.Agent, job *domain.Job, deployment *domain.AgentDeployment, runID string) {
	if err := s.transitionRunToExecuting(ctx, runID); err != nil {
		return
	}

	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return
	}

	envelope, token, err := s.buildRuntimeEnvelope(ctx, agent, job, deployment, run)
	if err != nil {
		s.markRuntimeSystemFailed(ctx, runID, fmt.Sprintf("build runtime envelope: %v", err))
		return
	}

	if deployment.Provider == ProviderNameCloudflare {
		if err := s.dispatchCloudflareRun(ctx, deployment, envelope); err != nil {
			s.markRuntimeSystemFailed(ctx, runID, err.Error())
			return
		}
		_ = s.store.InsertEvent(ctx, &domain.RunEvent{
			RunID:   runID,
			Type:    domain.EventStateChange,
			Level:   "info",
			Message: "agent run dispatched to cloudflare runtime",
			Data: mustJSON(map[string]any{
				"deployment_id": deployment.ID,
				"provider":      deployment.Provider,
				"token_len":     len(token),
			}),
		})
		return
	}

	state := &runtimeEventState{}
	err = s.runtime.Run(ctx, envelope, func(handlerCtx context.Context, event RuntimeEvent) error {
		if validateErr := state.Validate(&event); validateErr != nil {
			return fmt.Errorf("validate runtime event: %w", validateErr)
		}
		_, sendErr := s.callbacks.Send(handlerCtx, run.ID, token, event)
		if sendErr != nil {
			return fmt.Errorf("forward runtime event %s: %w", event.Type, sendErr)
		}
		return nil
	})
	if err != nil {
		s.markRuntimeSystemFailed(ctx, runID, err.Error())
		return
	}
	if !state.terminalResult {
		s.markRuntimeSystemFailed(ctx, runID, "runtime exited without terminal event")
		return
	}

	// Fire webhook notification on successful terminal state (local path only;
	// Cloudflare runs fire webhooks from the /complete and /fail callback handlers).
	s.fireAgentWebhook(ctx, agent, runID)
}

func (s *localService) transitionRunToExecuting(ctx context.Context, runID string) error {
	if err := s.store.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{}); err != nil {
		return err
	}
	startedAt := s.now().UTC()
	if err := s.store.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": startedAt,
	}); err != nil {
		return err
	}
	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   runID,
		Type:    domain.EventStateChange,
		Level:   "info",
		Message: "agent run started",
		Data:    mustJSON(map[string]any{"status": domain.StatusExecuting}),
	})
	return nil
}

func (s *localService) buildRuntimeEnvelope(ctx context.Context, agent *domain.Agent, job *domain.Job, deployment *domain.AgentDeployment, run *domain.JobRun) (RuntimeDispatchEnvelope, string, error) {
	token, err := s.generateRunToken(run.ID, agent.ID, job.TimeoutSecs, run.ExpiresAt)
	if err != nil {
		return RuntimeDispatchEnvelope{}, "", err
	}

	var sandboxPolicy json.RawMessage
	if deployment.Provider == ProviderNameCloudflare {
		metadata, parseErr := ParseCloudflareDeploymentMetadata(deployment.ProviderMetadata)
		if parseErr != nil {
			return RuntimeDispatchEnvelope{}, "", fmt.Errorf("parse cloudflare deployment metadata: %w", parseErr)
		}
		sandboxPolicy = mustJSON(metadata.SandboxPolicy)
	}

	envelope := RuntimeDispatchEnvelope{
		Version: runtimeContractVersion,
		Run: RuntimeDispatchRun{
			ID:          run.ID,
			ProjectID:   run.ProjectID,
			Attempt:     run.Attempt,
			TimeoutSecs: job.TimeoutSecs,
		},
		Agent: RuntimeDispatchAgent{
			ID:             agent.ID,
			Slug:           agent.Slug,
			Model:          agent.Model,
			ModelFallbacks: agent.ModelFallbacks,
			Config:         agent.Config,
			ProviderKeys:   s.decryptProviderSecrets(ctx, agent),
		},
		Deployment: RuntimeDispatchDeployment{
			ID:             deployment.ID,
			Version:        deployment.Version,
			Provider:       deployment.Provider,
			ConfigSnapshot: deployment.ConfigSnapshot,
			SandboxPolicy:  sandboxPolicy,
		},
		Payload: run.Payload,
		Callback: RuntimeDispatchCallback{
			BaseURL:  s.apiBaseURL,
			RunID:    run.ID,
			RunToken: token,
		},
	}

	// Populate env-scoped project secrets. Only fetches when the
	// deployment is bound to an environment; legacy agents without an
	// env binding get no secrets (they can migrate to DeployAgentToEnv).
	// Fail-open: secret lookup errors log a warning but don't block the
	// run — matching the existing Jobs worker secrets-load behavior.
	if deployment.EnvironmentID != "" {
		secrets, secretsErr := s.store.ListProjectSecretsByEnv(ctx, agent.ProjectID, deployment.EnvironmentID)
		if secretsErr != nil {
			slog.Warn("failed to load project secrets for agent run",
				"run_id", run.ID,
				"environment_id", deployment.EnvironmentID,
				"error", secretsErr)
		} else if len(secrets) > 0 {
			envelope.Secrets = make(map[string]string, len(secrets))
			for _, s := range secrets {
				envelope.Secrets[s.SecretKey] = s.EncryptedValue
			}
		}
	}

	if run.Attempt > 1 {
		envelope.Retry = &RuntimeDispatchRetry{}
		cp, cpErr := s.store.GetLatestCheckpoint(ctx, run.ID)
		if cpErr == nil && cp != nil {
			envelope.Retry.LastCheckpoint = cp.State
			envelope.Retry.CheckpointAt = cp.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if run.Error != "" {
			envelope.Retry.PreviousError = run.Error
		}
		if envelope.Retry.LastCheckpoint == nil && envelope.Retry.CheckpointAt == "" && envelope.Retry.PreviousError == "" {
			envelope.Retry = nil
		}
	}

	return envelope, token, nil
}

// agentRunClaims extends the standard JWT claims with an agent ID so that
// SDK callback handlers can cryptographically verify which agent the
// runtime is acting on behalf of.
type agentRunClaims struct {
	jwt.RegisteredClaims
	AgentID string `json:"agent_id,omitempty"`
}

func (s *localService) generateRunToken(runID, agentID string, timeoutSecs int, expiresAt *time.Time) (string, error) {
	if s.jwtSigningKey == "" {
		return "", fmt.Errorf("JWT signing key is not configured")
	}
	exp := s.now().Add(time.Duration(timeoutSecs) * time.Second).Add(60 * time.Second)
	if expiresAt != nil {
		exp = *expiresAt
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, agentRunClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "strait-agents",
			Audience:  jwt.ClaimStrings{"strait-sdk"},
			Subject:   runID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(s.now()),
		},
		AgentID: agentID,
	})
	signed, err := token.SignedString([]byte(s.jwtSigningKey))
	if err != nil {
		return "", fmt.Errorf("sign runtime run token: %w", err)
	}
	return signed, nil
}

// classifyRuntimeError inspects an error message from the Cloudflare Worker
// runtime and returns a classification and optional user-facing suggestion.
func classifyRuntimeError(errMsg string) (class string, suggestion string) {
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "1101") ||
		strings.Contains(lower, "exceeded resource limits") ||
		strings.Contains(lower, "exceeded cpu") ||
		strings.Contains(lower, "out of memory") ||
		strings.Contains(lower, "oom"):
		return "oom", "Worker exceeded resource limits. Consider reducing tool complexity or using a smaller model."
	case strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "timed out") ||
		strings.Contains(lower, "deadline exceeded"):
		return "timeout", "Agent execution timed out. Consider increasing the timeout or simplifying the task."
	case strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "429"):
		return "rate_limited", "Provider rate limit hit. Consider adding retry delays or reducing concurrency."
	default:
		return "runtime_error", ""
	}
}

func (s *localService) markRuntimeSystemFailed(ctx context.Context, runID, errMsg string) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil || run.Status.IsTerminal() {
		return
	}

	errorClass, suggestion := classifyRuntimeError(errMsg)

	finishedAt := s.now().UTC()
	if updateErr := s.store.UpdateRunStatus(ctx, runID, run.Status, domain.StatusSystemFailed, map[string]any{
		"finished_at": finishedAt,
		"error":       errMsg,
		"error_class": errorClass,
	}); updateErr != nil {
		return
	}

	eventData := map[string]any{
		"error":       errMsg,
		"error_class": errorClass,
	}
	if suggestion != "" {
		eventData["suggestion"] = suggestion
	}

	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   runID,
		Type:    domain.EventError,
		Level:   "error",
		Message: "agent runtime failed",
		Data:    mustJSON(eventData),
	})

	// Retry if the backing job allows more attempts. The next dispatch
	// will include the last checkpoint via buildRuntimeEnvelope.
	job, jobErr := s.store.GetJob(ctx, run.JobID)
	if jobErr != nil || job == nil {
		return
	}
	if run.Attempt >= job.MaxAttempts {
		return
	}
	s.scheduleAgentRetry(ctx, run, job)
}

func (s *localService) scheduleAgentRetry(ctx context.Context, failedRun *domain.JobRun, job *domain.Job) {
	retryRun := &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         failedRun.JobID,
		ProjectID:     failedRun.ProjectID,
		Status:        domain.StatusQueued,
		Attempt:       failedRun.Attempt + 1,
		Payload:       failedRun.Payload,
		TriggeredBy:   domain.TriggerRetry,
		JobVersion:    failedRun.JobVersion,
		JobVersionID:  failedRun.JobVersionID,
		ExecutionMode: failedRun.ExecutionMode,
		Error:         failedRun.Error,
	}
	if err := s.store.CreateRun(ctx, retryRun); err != nil {
		return
	}

	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   retryRun.ID,
		Type:    domain.EventStateChange,
		Level:   "info",
		Message: fmt.Sprintf("agent retry scheduled (attempt %d of %d)", retryRun.Attempt, job.MaxAttempts),
		Data: mustJSON(map[string]any{
			"previous_run_id": failedRun.ID,
			"attempt":         retryRun.Attempt,
			"max_attempts":    job.MaxAttempts,
		}),
	})

	// Look up the agent by scanning project agents for the matching job ID.
	// Paginate to avoid missing agents if the project has many.
	var foundAgent *domain.Agent
	var cursor *time.Time
	for foundAgent == nil {
		batch, listErr := s.store.ListAgents(ctx, failedRun.ProjectID, 100, cursor)
		if listErr != nil || len(batch) == 0 {
			return
		}
		for _, a := range batch {
			if a.JobID == failedRun.JobID {
				agentCopy := a
				foundAgent = &agentCopy
				break
			}
		}
		last := batch[len(batch)-1]
		cursor = &last.CreatedAt
	}

	deployment, depErr := s.store.GetLatestAgentDeployment(ctx, foundAgent.ID)
	if depErr != nil || deployment == nil || deployment.Status != domain.AgentDeploymentStatusDeployed {
		return
	}
	jobCopy := *job
	deploymentCopy := *deployment
	if s.dispatchPool != nil {
		s.dispatchPool.Submit(func() {
			s.dispatchRun(context.Background(), foundAgent, &jobCopy, &deploymentCopy, retryRun.ID)
		})
	}
}

// decryptProviderSecrets decrypts the agent's encrypted provider secrets for
// inclusion in the runtime dispatch envelope. Returns nil if no secrets are set
// or if decryption fails (logged to sentry). This is the ONLY place where
// provider secrets are decrypted.
func (s *localService) decryptProviderSecrets(_ context.Context, agent *domain.Agent) map[string]string {
	if agent.ProviderSecretsEncrypted == "" {
		return nil
	}
	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil
	}
	decrypted, err := q.DecryptAgentProviderSecrets(agent.ProviderSecretsEncrypted)
	if err != nil {
		slog.Error("agent: failed to decrypt provider secrets",
			"agent_id", agent.ID,
			"error", err,
		)
		return nil
	}
	return decrypted
}

// buildWebhookPayload constructs the JSON payload for agent webhook notifications.
func (s *localService) buildWebhookPayload(agent *domain.Agent, run *domain.JobRun) json.RawMessage {
	return mustJSON(map[string]any{
		"event":      "agent.run.terminal",
		"agent_id":   agent.ID,
		"agent_slug": agent.Slug,
		"run_id":     run.ID,
		"status":     string(run.Status),
		"attempt":    run.Attempt,
		"result":     run.Result,
		"error":      run.Error,
		"timestamp":  s.now().UTC(),
	})
}

// fireAgentWebhook sends a webhook notification if the agent has a webhook URL configured.
// When a WebhookDeliveryStore is available, it creates a durable delivery record
// processed by the existing DeliveryWorker with exponential backoff and retries.
// Falls back to fire-and-forget HTTP when no store is configured.
func (s *localService) fireAgentWebhook(ctx context.Context, agent *domain.Agent, runID string) {
	if agent == nil {
		return
	}
	webhookURL := ExtractWebhookURL(agent.Config)
	if webhookURL == "" {
		return
	}

	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil {
		slog.Error("agent webhook: failed to load run",
			"agent_id", agent.ID,
			"run_id", runID,
			"error", err,
		)
		return
	}

	payload := s.buildWebhookPayload(agent, run)

	// Prefer durable delivery when the webhook store is available.
	if s.webhookStore != nil {
		delivery := &domain.WebhookDelivery{
			ID:          uuid.Must(uuid.NewV7()).String(),
			RunID:       run.ID,
			JobID:       run.JobID,
			WebhookURL:  webhookURL,
			RetryPolicy: domain.WebhookRetryPolicyExponential,
			Status:      "pending",
			MaxAttempts: 5,
		}
		if createErr := s.webhookStore.CreateWebhookDelivery(ctx, delivery); createErr != nil {
			slog.Error("agent webhook: failed to create durable delivery",
				"agent_id", agent.ID,
				"run_id", runID,
				"error", createErr,
			)
		}
		return
	}

	// Fallback: fire-and-forget when no webhook store is configured.
	if s.dispatchHTTP == nil {
		return
	}
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if reqErr != nil {
		slog.Error("agent webhook: failed to build request",
			"agent_id", agent.ID,
			"run_id", runID,
			"error", reqErr,
		)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "strait-agents/1.0")
	if webhookSecret := ExtractWebhookSecret(agent.Config); webhookSecret != "" {
		req.Header.Set("X-Strait-Signature", SignWebhookPayload(webhookSecret, payload, s.now()))
	}

	resp, doErr := s.dispatchHTTP.Do(req)
	if doErr != nil {
		slog.Error("agent webhook: delivery failed",
			"agent_id", agent.ID,
			"run_id", runID,
			"error", doErr,
		)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		slog.Error("agent webhook: non-success response",
			"agent_id", agent.ID,
			"run_id", runID,
			"status_code", resp.StatusCode,
		)
	}
}

// ExtractWebhookURL reads the webhook_url from agent config after SSRF validation.
func ExtractWebhookURL(config json.RawMessage) string {
	if len(config) == 0 {
		return ""
	}
	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return ""
	}
	webhookVal, ok := cfg["webhook_url"].(string)
	if !ok {
		return ""
	}
	raw := strings.TrimSpace(webhookVal)
	if raw == "" {
		return ""
	}
	if !isSafeWebhookURL(raw) {
		return ""
	}
	return raw
}

// isSafeWebhookURL rejects URLs that could target internal services or
// cloud metadata endpoints (SSRF prevention). It checks both the hostname
// string and the resolved IP addresses to defend against DNS rebinding.
func isSafeWebhookURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	// Block cloud metadata, localhost, and private IP ranges.
	blocked := []string{
		"169.254.169.254",
		"metadata.google.internal",
		"localhost",
		"127.0.0.1",
		"[::1]",
		"0.0.0.0",
	}
	if slices.Contains(blocked, host) {
		return false
	}
	// Block .local and .internal TLDs.
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return false
	}
	// Resolve DNS and block private/reserved IP ranges to prevent DNS rebinding.
	// Use a 3-second timeout to avoid blocking the dispatch pool on slow resolvers.
	resolver := &net.Resolver{}
	dnsCtx, dnsCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dnsCancel()
	ips, err := resolver.LookupHost(dnsCtx, host)
	if err != nil {
		return false
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return false
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false
		}
		// Block CGNAT range (100.64.0.0/10) not covered by IsPrivate.
		if cgnatBlock.Contains(ip) {
			return false
		}
	}
	return true
}
