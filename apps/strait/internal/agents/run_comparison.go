package agents

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"
)

// AgentRunComparison holds the side-by-side diff between two agent runs.
type AgentRunComparison struct {
	RunA          AgentRunSummary `json:"run_a"`
	RunB          AgentRunSummary `json:"run_b"`
	CostDiff      int64           `json:"cost_diff_microusd"`
	TokenDiff     int64           `json:"token_diff"`
	DurationDiff  float64         `json:"duration_diff_secs"`
	StatusMatch   bool            `json:"status_match"`
	ModelMatch    bool            `json:"model_match"`
	ToolCallDiffs []ToolCallDiff  `json:"tool_call_diffs,omitempty"`
}

// AgentRunSummary holds aggregated metrics for a single agent run.
type AgentRunSummary struct {
	RunID         string  `json:"run_id"`
	Status        string  `json:"status"`
	Model         string  `json:"model"`
	TotalTokens   int64   `json:"total_tokens"`
	CostMicrousd  int64   `json:"cost_microusd"`
	DurationSecs  float64 `json:"duration_secs"`
	ToolCallCount int     `json:"tool_call_count"`
	ErrorClass    string  `json:"error_class,omitempty"`
	Attempt       int     `json:"attempt"`
}

// ToolCallDiff shows how tool usage differs between two runs.
type ToolCallDiff struct {
	ToolName string `json:"tool_name"`
	CountA   int    `json:"count_a"`
	CountB   int    `json:"count_b"`
}

// RunComparisonStore defines the store methods needed for run comparison.
type RunComparisonStore interface {
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	SumRunCostMicrousd(ctx context.Context, runID string) (int64, error)
	SumRunTotalTokens(ctx context.Context, runID string) (int64, error)
	CountRunToolCalls(ctx context.Context, runID string) (int, error)
	ListRunToolCalls(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error)
}

// CompareAgentRuns builds a side-by-side comparison of two agent runs.
func CompareAgentRuns(ctx context.Context, store RunComparisonStore, runIDA, runIDB string) (*AgentRunComparison, error) {
	summaryA, err := buildRunSummary(ctx, store, runIDA)
	if err != nil {
		return nil, fmt.Errorf("run A (%s): %w", runIDA, err)
	}
	summaryB, err := buildRunSummary(ctx, store, runIDB)
	if err != nil {
		return nil, fmt.Errorf("run B (%s): %w", runIDB, err)
	}

	comp := &AgentRunComparison{
		RunA:         *summaryA,
		RunB:         *summaryB,
		CostDiff:     summaryA.CostMicrousd - summaryB.CostMicrousd,
		TokenDiff:    summaryA.TotalTokens - summaryB.TotalTokens,
		DurationDiff: summaryA.DurationSecs - summaryB.DurationSecs,
		StatusMatch:  summaryA.Status == summaryB.Status,
		ModelMatch:   summaryA.Model == summaryB.Model,
	}

	// Build tool call diffs.
	toolsA := countToolCalls(ctx, store, runIDA)
	toolsB := countToolCalls(ctx, store, runIDB)
	allTools := make(map[string]struct{})
	for t := range toolsA {
		allTools[t] = struct{}{}
	}
	for t := range toolsB {
		allTools[t] = struct{}{}
	}
	for tool := range allTools {
		if toolsA[tool] != toolsB[tool] {
			comp.ToolCallDiffs = append(comp.ToolCallDiffs, ToolCallDiff{
				ToolName: tool,
				CountA:   toolsA[tool],
				CountB:   toolsB[tool],
			})
		}
	}

	return comp, nil
}

func buildRunSummary(ctx context.Context, store RunComparisonStore, runID string) (*AgentRunSummary, error) {
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	cost, _ := store.SumRunCostMicrousd(ctx, runID)
	tokens, _ := store.SumRunTotalTokens(ctx, runID)
	toolCount, _ := store.CountRunToolCalls(ctx, runID)

	var durationSecs float64
	if run.StartedAt != nil && run.FinishedAt != nil {
		durationSecs = run.FinishedAt.Sub(*run.StartedAt).Seconds()
	}

	return &AgentRunSummary{
		RunID:         run.ID,
		Status:        string(run.Status),
		TotalTokens:   tokens,
		CostMicrousd:  cost,
		DurationSecs:  durationSecs,
		ToolCallCount: toolCount,
		Attempt:       run.Attempt,
	}, nil
}

func countToolCalls(ctx context.Context, store RunComparisonStore, runID string) map[string]int {
	calls, err := store.ListRunToolCalls(ctx, runID, 1000, nil)
	if err != nil {
		return nil
	}
	counts := make(map[string]int)
	for _, c := range calls {
		counts[c.ToolName]++
	}
	return counts
}
