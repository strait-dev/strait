package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"strait/internal/agents"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

const maxAwaitTimeoutMs = 60000 // 60 seconds. Capped to prevent goroutine exhaustion.

// Trigger job.

type SDKTriggerJobRequest struct {
	JobSlug string          `json:"job_slug" validate:"required"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type SDKTriggerJobInput struct {
	RunID string `path:"runID"`
	Body  SDKTriggerJobRequest
}

type SDKTriggerJobOutput struct {
	Body struct {
		RunID  string `json:"run_id"`
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
}

func (s *Server) handleSDKTriggerJob(ctx context.Context, input *SDKTriggerJobInput) (*SDKTriggerJobOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	slug := strings.TrimSpace(input.Body.JobSlug)
	if slug == "" {
		return nil, huma.Error400BadRequest("job_slug is required")
	}
	if err := validatePayloadSize(input.Body.Payload); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("platform triggers not supported")
	}

	job, jobErr := q.GetJobBySlug(ctx, run.ProjectID, slug)
	if jobErr != nil {
		if errors.Is(jobErr, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to look up job")
	}
	if !job.Enabled {
		return nil, huma.Error400BadRequest("job is disabled")
	}
	if job.Paused {
		return nil, huma.Error409Conflict("job is paused")
	}

	jobRun := &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         job.ID,
		ProjectID:     run.ProjectID,
		Status:        domain.StatusQueued,
		Attempt:       1,
		Payload:       input.Body.Payload,
		TriggeredBy:   domain.TriggerManual,
		JobVersion:    job.Version,
		JobVersionID:  job.VersionID,
		ExecutionMode: job.ExecutionMode,
	}
	if err := s.store.CreateRun(ctx, jobRun); err != nil {
		return nil, huma.Error500InternalServerError("failed to create job run")
	}

	if s.queue != nil {
		if enqErr := s.queue.Enqueue(ctx, jobRun); enqErr != nil {
			slog.Error("sdk platform: failed to enqueue job run", "run_id", jobRun.ID, "error", enqErr)
		}
	}

	s.publishRunEvent(ctx, input.RunID, map[string]any{
		"type": "platform_trigger", "target_type": "job",
		"target_slug": slug, "target_run_id": jobRun.ID,
		"timestamp": time.Now().UTC(),
	})

	return &SDKTriggerJobOutput{Body: struct {
		RunID  string `json:"run_id"`
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}{RunID: jobRun.ID, JobID: job.ID, Status: string(jobRun.Status)}}, nil
}

// Trigger workflow.

type SDKTriggerWorkflowRequest struct {
	WorkflowSlug string          `json:"workflow_slug" validate:"required"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type SDKTriggerWorkflowInput struct {
	RunID string `path:"runID"`
	Body  SDKTriggerWorkflowRequest
}

type SDKTriggerWorkflowOutput struct {
	Body struct {
		WorkflowRunID string `json:"workflow_run_id"`
		Status        string `json:"status"`
	}
}

func (s *Server) handleSDKTriggerWorkflow(ctx context.Context, input *SDKTriggerWorkflowInput) (*SDKTriggerWorkflowOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	slug := strings.TrimSpace(input.Body.WorkflowSlug)
	if slug == "" {
		return nil, huma.Error400BadRequest("workflow_slug is required")
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("platform triggers not supported")
	}

	wf, wfErr := q.GetWorkflowBySlug(ctx, run.ProjectID, slug)
	if wfErr != nil {
		if errors.Is(wfErr, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to look up workflow")
	}
	if !wf.Enabled {
		return nil, huma.Error409Conflict("workflow is disabled")
	}

	if s.workflowEngine == nil {
		return nil, huma.Error503ServiceUnavailable("workflow engine unavailable")
	}

	wfRun, triggerErr := s.workflowEngine.TriggerWorkflow(ctx, wf.ID, run.ProjectID, input.Body.Payload, "agent:"+input.RunID, nil, nil)
	if triggerErr != nil {
		slog.Error("sdk platform: failed to trigger workflow", "workflow_id", wf.ID, "error", triggerErr)
		return nil, huma.Error500InternalServerError("failed to trigger workflow")
	}

	s.publishRunEvent(ctx, input.RunID, map[string]any{
		"type": "platform_trigger", "target_type": "workflow",
		"target_slug": slug, "workflow_run_id": wfRun.ID,
		"timestamp": time.Now().UTC(),
	})

	return &SDKTriggerWorkflowOutput{Body: struct {
		WorkflowRunID string `json:"workflow_run_id"`
		Status        string `json:"status"`
	}{WorkflowRunID: wfRun.ID, Status: string(wfRun.Status)}}, nil
}

// Trigger agent.

type SDKTriggerAgentRequest struct {
	AgentSlug string          `json:"agent_slug" validate:"required"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type SDKTriggerAgentInput struct {
	RunID string `path:"runID"`
	Body  SDKTriggerAgentRequest
}

type SDKTriggerAgentOutput struct {
	Body struct {
		RunID   string `json:"run_id"`
		AgentID string `json:"agent_id"`
		Status  string `json:"status"`
	}
}

func (s *Server) handleSDKTriggerAgent(ctx context.Context, input *SDKTriggerAgentInput) (*SDKTriggerAgentOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	slug := strings.TrimSpace(input.Body.AgentSlug)
	if slug == "" {
		return nil, huma.Error400BadRequest("agent_slug is required")
	}

	svc, svcErr := s.requireAgentService()
	if svcErr != nil {
		return nil, svcErr
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("platform triggers not supported")
	}

	agent, agentErr := q.GetAgentBySlug(ctx, run.ProjectID, slug)
	if agentErr != nil {
		if errors.Is(agentErr, store.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("agent not found")
		}
		return nil, huma.Error500InternalServerError("failed to look up agent")
	}

	agentRun, runErr := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: run.ProjectID,
		AgentID:   agent.ID,
		Payload:   input.Body.Payload,
		Actor:     "agent:" + input.RunID,
	})
	if runErr != nil {
		return nil, mapAgentServiceError(runErr)
	}

	s.publishRunEvent(ctx, input.RunID, map[string]any{
		"type": "platform_trigger", "target_type": "agent",
		"target_slug": slug, "target_run_id": agentRun.ID,
		"timestamp": time.Now().UTC(),
	})

	return &SDKTriggerAgentOutput{Body: struct {
		RunID   string `json:"run_id"`
		AgentID string `json:"agent_id"`
		Status  string `json:"status"`
	}{RunID: agentRun.ID, AgentID: agent.ID, Status: string(agentRun.Status)}}, nil
}

// Await run.

type SDKAwaitRunRequest struct {
	RunID     string `json:"run_id" validate:"required"`
	TimeoutMs int    `json:"timeout_ms"`
}

type SDKAwaitRunInput struct {
	CallerRunID string `path:"runID"`
	Body        SDKAwaitRunRequest
}

type SDKAwaitRunOutput struct {
	Body struct {
		RunID  string          `json:"run_id"`
		Status string          `json:"status"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  string          `json:"error,omitempty"`
	}
}

func (s *Server) handleSDKAwaitRun(ctx context.Context, input *SDKAwaitRunInput) (*SDKAwaitRunOutput, error) {
	callerRun, err := s.store.GetRun(ctx, input.CallerRunID)
	if err != nil {
		return nil, huma.Error404NotFound("caller run not found")
	}

	targetRunID := strings.TrimSpace(input.Body.RunID)
	if targetRunID == "" {
		return nil, huma.Error400BadRequest("run_id is required")
	}

	targetRun, targetErr := s.store.GetRun(ctx, targetRunID)
	if targetErr != nil {
		return nil, huma.Error404NotFound("target run not found")
	}
	if targetRun.ProjectID != callerRun.ProjectID {
		return nil, huma.Error404NotFound("target run not found")
	}

	// Return immediately if already terminal.
	if targetRun.Status.IsTerminal() {
		return buildAwaitResponse(targetRun), nil
	}

	// Poll until terminal or timeout.
	timeoutMs := input.Body.TimeoutMs
	if timeoutMs <= 0 {
		return buildAwaitResponse(targetRun), nil
	}
	if timeoutMs > maxAwaitTimeoutMs {
		timeoutMs = maxAwaitTimeoutMs
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
			latest, latestErr := s.store.GetRun(ctx, targetRunID)
			if latestErr == nil && latest.Status.IsTerminal() {
				return buildAwaitResponse(latest), nil
			}
			return nil, huma.Error408RequestTimeout(fmt.Sprintf("run %s did not complete within %dms", targetRunID, timeoutMs))
		case <-ticker.C:
			latest, latestErr := s.store.GetRun(ctx, targetRunID)
			if latestErr != nil {
				continue
			}
			if latest.Status.IsTerminal() {
				return buildAwaitResponse(latest), nil
			}
		}
	}
}

func buildAwaitResponse(run *domain.JobRun) *SDKAwaitRunOutput {
	out := &SDKAwaitRunOutput{}
	out.Body.RunID = run.ID
	out.Body.Status = string(run.Status)
	out.Body.Result = run.Result
	out.Body.Error = run.Error
	return out
}
