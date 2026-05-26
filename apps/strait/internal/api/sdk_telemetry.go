package api

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type SDKUsageRequest struct {
	Provider         string `json:"provider" validate:"required"`
	Model            string `json:"model" validate:"required"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	CostMicrousd     int64  `json:"cost_microusd,omitempty"`
}
type SDKUsageInput struct {
	RunID string `path:"runID"`
	Body  SDKUsageRequest
}
type SDKUsageOutput struct{ Body *domain.RunUsage }

func (s *Server) handleSDKUsage(ctx context.Context, input *SDKUsageInput) (*SDKUsageOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := s.checkDailyAIModelCallLimit(ctx, runID); err != nil {
		return nil, err
	}
	usage := &domain.RunUsage{ID: uuid.Must(uuid.NewV7()).String(), RunID: runID, Provider: req.Provider, Model: req.Model, PromptTokens: req.PromptTokens, CompletionTokens: req.CompletionTokens, TotalTokens: req.TotalTokens, CostMicrousd: req.CostMicrousd}
	if err := s.checkSDKUsageBudgets(ctx, runID, req); err != nil {
		return nil, err
	}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.CreateRunUsageForActiveRun(ctx, usage, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.CreateRunUsage(ctx, usage)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to create run usage")
	}
	return &SDKUsageOutput{Body: usage}, nil
}

type SDKToolCallRequest struct {
	ToolName   string          `json:"tool_name" validate:"required"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	DurationMs int             `json:"duration_ms,omitempty"`
	Status     string          `json:"status,omitempty"`
}
type SDKToolCallInput struct {
	RunID string `path:"runID"`
	Body  SDKToolCallRequest
}
type SDKToolCallOutput struct{ Body *domain.RunToolCall }

func (s *Server) handleSDKToolCall(ctx context.Context, input *SDKToolCallInput) (*SDKToolCallOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	run, runErr := s.store.GetRun(ctx, runID)
	if runErr == nil && run != nil {
		if err := s.checkSDKToolCallAllowed(ctx, run, req); err != nil {
			return nil, err
		}
	}
	call := &domain.RunToolCall{ID: uuid.Must(uuid.NewV7()).String(), RunID: runID, ToolName: req.ToolName, Input: req.Input, Output: req.Output, DurationMs: req.DurationMs, Status: req.Status}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.CreateRunToolCallForActiveRun(ctx, call, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.CreateRunToolCall(ctx, call)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to create run tool call")
	}
	return &SDKToolCallOutput{Body: call}, nil
}

type SDKOutputRequest struct {
	OutputKey string          `json:"output_key" validate:"required"`
	Schema    json.RawMessage `json:"schema,omitempty"`
	Value     json.RawMessage `json:"value" validate:"required"`
}
type SDKOutputInput struct {
	RunID string `path:"runID"`
	Body  SDKOutputRequest
}
type SDKOutputOutput struct{ Body *domain.RunOutput }

func (s *Server) handleSDKOutput(ctx context.Context, input *SDKOutputInput) (*SDKOutputOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := validatePayloadAgainstSchema(req.Value, req.Schema); err != nil {
		return nil, huma.Error400BadRequest("output schema validation failed: " + err.Error())
	}
	output := &domain.RunOutput{ID: uuid.Must(uuid.NewV7()).String(), RunID: runID, OutputKey: req.OutputKey, Schema: req.Schema, Value: req.Value}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.UpsertRunOutputForActiveRun(ctx, output, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.UpsertRunOutput(ctx, output)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to upsert run output")
	}
	return &SDKOutputOutput{Body: output}, nil
}

func (s *Server) checkSDKUsageBudgets(ctx context.Context, runID string, req SDKUsageRequest) error {
	if req.CostMicrousd <= 0 && req.TotalTokens <= 0 {
		return nil
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil {
		return nil
	}
	if err := s.checkSDKUsageCostBudget(ctx, runID, run.ProjectID, req.CostMicrousd); err != nil {
		return err
	}
	return s.checkSDKUsageTokenBudget(ctx, runID, run, req.TotalTokens)
}

func (s *Server) checkSDKUsageCostBudget(ctx context.Context, runID, projectID string, costMicrousd int64) error {
	if costMicrousd <= 0 {
		return nil
	}
	quota, err := s.quotaCache.Get(ctx, projectID)
	if err != nil || quota == nil || quota.MaxCostPerRunMicrousd <= 0 {
		return nil
	}
	totalCost, err := s.store.SumRunCostMicrousd(ctx, runID)
	if err == nil && totalCost+costMicrousd >= quota.MaxCostPerRunMicrousd {
		return huma.Error429TooManyRequests("per-run cost budget exceeded")
	}
	return nil
}

func (s *Server) checkSDKUsageTokenBudget(ctx context.Context, runID string, run *domain.JobRun, totalTokens int) error {
	if totalTokens <= 0 {
		return nil
	}
	quotaLimit := int64(0)
	if quota, err := s.quotaCache.Get(ctx, run.ProjectID); err == nil && quota != nil {
		quotaLimit = quota.MaxTokensPerRun
	}
	jobLimit := int64(0)
	if job, err := s.store.GetJob(ctx, run.JobID); err == nil && job != nil {
		jobLimit = job.MaxTokensPerRun
	}
	tokenLimit := resolveGuardrailInt64(quotaLimit, jobLimit)
	if tokenLimit <= 0 {
		return nil
	}
	currentTokens, err := s.store.SumRunTotalTokens(ctx, runID)
	if err == nil && currentTokens+int64(totalTokens) > tokenLimit {
		return &typedAPIError{status: 429, apiError: APIError{Code: "token_budget_exceeded", Message: "token_budget_exceeded", Details: []string{fmt.Sprintf("current=%d limit=%d", currentTokens, tokenLimit)}}}
	}
	return nil
}

func (s *Server) checkSDKToolCallAllowed(ctx context.Context, run *domain.JobRun, req SDKToolCallRequest) error {
	job, err := s.store.GetJob(ctx, run.JobID)
	if err != nil || job == nil {
		return nil
	}
	if len(job.AllowedTools) > 0 && !slices.Contains(job.AllowedTools, req.ToolName) {
		return &typedAPIError{status: 403, apiError: APIError{Code: "tool_not_allowed", Message: "tool_not_allowed", Details: []string{fmt.Sprintf("tool=%s", req.ToolName)}}}
	}
	if len(job.BlockedTools) > 0 && slices.Contains(job.BlockedTools, req.ToolName) {
		return &typedAPIError{status: 403, apiError: APIError{Code: "tool_blocked", Message: "tool_blocked", Details: []string{fmt.Sprintf("tool=%s", req.ToolName)}}}
	}
	quotaLimit := 0
	if quota, err := s.quotaCache.Get(ctx, run.ProjectID); err == nil && quota != nil {
		quotaLimit = quota.MaxToolCallsPerRun
	}
	return s.checkSDKToolCallLimit(ctx, run.ID, resolveGuardrailInt(quotaLimit, job.MaxToolCallsPerRun))
}

func (s *Server) checkSDKToolCallLimit(ctx context.Context, runID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	count, err := s.store.CountRunToolCalls(ctx, runID)
	if err == nil && count >= limit {
		return &typedAPIError{status: 429, apiError: APIError{Code: "tool_call_limit_exceeded", Message: "tool_call_limit_exceeded", Details: []string{fmt.Sprintf("current=%d limit=%d", count, limit)}}}
	}
	return nil
}
