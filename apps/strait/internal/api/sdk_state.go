package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"strait/internal/agents"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type SDKSetStateRequest struct {
	Key   string          `json:"key" validate:"required"`
	Value json.RawMessage `json:"value" validate:"required"`
}
type SDKSetStateInput struct {
	RunID string `path:"runID"`
	Body  SDKSetStateRequest
}
type SDKSetStateOutput struct{ Body *domain.RunState }

func (s *Server) validateSDKStateRequest(req SDKSetStateRequest) error {
	if err := s.validate.Struct(&req); err != nil {
		return newValidationError(err)
	}
	if len(req.Key) > 256 {
		return huma.Error400BadRequest("state key must be 256 characters or fewer")
	}
	if len(req.Value) > 65536 {
		return huma.Error400BadRequest("state value must not exceed 64KB")
	}
	return nil
}

func (s *Server) workflowStateScopeRunID(ctx context.Context, runID string) (string, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return "", huma.Error404NotFound("run not found")
		}
		return "", huma.Error500InternalServerError("failed to get run")
	}
	if run.WorkflowStepRunID == "" {
		return "", huma.Error409Conflict("workflow state is only available for workflow-backed runs")
	}

	stepRun, err := s.store.GetStepRunByJobRunID(ctx, runID)
	if err != nil {
		return "", huma.Error500InternalServerError("failed to resolve workflow state scope")
	}
	if stepRun == nil || stepRun.WorkflowRunID == "" {
		return "", huma.Error409Conflict("workflow state is only available for workflow-backed runs")
	}

	return stepRun.WorkflowRunID, nil
}

func (s *Server) upsertSDKState(ctx context.Context, runID string, req SDKSetStateRequest) (*SDKSetStateOutput, error) {
	if err := s.validateSDKStateRequest(req); err != nil {
		return nil, err
	}
	state := &domain.RunState{RunID: runID, StateKey: req.Key, Value: req.Value}
	if err := s.store.UpsertRunState(ctx, state); err != nil {
		return nil, huma.Error500InternalServerError("failed to upsert run state")
	}
	return &SDKSetStateOutput{Body: state}, nil
}

func (s *Server) handleSDKSetState(ctx context.Context, input *SDKSetStateInput) (*SDKSetStateOutput, error) {
	return s.upsertSDKState(ctx, input.RunID, input.Body)
}

type SDKSetWorkflowStateInput struct {
	RunID string `path:"runID"`
	Body  SDKSetStateRequest
}

func (s *Server) handleSDKSetWorkflowState(ctx context.Context, input *SDKSetWorkflowStateInput) (*SDKSetStateOutput, error) {
	workflowRunID, err := s.workflowStateScopeRunID(ctx, input.RunID)
	if err != nil {
		return nil, err
	}
	return s.upsertSDKState(ctx, workflowRunID, input.Body)
}

type SDKGetStateInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}
type SDKGetStateOutput struct{ Body *domain.RunState }

func (s *Server) handleSDKGetState(ctx context.Context, input *SDKGetStateInput) (*SDKGetStateOutput, error) {
	state, err := s.store.GetRunState(ctx, input.RunID, input.Key)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get run state")
	}
	if state == nil {
		return nil, huma.Error404NotFound("state key not found")
	}
	return &SDKGetStateOutput{Body: state}, nil
}

type SDKGetWorkflowStateInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}

func (s *Server) handleSDKGetWorkflowState(ctx context.Context, input *SDKGetWorkflowStateInput) (*SDKGetStateOutput, error) {
	workflowRunID, err := s.workflowStateScopeRunID(ctx, input.RunID)
	if err != nil {
		return nil, err
	}
	state, err := s.store.GetRunState(ctx, workflowRunID, input.Key)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get run state")
	}
	if state == nil {
		return nil, huma.Error404NotFound("state key not found")
	}
	return &SDKGetStateOutput{Body: state}, nil
}

type SDKListStateOutput struct{ Body any }

