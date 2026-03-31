package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"strait/internal/domain"

	"github.com/google/uuid"
)

const (
	maxChainDepth       = 20
	maxMessagesPerChain = 100
)

var (
	ErrCycleDetected     = errors.New("circular agent message chain detected")
	ErrChainTooDeep      = errors.New("agent message chain exceeds maximum depth")
	ErrChainMessageLimit = errors.New("message chain exceeds maximum message count")
	ErrSelfMessage       = errors.New("agent cannot send a message to itself")
	ErrTargetNotFound    = errors.New("target agent not found")
)

// MessageStore defines the store methods needed by the messaging service.
type MessageStore interface {
	CreateAgentMessage(ctx context.Context, msg *domain.AgentMessage) error
	GetAgentMessage(ctx context.Context, id string) (*domain.AgentMessage, error)
	ListAgentMessagesByChain(ctx context.Context, chainID string) ([]domain.AgentMessage, error)
	GetAgent(ctx context.Context, id string) (*domain.Agent, error)
}

// AgentMessageService handles agent-to-agent message delivery with cycle detection.
type AgentMessageService struct {
	store MessageStore
}

// NewAgentMessageService creates a new messaging service.
func NewAgentMessageService(store MessageStore) *AgentMessageService {
	return &AgentMessageService{store: store}
}

// SendRequest contains the parameters for sending an agent message.
type SendRequest struct {
	ProjectID     string
	SourceAgentID string
	TargetAgentID string
	SourceRunID   string
	ChainID       string
	ChainDepth    int
	Payload       json.RawMessage
}

// Send validates, checks for cycles, and persists a new agent message.
func (s *AgentMessageService) Send(ctx context.Context, req SendRequest) (*domain.AgentMessage, error) {
	if strings.TrimSpace(req.SourceAgentID) == "" {
		return nil, fmt.Errorf("source_agent_id is required")
	}
	if strings.TrimSpace(req.TargetAgentID) == "" {
		return nil, ErrTargetNotFound
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if req.SourceAgentID == req.TargetAgentID {
		return nil, ErrSelfMessage
	}

	// Verify target agent exists.
	target, err := s.store.GetAgent(ctx, req.TargetAgentID)
	if err != nil {
		if errors.Is(err, ErrTargetNotFound) {
			return nil, ErrTargetNotFound
		}
		return nil, fmt.Errorf("look up target agent: %w", err)
	}
	if target == nil {
		return nil, ErrTargetNotFound
	}
	if target.ProjectID != req.ProjectID {
		return nil, ErrTargetNotFound
	}

	// Assign chain ID if this is the first message in a chain.
	chainID := req.ChainID
	if chainID == "" {
		chainID = uuid.Must(uuid.NewV7()).String()
	}

	depth := req.ChainDepth
	if depth <= 0 {
		depth = 1
	}

	// Check chain depth limit.
	if depth > maxChainDepth {
		return nil, ErrChainTooDeep
	}

	// Load chain messages once for both cycle detection and flood check.
	chainMessages, chainErr := s.store.ListAgentMessagesByChain(ctx, chainID)
	if chainErr != nil {
		return nil, fmt.Errorf("load chain messages: %w", chainErr)
	}

	// Run cycle detection on the chain.
	if err := detectCycleFromMessages(chainMessages, req.SourceAgentID, req.TargetAgentID); err != nil {
		return nil, err
	}

	// Enforce per-chain message limit to prevent message flooding.
	if len(chainMessages) >= maxMessagesPerChain {
		return nil, ErrChainMessageLimit
	}

	msg := &domain.AgentMessage{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ProjectID:     req.ProjectID,
		SourceAgentID: req.SourceAgentID,
		TargetAgentID: req.TargetAgentID,
		SourceRunID:   req.SourceRunID,
		ChainID:       chainID,
		ChainDepth:    depth,
		Payload:       normalizePayload(req.Payload),
		Status:        domain.AgentMessagePending,
	}

	if err := s.store.CreateAgentMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("create agent message: %w", err)
	}

	return msg, nil
}

// detectCycleFromMessages checks whether delivering a message from source to
// target would create a cycle in the message chain. It builds an adjacency
// list from the provided messages and runs DFS to find back edges.
func detectCycleFromMessages(messages []domain.AgentMessage, sourceAgentID, targetAgentID string) error {
	// Build adjacency list: source -> set of targets.
	graph := make(map[string]map[string]struct{})
	for _, msg := range messages {
		if graph[msg.SourceAgentID] == nil {
			graph[msg.SourceAgentID] = make(map[string]struct{})
		}
		graph[msg.SourceAgentID][msg.TargetAgentID] = struct{}{}
	}

	// Add the proposed edge.
	if graph[sourceAgentID] == nil {
		graph[sourceAgentID] = make(map[string]struct{})
	}
	graph[sourceAgentID][targetAgentID] = struct{}{}

	// DFS cycle detection.
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully processed
	)

	color := make(map[string]int)
	var hasCycle bool

	var dfs func(node string)
	dfs = func(node string) {
		if hasCycle {
			return
		}
		color[node] = gray
		for neighbor := range graph[node] {
			switch color[neighbor] {
			case gray:
				hasCycle = true
				return
			case white:
				dfs(neighbor)
			}
		}
		color[node] = black
	}

	// Run DFS from all nodes.
	for node := range graph {
		if color[node] == white {
			dfs(node)
			if hasCycle {
				return ErrCycleDetected
			}
		}
	}

	return nil
}

func normalizePayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
