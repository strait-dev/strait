package agents

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

// mockModelRoutingStore is an in-memory store for testing model routing.
type mockModelRoutingStore struct {
	routes map[string]*domain.ModelRoute // key: agentID+tier
}

func newMockModelRoutingStore() *mockModelRoutingStore {
	return &mockModelRoutingStore{routes: make(map[string]*domain.ModelRoute)}
}

func (m *mockModelRoutingStore) key(agentID, tier string) string {
	return agentID + ":" + tier
}

func (m *mockModelRoutingStore) GetModelRouting(ctx context.Context, agentID string) ([]domain.ModelRoute, error) {
	var result []domain.ModelRoute
	for _, r := range m.routes {
		if r.AgentID == agentID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockModelRoutingStore) GetModelRoutingByTier(ctx context.Context, agentID, tier string) (*domain.ModelRoute, error) {
	r, ok := m.routes[m.key(agentID, tier)]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *mockModelRoutingStore) UpsertModelRouting(ctx context.Context, route *domain.ModelRoute) error {
	k := m.key(route.AgentID, route.Tier)
	m.routes[k] = route
	return nil
}

// --- ClassifyRequest tests ---

func TestClassifyRequest_Simple(t *testing.T) {
	tier := ClassifyRequest(100, 0, false)
	if tier != TierSimple {
		t.Errorf("expected simple, got %s", tier)
	}
}

func TestClassifyRequest_Standard(t *testing.T) {
	tier := ClassifyRequest(1000, 0, false)
	if tier != TierStandard {
		t.Errorf("expected standard, got %s", tier)
	}
}

func TestClassifyRequest_StandardWithTools(t *testing.T) {
	tier := ClassifyRequest(0, 2, false)
	if tier != TierStandard {
		t.Errorf("expected standard, got %s", tier)
	}
}

func TestClassifyRequest_Complex_HighTokens(t *testing.T) {
	tier := ClassifyRequest(5000, 0, false)
	if tier != TierComplex {
		t.Errorf("expected complex, got %s", tier)
	}
}

func TestClassifyRequest_Complex_ManyTools(t *testing.T) {
	tier := ClassifyRequest(0, 4, false)
	if tier != TierComplex {
		t.Errorf("expected complex, got %s", tier)
	}
}

func TestClassifyRequest_Complex_StructuredOutput(t *testing.T) {
	tier := ClassifyRequest(0, 0, true)
	if tier != TierComplex {
		t.Errorf("expected complex, got %s", tier)
	}
}

// --- ResolveModel tests ---

func TestResolveModel_WithRouting(t *testing.T) {
	store := newMockModelRoutingStore()
	store.routes["agent1:standard"] = &domain.ModelRoute{
		AgentID: "agent1",
		Tier:    "standard",
		Model:   "gpt-4o",
	}
	router := NewModelRouter(store)

	model, err := router.ResolveModel(context.Background(), "agent1", "gpt-3.5-turbo", TierStandard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", model)
	}
}

func TestResolveModel_NoRouting(t *testing.T) {
	store := newMockModelRoutingStore()
	router := NewModelRouter(store)

	model, err := router.ResolveModel(context.Background(), "agent1", "gpt-3.5-turbo", TierStandard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "gpt-3.5-turbo" {
		t.Errorf("expected gpt-3.5-turbo, got %s", model)
	}
}

// --- CheckQualityGate tests ---

func TestCheckQualityGate_AboveThreshold(t *testing.T) {
	store := newMockModelRoutingStore()
	store.routes["agent1:standard"] = &domain.ModelRoute{
		AgentID:       "agent1",
		Tier:          "standard",
		Model:         "gpt-4o",
		PreviousModel: "gpt-3.5-turbo",
		QualityScore:  90.0,
	}
	router := NewModelRouter(store)

	err := router.CheckQualityGate(context.Background(), "agent1", TierStandard, 92.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := store.routes["agent1:standard"]
	if route.Model != "gpt-4o" {
		t.Errorf("expected model to remain gpt-4o, got %s", route.Model)
	}
	if route.QualityScore != 92.0 {
		t.Errorf("expected quality score 92.0, got %f", route.QualityScore)
	}
}

func TestCheckQualityGate_BelowThreshold_Reverts(t *testing.T) {
	store := newMockModelRoutingStore()
	store.routes["agent1:standard"] = &domain.ModelRoute{
		AgentID:       "agent1",
		Tier:          "standard",
		Model:         "gpt-4o",
		PreviousModel: "gpt-3.5-turbo",
		QualityScore:  90.0,
	}
	router := NewModelRouter(store)

	err := router.CheckQualityGate(context.Background(), "agent1", TierStandard, 70.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := store.routes["agent1:standard"]
	if route.Model != "gpt-3.5-turbo" {
		t.Errorf("expected model to revert to gpt-3.5-turbo, got %s", route.Model)
	}
	if route.PreviousModel != "gpt-4o" {
		t.Errorf("expected previous model to be gpt-4o, got %s", route.PreviousModel)
	}
	if route.QualityScore != 70.0 {
		t.Errorf("expected quality score 70.0, got %f", route.QualityScore)
	}
}

func TestCheckQualityGate_BelowThreshold_NoPrevious(t *testing.T) {
	store := newMockModelRoutingStore()
	store.routes["agent1:standard"] = &domain.ModelRoute{
		AgentID:      "agent1",
		Tier:         "standard",
		Model:        "gpt-4o",
		QualityScore: 90.0,
	}
	router := NewModelRouter(store)

	err := router.CheckQualityGate(context.Background(), "agent1", TierStandard, 70.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := store.routes["agent1:standard"]
	if route.Model != "gpt-4o" {
		t.Errorf("expected model to remain gpt-4o (no previous), got %s", route.Model)
	}
	if route.QualityScore != 70.0 {
		t.Errorf("expected quality score 70.0, got %f", route.QualityScore)
	}
}


func TestCountConfigTools_Empty(t *testing.T) {
	if got := countConfigTools(nil); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestCountConfigTools_WithTools(t *testing.T) {
	cfg := json.RawMessage(`{"tools":[{"name":"a"},{"name":"b"},{"name":"c"}]}`)
	if got := countConfigTools(cfg); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestCountConfigTools_NoToolsKey(t *testing.T) {
	cfg := json.RawMessage(`{"model":"gpt-4"}`)
	if got := countConfigTools(cfg); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}
