package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/agents"
	"strait/internal/store"
)

const maxInvokeTimeoutMs = 300_000 // 5 minutes. Capped to prevent goroutine exhaustion.

type InvokeAgentRequest struct {
	AgentSlug  string          `json:"agent_slug" validate:"required"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	TimeoutMs  int             `json:"timeout_ms,omitempty"`
}

type InvokeAgentInput struct {
	RunID string `path:"runID"`
	Body  InvokeAgentRequest
}

type InvokeAgentOutput struct {
	Body struct {
		RunID  string          `json:"run_id"`
		Status string          `json:"status"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  string          `json:"error,omitempty"`
	}
}

func (s *Server) handleInvokeAgent(ctx context.Context, input *InvokeAgentInput) (*InvokeAgentOutput, error) {
	slug := strings.TrimSpace(input.Body.AgentSlug)
	if slug == "" {
		return nil, huma.Error400BadRequest("agent_slug is required")
	}

	callerRun, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error404NotFound("caller run not found")
	}

	// Check invocation depth to prevent unbounded recursion.
	if callerRun.LineageDepth >= 5 {
		return nil, huma.Error400BadRequest(fmt.Sprintf("agent invocation depth exceeds maximum (%d)", 5))
	}

	svc, svcErr := s.requireAgentService()
	if svcErr != nil {
		return nil, svcErr
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("invoke-agent not supported")
	}

	agent, agentErr := q.GetAgentBySlug(ctx, callerRun.ProjectID, slug)
	if agentErr != nil {
		if errors.Is(agentErr, store.ErrAgentNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("agent %q not found in project", slug))
		}
		return nil, huma.Error500InternalServerError("failed to look up agent")
	}

	if !agent.Enabled {
		return nil, huma.Error409Conflict("target agent is disabled")
	}

	// Trigger the target agent run.
	agentRun, runErr := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: callerRun.ProjectID,
		AgentID:   agent.ID,
		Payload:   input.Body.Payload,
		Actor:     "agent:" + input.RunID,
	})
	if runErr != nil {
		return nil, mapAgentServiceError(runErr)
	}

	s.publishRunEvent(ctx, input.RunID, map[string]any{
		"type": "agent_invoke", "target_slug": slug,
		"target_run_id": agentRun.ID,
		"timestamp":     time.Now().UTC(),
	})

	// If already terminal, return immediately.
	if agentRun.Status.IsTerminal() {
		return buildInvokeResponse(agentRun.ID, string(agentRun.Status), agentRun.Result, agentRun.Error), nil
	}

	// Poll until terminal or timeout.
	timeoutMs := input.Body.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = maxInvokeTimeoutMs
	}
	if timeoutMs > maxInvokeTimeoutMs {
		timeoutMs = maxInvokeTimeoutMs
	}

	deadline := time.Duration(timeoutMs) * time.Millisecond
	pollCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			// Re-fetch one last time.
			latest, latestErr := s.store.GetRun(ctx, agentRun.ID)
			if latestErr == nil && latest.Status.IsTerminal() {
				return buildInvokeResponse(latest.ID, string(latest.Status), latest.Result, latest.Error), nil
			}
			return nil, huma.Error408RequestTimeout(fmt.Sprintf("agent run %s did not complete within %dms", agentRun.ID, timeoutMs))
		case <-ticker.C:
			latest, latestErr := s.store.GetRun(ctx, agentRun.ID)
			if latestErr != nil {
				continue
			}
			if latest.Status.IsTerminal() {
				return buildInvokeResponse(latest.ID, string(latest.Status), latest.Result, latest.Error), nil
			}
		}
	}
}

func buildInvokeResponse(runID, status string, result json.RawMessage, errMsg string) *InvokeAgentOutput {
	out := &InvokeAgentOutput{}
	out.Body.RunID = runID
	out.Body.Status = status
	out.Body.Result = result
	out.Body.Error = errMsg
	return out
}
