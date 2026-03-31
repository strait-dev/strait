package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"strconv"
	"time"

	"strait/internal/agents"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type CreateAgentRequest struct {
	ProjectID    string          `json:"project_id" validate:"required"`
	Name         string          `json:"name" validate:"required"`
	Slug         string          `json:"slug" validate:"required"`
	Description  string          `json:"description,omitempty"`
	Model        string          `json:"model" validate:"required"`
	Config       json.RawMessage `json:"config,omitempty"`
	Cron         string          `json:"cron,omitempty"`
	CronTimezone string          `json:"cron_timezone,omitempty"`
}

type UpdateAgentRequest struct {
	Name         *string          `json:"name,omitempty"`
	Slug         *string          `json:"slug,omitempty"`
	Description  *string          `json:"description,omitempty"`
	Model        *string          `json:"model,omitempty"`
	Config       *json.RawMessage `json:"config,omitempty"`
	Cron         *string          `json:"cron,omitempty"`
	CronTimezone *string          `json:"cron_timezone,omitempty"`
}

type RunAgentRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

type CreateAgentInput struct {
	Body CreateAgentRequest
}

type CreateAgentOutput struct {
	Body *domain.Agent
}

type ListAgentsInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListAgentsOutput struct {
	Body []domain.Agent
}

type GetAgentInput struct {
	AgentID string `path:"agentID"`
}

type GetAgentOutput struct {
	Body *domain.Agent
}

type UpdateAgentInput struct {
	AgentID string `path:"agentID"`
	Body    UpdateAgentRequest
}

type UpdateAgentOutput struct {
	Body *domain.Agent
}

type DeleteAgentInput struct {
	AgentID string `path:"agentID"`
}

type DeployAgentInput struct {
	AgentID string `path:"agentID"`
}

type DeployAgentOutput struct {
	Body *domain.AgentDeployment
}

type RunAgentInput struct {
	AgentID string `path:"agentID"`
	Body    RunAgentRequest
}

type RunAgentOutput struct {
	Body *domain.JobRun
}

type ListAgentRunsInput struct {
	AgentID string `path:"agentID"`
	Limit   string `query:"limit"`
	Offset  string `query:"offset"`
}

type ListAgentRunsOutput struct {
	Body []domain.JobRun
}

func (s *Server) requireAgentService() (agents.Service, error) {
	if s.agentService == nil {
		return nil, huma.Error503ServiceUnavailable("agent service unavailable")
	}
	return s.agentService, nil
}

func (s *Server) handleCreateAgent(ctx context.Context, input *CreateAgentInput) (*CreateAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		projectID = req.ProjectID
	}
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if req.ProjectID != "" && projectID != req.ProjectID {
		return nil, huma.Error403Forbidden("access denied")
	}
	if err := validateAgentConfigJSON(req.Config); err != nil {
		return nil, err
	}

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID:    projectID,
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Model:        req.Model,
		Config:       req.Config,
		Cron:         req.Cron,
		CronTimezone: req.CronTimezone,
		Actor:        actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapAgentServiceError(err)
	}

	return &CreateAgentOutput{Body: agent}, nil
}

func (s *Server) handleListAgents(ctx context.Context, input *ListAgentsInput) (*ListAgentsOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	limit := defaultPageLimit
	if input.Limit != "" {
		parsed, err := strconv.Atoi(input.Limit)
		if err != nil || parsed <= 0 || parsed > maxPageLimit {
			return nil, huma.Error400BadRequest("invalid limit")
		}
		limit = parsed
	}

	var cursor *time.Time
	if input.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339Nano, input.Cursor)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid cursor")
		}
		cursor = &parsed
	}

	items, err := svc.ListAgents(ctx, projectID, limit, cursor)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &ListAgentsOutput{Body: items}, nil
}

func (s *Server) handleGetAgent(ctx context.Context, input *GetAgentInput) (*GetAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	agent, err := svc.GetAgent(ctx, projectID, input.AgentID)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &GetAgentOutput{Body: agent}, nil
}

