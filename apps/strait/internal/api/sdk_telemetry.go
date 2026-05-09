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
	if req.CostMicrousd > 0 {
		run, runErr := s.store.GetRun(ctx, runID)
		if runErr == nil && run != nil {
			quota, qErr := s.store.GetProjectQuota(ctx, run.ProjectID)
			if qErr == nil && quota != nil && quota.MaxCostPerRunMicrousd > 0 {
				totalCost, costErr := s.store.SumRunCostMicrousd(ctx, runID)
				if costErr == nil && totalCost+req.CostMicrousd >= quota.MaxCostPerRunMicrousd {
					return nil, huma.Error429TooManyRequests("per-run cost budget exceeded")
				}
			}
		}
	}
	if req.TotalTokens > 0 { //nolint:nestif
		run, runErr := s.store.GetRun(ctx, runID)
		if runErr == nil && run != nil {
			quota, qErr := s.store.GetProjectQuota(ctx, run.ProjectID)
			job, jobErr := s.store.GetJob(ctx, run.JobID)
			var quotaTokens int64
			if qErr == nil && quota != nil {
				quotaTokens = quota.MaxTokensPerRun
			}
			var jobTokens int64
			if jobErr == nil && job != nil {
				jobTokens = job.MaxTokensPerRun
			}
			tokenLimit := resolveGuardrailInt64(quotaTokens, jobTokens)
			if tokenLimit > 0 {
				totalTokens, tokErr := s.store.SumRunTotalTokens(ctx, runID)
				if tokErr == nil && totalTokens+int64(req.TotalTokens) > tokenLimit {
					return nil, &typedAPIError{status: 429, apiError: APIError{Code: "token_budget_exceeded", Message: "token_budget_exceeded", Details: []string{fmt.Sprintf("current=%d limit=%d", totalTokens, tokenLimit)}}}
				}
			}
		}
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
	if runErr == nil && run != nil { //nolint:nestif
		job, jobErr := s.store.GetJob(ctx, run.JobID)
		if jobErr == nil && job != nil {
			if len(job.AllowedTools) > 0 && !slices.Contains(job.AllowedTools, req.ToolName) {
				return nil, &typedAPIError{status: 403, apiError: APIError{Code: "tool_not_allowed", Message: "tool_not_allowed", Details: []string{fmt.Sprintf("tool=%s", req.ToolName)}}}
			}
			if len(job.BlockedTools) > 0 && slices.Contains(job.BlockedTools, req.ToolName) {
				return nil, &typedAPIError{status: 403, apiError: APIError{Code: "tool_blocked", Message: "tool_blocked", Details: []string{fmt.Sprintf("tool=%s", req.ToolName)}}}
			}
			quota, qErr := s.store.GetProjectQuota(ctx, run.ProjectID)
			var quotaLimit int
			if qErr == nil && quota != nil {
				quotaLimit = quota.MaxToolCallsPerRun
			}
			toolCallLimit := resolveGuardrailInt(quotaLimit, job.MaxToolCallsPerRun)
			if toolCallLimit > 0 {
				count, cErr := s.store.CountRunToolCalls(ctx, runID)
				if cErr == nil && count >= toolCallLimit {
					return nil, &typedAPIError{status: 429, apiError: APIError{Code: "tool_call_limit_exceeded", Message: "tool_call_limit_exceeded", Details: []string{fmt.Sprintf("current=%d limit=%d", count, toolCallLimit)}}}
				}
			}
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
