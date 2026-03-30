package api

import (
	"context"
	"encoding/json"
	"errors"

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

	// Find the source agent by matching the run's job ID against agent backing jobs.
	job, jobErr := s.store.GetJob(ctx, run.JobID)
	if jobErr != nil || job == nil {
		return nil, huma.Error500InternalServerError("failed to resolve agent for run")
	}
	sourceAgentSlug := job.Tags["agent_slug"]
	if sourceAgentSlug == "" {
		return nil, huma.Error409Conflict("run is not associated with an agent")
	}

	// Find source agent by listing agents and matching slug.
	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("agent resolution not supported")
	}
	agentList, listErr := q.ListAgents(ctx, run.ProjectID, 500, nil)
	if listErr != nil {
		return nil, huma.Error500InternalServerError("failed to list agents")
	}

	var sourceAgentID, targetAgentID string
	for _, a := range agentList {
		if a.Slug == sourceAgentSlug {
			sourceAgentID = a.ID
		}
		if a.Slug == input.Body.TargetAgentSlug {
			targetAgentID = a.ID
		}
	}
	if sourceAgentID == "" {
		return nil, huma.Error500InternalServerError("source agent not found")
	}
	if targetAgentID == "" {
		return nil, huma.Error404NotFound("target agent not found")
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
		WorkflowID string `json:"workflow_id"`
	}
}

func (s *Server) handleSDKSubmitWorkflow(ctx context.Context, input *SDKSubmitWorkflowInput) (*SDKSubmitWorkflowOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		return nil, huma.Error404NotFound("run not found")
	}

	// For now, store the workflow definition as a run state entry so it can
	// be picked up by the workflow engine. Full workflow creation requires
	// the workflow store interface which is not part of the SDK callback path.
	state := &domain.RunState{
		RunID:    run.ID,
		StateKey: "_submitted_workflow",
		Value: mustSDKJSON(map[string]any{
			"name":  input.Body.Name,
			"slug":  input.Body.Slug,
			"steps": input.Body.Steps,
		}),
	}
	if err := s.store.UpsertRunState(ctx, state); err != nil {
		return nil, huma.Error500InternalServerError("failed to store workflow definition")
	}

	return &SDKSubmitWorkflowOutput{Body: struct {
		WorkflowID string `json:"workflow_id"`
	}{WorkflowID: run.ID + ":workflow"}}, nil
}

func mustSDKJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}
