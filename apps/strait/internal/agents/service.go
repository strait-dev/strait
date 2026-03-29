package agents

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/alitto/pond/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	backingJobSlugPrefix = "__agent__"
	backingJobEndpoint   = "https://agents.local.invalid/dispatch"
)

var (
	ErrNotDeployed = errors.New("agent is not deployed")
)

type Service interface {
	CreateAgent(ctx context.Context, req CreateAgentRequest) (*domain.Agent, error)
	GetAgent(ctx context.Context, projectID, agentID string) (*domain.Agent, error)
	ListAgents(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Agent, error)
	UpdateAgent(ctx context.Context, req UpdateAgentRequest) (*domain.Agent, error)
	DeleteAgent(ctx context.Context, projectID, agentID string) error
	DeployAgent(ctx context.Context, projectID, agentID, actor string) (*domain.AgentDeployment, error)
	RunAgent(ctx context.Context, req RunAgentRequest) (*domain.JobRun, error)
	ListAgentRuns(ctx context.Context, projectID, agentID string, limit, offset int) ([]domain.JobRun, error)
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
	ListAgentDeployments(ctx context.Context, agentID string, limit int, cursor *time.Time) ([]domain.AgentDeployment, error)
	UpdateAgentDeployment(ctx context.Context, id string, patch map[string]any) error
	ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	AdvisoryXactLock(ctx context.Context, lockID int64) error
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

type localService struct {
	store          agentStore
	txb            store.TxBeginner
	p              Provider
	runtime        RuntimeRunner
	callbacks      RuntimeCallbackClient
	dispatchHTTP   *http.Client
	internalSecret string
	now            func() time.Time
	apiBaseURL     string
	jwtSigningKey  string
	dispatchPool   pond.Pool
}

type Option func(*localService)

type CreateAgentRequest struct {
	ProjectID   string
	Name        string
	Slug        string
	Description string
	Model       string
	Config      json.RawMessage
	Actor       string
}

type UpdateAgentRequest struct {
	ProjectID   string
	AgentID     string
	Name        string
	Slug        string
	Description string
	Model       string
	Config      json.RawMessage
	Actor       string
}

type RunAgentRequest struct {
	ProjectID string
	AgentID   string
	Payload   json.RawMessage
	Actor     string
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

func (s *localService) Close() {
	if s.dispatchPool != nil {
		s.dispatchPool.StopAndWait()
	}
}

func (s *localService) CreateAgent(ctx context.Context, req CreateAgentRequest) (*domain.Agent, error) {
	if err := validateCreateRequest(req); err != nil {
		return nil, err
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

		agent := &domain.Agent{
			ID:          uuid.Must(uuid.NewV7()).String(),
			ProjectID:   req.ProjectID,
			JobID:       backingJob.ID,
			Name:        req.Name,
			Slug:        req.Slug,
			Description: req.Description,
			Model:       req.Model,
			Config:      normalizedConfig(req.Config),
			CreatedBy:   req.Actor,
			UpdatedBy:   req.Actor,
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
		_ = s.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusCanceled, map[string]any{
			"finished_at": now,
		})
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
	agent, err := s.GetAgent(ctx, projectID, agentID)
	if err != nil {
		return nil, err
	}

	var deployment *domain.AgentDeployment
	err = store.WithTx(ctx, s.txb, func(txQ *store.Queries) error {
		if err := txQ.AdvisoryXactLock(ctx, advisoryLockID(agent.ID)); err != nil {
			return fmt.Errorf("lock agent deployment: %w", err)
		}
		version, err := txQ.NextAgentDeploymentVersion(ctx, agent.ID)
		if err != nil {
			return err
		}

		deployment = &domain.AgentDeployment{
			ID:             uuid.Must(uuid.NewV7()).String(),
			AgentID:        agent.ID,
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
		s.dispatchPool.Submit(func() {
			s.dispatchRun(context.Background(), &agentCopy, &jobCopy, &deploymentCopy, run.ID)
		})
	}
	return run, nil
}

func (s *localService) ListAgentRuns(ctx context.Context, projectID, agentID string, limit, offset int) ([]domain.JobRun, error) {
	agent, err := s.GetAgent(ctx, projectID, agentID)
	if err != nil {
		return nil, err
	}
	return s.store.ListRunsByJob(ctx, agent.JobID, limit, offset)
}

func buildBackingJob(req CreateAgentRequest) *domain.Job {
	return &domain.Job{
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
		MaxAttempts:   1,
		TimeoutSecs:   300,
		Enabled:       false,
		ExecutionMode: domain.ExecutionModeHTTP,
		CreatedBy:     req.Actor,
		UpdatedBy:     req.Actor,
	}
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
	}
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
	token, err := s.generateRunToken(run.ID, job.TimeoutSecs, run.ExpiresAt)
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
			ID:     agent.ID,
			Slug:   agent.Slug,
			Model:  agent.Model,
			Config: agent.Config,
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

func (s *localService) generateRunToken(runID string, timeoutSecs int, expiresAt *time.Time) (string, error) {
	if s.jwtSigningKey == "" {
		return "", fmt.Errorf("JWT signing key is not configured")
	}
	exp := s.now().Add(time.Duration(timeoutSecs) * time.Second).Add(60 * time.Second)
	if expiresAt != nil {
		exp = *expiresAt
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   runID,
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(s.now()),
	})
	signed, err := token.SignedString([]byte(s.jwtSigningKey))
	if err != nil {
		return "", fmt.Errorf("sign runtime run token: %w", err)
	}
	return signed, nil
}

func (s *localService) markRuntimeSystemFailed(ctx context.Context, runID, errMsg string) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil || run.Status.IsTerminal() {
		return
	}

	finishedAt := s.now().UTC()
	if updateErr := s.store.UpdateRunStatus(ctx, runID, run.Status, domain.StatusSystemFailed, map[string]any{
		"finished_at": finishedAt,
		"error":       errMsg,
	}); updateErr != nil {
		return
	}
	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   runID,
		Type:    domain.EventError,
		Level:   "error",
		Message: "agent runtime failed",
		Data:    mustJSON(map[string]any{"error": errMsg}),
	})
}
