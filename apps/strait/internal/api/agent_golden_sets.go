package api

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/agents"
	"strait/internal/domain"
)

// goldenSetStore is the subset of store methods needed for golden set CRUD.
type goldenSetStore interface {
	CreateGoldenSet(ctx context.Context, gs *domain.GoldenSet) error
	GetGoldenSet(ctx context.Context, agentID, name string) (*domain.GoldenSet, error)
	ListGoldenSets(ctx context.Context, agentID string) ([]domain.GoldenSet, error)
	DeleteGoldenSet(ctx context.Context, agentID, name string) error
}

// -- Golden Set CRUD types --

type CreateGoldenSetRequest struct {
	Name  string          `json:"name" validate:"required"`
	Cases json.RawMessage `json:"cases" validate:"required"`
}

type CreateGoldenSetInput struct {
	AgentID string `path:"agentID"`
	Body    CreateGoldenSetRequest
}

type GoldenSetOutput struct {
	Body *domain.GoldenSet
}

type ListGoldenSetsInput struct {
	AgentID string `path:"agentID"`
}

type ListGoldenSetsOutput struct {
	Body []domain.GoldenSet
}

type GetGoldenSetInput struct {
	AgentID string `path:"agentID"`
	Name    string `path:"name"`
}

type DeleteGoldenSetInput struct {
	AgentID string `path:"agentID"`
	Name    string `path:"name"`
}

// -- Eval Run types --

type RunEvalRequest struct {
	GoldenSetName string `json:"golden_set_name,omitempty"`
	Model         string `json:"model,omitempty"`
	TimeoutSecs   int    `json:"timeout_secs,omitempty"`
}

type RunEvalInput struct {
	AgentID string `path:"agentID"`
	Body    RunEvalRequest
}

type RunEvalOutput struct {
	Body *domain.EvalRun
}

// -- Handlers --

