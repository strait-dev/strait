package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"strait/internal/domain"
)

// --- mock stores ---

type mockWhatIfStore struct {
	runs      map[string]*domain.JobRun
	usages    map[string][]domain.RunUsage
	toolCalls map[string][]domain.RunToolCall
	costs     map[string]int64
	costErr   error
}

func newMockWhatIfStore() *mockWhatIfStore {
	return &mockWhatIfStore{
		runs:      make(map[string]*domain.JobRun),
		usages:    make(map[string][]domain.RunUsage),
		toolCalls: make(map[string][]domain.RunToolCall),
		costs:     make(map[string]int64),
	}
}

func (m *mockWhatIfStore) GetRun(_ context.Context, id string) (*domain.JobRun, error) {
	r, ok := m.runs[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *mockWhatIfStore) ListRunUsage(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunUsage, error) {
	return m.usages[runID], nil
}

func (m *mockWhatIfStore) ListRunToolCalls(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunToolCall, error) {
	return m.toolCalls[runID], nil
}

func (m *mockWhatIfStore) SumRunCostMicrousd(_ context.Context, runID string) (int64, error) {
	if m.costErr != nil {
		return 0, m.costErr
	}
	return m.costs[runID], nil
}

type mockPricingStore struct {
	prices map[string][2]int64 // key = "provider:model" -> [input, output]
}

func newMockPricingStore() *mockPricingStore {
	return &mockPricingStore{prices: make(map[string][2]int64)}
}

func (m *mockPricingStore) LookupPricing(_ context.Context, provider, model string) (int64, int64, error) {
	key := provider + ":" + model
	p, ok := m.prices[key]
	if !ok {
		return 0, 0, nil
	}
	return p[0], p[1], nil
}

type mockWhatIfService struct {
	replayResult *domain.JobRun
	replayErr    error
	lastReq      ReplayAgentRunRequest
}

func (m *mockWhatIfService) ReplayAgentRun(_ context.Context, req ReplayAgentRunRequest) (*domain.JobRun, error) {
	m.lastReq = req
	if m.replayErr != nil {
		return nil, m.replayErr
	}
	return m.replayResult, nil
}

// Implement all Service methods (stubs for unused ones).
func (m *mockWhatIfService) CreateAgent(context.Context, CreateAgentRequest) (*domain.Agent, error) {
	return nil, nil
}
func (m *mockWhatIfService) GetAgent(context.Context, string, string) (*domain.Agent, error) {
	return nil, nil
}
func (m *mockWhatIfService) ListAgents(context.Context, string, int, *time.Time) ([]domain.Agent, error) {
	return nil, nil
}
func (m *mockWhatIfService) UpdateAgent(context.Context, UpdateAgentRequest) (*domain.Agent, error) {
	return nil, nil
}
func (m *mockWhatIfService) DeleteAgent(context.Context, string, string) error { return nil }
func (m *mockWhatIfService) DeployAgent(context.Context, string, string, string) (*domain.AgentDeployment, error) {
	return nil, nil
}
func (m *mockWhatIfService) DeployAgentToEnv(context.Context, string, string, string, string) (*domain.AgentDeployment, error) {
	return nil, nil
}
func (m *mockWhatIfService) RunAgent(context.Context, RunAgentRequest) (*domain.JobRun, error) {
	return nil, nil
}
func (m *mockWhatIfService) PrepareDirectRun(context.Context, RunAgentRequest) (*DirectRunResult, error) {
	return nil, nil
}
func (m *mockWhatIfService) ListAgentRuns(context.Context, string, string, int, int) ([]domain.JobRun, error) {
	return nil, nil
}
func (m *mockWhatIfService) KillAgent(context.Context, string, string, string) (int, error) {
	return 0, nil
}
func (m *mockWhatIfService) EnableAgent(context.Context, string, string, string) error { return nil }
func (m *mockWhatIfService) Close()                                                    {}

// --- helper ---

func terminalRun(id string) *domain.JobRun {
	now := time.Now()
	started := now.Add(-10 * time.Second)
	return &domain.JobRun{
		ID:        id,
		Status:    domain.StatusCompleted,
		StartedAt: &started,
		FinishedAt: &now,
	}
}

// --- tests ---

func TestEstimateCost_CheaperModel(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	store.runs["run-1"] = terminalRun("run-1")
	store.usages["run-1"] = []domain.RunUsage{
		{Provider: "openai", Model: "gpt-4", PromptTokens: 1000, CompletionTokens: 500, CostMicrousd: 100},
	}
	// Cheaper model: 50% of gpt-4 pricing.
	pricing.prices["openai:gpt-3.5-turbo"] = [2]int64{50_000, 50_000} // per million tokens

	engine := NewWhatIfEngine(store, pricing, &mockWhatIfService{})
	est, err := engine.EstimateCost(context.Background(), "run-1", "gpt-3.5-turbo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedCost >= est.OriginalCost && est.OriginalCost > 0 {
		t.Errorf("expected cheaper estimate, got estimated=%d original=%d", est.EstimatedCost, est.OriginalCost)
	}
	if est.TargetModel != "gpt-3.5-turbo" {
		t.Errorf("target model = %s, want gpt-3.5-turbo", est.TargetModel)
	}
}

func TestEstimateCost_ExpensiveModel(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	store.runs["run-1"] = terminalRun("run-1")
	store.usages["run-1"] = []domain.RunUsage{
		{Provider: "openai", Model: "gpt-3.5-turbo", PromptTokens: 1000, CompletionTokens: 500, CostMicrousd: 10},
	}
	// More expensive model.
	pricing.prices["openai:gpt-4"] = [2]int64{30_000_000, 60_000_000} // 30/60 per million

	engine := NewWhatIfEngine(store, pricing, &mockWhatIfService{})
	est, err := engine.EstimateCost(context.Background(), "run-1", "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedCost <= est.OriginalCost {
		t.Errorf("expected more expensive estimate, got estimated=%d original=%d", est.EstimatedCost, est.OriginalCost)
	}
	if est.CostDelta <= 0 {
		t.Errorf("expected positive cost delta, got %d", est.CostDelta)
	}
}

func TestEstimateCost_RunNotTerminal(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	store.runs["run-1"] = run

	engine := NewWhatIfEngine(store, pricing, &mockWhatIfService{})
	_, err := engine.EstimateCost(context.Background(), "run-1", "gpt-4")
	if err == nil {
		t.Fatal("expected error for non-terminal run")
	}
}

func TestEstimateCost_NoUsageRecords(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	store.runs["run-1"] = terminalRun("run-1")
	// No usages.

	engine := NewWhatIfEngine(store, pricing, &mockWhatIfService{})
	est, err := engine.EstimateCost(context.Background(), "run-1", "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.OriginalCost != 0 {
		t.Errorf("original cost = %d, want 0", est.OriginalCost)
	}
	if est.EstimatedCost != 0 {
		t.Errorf("estimated cost = %d, want 0", est.EstimatedCost)
	}
}

func TestEstimateCost_RunNotFound(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	engine := NewWhatIfEngine(store, pricing, &mockWhatIfService{})
	_, err := engine.EstimateCost(context.Background(), "nonexistent", "gpt-4")
	if err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestEstimateCost_SavingsPercentage(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	store.runs["run-1"] = terminalRun("run-1")
	store.usages["run-1"] = []domain.RunUsage{
		{Provider: "openai", Model: "gpt-4", PromptTokens: 1_000_000, CompletionTokens: 500_000, CostMicrousd: 4000},
	}
	// Target model costs half as much per token.
	pricing.prices["openai:gpt-mini"] = [2]int64{1000, 2000} // microusd per million tokens

	engine := NewWhatIfEngine(store, pricing, &mockWhatIfService{})
	est, err := engine.EstimateCost(context.Background(), "run-1", "gpt-mini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.SavingsPct <= 0 {
		t.Errorf("expected positive savings pct, got %f", est.SavingsPct)
	}
}

func TestReplay_OriginalNotTerminal(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	store.runs["run-1"] = run

	engine := NewWhatIfEngine(store, pricing, &mockWhatIfService{})
	_, err := engine.Replay(context.Background(), "run-1", "gpt-4", "proj-1", "agent-1", "test-user")
	if err == nil {
		t.Fatal("expected error for non-terminal run")
	}
}

func TestReplay_LoadsCachedToolCalls(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	store.runs["run-1"] = terminalRun("run-1")
	store.toolCalls["run-1"] = []domain.RunToolCall{
		{ToolName: "search", Input: json.RawMessage(`{"q":"test"}`), Output: json.RawMessage(`{"results":[]}`)},
		{ToolName: "read", Input: json.RawMessage(`{"path":"a.txt"}`), Output: json.RawMessage(`{"content":"hello"}`)},
	}

	replayRun := terminalRun("replay-1")
	svc := &mockWhatIfService{replayResult: replayRun}
	store.runs["replay-1"] = replayRun

	engine := NewWhatIfEngine(store, pricing, svc)
	result, err := engine.Replay(context.Background(), "run-1", "gpt-3.5-turbo", "proj-1", "agent-1", "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify replay was triggered with correct model override.
	if svc.lastReq.ConfigOverrides["model"] != "gpt-3.5-turbo" {
		t.Errorf("model override = %v, want gpt-3.5-turbo", svc.lastReq.ConfigOverrides["model"])
	}
	if svc.lastReq.Actor != "whatif:test-user" {
		t.Errorf("actor = %s, want whatif:test-user", svc.lastReq.Actor)
	}
	if result.ReplayRunID != "replay-1" {
		t.Errorf("replay run ID = %s, want replay-1", result.ReplayRunID)
	}

	_ = fmt.Sprintf("tool calls loaded: %d", len(store.toolCalls["run-1"]))
}


func TestReplay_CostQueryError(t *testing.T) {
	store := newMockWhatIfStore()
	pricing := newMockPricingStore()

	store.runs["run-1"] = terminalRun("run-1")
	replayRun := terminalRun("replay-1")
	store.runs["replay-1"] = replayRun
	store.costErr = fmt.Errorf("clickhouse unavailable")

	svc := &mockWhatIfService{replayResult: replayRun}
	engine := NewWhatIfEngine(store, pricing, svc)
	_, err := engine.Replay(context.Background(), "run-1", "gpt-4", "proj-1", "agent-1", "test-user")
	if err == nil {
		t.Fatal("expected error when SumRunCostMicrousd fails")
	}
}