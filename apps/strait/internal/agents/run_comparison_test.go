package agents

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

type mockCompStore struct {
	runs      map[string]*domain.JobRun
	costs     map[string]int64
	tokens    map[string]int64
	toolCount map[string]int
	toolCalls map[string][]domain.RunToolCall
}

func (m *mockCompStore) GetRun(_ context.Context, id string) (*domain.JobRun, error) {
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return nil, ErrNotDeployed
}

func (m *mockCompStore) SumRunCostMicrousd(_ context.Context, runID string) (int64, error) {
	return m.costs[runID], nil
}

func (m *mockCompStore) SumRunTotalTokens(_ context.Context, runID string) (int64, error) {
	return m.tokens[runID], nil
}

func (m *mockCompStore) CountRunToolCalls(_ context.Context, runID string) (int, error) {
	return m.toolCount[runID], nil
}

func (m *mockCompStore) ListRunToolCalls(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunToolCall, error) {
	return m.toolCalls[runID], nil
}

func TestCompareAgentRuns_IdenticalRuns(t *testing.T) {
	t.Parallel()
	now := time.Now()
	start := now.Add(-10 * time.Second)
	store := &mockCompStore{
		runs: map[string]*domain.JobRun{
			"run-a": {ID: "run-a", Status: domain.StatusCompleted, Attempt: 1, StartedAt: &start, FinishedAt: &now},
			"run-b": {ID: "run-b", Status: domain.StatusCompleted, Attempt: 1, StartedAt: &start, FinishedAt: &now},
		},
		costs:     map[string]int64{"run-a": 500, "run-b": 500},
		tokens:    map[string]int64{"run-a": 1000, "run-b": 1000},
		toolCount: map[string]int{"run-a": 3, "run-b": 3},
	}

	comp, err := CompareAgentRuns(context.Background(), store, "run-a", "run-b")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if comp.CostDiff != 0 {
		t.Fatalf("CostDiff = %d, want 0", comp.CostDiff)
	}
	if comp.TokenDiff != 0 {
		t.Fatalf("TokenDiff = %d, want 0", comp.TokenDiff)
	}
	if !comp.StatusMatch {
		t.Fatal("StatusMatch should be true")
	}
}

func TestCompareAgentRuns_DifferentModels(t *testing.T) {
	t.Parallel()
	store := &mockCompStore{
		runs: map[string]*domain.JobRun{
			"run-a": {ID: "run-a", Status: domain.StatusCompleted, Attempt: 1},
			"run-b": {ID: "run-b", Status: domain.StatusCompleted, Attempt: 1},
		},
		costs:  map[string]int64{},
		tokens: map[string]int64{},
	}

	comp, err := CompareAgentRuns(context.Background(), store, "run-a", "run-b")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	// Both models are empty string -- they match.
	if !comp.ModelMatch {
		t.Fatal("ModelMatch should be true for identical empty models")
	}
}

func TestCompareAgentRuns_CostDiff(t *testing.T) {
	t.Parallel()
	store := &mockCompStore{
		runs: map[string]*domain.JobRun{
			"run-a": {ID: "run-a", Status: domain.StatusCompleted, Attempt: 1},
			"run-b": {ID: "run-b", Status: domain.StatusCompleted, Attempt: 1},
		},
		costs:  map[string]int64{"run-a": 1000, "run-b": 300},
		tokens: map[string]int64{},
	}

	comp, err := CompareAgentRuns(context.Background(), store, "run-a", "run-b")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if comp.CostDiff != 700 {
		t.Fatalf("CostDiff = %d, want 700 (A is more expensive)", comp.CostDiff)
	}
}

func TestCompareAgentRuns_ToolCallDiffs(t *testing.T) {
	t.Parallel()
	store := &mockCompStore{
		runs: map[string]*domain.JobRun{
			"run-a": {ID: "run-a", Status: domain.StatusCompleted, Attempt: 1},
			"run-b": {ID: "run-b", Status: domain.StatusCompleted, Attempt: 1},
		},
		costs:     map[string]int64{},
		tokens:    map[string]int64{},
		toolCount: map[string]int{"run-a": 5, "run-b": 3},
		toolCalls: map[string][]domain.RunToolCall{
			"run-a": {{ToolName: "search"}, {ToolName: "search"}, {ToolName: "search"}, {ToolName: "analyze"}, {ToolName: "analyze"}},
			"run-b": {{ToolName: "search"}, {ToolName: "search"}, {ToolName: "analyze"}},
		},
	}

	comp, err := CompareAgentRuns(context.Background(), store, "run-a", "run-b")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(comp.ToolCallDiffs) == 0 {
		t.Fatal("expected tool call diffs")
	}
	// Verify at least one diff shows search: 3 vs 2.
	found := false
	for _, d := range comp.ToolCallDiffs {
		if d.ToolName == "search" && d.CountA == 3 && d.CountB == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected search diff (3 vs 2), got %+v", comp.ToolCallDiffs)
	}
}

func TestCompareAgentRuns_OneFailed(t *testing.T) {
	t.Parallel()
	store := &mockCompStore{
		runs: map[string]*domain.JobRun{
			"run-a": {ID: "run-a", Status: domain.StatusCompleted, Attempt: 1},
			"run-b": {ID: "run-b", Status: domain.StatusFailed, Attempt: 1},
		},
		costs:  map[string]int64{},
		tokens: map[string]int64{},
	}

	comp, err := CompareAgentRuns(context.Background(), store, "run-a", "run-b")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if comp.StatusMatch {
		t.Fatal("StatusMatch should be false when one completed and one failed")
	}
}

func TestCompareAgentRuns_RunNotFound(t *testing.T) {
	t.Parallel()
	store := &mockCompStore{
		runs: map[string]*domain.JobRun{
			"run-a": {ID: "run-a", Status: domain.StatusCompleted, Attempt: 1},
		},
	}
	_, err := CompareAgentRuns(context.Background(), store, "run-a", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing run B")
	}
}
