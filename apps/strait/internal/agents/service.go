package agents

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
)

const (
	backingJobSlugPrefix = "__agent__"
	backingJobEndpoint   = "https://agents.local.invalid/dispatch"
	localProviderName    = "local_stub"
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
	UpdateAgentDeployment(ctx context.Context, id string, patch map[string]any) error
	ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error)
	CreateRun(ctx context.Context, run *domain.JobRun) error
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	AdvisoryXactLock(ctx context.Context, lockID int64) error
}

type Provider interface {
	Deploy(ctx context.Context, agent *domain.Agent, deployment *domain.AgentDeployment) (json.RawMessage, error)
	Run(ctx context.Context, agent *domain.Agent, deployment *domain.AgentDeployment, run *domain.JobRun) (json.RawMessage, error)
}

type LocalStubProvider struct{}

func (LocalStubProvider) Deploy(_ context.Context, agent *domain.Agent, deployment *domain.AgentDeployment) (json.RawMessage, error) {
	return mustJSON(map[string]any{
		"provider":       localProviderName,
		"agent_id":       agent.ID,
		"deployment_id":  deployment.ID,
		"deployment_ver": deployment.Version,
	}), nil
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
		"provider":           localProviderName,
		"received_payload":   payload,
	}), nil
}

type localService struct {
	store agentStore
	txb   store.TxBeginner
	p     Provider
	now   func() time.Time
}

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

func NewService(q *store.Queries, txb store.TxBeginner) Service {
	return &localService{
		store: q,
		txb:   txb,
		p:     LocalStubProvider{},
		now:   time.Now,
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
			Provider:       localProviderName,
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

	now := s.now().UTC()
	run := &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         agent.JobID,
		ProjectID:     agent.ProjectID,
		Status:        domain.StatusExecuting,
		Attempt:       1,
		Payload:       req.Payload,
		TriggeredBy:   domain.TriggerManual,
		JobVersion:    job.Version,
		JobVersionID:  job.VersionID,
		ExecutionMode: job.ExecutionMode,
		StartedAt:     &now,
		CreatedBy:     req.Actor,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   run.ID,
		Type:    domain.EventStateChange,
		Level:   "info",
		Message: "agent run started",
		Data: mustJSON(map[string]any{
			"agent_id":       agent.ID,
			"deployment_id":  deployment.ID,
			"deployment_ver": deployment.Version,
		}),
	})

	result, runErr := s.p.Run(ctx, agent, deployment, run)
	finishedAt := s.now().UTC()
	if runErr != nil {
		run.Status = domain.StatusFailed
		run.Error = runErr.Error()
		run.FinishedAt = &finishedAt
		if err := s.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
			"finished_at": finishedAt,
			"error":       runErr.Error(),
		}); err != nil {
			return nil, err
		}
		_ = s.store.InsertEvent(ctx, &domain.RunEvent{
			RunID:   run.ID,
			Type:    domain.EventError,
			Level:   "error",
			Message: "agent run failed",
			Data:    mustJSON(map[string]any{"error": runErr.Error()}),
		})
		return run, nil
	}

	run.Status = domain.StatusCompleted
	run.Result = result
	run.FinishedAt = &finishedAt
	if err := s.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": finishedAt,
		"result":      result,
	}); err != nil {
		return nil, err
	}
	_ = s.store.InsertEvent(ctx, &domain.RunEvent{
		RunID:   run.ID,
		Type:    domain.EventStateChange,
		Level:   "info",
		Message: "agent run completed",
		Data: mustJSON(map[string]any{
			"agent_id":       agent.ID,
			"deployment_id":  deployment.ID,
			"deployment_ver": deployment.Version,
		}),
	})
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
