package store

import "testing"

func TestComputeAgentHealthScore_NoRuns(t *testing.T) {
	t.Parallel()
	score, level := ComputeAgentHealthScore(&AgentHealthStats{TotalRuns: 0})
	if score != 0 {
		t.Fatalf("score = %f, want 0", score)
	}
	if level != "unknown" {
		t.Fatalf("level = %q, want unknown", level)
	}
}

func TestComputeAgentHealthScore_PerfectHealth(t *testing.T) {
	t.Parallel()
	score, level := ComputeAgentHealthScore(&AgentHealthStats{
		TotalRuns:       100,
		CompletedRuns:   100,
		AvgDurationSecs: 5,
	})
	if score < 90 {
		t.Fatalf("score = %f, want >= 90 for perfect health", score)
	}
	if level != "healthy" {
		t.Fatalf("level = %q, want healthy", level)
	}
}

func TestComputeAgentHealthScore_AllFailed(t *testing.T) {
	t.Parallel()
	score, level := ComputeAgentHealthScore(&AgentHealthStats{
		TotalRuns:       100,
		CompletedRuns:   0,
		FailedRuns:      100,
		AvgDurationSecs: 5,
	})
	if score >= 30 {
		t.Fatalf("score = %f, want < 30 for all failed", score)
	}
	if level != "unhealthy" {
		t.Fatalf("level = %q, want unhealthy", level)
	}
}

func TestComputeAgentHealthScore_OOMPenalty(t *testing.T) {
	t.Parallel()
	// Same failure rate, but OOM errors should penalize more.
	genericScore, _ := ComputeAgentHealthScore(&AgentHealthStats{
		TotalRuns:       100,
		CompletedRuns:   70,
		FailedRuns:      30,
		AvgDurationSecs: 5,
	})
	oomScore, _ := ComputeAgentHealthScore(&AgentHealthStats{
		TotalRuns:       100,
		CompletedRuns:   70,
		FailedRuns:      30,
		OOMRuns:         20,
		TimeoutRuns:     10,
		AvgDurationSecs: 5,
	})
	if oomScore >= genericScore {
		t.Fatalf("OOM score (%f) should be lower than generic score (%f)", oomScore, genericScore)
	}
}

func TestComputeAgentHealthScore_DegradedRange(t *testing.T) {
	t.Parallel()
	score, level := ComputeAgentHealthScore(&AgentHealthStats{
		TotalRuns:       100,
		CompletedRuns:   50,
		FailedRuns:      50,
		AvgDurationSecs: 10,
	})
	if score < 30 || score > 60 {
		t.Fatalf("score = %f, want 30-60 for 50%% success", score)
	}
	if level != "degraded" {
		t.Fatalf("level = %q, want degraded", level)
	}
}

func TestComputeAgentHealthScore_LongDurationPenalty(t *testing.T) {
	t.Parallel()
	fastScore, _ := ComputeAgentHealthScore(&AgentHealthStats{
		TotalRuns:       100,
		CompletedRuns:   100,
		AvgDurationSecs: 10,
	})
	slowScore, _ := ComputeAgentHealthScore(&AgentHealthStats{
		TotalRuns:       100,
		CompletedRuns:   100,
		AvgDurationSecs: 300, // 5 minutes average.
	})
	if slowScore >= fastScore {
		t.Fatalf("slow score (%f) should be lower than fast score (%f)", slowScore, fastScore)
	}
}
