package agents

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"

	"strait/internal/domain"
)

// WhatIfStore defines store methods for what-if replay.
type WhatIfStore interface {
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	ListRunUsage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error)
	ListRunToolCalls(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error)
	SumRunCostMicrousd(ctx context.Context, runID string) (int64, error)
}

// PricingStore looks up model pricing.
type PricingStore interface {
	LookupPricing(ctx context.Context, provider, model string) (inputCostMicrousd, outputCostMicrousd int64, err error)
}

// WhatIfEngine handles model-swap replay and cost estimation.
type WhatIfEngine struct {
	store   WhatIfStore
	pricing PricingStore
	service Service
}

// NewWhatIfEngine creates a new what-if replay engine.
func NewWhatIfEngine(store WhatIfStore, pricing PricingStore, service Service) *WhatIfEngine {
	return &WhatIfEngine{store: store, pricing: pricing, service: service}
}

// EstimateCost estimates what a run would cost with a different model
// without actually executing it.
func (w *WhatIfEngine) EstimateCost(ctx context.Context, runID, targetModel string) (*domain.WhatIfEstimate, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "WhatIfEngine.EstimateCost")
	defer span.End()

	run, err := w.store.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("run not found: %s", runID)
	}
	if !run.Status.IsTerminal() && run.Status != domain.StatusDeadLetter {
		return nil, fmt.Errorf("run %s is not terminal (status: %s)", runID, run.Status)
	}

	// Load original usage records.
	usages, err := w.store.ListRunUsage(ctx, runID, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("list run usage: %w", err)
	}

	// Look up target model pricing. Assume same provider as original.
	provider := ""
	if len(usages) > 0 {
		provider = usages[0].Provider
	}

	inputPrice, outputPrice, err := w.pricing.LookupPricing(ctx, provider, targetModel)
	if err != nil {
		return nil, fmt.Errorf("lookup pricing for %s: %w", targetModel, err)
	}

	// Calculate estimated cost.
	var estimatedCost int64
	var originalCost int64
	originalModel := ""
	for _, u := range usages {
		originalCost += u.CostMicrousd
		if originalModel == "" {
			originalModel = u.Model
		}
		// Estimate cost with new model pricing.
		// Multiply first to preserve precision, then divide with rounding.
		estimatedCost += (int64(u.PromptTokens) * inputPrice + 500_000) / 1_000_000
		estimatedCost += (int64(u.CompletionTokens) * outputPrice + 500_000) / 1_000_000
	}

	costDelta := estimatedCost - originalCost
	savingsPct := float64(0)
	if originalCost > 0 {
		savingsPct = float64(-costDelta) / float64(originalCost) * 100
	}

	return &domain.WhatIfEstimate{
		OriginalRunID: runID,
		TargetModel:   targetModel,
		OriginalModel: originalModel,
		OriginalCost:  originalCost,
		EstimatedCost: estimatedCost,
		CostDelta:     costDelta,
		SavingsPct:    savingsPct,
	}, nil
}

// Replay executes a what-if replay: re-runs LLM calls with a different model
// using cached tool responses from the original run.
func (w *WhatIfEngine) Replay(ctx context.Context, runID, targetModel, projectID, agentID, actor string) (*domain.WhatIfReplayResult, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "WhatIfEngine.Replay")
	defer span.End()

	run, err := w.store.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("run not found: %s", runID)
	}
	if !run.Status.IsTerminal() && run.Status != domain.StatusDeadLetter {
		return nil, fmt.Errorf("run %s is not terminal (status: %s)", runID, run.Status)
	}

	// Load original tool calls for caching.
	toolCalls, err := w.store.ListRunToolCalls(ctx, runID, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("list tool calls: %w", err)
	}

	cached := make([]CachedToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		cached = append(cached, CachedToolCall{
			ToolName: tc.ToolName,
			Input:    tc.Input,
			Output:   tc.Output,
		})
	}

	// Trigger replay via ReplayAgentRun with model override.
	replayRun, err := w.service.ReplayAgentRun(ctx, ReplayAgentRunRequest{
		ProjectID:       projectID,
		AgentID:         agentID,
		OriginalRunID:   runID,
		ConfigOverrides: map[string]any{"model": targetModel},
		Actor:           "whatif:" + actor,
		CachedToolCalls: cached,
	})
	if err != nil {
		return nil, fmt.Errorf("replay agent run: %w", err)
	}

	// Poll for completion.
	deadline := time.Now().Add(5 * time.Minute)
	var finalRun *domain.JobRun
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		r, rErr := w.store.GetRun(ctx, replayRun.ID)
		if rErr != nil {
			return nil, fmt.Errorf("poll run: %w", rErr)
		}
		if r.Status.IsTerminal() || r.Status == domain.StatusDeadLetter {
			finalRun = r
			break
		}
		time.Sleep(2 * time.Second)
	}
	if finalRun == nil {
		return nil, fmt.Errorf("replay timed out")
	}

	// Compute costs.
	originalCost, costErr := w.store.SumRunCostMicrousd(ctx, runID)
	if costErr != nil {
		return nil, fmt.Errorf("sum original run cost: %w", costErr)
	}
	replayCost, costErr2 := w.store.SumRunCostMicrousd(ctx, replayRun.ID)
	if costErr2 != nil {
		return nil, fmt.Errorf("sum replay run cost: %w", costErr2)
	}

	originalDuration := 0
	if run.FinishedAt != nil && run.StartedAt != nil {
		originalDuration = int(run.FinishedAt.Sub(*run.StartedAt).Milliseconds())
	}
	replayDuration := 0
	if finalRun.FinishedAt != nil && finalRun.StartedAt != nil {
		replayDuration = int(finalRun.FinishedAt.Sub(*finalRun.StartedAt).Milliseconds())
	}

	// Get original model from usage.
	usages, _ := w.store.ListRunUsage(ctx, runID, 1, nil)
	originalModel := ""
	if len(usages) > 0 {
		originalModel = usages[0].Model
	}

	return &domain.WhatIfReplayResult{
		OriginalRunID:     runID,
		ReplayRunID:       replayRun.ID,
		TargetModel:       targetModel,
		OriginalModel:     originalModel,
		CostDelta:         replayCost - originalCost,
		OriginalCost:      originalCost,
		ReplayCost:        replayCost,
		LatencyDeltaMs:    replayDuration - originalDuration,
		OriginalLatencyMs: originalDuration,
		ReplayLatencyMs:   replayDuration,
		StatusMatch:       string(run.Status) == string(finalRun.Status),
		OriginalStatus:    string(run.Status),
		ReplayStatus:      string(finalRun.Status),
	}, nil
}
