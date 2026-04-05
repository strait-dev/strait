package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// AgentRunCostMicrousd is the flat per-run cost in micro-USD ($0.001).
const AgentRunCostMicrousd int64 = 1000

// AgentToolCallCostMicrousd is the per-tool-call cost in micro-USD ($0.0001).
const AgentToolCallCostMicrousd int64 = 100

// AgentBillingReporter sends agent run usage events to three separate Stripe
// billing meters: execution runs, token tracking, and tool calls.
// Safe for concurrent use.
type AgentBillingReporter struct {
	runReporter   *StripeUsageReporter
	tokenReporter *StripeUsageReporter
	toolReporter  *StripeUsageReporter
	logger        *slog.Logger
}

// NewAgentBillingReporter creates a reporter that meters agent runs across
// three Stripe billing meters. The opts are applied to all three internal
// reporters (e.g. WithUsageReporterMetrics).
func NewAgentBillingReporter(secretKey string, logger *slog.Logger, opts ...StripeUsageReporterOption) *AgentBillingReporter {
	if logger == nil {
		logger = slog.Default()
	}
	makeReporter := func(meter string) *StripeUsageReporter {
		combined := append([]StripeUsageReporterOption{WithMeterEventName(meter)}, opts...)
		return NewStripeUsageReporter(secretKey, logger, combined...)
	}
	return &AgentBillingReporter{
		runReporter:   makeReporter(MeterAgentRunOverage),
		tokenReporter: makeReporter(MeterAgentTokenTracking),
		toolReporter:  makeReporter(MeterAgentToolCalls),
		logger:        logger,
	}
}

// IngestAgentRunUsage sends three Stripe meter events for a completed agent run.
// Each event uses a unique identifier for Stripe deduplication.
//
// Meters:
//   - agent_run_overage:    1000 micro-USD (flat $0.001/run)
//   - agent_token_tracking: raw token count (Stripe pricing handles per-unit rate)
//   - agent_tool_calls:     raw tool call count
func (r *AgentBillingReporter) IngestAgentRunUsage(
	ctx context.Context,
	stripeCustomerID string,
	runID string,
	totalTokens int64,
	toolCallCount int,
) error {
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Execution run — always billed.
	record(r.runReporter.Ingest(ctx, stripeCustomerID, runID, AgentRunCostMicrousd))

	// Token tracking — only bill if tokens were consumed.
	if totalTokens > 0 {
		record(r.tokenReporter.Ingest(ctx, stripeCustomerID, runID+":tokens", totalTokens))
	}

	// Tool calls — only bill if tools were called.
	if toolCallCount > 0 {
		record(r.toolReporter.Ingest(ctx, stripeCustomerID, runID+":tools", int64(toolCallCount)))
	}

	if firstErr != nil {
		return fmt.Errorf("agent billing ingestion: %w", firstErr)
	}
	return nil
}

// CalculateAgentRunCost computes the total cost in micro-USD for an agent run.
func CalculateAgentRunCost(totalTokens int64, toolCallCount int) (int64, int64, int64, int64) {
	runCost := AgentRunCostMicrousd
	// Token cost: $0.10 per 1M tokens = 100,000 micro-USD per 1M tokens.
	tokenCost := (totalTokens * 100_000) / 1_000_000
	toolCost := int64(toolCallCount) * AgentToolCallCostMicrousd
	totalCost := runCost + tokenCost + toolCost
	return runCost, tokenCost, toolCost, totalCost
}

// AgentBillingTimeout is the maximum duration for fire-and-forget Stripe calls.
const AgentBillingTimeout = 10 * time.Second