func (s *Server) handleSDKListState(ctx context.Context, input *SDKRunIDInput) (*SDKListStateOutput, error) {
	items, err := s.store.ListRunState(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run state")
	}
	return &SDKListStateOutput{Body: items}, nil
}

func (s *Server) handleSDKListWorkflowState(ctx context.Context, input *SDKRunIDInput) (*SDKListStateOutput, error) {
	workflowRunID, err := s.workflowStateScopeRunID(ctx, input.RunID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListRunState(ctx, workflowRunID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run state")
	}
	return &SDKListStateOutput{Body: items}, nil
}

type SDKDeleteStateInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}

func (s *Server) handleSDKDeleteState(ctx context.Context, input *SDKDeleteStateInput) (*struct{}, error) {
	if err := s.store.DeleteRunState(ctx, input.RunID, input.Key); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete run state")
	}
	return nil, nil
}

type SDKDeleteWorkflowStateInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}

func (s *Server) handleSDKDeleteWorkflowState(ctx context.Context, input *SDKDeleteWorkflowStateInput) (*struct{}, error) {
	workflowRunID, err := s.workflowStateScopeRunID(ctx, input.RunID)
	if err != nil {
		return nil, err
	}
	if err := s.store.DeleteRunState(ctx, workflowRunID, input.Key); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete run state")
	}
	return nil, nil
}

type ListRunStateInput struct {
	RunID string `path:"runID"`
}
type ListRunStateOutput struct{ Body any }

func (s *Server) handleListRunState(ctx context.Context, input *ListRunStateInput) (*ListRunStateOutput, error) {
	items, err := s.store.ListRunState(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list run state")
	}
	return &ListRunStateOutput{Body: items}, nil
}

// SDK agent messaging callback.

type SDKSendMessageRequest struct {
	TargetAgentSlug string          `json:"target_agent_slug" validate:"required"`
	Payload         json.RawMessage `json:"payload"`
}

type SDKSendMessageInput struct {
	RunID string `path:"runID"`
	Body  SDKSendMessageRequest
}

type SDKSendMessageOutput struct {
	Body struct {
		MessageID string `json:"message_id"`
	}
}

func (s *Server) handleSDKSendMessage(ctx context.Context, input *SDKSendMessageInput) (*SDKSendMessageOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	// Prefer the cryptographically-bound agent ID from the JWT token.
	// Fall back to slug-based resolution for tokens issued before this change.
	sourceAgentID := agentIDFromTokenContext(ctx)
	if sourceAgentID == "" {
		sourceAgentID, err = s.resolveSourceAgentBySlug(ctx, run)
		if err != nil {
			return nil, err
		}
	}

	// Resolve target agent by slug.
	targetAgentID, targetErr := s.resolveTargetAgentBySlug(ctx, run.ProjectID, input.Body.TargetAgentSlug)
	if targetErr != nil {
		return nil, targetErr
	}

	msgStore, ok := s.store.(agents.MessageStore)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("messaging not supported")
	}

	msgSvc := agents.NewAgentMessageService(msgStore)
	msg, sendErr := msgSvc.Send(ctx, agents.SendRequest{
		ProjectID:     run.ProjectID,
		SourceAgentID: sourceAgentID,
		TargetAgentID: targetAgentID,
		SourceRunID:   run.ID,
		Payload:       input.Body.Payload,
	})
	if sendErr != nil {
		return nil, mapMessageError(sendErr)
	}

	return &SDKSendMessageOutput{Body: struct {
		MessageID string `json:"message_id"`
	}{MessageID: msg.ID}}, nil
}

