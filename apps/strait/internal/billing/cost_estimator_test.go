package billing

import (
	"testing"

	"strait/internal/compute"
)

func TestEstimateJobCost_ValidPreset(t *testing.T) {
	wantCost := compute.CostMicro * 60
	est, err := EstimateJobCost("micro", 60, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CostMicro != wantCost {
		t.Errorf("expected cost %d, got %d", wantCost, est.CostMicro)
	}
	if est.RateMicroPerSec != compute.CostMicro {
		t.Errorf("expected rate %d, got %d", compute.CostMicro, est.RateMicroPerSec)
	}
	wantRuns := int64(10000) / wantCost
	if est.CreditRunsRemaining != wantRuns {
		t.Errorf("expected %d runs remaining, got %d", wantRuns, est.CreditRunsRemaining)
	}
	if est.CheaperAlternative != nil {
		t.Errorf("expected no cheaper alternative for micro preset")
	}
}

func TestEstimateJobCost_WithCheaperAlternative(t *testing.T) {
	wantCost := compute.CostMedium1x * 60
	est, err := EstimateJobCost("medium-1x", 60, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CostMicro != wantCost {
		t.Errorf("expected cost %d, got %d", wantCost, est.CostMicro)
	}
	if est.CheaperAlternative == nil {
		t.Fatal("expected a cheaper alternative")
	}
	if est.CheaperAlternative.Preset != "micro" {
		t.Errorf("expected cheaper alternative to be micro, got %s", est.CheaperAlternative.Preset)
	}
	if est.CheaperAlternative.SavingsPct <= 0 {
		t.Errorf("expected positive savings percentage, got %f", est.CheaperAlternative.SavingsPct)
	}
}

func TestEstimateJobCost_UnknownPreset(t *testing.T) {
	_, err := EstimateJobCost("nonexistent", 60, 10000)
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
}

func TestEstimateJobCost_ZeroCredit(t *testing.T) {
	est, err := EstimateJobCost("micro", 60, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CreditRunsRemaining != 0 {
		t.Errorf("expected 0 runs remaining with 0 credit, got %d", est.CreditRunsRemaining)
	}
}
