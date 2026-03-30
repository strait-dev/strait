package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
)

type mockMessageStore struct {
	agents   map[string]*domain.Agent
	messages []domain.AgentMessage
}

func (m *mockMessageStore) CreateAgentMessage(_ context.Context, msg *domain.AgentMessage) error {
	m.messages = append(m.messages, *msg)
	return nil
}

func (m *mockMessageStore) GetAgentMessage(_ context.Context, id string) (*domain.AgentMessage, error) {
	for _, msg := range m.messages {
		if msg.ID == id {
			return &msg, nil
		}
	}
	return nil, nil
}

func (m *mockMessageStore) ListAgentMessagesByChain(_ context.Context, chainID string) ([]domain.AgentMessage, error) {
	var result []domain.AgentMessage
	for _, msg := range m.messages {
		if msg.ChainID == chainID {
			result = append(result, msg)
		}
	}
	return result, nil
}

func (m *mockMessageStore) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	if agent, ok := m.agents[id]; ok {
		return agent, nil
	}
	return nil, ErrTargetNotFound
}

func newMockStore(agentIDs ...string) *mockMessageStore {
	store := &mockMessageStore{
		agents: make(map[string]*domain.Agent),
	}
	for _, id := range agentIDs {
		store.agents[id] = &domain.Agent{ID: id, ProjectID: "proj-1"}
	}
	return store
}

func TestSendMessageLinearChain(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b", "c")
	svc := NewAgentMessageService(store)
	ctx := context.Background()

	// A -> B -> C: no cycle.
	msg1, err := svc.Send(ctx, SendRequest{ProjectID: "proj-1", SourceAgentID: "a", TargetAgentID: "b", ChainID: "chain-1", ChainDepth: 1})
	if err != nil {
		t.Fatalf("A->B: %v", err)
	}
	store.messages = append(store.messages[:0], *msg1)

	_, err = svc.Send(ctx, SendRequest{ProjectID: "proj-1", SourceAgentID: "b", TargetAgentID: "c", ChainID: "chain-1", ChainDepth: 2})
	if err != nil {
		t.Fatalf("B->C: %v", err)
	}
}

func TestSendMessageSelfLoopRejected(t *testing.T) {
	t.Parallel()
	store := newMockStore("a")
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{ProjectID: "proj-1", SourceAgentID: "a", TargetAgentID: "a"})
	if !errors.Is(err, ErrSelfMessage) {
		t.Fatalf("expected ErrSelfMessage, got %v", err)
	}
}

func TestSendMessageTriangleCycleDetected(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b", "c")
	svc := NewAgentMessageService(store)
	ctx := context.Background()

	// Build chain: A -> B -> C, then try C -> A (cycle).
	store.messages = []domain.AgentMessage{
		{SourceAgentID: "a", TargetAgentID: "b", ChainID: "chain-1", ChainDepth: 1},
		{SourceAgentID: "b", TargetAgentID: "c", ChainID: "chain-1", ChainDepth: 2},
	}

	_, err := svc.Send(ctx, SendRequest{ProjectID: "proj-1", SourceAgentID: "c", TargetAgentID: "a", ChainID: "chain-1", ChainDepth: 3})
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

func TestSendMessageDiamondNoCycle(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b", "c", "d")
	svc := NewAgentMessageService(store)
	ctx := context.Background()

	// Diamond: A -> B, A -> C, B -> D, C -> D. No cycle.
	store.messages = []domain.AgentMessage{
		{SourceAgentID: "a", TargetAgentID: "b", ChainID: "chain-1", ChainDepth: 1},
		{SourceAgentID: "a", TargetAgentID: "c", ChainID: "chain-1", ChainDepth: 1},
		{SourceAgentID: "b", TargetAgentID: "d", ChainID: "chain-1", ChainDepth: 2},
	}

	_, err := svc.Send(ctx, SendRequest{ProjectID: "proj-1", SourceAgentID: "c", TargetAgentID: "d", ChainID: "chain-1", ChainDepth: 2})
	if err != nil {
		t.Fatalf("diamond should not be a cycle: %v", err)
	}
}

func TestSendMessageChainDepthExceeded(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b")
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "a",
		TargetAgentID: "b",
		ChainDepth:    maxChainDepth + 1,
	})
	if !errors.Is(err, ErrChainTooDeep) {
		t.Fatalf("expected ErrChainTooDeep, got %v", err)
	}
}

func TestSendMessageTargetNotFound(t *testing.T) {
	t.Parallel()
	store := newMockStore("a")
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{ProjectID: "proj-1", SourceAgentID: "a", TargetAgentID: "nonexistent"})
	if !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("expected ErrTargetNotFound, got %v", err)
	}
}

func TestSendMessagePayloadNormalized(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b")
	svc := NewAgentMessageService(store)

	msg, err := svc.Send(context.Background(), SendRequest{ProjectID: "proj-1", SourceAgentID: "a", TargetAgentID: "b", Payload: nil})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if string(msg.Payload) != "{}" {
		t.Fatalf("payload = %s, want {}", msg.Payload)
	}

	msg2, err := svc.Send(context.Background(), SendRequest{ProjectID: "proj-1", SourceAgentID: "a", TargetAgentID: "b", Payload: json.RawMessage(`{"key":"value"}`)})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if string(msg2.Payload) != `{"key":"value"}` {
		t.Fatalf("payload = %s", msg2.Payload)
	}
}
