package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"strait/internal/domain"
)

// Adversarial tests for new agents code -- trying to break things.

func TestCostOptimizerWithZeroRunsReturnsNil(t *testing.T) {
	t.Parallel()
	store := &mockCostStore{runs: nil}
	agent := &domain.Agent{ID: "agent-12345678", Model: "gpt-5.4", Config: json.RawMessage(`{}`)}
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recs != nil {
		t.Fatalf("expected nil recs for 0 runs, got %d", len(recs))
	}
}

func TestCostOptimizerWithAllFailedRuns(t *testing.T) {
	t.Parallel()
	runs := make([]domain.JobRun, 10)
	for i := range runs {
		runs[i] = domain.JobRun{Status: domain.StatusFailed}
	}
	store := &mockCostStore{runs: runs}
	agent := &domain.Agent{ID: "agent-12345678", Model: "gpt-5.4-mini", Config: json.RawMessage(`{}`)}
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not recommend model downgrade when success rate is 0%.
	for _, rec := range recs {
		if rec.Type == RecModelDowngrade {
			t.Fatal("should not recommend model downgrade with 0% success rate")
		}
	}
}

func TestCostOptimizerCheapModelNotDowngraded(t *testing.T) {
	t.Parallel()
	runs := make([]domain.JobRun, 10)
	for i := range runs {
		runs[i] = domain.JobRun{Status: domain.StatusCompleted}
	}
	store := &mockCostStore{runs: runs}
	// gpt-5.4-mini is already cheap -- should not suggest downgrade.
	agent := &domain.Agent{ID: "agent-12345678", Model: "gpt-5.4-mini", Config: json.RawMessage(`{}`)}
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, rec := range recs {
		if rec.Type == RecModelDowngrade {
			t.Fatal("should not recommend downgrade for already-cheap model")
		}
	}
}

func TestCycleDetectionWithLargeLinearChain(t *testing.T) {
	t.Parallel()
	store := newMockStore()
	// Build agents a1 through a20.
	for i := 1; i <= 20; i++ {
		id := agentID(i)
		store.agents[id] = &domain.Agent{ID: id, ProjectID: "proj-1"}
	}

	svc := NewAgentMessageService(store)
	ctx := context.Background()

	// Build a linear chain: a1->a2->a3->...->a19.
	for i := 1; i < 20; i++ {
		store.messages = append(store.messages, domain.AgentMessage{
			SourceAgentID: agentID(i),
			TargetAgentID: agentID(i + 1),
			ChainID:       "long-chain",
			ChainDepth:    i,
		})
	}

	// a19->a20 should succeed (no cycle).
	_, err := svc.Send(ctx, SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: agentID(19),
		TargetAgentID: agentID(20),
		ChainID:       "long-chain",
		ChainDepth:    19,
	})
	if err != nil {
		t.Fatalf("linear chain should not be a cycle: %v", err)
	}

	// a20->a1 should detect cycle.
	_, err = svc.Send(ctx, SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: agentID(20),
		TargetAgentID: agentID(1),
		ChainID:       "long-chain",
		ChainDepth:    20,
	})
	if err == nil {
		t.Fatal("expected cycle detection for a20->a1")
	}
}

func TestCycleDetectionWithDisconnectedComponents(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b", "c", "d", "e")
	svc := NewAgentMessageService(store)
	ctx := context.Background()

	// Two disconnected components: a->b and c->d.
	store.messages = []domain.AgentMessage{
		{SourceAgentID: "a", TargetAgentID: "b", ChainID: "chain-1"},
		{SourceAgentID: "c", TargetAgentID: "d", ChainID: "chain-1"},
	}

	// d->e should succeed (no connection to a->b component).
	_, err := svc.Send(ctx, SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "d",
		TargetAgentID: "e",
		ChainID:       "chain-1",
		ChainDepth:    2,
	})
	if err != nil {
		t.Fatalf("disconnected components should not be a cycle: %v", err)
	}
}

func TestSendMessageWithMaxChainDepth(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b")
	svc := NewAgentMessageService(store)

	// Exactly at max depth should succeed.
	_, err := svc.Send(context.Background(), SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "a",
		TargetAgentID: "b",
		ChainDepth:    maxChainDepth,
	})
	if err != nil {
		t.Fatalf("max depth should succeed: %v", err)
	}

	// One over should fail.
	_, err = svc.Send(context.Background(), SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "a",
		TargetAgentID: "b",
		ChainDepth:    maxChainDepth + 1,
	})
	if err == nil {
		t.Fatal("expected ErrChainTooDeep")
	}
}