// resolveSourceAgentBySlug is the legacy path for tokens that don't
// include an agent_id claim. It scans project agents by job slug tag.
func (s *Server) resolveSourceAgentBySlug(ctx context.Context, run *domain.JobRun) (string, error) {
	job, jobErr := s.store.GetJob(ctx, run.JobID)
	if jobErr != nil || job == nil {
		return "", huma.Error500InternalServerError("failed to resolve agent for run")
	}
	sourceAgentSlug := job.Tags["agent_slug"]
	if sourceAgentSlug == "" {
		return "", huma.Error409Conflict("run is not associated with an agent")
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return "", huma.Error503ServiceUnavailable("agent resolution not supported")
	}
	agentList, listErr := q.ListAgents(ctx, run.ProjectID, 500, nil)
	if listErr != nil {
		return "", huma.Error500InternalServerError("failed to list agents")
	}
	for _, a := range agentList {
		if a.Slug == sourceAgentSlug {
			return a.ID, nil
		}
	}
	return "", huma.Error500InternalServerError("source agent not found")
}

// resolveTargetAgentBySlug finds a target agent by slug within a project.
func (s *Server) resolveTargetAgentBySlug(ctx context.Context, projectID, slug string) (string, error) {
	q, ok := s.store.(*store.Queries)
	if !ok {
		return "", huma.Error503ServiceUnavailable("agent resolution not supported")
	}
	agentList, listErr := q.ListAgents(ctx, projectID, 500, nil)
	if listErr != nil {
		return "", huma.Error500InternalServerError("failed to list agents")
	}
	for _, a := range agentList {
		if a.Slug == slug {
			return a.ID, nil
		}
	}
	return "", huma.Error404NotFound("target agent not found")
}

// SDK workflow submission callback.

type SDKSubmitWorkflowRequest struct {
	Name  string          `json:"name" validate:"required"`
	Slug  string          `json:"slug" validate:"required"`
	Steps json.RawMessage `json:"steps" validate:"required"`
}

type SDKSubmitWorkflowInput struct {
	RunID string `path:"runID"`
	Body  SDKSubmitWorkflowRequest
}

type SDKSubmitWorkflowOutput struct {
	Body struct {
		WorkflowRunID string `json:"workflow_run_id"`
	}
}

func (s *Server) handleSDKSubmitWorkflow(ctx context.Context, input *SDKSubmitWorkflowInput) (*SDKSubmitWorkflowOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	name := strings.TrimSpace(input.Body.Name)
	slug := strings.TrimSpace(input.Body.Slug)
	if name == "" {
		return nil, huma.Error400BadRequest("workflow name is required")
	}
	if slug == "" {
		return nil, huma.Error400BadRequest("workflow slug is required")
	}
	if len(input.Body.Steps) == 0 || string(input.Body.Steps) == "null" {
		return nil, huma.Error400BadRequest("workflow steps are required")
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("workflow submission not supported")
	}

	// Look up existing workflow by slug. Agents cannot create new workflow
	// definitions -- only trigger existing ones. This prevents a compromised
	// runtime from creating unlimited workflow records.
	existingWF, wfLookupErr := q.GetWorkflowBySlug(ctx, run.ProjectID, slug)
	if wfLookupErr != nil || existingWF == nil {
		return nil, huma.Error404NotFound("workflow not found -- agents can only trigger existing workflows")
	}
	wfID := existingWF.ID

	// Trigger the workflow run.
	if s.workflowEngine == nil {
		return nil, huma.Error503ServiceUnavailable("workflow engine unavailable")
	}

	wfRun, triggerErr := s.workflowEngine.TriggerWorkflow(ctx, wfID, run.ProjectID, input.Body.Steps, "agent:"+run.ID, nil, nil)
	if triggerErr != nil {
		slog.Error("sdk: failed to trigger workflow from agent", "run_id", run.ID, "workflow_id", wfID, "error", triggerErr)
		return nil, huma.Error500InternalServerError("failed to trigger workflow")
	}

	s.publishRunEvent(ctx, input.RunID, map[string]any{
		"type": "workflow_submitted", "workflow_run_id": wfRun.ID,
		"workflow_slug": slug, "timestamp": time.Now().UTC(),
	})

	return &SDKSubmitWorkflowOutput{Body: struct {
		WorkflowRunID string `json:"workflow_run_id"`
	}{WorkflowRunID: wfRun.ID}}, nil
}