func (s *Server) handleUpdateAgent(ctx context.Context, input *UpdateAgentInput) (*UpdateAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	existing, err := svc.GetAgent(ctx, projectID, input.AgentID)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}

	req := input.Body
	updated := agents.UpdateAgentRequest{
		ProjectID:   projectID,
		AgentID:     input.AgentID,
		Name:        existing.Name,
		Slug:        existing.Slug,
		Description: existing.Description,
		Model:       existing.Model,
		Config:      existing.Config,
		Actor:       actorFromContext(ctx),
	}
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.Slug != nil {
		updated.Slug = *req.Slug
	}
	if req.Description != nil {
		updated.Description = *req.Description
	}
	if req.Model != nil {
		updated.Model = *req.Model
	}
	if req.Config != nil {
		if err := validateAgentConfigJSON(*req.Config); err != nil {
			return nil, err
		}
		updated.Config = *req.Config
	}
	if req.Cron != nil {
		updated.Cron = *req.Cron
	}
	if req.CronTimezone != nil {
		updated.CronTimezone = *req.CronTimezone
	}

	agent, err := svc.UpdateAgent(ctx, updated)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &UpdateAgentOutput{Body: agent}, nil
}

func (s *Server) handleDeleteAgent(ctx context.Context, input *DeleteAgentInput) (*struct{}, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	if err := svc.DeleteAgent(ctx, projectID, input.AgentID); err != nil {
		return nil, mapAgentServiceError(err)
	}
	return nil, nil
}

func (s *Server) handleDeployAgent(ctx context.Context, input *DeployAgentInput) (*DeployAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	deployment, err := svc.DeployAgent(ctx, projectID, input.AgentID, actorFromContext(ctx))
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &DeployAgentOutput{Body: deployment}, nil
}

func (s *Server) handleRunAgent(ctx context.Context, input *RunAgentInput) (*RunAgentOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   input.AgentID,
		Payload:   input.Body.Payload,
		Actor:     actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &RunAgentOutput{Body: run}, nil
}

func (s *Server) handleListAgentRuns(ctx context.Context, input *ListAgentRunsInput) (*ListAgentRunsOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	limit := defaultPageLimit
	if input.Limit != "" {
		parsed, err := strconv.Atoi(input.Limit)
		if err != nil || parsed <= 0 || parsed > maxPageLimit {
			return nil, huma.Error400BadRequest("invalid limit")
		}
		limit = parsed
	}

	offset := 0
	if input.Offset != "" {
		parsed, err := strconv.Atoi(input.Offset)
		if err != nil || parsed < 0 {
			return nil, huma.Error400BadRequest("invalid offset")
		}
		offset = parsed
	}

	runs, err := svc.ListAgentRuns(ctx, projectID, input.AgentID, limit, offset)
	if err != nil {
		return nil, mapAgentServiceError(err)
	}
	return &ListAgentRunsOutput{Body: runs}, nil
}

func mapAgentServiceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrAgentNotFound):
		return huma.Error404NotFound("agent not found")
	case errors.Is(err, store.ErrAgentSlugConflict):
		return huma.Error409Conflict("agent slug already exists")
	case errors.Is(err, agents.ErrNotDeployed):
		return huma.Error409Conflict("agent is not deployed")
	case errors.Is(err, store.ErrAgentDeploymentNotFound):
		return huma.Error404NotFound("agent deployment not found")
	case errors.Is(err, agents.ErrAgentQuotaExceeded):
		return huma.Error429TooManyRequests("agent quota exceeded for this project")
	case errors.Is(err, agents.ErrRunQuotaExceeded):
		return huma.Error429TooManyRequests("monthly agent run quota exceeded")
	case errors.Is(err, agents.ErrConcurrencyExceeded):
		return huma.Error429TooManyRequests("agent has too many concurrent runs")
	}

	var fieldErr *domain.FieldError
	if errors.As(err, &fieldErr) {
		return huma.Error400BadRequest(fieldErr.Error())
	}
	var validationErr *agents.ValidationError
	if errors.As(err, &validationErr) {
		return huma.Error400BadRequest(validationErr.Error())
	}

	slog.Error("agent handler failed", "error", err)
	return huma.Error500InternalServerError("agent request failed")
}

func validateAgentConfigJSON(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}
	if !json.Valid(config) {
		return huma.Error400BadRequest("config must be valid JSON")
	}

	var decoded any
	if err := json.Unmarshal(config, &decoded); err != nil {
		return huma.Error400BadRequest("config must be valid JSON")
	}
	if _, ok := decoded.(map[string]any); !ok {
		return huma.Error400BadRequest("config must be a JSON object")
	}
	return nil
}

type ListAgentVersionsInput struct {
	AgentID string `path:"agentID"`
	Limit   string `query:"limit"`
}

type ListAgentVersionsOutput struct {
	Body []domain.AgentDeployment
}

