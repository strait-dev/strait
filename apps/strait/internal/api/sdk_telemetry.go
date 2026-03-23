package api

import (
	"encoding/json"
	"net/http"
	"slices"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleSDKUsage(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Provider         string `json:"provider" validate:"required"`
		Model            string `json:"model" validate:"required"`
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		TotalTokens      int    `json:"total_tokens,omitempty"`
		CostMicrousd     int64  `json:"cost_microusd,omitempty"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	usage := &domain.RunUsage{
		ID:               uuid.Must(uuid.NewV7()).String(),
		RunID:            runID,
		Provider:         req.Provider,
		Model:            req.Model,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		TotalTokens:      req.TotalTokens,
		CostMicrousd:     req.CostMicrousd,
	}

	// Cost budget check BEFORE recording usage to prevent overspend.
	if req.CostMicrousd > 0 {
		run, runErr := s.store.GetRun(r.Context(), runID)
		if runErr == nil && run != nil {
			quota, qErr := s.store.GetProjectQuota(r.Context(), run.ProjectID)
			if qErr == nil && quota != nil && quota.MaxCostPerRunMicrousd > 0 {
				totalCost, costErr := s.store.SumRunCostMicrousd(r.Context(), runID)
				if costErr == nil && totalCost+req.CostMicrousd >= quota.MaxCostPerRunMicrousd {
					respondError(w, r, http.StatusTooManyRequests, "per-run cost budget exceeded")
					return
				}
			}
		}
	}

	// Token budget check BEFORE recording usage.
	if req.TotalTokens > 0 { //nolint:nestif
		run, runErr := s.store.GetRun(r.Context(), runID)
		if runErr == nil && run != nil {
			quota, qErr := s.store.GetProjectQuota(r.Context(), run.ProjectID)
			job, jobErr := s.store.GetJob(r.Context(), run.JobID)
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
				totalTokens, tokErr := s.store.SumRunTotalTokens(r.Context(), runID)
				if tokErr == nil && totalTokens+int64(req.TotalTokens) > tokenLimit {
					respondJSON(w, http.StatusTooManyRequests, map[string]any{
						"error": "token_budget_exceeded", "current": totalTokens, "limit": tokenLimit,
					})
					return
				}
			}
		}
	}

	if err := s.store.CreateRunUsage(r.Context(), usage); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create run usage")
		return
	}

	respondJSON(w, http.StatusCreated, usage)
}

func (s *Server) handleSDKToolCall(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		ToolName   string          `json:"tool_name" validate:"required"`
		Input      json.RawMessage `json:"input,omitempty"`
		Output     json.RawMessage `json:"output,omitempty"`
		DurationMs int             `json:"duration_ms,omitempty"`
		Status     string          `json:"status,omitempty"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	// Tool allowlist/blocklist and count limit checks.
	run, runErr := s.store.GetRun(r.Context(), runID)
	if runErr == nil && run != nil { //nolint:nestif
		job, jobErr := s.store.GetJob(r.Context(), run.JobID)
		if jobErr == nil && job != nil {
			// Tool allowlist check.
			if len(job.AllowedTools) > 0 && !slices.Contains(job.AllowedTools, req.ToolName) {
				respondJSON(w, http.StatusForbidden, map[string]any{
					"error": "tool_not_allowed", "tool": req.ToolName,
				})
				return
			}
			// Tool blocklist check.
			if len(job.BlockedTools) > 0 && slices.Contains(job.BlockedTools, req.ToolName) {
				respondJSON(w, http.StatusForbidden, map[string]any{
					"error": "tool_blocked", "tool": req.ToolName,
				})
				return
			}

			// Tool call count limit check.
			quota, qErr := s.store.GetProjectQuota(r.Context(), run.ProjectID)
			var quotaLimit int
			if qErr == nil && quota != nil {
				quotaLimit = quota.MaxToolCallsPerRun
			}
			toolCallLimit := resolveGuardrailInt(quotaLimit, job.MaxToolCallsPerRun)
			if toolCallLimit > 0 {
				count, cErr := s.store.CountRunToolCalls(r.Context(), runID)
				if cErr == nil && count >= toolCallLimit {
					respondJSON(w, http.StatusTooManyRequests, map[string]any{
						"error": "tool_call_limit_exceeded", "current": count, "limit": toolCallLimit,
					})
					return
				}
			}
		}
	}

	call := &domain.RunToolCall{
		ID:         uuid.Must(uuid.NewV7()).String(),
		RunID:      runID,
		ToolName:   req.ToolName,
		Input:      req.Input,
		Output:     req.Output,
		DurationMs: req.DurationMs,
		Status:     req.Status,
	}
	if err := s.store.CreateRunToolCall(r.Context(), call); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create run tool call")
		return
	}

	respondJSON(w, http.StatusCreated, call)
}

func (s *Server) handleSDKOutput(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		OutputKey string          `json:"output_key" validate:"required"`
		Schema    json.RawMessage `json:"schema,omitempty"`
		Value     json.RawMessage `json:"value" validate:"required"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}
	if err := validatePayloadAgainstSchema(req.Value, req.Schema); err != nil {
		respondError(w, r, http.StatusBadRequest, "output schema validation failed: "+err.Error())
		return
	}

	output := &domain.RunOutput{
		ID:        uuid.Must(uuid.NewV7()).String(),
		RunID:     runID,
		OutputKey: req.OutputKey,
		Schema:    req.Schema,
		Value:     req.Value,
	}
	if err := s.store.UpsertRunOutput(r.Context(), output); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to upsert run output")
		return
	}

	respondJSON(w, http.StatusCreated, output)
}
