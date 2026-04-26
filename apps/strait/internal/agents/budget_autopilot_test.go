package agents

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

// mockAutopilotStore is an in-memory mock for AutopilotStore.
type mockAutopilotStore struct {
	agent   *domain.Agent
	actions []*domain.AutopilotAction
}

func (m *mockAutopilotStore) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	if m.agent != nil && m.agent.ID == id {
		return m.agent, nil
	}
	return nil, nil
}

func (m *mockAutopilotStore) CreateAutopilotAction(_ context.Context, action *domain.AutopilotAction) error {
	action.ID = "act_test"
	action.CreatedAt = time.Now()
	m.actions = append(m.actions, action)
	return nil
}

func (m *mockAutopilotStore) GetLatestAutopilotAction(_ context.Context, agentID string) (*domain.AutopilotAction, error) {
	for i := len(m.actions) - 1; i >= 0; i-- {
		if m.actions[i].AgentID == agentID {
			return m.actions[i], nil
		}
	}
	return nil, nil
}

// mockAutopilotRoutingStore is an in-memory mock for modelRoutingStore.
type mockAutopilotRoutingStore struct {
	routes map[string]map[string]*domain.ModelRoute // agentID -> tier -> route
}

func newMockAutopilotRoutingStore() *mockAutopilotRoutingStore {
	return &mockAutopilotRoutingStore{routes: make(map[string]map[string]*domain.ModelRoute)}
}

func (m *mockAutopilotRoutingStore) GetModelRouting(_ context.Context, agentID string) ([]domain.ModelRoute, error) {
	tiers, ok := m.routes[agentID]
	if !ok {
		return nil, nil
	}
	var out []domain.ModelRoute
	for _, r := range tiers {
		out = append(out, *r)
	}
	return out, nil
}

func (m *mockAutopilotRoutingStore) GetModelRoutingByTier(_ context.Context, agentID, tier string) (*domain.ModelRoute, error) {
	tiers, ok := m.routes[agentID]
	if !ok {
		return nil, nil
	}
	r, ok := tiers[tier]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *mockAutopilotRoutingStore) UpsertModelRouting(_ context.Context, route *domain.ModelRoute) error {
	if m.routes[route.AgentID] == nil {
		m.routes[route.AgentID] = make(map[string]*domain.ModelRoute)
	}
	route.ID = "route_test"
	route.UpdatedAt = time.Now()
	m.routes[route.AgentID][route.Tier] = route
	return nil
}

func makeAgentWithAutopilot(id string, cfg domain.AutopilotConfig) *domain.Agent {
	autopilotJSON, _ := json.Marshal(cfg)
	configMap := map[string]json.RawMessage{"autopilot": autopilotJSON}
	fullConfig, _ := json.Marshal(configMap)
	return &domain.Agent{
		ID:    id,
		Model: "gpt-4o",
		Config: fullConfig,
	}
}

func TestBudgetAutopilot_Under80Pct(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:        true,
			BudgetMicrousd: 1000000,
			CheapestModel:  "gpt-4o-mini",
		}),
	}
	mrStore := newMockAutopilotRoutingStore()
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 700000) // 70%
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatal("expected no action for spend under 80%, got action")
	}
}

func TestBudgetAutopilot_80PctDowngradesSimple(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:        true,
			BudgetMicrousd: 1000000,
			CheapestModel:  "gpt-4o-mini",
		}),
	}
	mrStore := newMockAutopilotRoutingStore()
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 850000) // 85%
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action == nil {
		t.Fatal("expected downgrade action at 85%")
	}
	if action.Tier != "simple" {
		t.Fatalf("expected tier simple, got %s", action.Tier)
	}
	if action.NewModel != "gpt-4o-mini" {
		t.Fatalf("expected new model gpt-4o-mini, got %s", action.NewModel)
	}
	if action.Action != "downgrade" {
		t.Fatalf("expected action downgrade, got %s", action.Action)
	}
}

func TestBudgetAutopilot_90PctDowngradesStandard(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:        true,
			BudgetMicrousd: 1000000,
			CheapestModel:  "gpt-4o-mini",
		}),
	}
	mrStore := newMockAutopilotRoutingStore()
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 950000) // 95%
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action == nil {
		t.Fatal("expected downgrade action at 95%")
	}
	if action.Tier != "standard" {
		t.Fatalf("expected tier standard, got %s", action.Tier)
	}
	if action.NewModel != "gpt-4o-mini" {
		t.Fatalf("expected new model gpt-4o-mini, got %s", action.NewModel)
	}
}

func TestBudgetAutopilot_AlreadyCheapest(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:        true,
			BudgetMicrousd: 1000000,
			CheapestModel:  "gpt-4o-mini",
		}),
	}
	mrStore := newMockAutopilotRoutingStore()
	// Pre-set the simple tier to already use gpt-4o-mini.
	_ = mrStore.UpsertModelRouting(context.Background(), &domain.ModelRoute{
		AgentID: "agent1",
		Tier:    "simple",
		Model:   "gpt-4o-mini",
	})
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 850000) // 85%
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatal("expected no action when already at cheapest model")
	}
}

func TestBudgetAutopilot_ObservationWindow(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:         true,
			BudgetMicrousd:  1000000,
			CheapestModel:   "gpt-4o-mini",
			ObservationMins: 10,
		}),
		actions: []*domain.AutopilotAction{
			{
				AgentID:   "agent1",
				CreatedAt: time.Now().Add(-5 * time.Minute), // 5 mins ago
			},
		},
	}
	mrStore := newMockAutopilotRoutingStore()
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	// At 85% but within observation window (5 min < 10 min).
	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 850000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatal("expected no action within observation window")
	}
}

func TestBudgetAutopilot_LowBudgetSkipsWindow(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:         true,
			BudgetMicrousd:  1000000,
			CheapestModel:   "gpt-4o-mini",
			ObservationMins: 10,
		}),
		actions: []*domain.AutopilotAction{
			{
				AgentID:   "agent1",
				CreatedAt: time.Now().Add(-2 * time.Minute), // 2 mins ago
			},
		},
	}
	mrStore := newMockAutopilotRoutingStore()
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	// At 96% (remaining < 5%), should skip observation window.
	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 960000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action == nil {
		t.Fatal("expected action when remaining budget < 5%, even within observation window")
	}
	if action.Tier != "standard" {
		t.Fatalf("expected tier standard at 96%%, got %s", action.Tier)
	}
}

func TestBudgetAutopilot_Disabled(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:        false,
			BudgetMicrousd: 1000000,
			CheapestModel:  "gpt-4o-mini",
		}),
	}
	mrStore := newMockAutopilotRoutingStore()
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 950000) // 95%
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatal("expected no action when autopilot is disabled")
	}
}

func TestBudgetAutopilot_NoBudget(t *testing.T) {
	store := &mockAutopilotStore{
		agent: makeAgentWithAutopilot("agent1", domain.AutopilotConfig{
			Enabled:        true,
			BudgetMicrousd: 0,
			CheapestModel:  "gpt-4o-mini",
		}),
	}
	mrStore := newMockAutopilotRoutingStore()
	router := NewModelRouter(mrStore)
	autopilot := NewBudgetAutopilot(store, router)

	action, err := autopilot.CheckAndAdjust(context.Background(), "agent1", 950000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatal("expected no action when budget is 0")
	}
}