func (s *Server) handleListAgentVersions(ctx context.Context, input *ListAgentVersionsInput) (*ListAgentVersionsOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	// Verify the agent belongs to the project.
	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	limit := 20
	if input.Limit != "" {
		if parsed, parseErr := strconv.Atoi(input.Limit); parseErr == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	// Use the store directly since the agent service's agentStore interface
	// has ListAgentDeployments but the Service interface doesn't expose it.
	// The store.Queries implements both APIStore and the needed method.
	type deploymentLister interface {
		ListAgentDeployments(ctx context.Context, agentID string, limit int, cursor *time.Time) ([]domain.AgentDeployment, error)
	}
	lister, ok := s.store.(deploymentLister)
	if !ok {
		return nil, huma.Error500InternalServerError("deployment listing not supported")
	}

	deployments, listErr := lister.ListAgentDeployments(ctx, input.AgentID, limit, nil)
	if listErr != nil {
		return nil, huma.Error500InternalServerError("failed to list agent versions")
	}
	if deployments == nil {
		deployments = []domain.AgentDeployment{}
	}
	return &ListAgentVersionsOutput{Body: deployments}, nil
}

type PlaygroundRunRequest struct {
	Model        string          `json:"model" validate:"required"`
	SystemPrompt string          `json:"system_prompt,omitempty"`
	MaxIter      int             `json:"max_iterations,omitempty"`
	Budget       string          `json:"budget,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type PlaygroundRunInput struct {
	Body PlaygroundRunRequest
}

type PlaygroundRunOutput struct {
	Body struct {
		RunID   string `json:"run_id"`
		AgentID string `json:"agent_id"`
	}
}

func (s *Server) handlePlaygroundRun(ctx context.Context, input *PlaygroundRunInput) (*PlaygroundRunOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}
	actor := actorFromContext(ctx)

	model := input.Body.Model
	if model == "" {
		model = "gpt-5.4-mini"
	}

	config := map[string]any{
		"playground": true,
	}
	if input.Body.SystemPrompt != "" {
		config["system_prompt"] = input.Body.SystemPrompt
	}
	if input.Body.MaxIter > 0 {
		config["max_iterations"] = input.Body.MaxIter
	}
	if input.Body.Budget != "" {
		config["budget"] = input.Body.Budget
	}

	configJSON, _ := json.Marshal(config)
	randSuffix := make([]byte, 4)
	_, _ = rand.Read(randSuffix)
	slug := "playground-" + time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(randSuffix)

	agent, createErr := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID:   projectID,
		Name:        "Playground " + slug,
		Slug:        slug,
		Description: "Temporary playground agent",
		Model:       model,
		Config:      configJSON,
		Actor:       actor,
	})
	if createErr != nil {
		return nil, mapAgentServiceError(createErr)
	}

	if _, deployErr := svc.DeployAgent(ctx, projectID, agent.ID, actor); deployErr != nil {
		return nil, mapAgentServiceError(deployErr)
	}

	run, runErr := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   input.Body.Payload,
		Actor:     actor,
	})
	if runErr != nil {
		return nil, mapAgentServiceError(runErr)
	}

	return &PlaygroundRunOutput{Body: struct {
		RunID   string `json:"run_id"`
		AgentID string `json:"agent_id"`
	}{
		RunID:   run.ID,
		AgentID: agent.ID,
	}}, nil
}

type GetRecommendationsInput struct {
	AgentID string `path:"agentID"`
}

type GetRecommendationsOutput struct {
	Body []agents.CostRecommendation
}

func (s *Server) handleGetRecommendations(ctx context.Context, input *GetRecommendationsInput) (*GetRecommendationsOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	agent, agentErr := svc.GetAgent(ctx, projectID, input.AgentID)
	if agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	costStore, ok := s.store.(agents.CostOptimizerStore)
	if !ok {
		return nil, huma.Error500InternalServerError("cost optimization not supported")
	}
	recs, recErr := agents.GenerateRecommendations(ctx, costStore, agent)
	if recErr != nil {
		return nil, huma.Error500InternalServerError("failed to generate recommendations")
	}
	if recs == nil {
		recs = []agents.CostRecommendation{}
	}
	return &GetRecommendationsOutput{Body: recs}, nil
}

type ApplyRecommendationRequest struct {
	RecommendationID string `json:"recommendation_id" validate:"required"`
}

type ApplyRecommendationInput struct {
	AgentID string `path:"agentID"`
	Body    ApplyRecommendationRequest
}

type ApplyRecommendationOutput struct {
	Body *domain.Agent
}

func (s *Server) handleApplyRecommendation(ctx context.Context, input *ApplyRecommendationInput) (*ApplyRecommendationOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	actor := actorFromContext(ctx)

	agent, agentErr := svc.GetAgent(ctx, projectID, input.AgentID)
	if agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	// Generate recommendations and find the one the client approved.
	costStore, ok := s.store.(agents.CostOptimizerStore)
	if !ok {
		return nil, huma.Error500InternalServerError("cost optimization not supported")
	}
	recs, recErr := agents.GenerateRecommendations(ctx, costStore, agent)
	if recErr != nil {
		return nil, huma.Error500InternalServerError("failed to generate recommendations")
	}

	var matched *agents.CostRecommendation
	for i := range recs {
		if recs[i].ID == input.Body.RecommendationID {
			matched = &recs[i]
			break
		}
	}
	if matched == nil {
		return nil, huma.Error404NotFound("recommendation not found")
	}

	// Apply the server-computed patch (not client-provided) with an allowlist.
	var existingCfg map[string]any
	if len(agent.Config) > 0 {
		_ = json.Unmarshal(agent.Config, &existingCfg)
	}
	if existingCfg == nil {
		existingCfg = make(map[string]any)
	}

	safePatch := agents.FilterAllowedPatchKeys(matched.SuggestedPatch)
	maps.Copy(existingCfg, safePatch)

	// If the recommendation suggests a model change, apply it at the agent level too.
	newModel := agent.Model
	if m, ok := safePatch["model"].(string); ok && m != "" {
		newModel = m
	}

	mergedConfig, _ := json.Marshal(existingCfg)

	updated, updateErr := svc.UpdateAgent(ctx, agents.UpdateAgentRequest{
		ProjectID:   projectID,
		AgentID:     agent.ID,
		Name:        agent.Name,
		Slug:        agent.Slug,
		Description: agent.Description,
		Model:       newModel,
		Config:      mergedConfig,
		Actor:       actor,
	})
	if updateErr != nil {
		return nil, mapAgentServiceError(updateErr)
	}

	// Trigger re-deploy.
	_, _ = svc.DeployAgent(ctx, projectID, agent.ID, actor)

	return &ApplyRecommendationOutput{Body: updated}, nil
}

type DismissRecommendationInput struct {
	AgentID string `path:"agentID"`
}

func (s *Server) handleDismissRecommendation(ctx context.Context, _ *DismissRecommendationInput) (*struct{}, error) {
	// Dismiss is a no-op for now. Could store dismissed recs in a JSONB field later.
	_ = ctx
	return nil, nil
}

type ReplayAgentRunRequest struct {
	ConfigOverrides map[string]any `json:"config_overrides,omitempty"`
	FromCheckpoint  int            `json:"from_checkpoint,omitempty"`
}

type ReplayAgentRunInput struct {
	AgentID string `path:"agentID"`
	RunID   string `path:"runID"`
	Body    ReplayAgentRunRequest
}

type ReplayAgentRunOutput struct {
	Body *domain.JobRun
}

func (s *Server) handleReplayAgentRun(ctx context.Context, input *ReplayAgentRunInput) (*ReplayAgentRunOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	run, runErr := svc.ReplayAgentRun(ctx, agents.ReplayAgentRunRequest{
		ProjectID:       projectID,
		AgentID:         input.AgentID,
		OriginalRunID:   input.RunID,
		ConfigOverrides: input.Body.ConfigOverrides,
		FromCheckpoint:  input.Body.FromCheckpoint,
		Actor:           actorFromContext(ctx),
	})
	if runErr != nil {
		return nil, mapAgentServiceError(runErr)
	}

	return &ReplayAgentRunOutput{Body: run}, nil
}

type GetAgentHealthInput struct {
	AgentID string `path:"agentID"`
}

type GetAgentHealthOutput struct {
	Body *store.AgentHealthStats
}

func (s *Server) handleGetAgentHealth(ctx context.Context, input *GetAgentHealthInput) (*GetAgentHealthOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error500InternalServerError("health stats not supported")
	}

	since := time.Now().Add(-24 * time.Hour)
	stats, statsErr := q.GetAgentHealthStats(ctx, input.AgentID, since)
	if statsErr != nil {
		return nil, huma.Error500InternalServerError("failed to get agent health stats")
	}

	return &GetAgentHealthOutput{Body: stats}, nil
}