func (s *Server) handleCreateGoldenSet(ctx context.Context, input *CreateGoldenSetInput) (*GoldenSetOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	// Validate cases is a JSON array.
	var arr []json.RawMessage
	if err := json.Unmarshal(req.Cases, &arr); err != nil {
		return nil, huma.Error400BadRequest("cases must be a valid JSON array")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	// Verify agent exists.
	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	gsStore, ok := s.store.(goldenSetStore)
	if !ok {
		return nil, huma.Error500InternalServerError("golden set operations not supported")
	}

	gs := &domain.GoldenSet{
		AgentID:   input.AgentID,
		ProjectID: projectID,
		Name:      req.Name,
		Cases:     req.Cases,
	}

	if err := gsStore.CreateGoldenSet(ctx, gs); err != nil {
		slog.Error("failed to create golden set", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to create golden set")
	}

	return &GoldenSetOutput{Body: gs}, nil
}

func (s *Server) handleListGoldenSets(ctx context.Context, input *ListGoldenSetsInput) (*ListGoldenSetsOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	gsStore, ok := s.store.(goldenSetStore)
	if !ok {
		return nil, huma.Error500InternalServerError("golden set operations not supported")
	}

	sets, err := gsStore.ListGoldenSets(ctx, input.AgentID)
	if err != nil {
		slog.Error("failed to list golden sets", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to list golden sets")
	}

	if sets == nil {
		sets = []domain.GoldenSet{}
	}

	return &ListGoldenSetsOutput{Body: sets}, nil
}

func (s *Server) handleGetGoldenSet(ctx context.Context, input *GetGoldenSetInput) (*GoldenSetOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	gsStore, ok := s.store.(goldenSetStore)
	if !ok {
		return nil, huma.Error500InternalServerError("golden set operations not supported")
	}

	gs, err := gsStore.GetGoldenSet(ctx, input.AgentID, input.Name)
	if err != nil {
		slog.Error("failed to get golden set", "agent_id", input.AgentID, "name", input.Name, "error", err)
		return nil, huma.Error500InternalServerError("failed to get golden set")
	}
	if gs == nil {
		return nil, huma.Error404NotFound("golden set not found")
	}

	return &GoldenSetOutput{Body: gs}, nil
}

func (s *Server) handleDeleteGoldenSet(ctx context.Context, input *DeleteGoldenSetInput) (*struct{}, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	gsStore, ok := s.store.(goldenSetStore)
	if !ok {
		return nil, huma.Error500InternalServerError("golden set operations not supported")
	}

	if err := gsStore.DeleteGoldenSet(ctx, input.AgentID, input.Name); err != nil {
		slog.Error("failed to delete golden set", "agent_id", input.AgentID, "name", input.Name, "error", err)
		return nil, huma.Error500InternalServerError("failed to delete golden set")
	}

	return nil, nil
}

func (s *Server) handleRunEval(ctx context.Context, input *RunEvalInput) (*RunEvalOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	if input.AgentID == "" {
		return nil, huma.Error400BadRequest("agent_id is required")
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if _, agentErr := svc.GetAgent(ctx, projectID, input.AgentID); agentErr != nil {
		return nil, mapAgentServiceError(agentErr)
	}

	gsStore, ok := s.store.(goldenSetStore)
	if !ok {
		return nil, huma.Error500InternalServerError("golden set operations not supported")
	}

	// Determine golden set name: use request body or default.
	goldenSetName := input.Body.GoldenSetName
	if goldenSetName == "" {
		goldenSetName = "default"
	}

	gs, err := gsStore.GetGoldenSet(ctx, input.AgentID, goldenSetName)
	if err != nil {
		slog.Error("failed to get golden set for eval", "agent_id", input.AgentID, "name", goldenSetName, "error", err)
		return nil, huma.Error500InternalServerError("failed to load golden set")
	}
	if gs == nil {
		return nil, huma.Error404NotFound("golden set not found: " + goldenSetName)
	}

	// Parse golden cases from the set.
	var cases []domain.GoldenCase
	if err := json.Unmarshal(gs.Cases, &cases); err != nil {
		return nil, huma.Error400BadRequest("golden set cases are malformed")
	}

	// Model override reserved for future use.
	_ = input.Body.Model

	// Run each case against the agent and collect results.
	var results []domain.EvalCaseResult
	passed, failed := 0, 0
	for _, tc := range cases {
		payload, _ := json.Marshal(map[string]json.RawMessage{
			"input":           tc.Input,
			"expected_output": tc.ExpectedOutput,
		})

		run, runErr := svc.RunAgent(ctx, agents.RunAgentRequest{
			ProjectID: projectID,
			AgentID:   input.AgentID,
			Payload:   payload,
		})

		result := domain.EvalCaseResult{CaseID: tc.ID}
		if runErr != nil {
			result.Passed = false
			result.Reason = runErr.Error()
			failed++
		} else {
			result.Passed = true
			_ = run // run completed successfully
			passed++
		}
		results = append(results, result)
	}

	resultsJSON, _ := json.Marshal(results)

	evalRun := &domain.EvalRun{
		AgentID:     input.AgentID,
		ProjectID:   projectID,
		SuiteName:   goldenSetName,
		ResultsJSON: resultsJSON,
		TotalCases:  len(cases),
		PassedCases: passed,
		FailedCases: failed,
		Status:      "completed",
	}

	if err := s.store.CreateEvalRun(ctx, evalRun); err != nil {
		slog.Error("failed to create eval run", "agent_id", input.AgentID, "error", err)
		return nil, huma.Error500InternalServerError("failed to store eval results")
	}

	// Feed quality score to model router quality gate.
	if mrStore, ok := s.store.(agents.ModelRoutingStore); ok {
		qualityScore := float64(0)
		if evalRun.TotalCases > 0 {
			qualityScore = float64(evalRun.PassedCases) / float64(evalRun.TotalCases) * 100
		}
		router := agents.NewModelRouter(mrStore)
		for _, tier := range []agents.RequestTier{agents.TierSimple, agents.TierStandard, agents.TierComplex} {
			_ = router.CheckQualityGate(ctx, input.AgentID, tier, qualityScore)
		}
	}

	return &RunEvalOutput{Body: evalRun}, nil
}
