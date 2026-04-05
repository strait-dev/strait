package billing

import (
	"math"
	"testing"
)

func TestCalculateAgentRunCost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		totalTokens   int64
		toolCallCount int
		wantRunCost   int64
		wantTokenCost int64
		wantToolCost  int64
		wantTotal     int64
	}{
		{
			name:          "typical run: 15K tokens, 10 tool calls",
			totalTokens:   15_000,
			toolCallCount: 10,
			wantRunCost:   1000,
			wantTokenCost: 1500, // 15000 * 100000 / 1000000 = 1500
			wantToolCost:  1000,
			wantTotal:     3500,
		},
		{
			name:          "zero tokens and zero tools",
			totalTokens:   0,
			toolCallCount: 0,
			wantRunCost:   1000,
			wantTokenCost: 0,
			wantToolCost:  0,
			wantTotal:     1000,
		},
		{
			name:          "1M tokens: $0.10 = 100000 microusd",
			totalTokens:   1_000_000,
			toolCallCount: 0,
			wantRunCost:   1000,
			wantTokenCost: 100_000, // 1M * 100000 / 1M = 100000
			wantToolCost:  0,
			wantTotal:     101_000,
		},
		{
			name:          "single tool call",
			totalTokens:   0,
			toolCallCount: 1,
			wantRunCost:   1000,
			wantTokenCost: 0,
			wantToolCost:  100,
			wantTotal:     1100,
		},
		{
			name:          "heavy agent: 10M tokens, 50 tools",
			totalTokens:   10_000_000,
			toolCallCount: 50,
			wantRunCost:   1000,
			wantTokenCost: 1_000_000, // 10M * 100000 / 1M = 1000000
			wantToolCost:  5000,
			wantTotal:     1_006_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runCost, tokenCost, toolCost, totalCost := CalculateAgentRunCost(tt.totalTokens, tt.toolCallCount)
			if runCost != tt.wantRunCost {
				t.Errorf("runCost = %d, want %d", runCost, tt.wantRunCost)
			}
			if tokenCost != tt.wantTokenCost {
				t.Errorf("tokenCost = %d, want %d", tokenCost, tt.wantTokenCost)
			}
			if toolCost != tt.wantToolCost {
				t.Errorf("toolCost = %d, want %d", toolCost, tt.wantToolCost)
			}
			if totalCost != tt.wantTotal {
				t.Errorf("totalCost = %d, want %d", totalCost, tt.wantTotal)
			}
		})
	}
}

func TestCalculateAgentRunCost_LargeValues(t *testing.T) {
	t.Parallel()
	// 1 billion tokens: should produce tokenCost = 1B * 100_000 / 1M = 100_000_000.
	_, tokenCost, _, totalCost := CalculateAgentRunCost(1_000_000_000, 0)
	if tokenCost != 100_000_000 {
		t.Errorf("tokenCost = %d, want 100000000", tokenCost)
	}
	if totalCost < 0 {
		t.Error("totalCost overflowed to negative")
	}
}

func TestCalculateAgentRunCost_Constants(t *testing.T) {
	t.Parallel()
	if AgentRunCostMicrousd != 1000 {
		t.Errorf("AgentRunCostMicrousd = %d, want 1000 ($0.001)", AgentRunCostMicrousd)
	}
	if AgentToolCallCostMicrousd != 100 {
		t.Errorf("AgentToolCallCostMicrousd = %d, want 100 ($0.0001)", AgentToolCallCostMicrousd)
	}
}

func TestWithMeterEventName_ValidNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{MeterComputeOverage, MeterAgentRunOverage, MeterAgentTokenTracking, MeterAgentToolCalls} {
		t.Run(name, func(t *testing.T) {
			// Should not panic.
			opt := WithMeterEventName(name)
			r := &StripeUsageReporter{meterEventName: "default"}
			opt(r)
			if r.meterEventName != name {
				t.Errorf("meterEventName = %q, want %q", r.meterEventName, name)
			}
		})
	}
}

func TestWithMeterEventName_InvalidPanics(t *testing.T) {
	t.Parallel()
	tests := []string{"", "invalid_meter", "compute_overage_typo", "AGENT_RUN_OVERAGE"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("WithMeterEventName(%q) did not panic", name)
				}
			}()
			WithMeterEventName(name)
		})
	}
}

func TestIngest_EmptyCustomerID_Skips(t *testing.T) {
	t.Parallel()
	r := &StripeUsageReporter{secretKey: "sk_test", meterEventName: MeterAgentRunOverage}
	err := r.Ingest(nil, "", "run-123", 1000) //nolint:staticcheck // nil ctx for test
	if err != nil {
		t.Errorf("Ingest with empty customer should skip silently, got: %v", err)
	}
}

func TestIngest_EmptySecretKey_Skips(t *testing.T) {
	t.Parallel()
	r := &StripeUsageReporter{secretKey: "", meterEventName: MeterAgentRunOverage}
	err := r.Ingest(nil, "cus_123", "run-123", 1000) //nolint:staticcheck // nil ctx for test
	if err != nil {
		t.Errorf("Ingest with empty secret key should skip silently, got: %v", err)
	}
}

func TestAgentBillingTimeout_Reasonable(t *testing.T) {
	t.Parallel()
	if AgentBillingTimeout.Seconds() < 5 || AgentBillingTimeout.Seconds() > 30 {
		t.Errorf("AgentBillingTimeout = %v, want between 5s and 30s", AgentBillingTimeout)
	}
}

func TestCalculateAgentRunCost_RunCostAlwaysFlat(t *testing.T) {
	t.Parallel()
	// Run cost should be the same regardless of tokens or tool calls.
	for _, tokens := range []int64{0, 100, 1_000_000, math.MaxInt64 / 200_000} {
		runCost, _, _, _ := CalculateAgentRunCost(tokens, 0)
		if runCost != AgentRunCostMicrousd {
			t.Errorf("tokens=%d: runCost = %d, want flat %d", tokens, runCost, AgentRunCostMicrousd)
		}
	}
}