func TestSendMessageCrossProjectRejected(t *testing.T) {
	t.Parallel()
	store := &mockMessageStore{
		agents: map[string]*domain.Agent{
			"a": {ID: "a", ProjectID: "proj-1"},
			"b": {ID: "b", ProjectID: "proj-2"}, // Different project.
		},
	}
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "a",
		TargetAgentID: "b",
	})
	if err == nil {
		t.Fatal("expected error for cross-project message")
	}
}

func TestExtractWebhookURLWithNumericValue(t *testing.T) {
	t.Parallel()
	// webhook_url is a number, not a string.
	got := ExtractWebhookURL(json.RawMessage(`{"webhook_url": 123}`))
	if got != "" {
		t.Fatalf("numeric webhook_url should return empty, got %q", got)
	}
}

func TestExtractWebhookURLWithArrayConfig(t *testing.T) {
	t.Parallel()
	// Config is an array, not an object.
	got := ExtractWebhookURL(json.RawMessage(`[1, 2, 3]`))
	if got != "" {
		t.Fatalf("array config should return empty, got %q", got)
	}
}

func TestBuildBackingJobConfigParsing(t *testing.T) {
	t.Parallel()

	// Valid config with max_attempts and timeout_secs.
	job := buildBackingJob(CreateAgentRequest{
		ProjectID: "proj-1",
		Name:      "Test",
		Slug:      "test",
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{"max_attempts": 5, "timeout_secs": 600}`),
	})
	if job.MaxAttempts != 5 {
		t.Fatalf("MaxAttempts = %d, want 5", job.MaxAttempts)
	}
	if job.TimeoutSecs != 600 {
		t.Fatalf("TimeoutSecs = %d, want 600", job.TimeoutSecs)
	}

	// Invalid config (not JSON).
	job2 := buildBackingJob(CreateAgentRequest{
		ProjectID: "proj-1",
		Name:      "Test",
		Slug:      "test",
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`not valid json`),
	})
	if job2.MaxAttempts != 1 {
		t.Fatalf("invalid JSON MaxAttempts = %d, want default 1", job2.MaxAttempts)
	}

	// Overflow max_attempts (> 10 should be ignored).
	job3 := buildBackingJob(CreateAgentRequest{
		ProjectID: "proj-1",
		Name:      "Test",
		Slug:      "test",
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{"max_attempts": 999}`),
	})
	if job3.MaxAttempts != 1 {
		t.Fatalf("overflow MaxAttempts = %d, want default 1", job3.MaxAttempts)
	}
}

func TestNormalizePayloadEdgeCases(t *testing.T) {
	t.Parallel()

	if got := normalizePayload(nil); string(got) != "{}" {
		t.Fatalf("nil -> %s, want {}", got)
	}
	if got := normalizePayload(json.RawMessage(``)); string(got) != "{}" {
		t.Fatalf("empty -> %s, want {}", got)
	}
	if got := normalizePayload(json.RawMessage(`{"key":"val"}`)); string(got) != `{"key":"val"}` {
		t.Fatalf("valid -> %s", got)
	}
}

func TestCanaryRouterEdgeCases(t *testing.T) {
	t.Parallel()

	router := NewAgentCanaryRouter()

	// TrafficPct exactly 0 -> always source.
	canary := &AgentCanaryDeployment{
		Status: AgentCanaryStatusActive, TrafficPct: 0,
		SourceDeploymentID: "s", TargetDeploymentID: "t",
	}
	for range 10 {
		if got := router.Route(canary); got != "s" {
			t.Fatalf("0%% traffic should always route to source, got %s", got)
		}
	}

	// TrafficPct exactly 100 -> always target.
	canary.TrafficPct = 100
	for range 10 {
		if got := router.Route(canary); got != "t" {
			t.Fatalf("100%% traffic should always route to target, got %s", got)
		}
	}
}

