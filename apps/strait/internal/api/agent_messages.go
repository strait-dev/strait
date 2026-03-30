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

type SendAgentMessageRequest struct {
	Payload json.RawMessage `json:"payload"`
	ChainID string          `json:"chain_id,omitempty"`
}

type SendAgentMessageInput struct {
	AgentID string `path:"agentID"`
	Body    SendAgentMessageRequest
}

type SendAgentMessageOutput struct {
	Body *domain.AgentMessage
}

type ListAgentMessagesInput struct {
	AgentID string `path:"agentID"`
	Limit   string `query:"limit"`
}

type ListAgentMessagesOutput struct {
	Body []domain.AgentMessage
}

type AgentTopologyNode struct {
	AgentID   string `json:"agent_id"`
	AgentSlug string `json:"agent_slug"`
	AgentName string `json:"agent_name"`
}

type AgentTopologyEdge struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	MessageCount int    `json:"message_count"`
}

type AgentTopologyOutput struct {
	Body struct {
		Nodes []AgentTopologyNode `json:"nodes"`
		Edges []AgentTopologyEdge `json:"edges"`
	}
}

type GetAgentTopologyInput struct {
}

func (s *Server) handleSendAgentMessage(ctx context.Context, input *SendAgentMessageInput) (*SendAgentMessageOutput, error) {
	svc, err := s.requireAgentService()
	if err != nil {
		return nil, err
	}

	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	// Resolve the source agent (the caller is sending TO this agent).
	// For API-initiated messages, we need a source_agent_id from the request or context.
	// For now, treat this as an external message with no source agent.
	msgSvc := agents.NewAgentMessageService(s.store.(agents.MessageStore))
	_ = svc

	msg, sendErr := msgSvc.Send(ctx, agents.SendRequest{
		ProjectID:     projectID,
		SourceAgentID: "", // Will be set when called from SDK callback
		TargetAgentID: input.AgentID,
		Payload:       input.Body.Payload,
		ChainID:       input.Body.ChainID,
	})
	if sendErr != nil {
		return nil, mapMessageError(sendErr)
	}

	return &SendAgentMessageOutput{Body: msg}, nil
}

func (s *Server) handleListAgentMessages(ctx context.Context, input *ListAgentMessagesInput) (*ListAgentMessagesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error500InternalServerError("message listing not supported")
	}

	messages, err := q.ListAgentMessagesByAgent(ctx, input.AgentID, 50, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list messages")
	}
	if messages == nil {
		messages = []domain.AgentMessage{}
	}
	return &ListAgentMessagesOutput{Body: messages}, nil
}

func (s *Server) handleGetAgentTopology(ctx context.Context, _ *GetAgentTopologyInput) (*AgentTopologyOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required")
	}

	q, ok := s.store.(*store.Queries)
	if !ok {
		return nil, huma.Error500InternalServerError("topology not supported")
	}

	// Get all agents for the project to build nodes.
	agentList, err := q.ListAgents(ctx, projectID, 500, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list agents")
	}

	nodes := make([]AgentTopologyNode, 0, len(agentList))
	for _, a := range agentList {
		nodes = append(nodes, AgentTopologyNode{
			AgentID:   a.ID,
			AgentSlug: a.Slug,
			AgentName: a.Name,
		})
	}

	// Get edges from message flow.
	edgeRows, err := q.GetAgentTopologyEdges(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get topology edges")
	}

	edges := make([]AgentTopologyEdge, 0, len(edgeRows))
	for _, e := range edgeRows {
		edges = append(edges, AgentTopologyEdge{
			Source:       e.SourceAgentID,
			Target:       e.TargetAgentID,
			MessageCount: e.MessageCount,
		})
	}

	out := &AgentTopologyOutput{}
	out.Body.Nodes = nodes
	out.Body.Edges = edges
	return out, nil
}

func mapMessageError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, agents.ErrSelfMessage):
		return huma.Error400BadRequest("agent cannot send a message to itself")
	case errors.Is(err, agents.ErrCycleDetected):
		return huma.Error409Conflict("circular message chain detected")
	case errors.Is(err, agents.ErrChainTooDeep):
		return huma.Error400BadRequest("message chain exceeds maximum depth")
	case errors.Is(err, agents.ErrTargetNotFound):
		return huma.Error404NotFound("target agent not found")
	default:
		return huma.Error500InternalServerError("failed to send message")
	}
}