func TestValidationEdgeCases(t *testing.T) {
	t.Parallel()

	// Very long name should fail.
	longName := strings.Repeat("a", 300)
	err := validateCreateRequest(CreateAgentRequest{
		ProjectID: "proj-1", Name: longName, Slug: "test", Model: "gpt-5.4",
	})
	if err == nil {
		t.Fatal("expected error for long name")
	}

	// Very long slug should fail.
	longSlug := strings.Repeat("a", 200)
	err = validateCreateRequest(CreateAgentRequest{
		ProjectID: "proj-1", Name: "Test", Slug: longSlug, Model: "gpt-5.4",
	})
	if err == nil {
		t.Fatal("expected error for long slug")
	}

	// Empty model should fail.
	err = validateCreateRequest(CreateAgentRequest{
		ProjectID: "proj-1", Name: "Test", Slug: "test", Model: "",
	})
	if err == nil {
		t.Fatal("expected error for empty model")
	}

	// Config that's not a JSON object should fail.
	err = validateConfig(json.RawMessage(`"just a string"`))
	if err == nil {
		t.Fatal("expected error for non-object config")
	}

	// Config that's an array should fail.
	err = validateConfig(json.RawMessage(`[1, 2, 3]`))
	if err == nil {
		t.Fatal("expected error for array config")
	}

	// Oversized config should fail.
	bigConfig := json.RawMessage(`{"key":"` + strings.Repeat("x", maxAgentConfigSize) + `"}`)
	err = validateConfig(bigConfig)
	if err == nil {
		t.Fatal("expected error for oversized config")
	}
}

func agentID(n int) string {
	return "agent-" + strings.Repeat("0", 3) + strconv.Itoa(n)
}

// -- Cost optimizer happy path tests.

func makeRuns(completed, failed int) []domain.JobRun {
	runs := make([]domain.JobRun, 0, completed+failed)
	for range completed {
		runs = append(runs, domain.JobRun{Status: domain.StatusCompleted})
	}
	for range failed {
		runs = append(runs, domain.JobRun{Status: domain.StatusFailed})
	}
	return runs
}

func TestGenerateRecommendations_ModelDowngrade(t *testing.T) {
	t.Parallel()
	store := &mockCostStore{runs: makeRuns(49, 1)} // 98% success
	agent := &domain.Agent{ID: "agent-1", Model: "gpt-5.4", Config: json.RawMessage(`{"budget":"$10"}`)}
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	found := false
	for _, r := range recs {
		if r.Type == RecModelDowngrade {
			found = true
			if r.SuggestedPatch["model"] != "gpt-5.4-mini" {
				t.Fatalf("suggested model = %v", r.SuggestedPatch["model"])
			}
		}
	}
	if !found {
		t.Fatal("expected model_downgrade recommendation")
	}
}

func TestGenerateRecommendations_BudgetReduction(t *testing.T) {
	t.Parallel()
	store := &mockCostStore{runs: makeRuns(8, 2)} // 10 runs, no budget
	agent := &domain.Agent{ID: "agent-1", Model: "claude-haiku-4-5", Config: json.RawMessage(`{}`)}
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	found := false
	for _, r := range recs {
		if r.Type == RecBudgetReduction {
			found = true
		}
	}
	if !found {
		t.Fatal("expected budget_reduction recommendation when no budget set")
	}
}

func TestGenerateRecommendations_PromptCaching(t *testing.T) {
	t.Parallel()
	store := &mockCostStore{runs: makeRuns(25, 0)}
	agent := &domain.Agent{ID: "agent-1", Model: "claude-haiku-4-5", Config: json.RawMessage(`{"budget":"$5"}`)}
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	found := false
	for _, r := range recs {
		if r.Type == RecPromptCaching {
			found = true
			if !strings.Contains(r.Description, "25") {
				t.Fatalf("description should mention run count, got %q", r.Description)
			}
		}
	}
	if !found {
		t.Fatal("expected prompt_caching recommendation for 25+ runs")
	}
}

func TestGenerateRecommendations_AllThree(t *testing.T) {
	t.Parallel()
	store := &mockCostStore{runs: makeRuns(49, 1)}                                         // 98% success, 50 runs
	agent := &domain.Agent{ID: "agent-1", Model: "gpt-5.4", Config: json.RawMessage(`{}`)} // expensive, no budget
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected 3 recommendations, got %d", len(recs))
	}
}

func TestGenerateRecommendations_StoreError(t *testing.T) {
	t.Parallel()
	store := &mockCostStore{err: errors.New("store error")}
	agent := &domain.Agent{ID: "agent-1", Model: "gpt-5.4"}
	_, err := GenerateRecommendations(context.Background(), store, agent)
	if err == nil {
		t.Fatal("expected error when store fails")
	}
}

func TestGenerateRecommendations_LowSuccessRate(t *testing.T) {
	t.Parallel()
	store := &mockCostStore{runs: makeRuns(3, 7)} // 30% success
	agent := &domain.Agent{ID: "agent-1", Model: "gpt-5.4", Config: json.RawMessage(`{}`)}
	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	for _, r := range recs {
		if r.Type == RecModelDowngrade {
			t.Fatal("should NOT recommend downgrade with low success rate")
		}
	}
}
